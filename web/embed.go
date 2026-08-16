// Package web 内嵌前端构建产物（web/dist），
// 使单二进制同时提供 REST API 与页面。
//
// 更新前端后重新编译 Go 程序即可：
//
//	cd web && npm install && npm run build
//	go build ./cmd/bangumi
//
// 或在 web 目录内执行 go generate。
package web

import (
	"embed"
	"io/fs"
)

//go:generate npm run build

//go:embed all:dist
var dist embed.FS

// FS 返回内嵌前端文件系统（dist 根目录）。
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}