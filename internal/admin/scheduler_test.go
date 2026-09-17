package admin

import (
	"os"
	"strings"
	"testing"
	"time"
)

// 2026-09-16 为周三；线上日志中的 2026/09/15 21:30:00（UTC）正是
// 2026-09-16T05:30:00+08:00，即当次调度时刻。
func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("解析时间 %q: %v", s, err)
	}
	return v
}

func TestNextWednesday0530(t *testing.T) {
	cases := []struct {
		name string
		now  string
		want string // RFC3339（UTC+8）
	}{
		// 事故现场：调度器在周三 05:30 CST 准点醒来，下一次应为 7 天后
		{"准点周三05:30", "2026-09-15T21:30:00Z", "2026-09-23T05:30:00+08:00"},
		{"周三05:29:59", "2026-09-15T21:29:59Z", "2026-09-16T05:30:00+08:00"},
		{"周三05:30:01", "2026-09-15T21:30:01Z", "2026-09-23T05:30:00+08:00"},
		{"周三凌晨", "2026-09-15T16:00:00Z", "2026-09-16T05:30:00+08:00"},
		{"周四", "2026-09-17T02:00:00+08:00", "2026-09-23T05:30:00+08:00"},
		{"周日", "2026-09-13T00:00:00+08:00", "2026-09-16T05:30:00+08:00"},
		{"周二深夜本地时区", "2026-09-15T23:00:00+08:00", "2026-09-16T05:30:00+08:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextWednesday0530(mustParse(t, tc.now))
			if got.Format(time.RFC3339) != tc.want {
				t.Errorf("NextWednesday0530(%s) = %s, want %s", tc.now, got.Format(time.RFC3339), tc.want)
			}
			if got.Weekday() != time.Wednesday {
				t.Errorf("结果 %s 不是周三", got.Format(time.RFC3339))
			}
			if got.Hour() != 5 || got.Minute() != 30 {
				t.Errorf("结果 %s 不是 05:30", got.Format(time.RFC3339))
			}
			if !got.After(mustParse(t, tc.now)) {
				t.Errorf("结果 %s 必须晚于 now %s", got.Format(time.RFC3339), tc.now)
			}
		})
	}
}

func TestLastWednesday0530(t *testing.T) {
	cases := []struct {
		name string
		now  string
		want string
	}{
		// 调度后当天早上启动：最近一次调度就是当天 05:30（触发补做）
		{"周三08:00", "2026-09-16T08:00:00+08:00", "2026-09-16T05:30:00+08:00"},
		{"周三05:30准点", "2026-09-16T05:30:00+08:00", "2026-09-16T05:30:00+08:00"},
		// 调度前启动：最近一次是上周三
		{"周三05:00", "2026-09-16T05:00:00+08:00", "2026-09-09T05:30:00+08:00"},
		{"周五", "2026-09-18T12:00:00+08:00", "2026-09-16T05:30:00+08:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LastWednesday0530(mustParse(t, tc.now))
			if got.Format(time.RFC3339) != tc.want {
				t.Errorf("LastWednesday0530(%s) = %s, want %s", tc.now, got.Format(time.RFC3339), tc.want)
			}
			if got.After(mustParse(t, tc.now)) {
				t.Errorf("结果 %s 不应晚于 now %s", got.Format(time.RFC3339), tc.now)
			}
		})
	}
}

// 事故时间线复现：本地库为 dump-2026-09-08，缓存于 2026-09-15T18:07:43Z 刷新
// （当时上游尚未发布 21:03:37Z 的新导出），调度在 21:30:00Z 触发。
// 旧实现直接读缓存 => 误判「已是最新」并跳过一整周。
func TestScheduledUpdateIgnoresStaleCache(t *testing.T) {
	dbPath, _, srv := newScheduledFixture(t, oldDumpVersion)

	vc := newTestChecker(dbPath, srv)
	// 上游发布新导出之前的检查：缓存中只有旧版本
	if _, err := vc.CheckNow(t.Context()); err != nil {
		t.Fatalf("预热缓存失败: %v", err)
	}
	// 上游发布 dump-2026-09-15（对应日志中的 21:03:37Z 发布）
	setLatestName(t, srv, newDumpVersion)

	// 缓存仍然是旧的：这正是旧实现据以判断的依据
	if vc.Status().UpdateAvailable {
		t.Fatal("测试前提不成立：未刷新的缓存不应看到新版本")
	}

	m := NewManager(dbPath, "", nil, vc)
	triggered := 0
	m.runScheduledUpdate(t.Context(), func() { triggered++ })

	if triggered != 1 {
		t.Fatalf("应触发一次自动更新，实际 %d 次；日志: %v", triggered, m.Status().Logs)
	}
	assertLogContains(t, m, "检测到新版本 "+newDumpVersion)
}

