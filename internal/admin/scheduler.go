package admin

import (
	"context"
	"fmt"
	"log"
	"time"

	"bangumi-subject-go/internal/config"
)

// cst 为 UTC+8（Asia/Shanghai）。自动更新固定按此时间每周三 05:30 触发：
// 上游 Archive 导出一般在周三 05:03（UTC+8）前后发布，留出约 27 分钟余量。
//
// 注意：容器内 TZ 默认为 UTC，因此 `log` 输出的时间戳是 UTC，而调度时间是
// UTC+8，两者相差 8 小时。镜像已设置 TZ=Asia/Shanghai，日志与调度同为 UTC+8；
// 若仍看到 UTC 时间戳，本文件输出的调度日志会同时给出两种时区。
var cst = time.FixedZone("CST", 8*3600)

// 以下三个参数为变量（而非常量）以便测试缩短等待时间。
var (
	// autoUpdateRetryInterval 到点检查仍未看到新版时的复查间隔。
	autoUpdateRetryInterval = 10 * time.Minute
	// autoUpdateRetryWindow 到点检查的重试总时长。上游发布延迟、raw.githubusercontent
	// 的 CDN 缓存都可能导致「刚发布的导出暂时看不到」，重试可避免直接跳过一整周。
	autoUpdateRetryWindow = 2 * time.Hour
	// autoUpdateCatchUpWindow 进程重启后仍补做本次调度的时长：若启动时刻距本周
	// 调度时刻不足该时长（例如在更新窗口内重启了容器），立即补做一次检查。
	autoUpdateCatchUpWindow = 2 * time.Hour
)

// NextWednesday0530 计算从 now 起下一个周三 05:30（UTC+8）。
// 若 now 恰在周三 05:30（含该时刻本身）则返回下一周，用于一次调度执行完毕后的排期。
func NextWednesday0530(now time.Time) time.Time {
	now = now.In(cst)
	// 今天 05:30
	today530 := time.Date(now.Year(), now.Month(), now.Day(), 5, 30, 0, 0, cst)
	diff := (int(time.Wednesday) - int(now.Weekday()) + 7) % 7 // Sunday=0
	candidate := today530.AddDate(0, 0, diff)
	// candidate 不晚于 now（当天已过点或正好在点上）则顺延一周
	for !candidate.After(now) {
		candidate = candidate.AddDate(0, 0, 7)
	}
	return candidate
}

// LastWednesday0530 计算不晚于 now 的最近一个周三 05:30（UTC+8）。
func LastWednesday0530(now time.Time) time.Time {
	return NextWednesday0530(now).AddDate(0, 0, -7)
}

// ShouldAutoUpdate 读取配置判断是否启用自动更新。
func (m *Manager) ShouldAutoUpdate() bool {
	cfg, err := config.Load(m.cfgPath)
	if err != nil {
		return false
	}
	return cfg.AutoUpdateEnabled()
}

// StartScheduler 启动每周三 05:30（UTC+8）的自动更新调度。
// 到点后先强制刷新上游信息（不使用后台缓存）再决定是否更新；若未看到新版，
// 会在 autoUpdateRetryWindow 内按 autoUpdateRetryInterval 复查，避免因上游
// 发布延迟或 CDN 缓存而整周不更新。
func (m *Manager) StartScheduler(ctx context.Context) {
	go func() {
		if !m.DBExists() {
			log.Printf("admin: 数据库不存在，请通过 /setup 页面完成初始化（或运行 bangumi update）")
		}

		// 重启补做：容器在调度时刻附近重启时，本次调度不能丢。
		if last := LastWednesday0530(time.Now()); time.Since(last) < autoUpdateCatchUpWindow {
			log.Printf("admin: 距本周自动更新时刻 %s 不足 %s，启动后立即补做一次检查",
				last.Format(time.RFC3339), autoUpdateCatchUpWindow)
			m.RunScheduledUpdate(ctx)
		}

		for {
			next := NextWednesday0530(time.Now())
			dur := time.Until(next)
			log.Printf("admin: 下次自动更新检查安排在 %s（UTC %s，%s 后）",
				next.Format(time.RFC3339), next.UTC().Format(time.RFC3339), dur.Round(time.Second))

			if !sleepCtx(ctx, dur) {
				return
			}
			m.RunScheduledUpdate(ctx)
		}
	}()
}

