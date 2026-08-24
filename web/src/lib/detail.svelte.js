// 全局详情抽屉状态（应用内唯一实例，由 DetailDrawer 组件渲染）。
//
// 抽屉状态完全由地址栏锚点描述，形如：#/subject@1、#/person@596、#/character@123
// - 打开/切换实体 = 写入锚点（产生历史记录，可后退关闭/回溯）
// - 锚点变化 = 加载新内容替换当前抽屉内容；浏览器前进/后退同样生效
// - 锚点不匹配任何实体时抽屉关闭；因此同一时刻至多只有一个抽屉

export const detailDrawer = $state({ kind: null, id: null })

const HASH_RE = /^#\/(subject|person|character)@(\d+)$/

function parseHash(hash) {
  const m = HASH_RE.exec(hash ?? '')
  return m ? { kind: m[1], id: Number(m[2]) } : null
}

// 以当前地址栏锚点为准同步抽屉状态。
// hashchange 与路由跳转（pushState 会剥离锚点、且不触发 hashchange）后都需调用。
export function syncDetailFromHash() {
  const t = parseHash(location.hash)
  detailDrawer.kind = t?.kind ?? null
  detailDrawer.id = t?.id ?? null
}

// 锚点与当前地址相同的内部链接，浏览器默认不会触发 hashchange（表现为"点了没反应"）。
// 全局委托拦截后强制按锚点重新加载，保证同一链接可重复点击。
function onDocClick(e) {
  const a = e.target?.closest?.('a[href^="#/"]')
  if (a && a.getAttribute('href') === location.hash) {
    e.preventDefault()
    syncDetailFromHash()
  }
}

// 初始化：解析初始锚点并监听后续变化；返回清理函数
export function initDetailDrawer() {
  syncDetailFromHash()
  window.addEventListener('hashchange', syncDetailFromHash)
  document.addEventListener('click', onDocClick)
  return () => {
    window.removeEventListener('hashchange', syncDetailFromHash)
    document.removeEventListener('click', onDocClick)
  }
}

// 打开/切换到指定实体；brief 为可选的列表行数据，
// 暂存后在详情请求返回前供抽屉头部即时渲染名称等信息。
export function openDetail(kind, id, brief) {
  primeBrief(kind, id, brief)
  const href = detailHref(kind, id)
  if (location.hash === href) syncDetailFromHash()
  else location.hash = href
}

// 关闭抽屉：移除锚点（replace 不产生多余历史记录）
export function closeDetail() {
  if (!parseHash(location.hash)) return
  history.replaceState(history.state, '', location.pathname + location.search)
  syncDetailFromHash()
}

// 实体内部链接地址（抽屉页内部跳转统一使用）。
// 兼容数据源原始类型值：prsn -> person、crt -> character
const KIND_ALIASES = { prsn: 'person', crt: 'character' }

export function detailHref(kind, id) {
  const k = KIND_ALIASES[kind] ?? kind
  return `#/${k}@${Number(id)}`
}

// ---- 详情就绪前的即时渲染缓存 ----

const briefCache = new Map()

function normKind(kind) {
  return KIND_ALIASES[kind] ?? kind
}

function primeBrief(kind, id, brief) {
  if (kind && id != null && brief) briefCache.set(`${normKind(kind)}/${Number(id)}`, brief)
}

// 读取暂存的列表行基础信息（无则返回 null）
export function peekBrief(kind, id) {
  return briefCache.get(`${normKind(kind)}/${Number(id)}`) ?? null
}
