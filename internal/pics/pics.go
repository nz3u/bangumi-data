// Package pics 图片解析服务（人物头像 / 条目封面 / 角色头像）。
//
// 前端首次请求某张图片时若本地未保存，则通过 next.bgm.tv 的
// API（HTTP Bearer 鉴权，Key 存放于 data/config.json 或环境变量）
// 异步抓取 images 中的 URL，提取其中相对路径部分（如
// a6/e8/1_prsn_k7wpt.jpg）存入独立 SQLite 库（bgm_pic，按类型分表
// person / subject / character），并按需拼回不同尺寸的 lain.bgm.tv
// CDN URL 供展示。三类类型共用同一套抓取队列、并发额度与负缓存，
// 仅上游 API 段、CDN 路径前缀与存储表不同：
//
//	person    https://next.bgm.tv/p1/persons/{id}     lain /pic/crt/l/
//	subject   https://next.bgm.tv/p1/subjects/{id}    lain /pic/cover/l/
//	character https://next.bgm.tv/p1/characters/{id}  lain /pic/crt/l/
//
// Resolve 返回三种状态供前端轮询：
//   - ok      已有可用 URL
//   - pending 已触发后台抓取，稍后重试
//   - failed  无法提供（未配置 Key / 上游失败进入负缓存期 / 确认无图）
package pics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bangumi-subject-go/internal/config"
	"bangumi-subject-go/internal/db"
)

const (
	apiHost   = "https://next.bgm.tv/p1/"
	cdnHost   = "https://lain.bgm.tv"
	crtBase   = "/pic/crt/"   // 人物 / 角色图片路径前缀（后接尺寸字母）
	coverBase = "/pic/cover/" // 条目封面路径前缀（后接尺寸字母）

	fetchConcurrency = 3                // 上游并发抓取上限
	fetchTimeout     = 15 * time.Second // 单次上游请求超时
	failedTTL        = 10 * time.Minute // 失败负缓存时长，避免轮询期间反复打上游
	slotWait         = time.Minute      // 等待并发额度的最长时间
	maxRedirectHops  = 5                // 合并重定向跟随上限
)

// 受支持的图片类型（同时是 bgm_pic 库中的表名）。
const (
	KindPerson    = "person"
	KindSubject   = "subject"
	KindCharacter = "character"
)

// kindConf 单一类型的差异化配置。
type kindConf struct {
	table   string // 存储表名（白名单，可安全拼入 SQL）
	apiSeg  string // next.bgm.tv API 路径段
	picBase string // lain CDN 图片路径前缀（尺寸字母之前）
}

var kinds = map[string]kindConf{
	KindPerson:    {table: KindPerson, apiSeg: "persons", picBase: crtBase},
	KindSubject:   {table: KindSubject, apiSeg: "subjects", picBase: coverBase},
	KindCharacter: {table: KindCharacter, apiSeg: "characters", picBase: crtBase},
}

// ValidKind 是否为受支持的图片类型（API 层入参校验用）。
func ValidKind(kind string) bool {
	_, ok := kinds[kind]
	return ok
}

// picKey 区分类型的抓取去重 / 负缓存键。
type picKey struct {
	kind string
	id   int64
}

// Service 独立图片库连接 + 抓取调度。
type Service struct {
	conn     *sql.DB
	key      string
	http     *http.Client
	sem      chan struct{} // 并发额度
	inflight sync.Map      // picKey -> struct{}，去重进行中的抓取
	failed   sync.Map      // picKey -> time.Time，失败负缓存
}

