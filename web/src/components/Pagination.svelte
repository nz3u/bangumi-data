<script>
  let { total, page, size, onchange } = $props()

  const pages = $derived(Math.max(1, Math.ceil(total / size)))
  let inputPage = $state('')

  function handleJump() {
    const p = parseInt(inputPage, 10)
    if (!isNaN(p) && p >= 1 && p <= pages && p !== page) {
      onchange?.(p)
    }
    inputPage = ''
  }

  function handleKeydown(e) {
    if (e.key === 'Enter') {
      handleJump()
    }
  }
</script>

<div class="flex flex-wrap items-center gap-x-3 gap-y-2 text-xs text-neutral-400">
  <span class="tabular-nums">共 <b class="font-mono font-medium text-neutral-600 dark:text-neutral-300">{total}</b> 条 · 第 <b class="font-mono font-medium text-neutral-600 dark:text-neutral-300">{page}</b> / {pages} 页</span>
  <div class="flex gap-1">
    <button class="btn-mini" disabled={page <= 1} onclick={() => onchange?.(page - 1)}>上一页</button>
    <button class="btn-mini" disabled={page >= pages} onclick={() => onchange?.(page + 1)}>下一页</button>
  </div>
  {#if pages > 1}
    <div class="flex items-center gap-1">
      <span>跳至</span>
      <input
        type="text"
        class="input w-14 px-1.5 py-0.5 text-center font-mono text-sm"
        bind:value={inputPage}
        onkeydown={handleKeydown}
        placeholder="{page}"
      />
      <span>页</span>
      <button class="btn-mini" onclick={handleJump}>确定</button>
    </div>
  {/if}
</div>
