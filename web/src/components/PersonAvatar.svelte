<script>
  import { onDestroy } from 'svelte'
  import { requestPic, picStore } from '../lib/personPic.svelte.js'

  // 统一的人物头像组件：
  // - 懒加载：进入视口附近才向全局队列发起解析请求
  // - 等待/失败/为空时显示名字首字符占位（沿用调用处的配色样式）
  // - 右上角状态角标：解析中/图片下载中显示转圈，无图或加载失败显示打叉
  // 外层尺寸/圆角/配色通过 class 传入，与原字符头像完全一致。
  let {
    pid,
    name = '',
    class: cls = 'size-14',
    title = '',
    onclick = null
  } = $props()

  let el = $state(null)
  let visible = $state(false)
  let imgState = $state('idle') // idle | loaded | broken

  const info = $derived(pid != null ? picStore[String(pid)] : null)
  const initial = $derived((name || '?').slice(0, 1))
  // 解析中，或 URL 已就绪但图片本体仍在下载 → 转圈
  const busy = $derived(info?.status === 'loading' || (!!info?.url && imgState === 'idle'))
  // 后端确认无图/抓取失败，或图片请求出错 → 打叉
  const bad = $derived(info?.status === 'failed' || imgState === 'broken')

  // 缓存命中时解析+下载往往几十毫秒就完成，转圈一闪而过；
  // 这里保证转圈出现后至少停留 MIN_SPINNER_MS 再消失（不阻塞队列）。
  const MIN_SPINNER_MS = 500
  let spinnerOn = $state(false)
  let spinnerTimer = null

  $effect(() => {
    if (busy) {
      clearTimeout(spinnerTimer)
      spinnerTimer = null
      spinnerOn = true
    } else if (spinnerOn && !spinnerTimer) {
      spinnerTimer = setTimeout(() => {
        spinnerTimer = null
        spinnerOn = false
      }, MIN_SPINNER_MS)
    }
  })

  onDestroy(() => clearTimeout(spinnerTimer))

  // 头像 URL 变化后重置图片加载状态
  $effect(() => {
    void info?.url
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
    if (visible && pid != null) requestPic(pid)
  })
</script>

{#snippet inner()}
  <span class="absolute inset-0 flex items-center justify-center">{initial}</span>
  {#if info?.status === 'ok' && imgState !== 'broken'}
    <img
      src={info.url}
      alt={name}
      loading="lazy"
      decoding="async"
      class={`size-full object-cover transition-opacity duration-300 ${imgState === 'loaded' ? 'opacity-100' : 'opacity-0'}`}
      onload={() => (imgState = 'loaded')}
      onerror={() => (imgState = 'broken')}
    />
  {/if}
{/snippet}

<div class="relative w-fit shrink-0">
  {#if onclick}
    <button bind:this={el} type="button" class={`relative block overflow-hidden ${cls}`} {title} {onclick}>
      {@render inner()}
    </button>
  {:else}
    <div bind:this={el} class={`relative overflow-hidden ${cls}`}>
      {@render inner()}
    </div>
  {/if}
  {#if spinnerOn}
    <span
      class="absolute -right-1 -top-1 z-10 flex size-4 items-center justify-center rounded-full bg-white shadow-sm ring-1 ring-neutral-200 dark:bg-neutral-900 dark:ring-neutral-700"
      title="头像解析中…"
    >
      <svg class="size-2.5 animate-spin text-sky-600 dark:text-sky-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" aria-hidden="true">
        <path d="M12 3a9 9 0 1 0 9 9" stroke-linecap="round" />
      </svg>
    </span>
  {:else if bad}
    <span
      class="absolute -right-1 -top-1 z-10 flex size-4 items-center justify-center rounded-full bg-white shadow-sm ring-1 ring-neutral-200 dark:bg-neutral-900 dark:ring-neutral-700"
      title={info?.status === 'failed' ? '暂无头像' : '头像加载失败'}
    >
      <svg class="size-2.5 text-red-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" aria-hidden="true">
        <path d="M18 6 6 18M6 6l12 12" />
      </svg>
    </span>
  {/if}
</div>
