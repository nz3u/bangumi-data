import { navigate } from 'svelte5-router'

// 标签页搜索参数 <-> 地址栏查询串 双向绑定工具。
//
// 约定：视图以 $location.search 为唯一搜索驱动源——
// - 挂载/地址变化（含前进后退）时解析参数并执行对应搜索；
// - 用户触发搜索/翻页时把表单参数写回地址栏（产生历史记录）。
// 弹窗锚点（#/subject@1 等）仅作用于 hash，与本机制互不影响。

export function parseQuery(search) {
  return new URLSearchParams(String(search ?? '').replace(/^\?/, ''))
}

// 序列化参数：跳过空值；返回不含前导 ? 的查询串
export function buildQuery(params) {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    sp.set(k, String(v))
  }
  return sp.toString()
}

// 导航到同路径的新查询串（push，产生历史记录）；
// 与当前地址完全相同时跳过，避免重复搜索与冗余历史项。
export function pushSearch(path, params) {
  const qs = buildQuery(params)
  const next = qs ? `${path}?${qs}` : path
  const cur = window.location.pathname + window.location.search
  if (cur === next) return
  navigate(next)
}

// 从整数参数读取，缺省/非法返回 fallback
export function intParam(sp, key, fallback = 0) {
  const n = Number.parseInt(sp.get(key) ?? '', 10)
  return Number.isFinite(n) && n > 0 ? n : fallback
}
