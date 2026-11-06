package api

import (
	"errors"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/config"
)

// trackingData GET /api/tracking 的 data（免鉴权）。
// 只包含浏览器需要的内容：Umami 参数与待注入的自定义脚本，不含任何密钥。
// UmamiEnabled 表示「实际生效」：已开启且填了站点 ID，前端据此决定是否注入脚本。
type trackingData struct {
	UmamiEnabled   bool   `json:"umami_enabled"`
	UmamiScriptURL string `json:"umami_script_url,omitempty"`
	UmamiWebsiteID string `json:"umami_website_id,omitempty"`
	UmamiEvents    bool   `json:"umami_events"`
	CustomJS       string `json:"custom_js,omitempty"`
}

// adminTracking 返回前台可用的统计追踪配置。
// 前端启动时拉取：Umami 开启则注入脚本并上报路由/事件，custom_js 非空则注入页面底部；
// 两者互相独立。读取配置失败按「未开启」返回，避免统计配置导致页面不可用。
//
//	@Summary		统计追踪配置
//	@Description	免鉴权的公开配置：Umami 开关/脚本地址/站点 ID、是否采集表单与按钮事件，以及页面底部自定义脚本。两者相互独立，均可不配置。
//	@Tags			服务状态
//	@Produce		json
//	@Success		200	{object}	apiEnvelope{data=trackingData}
//	@Router			/api/tracking [get]
func adminTracking(c *gin.Context, deps adminDeps) {
	cfg, err := config.Load(deps.cfgPath)
	if err != nil {
		respOK(c, trackingData{})
		return
	}
	t := cfg.TrackingSettings()
	respOK(c, trackingData{
		UmamiEnabled:   t.UmamiActive(),
		UmamiScriptURL: t.EffectiveUmamiScriptURL(),
		UmamiWebsiteID: strings.TrimSpace(t.UmamiWebsiteID),
		UmamiEvents:    t.UmamiEvents,
		CustomJS:       t.CustomJS,
	})
}

// 追踪配置的长度上限，避免把超大脚本写进 config.json 后被每个页面拉取。
const (
	maxTrackingScriptURLLen = 512
	maxTrackingWebsiteIDLen = 128
	maxTrackingCustomJSLen  = 64 << 10 // 64 KB
)

// normalizeTracking 清洗并校验后台提交的追踪配置（就地修改）。
// 脚本地址允许 http(s) 绝对地址或站内相对路径（自建反向代理场景）。
func normalizeTracking(t *config.TrackingConfig) error {
	t.UmamiScriptURL = strings.TrimSpace(t.UmamiScriptURL)
	t.UmamiWebsiteID = strings.TrimSpace(t.UmamiWebsiteID)
	if t.UmamiScriptURL != "" {
		if len(t.UmamiScriptURL) > maxTrackingScriptURLLen {
			return errors.New("umami_script_url 过长")
		}
		if !validTrackingScriptURL(t.UmamiScriptURL) {
			return errors.New("umami_script_url 需为 http(s) 地址或以 / 开头的站内路径")
		}
	}
	if len(t.UmamiWebsiteID) > maxTrackingWebsiteIDLen {
		return errors.New("umami_website_id 过长")
	}
	if len(t.CustomJS) > maxTrackingCustomJSLen {
		return errors.New("custom_js 不能超过 64 KB")
	}
	return nil
}

// validTrackingScriptURL 校验脚本地址：http(s) 绝对地址，或以单个 / 开头的站内路径。
func validTrackingScriptURL(s string) bool {
	if strings.HasPrefix(s, "/") {
		return !strings.HasPrefix(s, "//") // 排除协议相对地址，避免绕过 scheme 校验
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
