// Package pics 人物头像图片服务。
//
// 前端首次请求某个人物头像时若本地未保存，则通过 next.bgm.tv 的
// API（HTTP Bearer 鉴权，Key 存放于 data/config.json 或环境变量）
// 异步抓取 images.large，提取其中相对路径部分（如
// a6/e8/1_prsn_k7wpt.jpg）存入独立 SQLite 库（bgm_pic，表 person），
// 并按需拼回不同尺寸的 lain.bgm.tv CDN URL 供展示。
//
// Resolve 返回三种状态供前端轮询：
//   - ok      已有可用 URL
//   - pending 已触发后台抓取，稍后重试
//   - failed  无法提供（未配置 Key / 上游失败进入负缓存期）
package pics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bangumi-subject-go/internal/db"
)

const (
	apiBase   = "https://next.bgm.tv/p1/persons/"
	cdnHost   = "https://lain.bgm.tv"
	crtPrefix = "/pic/crt/l/"

	fetchConcurrency = 3                // 上游并发抓取上限
	fetchTimeout     = 15 * time.Second // 单次上游请求超时
	failedTTL        = 10 * time.Minute // 失败负缓存时长，避免轮询期间反复打上游
	slotWait         = time.Minute      // 等待并发额度的最长时间
	maxRedirectHops  = 5                // 合并重定向跟随上限
)

// Service 独立图片库连接 + 抓取调度。
type Service struct {
	conn     *sql.DB
	key      string
	http     *http.Client
	sem      chan struct{} // 并发额度
	inflight sync.Map      // personID -> struct{}，去重进行中的抓取
	failed   sync.Map      // personID -> time.Time，失败负缓存
}

// Open 打开（或创建）bgm_pic SQLite 库并建表 person(id, url)。
func Open(path, apiKey string) (*Service, error) {
	conn, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS person (
		id  INTEGER PRIMARY KEY,
		url TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		db.Close(conn)
		return nil, fmt.Errorf("pics: 建表 person: %w", err)
	}
	return &Service{
		conn: conn,
		key:  apiKey,
		http: &http.Client{Timeout: fetchTimeout},
		sem:  make(chan struct{}, fetchConcurrency),
	}, nil
}

// Close 关闭数据库连接。
func (s *Service) Close() {
	if s != nil && s.conn != nil {
		db.Close(s.conn)
	}
}

// HasKey 是否已配置上游 API Key（未配置时头像功能停用）。
func (s *Service) HasKey() bool { return s != nil && s.key != "" }

// LoadAPIKey 读取 next.bgm.tv API Key：优先环境变量 BANGUMI_API_KEY，
// 其次 data 目录下 config.json 的 bgm_api_key 字段。均未配置时返回
// 空串（头像功能停用，Resolve 恒为 failed）。
func LoadAPIKey(dataDir string) string {
	if v := strings.TrimSpace(os.Getenv("BANGUMI_API_KEY")); v != "" {
		return v
	}
	b, err := os.ReadFile(filepath.Join(dataDir, "config.json"))
	if err != nil {
		return ""
	}
	var conf struct {
		BgmApiKey string `json:"bgm_api_key"`
	}
	if json.Unmarshal(b, &conf) != nil {
		return ""
	}
	return strings.TrimSpace(conf.BgmApiKey)
}

// Resolve 解析人物头像状态与 URL（URL 仅在 ok 时非空）。
func (s *Service) Resolve(id int64) (status, url string) {
	var rel string
	err := s.conn.QueryRow(`SELECT url FROM person WHERE id = ?`, id).Scan(&rel)
	switch {
	case err == nil && rel != "":
		return "ok", BuildURL(rel, "l")
	case err == nil:
		return "failed", "" // 已确认无头像（空标记），无需再抓
	case !errors.Is(err, sql.ErrNoRows):
		log.Printf("pics: 查询人物 %d 头像: %v", id, err)
	}
	if s.key == "" {
		return "failed", ""
	}
	if t, ok := s.failed.Load(id); ok {
		if time.Since(t.(time.Time)) < failedTTL {
			return "failed", ""
		}
		s.failed.Delete(id)
	}
	s.scheduleFetch(id)
	return "pending", ""
}

