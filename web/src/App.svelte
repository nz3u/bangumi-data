<script>
  import { onMount } from 'svelte'
  import { Router, Route, navigate, listen } from 'svelte5-router'
  import Tabs from './components/Tabs.svelte'
  import ThemeToggle from './components/ThemeToggle.svelte'
  import SubjectsView from './views/SubjectsView.svelte'
  import PersonsView from './views/PersonsView.svelte'
  import CharactersView from './views/CharactersView.svelte'
  import CollaborationsView from './views/CollaborationsView.svelte'
  import PairWorksView from './views/PairWorksView.svelte'
  import SingleWorksView from './views/SingleWorksView.svelte'
  import DetailDrawer from './components/DetailDrawer.svelte'
  import DbBadge from './components/DbBadge.svelte'
  import SettingsPanel from './components/SettingsPanel.svelte'
  import { health, stats, dbInfo } from './lib/api.js'
  import GitHub from './components/GitHub.svelte';

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
  const titles = new Map(tabs.map((t) => [t.path, t.label]))
  const knownPaths = new Set(['/', ...tabs.map((t) => t.path)])
  function titleOf(pathname) {
    return titles.get(pathname) ?? titles.get(DEFAULT_PATH)
  }

  let svc = $state(null)
  let st = $state(null)
  let dbVer = $state(null)
  let svcError = $state('')
  let lastUpdate = $state(null)

  const POLL_MS = 60000

  async function refreshHealth() {
    try {
      svc = await health()
      svcError = ''
    } catch (e) {
      svc = null
      svcError = e.message
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

  let inflight = false
  async function pollOnce() {
    if (document.hidden || inflight) return
    inflight = true
    try {
      await Promise.all([refreshHealth(), refreshStats()])
      lastUpdate = new Date()
    } finally {
      inflight = false
    }
  }

  onMount(() => {
    const timer = setInterval(pollOnce, POLL_MS)

    const onVisibility = () => {
      if (!document.hidden) pollOnce()
    }
    document.addEventListener('visibilitychange', onVisibility)

    // 路由副作用：同步页面标题；未知路径（含尾斜杠等）替换为默认页。
    const offRoute = listen(({ location }) => {
      const p = location.pathname
      const normalized = p.length > 1 ? p.replace(/\/+$/, '') || '/' : p
      if (!knownPaths.has(normalized)) {
        navigate(DEFAULT_PATH, { replace: true })
        return
      }
      document.title = `${titleOf(normalized)} · Bangumi 本地数据搜索`
    })

    // 数据库版本状态只在进入页面时请求一次（后端缓存检查结果，无需轮询）
    refreshDbInfo()
    pollOnce()

    return () => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisibility)
      offRoute()
    }
  })

  const statLabels = {
    subjects: '条目',
    persons: '人物',
    characters: '角色',
    episodes: '章节'
  }
</script>

<div class="mx-auto max-w-6xl px-4 py-6">
  <header class="mb-4 flex flex-wrap items-center gap-4">
    <h1 class="text-xl font-bold">Bangumi 本地数据搜索</h1>
    {#if svc?.version}
      <span class="text-xs tabular-nums text-neutral-400" title="后端编译期注入的版本号（与发布标签一致）">v{svc.version}</span>
    {/if}
    {#if svc}
      <span class="chip text-emerald-600 dark:text-emerald-400">
        服务正常{#if lastUpdate} · {lastUpdate.toLocaleTimeString('zh-CN', { hour12: false })}{/if}
      </span>
    {:else if svcError}
      <span class="chip text-red-600 dark:text-red-400">服务异常：{svcError}</span>
    {/if}
    {#if st}
      <div class="flex flex-wrap gap-1">
        {#each Object.entries(statLabels) as [key, label]}
          <span class="chip">{label} {st[key]}</span>
        {/each}
      </div>
    {/if}
    <span class="ml-auto text-xs text-neutral-500">本地 SQLite 数据 · FTS5 检索</span>
    <SettingsPanel />
    <GitHub />
    <ThemeToggle />
  </header>

  <Router>
    <Tabs items={tabs} />

    <main class="mt-4">
      <Route path="/" component={CollaborationsView} />
      <Route path="/collaborations" component={CollaborationsView} />
      <Route path="/pairworks" component={PairWorksView} />
      <Route path="/singleworks" component={SingleWorksView} />
      <Route path="/subjects" component={SubjectsView} />
      <Route path="/persons" component={PersonsView} />
      <Route path="/characters" component={CharactersView} />
    </main>
  </Router>

  <!-- 全局唯一详情抽屉：由内部状态驱动（不写入地址栏），任何页面内打开均复用此实例 -->
  <DetailDrawer />

  <footer class="mt-8 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-neutral-200 pt-3 text-xs text-neutral-500 dark:border-neutral-800">
    <span>
      接口文档见项目 README（REST API 一节）；开发模式：<code>cd web && npm run dev</code>（代理 /api 到 :8080）。
    </span>
    <!-- 数据库版本标注：右对齐，落后时琥珀色提醒 -->
    <DbBadge info={dbVer} />
  </footer>
</div>
