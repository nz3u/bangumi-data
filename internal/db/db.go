// Package db 负责 SQLite 连接与建表（schema）。
// 使用 modernc.org/sqlite（纯 Go 实现，无 CGO，跨平台可直接编译）。
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Open 打开（或创建）SQLite 数据库，并设置合理的连接参数。
// WAL 模式 + synchronous=NORMAL 兼顾导入速度与读并发。
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录: %w", err)
		}
	}
	dsn := "file:" + path +
		"?_pragma=busy_timeout(30000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=cache_size(-65536)" // 64MB page cache

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s: %w", path, err)
	}
	// 单写者场景下限制连接数，避免 SQLITE_BUSY
	conn.SetMaxOpenConns(4)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("连接数据库 %s: %w", path, err)
	}
	return conn, nil
}

// Close 关闭数据库连接。
func Close(conn *sql.DB) {
	if conn != nil {
		_ = conn.Close()
	}
}

// ExecMulti 按分号拆分执行多条 SQL（schema 文件用）。
func ExecMulti(conn *sql.DB, sqls string) error {
	for _, stmt := range strings.Split(sqls, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("执行 SQL 失败: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}
