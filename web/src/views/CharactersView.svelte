<script>
  import { onMount } from 'svelte'
  import { searchCharacters, getCharacter } from '../lib/api.js'
  import { loadConstants, enumList } from '../lib/constants.js'
  import { fmtScore } from '../lib/format.js'
  import Pagination from '../components/Pagination.svelte'
  import Drawer from '../components/Drawer.svelte'

  let cons = $state(null)
  let roles = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)
  let selected = $state(null)
  let detail = $state(null)
  let detailLoading = $state(false)

  const cur = $derived(detail && !detail.error ? detail : selected)

  let f = $state({ q: '', role: '', page: 1, size: 30 })

  onMount(async () => {
    cons = await loadConstants()
    roles = enumList(cons.character_roles)
    await doSearch()
  })

  async function doSearch() {
    loading = true
    error = ''
    result = null
    try {
      result = await searchCharacters(f)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function submit() {
    f.page = 1
    doSearch()
  }

  function changePage(p) {
    f.page = p
    doSearch()
  }

  async function openDetail(item) {
    selected = item
    detail = null
    detailLoading = true
    try {
      detail = await getCharacter(item.id)
    } catch (e) {
      detail = { error: e.message }
    } finally {
      detailLoading = false
    }
  }

  function closeDetail() {
    selected = null
    detail = null
  }
</script>

<div class="grid gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-200 bg-white/60 p-4 lg:grid-cols-3 dark:border-neutral-800 dark:bg-neutral-900/60" onsubmit={(e) => { e.preventDefault(); submit() }}>
    <div>
      <label class="label" for="character-q">关键词</label>
      <input id="character-q" class="input" type="text" placeholder="如：五河士道" bind:value={f.q} />
    </div>
    <div>
      <label class="label" for="character-role">类型</label>
      <select id="character-role" class="input" bind:value={f.role}>
        <option value="">全部</option>
        {#each roles as r}
          <option value={r.id}>{r.name}</option>
        {/each}
      </select>
    </div>
    <div class="flex items-end gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '搜索中…' : '搜索'}</button>
      <span class="text-xs text-neutral-500">GET /api/characters/search</span>
    </div>
  </form>

  {#if error}
    <div class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-900 dark:bg-red-950/50 dark:text-red-400">请求失败：{error}</div>
  {/if}

  {#if loading}
    <div class="py-8 text-center text-sm text-neutral-500">加载中…</div>
  {:else if result}
    <div class="rounded-lg border border-neutral-200 bg-white/60 dark:border-neutral-800 dark:bg-neutral-900/60">
      <div class="border-b border-neutral-200 px-4 py-2 dark:border-neutral-800">
        <Pagination total={result.total} page={result.page} size={result.size} onchange={changePage} />
      </div>
      <div class="overflow-x-auto p-2">
        <table class="tbl">
          <thead>
            <tr>
              <th>ID</th>
              <th>名字</th>
              <th>中文名</th>
              <th>类型</th>
              <th>收藏</th>
              <th>评论</th>
            </tr>
          </thead>
          <tbody>
            {#each result.items as it (it.id)}
              <tr class="cursor-pointer hover:bg-neutral-100 dark:hover:bg-neutral-800/50" onclick={() => openDetail(it)}>
                <td class="text-neutral-500"><a href={`https://bgm.tv/character/${it.id}`} target="_blank" rel="noreferrer" class="hover:underline" onclick={(e) => e.stopPropagation()}>{it.id}</a></td>
                <td class="max-w-64 truncate"><a href={`https://bgm.tv/character/${it.id}`} target="_blank" rel="noreferrer" class="text-sky-600 hover:underline dark:text-sky-400" onclick={(e) => e.stopPropagation()}>{it.name}</a></td>
                <td class="max-w-52 truncate">{it.name_cn || '—'}</td>
                <td>{it.role_name}</td>
                <td class="text-neutral-500 dark:text-neutral-400">{it.collects}</td>
                <td class="text-neutral-500 dark:text-neutral-400">{it.comments}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if result.items.length === 0}
        <div class="py-8 text-center text-sm text-neutral-500">无结果</div>
      {/if}
    </div>
  {/if}
</div>

<Drawer open={!!selected} label="角色详情" onclose={closeDetail}>
  <div class="flex items-start gap-2 border-b border-neutral-200 px-4 py-3 dark:border-neutral-800">
    <h3 class="min-w-0 flex-1 truncate text-base font-semibold">
      <a href={`https://bgm.tv/character/${selected.id}`} target="_blank" rel="noreferrer" class="text-sky-600 hover:underline dark:text-sky-400">{cur.name_cn || cur.name}</a>
    </h3>
    <button class="btn-mini shrink-0" onclick={closeDetail}>关闭（Esc）</button>
  </div>

  <div class="flex flex-wrap items-center gap-1 border-b border-neutral-100 px-4 py-2 dark:border-neutral-900">
    {#if cur.role_name}<span class="chip">{cur.role_name}</span>{/if}
  </div>

  <div class="flex-1 overflow-y-auto px-4 py-3">
    {#if detailLoading}
      <div class="py-8 text-center text-sm text-neutral-500">加载详情…</div>
    {:else if detail?.error}
      <p class="text-sm text-red-600 dark:text-red-400">详情加载失败：{detail.error}</p>
    {:else}
      <section class="mb-4">
        <dl class="grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-3">
          <div><dt class="label">ID</dt><dd class="text-neutral-500">{cur.id}</dd></div>
          <div><dt class="label">收藏</dt><dd>{cur.collects}</dd></div>
          <div><dt class="label">评论</dt><dd>{cur.comments}</dd></div>
          <div><dt class="label">出演作品数</dt><dd>{detail ? (detail.subjects.length || '—') : '…'}</dd></div>
        </dl>
        <p class="mt-1.5 truncate text-sm text-neutral-500 dark:text-neutral-400" title={cur.name}>原名：{cur.name}</p>
      </section>

      <section class="mb-4">
        <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
        <p class="whitespace-pre-wrap text-sm leading-relaxed">{cur.summary || '（无简介）'}</p>
      </section>

      <section class="mb-4">
        <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">出演作品（{detail?.subjects.length ?? '…'}）</h4>
        <ul class="space-y-0.5 text-sm">
          {#each detail?.subjects ?? [] as s}
            <li class="text-neutral-700 dark:text-neutral-300">
              <span class="text-neutral-500">{s.date || '—'}</span>
              <a href={`https://bgm.tv/subject/${s.subject_id}`} target="_blank" rel="noreferrer" class="hover:underline">{s.name_cn || s.name}</a>
              {#if s.score > 0}<span class="text-amber-600 dark:text-amber-400">{fmtScore(s.score)}</span>{/if}
            </li>
          {/each}
        </ul>
      </section>

      <section>
        <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">声优 / 演员（{detail?.cvs.length ?? '…'}）</h4>
        <ul class="space-y-0.5 text-sm">
          {#each detail?.cvs ?? [] as c}
            <li class="text-neutral-700 dark:text-neutral-300">
              <a href={`https://bgm.tv/person/${c.person_id}`} target="_blank" rel="noreferrer" class="hover:underline">{c.name}</a>
              {#if c.type_name}<span class="chip">{c.type_name}</span>{/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>
</Drawer>