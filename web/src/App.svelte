<script>
  import { onMount } from 'svelte'
import { Router, Route, navigate, listen } from 'svelte5-router'
import Tabs from './components/Tabs.svelte'
import ThemeToggle from './components/ThemeToggle.svelte'
import DetailDrawer from './components/DetailDrawer.svelte'
import DbBadge from './components/DbBadge.svelte'
import SettingsPanel from './components/SettingsPanel.svelte'
import LazyRoute from './components/LazyRoute.svelte'
  import { health, stats, dbInfo, openSystemStream } from './lib/api.js'
  import { adminConfig, getStoredToken } from './lib/admin.js'
  import GitHub from './components/GitHub.svelte';
  import UpdatingBanner from './components/UpdatingBanner.svelte';

  const tabs = [
    { key: 'collaborations', label: '人物合作', path: '/collaborations' },
    { key: 'pairworks', label: '双人合作', path: '/pairworks' },
    { key: 'singleworks', label: '单人作品', path: '/singleworks' },
    { key: 'subjects', label: '条目搜索', path: '/subjects' },
    { key: 'persons', label: '人物搜索', path: '/persons' },
    { key: 'characters', label: '角色搜索', path: '/characters' }
  ]
  const DEFAULT_PATH = '/collaborations'

  // 路径 -> 页面标题；根路径与未知路径均视为默认页
  const extraPaths = ['/setup', '/admin']
  const extraTitles = new Map([['/setup', '初始化/更新'], ['/admin', '配置']])
  const titles = new Map([...tabs.map((t) => [t.path, t.label]), ...extraTitles])
  const knownPaths = new Set(['/', ...tabs.map((t) => t.path), ...extraPaths])
  let currentPath = $state(typeof window !== 'undefined' ? (window.location.pathname.replace(/\/+$/, '') || '/') : '/')
  const routeIsAdmin = $derived(currentPath === '/setup' || currentPath === '/admin')
  function titleOf(pathname) {
    return titles.get(pathname) ?? titles.get(DEFAULT_PATH)
  }

  let svc = $state(null)
  let st = $state(null)
  let dbVer = $state(null)
  let svcError = $state('')
  let lastUpdate = $state(null)
  let hasToken = $state(false)
  let autoEnabled = $state(null)

  function getQueryToken() {
    try { return new URL(window.location.href).searchParams.get('token') || '' } catch { return '' }
  }
  async function refreshAuthState() {
    try {
      const t = getStoredToken() || getQueryToken()
      hasToken = !!t
      if (!hasToken) { autoEnabled = null; return }
      const cfg = await adminConfig()
      autoEnabled = !!cfg.auto_update?.enabled
    } catch {
      // 未鉴权或网络错误时保持 hasToken 但 autoEnabled 未知
      if (!hasToken) autoEnabled = null
    }
  }

  async function refreshStats() {
    try {
      st = await stats()
    } catch {
      st = null
    }
  }

  async function refreshDbInfo() {
    try {
      dbVer = await dbInfo()
    } catch {
      dbVer = null
    }
  }

  let systemEsClose = null
  function startSystemStream() {
    if (systemEsClose) systemEsClose()
    systemEsClose = openSystemStream((data) => {
      if (data.health) {
        svc = data.health
        svcError = data.health.status === 'ok' ? '' : (data.health.error || '异常')
        lastUpdate = new Date()
      }
      if (data.stats && !data.stats.error) {
        st = data.stats
      }
    }, () => {
      // SSE 出错回退轮询
      health().then(d=>{ svc=d; svcError=''; lastUpdate=new Date() }).catch(e=>{ svc=null; svcError=e.message })
      refreshStats()
    })
  }

  // 监听 token 变化以控制更新横幅显隐
  $effect(() => {
    void currentPath
    refreshAuthState()
  })

  onMount(() => {
    startSystemStream()
    refreshAuthState()
    // 监听 storage 变化（/setup 设置 token 后回退到首页需刷新横幅）
    const onStorage = () => refreshAuthState()
    window.addEventListener('storage', onStorage)
    // 路由副作用：同步页面标题；未知路径（含尾斜杠等）替换为默认页。
    const offRoute = listen(({ location }) => {
      const p = location.pathname
      const normalized = p.length > 1 ? p.replace(/\/+$/, '') || '/' : p
      currentPath = normalized
      if (!knownPaths.has(normalized)) {
        navigate(DEFAULT_PATH, { replace: true })
        return
      }
      document.title = `${titleOf(normalized)} · Bangumi 本地数据搜索`
    })
    // 初始化 currentPath（处理直接刷新进入 /setup 等场景）
    currentPath = (window.location.pathname.replace(/\/+$/, '') || '/')

    // 数据库版本状态只在进入页面时请求一次（后端缓存检查结果，无需轮询）
    refreshDbInfo()

    return () => {
      if (systemEsClose) systemEsClose()
      window.removeEventListener('storage', onStorage)
      offRoute()
    }
  })

  const statLabels = {
    subjects: '条目',
    persons: '人物',
    characters: '角色',
    episodes: '章节'
  }

  // 品牌看板娘姿势：每次页面加载随机抽 musume_1..6 之一，内部切换页面不重抽
  const musumeN = 1 + Math.floor(Math.random() * 6)

  // 统计数字压缩显示（移动端）：≥1 万取整用「w」单位（681187 → 68w），完整值见 title 提示
  const fmtWan = (n) => (n >= 10000 ? Math.round(n / 10000) + 'w' : String(n))
