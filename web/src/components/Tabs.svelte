<script>
  import { link, useLocation } from 'svelte5-router'

  let { items } = $props()

  const location = useLocation()

  // 当前激活标签：路径命中则高亮对应项；否则（含根路径 /）默认聚焦第一个标签页。
  // URL 保持不变，点击后仍按原逻辑跳转并更新地址栏。
  const active = $derived.by(() => {
    const p = $location.pathname
    return items.some((it) => it.path === p) ? p : (items[0]?.path ?? '')
  })
</script>

<div class="flex gap-1 border-b border-neutral-200 dark:border-neutral-800">
  {#each items as it (it.key)}
    <a
      href={it.path}
      use:link
      aria-current={active === it.path ? 'page' : undefined}
      class="px-4 py-2 text-sm transition-colors border-b-2 {active === it.path
        ? 'border-sky-500 text-sky-600 dark:text-sky-400'
        : 'border-transparent text-neutral-500 hover:text-neutral-800 dark:text-neutral-400 dark:hover:text-neutral-200'}"
    >
      {it.label}
    </a>
  {/each}
</div>
