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
