<script>
  import { fade, fly } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'

  // 移动端全宽，sm 起为 60% 宽度
  let { open = false, widthClass = 'w-full sm:w-[60%] sm:min-w-96', label = '详情', onclose, children } = $props()

  $effect(() => {
    if (!open) return
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = ''
    }
  })
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && open) onclose?.() }} />

{#if open}
  <div class="fixed inset-0 z-50" role="dialog" aria-modal="true" aria-label={label}>
    <div class="absolute inset-0 bg-neutral-950/45 backdrop-blur-[2px]" transition:fade={{ duration: 180 }} aria-hidden="true" onclick={() => onclose?.()}></div>
    <aside
      class={`absolute inset-y-0 right-0 flex ${widthClass} max-w-full flex-col border-l border-neutral-200/80 bg-white shadow-2xl dark:border-white/[0.08] dark:bg-neutral-900`}
      transition:fly={{ x: 480, duration: 320, easing: cubicOut }}
    >
      {@render children?.()}
    </aside>
  </div>
{/if}
