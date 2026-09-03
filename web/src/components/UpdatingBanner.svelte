<script>
  import { onMount } from 'svelte'
  import { openPublicStatusStream } from '../lib/admin.js'

  let updating = $state(false)
  let progress = $state('')

  let esClose = null
  onMount(() => {
    // SSE 即时推送，更新中 5s、空闲 15s 由服务端控制，首包即时
    esClose = openPublicStatusStream((st) => {
      updating = st.state === 'updating'
      progress = st.progress || ''
    })
    return () => esClose && esClose()
  })
</script>

{#if updating}
  <div class="border-b border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-950/40 dark:text-amber-200">
    <div class="mx-auto flex max-w-6xl items-center gap-2">
      <span class="size-2 animate-pulse rounded-full bg-current"></span>
      <span class="font-medium">正在更新中{progress ? `：${progress}` : '…'}</span>
      <span class="hidden sm:inline text-xs opacity-70">服务处于维护模式，数据查询暂时不可用，完成后自动恢复。</span>
      <a href="/setup" class="ml-auto rounded bg-amber-500 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-600">查看进度 →</a>
    </div>
  </div>
{/if}