// scheduleFetch 异步抓取（同一 id 去重，受并发额度限制）。
func (s *Service) scheduleFetch(id int64) {
	if _, loaded := s.inflight.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	go func() {
		defer s.inflight.Delete(id)
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-time.After(slotWait):
			return // 等不到额度则放弃，前端下次轮询会再次触发
		}
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		if err := s.fetch(ctx, id); err != nil {
			s.failed.Store(id, time.Now())
			log.Printf("pics: 抓取人物 %d 头像失败: %v", id, err)
		}
	}()
}

// apiPerson next.bgm.tv 响应中本服务关心的字段。
type apiPerson struct {
	ID       int64 `json:"id"`
	Redirect int64 `json:"redirect"` // 人物被合并时指向规范 ID
	Images   struct {
		Large string `json:"large"`
	} `json:"images"`
}

// fetch 抓取并保存人物头像相对路径。
// 兼容两类特殊情况：
//   - redirect 非零：人物已合并到其他 ID，跟随重定向取规范人物的头像；
//   - 无 images.large 且无重定向：该人物确实没有头像，写入空标记，
//     之后 Resolve 直接返回 failed，不再重复请求上游。
func (s *Service) fetch(ctx context.Context, id int64) error {
	cur := id
	for hop := 0; hop < maxRedirectHops; hop++ {
		body, err := s.getPerson(ctx, cur)
		if err != nil {
			return err
		}
		if rel := ExtractRel(body.Images.Large); rel != "" {
			return s.store(id, rel)
		}
		if body.Redirect > 0 && body.Redirect != cur {
			log.Printf("pics: 人物 %d 已合并至 %d，跟随重定向", id, body.Redirect)
			cur = body.Redirect
			continue
		}
		if err := s.store(id, ""); err != nil {
			return err
		}
		log.Printf("pics: 人物 %d 没有头像图片", id)
		return nil
	}
	return fmt.Errorf("人物 %d 重定向层级超过上限", id)
}

func (s *Service) getPerson(ctx context.Context, personID int64) (*apiPerson, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		apiBase+strconv.FormatInt(personID, 10), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("User-Agent", "bangumi-subject-go/local")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New("上游 404（人物不存在）")
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("上游 429（请求过于频繁）")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("上游 HTTP %d", resp.StatusCode)
	}
	var body apiPerson
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}
	return &body, nil
}

func (s *Service) store(personID int64, rel string) error {
	_, err := s.conn.Exec(`INSERT INTO person(id, url) VALUES(?, ?)
		ON CONFLICT(id) DO UPDATE SET url = excluded.url`, personID, rel)
	if err != nil {
		return fmt.Errorf("写入 bgm_pic.person: %w", err)
	}
	return nil
}

// ExtractRel 从 images.large 提取需持久化的相对路径（去掉域名、
// /pic/crt/l/ 前缀与查询参数）：保留扩展名以兼容 jpg/png 等格式。
func ExtractRel(large string) string {
	i := strings.Index(large, crtPrefix)
	if i < 0 {
		return ""
	}
	rel := large[i+len(crtPrefix):]
	if j := strings.IndexAny(rel, "?#"); j >= 0 {
		rel = rel[:j]
	}
	return rel
}

// BuildURL 输入保存的相对路径与尺寸类型，返回可直接展示的完整 URL。
// size 支持 l/large、m/medium、s/small、g/grid，默认 l。
func BuildURL(rel, size string) string {
	var prefix string
	switch strings.ToLower(size) {
	case "m", "medium":
		prefix = cdnHost + "/r/200" + crtPrefix
	case "s", "small":
		prefix = cdnHost + "/r/100" + crtPrefix
	case "g", "grid":
		prefix = cdnHost + "/r/100x100" + crtPrefix
	default:
		prefix = cdnHost + crtPrefix
	}
	return prefix + rel
}
