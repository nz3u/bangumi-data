package update

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bangumi-subject-go/internal/config"
	"bangumi-subject-go/internal/download"
)

// latestTimeout 单次检查上游元信息的超时（离线时快速失败，不阻塞服务）。
const latestTimeout = 20 * time.Second

// DefaultCheckInterval serve 启动后的版本复查周期（启动即先查一次）。
const DefaultCheckInterval = 6 * time.Hour

// LatestInfo 上游最新导出的概要（供 API 返回与前端展示）。
type LatestInfo struct {
	Version     string `json:"version"`                // 快照文件名
	PublishedAt string `json:"published_at,omitempty"` // 上游文件创建时间
}

// Status 数据库版本与最新导出的对比结果。
type Status struct {
	Database        *config.DatabaseInfo `json:"database,omitempty"` // 本地版本记录；nil=无记录（视为旧版本）
	Latest          *LatestInfo          `json:"latest,omitempty"`   // 上游最新；nil=尚未成功获取（离线等），此时不判定新旧
	UpdateAvailable bool                 `json:"update_available"`
	CheckedAt       string               `json:"checked_at,omitempty"` // 最近一次检查时间
}

// VersionChecker 维护数据库版本与 Archive 最新导出的对比：
//   - 上游部分（latest.json）后台定时拉取并缓存（离线时静默降级）；
//   - 本地部分（config.json 版本记录、库文件存在性）在每次 Status() 时实时读取，
//     因此 serve 运行期间执行 bangumi update 后，前端下次请求即反映新状态；
//   - 检查到落后时打印一次日志提醒。
//
// 所有方法并发安全。
type VersionChecker struct {
	dbPath    string
	cfgPath   string
	latestURL string // 空则用 download.LatestURL；测试可指向本地服务

	mu      sync.Mutex
	latest  *LatestInfo // 最近一次成功获取的上游信息；nil=尚未成功（离线）
	checked time.Time   // 最近一次尝试检查的时间
	warned  bool        // 已就当前落后状态打印过提醒，恢复最新后重置
	offline bool        // 上游连续不可达（失败只提示一次）
}

// NewVersionChecker 创建检查器；dbPath 为数据库文件路径，
// config.json 取其同目录。
func NewVersionChecker(dbPath string) *VersionChecker {
	return &VersionChecker{
		dbPath:  dbPath,
		cfgPath: config.FilePath(filepath.Dir(dbPath)),
	}
}

// Start 启动后台检查：立即执行一次，之后每 interval 刷新一次，直到 ctx 取消。
func (vc *VersionChecker) Start(ctx context.Context, interval time.Duration) {
	go func() {
		vc.checkOnce(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				vc.checkOnce(ctx)
			}
		}
	}()
}

// Status 当前对比结果。本地状态实时读取，仅上游结果来自缓存：
//
//	database 为空            -> 本地无版本记录（旧库），有上游信息即视为可更新
//	latest 为空              -> 离线或尚未完成首次检查，不判定新旧（update_available=false）
//	update_available         -> 有库文件且本地版本 != 上游最新版本（或本地未记录）
func (vc *VersionChecker) Status() Status {
	var db *config.DatabaseInfo
	if cfg, err := config.Load(vc.cfgPath); err == nil {
		db = cfg.Database
	}

	vc.mu.Lock()
	latest := vc.latest
	checked := vc.checked
	vc.mu.Unlock()

	st := Status{
		Database:  db,
		Latest:    latest,
		CheckedAt: formatChecked(checked),
	}
	switch {
	case !vc.dbExists(): // 无库文件无从更新
	case localVersion(db) == "":
		st.UpdateAvailable = latest != nil
	case latest != nil:
		st.UpdateAvailable = db.Version != latest.Version
	}
	return st
}

// checkOnce 执行一次上游检查并更新缓存；失败时保留原缓存、连续失败只打一条日志。
func (vc *VersionChecker) checkOnce(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, latestTimeout)
	defer cancel()

	rel, err := download.FetchLatest(cctx, vc.url())

	vc.mu.Lock()
	if err != nil {
		if !vc.offline {
			vc.offline = true
			log.Printf("dbver: 暂无法获取最新导出信息（离线或上游不可达: %s）；本地功能不受影响，每 %s 自动重试",
				err.Error(), DefaultCheckInterval)
		}
		vc.checked = time.Now()
		vc.mu.Unlock()
		return
	}
	if vc.offline {
		vc.offline = false
		log.Printf("dbver: 已恢复与上游的连接，最新导出 %s", rel.Name)
	}
	vc.latest = &LatestInfo{Version: rel.Name, PublishedAt: rel.CreatedAt}
	vc.checked = time.Now()
	prevWarned := vc.warned
	vc.mu.Unlock()

	st := vc.Status()
	switch {
	case st.UpdateAvailable && !prevWarned:
		log.Printf("提醒：%s，上游已有新导出 %s（%s 发布），建议运行 bangumi update 更新",
			localDesc(st.Database), rel.Name, rel.CreatedAt)
		vc.mu.Lock()
		vc.warned = true
		vc.mu.Unlock()
	case !st.UpdateAvailable:
		vc.mu.Lock()
		vc.warned = false
		vc.mu.Unlock()
	}
}

// localVersion 已记录的本地版本号；未记录返回空串。
func localVersion(db *config.DatabaseInfo) string {
	if db == nil {
		return ""
	}
	return db.Version
}

// localDesc 本地版本的日志描述。
func localDesc(db *config.DatabaseInfo) string {
	if v := localVersion(db); v != "" {
		return "当前数据版本 " + v + " 落后于最新"
	}
	return "当前数据库未记录版本（旧版本程序创建）"
}

func (vc *VersionChecker) dbExists() bool {
	fi, err := os.Stat(vc.dbPath)
	return err == nil && !fi.IsDir()
}

func (vc *VersionChecker) url() string {
	if vc.latestURL != "" {
		return vc.latestURL
	}
	return download.LatestURL
}

func formatChecked(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
