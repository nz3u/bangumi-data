// Package admin 维护更新状态、日志流与自动更新调度。
// 该包与 update/download/importer 协作，在 serve 运行期间支持：
//   - 受控的在线更新（下载 -> 导入临时库 -> 切库 -> 重开连接）
//   - 维护模式（更新期间前端展示更新中页面，后端数据接口返回 503）
//   - 日志订阅（供前端 SSE 实时展示命令行日志）
//   - 每周三 05:30 的自动触发（可由配置开关控制）
package admin

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bangumi-subject-go/internal/config"
	"bangumi-subject-go/internal/db"
	"bangumi-subject-go/internal/importer"
	"bangumi-subject-go/internal/update"
)

// State 更新状态机。
type State string

const (
	StateIdle     State = "idle"
	StateUpdating State = "updating"
	StateSuccess  State = "success"
	StateFailed   State = "failed"
)

// Status 对外暴露的状态（供 API 返回）。
type Status struct {
	State     State          `json:"state"`
	DBExists  bool           `json:"db_exists"`
	Progress  string         `json:"progress,omitempty"`
	Logs      []string       `json:"logs,omitempty"`
	Stats     *importer.Stats `json:"stats,omitempty"`
	Error     string         `json:"error,omitempty"`
	StartedAt string         `json:"started_at,omitempty"`
	EndedAt   string         `json:"ended_at,omitempty"`
	// 新版可用性由 VersionChecker 提供，前端通过 /api/dbinfo 已有信息补充判断
}

// Manager 负责更新编排与状态广播。
type Manager struct {
	dbPath    string
	commonDir string
	dataDir   string
	cfgPath   string
	versionChecker *update.VersionChecker

	mu        sync.RWMutex
	state     State
	progress  string
	logs      []string // 循环缓冲，保留最近 N 行
	startedAt time.Time
	endedAt   time.Time
	lastError string
	lastStats *importer.Stats

	// 当前持有的 DB 连接（更新成功后会替换）
	dbMu sync.RWMutex
	db   *sql.DB

	// 日志订阅者（SSE）
	subMu sync.Mutex
	subs  map[chan string]struct{}

	// 状态订阅者（SSE）
	statusMu   sync.Mutex
	statusSubs map[chan Status]struct{}

	// 日志缓冲上限
	maxLogs int

	// 用于取消正在进行的更新
	cancel context.CancelFunc

	capturing  bool
	origWriter io.Writer
}

const defaultMaxLogs = 500

// NewManager 创建管理器；dbConn 为当前已打开的数据库连接（可为 nil，若无库时）。
func NewManager(dbPath, commonDir string, dbConn *sql.DB, vc *update.VersionChecker) *Manager {
	dataDir := filepath.Dir(dbPath)
	return &Manager{
		dbPath:         dbPath,
		commonDir:      commonDir,
		dataDir:        dataDir,
		cfgPath:        config.FilePath(dataDir),
		versionChecker: vc,
		state:          StateIdle,
		logs:           make([]string, 0, 128),
		maxLogs:        defaultMaxLogs,
		db:             dbConn,
		subs:           make(map[chan string]struct{}),
		statusSubs:     make(map[chan Status]struct{}),
	}
}

// IsUpdating 是否处于更新中（供中间件判断维护模式）。
func (m *Manager) IsUpdating() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state == StateUpdating
}

// DB 返回当前 DB 连接（并发安全）。
func (m *Manager) DB() *sql.DB {
	m.dbMu.RLock()
	defer m.dbMu.RUnlock()
	return m.db
}

// Status 快照。
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logsCopy := make([]string, len(m.logs))
	copy(logsCopy, m.logs)
	st := Status{
		State:    m.state,
		DBExists: m.dbExistsLocked(),
		Progress: m.progress,
		Logs:     logsCopy,
		Stats:    m.lastStats,
		Error:    m.lastError,
	}
	if !m.startedAt.IsZero() {
		st.StartedAt = m.startedAt.Format(time.RFC3339)
	}
	if !m.endedAt.IsZero() {
		st.EndedAt = m.endedAt.Format(time.RFC3339)
	}
	return st
}