// RunScheduledUpdate 执行一次到点的自动更新调度（阻塞至结束）。
// 检测到更新失败或数据库不存在时不会自动触发，仅记录日志，留给前端手动触发。
func (m *Manager) RunScheduledUpdate(ctx context.Context) {
	m.runScheduledUpdate(ctx, m.triggerAutoUpdate)
}

// runScheduledUpdate 是 RunScheduledUpdate 的实现，trigger 便于测试替换。
func (m *Manager) runScheduledUpdate(ctx context.Context, trigger func()) {
	m.AppendLog("自动更新检查：开始（" + time.Now().In(cst).Format("2006-01-02 15:04:05 MST") + "）")

	// 到点后再次检查开关（配置可能已被前端修改）
	if !m.ShouldAutoUpdate() {
		m.AppendLog("自动更新未启用，跳过本次调度")
		return
	}
	if m.IsUpdating() {
		m.AppendLog("已有更新进行中，跳过本次自动调度")
		return
	}
	if !m.DBExists() {
		// 无库文件时谈不上「更新」，首次导入留给 /setup 或 bangumi update
		m.AppendLog("自动更新检查：数据库不存在，跳过（请通过 /setup 初始化或运行 bangumi update）")
		return
	}

	if m.versionChecker == nil {
		m.AppendLog("自动更新检查：无版本检查器，直接尝试更新")
		trigger()
		return
	}

	// 关键：CheckNow 会实时向上游拉取，而不是读后台每 6 小时刷新一次的缓存。
	// 上游导出在 05:03（UTC+8）左右发布，若沿用发布前刷新的缓存会误判为「已是最新」。
	deadline := time.Now().Add(autoUpdateRetryWindow)
	for attempt := 1; ; attempt++ {
		st, err := m.versionChecker.CheckNow(ctx)
		switch {
		case err != nil:
			m.AppendLog("自动更新检查：暂无法获取上游信息（离线或上游不可达）: " + err.Error())
		case st.UpdateAvailable:
			latest := ""
			if st.Latest != nil {
				latest = st.Latest.Version
			}
			m.AppendLog(fmt.Sprintf("自动更新检查：检测到新版本 %s，开始自动更新", latest))
			trigger()
			return
		default:
			if st.Latest != nil {
				m.AppendLog("自动更新检查：已是最新版本 " + st.Latest.Version + "，无需更新")
			} else {
				m.AppendLog("自动更新检查：本地库无版本记录且上游信息不可用，跳过")
			}
		}

		if !time.Now().Before(deadline) {
			m.AppendLog("自动更新检查：重试窗口结束仍未见新版，本次跳过，等待下次调度")
			return
		}
		m.AppendLog(fmt.Sprintf("自动更新检查：%s 后复查（第 %d 次）", autoUpdateRetryInterval, attempt))
		if !sleepCtx(ctx, autoUpdateRetryInterval) {
			return
		}
	}
}

// triggerAutoUpdate 同步执行一次非 force 更新（版本一致时 update.Run 会直接返回）。
func (m *Manager) triggerAutoUpdate() {
	if !m.CanTrigger() {
		m.AppendLog("已有更新/迁移正在进行中，跳过本次自动更新")
		return
	}
	// 使用独立上下文：调度被取消（如服务关闭）时不中断已开始的下载/导入
	if err := m.Trigger(context.Background(), false); err != nil {
		m.AppendLog("自动更新失败: " + err.Error())
		log.Printf("admin: 自动更新失败: %v", err)
		return
	}
	m.AppendLog("自动更新完成")
	log.Printf("admin: 自动更新完成")
}

// sleepCtx 睡眠 d，ctx 取消时提前返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
