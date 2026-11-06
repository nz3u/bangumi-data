package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("Load 不存在的文件: %v", err)
	}
	if c.Version() != "" {
		t.Errorf("空配置 Version = %q, want 空", c.Version())
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := &Config{
		BgmApiKey: "key-123",
		Database: &DatabaseInfo{
			Version:     "dump-2026-08-25.210336Z.zip",
			Digest:      "sha256:abc",
			Size:        434740633,
			SourceURL:   "https://example.com/dump.zip",
			PublishedAt: "2026-08-25T21:03:37Z",
			ImportedAt:  "2026-08-26T00:00:00+08:00",
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.BgmApiKey != want.BgmApiKey {
		t.Errorf("BgmApiKey = %q, want %q", got.BgmApiKey, want.BgmApiKey)
	}
	if got.Version() != want.Version() || got.Database == nil || *got.Database != *want.Database {
		t.Errorf("Database 信息不一致: got %+v, want %+v", got.Database, want.Database)
	}
}

func TestSaveAtomicTmpNotLeftBehindOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := Save(path, &Config{BgmApiKey: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("成功保存后不应残留临时文件 (err=%v)", err)
	}
}

// TestTrackingConfigRoundTrip 追踪配置随 config.json 原子保存与读取。
func TestTrackingConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := &TrackingConfig{
		UmamiEnabled:   true,
		UmamiScriptURL: "https://umami.example.com/script.js",
		UmamiWebsiteID: "abc-123",
		UmamiEvents:    true,
		CustomJS:       "console.log('track')",
	}
	if err := Save(path, &Config{Tracking: want}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Tracking == nil || *got.Tracking != *want {
		t.Errorf("Tracking = %+v, want %+v", got.Tracking, want)
	}
	if !got.Tracking.UmamiActive() {
		t.Error("启用且填了 Website ID 时应生效")
	}
}

// TestTrackingDefaults 旧配置（无 tracking 字段）按默认值补全：
// Umami 关闭、事件采集开启、脚本地址回退官方云服务；自定义 JS 独立于 Umami。
func TestTrackingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"bgm_api_key":"k"}`), 0o644); err != nil {
		t.Fatalf("写旧配置: %v", err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// 未 EnsureDefaults 时也不得 panic：TrackingSettings 返回默认值副本
	if c.TrackingSettings().UmamiEnabled {
		t.Error("Umami 应默认关闭")
	}
	c.EnsureDefaults()
	if c.Tracking.UmamiEnabled {
		t.Error("Umami 应默认关闭")
	}
	if !c.Tracking.UmamiEvents {
		t.Error("事件采集应默认开启")
	}
	if got := c.Tracking.EffectiveUmamiScriptURL(); got != DefaultUmamiScriptURL {
		t.Errorf("脚本地址 = %q, want %q", got, DefaultUmamiScriptURL)
	}
	if c.Tracking.UmamiActive() {
		t.Error("未启用时不应生效")
	}
}

func TestTrackingUmamiActiveRequiresWebsiteID(t *testing.T) {
	var nilCfg *TrackingConfig
	if nilCfg.UmamiActive() {
		t.Error("nil 配置不应生效")
	}
	if got := nilCfg.EffectiveUmamiScriptURL(); got != DefaultUmamiScriptURL {
		t.Errorf("nil 配置脚本地址 = %q, want 默认值", got)
	}
	if (&TrackingConfig{UmamiEnabled: true}).UmamiActive() {
		t.Error("缺 Website ID 不应生效")
	}
	if !(&TrackingConfig{UmamiEnabled: true, UmamiWebsiteID: " site "}).UmamiActive() {
		t.Error("启用且填了 Website ID 应生效")
	}
}

func TestVersionNilSafety(t *testing.T) {
	var c *Config
	if v := c.Version(); v != "" {
		t.Errorf("nil 配置 Version = %q, want 空", v)
	}
	if v := (&Config{}).Version(); v != "" {
		t.Errorf("无 database 字段 Version = %q, want 空", v)
	}
}

func TestLoadAPIKeyEnvPriority(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := Save(path, &Config{BgmApiKey: "from-file"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := LoadAPIKey(dir); got != "from-file" {
		t.Errorf("LoadAPIKey = %q, want from-file", got)
	}
	t.Setenv("BANGUMI_API_KEY", "  from-env  ")
	if got := LoadAPIKey(dir); got != "from-env" {
		t.Errorf("环境变量优先: LoadAPIKey = %q, want from-env", got)
	}
}

func TestLoadAPIKeyBrokenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadAPIKey(dir); got != "" {
		t.Errorf("损坏配置应返回空串，got %q", got)
	}
}
