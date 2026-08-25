import { constants } from './api.js'

// 条目/人物/角色三个搜索标签页的输入停顿自动搜索间隔：
// 表单相对最近一次已执行的搜索快照有任何变更后，静默该时长即自动提交。
export const AUTO_SEARCH_DEBOUNCE_MS = 1000

let cache = null

export async function loadConstants() {
  if (!cache) cache = await constants()
  return cache
}

export function enumList(map) {
  return Object.entries(map ?? {})
    .map(([id, name]) => ({ id: Number(id), name }))
    .sort((a, b) => a.id - b.id)
}

export function platformsFor(cons, type) {
  const map = cons?.platforms?.[String(type)]
  if (!map) return []
  return Object.entries(map)
    .map(([id, p]) => ({ id: Number(id), name: p.type_cn || p.type || `平台${id}` }))
    .sort((a, b) => a.id - b.id)
}