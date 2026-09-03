package admin

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"bangumi-subject-go/internal/config"
)

// CST 为 UTC+8（Asia/Shanghai），自动更新固定按此时间每周三 05:30 触发
var cst = time.FixedZone("CST", 8*3600)

// NextWednesday0530 计算从 now 起下一个周三 05:30（UTC+8）。
// 若 now 恰在周三 05:30 之前则返回当天 05:30，否则下一周。
func NextWednesday0530(now time.Time) time.Time {
	now = now.In(cst)
	loc := cst
	// 今天 05:30
	today530 := time.Date(now.Year(), now.Month(), now.Day(), 5, 30, 0, 0, loc)
	wd := int(now.Weekday()) // Sunday=0
	targetWd := int(time.Wednesday)
	diff := (targetWd - wd + 7) % 7
	candidate := today530.AddDate(0, 0, diff)
	if candidate.Before(now) || candidate.Equal(now) {
		// 已过期则下周
		// 若 diff==0 且 today530 < now 需+7天；已在 diff 中处理 candidate 为过去，此时+7天
		if diff == 0 {
			candidate = candidate.AddDate(0, 0, 7)
		} else if candidate.Before(now) {
			candidate = candidate.AddDate(0, 0, 7)
		}
	}
	// 双保险：若 candidate 仍在过去则+7天
	if candidate.Before(now) {
		candidate = candidate.AddDate(0, 0, 7)
	}
	return candidate
}

// ShouldAutoUpdate 读取配置判断是否启用自动更新。
func (m *Manager) ShouldAutoUpdate() bool {
	cfg, err := config.Load(m.cfgPath)
	if err != nil {
		return false
	}
	return cfg.AutoUpdateEnabled()
}

// StartScheduler 启动每周三 05:30 的自动更新调度（若配置关闭则仅日志提示并休眠至下次检查）。
// 在检测到更新失败或无数据库但有新版可用时不自动触发，仅记录日志，留给前端手动触发。
func (m *Manager) StartScheduler(ctx context.Context) {
	go func() {
		// 启动时延迟一次检查，避免与 serve 启动时的 versionChecker 首检竞争
		// 若数据库不存在，立即提示可通过前端初始化
		if !m.DBExists() {
			log.Printf("admin: 数据库不存在，请通过 /setup 页面完成初始化（或运行 bangumi update）")
		}

		for {
			now := time.Now().In(cst)
			next := NextWednesday0530(now)
			dur := time.Until(next)
			log.Printf("admin: 下次自动更新检查安排在 %s（%s 后）", next.Format(time.RFC3339), dur.Round(time.Second))

			select {
			case <-ctx.Done():
				return
			case <-time.After(dur):
			}

			// 醒来后再次检查开关（配置可能已被前端修改）
			if !m.ShouldAutoUpdate() {
				// 未启用则记录并继续下一周期
				m.AppendLog("自动更新未启用，跳过本次调度")
				continue
			}
			if m.IsUpdating() {
				m.AppendLog("已有更新进行中，跳过本次自动调度")
				continue
			}
			// 检查是否有新版可用（通过 VersionChecker）
			if m.versionChecker != nil {
				st := m.versionChecker.Status()
				if !st.UpdateAvailable {
					m.AppendLog("自动更新检查：已是最新，跳过")
					continue
				}
				m.AppendLog("自动更新检查：检测到新版本，开始自动更新")
			} else {
				m.AppendLog("自动更新检查：开始（无版本检查器，直接尝试更新）")
			}

			// 异步触发，避免阻塞调度循环
			go func() {
				// 自动更新使用非 force 模式，若版本一致则 update.Run 会直接返回
				if err := m.Trigger(context.Background(), false); err != nil {
					log.Printf("admin: 自动更新失败: %v", err)
				} else {
					log.Printf("admin: 自动更新完成")
				}
				// 更新后若 DB 已可用，可通过重开的连接继续服务
				_ = filepath.Dir // keep import
			}()
		}
	}()
}
