// Package api 提供 HTTP REST 接口，供前端页面查询。
package api

import (
	"database/sql"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/admin"
	"bangumi-subject-go/internal/common"
	"bangumi-subject-go/internal/config"
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
	mgr     *admin.Manager
}

func (h *handler) getDB() *sql.DB {
	if h.mgr != nil {
		if c := h.mgr.DB(); c != nil {
			return c
		}
	}
	return h.db
}

// NewRouter 构建 gin 路由。
// version 为编译期注入的版本标识（发布流水线取 git 标签，开发构建为 "dev"）。
// webDir 非空且存在时，优先托管磁盘上的前端目录（便于开发热更新）；
// 否则回退到编译期内嵌的 web/dist（单二进制部署，无需额外参数）。
// picSvc 为图片解析服务（人物头像/条目封面/角色头像），可为 nil（图片接口返回未启用）。
// dbVer 为数据库版本检查服务，可为 nil（/api/dbinfo 返回未知状态）。
func NewRouter(conn *sql.DB, cons *common.Constants, webDir string, picSvc *pics.Service, version string, dbVer *update.VersionChecker) *gin.Engine {
	return NewRouterWithManager(conn, cons, webDir, picSvc, version, dbVer, nil, "")
}

// NewRouterWithManager 带更新管理器与数据目录的路由（serve 模式使用；支持维护模式与管理接口）。
func NewRouterWithManager(conn *sql.DB, cons *common.Constants, webDir string, picSvc *pics.Service, version string, dbVer *update.VersionChecker, mgr *admin.Manager, dataDir string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(corsMiddleware())
	if mgr != nil {
		r.Use(maintenanceMiddleware(mgr))
	}

	h := &handler{db: conn, cons: cons, pics: picSvc, version: version, dbver: dbVer, mgr: mgr}

	api := r.Group("/api")
	{
		api.GET("/health", h.health)
		api.GET("/health/stream", h.healthStream)
		api.GET("/stats", h.stats)
		api.GET("/stats/stream", h.statsStream)
		api.GET("/system/stream", h.systemStream)
		api.GET("/constants", h.constants)
		api.GET("/dbinfo", h.dbInfo)

		// 图片解析（person=人物头像 / subject=条目封面 / character=角色头像）
		api.GET("/pics/:kind/:id", h.pic)

		// 条目（静态段路由需先于 :id 参数路由注册）
		api.GET("/subjects/search", h.searchSubjects)
		api.GET("/subjects/tags", h.suggestSubjectTags)
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

	if mgr != nil {
		if dataDir == "" {
			dataDir = "data"
		}
		registerAdminRoutes(r, adminDeps{mgr: mgr, dataDir: dataDir, cfgPath: config.FilePath(dataDir)}, h)
		// 供前端开箱检测的轻量接口（无需鉴权）
		r.GET("/api/admin/public-status", func(c *gin.Context) {
			st := mgr.Status()
			// 不暴露日志，仅返回状态与 db 存在性
			c.JSON(200, gin.H{"ok": true, "data": gin.H{
				"state":     st.State,
				"db_exists": st.DBExists,
				"progress":  st.Progress,
			}})
		})
		r.GET("/api/admin/public-status/stream", func(c *gin.Context) {
			adminPublicStatusStream(c, adminDeps{mgr: mgr, dataDir: dataDir, cfgPath: config.FilePath(dataDir)})
		})
	}

	if webDir != "" {
		if st, err := os.Stat(webDir); err == nil && st.IsDir() {
			// gin 1.12 起根级 catch-all（r.Static("/") 内部为 /*filepath）与已注册的
			// /api 前缀冲突会 panic，改用与内嵌模式一致的 NoRoute 托管。
			serveEmbedded(r, os.DirFS(webDir))
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
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Token")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// maintenanceMiddleware 更新期间的维护模式：数据接口返回 503，前端据此展示更新中页面。
func maintenanceMiddleware(mgr *admin.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !mgr.IsUpdating() {
			c.Next()
			return
		}
		p := c.Request.URL.Path
		// 放行管理与健康检查、静态资源
		if strings.HasPrefix(p, "/api/admin/") || p == "/api/health" || p == "/api/admin/public-status" || p == "/api/dbinfo" {
			c.Next()
			return
		}
		if strings.HasPrefix(p, "/api/") {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "服务正在更新中，请稍后重试", "updating": true})
			c.Abort()
			return
		}
		c.Next()
	}
}

func init() {
	// 确保 filepath 导入被使用（避免未使用报错，仅在特定构建下）
	_ = filepath.Dir
}
