// Package pics 图片解析服务（人物头像 / 条目封面 / 角色头像）。
//
// 前端首次请求某张图片时若本地未保存，则异步抓取上游 API 响应
// images 中的 URL，提取其中相对路径部分（如 a6/e8/1_prsn_k7wpt.jpg）
// 存入独立 SQLite 库（bgm_pic，按类型分表 person / subject /
// character），并按需拼回不同尺寸的 lain.bgm.tv CDN URL 供展示。
//
// 上游按优先级依次尝试，任一成功即采用其结果：
//  1. next API https://next.bgm.tv/p1/（首选；HTTP Bearer 鉴权，
//     Key 存放于 data/config.json 或环境变量）
//  2. v0 API   https://api.bgm.tv/v0/（官方公开 API，next 不可用时回退）
//
// 两类接口的 images 字段结构一致，图片提取逻辑完全相同；已合并
// 条目 next 在 JSON 中给出 redirect 字段，v0 则返回 HTTP 302 由
// HTTP 客户端自动跟随，最终都能取到规范条目的图片。两个上游共用
// 同一超时设定（各 2.5s，合计最长 5s），任一失败（含超时）立即
// 尝试另一个，均失败时不入库，仅进入失败负缓存（failedTTL），
// 期间 /api/pics 返回 502，负缓存过期后再次轮询会重新触发抓取。
//
// 三类类型共用同一套抓取队列、并发额度与负缓存，仅上游 API 段、
// CDN 路径前缀与存储表不同：
//
//	person    {next|v0}/persons/{id}     lain /pic/crt/l/
//	subject   {next|v0}/subjects/{id}    lain /pic/cover/l/
//	character {next|v0}/characters/{id}  lain /pic/crt/l/
//
// Resolve 返回四种状态供前端轮询：
//   - ok          已有可用 URL
//   - pending     已触发后台抓取，稍后重试
//   - unavailable 上游暂时不可用（双上游均失败后的负缓存期内），可稍后重试
//   - failed      无法提供（未配置 Key / 确认无图），前端停止轮询
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
	nextAPIHost = "https://next.bgm.tv/p1/" // 首选上游：next API（需 Bearer Key）
	v0APIHost   = "https://api.bgm.tv/v0/"  // 回退上游：官方 v0 API（Key 可选，附带亦有效）
	cdnHost     = "https://lain.bgm.tv"
	crtBase     = "/pic/crt/"   // 人物 / 角色图片路径前缀（后接尺寸字母）
	coverBase   = "/pic/cover/" // 条目封面路径前缀（后接尺寸字母）

	fetchConcurrency = 3                    // 上游并发抓取上限
	fetchTimeout     = 2500 * time.Millisecond // 单个上游请求超时：next 与 v0 共用同一设定，两段合计最长 5s
	failedTTL        = 10 * time.Minute     // 失败负缓存时长，避免轮询期间反复打上游
	slotWait         = time.Minute          // 等待并发额度的最长时间
	maxRedirectHops  = 5                    // 合并重定向跟随上限
)

// 受支持的图片类型（同时是 bgm_pic 库中的表名）。
const (
	KindPerson    = "person"
	KindSubject   = "subject"
	KindCharacter = "character"
)

// Resolve 系列方法返回的状态值。
const (
	StatusOK          = "ok"          // 已有可用 URL
	StatusPending     = "pending"     // 已触发后台抓取，稍后重试
	StatusUnavailable = "unavailable" // 双上游均失败的负缓存期内，可稍后重试
	StatusFailed      = "failed"      // 终态：未配置 Key / 确认无图
)

// kindConf 单一类型的差异化配置。
type kindConf struct {
	table   string // 存储表名（白名单，可安全拼入 SQL）
	apiSeg  string // 上游 API 路径段（next 与 v0 同名）
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
	nextHost string // 首选上游根地址（测试可注入本地服务）
	v0Host   string // 回退上游根地址（测试可注入本地服务）
	sem      chan struct{}  // 并发额度
	inflight sync.Map       // picKey -> struct{}，去重进行中的抓取
	failed   sync.Map       // picKey -> time.Time，失败负缓存
	stopCh   chan struct{}  // 停止后台清理协程
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
	stopCh := make(chan struct{})
	svc := &Service{
		conn:     conn,
		key:      apiKey,
		http:     &http.Client{Timeout: fetchTimeout},
		nextHost: nextAPIHost,
		v0Host:   v0APIHost,
		sem:      make(chan struct{}, fetchConcurrency),
		stopCh:   stopCh,
	}
	go svc.cleanupLoop(stopCh)
	return svc, nil
}

// Close 关闭后台清理协程并断开数据库连接。
func (s *Service) Close() {
	if s != nil {
		close(s.stopCh)
		if s.conn != nil {
			db.Close(s.conn)
		}
	}
}

// cleanupLoop 每 10 分钟扫描 failed 负缓存，清除已过期条目，
// 防止 sync.Map 无限增长导致内存泄漏。
func (s *Service) cleanupLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			s.failed.Range(func(key, value any) bool {
				if now.Sub(value.(time.Time)) >= failedTTL {
					s.failed.Delete(key)
				}
				return true
			})
		}
	}
}

// HasKey 是否已配置上游 API Key（未配置时图片功能停用）。
func (s *Service) HasKey() bool { return s != nil && s.key != "" }

