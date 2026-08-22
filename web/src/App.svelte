<script>
  import { onMount } from 'svelte'
  import Tabs from './components/Tabs.svelte'
  import ThemeToggle from './components/ThemeToggle.svelte'
  import SubjectsView from './views/SubjectsView.svelte'
  import PersonsView from './views/PersonsView.svelte'
  import CharactersView from './views/CharactersView.svelte'
  import CollaborationsView from './views/CollaborationsView.svelte'
  import PairWorksView from './views/PairWorksView.svelte'
  import { health, stats } from './lib/api.js'

  const tabs = [
    { key: 'subjects', label: '条目搜索' },
    { key: 'persons', label: '人物搜索' },
    { key: 'characters', label: '角色搜索' },
    { key: 'collaborations', label: '人物合作' },
    { key: 'pairworks', label: '双人合作' }
  ]

  let active = $state('subjects')
  let svc = $state(null)
  let st = $state(null)
  let svcError = $state('')

  onMount(async () => {
    try {
      svc = await health()
      st = await stats()
    } catch (e) {
      svcError = e.message
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
    {#if svc}
      <span class="chip text-emerald-600 dark:text-emerald-400">服务正常</span>
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
    <ThemeToggle />
  </header>

  <Tabs items={tabs} active={active} onchange={(k) => (active = k)} />

  <main class="mt-4">
    {#if active === 'subjects'}
      <SubjectsView />
    {:else if active === 'persons'}
      <PersonsView />
    {:else if active === 'characters'}
      <CharactersView />
    {:else if active === 'collaborations'}
      <CollaborationsView />
    {:else}
      <PairWorksView />
    {/if}
  </main>

  <footer class="mt-8 border-t border-neutral-200 pt-3 text-xs text-neutral-500 dark:border-neutral-800">
    接口文档见项目 README（REST API 一节）；开发模式：<code>cd web && npm run dev</code>（代理 /api 到 :8080）。
  </footer>
</div>