func (m *Manager) dbExistsLocked() bool {
	fi, err := os.Stat(m.dbPath)
	return err == nil && !fi.IsDir()
}

// DBExists 是否存在数据库文件。
func (m *Manager) DBExists() bool {
	_, err := os.Stat(m.dbPath)
	return err == nil
}

// AppendLog 追加日志行并广播给订阅者（截断过长单行）。
func (m *Manager) AppendLog(line string) {
	line = strings.TrimRight(line, "\r\n")
	if len(line) > 2000 {
		line = line[:2000] + "…"
	}
	// 捕获期间直接走内部追加并直写原始输出，避免经由 log 再次触发捕获导致重复
	if m.capturing && m.origWriter != nil {
		m.appendLogInternal(line)
		_, _ = fmt.Fprintln(m.origWriter, line)
		return
	}
	m.mu.Lock()
	m.logs = append(m.logs, line)
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
	m.mu.Unlock()

	// 同时打印到标准日志
	log.Println(line)

	m.subMu.Lock()
	for ch := range m.subs {
		select {
		case ch <- line:
		default:
		}
	}
	m.subMu.Unlock()
}

// Subscribe 订阅日志流，返回通道与取消函数。通道会在更新结束或取消时关闭。
func (m *Manager) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 64)
	m.subMu.Lock()
	m.subs[ch] = struct{}{}
	m.subMu.Unlock()

	// 立即推送历史日志
	m.mu.RLock()
	hist := append([]string(nil), m.logs...)
	m.mu.RUnlock()
	go func() {
		for _, l := range hist {
			ch <- l
		}
	}()

	cancel := func() {
		m.subMu.Lock()
		if _, ok := m.subs[ch]; ok {
			delete(m.subs, ch)
			close(ch)
		}
		m.subMu.Unlock()
	}
	return ch, cancel
}

// SubscribeStatus 订阅状态流（不含大体积 logs），用于横幅/配置页的 SSE 推送
func (m *Manager) SubscribeStatus() (<-chan Status, func()) {
	ch := make(chan Status, 16)
	m.statusMu.Lock()
	m.statusSubs[ch] = struct{}{}
	m.statusMu.Unlock()
	// 立即推送当前快照（不含 logs 以免过大）
	go func() { ch <- m.StatusWithoutLogs() }()
	cancel := func() {
		m.statusMu.Lock()
		if _, ok := m.statusSubs[ch]; ok {
			delete(m.statusSubs, ch)
			close(ch)
		}
		m.statusMu.Unlock()
	}
	return ch, cancel
}

// StatusWithoutLogs 返回不含 logs 的状态快照（用于 SSE 广播，避免大包）
func (m *Manager) StatusWithoutLogs() Status {
	st := m.Status()
	st.Logs = nil
	return st
}

func (m *Manager) broadcastStatus() {
	st := m.StatusWithoutLogs()
	m.statusMu.Lock()
	for ch := range m.statusSubs {
		select {
		case ch <- st:
		default:
		}
	}
	m.statusMu.Unlock()
}

// broadcastDone 关闭所有订阅（更新结束时调用，若需保持连接则不关闭，由调用方决定重用）。
// 这里不关闭，仅在 Trigger 时替换订阅生命周期由各自 cancel 负责。

// CanTrigger 是否可以发起新的更新（空闲或上次已结束）。
func (m *Manager) CanTrigger() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state != StateUpdating
}

