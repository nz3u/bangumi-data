package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/getkin/kin-openapi/openapi3"

	"bangumi-subject-go/docs"
)

// openapiSpec 输出 OpenAPI 3.0 规范（docs/openapi.json，由 make docs 生成），
// 并在运行时把 info.version 替换为编译期注入的程序版本号（本地为 dev，
// 发布构建为 git 标签）——接口能力与程序版本对应，spec 随发布演进。
func (h *handler) openapiSpec(c *gin.Context) {
	var doc openapi3.T
	if err := json.Unmarshal(docs.OpenAPI3JSON(), &doc); err != nil {
		fail(c, 500, err.Error())
		return
	}
	doc.OpenAPI = "3.0.3"
	doc.Info.Version = h.version
	data, err := json.Marshal(&doc)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}
