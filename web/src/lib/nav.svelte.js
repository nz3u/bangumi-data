import { navigate } from 'svelte5-router'

// 跨标签页跳转的内部传参：参数只经内存传递，不写入地址栏（不产生历史记录）。
//
// - goToTab(path, params)：登记目标页参数后跳转；
//   若当前已在该标签页，路由不变，参数同样会被消费逻辑送达（seq 变化触发）。
// - 目标视图在 $effect 中调用 consumeNav(path) 消费参数：
//   挂载时消费「在途」参数；同页重复跳转时再次消费。

export const pendingNav = $state({ target: null, params: null, seq: 0 })

// 已消费到的序号：模块级记录，避免视图重挂载后重复应用过期的旧传参
let consumedSeq = 0

export function goToTab(path, params) {
  pendingNav.target = path
  pendingNav.params = params ?? null
  pendingNav.seq++
  navigate(path)
}

// 目标视图消费传参：仅当存在未消费的、指向本页的跳转时返回参数，否则返回 null
export function consumeNav(path) {
  if (pendingNav.target !== path || consumedSeq >= pendingNav.seq) return null
  consumedSeq = pendingNav.seq
  return pendingNav.params
}