// Trigger 同步触发一次更新（阻塞直到完成）。force 为 true 时忽略版本一致性检查。
// 调用方应确保 CanTrigger() 为真，否则返回错误。
func (m *Manager) Trigger(ctx context.Context, force bool) error {
	m.mu.Lock()
	if m.state == StateUpdating {
		m.mu.Unlock()
		return fmt.Errorf("已有更新正在进行中")
	}
	m.state = StateUpdating
	m.progress = "准备更新..."
	m.startedAt = time.Now()
	m.endedAt = time.Time{}
	m.lastError = ""
	m.lastStats = nil
	// 保留历史日志但追加分隔符
	m.logs = append(m.logs, fmt.Sprintf("=== 更新开始于 %s (force=%v) ===", m.startedAt.Format(time.RFC3339), force))
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
	m.mu.Unlock()
	m.broadcastStatus()

	// 可取消上下文
	ctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		m.cancel = nil
		m.mu.Unlock()
	}()

	err := m.runUpdate(ctx, force)

	m.mu.Lock()
	m.endedAt = time.Now()
	if err != nil {
		m.state = StateFailed
		m.lastError = err.Error()
		m.progress = "更新失败"
		m.logs = append(m.logs, "=== 更新失败: "+err.Error()+" ===")
		m.AppendLogLocked("更新结束于 " + m.endedAt.Format(time.RFC3339))
		m.mu.Unlock()
		m.broadcastStatus()
		m.AppendLog("更新失败: " + err.Error())
		m.broadcastStatus()
		return err
	}
	m.state = StateSuccess
	m.progress = "更新完成"
	m.logs = append(m.logs, "=== 更新成功于 "+m.endedAt.Format(time.RFC3339)+" ===")
	m.mu.Unlock()
	m.broadcastStatus()
	m.AppendLog("更新完成")
	m.broadcastStatus()
	return nil
}

func (m *Manager) AppendLogLocked(line string) {
	m.logs = append(m.logs, line)
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
}

// Cancel 取消正在进行的更新。
func (m *Manager) Cancel() {
	m.mu.RLock()
	cancel := m.cancel
	m.mu.RUnlock()
	if cancel != nil {
		cancel()
		m.AppendLog("已请求取消更新...")
	}
}

// logCaptureWriter 将 log 输出同时转发到 Manager 日志。
type logCaptureWriter struct {
	orig io.Writer
	m    *Manager
	buf  bytes.Buffer
}

func (w *logCaptureWriter) Write(p []byte) (int, error) {
	n, _ := w.orig.Write(p)
	w.buf.Write(p)
	// 按行拆分
	for {
		s := w.buf.String()
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(s[:idx], "\r")
		if line != "" {
			w.m.appendLogInternal(line)
		}
		w.buf.Next(idx + 1)
	}
	return n, nil
}

func (m *Manager) appendLogInternal(line string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}
	m.mu.Lock()
	m.logs = append(m.logs, line)
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
	m.mu.Unlock()
	m.subMu.Lock()
	for ch := range m.subs {
		select {
		case ch <- line:
		default:
		}
	}
	m.subMu.Unlock()
}

