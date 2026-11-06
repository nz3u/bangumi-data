package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/config"
)

// newTrackingTestDeps 用临时 config.json 构造依赖。
func newTrackingTestDeps(t *testing.T, cfg *config.Config) adminDeps {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if cfg != nil {
		if err := config.Save(cfgPath, cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	return adminDeps{cfgPath: cfgPath}
}

// doTrackingRequest 以给定方法/请求体调用追踪相关 handler，返回响应。
func doTrackingRequest(t *testing.T, method, path string, body string, route string, h gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, path, h)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestAdminTrackingPublic 公开的追踪配置只输出浏览器需要的字段，
// 且 Umami 未填 Website ID 时不生效（enabled=false）。
func TestAdminTrackingPublic(t *testing.T) {
	deps := newTrackingTestDeps(t, &config.Config{
		BgmApiKey:  "secret-key",
		AdminToken: "secret-token",
		Tracking: &config.TrackingConfig{
			UmamiEnabled:   true,
			UmamiWebsiteID: "site-1",
			UmamiEvents:    true,
			CustomJS:       "console.log('x')",
		},
	})
	w := doTrackingRequest(t, http.MethodGet, "/api/tracking", "", "/api/tracking",
		func(c *gin.Context) { adminTracking(c, deps) })
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, leak := range []string{"secret-key", "secret-token", "admin_token", "bgm_api_key"} {
		if strings.Contains(body, leak) {
			t.Errorf("公开接口泄漏敏感字段 %q: %s", leak, body)
		}
	}

	var got struct {
		OK   bool         `json:"ok"`
		Data trackingData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if !got.Data.UmamiEnabled || got.Data.UmamiWebsiteID != "site-1" || !got.Data.UmamiEvents {
		t.Errorf("tracking data = %+v", got.Data)
	}
	// 未填脚本地址时回退官方云服务地址，前端可直接注入
	if got.Data.UmamiScriptURL != config.DefaultUmamiScriptURL {
		t.Errorf("umami_script_url = %q, want %q", got.Data.UmamiScriptURL, config.DefaultUmamiScriptURL)
	}
	if got.Data.CustomJS != "console.log('x')" {
		t.Errorf("custom_js = %q", got.Data.CustomJS)
	}
}

// TestAdminTrackingInactive 启用但缺 Website ID 时按未生效返回；
// 关闭 Umami 不影响自定义 JS。
func TestAdminTrackingInactive(t *testing.T) {
	deps := newTrackingTestDeps(t, &config.Config{Tracking: &config.TrackingConfig{
		UmamiEnabled: true, // 没填 Website ID
		CustomJS:     "window.__other_tracker = 1",
	}})
	w := doTrackingRequest(t, http.MethodGet, "/api/tracking", "", "/api/tracking",
		func(c *gin.Context) { adminTracking(c, deps) })
	var got struct {
		Data trackingData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got.Data.UmamiEnabled {
		t.Error("缺 Website ID 时不应启用 Umami")
	}
	if got.Data.CustomJS == "" {
		t.Error("自定义 JS 与 Umami 开关无关，应继续返回")
	}
}

// TestAdminPutConfigTracking 写入追踪配置：合法值落盘并清洗，
// 非法脚本地址（javascript:/协议相对）返回 400 且不覆盖旧值。
func TestAdminPutConfigTracking(t *testing.T) {
	deps := newTrackingTestDeps(t, &config.Config{Tracking: &config.TrackingConfig{UmamiEvents: true}})
	put := func(body string) *httptest.ResponseRecorder {
		return doTrackingRequest(t, http.MethodPut, "/api/admin/config", body, "/api/admin/config",
			func(c *gin.Context) { adminPutConfig(c, deps) })
	}

	if w := put(`{"tracking":{"umami_enabled":true,"umami_website_id":" site-1 ",` +
		`"umami_script_url":"/stats/script.js","umami_events":true,"custom_js":"console.log(1)"}}`); w.Code != http.StatusOK {
		t.Fatalf("合法配置 HTTP %d: %s", w.Code, w.Body.String())
	}
	cfg, err := config.Load(deps.cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tracking == nil || cfg.Tracking.UmamiWebsiteID != "site-1" || cfg.Tracking.UmamiScriptURL != "/stats/script.js" {
		t.Errorf("落盘结果 = %+v（应去掉首尾空白）", cfg.Tracking)
	}

	for _, bad := range []string{
		`{"tracking":{"umami_script_url":"javascript:alert(1)"}}`,
		`{"tracking":{"umami_script_url":"//evil.example.com/s.js"}}`,
		`{"tracking":{"custom_js":"` + strings.Repeat("a", maxTrackingCustomJSLen+1) + `"}}`,
	} {
		if w := put(bad); w.Code != http.StatusBadRequest {
			t.Errorf("非法配置 %s: HTTP %d, want 400", bad[:40], w.Code)
		}
	}
	cfg, err = config.Load(deps.cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tracking.UmamiScriptURL != "/stats/script.js" {
		t.Errorf("非法请求不应覆盖已有配置，当前 %q", cfg.Tracking.UmamiScriptURL)
	}
}
