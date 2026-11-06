// 前台统计追踪：Umami 与「页面底部自定义 JS」两段互相独立。
// 配置来自免鉴权接口 /api/tracking（后台 /admin 页写入 data/config.json 的 tracking 字段）：
//   umami_enabled / umami_script_url / umami_website_id / umami_events / custom_js
// 只关 Umami 不影响 custom_js，反之亦然；后台保存后刷新页面生效。

const CONFIG_URL = '/api/tracking'

let inited = false
let umami = null // { events, loaded }

// initTracking 拉取配置并注入脚本；重复调用为空操作，请求失败静默跳过。
export async function initTracking() {
  if (inited) return
  inited = true
  let data
  try {
    const body = await (await fetch(CONFIG_URL)).json()
    data = body?.data
  } catch {
    return
  }
  if (!data) return

  if (data.custom_js) injectCustomJS(data.custom_js)
  if (data.umami_enabled) loadUmami(data)
}

// injectCustomJS 把自定义脚本注入页面底部（其他追踪器用，内容由管理员填写）。
function injectCustomJS(code) {
  const s = document.createElement('script')
  s.dataset.source = 'custom-tracking'
  s.textContent = code
  document.body.appendChild(s)
}

// loadUmami 注入 Umami 脚本并监听事件基础。
// 显式设置 data-auto-track="false"：页面浏览由 trackPageview 在路由切换后上报，
// 这样能带上更新后的 document.title，也避免与脚本自带的 history 监听重复计数。
function loadUmami(cfg) {
  umami = { events: !!cfg.umami_events, loaded: false }
  const s = document.createElement('script')
  s.async = true
  s.src = cfg.umami_script_url
  s.dataset.websiteId = cfg.umami_website_id
  s.dataset.autoTrack = 'false'
  s.onload = () => {
    umami.loaded = true
    reportPageview() // 首屏
    if (umami.events) bindEvents()
  }
  s.onerror = () => { umami = null }
  document.head.appendChild(s)
}

// trackPageview 上报一次页面浏览（路由切换后调用，失败静默）。
export function trackPageview() {
  if (!umami?.loaded) return
  reportPageview()
}

function reportPageview() {
  try {
    window.umami?.track() // 无参 = 页面浏览，URL 与标题取当前文档状态
  } catch {}
}

// trackEvent 上报自定义事件（表单提交、按钮点击等）。
export function trackEvent(name, data) {
  try {
    window.umami?.track(name, data)
  } catch {}
}

// bindEvents 事件委托：按钮点击与表单提交，无需逐个组件埋点。
function bindEvents() {
  document.addEventListener('click', onClick, true)
  document.addEventListener('submit', onSubmit, true)
}

function onClick(e) {
  const el = e.target instanceof Element ? e.target.closest('button, [data-track]') : null
  if (!el || el.disabled) return
  const label = (el.dataset.trackLabel || el.getAttribute('aria-label') || el.textContent || '')
    .trim()
    .replace(/\s+/g, ' ')
    .slice(0, 64)
  trackEvent('button_click', { label, id: el.id || '', path: location.pathname })
}

function onSubmit(e) {
  const form = e.target
  if (!(form instanceof HTMLFormElement)) return
  trackEvent('form_submit', {
    form: form.dataset.trackForm || form.id || 'form',
    path: location.pathname
  })
}
