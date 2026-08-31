package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/pics"
)

// pic GET /api/pics/:kind/:id
//
// 统一图片解析接口（前端轮询使用），kind 取
// person（人物头像）/ subject（条目封面）/ character（角色头像）：
//   - 200 {status:"ok",path}     本地已有，path 为不含主机的 CDN 路径，
//                                由前端按设置拼接图片主机（默认 lain.bgm.tv）
//   - 202 {status:"pending"}     已触发后台抓取，稍后重试
//   - 200 {status:"failed"}      终态无法提供（未配置 Key / 确认无图），
//                                前端停止轮询
//   - 502 {ok:false}             上游 next API 与 v0 API 均不可用
//                                （负缓存期内），不入库，前端可稍后重试
//
// 可选查询参数 size 决定返回的尺寸：l/large、m/medium、s/small、g/grid，
// 空或未知值按 l（原始大图）处理。
func (h *handler) pic(c *gin.Context) {
	kind := c.Param("kind")
	if !pics.ValidKind(kind) {
		fail(c, http.StatusBadRequest, "无效的图片类型: "+kind)
		return
	}
	id, found := intParam(c, "id")
	if !found {
		fail(c, http.StatusBadRequest, "无效的 id")
		return
	}
	if h.pics == nil {
		fail(c, http.StatusInternalServerError, "图片服务未启用")
		return
	}
	status, path := h.pics.ResolvePath(kind, id, c.Query("size"))
	switch status {
	case pics.StatusPending:
		c.JSON(http.StatusAccepted, gin.H{"ok": true, "data": gin.H{"status": status, "path": path}})
	case pics.StatusUnavailable:
		fail(c, http.StatusBadGateway, "上游 API 暂不可用，请稍后重试")
	case pics.StatusOK:
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"status": status, "path": path}})
	default:
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"status": pics.StatusFailed, "path": ""}})
	}
}
