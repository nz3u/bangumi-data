<script>
  let { fields = [] } = $props()
</script>

{#if fields.length > 0}
  <section class="mb-4">
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">资料</h4>
    <ul class="space-y-0.5 text-sm">
      {#each fields as f}
        <li class="break-words text-neutral-700 dark:text-neutral-300">
          <span class="mr-1 text-neutral-500 dark:text-neutral-400">{f.key}:</span>{String(f.value).replace(/\n+/g, ' / ')}
        </li>
      {/each}
    </ul>
  </section>
{/if}
