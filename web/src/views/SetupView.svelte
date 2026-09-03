<script>
  import { onMount, onDestroy } from 'svelte'
  import { link } from 'svelte5-router'
  import { dbInfo } from '../lib/api.js'
  import { adminStatus, adminTriggerUpdate, adminCancel, openLogStream, openStatusStream, getStoredToken, setStoredToken } from '../lib/admin.js'

  let status = $state(null)
  let db = $state(null)
  let error = $state('')
  let tokenInput = $state('')
  let showToken = $state(false)
  let logs = $state([])
  let sseClose = $state(null)
  let triggering = $state(false)
  let force = $state(false)
  let showLogs = $state(true)

  let checking = $state(true)
  let needsAuth = $state(false)
  let authed = $state(false)

  let isUpdating = $derived(status?.state === 'updating')
  let dbExists = $derived(status?.db_exists ?? db?.database != null)
  let updateAvailable = $derived(db?.update_available === true)

  function getQueryToken() {
    try { return new URL(window.location.href).searchParams.get('token') || '' } catch { return '' }
  }

  async function checkAuth() {
    checking = true
    needsAuth = false
    try {
      status = await adminStatus()
      authed = true
      error = ''
      // 若 URL 携带 token 且验证通过，持久化
      const qt = getQueryToken()
      if (qt) setStoredToken(qt)
    } catch (e) {
      if (e.status === 401) {
        needsAuth = true
        authed = false
        error = ''
      } else {
        error = e.message
      }
    } finally {
      checking = false
    }
  }

  async function loadDb() {
    try { db = await dbInfo() } catch {}
  }
  async function loadStatus() {
    try {
      status = await adminStatus()
      // 同步更新日志历史（非 updating 也可展示）
      if (status?.logs?.length) {
        // 合并但不重复：若 logs 为空则用 status 历史
        if (logs.length === 0) logs = status.logs.slice(-300)
      }
    } catch (e) {
      if (e.status === 401) { needsAuth = true; authed = false }
    }
  }

  function startStream() {
    if (sseClose) sseClose()
    sseClose = openLogStream((line) => {
      logs = [...logs, line].slice(-600)
      queueMicrotask(() => {
        const el = document.getElementById('setup-log')
        if (el && showLogs) el.scrollTop = el.scrollHeight
      })
    })
  }

  async function doTrigger() {
    if (triggering) return
    triggering = true
    error = ''
    try {
      await adminTriggerUpdate(force)
      await loadStatus()
      await loadDb()
    } catch (e) {
      if (e.status === 401) { needsAuth = true; authed = false }
      else error = e.message
    } finally {
      triggering = false
    }
  }

  async function doCancel() {
    try { await adminCancel() } catch (e) { error = e.message }
    await loadStatus()
  }

  function saveToken() {
    const v = tokenInput.trim()
    if (!v) { error = '请输入 token'; return }
    setStoredToken(v)
    // 清理 URL 中的 token 参数（避免泄露）
    try {
      const url = new URL(window.location.href)
      if (url.searchParams.has('token')) {
        url.searchParams.delete('token')
        history.replaceState(null, '', url.toString())
      }
    } catch {}
    checkAuth().then(() => {
      if (authed) {
        loadDb(); loadStatus(); startStream();
      }
    })
  }

  function clearToken() {
    setStoredToken('')
    tokenInput = ''
    needsAuth = true
    authed = false
    status = null
  }

  let statusEsClose = null
  function startStatusStream() {
    if (statusEsClose) statusEsClose()
    statusEsClose = openStatusStream((st) => {
      status = st
    })
  }
  function stopStatusStream() {
    if (statusEsClose) { statusEsClose(); statusEsClose = null }
  }

  // SSE 状态推送：服务端即时推送 + 更新中 3s / 空闲 15s 间隔
  $effect(() => {
    if (!authed || checking || needsAuth) {
      stopStatusStream()
      return
    }
    startStatusStream()
    return () => stopStatusStream()
  })

  // dbInfo 间隔推送 30s（status 已 SSE 即时，仅版本信息可慢）
  let dbTimer = null
  $effect(() => {
    if (!authed || checking || needsAuth) {
      if (dbTimer) clearInterval(dbTimer)
      return
    }
    loadDb()
    dbTimer = setInterval(() => {
      if (document.hidden) return
      loadDb()
    }, 30000)
    return () => clearInterval(dbTimer)
  })

  onMount(async () => {
    const qt = getQueryToken()
    tokenInput = qt || getStoredToken()
    if (qt) setStoredToken(qt)
    await checkAuth()
    if (authed) {
      await loadDb()
      startStream()
    }
  })

  onDestroy(() => {
    if (dbTimer) clearInterval(dbTimer)
    stopStatusStream()
    if (sseClose) sseClose()
  })
</script>

