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

<div class="flex items-center gap-3 text-sm text-neutral-400">
  <span>共 {total} 条 · 第 {page} / {pages} 页</span>
  <div class="flex gap-1">
    <button class="btn-mini" disabled={page <= 1} onclick={() => onchange?.(page - 1)}>上一页</button>
    <button class="btn-mini" disabled={page >= pages} onclick={() => onchange?.(page + 1)}>下一页</button>
  </div>
  {#if pages > 1}
    <div class="flex items-center gap-1">
      <span>跳至</span>
      <input
        type="text"
        class="w-16 px-2 py-1 text-center text-sm font-medium
         border border-gray-300 rounded-md
         bg-white text-gray-800
         placeholder-gray-400
         focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
         dark:bg-neutral-800 dark:text-neutral-200
         dark:border-neutral-600
         dark:placeholder-neutral-500
         dark:focus:ring-blue-400
         transition duration-150 ease-in-out"
        bind:value={inputPage}
        onkeydown={handleKeydown}
        placeholder="{page}"
      />
      <span>页</span>
      <button class="btn-mini" onclick={handleJump}>确定</button>
    </div>
  {/if}
</div>