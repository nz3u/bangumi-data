package api

import (
	"github.com/gin-gonic/gin"
)

// personAvatar GET /api/persons/:id/avatar
//
// 人物头像解析接口（前端轮询使用）：
//   - 200 {status:"ok",url}      本地已有，可直接展示
//   - 202 {status:"pending"}     已触发后台抓取，稍后重试
//   - 200 {status:"failed"}      无法提供（未配置 Key/上游失败），前端停止轮询
func (h *handler) personAvatar(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	if h.pics == nil {
		fail(c, 500, "图片服务未启用")
		return
	}
	status, url := h.pics.Resolve(id)
	code := 200
	if status == "pending" {
		code = 202
	}
	c.JSON(code, gin.H{"ok": true, "data": gin.H{"status": status, "url": url}})
}
