<script>
  import { onMount } from 'svelte'
  import { link } from 'svelte5-router'
  import { adminConfig, adminSaveConfig, getStoredToken, setStoredToken } from '../lib/admin.js'

  let cfg = $state(null)
  let error = $state('')
  let saving = $state(false)
  let saved = $state(false)
  let tokenInput = $state('')
  let showToken = $state(false)

  let checking = $state(true)
  let needsAuth = $state(false)

  let form = $state({
    bgm_api_key: '',
    admin_token: '',
    auto_enabled: false,
    auto_threads: 0,
    auto_keep: false,
    listen: ''
  })

  function getQueryToken() {
    try { return new URL(window.location.href).searchParams.get('token') || '' } catch { return '' }
  }

  async function load() {
    checking = true
    try {
      cfg = await adminConfig()
      form.bgm_api_key = cfg.bgm_api_key ?? ''
      form.admin_token = cfg.admin_token ?? ''
      form.auto_enabled = cfg.auto_update?.enabled ?? false
      form.auto_threads = cfg.auto_update?.threads ?? 0
      form.auto_keep = cfg.auto_update?.keep_zip ?? false
      form.listen = cfg.server?.listen ?? ''
      needsAuth = false
      error = ''
      const qt = getQueryToken()
      if (qt) setStoredToken(qt)
    } catch (e) {
      if (e.status === 401) {
        needsAuth = true
        error = ''
      } else {
        error = e.message
      }
    } finally {
      checking = false
    }
  }

  async function save() {
    saving = true
    error = ''
    saved = false
    try {
      await adminSaveConfig({
        bgm_api_key: form.bgm_api_key,
        admin_token: form.admin_token,
        auto_update: { enabled: !!form.auto_enabled, threads: Number(form.auto_threads) || 0, keep_zip: !!form.auto_keep },
        server: { listen: form.listen }
      })
      if (form.admin_token && form.admin_token !== getStoredToken()) {
        setStoredToken(form.admin_token)
        tokenInput = form.admin_token
      }
      saved = true
      setTimeout(() => saved = false, 2000)
      await load()
    } catch (e) {
      if (e.status === 401) needsAuth = true
      error = e.message
    } finally {
      saving = false
    }
  }

  function handleAuth() {
    const v = tokenInput.trim()
    if (!v) { error = '请输入 token'; return }
    setStoredToken(v)
    try {
      const url = new URL(window.location.href)
      if (url.searchParams.has('token')) { url.searchParams.delete('token'); history.replaceState(null,'',url.toString()) }
    } catch {}
    load()
  }

  function clearToken() {
    setStoredToken('')
    tokenInput = ''
    needsAuth = true
  }

  onMount(() => {
    const qt = getQueryToken()
    tokenInput = qt || getStoredToken()
    if (qt) setStoredToken(qt)
    load()
  })
</script>

