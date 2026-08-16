// Package embedded 将 common 子模块中的 yaml 常量文件内嵌进二进制，
// 使程序可以自包含运行（单二进制部署无需携带 common 目录）。
//
// 若 common 子模块更新，只需重新编译即可带上最新常量。
package embedded

import "embed"

//go:embed common/*.yml
var YML embed.FS