{#if checking}
  <div class="flex justify-center py-16">
    <div class="text-sm text-neutral-500">校验中…</div>
  </div>
{:else if needsAuth}
  <div class="mx-auto flex min-h-[50vh] max-w-md items-center justify-center px-4">
    <div class="w-full rounded-2xl border border-neutral-200 bg-white p-6 shadow-sm dark:border-white/10 dark:bg-neutral-900">
      <div class="flex items-center gap-2.5">
        <span class="inline-flex size-8 items-center justify-center rounded-xl bg-sakura-500 text-white">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg>
        </span>
        <h2 class="text-base font-semibold">需要验证</h2>
        <span class="ml-auto rounded-full bg-amber-50 px-2 py-0.5 text-xs text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">/setup</span>
      </div>
      <p class="mt-3 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
        访问 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">/setup</code> 与 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">/admin</code> 需要 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">admin_token</code> 鉴权。<br/>
        初始化时已自动生成并写入 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">data/config.json</code>，请通过 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">bangumi config get admin_token</code> 或启动日志获取，亦可通过 URL <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">?token=</code> 携带。
      </p>
      <div class="mt-4 space-y-2">
        <div class="text-xs font-medium text-neutral-600 dark:text-neutral-400">Admin Token</div>
        <div class="relative">
          <input class="input w-full pr-10" placeholder="粘贴 admin_token" bind:value={tokenInput} type={showToken ? 'text' : 'password'} onkeydown={(e)=> e.key==='Enter' && saveToken()} />
          <button type="button" class="absolute inset-y-0 right-0 flex items-center pr-3 text-neutral-400 hover:text-neutral-600 dark:text-neutral-500 dark:hover:text-neutral-300" onclick={() => showToken = !showToken} aria-label={showToken ? '隐藏' : '显示'} tabindex="-1">
            {#if showToken}
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-4"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
            {:else}
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-4"><path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.94 10.94 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.56 13.56 0 0 0 2 12s3 7 10 7a9.59 9.59 0 0 0 5.39-1.61"/><line x1="2" x2="22" y1="2" y2="22"/></svg>
            {/if}
          </button>
        </div>
        <button class="btn-primary w-full" onclick={saveToken}>验证并进入</button>
        {#if error}<p class="text-xs text-red-600 dark:text-red-400">{error}</p>{/if}
        <p class="text-xs text-neutral-400">验证后 token 将保存在浏览器本地存储，下次自动携带。</p>
      </div>
    </div>
  </div>
{:else}
  <div class="space-y-4 min-w-0">
    <div class="card p-5 min-w-0 overflow-hidden">
      <div class="flex flex-wrap items-start gap-3">
        <span class="inline-flex size-8 items-center justify-center rounded-xl bg-sakura-500 text-white shrink-0">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        </span>
        <div class="min-w-0 flex-1">
          <h2 class="text-lg font-semibold leading-none">初始化 / 更新</h2>
          <p class="mt-1.5 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
            携带 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">token</code> 即可执行初始化或更新。首次部署请点击“开始初始化”；检测到新版或失败时可在此重试。自动更新为 <b>每周三 05:30</b>（需在配置中开启）。
          </p>
        </div>
        <a href="/admin" use:link class="inline-flex shrink-0 rounded-lg border border-neutral-200 px-3 py-1.5 text-xs font-medium hover:bg-neutral-50 dark:border-white/10 dark:hover:bg-white/5">前往配置 →</a>
      </div>

      {#if error}
        <div class="mt-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/20 dark:bg-red-950/30 dark:text-red-300">{error}</div>
      {/if}

      <div class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2 min-w-0">
        <div class="rounded-xl border border-neutral-200 bg-neutral-50/60 p-3.5 dark:border-white/10 dark:bg-white/[0.03] min-w-0 overflow-hidden">
          <div class="text-xs font-medium text-neutral-500">数据库状态</div>
          <div class="mt-2 flex flex-wrap gap-1.5">
            {#if status}
              <span class={`chip ${status.db_exists ? 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950/30 dark:text-emerald-300 dark:border-emerald-800/50' : 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950/30 dark:text-amber-300 dark:border-amber-800/50'}`}>{status.db_exists ? '已存在' : '不存在（待初始化）'}</span>
              <span class="chip">{status.state}</span>
            {:else}
              <span class="text-sm text-neutral-400">加载中…</span>
            {/if}
          </div>
          {#if status?.progress}<div class="mt-2 rounded-lg bg-white px-2.5 py-1.5 text-xs text-neutral-600 dark:bg-white/5 dark:text-neutral-400">{status.progress}</div>{/if}
          <div class="mt-3 space-y-1 text-xs leading-relaxed text-neutral-600 dark:text-neutral-400 min-w-0">
            <div class="flex gap-2 min-w-0"><span class="shrink-0 text-neutral-400">本地</span><span class="min-w-0 flex-1 truncate">{db?.database?.version ?? '无记录'}{#if db?.database?.published_at} · {new Date(db.database.published_at).toLocaleString('zh-CN')}{/if}</span></div>
            <div class="flex gap-2 min-w-0"><span class="shrink-0 text-neutral-400">上游</span><span class="min-w-0 flex-1 truncate">{#if db?.latest}{db.latest.version} · {db.latest.published_at ? new Date(db.latest.published_at).toLocaleString('zh-CN') : '未知'}{#if updateAvailable}<span class="ml-1 rounded bg-amber-500 px-1 py-0.5 text-[10px] leading-none text-white">可更新</span>{/if}{:else}暂未获取{/if}</span></div>
          </div>
          <button class="mt-3 text-xs text-neutral-500 underline decoration-dotted underline-offset-2 hover:text-neutral-700 dark:text-neutral-400" onclick={clearToken}>清除本地 token</button>
        </div>

        <div class="rounded-xl border border-neutral-200 bg-white p-3.5 dark:border-white/10 dark:bg-neutral-900 min-w-0 overflow-hidden">
          <div class="text-xs font-medium text-neutral-500">操作</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <button class="btn-primary disabled:opacity-50" disabled={isUpdating || triggering} onclick={doTrigger}>
              {#if !status?.db_exists}开始初始化{:else if updateAvailable}立即更新{:else}检查并更新{/if}
            </button>
            {#if isUpdating}<button class="btn" onclick={doCancel}>取消</button>{/if}
          </div>
          <div class="mt-2.5 flex flex-wrap gap-3 text-xs">
            <label class="inline-flex items-center gap-1.5 text-neutral-600 dark:text-neutral-400"><input type="checkbox" class="rounded" bind:checked={force} /> 强制重下</label>
            <label class="inline-flex items-center gap-1.5 text-neutral-600 dark:text-neutral-400"><input type="checkbox" class="rounded" bind:checked={showLogs} /> 显示日志</label>
          </div>
          {#if isUpdating}
            <div class="mt-2 flex items-center gap-2 rounded-lg bg-amber-50 px-2.5 py-1.5 text-xs text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">
              <span class="size-2 animate-pulse rounded-full bg-current"></span> 正在更新中，服务处于维护模式…
            </div>
          {/if}
          <p class="mt-2 text-xs leading-relaxed text-neutral-400">流程：下载 → 导入临时库 → 完整性检查 → 原子切库 → 自动恢复。失败不影响旧库。</p>
        </div>
      </div>
    </div>

    {#if showLogs}
      <div class="card overflow-hidden">
        <div class="flex items-center justify-between border-b border-neutral-200 bg-neutral-50/70 px-3 py-2 text-xs dark:border-white/10 dark:bg-white/[0.03]">
          <span class="font-medium">实时日志</span>
          <span class="rounded-full bg-neutral-200 px-2 py-0.5 text-[11px] tabular-nums dark:bg-white/10">{logs.length} 行</span>
          <button class="btn ml-auto text-xs" onclick={() => logs = []}>清空</button>
        </div>
        <pre id="setup-log" class="max-h-[420px] overflow-auto bg-neutral-950 p-3 text-xs leading-relaxed text-neutral-100 dark:bg-[#0a0a0f]">{#each logs as line}{line}
{/each}{#if logs.length===0}<span class="text-neutral-500">暂无日志</span>{/if}</pre>
      </div>
    {/if}

    <p class="text-xs leading-relaxed text-neutral-500 dark:text-neutral-400">
      提示：配置页 <a href="/admin" use:link class="underline decoration-dotted underline-offset-2">/admin</a> 可开关自动更新；Docker 部署首次访问将引导至此页。
    </p>
  </div>
{/if}

<style>
  .card { border: 1px solid rgb(229 229 229 / 0.8); border-radius: 0.9rem; background: white; }
  :global(.dark) .card { border-color: rgb(255 255 255 / 0.08); background: rgb(24 24 27); }
  .btn { padding: 0.4rem 0.75rem; border-radius: 0.5rem; border: 1px solid rgb(229 229 229); background: white; color: rgb(39 39 42); font-size: 0.875rem; }
  :global(.dark) .btn { border-color: rgb(255 255 255 / 0.1); background: rgb(39 39 42); color: rgb(244 244 245); }
  .btn-primary { padding: 0.45rem 0.95rem; border-radius: 0.6rem; background: rgb(236 72 153); color: white; font-size: 0.875rem; font-weight: 600; }
  .btn-primary:disabled { opacity: 0.5; }
  .input { padding: 0.45rem 0.65rem; border-radius: 0.6rem; border: 1px solid rgb(212 212 212); font-size: 0.875rem; background: white; }
  :global(.dark) .input { background: rgb(39 39 42); border-color: rgb(63 63 70); color: white; }
  .chip { display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.18rem 0.55rem; border-radius: 9999px; border: 1px solid rgb(229 229 229); background: rgb(250 250 250); font-size: 11px; font-weight: 500; }
  :global(.dark) .chip { border-color: rgb(255 255 255 / 0.08); background: rgb(39 39 42); }
</style>
