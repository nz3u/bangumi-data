// Package api 提供 HTTP REST 接口，供前端页面查询。
package api

import (
	"database/sql"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/common"
	"bangumi-subject-go/internal/pics"
	"bangumi-subject-go/internal/update"
	"bangumi-subject-go/web"
)

// handler 持有 API 所需的依赖。
type handler struct {
	db      *sql.DB
	cons    *common.Constants
	pics    *pics.Service
	version string                  // 编译期注入的版本号（随 /api/health 返回）
	dbver   *update.VersionChecker  // 数据库版本检查（/api/dbinfo），可为 nil
}

// NewRouter 构建 gin 路由。
// version 为编译期注入的版本标识（发布流水线取 git 标签，开发构建为 "dev"）。
// webDir 非空且存在时，优先托管磁盘上的前端目录（便于开发热更新）；
// 否则回退到编译期内嵌的 web/dist（单二进制部署，无需额外参数）。
// picSvc 为图片解析服务（人物头像/条目封面/角色头像），可为 nil（图片接口返回未启用）。
// dbVer 为数据库版本检查服务，可为 nil（/api/dbinfo 返回未知状态）。
func NewRouter(conn *sql.DB, cons *common.Constants, webDir string, picSvc *pics.Service, version string, dbVer *update.VersionChecker) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(corsMiddleware())

	h := &handler{db: conn, cons: cons, pics: picSvc, version: version, dbver: dbVer}

	api := r.Group("/api")
	{
		api.GET("/health", h.health)
		api.GET("/stats", h.stats)
		api.GET("/constants", h.constants)
		api.GET("/dbinfo", h.dbInfo)

		// 图片解析（person=人物头像 / subject=条目封面 / character=角色头像）
		api.GET("/pics/:kind/:id", h.pic)

		// 条目
		api.GET("/subjects/search", h.searchSubjects)
		api.GET("/subjects/:id", h.getSubject)
		api.GET("/subjects/:id/episodes", h.getSubjectEpisodes)

		// 人物
		api.GET("/persons/search", h.searchPersons)
		api.GET("/persons/:id", h.getPerson)
		api.GET("/persons/:id/works", h.getPersonWorks)
		api.GET("/persons/:id/collaborators", h.getPersonCollaborators)
		api.GET("/persons/:id/collaboration", h.getPersonCollaboration)
		// 静态段路由需先于 :other 参数路由注册
		api.GET("/persons/:id/collaboration/positions", h.getPersonCollaborationPositions)
		api.GET("/persons/:id/collaboration/:other", h.getPersonCollaborationWith)
		api.GET("/persons/:id/roles", h.getPersonRoles)

		// 角色
		api.GET("/characters/search", h.searchCharacters)
		api.GET("/characters/:id", h.getCharacter)
	}

	if webDir != "" {
		if st, err := os.Stat(webDir); err == nil && st.IsDir() {
			r.Static("/", webDir)
			return r
		}
	}
	// 回退：托管内嵌前端（SPA，未知路径回退 index.html）
	serveEmbedded(r, web.FS())
	return r
}

// serveEmbedded 以内嵌文件系统托管前端。
// 未知的非 /api 路径回退到 index.html（支持前端路由刷新），
// 未匹配的 /api 路径返回 JSON 404。
func serveEmbedded(r *gin.Engine, fsys fs.FS) {
	fileServer := http.FileServer(http.FS(fsys))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(404, gin.H{"ok": false, "error": "接口不存在"})
			return
		}
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p != "" {
			if f, err := fsys.Open(p); err == nil {
				f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
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
