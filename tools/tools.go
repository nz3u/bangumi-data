//go:build tools

// Package tools 锁定代码生成类 CLI 工具的版本（经 go.mod），
// 使 `go run github.com/swaggo/swag/cmd/swag` 无需 @latest 也能得到确定版本。
package tools

import (
	_ "github.com/swaggo/swag/cmd/swag"
)
