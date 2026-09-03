// Package config 管理 data 目录下的 config.json：
// 保存 next.bgm.tv API Key（人物头像抓取用）与数据库版本信息
// （update 命令导入完成后写入，用于判断当前库是否落后最新导出）。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config data/config.json 的内容。
type Config struct {
	BgmApiKey string        `json:"bgm_api_key,omitempty"`
	Database  *DatabaseInfo `json:"database,omitempty"`
	// 管理与自动更新配置（新增字段，旧文件缺省时取默认值）
	AdminToken string             `json:"admin_token,omitempty"`
	AutoUpdate *AutoUpdateConfig `json:"auto_update,omitempty"`
	Server     *ServerConfig     `json:"server,omitempty"`
}

// AutoUpdateConfig 自动更新行为配置。
type AutoUpdateConfig struct {
	Enabled bool   `json:"enabled"`
	KeepZip bool   `json:"keep_zip,omitempty"`
	Threads int    `json:"threads,omitempty"` // 0 表示使用默认 8
}

// ServerConfig 服务端可选配置（与环境变量/命令行互补，优先级：命令行 > 环境变量 > 配置文件）。
type ServerConfig struct {
	Listen string `json:"listen,omitempty"`
}

// DatabaseInfo 数据库版本信息（import/update 完成后自动写入）。
type DatabaseInfo struct {
	Version     string `json:"version"`                // 导出快照文件名，如 dump-2026-08-25.210336Z.zip
	Digest      string `json:"digest,omitempty"`       // 上游 SHA256 摘要（sha256:...）
	Size        int64  `json:"size,omitempty"`         // 压缩包字节数
	SourceURL   string `json:"source_url,omitempty"`   // 下载地址
	PublishedAt string `json:"published_at,omitempty"` // 上游导出时间
	ImportedAt  string `json:"imported_at,omitempty"`  // 本地导入完成时间
}

// FilePath config.json 在数据目录下的路径。
func FilePath(dataDir string) string { return filepath.Join(dataDir, "config.json") }

// Load 读取配置；文件不存在时返回零值配置而非错误（首次运行）。
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", path, err)
	}
	return &c, nil
}

// Save 原子写回配置（先写临时文件再改名），避免中途失败留下半截文件。
func Save(path string, c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("写入 %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换 %s: %w", path, err)
	}
	return nil
}

// Version 已记录的数据库版本；未记录（旧版本程序或首次运行）返回空串。
func (c *Config) Version() string {
	if c == nil || c.Database == nil {
		return ""
	}
	return c.Database.Version
}

// LoadAPIKey 读取 next.bgm.tv API Key：优先环境变量 BANGUMI_API_KEY，
// 其次 data 目录下 config.json 的 bgm_api_key 字段。均未配置时返回空串。
func LoadAPIKey(dataDir string) string {
	if v := strings.TrimSpace(os.Getenv("BANGUMI_API_KEY")); v != "" {
		return v
	}
	c, err := Load(FilePath(dataDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(c.BgmApiKey)
}

// AutoUpdateEnabled 是否允许自动更新（缺省配置视为关闭）。
func (c *Config) AutoUpdateEnabled() bool {
	if c == nil || c.AutoUpdate == nil {
		return false
	}
	return c.AutoUpdate.Enabled
}

// EnsureDefaults 为缺省字段填充默认值（不改变已显式配置的值）。
func (c *Config) EnsureDefaults() {
	if c.AutoUpdate == nil {
		c.AutoUpdate = &AutoUpdateConfig{}
	}
	if c.Server == nil {
		c.Server = &ServerConfig{}
	}
}

// GenerateToken 生成随机 admin token（16 字节 hex）。
func GenerateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnsureAdminToken 确保配置中存在 admin_token，不存在则生成并返回（调用方决定是否持久化）。
func (c *Config) EnsureAdminToken() (string, error) {
	if c.AdminToken != "" {
		return c.AdminToken, nil
	}
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	c.AdminToken = tok
	return tok, nil
}

// EffectiveListen 生效的监听地址：环境变量 > 配置文件 > 默认 :8080。
func EffectiveListen(dataDir string, flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := strings.TrimSpace(os.Getenv("BANGUMI_LISTEN")); v != "" {
		return v
	}
	cfg, err := Load(FilePath(dataDir))
	if err == nil && cfg.Server != nil && strings.TrimSpace(cfg.Server.Listen) != "" {
		return strings.TrimSpace(cfg.Server.Listen)
	}
	return ":8080"
}

// EffectiveDBPath 生效的数据库路径：环境变量 > 配置文件 > 默认 data/bangumi.db。
func EffectiveDBPath(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := strings.TrimSpace(os.Getenv("BANGUMI_DB")); v != "" {
		return v
	}
	return "data/bangumi.db"
}
