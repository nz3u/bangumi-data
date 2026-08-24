<script>
  import { onMount } from 'svelte'
  import { searchPersons, getPerson } from '../lib/api.js'
  import { loadConstants, enumList } from '../lib/constants.js'
  import { careerCn } from '../lib/format.js'
  import Pagination from '../components/Pagination.svelte'
  import Drawer from '../components/Drawer.svelte'

  let cons = $state(null)
  let types = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)
  let selected = $state(null)
  let detail = $state(null)
  let detailLoading = $state(false)

  const cur = $derived(detail && !detail.error ? detail : selected)

  let f = $state({ q: '', type: '', page: 1, size: 30 })

  onMount(async () => {
    cons = await loadConstants()
    types = enumList(cons.person_types)
    await doSearch()
  })

  async function doSearch() {
    loading = true
    error = ''
    result = null
    try {
      result = await searchPersons(f)
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
      detail = await getPerson(item.id)
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
      <label class="label" for="person-q">关键词</label>
      <input id="person-q" class="input" type="text" placeholder="如：宫崎骏" bind:value={f.q} />
    </div>
    <div>
      <label class="label" for="person-type">类型</label>
      <select id="person-type" class="input" bind:value={f.type}>
        <option value="">全部</option>
        {#each types as t}
          <option value={t.id}>{t.name}</option>
        {/each}
      </select>
    </div>
    <div class="flex items-end gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '搜索中…' : '搜索'}</button>
      <span class="text-xs text-neutral-500">GET /api/persons/search</span>
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
              <th>职业</th>
              <th>评论</th>
              <th>收藏</th>
            </tr>
          </thead>
          <tbody>
            {#each result.items as it (it.id)}
              <tr class="cursor-pointer hover:bg-neutral-100 dark:hover:bg-neutral-800/50" onclick={() => openDetail(it)}>
                <td class="text-neutral-500"><a href={`https://bgm.tv/person/${it.id}`} target="_blank" rel="noreferrer" class="hover:underline" onclick={(e) => e.stopPropagation()}>{it.id}</a></td>
                <td class="max-w-64 truncate"><a href={`https://bgm.tv/person/${it.id}`} target="_blank" rel="noreferrer" class="text-sky-600 hover:underline dark:text-sky-400" onclick={(e) => e.stopPropagation()}>{it.name}</a></td>
                <td class="max-w-52 truncate">{it.name_cn || '—'}</td>
                <td>{it.type_name}</td>
                <td>
                  <div class="flex flex-wrap gap-1">
                    {#each it.career ?? [] as c}
                      <span class="chip">{careerCn(c)}</span>
                    {/each}
                  </div>
                </td>
                <td class="text-neutral-500 dark:text-neutral-400">{it.comments}</td>
                <td class="text-neutral-500 dark:text-neutral-400">{it.collects}</td>
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

<Drawer open={!!selected} label="人物详情" onclose={closeDetail}>
  <div class="flex items-start gap-2 border-b border-neutral-200 px-4 py-3 dark:border-neutral-800">
    <h3 class="min-w-0 flex-1 truncate text-base font-semibold">
      <a href={`https://bgm.tv/person/${selected.id}`} target="_blank" rel="noreferrer" class="text-sky-600 hover:underline dark:text-sky-400">{cur.name_cn || cur.name}</a>
    </h3>
    <button class="btn-mini shrink-0" onclick={closeDetail}>关闭（Esc）</button>
  </div>

  <div class="flex flex-wrap items-center gap-1 border-b border-neutral-100 px-4 py-2 dark:border-neutral-900">
    <span class="chip">{cur.type_name}</span>
    {#each cur.career ?? [] as c}
      <span class="chip">{careerCn(c)}</span>
    {/each}
  </div>

  <div class="flex-1 overflow-y-auto px-4 py-3">
    {#if detailLoading}
      <div class="py-8 text-center text-sm text-neutral-500">加载详情…</div>
    {:else if detail?.error}
      <p class="text-sm text-red-600 dark:text-red-400">详情加载失败：{detail.error}</p>
    {:else}
      <section class="mb-4">
        <dl class="grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-5">
          <div><dt class="label">ID</dt><dd class="text-neutral-500">{cur.id}</dd></div>
          <div><dt class="label">参与作品</dt><dd>{detail ? (detail.works_count || '—') : '…'}</dd></div>
          <div><dt class="label">出演角色</dt><dd>{detail ? (detail.roles_count || '—') : '…'}</dd></div>
          <div><dt class="label">评论</dt><dd>{cur.comments}</dd></div>
          <div><dt class="label">收藏</dt><dd>{cur.collects}</dd></div>
        </dl>
        <p class="mt-1.5 truncate text-sm text-neutral-500 dark:text-neutral-400" title={cur.name}>原名：{cur.name}</p>
      </section>

      <section class="mb-4">
        <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
        <p class="whitespace-pre-wrap text-sm leading-relaxed">{cur.summary || '（无简介）'}</p>
      </section>

      <section>
        <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">关联人物 / 角色（{detail?.relations.length ?? '…'}）</h4>
        <ul class="space-y-0.5 text-sm">
          {#each detail?.relations ?? [] as r}
            <li class="text-neutral-700 dark:text-neutral-300">
              <span class="text-neutral-500">{r.relation_name}</span> →
              <a href={`https://bgm.tv/${r.person_type}/${r.related_person_id}`} target="_blank" rel="noreferrer" class="hover:underline">{r.related_name || r.related_person_id}</a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>
</Drawer>