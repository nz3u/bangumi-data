import { constants } from './api.js'

// 条目/人物/角色三个搜索标签页的输入停顿自动搜索间隔：
// 表单相对最近一次已执行的搜索快照有任何变更后，静默该时长即自动提交。
export const AUTO_SEARCH_DEBOUNCE_MS = 1000

// 条目搜索页专属：整体停顿延长至 2s；若任一标签建议框仍处于激活（聚焦）状态，
// 需其内容静默 TAG_BOX_IDLE_SEARCH_MS 后才触发，避免选词过程中频繁发请求。
export const SUBJECT_AUTO_SEARCH_DEBOUNCE_MS = 2000
export const TAG_BOX_IDLE_SEARCH_MS = 3000

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