// LoadAPIKey 读取 next.bgm.tv API Key：优先环境变量 BANGUMI_API_KEY，
// 其次 data 目录下 config.json 的 bgm_api_key 字段。均未配置时返回
// 空串（图片功能停用，Resolve 恒为 failed）。
func LoadAPIKey(dataDir string) string { return config.LoadAPIKey(dataDir) }

// Resolve 解析指定类型图片的状态与完整 URL（含 lain CDN 主机，兼容旧调用方）。
// size 决定返回的 CDN 尺寸（l/m/s/g，空或未知值按 l 处理）。
func (s *Service) Resolve(kind string, id int64, size string) (status, url string) {
	status, rel := s.resolveRel(kind, id)
	return status, BuildURL(kind, rel, size)
}

// ResolvePath 同 Resolve，但返回不含主机的资源路径
// （如 /pic/crt/l/a6/e8/1.jpg），由前端按自身设置拼接主机。
func (s *Service) ResolvePath(kind string, id int64, size string) (status, path string) {
	status, rel := s.resolveRel(kind, id)
	return status, PathURL(kind, rel, size)
}

// resolveRel 查询/触发抓取，返回状态与相对路径（rel 为空表示无图或失败）。
func (s *Service) resolveRel(kind string, id int64) (status, rel string) {
	conf, ok := kinds[kind]
	if !ok {
		return StatusFailed, ""
	}
	key := picKey{kind: kind, id: id}
	var stored string
	err := s.conn.QueryRow(`SELECT url FROM `+conf.table+` WHERE id = ?`, id).Scan(&stored)
	switch {
	case err == nil && stored != "":
		return StatusOK, stored
	case err == nil:
		return StatusFailed, "" // 已确认无图片（空标记），无需再抓
	case !errors.Is(err, sql.ErrNoRows):
		log.Printf("pics: 查询 %s %d 图片: %v", kind, id, err)
	}
	if s.key == "" {
		return StatusFailed, ""
	}
	if t, ok := s.failed.Load(key); ok {
		if time.Since(t.(time.Time)) < failedTTL {
			return StatusUnavailable, "" // 双上游均失败后的负缓存期内
		}
		s.failed.Delete(key)
	}
	s.scheduleFetch(conf, key)
	return StatusPending, ""
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
		// 超时预算由 getEntity 内部统一控制（next / v0 共用 5s）。
		if err := s.fetch(context.Background(), conf, key); err != nil {
			s.failed.Store(key, time.Now())
			log.Printf("pics: 抓取 %s %d 图片失败: %v", key.kind, key.id, err)
		}
	}()
}

// apiEntity 上游 API 响应中本服务关心的字段
// （next 与 v0 的 person / subject / character 响应结构兼容）。
type apiEntity struct {
	ID       int64 `json:"id"`
	Redirect int64 `json:"redirect"` // next API：条目被合并时指向规范 ID（v0 经 HTTP 302 自动跟随，无此字段）
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
//   - redirect 非零（next）：已合并到其他 ID，跟随重定向取规范条目的图片；
//     v0 对已合并条目直接返回 HTTP 302，由 HTTP 客户端自动跟随；
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

// getEntity 优先请求 next API，失败（网络错误 / 超时 / 429 / 5xx / 404 等）
// 时立即请求官方 v0 API，任一成功即返回。两类接口的 images 字段结构
// 一致，后续图片提取逻辑完全相同。
// 两个上游共用同一超时设定（fetchTimeout，各 2.5s、合计最长 5s）：
// 上游及时返回失败状态则立刻切换下一个；未及时返回则超时后切换，
// 保证 next 故障时 v0 始终能获得完整的超时预算。
func (s *Service) getEntity(ctx context.Context, conf kindConf, id int64) (*apiEntity, error) {
	body, nextErr := s.tryEntity(ctx, s.nextHost, conf.apiSeg, id)
	if nextErr == nil {
		return body, nil
	}
	log.Printf("pics: next API 获取 %s %d 失败: %v，回退 v0 API", conf.table, id, nextErr)
	body, v0Err := s.tryEntity(ctx, s.v0Host, conf.apiSeg, id)
	if v0Err == nil {
		return body, nil
	}
	return nil, fmt.Errorf("next 与 v0 API 均失败（next: %v；v0: %w）", nextErr, v0Err)
}

// tryEntity 请求单个上游 API 并解析响应；每个上游独立受 fetchTimeout
// 约束，超时即返回错误，由调用方立刻切换下一个上游。
func (s *Service) tryEntity(ctx context.Context, host, seg string, id int64) (*apiEntity, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		host+seg+"/"+strconv.FormatInt(id, 10), nil)
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

// PathURL 输入类型、保存的相对路径与尺寸，返回不含主机的 CDN 路径。
// kind 取 person / subject / character；size 支持 l/large、m/medium、
// s/small、g/grid，默认 l。
func PathURL(kind, rel, size string) string {
	conf, ok := kinds[kind]
	if !ok || rel == "" {
		return ""
	}
	switch strings.ToLower(size) {
	case "m", "medium":
		return "/r/200" + conf.picBase + "l/" + rel
	case "s", "small":
		return "/r/100" + conf.picBase + "l/" + rel
	case "g", "grid":
		return "/r/100x100" + conf.picBase + "l/" + rel
	default:
		return conf.picBase + "l/" + rel
	}
}

// BuildURL 输入类型、保存的相对路径与尺寸，返回可直接展示的完整 URL。
func BuildURL(kind, rel, size string) string {
	p := PathURL(kind, rel, size)
	if p == "" {
		return ""
	}
	return cdnHost + p
}