{#if checking}
  <div class="flex justify-center py-16"><div class="text-sm text-neutral-500">校验中…</div></div>
{:else if needsAuth}
  <div class="mx-auto flex min-h-[50vh] max-w-md items-center justify-center px-4">
    <div class="w-full rounded-2xl border border-neutral-200 bg-white p-6 shadow-sm dark:border-white/10 dark:bg-neutral-900">
      <div class="flex items-center gap-2.5">
        <span class="inline-flex size-8 items-center justify-center rounded-xl bg-neutral-900 text-white dark:bg-white dark:text-neutral-900">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg>
        </span>
        <h2 class="text-base font-semibold">需要验证</h2>
        <span class="ml-auto rounded-full bg-neutral-100 px-2 py-0.5 text-xs dark:bg-white/10">/admin</span>
      </div>
      <p class="mt-3 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
        访问 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">/admin</code> 需要 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">admin_token</code> 鉴权。请通过 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">bangumi config get admin_token</code> 获取，或查看启动日志。
      </p>
      <div class="mt-4 space-y-2">
        <div class="relative">
          <input class="input w-full pr-10" placeholder="粘贴 admin_token" bind:value={tokenInput} type={showToken ? 'text' : 'password'} onkeydown={(e)=> e.key==='Enter' && handleAuth()} />
          <button type="button" class="absolute inset-y-0 right-0 flex items-center pr-3 text-neutral-400 hover:text-neutral-600 dark:text-neutral-500 dark:hover:text-neutral-300" onclick={() => showToken = !showToken} aria-label={showToken ? '隐藏' : '显示'} tabindex="-1">
            {#if showToken}
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-4"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
            {:else}
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-4"><path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.94 10.94 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.56 13.56 0 0 0 2 12s3 7 10 7a9.59 9.59 0 0 0 5.39-1.61"/><line x1="2" x2="22" y1="2" y2="22"/></svg>
            {/if}
          </button>
        </div>
        <button class="btn-primary w-full" onclick={handleAuth}>验证并进入</button>
        {#if error}<p class="text-xs text-red-600 dark:text-red-400">{error}</p>{/if}
      </div>
    </div>
  </div>
{:else}
  <div class="space-y-4 min-w-0">
    <div class="card p-5 min-w-0 overflow-hidden">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-lg font-semibold flex items-center gap-2">
            <span class="inline-flex size-7 items-center justify-center rounded-lg bg-neutral-900 text-white dark:bg-white dark:text-neutral-900">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
            </span>
            配置
          </h2>
          <p class="mt-1.5 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
            所有配置实时写入 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">data/config.json</code>，与命令行 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">bangumi config</code> 互通。修改 <code class="rounded bg-neutral-100 px-1 py-0.5 text-xs dark:bg-white/10">admin_token</code> 后需使用新 token 重新验证。
          </p>
        </div>
        <a href="/setup" use:link class="inline-flex shrink-0 rounded-lg border border-neutral-200 px-3 py-1.5 text-xs hover:bg-neutral-50 dark:border-white/10 dark:hover:bg-white/5">前往初始化 →</a>
      </div>

      <div class="mt-5 space-y-5 min-w-0">
        <div class="grid gap-4 sm:grid-cols-2 min-w-0">
          <label class="block">
            <span class="label">BGM API Key</span>
            <input class="input w-full" bind:value={form.bgm_api_key} placeholder="留空使用环境变量 BANGUMI_API_KEY" />
            <span class="hint">用于 next.bgm.tv 人物头像抓取</span>
          </label>
          <label class="block">
            <span class="label">Admin Token</span>
            <div class="relative">
              <input class="input w-full pr-10" bind:value={form.admin_token} placeholder="管理鉴权，访问 /setup 与 /admin 需携带" type={showToken ? 'text' : 'password'} />
              <button type="button" class="absolute inset-y-0 right-0 flex items-center pr-3 text-neutral-400 hover:text-neutral-600 dark:text-neutral-500 dark:hover:text-neutral-300" onclick={() => showToken = !showToken} aria-label={showToken ? '隐藏' : '显示'} tabindex="-1">
                {#if showToken}
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-4"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                {:else}
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-4"><path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.94 10.94 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.56 13.56 0 0 0 2 12s3 7 10 7a9.59 9.59 0 0 0 5.39-1.61"/><line x1="2" x2="22" y1="2" y2="22"/></svg>
                {/if}
              </button>
            </div>
            <span class="hint">点击眼睛图标显示/隐藏，保存后本地 token 将自动更新</span>
          </label>
        </div>

        <fieldset class="rounded-xl border border-neutral-200 bg-neutral-50/60 p-4 dark:border-white/10 dark:bg-white/[0.03]">
          <legend class="px-2 text-sm font-medium">自动更新</legend>
          <label class="flex items-center gap-2 text-sm font-medium">
            <input type="checkbox" class="rounded" bind:checked={form.auto_enabled} />
            启用每周三 05:30 (UTC+8) 自动更新
            <span class="text-xs font-normal text-neutral-500">（需 serve 常驻）</span>
          </label>
          <div class="mt-3 grid gap-4 sm:grid-cols-2 min-w-0">
            <label class="block">
              <span class="label">并发线程数</span>
              <input class="input w-full" type="number" min="0" max="32" bind:value={form.auto_threads} />
              <span class="hint">0 为默认 8</span>
            </label>
            <label class="flex items-center gap-2 text-sm mt-7">
              <input type="checkbox" class="rounded" bind:checked={form.auto_keep} /> 保留下载 zip
            </label>
          </div>
          <p class="mt-2 text-xs leading-relaxed text-neutral-500">下载 → 导入临时库 → 完整性检查 → 原子切库 → 自动恢复，失败不影响旧库。</p>
        </fieldset>

        <label class="block">
          <span class="label">监听地址</span>
          <input class="input w-full" bind:value={form.listen} placeholder=":8080（留空使用默认值或环境变量）" />
          <span class="hint">优先级：命令行 &gt; 环境变量 &gt; 此处配置 &gt; :8080</span>
        </label>

        {#if cfg?.database}
          <div class="rounded-xl border border-neutral-200 bg-neutral-50 p-3 text-xs dark:border-white/10 dark:bg-white/[0.03]">
            <div class="font-medium">数据库</div>
            <div class="mt-1 space-y-0.5 text-neutral-600 dark:text-neutral-400">
              <div>版本：{cfg.database.version ?? '无记录'}</div>
              {#if cfg.database.published_at}<div>导出：{cfg.database.published_at}</div>{/if}
              {#if cfg.database.imported_at}<div>导入：{cfg.database.imported_at}</div>{/if}
            </div>
          </div>
        {/if}

        {#if error && !needsAuth}
          <div class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/20 dark:bg-red-950/30 dark:text-red-300">{error}</div>
        {/if}

        <div class="flex flex-wrap gap-2">
          <button class="btn-primary" disabled={saving} onclick={save}>{saving ? '保存中…' : '保存配置'}</button>
          <button class="btn" onclick={clearToken}>清除本地 token</button>
          {#if saved}<span class="self-center text-sm font-medium text-emerald-600">已保存</span>{/if}
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .card { border: 1px solid rgb(229 229 229 / 0.8); border-radius: 0.9rem; background: white; }
  :global(.dark) .card { border-color: rgb(255 255 255 / 0.08); background: rgb(24 24 27); }
  .label { display: block; font-size: 12px; font-weight: 500; color: rgb(82 82 91); margin-bottom: 0.35rem; }
  :global(.dark) .label { color: rgb(161 161 170); }
  .hint { display: block; font-size: 11px; color: rgb(161 161 170); margin-top: 0.3rem; }
  .btn { padding: 0.45rem 0.75rem; border-radius: 0.6rem; border: 1px solid rgb(229 229 229); background: white; color: rgb(39 39 42); font-size: 0.875rem; }
  :global(.dark) .btn { border-color: rgb(255 255 255 / 0.1); background: rgb(39 39 42); color: rgb(244 244 245); }
  .btn-primary { padding: 0.45rem 0.95rem; border-radius: 0.6rem; background: rgb(236 72 153); color: white; font-size: 0.875rem; font-weight: 600; }
  .btn-primary:disabled { opacity: 0.5; }
  .input { padding: 0.45rem 0.65rem; border-radius: 0.6rem; border: 1px solid rgb(212 212 212); font-size: 0.875rem; background: white; }
  :global(.dark) .input { background: rgb(39 39 42); border-color: rgb(63 63 70); color: white; }
</style>
