// Package api 提供 HTTP REST 接口，供前端页面查询。
package api

import (
	"database/sql"
	"os"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/common"
)

// handler 持有 API 所需的依赖。
type handler struct {
	db   *sql.DB
	cons *common.Constants
}

// NewRouter 构建 gin 路由。
// webDir 非空且存在时，额外托管静态前端文件。
func NewRouter(conn *sql.DB, cons *common.Constants, webDir string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(corsMiddleware())

	h := &handler{db: conn, cons: cons}

	api := r.Group("/api")
	{
		api.GET("/health", h.health)
		api.GET("/stats", h.stats)
		api.GET("/constants", h.constants)

		// 条目
		api.GET("/subjects/search", h.searchSubjects)
		api.GET("/subjects/:id", h.getSubject)
		api.GET("/subjects/:id/episodes", h.getSubjectEpisodes)

		// 人物
		api.GET("/persons/search", h.searchPersons)
		api.GET("/persons/:id", h.getPerson)
		api.GET("/persons/:id/works", h.getPersonWorks)
		api.GET("/persons/:id/collaborators", h.getPersonCollaborators)

		// 角色
		api.GET("/characters/search", h.searchCharacters)
		api.GET("/characters/:id", h.getCharacter)
	}

	if webDir != "" {
		if st, err := os.Stat(webDir); err == nil && st.IsDir() {
			r.Static("/", webDir)
		}
	}
	return r
}

// corsMiddleware 允许跨域（前后端分离部署时方便本地调试）。
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
