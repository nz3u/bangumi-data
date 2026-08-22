<script>
  let { items, active, onchange } = $props()
</script>

<div class="flex gap-1 border-b border-neutral-200 dark:border-neutral-800">
  {#each items as it (it.key)}
    <button
      class="px-4 py-2 text-sm transition-colors border-b-2 {active === it.key
        ? 'border-sky-500 text-sky-600 dark:text-sky-400'
        : 'border-transparent text-neutral-500 hover:text-neutral-800 dark:text-neutral-400 dark:hover:text-neutral-200'}"
      onclick={() => onchange?.(it.key)}
    >
      {it.label}
    </button>
  {/each}
</div>