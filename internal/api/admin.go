package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/admin"
	"bangumi-subject-go/internal/config"
)

// adminHandler 依赖
type adminDeps struct {
	mgr     *admin.Manager
	dataDir string
	cfgPath string
}

// registerAdminRoutes 注册管理接口。
func registerAdminRoutes(r *gin.Engine, deps adminDeps, h *handler) {
	grp := r.Group("/api/admin")
	// 鉴权中间件（token 可选：若未配置则放行，配置后必须携带）
	grp.Use(adminAuthMiddleware(deps))

	grp.GET("/status", func(c *gin.Context) { adminStatus(c, deps) })
	grp.GET("/status/stream", func(c *gin.Context) { adminStatusStream(c, deps) })
	grp.GET("/config", func(c *gin.Context) { adminGetConfig(c, deps) })
	grp.PUT("/config", func(c *gin.Context) { adminPutConfig(c, deps) })
	grp.POST("/update", func(c *gin.Context) { adminTriggerUpdate(c, deps) })
	grp.POST("/cancel", func(c *gin.Context) { adminCancel(c, deps) })
	grp.POST("/reset", func(c *gin.Context) { adminReset(c, deps) })
	grp.GET("/logs", func(c *gin.Context) { adminLogs(c, deps) })
	grp.GET("/logs/stream", func(c *gin.Context) { adminLogsStream(c, deps) })
}

// adminAuthMiddleware 校验 admin_token。
// 规则：
//   - 配置中无 admin_token（空）时放行（首次初始化场景）
//   - 有 token 时要求请求携带正确 token，来源按优先级：
//     Header X-Admin-Token > Authorization Bearer > Query token
func adminAuthMiddleware(deps adminDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, _ := config.Load(deps.cfgPath)
		want := ""
		if cfg != nil {
			want = strings.TrimSpace(cfg.AdminToken)
		}
		// 环境变量覆盖？暂不支持，统一走配置
		if want == "" {
			c.Next()
			return
		}
		got := strings.TrimSpace(c.GetHeader("X-Admin-Token"))
		if got == "" {
			if ah := c.GetHeader("Authorization"); strings.HasPrefix(strings.ToLower(ah), "bearer ") {
				got = strings.TrimSpace(ah[7:])
			} else if ah != "" {
				got = strings.TrimSpace(ah)
			}
		}
		if got == "" {
			got = strings.TrimSpace(c.Query("token"))
		}
		if got != want {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "未授权：token 无效"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func adminStatus(c *gin.Context, deps adminDeps) {
	st := deps.mgr.Status()
	respOK(c, st)
}

func adminGetConfig(c *gin.Context, deps adminDeps) {
	cfg, err := config.Load(deps.cfgPath)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	cfg.EnsureDefaults()
	respOK(c, gin.H{
		"bgm_api_key": cfg.BgmApiKey,
		"admin_token": cfg.AdminToken,
		"auto_update": cfg.AutoUpdate,
		"server":      cfg.Server,
		"database":    cfg.Database,
	})
}

type putConfigReq struct {
	BgmApiKey  *string              `json:"bgm_api_key"`
	AdminToken *string              `json:"admin_token"`
	AutoUpdate *config.AutoUpdateConfig `json:"auto_update"`
	Server     *config.ServerConfig     `json:"server"`
}

func adminPutConfig(c *gin.Context, deps adminDeps) {
	var req putConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求体错误: "+err.Error())
		return
	}
	cfg, err := config.Load(deps.cfgPath)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	cfg.EnsureDefaults()
	if req.BgmApiKey != nil {
		cfg.BgmApiKey = strings.TrimSpace(*req.BgmApiKey)
	}
	if req.AdminToken != nil {
		cfg.AdminToken = strings.TrimSpace(*req.AdminToken)
	}
	if req.AutoUpdate != nil {
		if req.AutoUpdate.Threads < 0 || req.AutoUpdate.Threads > 32 {
			fail(c, 400, "threads 需在 0-32 之间")
			return
		}
		cfg.AutoUpdate = req.AutoUpdate
	}
	if req.Server != nil {
		cfg.Server = req.Server
		cfg.Server.Listen = strings.TrimSpace(cfg.Server.Listen)
	}
	if err := config.Save(deps.cfgPath, cfg); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, gin.H{"ok": true})
}

type triggerReq struct {
	Force bool `json:"force"`
}

func adminTriggerUpdate(c *gin.Context, deps adminDeps) {
	var req triggerReq
	_ = c.ShouldBindJSON(&req)
	// 也支持 query ?force=1
	if c.Query("force") == "1" || c.Query("force") == "true" {
		req.Force = true
	}
	if !deps.mgr.CanTrigger() {
		fail(c, 409, "已有更新正在进行中")
		return
	}
	// 异步触发，立即返回（使用独立上下文避免请求结束时取消）
	go func() {
		_ = deps.mgr.Trigger(context.Background(), req.Force)
	}()
	respOK(c, gin.H{"started": true, "force": req.Force})
}

func adminCancel(c *gin.Context, deps adminDeps) {
	deps.mgr.Cancel()
	respOK(c, gin.H{"cancelled": true})
}

func adminReset(c *gin.Context, deps adminDeps) {
	deps.mgr.Reset()
	respOK(c, gin.H{"reset": true})
}

func adminLogs(c *gin.Context, deps adminDeps) {
	st := deps.mgr.Status()
	respOK(c, gin.H{"logs": st.Logs, "state": st.State})
}

// adminLogsStream SSE 推送实时日志。
func adminLogsStream(c *gin.Context, deps adminDeps) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ch, cancel := deps.mgr.Subscribe()
	defer cancel()

	// Gin 的 SSE 需手动 flush
	c.Stream(func(w io.Writer) bool {
		select {
		case line, ok := <-ch:
			if !ok {
				return false
			}
			c.SSEvent("log", line)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// adminStatusStream SSE 推送状态（鉴权），即时推送 + 间隔推送（更新中 3s，空闲 15s）
func adminStatusStream(c *gin.Context, deps adminDeps) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Access-Control-Allow-Origin", "*")

	ch, cancel := deps.mgr.SubscribeStatus()
	defer cancel()

	c.Stream(func(w io.Writer) bool {
		// 根据是否更新中动态决定间隔：更新中 3s，空闲 15s
		delay := 15 * time.Second
		if deps.mgr.IsUpdating() {
			delay = 3 * time.Second
		}
		select {
		case st, ok := <-ch:
			if !ok {
				return false
			}
			b, _ := json.Marshal(st)
			c.SSEvent("status", string(b))
			return true
		case <-time.After(delay):
			st := deps.mgr.StatusWithoutLogs()
			b, _ := json.Marshal(st)
			c.SSEvent("status", string(b))
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// adminPublicStatusStream SSE 推送公开状态（免鉴权），用于横幅
func adminPublicStatusStream(c *gin.Context, deps adminDeps) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Access-Control-Allow-Origin", "*")

	ch, cancel := deps.mgr.SubscribeStatus()
	defer cancel()

	c.Stream(func(w io.Writer) bool {
		delay := 15 * time.Second
		if deps.mgr.IsUpdating() {
			delay = 5 * time.Second
		}
		select {
		case st, ok := <-ch:
			if !ok {
				return false
			}
			c.SSEvent("status", gin.H{"state": st.State, "db_exists": st.DBExists, "progress": st.Progress})
			return true
		case <-time.After(delay):
			st := deps.mgr.Status()
			c.SSEvent("status", gin.H{"state": st.State, "db_exists": st.DBExists, "progress": st.Progress})
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
