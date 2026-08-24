<script>
  // 详情抽屉左侧快速跳转导航：
  // - 收起时仅显示一个 "-" 小竖条；悬停（或点击，便于触屏）后展开为章节列表
  // - 列表项来自滚动容器内标记了 data-sec-label 的区块（随内容动态收集），
  //   点击后平滑滚动至对应区块；展开列表悬浮于内容之上
  let { container = null } = $props()

  let open = $state(false)

  function sections() {
    if (!container) return []
    return [...container.querySelectorAll('[data-sec-label]')].map((el) => ({
      label: el.getAttribute('data-sec-label'),
    }))
  }

  function jump(label) {
    if (!container) return
    const el = container.querySelector(`[data-sec-label="${label}"]`)
    if (!el) return
    const delta = el.getBoundingClientRect().top - container.getBoundingClientRect().top
    container.scrollTo({ top: container.scrollTop + delta - 8, behavior: 'smooth' })
    open = false
  }
</script>

<nav
  class="absolute left-1.5 top-1/2 z-10 -translate-y-1/2"
  aria-label="章节快速跳转"
  onmouseenter={() => (open = true)}
  onmouseleave={() => (open = false)}
>
  {#if open}
    <div
      class="max-h-[60vh] overflow-y-auto rounded-md border border-neutral-200 bg-white/95 py-1 shadow-lg backdrop-blur-sm dark:border-neutral-700 dark:bg-neutral-900/95"
    >
      {#each sections() as s (s.label)}
        <button
          type="button"
          class="block w-full cursor-pointer whitespace-nowrap px-3 py-1 text-left text-xs text-neutral-600 hover:bg-neutral-100 hover:text-sky-600 dark:text-neutral-300 dark:hover:bg-neutral-800 dark:hover:text-sky-400"
          onclick={() => jump(s.label)}
        >
          {s.label}
        </button>
      {/each}
    </div>
  {:else}
    <button
      type="button"
      aria-label="展开快速跳转"
      title="快速跳转"
      class="block cursor-pointer rounded-md border border-neutral-300 bg-white px-1 py-3 text-[10px] leading-none text-neutral-400 shadow-sm hover:text-sky-600 dark:border-neutral-600 dark:bg-neutral-900 dark:text-neutral-500 dark:hover:text-sky-400"
      onclick={() => (open = true)}
    >
      -
    </button>
  {/if}
</nav>
