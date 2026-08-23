<script>
  import { onMount } from 'svelte'
  import { searchPersons } from '../lib/api.js'
  import { loadConstants, enumList } from '../lib/constants.js'
  import { careerCn } from '../lib/format.js'
  import Pagination from '../components/Pagination.svelte'

  let cons = $state(null)
  let types = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)

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
              <tr>
                <td class="text-neutral-500">{it.id}</td>
                <td class="max-w-64 truncate">{it.name}</td>
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