// 上游发布延迟 / CDN 缓存：首次检查看不到新版时应在窗口内复查，而不是跳过整周。
func TestScheduledUpdateRetriesUntilPublished(t *testing.T) {
	oldInterval, oldWindow := autoUpdateRetryInterval, autoUpdateRetryWindow
	autoUpdateRetryInterval, autoUpdateRetryWindow = 5*time.Millisecond, 2*time.Second
	defer func() { autoUpdateRetryInterval, autoUpdateRetryWindow = oldInterval, oldWindow }()

	dbPath, _, srv := newScheduledFixture(t, oldDumpVersion)
	vc := newTestChecker(dbPath, srv)
	m := NewManager(dbPath, "", nil, vc)

	triggered := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		// 延迟 30ms 后上游才发布（模拟 CDN 缓存/发布延迟）
		time.AfterFunc(30*time.Millisecond, func() { setLatestName(t, srv, newDumpVersion) })
		m.runScheduledUpdate(t.Context(), func() { triggered++ })
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("调度未在预期时间内结束")
	}

	if triggered != 1 {
		t.Fatalf("延迟发布后应触发自动更新，实际 %d 次；日志: %v", triggered, m.Status().Logs)
	}
	assertLogContains(t, m, "后复查")
	assertLogContains(t, m, "检测到新版本 "+newDumpVersion)
}

// 上游确实没有新版：窗口结束后放弃本次调度，不触发更新。
func TestScheduledUpdateGivesUpWhenUpToDate(t *testing.T) {
	oldInterval, oldWindow := autoUpdateRetryInterval, autoUpdateRetryWindow
	autoUpdateRetryInterval, autoUpdateRetryWindow = 5*time.Millisecond, 40*time.Millisecond
	defer func() { autoUpdateRetryInterval, autoUpdateRetryWindow = oldInterval, oldWindow }()

	dbPath, _, srv := newScheduledFixture(t, newDumpVersion)
	vc := newTestChecker(dbPath, srv)
	m := NewManager(dbPath, "", nil, vc)

	triggered := 0
	m.runScheduledUpdate(t.Context(), func() { triggered++ })

	if triggered != 0 {
		t.Fatalf("已是最新不应触发更新，实际 %d 次", triggered)
	}
	assertLogContains(t, m, "已是最新版本 "+newDumpVersion)
	assertLogContains(t, m, "重试窗口结束")
}

// 未启用自动更新时不做任何检查。
func TestScheduledUpdateDisabled(t *testing.T) {
	dbPath, _, srv := newScheduledFixture(t, oldDumpVersion)
	if err := writeAutoUpdateEnabled(dbPath, false); err != nil {
		t.Fatal(err)
	}
	m := NewManager(dbPath, "", nil, newTestChecker(dbPath, srv))

	triggered := 0
	m.runScheduledUpdate(t.Context(), func() { triggered++ })
	if triggered != 0 {
		t.Fatalf("未启用时不应触发更新，实际 %d 次", triggered)
	}
	assertLogContains(t, m, "自动更新未启用")
}

// 尚无数据库（首次部署）时不自动更新，也不进入重试循环。
func TestScheduledUpdateWithoutDatabase(t *testing.T) {
	dbPath, _, srv := newScheduledFixture(t, oldDumpVersion)
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	m := NewManager(dbPath, "", nil, newTestChecker(dbPath, srv))

	triggered := 0
	m.runScheduledUpdate(t.Context(), func() { triggered++ })
	if triggered != 0 {
		t.Fatalf("无数据库时不应触发更新，实际 %d 次", triggered)
	}
	assertLogContains(t, m, "数据库不存在")
}

func assertLogContains(t *testing.T, m *Manager, want string) {
	t.Helper()
	for _, line := range m.Status().Logs {
		if strings.Contains(line, want) {
			return
		}
	}
	t.Errorf("日志中未找到 %q；实际日志: %v", want, m.Status().Logs)
}
