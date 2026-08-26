// Package config 管理 data 目录下的 config.json：
// 保存 next.bgm.tv API Key（人物头像抓取用）与数据库版本信息
// （update 命令导入完成后写入，用于判断当前库是否落后最新导出）。
package config

import (
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
