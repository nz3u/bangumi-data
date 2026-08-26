package db

import (
	"database/sql"
	"fmt"
)

// CheckIntegrity 执行 SQLite 的 PRAGMA integrity_check，
// 结果非 ok 时返回错误（update 换库前的最后一道校验）。
func CheckIntegrity(conn *sql.DB) error {
	var result string
	if err := conn.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("数据库完整性检查未通过: %s", result)
	}
	return nil
}
