// 站点设置：localStorage 持久化，改动实时保存、全局响应式生效。
const KEY = 'bgm.settings'

// 外部链接主机预设（Bangumi 官方域名 + 自定义镜像）
export const EXTERNAL_HOST_PRESETS = [
  { value: 'bgm.tv', label: 'bgm.tv' },
  { value: 'bangumi.tv', label: 'bangumi.tv' },
  { value: 'chii.in', label: 'chii.in' },
  { value: 'custom', label: '自定义…' }
]

export const DEFAULT_EXTERNAL_HOST = 'bgm.tv'

// 图片主机预设（后端只返回相对路径，由前端拼接完整地址）
export const PIC_HOST_PRESETS = [
  { value: 'lain.bgm.tv', label: 'lain.bgm.tv' },
  { value: 'custom', label: '自定义…' }
]

export const DEFAULT_PIC_HOST = 'lain.bgm.tv'

function load() {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) return JSON.parse(raw)
  } catch {
    /* 损坏的存储内容按默认处理 */
  }
  return {}
}

// 条目页高亮区域预设
export const HIGHLIGHT_AREA_PRESETS = [
  { value: 'title', label: '标题' },
  { value: 'tags', label: '标签' }
]

export const DEFAULT_HIGHLIGHT_AREAS = ['tags']

// 关键词高亮颜色预设（浅色模式下的背景色/着重线颜色；深色模式固定琥珀线）
export const HIGHLIGHT_COLOR_PRESETS = [
  { value: 'slate-200', label: 'slate-200', hex: '#e2e8f0' },
  { value: 'amber-100', label: 'amber-100', hex: '#fef3c7' },
  { value: 'custom', label: '自定义…', hex: '' }
]

export const DEFAULT_HIGHLIGHT_COLOR = 'amber-100'
export const DEFAULT_HIGHLIGHT_COLOR_CUSTOM = '#fff2b3'

export const settings = $state({
  externalHost: DEFAULT_EXTERNAL_HOST, // 'bgm.tv' | 'bangumi.tv' | 'chii.in' | 'custom'
  externalHostCustom: '', // 自定义主机名
  picHost: DEFAULT_PIC_HOST, // 'lain.bgm.tv' | 'custom'
  picHostCustom: '',
  highlightAreas: [...DEFAULT_HIGHLIGHT_AREAS], // 高亮区域：['title', 'tags'] 的子集
  searchHighlight: true, // 人物/角色搜索建议与结果的关键词高亮开关
  highlightColor: DEFAULT_HIGHLIGHT_COLOR, // 'slate-200' | 'amber-100' | 'custom'
  highlightColorCustom: DEFAULT_HIGHLIGHT_COLOR_CUSTOM, // 自定义颜色（#rrggbb）
  ...load()
})

// 变更后调用：实时写入 localStorage
export function saveSettings() {
  try {
    localStorage.setItem(
      KEY,
      JSON.stringify({
        externalHost: settings.externalHost,
        externalHostCustom: settings.externalHostCustom,
        picHost: settings.picHost,
        picHostCustom: settings.picHostCustom,
        highlightAreas: settings.highlightAreas,
        searchHighlight: settings.searchHighlight,
        highlightColor: settings.highlightColor,
        highlightColorCustom: settings.highlightColorCustom
      })
    )
  } catch {
    /* 隐私模式等场景下写入失败不影响使用 */
  }
}

// 搜索建议/结果是否高亮命中关键词（人物检索推荐、人物/角色搜索结果共用）
export function isSearchHighlight() {
  return settings.searchHighlight !== false
}

// 当前高亮颜色（#rrggbb）：预设直接取值；自定义非法时回退默认
export function highlightHex() {
  if (settings.highlightColor === 'slate-200') return '#e2e8f0'
  if (settings.highlightColor === 'custom') {
    return /^#[0-9a-fA-F]{6}$/.test(settings.highlightColorCustom ?? '')
      ? settings.highlightColorCustom.toLowerCase()
      : DEFAULT_HIGHLIGHT_COLOR_CUSTOM
  }
  return '#fef3c7' // amber-100（默认）
}

// 检查指定高亮区域是否启用
export function isHighlightEnabled(area) {
  return Array.isArray(settings.highlightAreas) && settings.highlightAreas.includes(area)
}

// sanitizeHost 净化自定义输入：去协议/路径/端口，仅保留合法主机名；非法返回空串
export function sanitizeHost(v) {
  let s = String(v ?? '')
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, '')
  const cut = s.search(/[/?#]/)
  if (cut >= 0) s = s.slice(0, cut)
  s = s.replace(/:\d+$/, '')
  return /^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(s) ? s : ''
}

// 当前生效的外链主机名；自定义无效时回退默认
export function externalHost() {
  if (settings.externalHost === 'custom') {
    return sanitizeHost(settings.externalHostCustom) || DEFAULT_EXTERNAL_HOST
  }
  return settings.externalHost || DEFAULT_EXTERNAL_HOST
}

// 当前生效的图片主机名；自定义无效时回退默认
export function picHost() {
  if (settings.picHost === 'custom') {
    return sanitizeHost(settings.picHostCustom) || DEFAULT_PIC_HOST
  }
  return settings.picHost || DEFAULT_PIC_HOST
}

// 站内实体外链统一入口：externalUrl('subject'|'person'|'character', id)
export function externalUrl(kind, id) {
  return `https://${externalHost()}/${kind}/${id}`
}
