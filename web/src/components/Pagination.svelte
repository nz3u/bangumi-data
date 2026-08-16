<script>
  let { total, page, size, onchange } = $props()

  const pages = $derived(Math.max(1, Math.ceil(total / size)))
</script>

<div class="flex items-center gap-3 text-sm text-neutral-400">
  <span>共 {total} 条 · 第 {page} / {pages} 页</span>
  <div class="flex gap-1">
    <button class="btn-mini" disabled={page <= 1} onclick={() => onchange?.(page - 1)}>上一页</button>
    <button class="btn-mini" disabled={page >= pages} onclick={() => onchange?.(page + 1)}>下一页</button>
  </div>
</div>