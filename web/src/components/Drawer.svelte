<script>
  import { fade, fly } from 'svelte/transition'

  let { open = false, widthClass = 'w-[60%]', label = '详情', onclose, children } = $props()

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
    <div class="absolute inset-0 bg-black/40" transition:fade={{ duration: 150 }} aria-hidden="true" onclick={() => onclose?.()}></div>
    <aside
      class={`absolute inset-y-0 right-0 flex ${widthClass} max-w-full min-w-80 flex-col border-l border-neutral-200 bg-white shadow-2xl dark:border-neutral-800 dark:bg-neutral-900`}
      transition:fly={{ x: 80, duration: 200 }}
    >
      {@render children?.()}
    </aside>
  </div>
{/if}
