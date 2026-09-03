<script>
  import { navigate } from 'svelte5-router'
  // 数据库版本标注：位于页脚右下角。
  //   - 有版本记录且已确认最新：显示导出文件创建时间 + 绿色「已是最新」；
  //   - 无记录（旧版本程序创建的库）：显示「旧版本」；
  //   - 检测到上游有新导出（update_available）：琥珀色高亮 + 「可更新」提醒，
  //     悬浮提示给出最新快照名与更新命令；点击跳转 /setup（SPA）；
  //   - 上游不可达（离线）：仅显示版本，不做最新/落后判定（灰色）。
  // info 为 null（接口不可用/旧后端）时不渲染，避免噪音。
  let { info } = $props()

  const fmtTime = (iso) =>
    new Date(iso).toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false
    })

  const versionLabel = $derived.by(() => {
    const db = info?.database
    if (!db) return null
    if (db.published_at) return fmtTime(db.published_at)
    if (db.version) return db.version.replace(/\.zip$/i, '')
    return null
  })

  const updatable = $derived(Boolean(info?.update_available && info?.latest))
  const uptodate = $derived(!updatable && Boolean(info?.latest))

  const tooltip = $derived.by(() => {
    if (updatable) {
      return `检测到新导出 ${info.latest.version}（${info.latest.published_at ? fmtTime(info.latest.published_at) : '时间未知'} 发布），请运行 bangumi update 更新`
    }
    if (uptodate) return `当前数据库导出于 ${versionLabel}，已是最新版本`
    if (versionLabel == null) return '数据库无版本记录（由旧版本程序导入），建议运行 bangumi import 或 bangumi update 导入最新数据'
    return `当前数据库导出于 ${versionLabel}；暂未获取到上游版本信息（离线或检查中）`
  })
</script>

{#if info}
  {#if updatable}
    <a
      href="/setup"
      onclick={(e)=>{e.preventDefault(); navigate('/setup')}}
      class="ml-auto inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] tabular-nums border-amber-300 bg-amber-50 text-amber-700 hover:bg-amber-100 dark:border-amber-500/40 dark:bg-amber-950/70 dark:text-amber-300 dark:hover:bg-amber-900/50 transition-colors cursor-pointer"
      title={tooltip}
    >
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-3.5 shrink-0" aria-hidden="true">
        <ellipse cx="12" cy="5" rx="9" ry="3" />
        <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5" />
        <path d="M3 12c0 1.66 4 3 9 3s9-1.34 9-3" />
      </svg>
      <span>数据版本：{versionLabel ?? '旧版本'}</span>
      <span class="flex items-center gap-1 font-medium">
        <span class="inline-block size-1.5 animate-pulse rounded-full bg-current"></span>
        可更新
      </span>
    </a>
  {:else}
    <span
      class="ml-auto inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] tabular-nums
        {uptodate
          ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-500/40 dark:bg-emerald-950/70 dark:text-emerald-300'
          : 'border-neutral-200 bg-white/80 text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/80 dark:text-neutral-400'}"
      title={tooltip}
    >
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-3.5 shrink-0" aria-hidden="true">
        <ellipse cx="12" cy="5" rx="9" ry="3" />
        <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5" />
        <path d="M3 12c0 1.66 4 3 9 3s9-1.34 9-3" />
      </svg>
      <span>数据版本：{versionLabel ?? '旧版本'}</span>
      {#if uptodate}
        <span class="flex items-center gap-0.5 font-medium">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="size-3" aria-hidden="true">
            <path d="M20 6 9 17l-5-5" />
          </svg>
          已是最新
        </span>
      {/if}
    </span>
  {/if}
{/if}
