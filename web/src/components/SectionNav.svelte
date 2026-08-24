<script>
  // 详情抽屉左侧快速跳转导航：
  // - 收起时显示一列 "-"（每个区块对应一个），滚动进度接近哪个区块就高亮对应的 "-"
  // - 悬停时 "-" 保留，原地展开为「- 信息」「- 简介」…；点击任意项平滑滚动至对应区块
  //   （触屏无悬停也可直接点击 "-" 跳转）
  // - 区块列表来自容器内标记 data-sec-label 的元素，经 MutationObserver 随内容变化自动刷新
  let { container = null } = $props()

  let labels = $state([])
  let active = $state(-1)

  function collect() {
    if (!container) {
      labels = []
      return
    }
    const next = [...container.querySelectorAll('[data-sec-label]')].map((el) => el.getAttribute('data-sec-label'))
    if (next.join('|') !== labels.join('|')) labels = next
  }

  // 以「区块顶部越过容器顶部下方 80px」为界确定当前区块；接近底部时强制高亮最后一项
  function updateActive() {
    if (!container || !labels.length) {
      active = -1
      return
    }
    const cTop = container.getBoundingClientRect().top
    let idx = 0
    container.querySelectorAll('[data-sec-label]').forEach((el, i) => {
      if (el.getBoundingClientRect().top - cTop <= 80) idx = i
    })
    if (container.scrollTop + container.clientHeight >= container.scrollHeight - 4) idx = labels.length - 1
    active = idx
  }

  function jump(label) {
    if (!container) return
    const el = container.querySelector(`[data-sec-label="${label}"]`)
    if (!el) return
    const delta = el.getBoundingClientRect().top - container.getBoundingClientRect().top
    container.scrollTo({ top: container.scrollTop + delta - 8, behavior: 'smooth' })
  }

  const onScroll = () => updateActive()

  $effect(() => {
    if (!container) return
    collect()
    updateActive()
    const mo = new MutationObserver(() => {
      collect()
      updateActive()
    })
    mo.observe(container, { childList: true, subtree: true })
    container.addEventListener('scroll', onScroll, { passive: true })
    return () => {
      mo.disconnect()
      container.removeEventListener('scroll', onScroll)
    }
  })
</script>

<nav class="group/nav absolute left-1.5 top-1/2 z-10 -translate-y-1/2" aria-label="章节快速跳转">
  <!-- 收起：仅一列细横杠；悬停：容器浮现卡片底色，各横杠右侧展开标签 -->
  <div
    class="flex flex-col items-stretch gap-1 rounded-md border border-transparent py-2 transition-all duration-150 group-hover/nav:border-neutral-200 group-hover/nav:bg-white/95 group-hover/nav:shadow-lg group-hover/nav:backdrop-blur-sm dark:group-hover/nav:border-neutral-700 dark:group-hover/nav:bg-neutral-900/95"
  >
    {#each labels as label, i (label)}
      <button
        type="button"
        title={label}
        aria-current={i === active ? 'true' : undefined}
        class={`flex cursor-pointer items-center px-1 py-0.5 text-left transition-colors ${
          i === active
            ? 'rounded bg-neutral-100 text-sky-600 dark:bg-neutral-800 dark:text-sky-400'
            : 'text-neutral-400 hover:text-neutral-600 dark:text-neutral-500 dark:hover:text-neutral-300'
        }`}
        onclick={() => jump(label)}
      >
        <span class={`w-2 text-center text-xs leading-none ${i === active ? 'font-bold' : ''}`}>-</span>
        <span
          class="max-w-0 overflow-hidden text-xs leading-none whitespace-nowrap opacity-0 transition-all duration-200 group-hover/nav:max-w-32 group-hover/nav:pl-1.5 group-hover/nav:opacity-100"
        >
          {label}
        </span>
      </button>
    {/each}
  </div>
</nav>