// runUpdate 实际执行下载+导入+切库+重开。
func (m *Manager) runUpdate(ctx context.Context, force bool) error {
	origOut := log.Writer()
	m.origWriter = origOut
	m.capturing = true
	cw := &logCaptureWriter{orig: origOut, m: m}
	log.SetOutput(cw)
	defer func() {
		// 刷新残留缓冲
		if cw.buf.Len() > 0 {
			s := strings.TrimSpace(cw.buf.String())
			if s != "" {
				m.appendLogInternal(s)
				_, _ = fmt.Fprintln(origOut, s)
			}
		}
		log.SetOutput(origOut)
		m.capturing = false
		m.origWriter = nil
	}()

	m.updateProgress("查询上游版本...")
	// 先检查是否需要更新（force 跳过）
	cfg, err := config.Load(m.cfgPath)
	if err != nil {
		m.updateProgress("读取配置失败")
		return err
	}
	// 获取 threads / keep 配置
	threads := 8
	keepZip := false
	if cfg.AutoUpdate != nil {
		if cfg.AutoUpdate.Threads > 0 {
			threads = cfg.AutoUpdate.Threads
		}
		keepZip = cfg.AutoUpdate.KeepZip
	}

	// 判断 DB 是否存在，仅通过文件存在性
	dbExists := m.DBExists()

	// 如果需要检查版本一致性（非 force），先尝试获取上游信息
	if !force {
		// 若已有数据库且版本一致，直接跳过（由 update.Run 内部判断），这里仅做日志
		// 实际仍交由 update.Run 的逻辑判定，避免重复请求
	}

	m.updateProgress("开始下载与导入（此过程可能持续数分钟）...")

	// 关键：如果是已有库且 serve 持有连接，需要先关闭旧连接再进行文件替换
	// 我们采用两阶段：先让 update.Run 执行到导入临时库阶段，但它内部会尝试删除旧文件
	// 为避免文件占用失败，先关闭当前 DB
	var needReopen bool
	if dbExists {
		m.dbMu.Lock()
		if m.db != nil {
			m.AppendLog("关闭当前数据库连接以准备切库...")
			_ = m.db.Close()
			m.db = nil
		}
		m.dbMu.Unlock()
		needReopen = true
	}

	opts := update.Options{
		DBPath:    m.dbPath,
		CommonDir: m.commonDir,
		Threads:   threads,
		Force:     force,
		KeepZip:   keepZip,
	}
	stats, err := update.Run(ctx, opts)
	if err != nil {
		// 失败后尝试重开旧库（如果曾关闭但更新未成功，旧文件仍在）
		if needReopen {
			m.reopenDB()
		}
		return err
	}
	if stats == nil {
		// 已是最新
		m.mu.Lock()
		m.lastStats = nil
		m.mu.Unlock()
		m.updateProgress("已是最新版本，无需更新")
		m.AppendLog("数据库已是最新版本")
		if needReopen {
			m.reopenDB()
		}
		return nil
	}

	m.mu.Lock()
	m.lastStats = stats
	m.mu.Unlock()
	m.updateProgress(fmt.Sprintf("导入完成，条目 %d 人物 %d", stats.Subjects, stats.Persons))

	// 成功后重开数据库
	if err := m.reopenDB(); err != nil {
		return fmt.Errorf("重开数据库失败: %w", err)
	}
	// 触发一次完整性后的索引/迁移检查已在 Open 时处理；EnsureIndexes/UpgradeSchema 由 serve 启动时执行，
	// 但在线切库后新库已是完整 Finalize 过的库，无需额外迁移。此处尝试轻量检查
	if conn := m.DB(); conn != nil {
		if err := db.EnsureIndexes(conn); err != nil {
			m.AppendLog("EnsureIndexes 警告: " + err.Error())
		}
		if err := db.UpgradeSchema(conn); err != nil {
			m.AppendLog("UpgradeSchema 警告: " + err.Error())
		}
	}
	m.updateProgress("数据库已切换，服务已恢复")
	return nil
}

func (m *Manager) updateProgress(p string) {
	m.mu.Lock()
	m.progress = p
	m.mu.Unlock()
	m.AppendLog(p)
	m.broadcastStatus()
}

func (m *Manager) reopenDB() error {
	conn, err := db.Open(m.dbPath)
	if err != nil {
		m.AppendLog("重开数据库失败: " + err.Error())
		return err
	}
	m.dbMu.Lock()
	m.db = conn
	m.dbMu.Unlock()
	m.AppendLog("数据库已重新打开")
	return nil
}



// Reset 状态重置为 idle（供前端手动清除成功/失败提示）。
func (m *Manager) Reset() {
	m.mu.Lock()
	if m.state != StateUpdating {
		m.state = StateIdle
		m.progress = ""
		m.lastError = ""
		m.endedAt = time.Time{}
		m.startedAt = time.Time{}
	}
	m.mu.Unlock()
	m.broadcastStatus()
}
