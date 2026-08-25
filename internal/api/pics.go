package api

import (
	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/pics"
)

// pic GET /api/pics/:kind/:id
//
// 统一图片解析接口（前端轮询使用），kind 取
// person（人物头像）/ subject（条目封面）/ character（角色头像）：
//   - 200 {status:"ok",url}      本地已有，可直接展示
//   - 202 {status:"pending"}     已触发后台抓取，稍后重试
//   - 200 {status:"failed"}      无法提供（未配置 Key/上游失败/确认无图），前端停止轮询
//
// 可选查询参数 size 决定返回的 CDN 尺寸：l/large、m/medium、s/small、g/grid，
// 空或未知值按 l（原始大图）处理。
func (h *handler) pic(c *gin.Context) {
	kind := c.Param("kind")
	if !pics.ValidKind(kind) {
		fail(c, 400, "无效的图片类型: "+kind)
		return
	}
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	if h.pics == nil {
		fail(c, 500, "图片服务未启用")
		return
	}
	status, url := h.pics.Resolve(kind, id, c.Query("size"))
	code := 200
	if status == "pending" {
		code = 202
	}
	c.JSON(code, gin.H{"ok": true, "data": gin.H{"status": status, "url": url}})
}