</script>

<UpdatingBanner />
{#if dbVer && !dbVer.database}
  <div class="border-b border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-950/40 dark:text-amber-200">
    <div class="mx-auto flex max-w-6xl items-center gap-2">
      <span>数据库尚未初始化（首次运行），请前往 <a href="/setup" onclick={(e)=>{e.preventDefault(); navigate('/setup')}} class="underline font-medium">初始化页面</a> 完成导入。</span>
      <a href="/setup" onclick={(e)=>{e.preventDefault(); navigate('/setup')}} class="ml-auto rounded bg-amber-500 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-600">去初始化 →</a>
    </div>
  </div>
{:else if dbVer?.update_available && dbVer?.latest && hasToken}
  <div class="border-b border-amber-200 bg-amber-50/70 px-4 py-1.5 text-xs text-amber-700 dark:border-amber-500/20 dark:bg-amber-950/20 dark:text-amber-300">
    <div class="mx-auto flex max-w-6xl items-center gap-2">
      <span>检测到新数据版本 {dbVer.latest.version} 可更新，</span>
      <a href="/setup" onclick={(e)=>{e.preventDefault(); navigate('/setup')}} class="underline font-medium">去更新 →</a>
      {#if autoEnabled === true}
        <span class="hidden sm:inline opacity-70">（每周三 05:30 (UTC+8) 自动更新已启用）</span>
      {:else if autoEnabled === false}
        <span class="hidden sm:inline opacity-70">（每周三 05:30 (UTC+8) 自动更新未启用）</span>
      {:else}
        <span class="hidden sm:inline opacity-70">（每周三 05:30 (UTC+8) 自动更新）</span>
      {/if}
    </div>
  </div>
{/if}
<!-- 全宽粘性顶栏：横跨整个页面宽度，内部内容与下方主体对齐。
     注意：不要给 header 加 backdrop-filter——它会让包含其内的设置下拉面板
     进入特殊合成上下文，Firefox(WebRender) 首次弹出时可能失效重绘
     （面板被下方内容“遮盖”，切页/聚焦后才恢复）；用更高不透明度替代毛玻璃。 -->
<header class="sticky top-0 z-40 border-b border-neutral-200/60 bg-white/90 dark:border-white/[0.06] dark:bg-[#0c0c10]/90">
  <!-- 桌面端顶栏行（md+）：品牌 → 状态 → 统计 → 右侧按钮，顺序由用户排定 -->
   <div class="mx-auto hidden max-w-6xl flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2.5 md:flex">
      <a href="/" onclick={(e)=>{e.preventDefault(); navigate('/')}} class="flex min-w-0 items-center gap-2.5 no-underline hover:opacity-90" title="返回首页">
        <!-- 品牌看板娘：bgm.tv rc3 雪碧图，加载时随机 musume_1..6（背景位移 -48px 步进），
             -mb-2.5 抵消顶栏底部内边距，使底边贴住底框而不撑高顶栏 -->
        <span
          class="brand-musume shrink-0 -mb-2.5"
          style:background-position={`-${musumeN * 48}px 0`}
          aria-hidden="true"
        ></span>
        <!-- data-darkreader-ignore：Dark Reader 会剥掉渐变裁切背景导致透明字隐身，豁免后由 .brand-title 自带浅/深渐变配色 -->
        <h1
          class="brand-title min-w-0 truncate text-base font-bold tracking-tight sm:text-lg"
          data-darkreader-ignore
        >
          Bangumi 本地数据搜索
        </h1>
      </a>
      {#if svc?.version}
        <span class="chip hidden font-mono sm:inline-flex" title="后端编译期注入的版本号（与发布标签一致）">v{svc.version}</span>
      {/if}

      {@render statusChip()}

      {@render statsRow('hidden flex-wrap items-center gap-1.5 md:flex')}

      {@render actions()}
    </div>

  <!-- 移动端顶栏（<md）：看板娘左侧通栏、底边贴住底框；
       右列上行=标题+按钮，下行=呼吸灯(仅时间)+压缩数字，可横向滑动 -->
  <div class="mx-auto flex max-w-6xl items-stretch gap-2.5 px-4 pt-2.5 md:hidden">
    <a href="/" onclick={(e)=>{e.preventDefault(); navigate('/')}} class="flex shrink-0 no-underline" title="返回首页">
      <span
        class="brand-musume shrink-0"
        style:background-position={`-${musumeN * 48}px 0`}
        aria-hidden="true"
      ></span>
    </a>
    <div class="flex min-w-0 flex-1 flex-col justify-between gap-1">
      <div class="flex min-w-0 items-center gap-2">
        <a href="/" onclick={(e)=>{e.preventDefault(); navigate('/')}} class="min-w-0 no-underline">
          <h1
            class="brand-title min-w-0 truncate text-base font-bold tracking-tight"
            data-darkreader-ignore
          >
            Bangumi 本地数据搜索
          </h1>
        </a>
        {@render actions()}
      </div>
      <div class="no-scrollbar flex items-center gap-1.5 overflow-x-auto pb-0.5">
        {@render statusChip(true)}
        {@render statsRow('flex shrink-0 items-center gap-1.5', true)}
      </div>
    </div>
  </div>
</header>

<div class="mx-auto max-w-6xl px-4 pb-10 pt-4">
  <Router>
    {#if !routeIsAdmin}
      <Tabs items={tabs} />
    {/if}

    <main class="mt-4">
      <!-- 视图按需加载：children snippet 仅在路由激活时渲染，LazyRoute 此时才触发对应 chunk 的导入 -->
      <Route path="/"><LazyRoute path="/collaborations" /></Route>
      <Route path="/collaborations"><LazyRoute path="/collaborations" /></Route>
      <Route path="/pairworks"><LazyRoute path="/pairworks" /></Route>
      <Route path="/singleworks"><LazyRoute path="/singleworks" /></Route>
      <Route path="/subjects"><LazyRoute path="/subjects" /></Route>
      <Route path="/persons"><LazyRoute path="/persons" /></Route>
      <Route path="/characters"><LazyRoute path="/characters" /></Route>
      <Route path="/setup"><LazyRoute path="/setup" /></Route>
      <Route path="/admin"><LazyRoute path="/admin" /></Route>
    </main>
  </Router>

  <!-- 全局唯一详情抽屉：由内部状态驱动（不写入地址栏），任何页面内打开均复用此实例 -->
  <DetailDrawer />

  <footer class="mt-10 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-neutral-200/70 pt-4 text-xs text-neutral-500 dark:border-white/[0.06]">
    <span>
      接口文档见项目 README（REST API 一节）；开发模式：<code>cd web && npm run dev</code>（代理 /api 到 :8080）。
    </span>
    <!-- 数据库版本标注：右对齐，落后时琥珀色提醒 -->
    <DbBadge info={dbVer} />
  </footer>
</div>

{#snippet actions()}
  <div class="ml-auto flex shrink-0 items-center gap-1.5">
    <SettingsPanel />
    <GitHub />
    <ThemeToggle />
  </div>
{/snippet}

{#snippet statusChip(compact = false)}
  {#if svc}
    <span class="chip shrink-0 text-emerald-600 dark:text-emerald-400">
      <span class="size-1.5 animate-pulse rounded-full bg-current"></span>
      {#if compact}
        {lastUpdate ? lastUpdate.toLocaleTimeString('zh-CN', { hour12: false }) : '检测中…'}
      {:else}
        服务正常{#if lastUpdate} · {lastUpdate.toLocaleTimeString('zh-CN', { hour12: false })}{/if}
      {/if}
    </span>
  {:else if svcError}
    <span class="chip shrink-0 text-red-600 dark:text-red-400">服务异常：{svcError}</span>
  {/if}
{/snippet}

{#snippet statsRow(cls, compact = false)}
  {#if st}
    <div class={cls}>
      {#each Object.entries(statLabels) as [key, label]}
        <span class="chip" title={compact ? `${st[key]} ${label}` : undefined}>
          <span class="font-mono tabular-nums">{compact ? fmtWan(st[key]) : st[key]}</span>{label}
        </span>
      {/each}
    </div>
  {/if}
{/snippet}
