// 全局详情抽屉状态（应用内唯一实例，由 DetailDrawer 组件渲染）。
//
// 纯内部状态驱动，不写入地址栏（不产生浏览器历史记录）：
// - 打开/切换实体 = 直接更新状态；关闭 = 清空状态
// - 同一时刻至多只有一个抽屉

export const detailDrawer = $state({ kind: null, id: null })

// 打开/切换到指定实体；brief 为可选的列表行数据，
// 暂存后在详情请求返回前供抽屉头部即时渲染名称等信息。
export function openDetail(kind, id, brief) {
  primeBrief(kind, id, brief)
  detailDrawer.kind = normKind(kind)
  detailDrawer.id = Number(id)
}

// 关闭抽屉：清空状态
export function closeDetail() {
  detailDrawer.kind = null
  detailDrawer.id = null
}

// ---- 详情就绪前的即时渲染缓存 ----

const briefCache = new Map()

// 兼容数据源原始类型值：prsn -> person、crt -> character
function normKind(kind) {
  const KIND_ALIASES = { prsn: 'person', crt: 'character' }
  return KIND_ALIASES[kind] ?? kind
}

function primeBrief(kind, id, brief) {
  if (kind && id != null && brief) briefCache.set(`${normKind(kind)}/${Number(id)}`, brief)
}

// 读取暂存的列表行基础信息（无则返回 null）
export function peekBrief(kind, id) {
  return briefCache.get(`${normKind(kind)}/${Number(id)}`) ?? null
}
