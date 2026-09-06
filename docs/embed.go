// 附加的内嵌文件：openapi.json 由 `make docs` 生成
// （swag init 产物 docs/swagger.json 经 cmd/openapiconv 转换为 OpenAPI 3.0），
// 供 /openapi.json 在运行时注入服务版本号与数据快照信息后输出。
// swag init 只覆写自身产物（docs.go/swagger.json/swagger.yaml），不会触碰本文件。
package docs

import _ "embed"

//go:embed openapi.json
var openapiJSON []byte

// OpenAPI3JSON 返回内嵌的 OpenAPI 3.0 规范原始 JSON。
func OpenAPI3JSON() []byte { return openapiJSON }