// Open 打开（或创建）bgm_pic SQLite 库并确保三张分表存在。
// CREATE TABLE IF NOT EXISTS 同时兼容两种情况：
// 旧库升级（仅有 person 表，补建 subject / character）与全新安装。
func Open(path, apiKey string) (*Service, error) {
	conn, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	for _, k := range []string{KindPerson, KindSubject, KindCharacter} {
		if _, err := conn.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id  INTEGER PRIMARY KEY,
			url TEXT NOT NULL DEFAULT ''
		)`, k)); err != nil {
			db.Close(conn)
			return nil, fmt.Errorf("pics: 建表 %s: %w", k, err)
		}
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

// HasKey 是否已配置上游 API Key（未配置时图片功能停用）。
func (s *Service) HasKey() bool { return s != nil && s.key != "" }

// LoadAPIKey 读取 next.bgm.tv API Key：优先环境变量 BANGUMI_API_KEY，
// 其次 data 目录下 config.json 的 bgm_api_key 字段。均未配置时返回
// 空串（图片功能停用，Resolve 恒为 failed）。
func LoadAPIKey(dataDir string) string { return config.LoadAPIKey(dataDir) }

// Resolve 解析指定类型图片的状态与 URL（URL 仅在 ok 时非空）。
// size 决定返回的 CDN 尺寸（l/m/s/g，空或未知值按 l 处理）。
func (s *Service) Resolve(kind string, id int64, size string) (status, url string) {
	conf, ok := kinds[kind]
	if !ok {
		return "failed", ""
	}
	key := picKey{kind: kind, id: id}
	var rel string
	err := s.conn.QueryRow(`SELECT url FROM `+conf.table+` WHERE id = ?`, id).Scan(&rel)
	switch {
	case err == nil && rel != "":
		return "ok", BuildURL(kind, rel, size)
	case err == nil:
		return "failed", "" // 已确认无图片（空标记），无需再抓
	case !errors.Is(err, sql.ErrNoRows):
		log.Printf("pics: 查询 %s %d 图片: %v", kind, id, err)
	}
	if s.key == "" {
		return "failed", ""
	}
	if t, ok := s.failed.Load(key); ok {
		if time.Since(t.(time.Time)) < failedTTL {
			return "failed", ""
		}
		s.failed.Delete(key)
	}
	s.scheduleFetch(conf, key)
	return "pending", ""
}

// scheduleFetch 异步抓取（同一 类型+id 去重，受并发额度限制）。
func (s *Service) scheduleFetch(conf kindConf, key picKey) {
	if _, loaded := s.inflight.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	go func() {
		defer s.inflight.Delete(key)
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-time.After(slotWait):
			return // 等不到额度则放弃，前端下次轮询会再次触发
		}
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		if err := s.fetch(ctx, conf, key); err != nil {
			s.failed.Store(key, time.Now())
			log.Printf("pics: 抓取 %s %d 图片失败: %v", key.kind, key.id, err)
		}
	}()
}

// apiEntity next.bgm.tv 响应中本服务关心的字段
// （person / subject / character 三类响应结构兼容）。
type apiEntity struct {
	ID       int64 `json:"id"`
	Redirect int64 `json:"redirect"` // 条目被合并时指向规范 ID
	Images   struct {
		Large  string `json:"large"`
		Common string `json:"common"`
		Medium string `json:"medium"`
		Small  string `json:"small"`
		Grid   string `json:"grid"`
	} `json:"images"`
}

// firstImageURL 依次尝试各尺寸字段，返回第一个可提取出相对路径的原始 URL。
func (e *apiEntity) firstImageURL(picBase string) string {
	for _, u := range []string{e.Images.Large, e.Images.Common, e.Images.Medium, e.Images.Small, e.Images.Grid} {
		if rel := extractRel(u, picBase); rel != "" {
			return u
		}
	}
	return ""
}

// fetch 抓取并保存图片相对路径。
// 兼容两类特殊情况：
//   - redirect 非零：已合并到其他 ID，跟随重定向取规范条目的图片；
//   - 无任何可用 image 且无重定向：确实没有图片，写入空标记，
//     之后 Resolve 直接返回 failed，不再重复请求上游。
func (s *Service) fetch(ctx context.Context, conf kindConf, key picKey) error {
	cur := key.id
	for hop := 0; hop < maxRedirectHops; hop++ {
		body, err := s.getEntity(ctx, conf, cur)
		if err != nil {
			return err
		}
		if raw := body.firstImageURL(conf.picBase); raw != "" {
			return s.store(conf, key.id, extractRel(raw, conf.picBase))
		}
		if body.Redirect > 0 && body.Redirect != cur {
			log.Printf("pics: %s %d 已合并至 %d，跟随重定向", conf.table, key.id, body.Redirect)
			cur = body.Redirect
			continue
		}
		if err := s.store(conf, key.id, ""); err != nil {
			return err
		}
		log.Printf("pics: %s %d 没有图片", conf.table, key.id)
		return nil
	}
	return fmt.Errorf("%s %d 重定向层级超过上限", conf.table, key.id)
}

func (s *Service) getEntity(ctx context.Context, conf kindConf, id int64) (*apiEntity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		apiHost+conf.apiSeg+"/"+strconv.FormatInt(id, 10), nil)
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
		return nil, errors.New("上游 404（不存在）")
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("上游 429（请求过于频繁）")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("上游 HTTP %d", resp.StatusCode)
	}
	var body apiEntity
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}
	return &body, nil
}

func (s *Service) store(conf kindConf, id int64, rel string) error {
	_, err := s.conn.Exec(`INSERT INTO `+conf.table+`(id, url) VALUES(?, ?)
		ON CONFLICT(id) DO UPDATE SET url = excluded.url`, id, rel)
	if err != nil {
		return fmt.Errorf("写入 bgm_pic.%s: %w", conf.table, err)
	}
	return nil
}

// extractRel 从图片 URL 提取需持久化的相对路径：定位 picBase 之后的部分，
// 去掉查询参数；若紧随前缀的是单字母尺寸目录（l/c/m/s/g/r，medium/grid
// 等衍生尺寸的路径形态），一并归一化去掉，使不同尺寸指向同一条记录。
// 非 CDN 图片 URL 返回空串。
func extractRel(rawURL, picBase string) string {
	i := strings.Index(rawURL, picBase)
	if i < 0 {
		return ""
	}
	rel := rawURL[i+len(picBase):]
	if j := strings.IndexAny(rel, "?#"); j >= 0 {
		rel = rel[:j]
	}
	if len(rel) >= 2 && rel[1] == '/' && strings.IndexByte("lcmsgr", rel[0]) >= 0 {
		rel = rel[2:]
	}
	return rel
}

// BuildURL 输入类型、保存的相对路径与尺寸，返回可直接展示的完整 URL。
// kind 取 person / subject / character；size 支持 l/large、m/medium、
// s/small、g/grid，默认 l。
func BuildURL(kind, rel, size string) string {
	conf, ok := kinds[kind]
	if !ok || rel == "" {
		return ""
	}
	var prefix string
	switch strings.ToLower(size) {
	case "m", "medium":
		prefix = cdnHost + "/r/200" + conf.picBase + "l/"
	case "s", "small":
		prefix = cdnHost + "/r/100" + conf.picBase + "l/"
	case "g", "grid":
		prefix = cdnHost + "/r/100x100" + conf.picBase + "l/"
	default:
		prefix = cdnHost + conf.picBase + "l/"
	}
	return prefix + rel
}
