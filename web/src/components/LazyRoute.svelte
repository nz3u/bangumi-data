<script>
  import { loadView } from '../lib/lazyViews.js'

  // 按需加载的路由视图容器：仅在所属路由激活（本组件被挂载）时才触发动态导入，
  // 配合 svelte5-router 的 children snippet 形式实现「访问哪个页面加载哪个 chunk」。
  // import() 结果由浏览器模块缓存去重，路由往返不会重复请求。
  let { path } = $props()

  let Comp = $state(null)
  let err = $state('')

  $effect(() => {
    let stale = false
    Comp = null
    err = ''
    loadView(path)
      .then((m) => {
        if (!stale) Comp = m.default
      })
      .catch((e) => {
        if (!stale) err = e?.message || String(e)
      })
    return () => {
      stale = true
    }
  })
</script>

{#if err}
  <div class="card p-4 text-sm text-red-600 dark:text-red-400">页面模块加载失败：{err}（可刷新重试）</div>
{:else if Comp}
  <Comp />
{:else}
  <!-- chunk 加载期间的骨架占位（本地服务通常一闪而过） -->
  <div class="card p-4" aria-busy="true" aria-label="页面加载中">
    <div class="skeleton mb-3 h-4 w-40"></div>
    <div class="space-y-2.5">
      {#each Array.from({ length: 6 }) as _, i}
        <div class="skeleton h-5" style:width="{96 - (i % 4) * 9}%"></div>
      {/each}
    </div>
  </div>
{/if}
