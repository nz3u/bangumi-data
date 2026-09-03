// 视图按需加载注册表：
// - loadView(path)：首次调用才触发动态导入（Vite 据此把每个视图拆成独立 chunk），
//   后续调用复用已加载模块（import() 自带模块缓存，路由往返不再产生请求）
// - prefetchView(path)：导航悬停/聚焦时预热对应 chunk，加载失败静默（进入页面时还会正式加载）
const registry = {
  '/': () => import('../views/CollaborationsView.svelte'),
  '/collaborations': () => import('../views/CollaborationsView.svelte'),
  '/pairworks': () => import('../views/PairWorksView.svelte'),
  '/singleworks': () => import('../views/SingleWorksView.svelte'),
  '/subjects': () => import('../views/SubjectsView.svelte'),
  '/persons': () => import('../views/PersonsView.svelte'),
  '/characters': () => import('../views/CharactersView.svelte'),
  '/setup': () => import('../views/SetupView.svelte'),
  '/admin': () => import('../views/AdminView.svelte')
}

const cache = new Map()

export function loadView(path) {
  if (!cache.has(path)) cache.set(path, registry[path]())
  return cache.get(path)
}

export function prefetchView(path) {
  if (registry[path]) loadView(path).catch(() => {})
}
