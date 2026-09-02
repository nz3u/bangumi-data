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

// 高亮功能预设：四个高亮位置各自独立开关，并可由总开关统一开/关。
//   suggest 搜索建议：搜索框下拉推荐、人物/角色搜索结果列表
//   filter  页内快速筛选：人物合作 / 双人合作 / 单人作品页筛选框命中的文本
//   title   条目页标题：条目搜索结果的中文名与原名
//   tags    条目页标签：条目搜索结果中命中的正向标签
export const HIGHLIGHT_FEATURE_PRESETS = [
  { value: 'suggest', label: '搜索建议', hint: '搜索框下拉推荐与人物/角色搜索结果' },
  { value: 'filter', label: '页内快速筛选', hint: '合作 / 双人 / 单人页筛选框命中的文本' },
  { value: 'title', label: '条目标题', hint: '条目搜索结果的中文名与原名' },
  { value: 'tags', label: '条目标签', hint: '条目搜索结果中命中的标签' }
]

export const DEFAULT_HIGHLIGHT_FEATURES = {
  suggest: true,
  filter: true,
  title: false,
  tags: true
}

// 旧版配置迁移：searchHighlight → suggest/filter，highlightAreas → title/tags。
// 已存过新版（含 highlightFeatures）时以新版为准，不再回退。
function normalizeHighlightFeatures(raw) {
  const feats = { ...DEFAULT_HIGHLIGHT_FEATURES }
  const cur = raw && typeof raw === 'object' ? raw.highlightFeatures : null
  if (cur && typeof cur === 'object') {
    for (const f of HIGHLIGHT_FEATURE_PRESETS) {
      if (typeof cur[f.value] === 'boolean') feats[f.value] = cur[f.value]
    }
    return feats
  }
  const legacy = !(raw && raw.searchHighlight === false)
  feats.suggest = legacy
  feats.filter = legacy
  if (raw && Array.isArray(raw.highlightAreas)) {
    feats.title = raw.highlightAreas.includes('title')
    feats.tags = raw.highlightAreas.includes('tags')
  }
  return feats
}

function load() {
  let raw = {}
  try {
    const text = localStorage.getItem(KEY)
    if (text) raw = JSON.parse(text) || {}
  } catch {
    /* 损坏的存储内容按默认处理 */
  }
  if (!raw || typeof raw !== 'object') raw = {}
  raw.highlightFeatures = normalizeHighlightFeatures(raw)
  return raw
}

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
  highlightFeatures: { ...DEFAULT_HIGHLIGHT_FEATURES }, // 四个高亮位置的独立开关
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
        highlightFeatures: { ...settings.highlightFeatures },
        // 兼容旧版前端：同步写出被取代的两个字段
        highlightAreas: ['title', 'tags'].filter((a) => isHighlightOn(a)),
        searchHighlight: isHighlightOn('suggest') || isHighlightOn('filter'),
        highlightColor: settings.highlightColor,
        highlightColorCustom: settings.highlightColorCustom
      })
    )
  } catch {
    /* 隐私模式等场景下写入失败不影响使用 */
  }
}

// 指定高亮功能是否开启（suggest | filter | title | tags）
export function isHighlightOn(feature) {
  const m = settings.highlightFeatures
  if (m && typeof m[feature] === 'boolean') return m[feature]
  return DEFAULT_HIGHLIGHT_FEATURES[feature] === true
}

// 是否有任意一个高亮功能处于开启状态（决定是否展示高亮颜色设置）
export function isAnyHighlightOn() {
  return HIGHLIGHT_FEATURE_PRESETS.some((f) => isHighlightOn(f.value))
}

// 已开启的高亮功能数量，用于设置面板的计数提示
export function highlightOnCount() {
  return HIGHLIGHT_FEATURE_PRESETS.filter((f) => isHighlightOn(f.value)).length
}

// 切换单个高亮功能并立即持久化
export function toggleHighlightFeature(feature) {
  setHighlightFeature(feature, !isHighlightOn(feature))
}

// 设置单个高亮功能并立即持久化
export function setHighlightFeature(feature, on) {
  settings.highlightFeatures = { ...settings.highlightFeatures, [feature]: !!on }
  saveSettings()
}

// 统一开关：一次性开启或关闭全部高亮功能
export function setAllHighlight(on) {
  const next = {}
  for (const f of HIGHLIGHT_FEATURE_PRESETS) next[f.value] = !!on
  settings.highlightFeatures = next
  saveSettings()
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
