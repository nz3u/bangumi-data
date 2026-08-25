import { navigate } from 'svelte5-router'

// 跨标签页跳转的内部传参：参数只经内存传递，不写入地址栏（不产生历史记录）。
//
// - goToTab(path, params)：向目标页送达参数并跳转。
//   目标页已挂载（含当前页跳转到自己）→ 直调其处理器；否则暂存，待挂载时领取。
// - onNavParams(path, fn)：目标视图在 onMount 中注册处理器；
//   注册时若存在「在途」暂存参数则立即送达（实现挂载即消费），
//   返回注销函数，避免组件卸载后再收到过期调用。

const consumers = new Map() // path -> Set<(params) => void>
const stash = new Map() // path -> 待消费参数（目标页尚未挂载）

export function goToTab(path, params) {
  const p = params ?? null
  const set = consumers.get(path)
  if (set && set.size > 0) {
    for (const fn of set) fn(p)
    if (window.location.pathname !== path) navigate(path)
  } else {
    stash.set(path, p)
    navigate(path)
  }
}

export function onNavParams(path, fn) {
  if (!consumers.has(path)) consumers.set(path, new Set())
  consumers.get(path).add(fn)

  let off = () => consumers.get(path)?.delete(fn)
  if (stash.has(path)) {
    const p = stash.get(path)
    stash.delete(path)
    try {
      fn(p)
    } catch (e) {
      console.error('onNavParams handler failed:', e)
    }
  }
  return off
}
