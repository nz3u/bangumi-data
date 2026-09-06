// openapiconv 将 swag 生成的 Swagger 2.0 规范（docs/swagger.json）
// 转换为 OpenAPI 3.0（docs/openapi.json）。作为 `make docs` 的一步执行。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
)

func main() {
	in := flag.String("in", "docs/swagger.json", "输入：swag 生成的 Swagger 2.0 JSON")
	out := flag.String("out", "docs/openapi.json", "输出：OpenAPI 3.0 JSON")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取输入失败:", err)
		os.Exit(1)
	}
	var doc2 openapi2.T
	if err := json.Unmarshal(data, &doc2); err != nil {
		fmt.Fprintln(os.Stderr, "解析 Swagger 2.0 失败:", err)
		os.Exit(1)
	}
	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		fmt.Fprintln(os.Stderr, "转换失败:", err)
		os.Exit(1)
	}
	if doc3.OpenAPI == "" {
		doc3.OpenAPI = "3.0.3"
	}
	out2, err := json.MarshalIndent(doc3, "", "    ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "序列化失败:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, out2, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写出失败:", err)
		os.Exit(1)
	}
}
