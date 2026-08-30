<script>
  import { link, useLocation } from 'svelte5-router'
  import { prefetchView } from '../lib/lazyViews.js'

  let { items } = $props()

  const location = useLocation()

  // 当前激活标签：路径命中则高亮对应项；否则（含根路径 /）默认聚焦第一个标签页。
  // URL 保持不变，点击后仍按原逻辑跳转并更新地址栏。
  const active = $derived.by(() => {
    const p = $location.pathname
    return items.some((it) => it.path === p) ? p : (items[0]?.path ?? '')
  })

  // 各功能模块图标（lucide 风格线性路径，stroke currentColor）
  const ICONS = {
    collaborations: [
      'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2',
      'M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z',
      'M22 21v-2a4 4 0 0 0-3-3.87',
      'M16 3.13a4 4 0 0 1 0 7.75'
    ],
    pairworks: ['m16 3 4 4-4 4', 'M20 7H4', 'm8 21-4-4 4-4', 'M4 17h16'],
    singleworks: ['M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2', 'M16 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z'],
    subjects: [
      'M12 7v14',
      'M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z'
    ],
    persons: ['M10.3 15H7a4 4 0 0 0-4 4v2', 'M10 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z', 'M17 20a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z', 'm21 21-1.9-1.9'],
    characters: ['M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z', 'M8 14s1.5 2 4 2 4-2 4-2', 'M9 9h.01', 'M15 9h.01']
  }

  // 滑动下划线指示器：测量激活标签的位置，切换时平滑滑动；
  // -bottom-px 使 2px 樱花色下划线压住容器 1px 分割线，视觉上融合为一体。
  let navEl = $state(null)
  let linkEls = {}
  let ind = $state({ left: 0, width: 0, ready: false })

  function measure() {
    const el = linkEls[active]
    if (!el) return
    ind = { left: el.offsetLeft, width: el.offsetWidth, ready: true }
    // 路由切换时把激活标签滚到横滑条中间，露出两侧模块提示可继续滚动；
    // 首尾标签自然停靠边界，已在中间时为最小滚动。尊重系统减少动态效果设置。
    const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches
    el.scrollIntoView({ inline: 'center', block: 'nearest', behavior: reduce ? 'auto' : 'smooth' })
  }

  // 激活项变化后测量（microtask 等 DOM 更新完成）
  $effect(() => {
    void active
    queueMicrotask(measure)
  })

  // 容器尺寸变化（视口缩放、字体加载）时重新测量
  $effect(() => {
    if (!navEl) return
    const ro = new ResizeObserver(() => measure())
    ro.observe(navEl)
    return () => ro.disconnect()
  })
</script>

<!-- 移动端横向滑动，桌面端完整展示 -->
<div bind:this={navEl} class="no-scrollbar overflow-x-auto border-b border-neutral-200/80 dark:border-white/[0.06]">
  <nav class="relative flex min-w-max gap-1" aria-label="主导航">
    {#if ind.ready}
      <span
        class="absolute -bottom-px h-0.5 rounded-full bg-sakura-500 transition-[left,width] duration-300 ease-out dark:bg-sakura-400"
        style="left: {ind.left}px; width: {ind.width}px"
        aria-hidden="true"
      ></span>
    {/if}
    {#each items as it (it.key)}
      <a
        href={it.path}
        use:link
        bind:this={linkEls[it.path]}
        aria-current={active === it.path ? 'page' : undefined}
        onpointerenter={() => prefetchView(it.path)}
        onfocus={() => prefetchView(it.path)}
        class="relative z-10 flex items-center gap-1.5 rounded-t-lg px-3 py-2.5 text-sm whitespace-nowrap transition-colors {active === it.path
          ? 'font-medium text-sakura-600 dark:text-sakura-300'
          : 'text-neutral-500 hover:bg-neutral-100/70 hover:text-neutral-900 dark:text-neutral-400 dark:hover:bg-white/[0.05] dark:hover:text-neutral-100'}"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          class="size-4 shrink-0"
          aria-hidden="true"
        >
          {#each ICONS[it.key] ?? [] as d}
            <path {d} />
          {/each}
        </svg>
        {it.label}
      </a>
    {/each}
  </nav>
</div>
