<script>
  import { requestPic, picInfo, picUrl } from '../lib/pics.svelte.js'

  // 实体图片组件（条目封面 / 人物头像 / 角色头像）：
  // - 经全局队列调用统一图片解析接口，结果跨页缓存共享
  // - 解析/下载中显示骨架动画，确认无图或加载失败显示占位图标
  // - 加载完成后按图片原始比例展示（不再强制 3:4 裁切），
  //   通过 max-h 约束高度上限；外部可用 class 传入最大宽度（如 max-w-40）
  // - href 可选：整卡点击跳转（如 bgm.tv 对应页面）
  let {
    kind, // 'subject' | 'person' | 'character'
    id,
    alt = '',
    class: cls = '',
    size = 'l',
    href = null
  } = $props()

  let el = $state(null)
  let visible = $state(false)
  let imgState = $state('idle') // idle | loaded | broken

  const info = $derived(id != null ? picInfo(kind, id, size) : null)
  const busy = $derived(info?.status === 'loading' || (!!info?.path && imgState === 'idle'))
  const bad = $derived(info?.status === 'failed' || imgState === 'broken')
  const loaded = $derived(info?.status === 'ok' && imgState === 'loaded')
  // 完整 URL 由前端拼接（响应式）：切换设置中的图片主机即时生效
  const src = $derived(picUrl(info))

  // 加载中/失败：固定小占位框；加载完成：包裹图片自然尺寸（受最大宽高约束）
  const cardCls = $derived(
    `relative overflow-hidden rounded-lg border border-neutral-200/80 bg-neutral-50 shadow-sm dark:border-white/[0.08] dark:bg-white/[0.04] ${
      loaded ? 'w-fit' : 'flex aspect-[3/4] w-24 items-center justify-center'
    } ${cls}`
  )

  // URL 变化后重置图片加载状态
  $effect(() => {
    void info?.path
    imgState = 'idle'
  })

  // 进入视口（含上下缓冲区）后标记可见并触发请求
  $effect(() => {
    if (!el || visible) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          visible = true
          io.disconnect()
        }
      },
      { rootMargin: '300px' }
    )
    io.observe(el)
    return () => io.disconnect()
  })

  $effect(() => {
    if (visible && id != null) requestPic(kind, id, size)
  })
</script>

{#snippet inner()}
  {#if info?.status === 'ok' && imgState !== 'broken'}
    <img
      src={src}
      alt={alt}
      loading="lazy"
      decoding="async"
      class={`block h-auto w-auto max-w-full max-h-72 object-contain transition-opacity duration-300 ${imgState === 'loaded' ? 'opacity-100' : 'opacity-0'}`}
      onload={() => (imgState = 'loaded')}
      onerror={() => (imgState = 'broken')}
    />
  {:else if busy}
    <span class="absolute inset-0 animate-pulse bg-neutral-200/80 dark:bg-neutral-700/60" aria-hidden="true"></span>
  {:else if bad}
    <span class="absolute inset-0 flex items-center justify-center text-neutral-300 dark:text-neutral-600" title="暂无图片">
      <svg class="size-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <circle cx="9" cy="9" r="2" />
        <path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21" />
      </svg>
    </span>
  {/if}
{/snippet}

<div class="w-fit shrink-0">
  {#if href}
    <a bind:this={el} {href} target="_blank" rel="noreferrer" class={`block ${cardCls}`} title={alt}>
      {@render inner()}
    </a>
  {:else}
    <div bind:this={el} class={cardCls} title={alt}>
      {@render inner()}
    </div>
  {/if}
</div>
