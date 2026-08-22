<script>
  import { onMount } from 'svelte'
  import { searchCharacters } from '../lib/api.js'
  import { loadConstants, enumList } from '../lib/constants.js'
  import Pagination from '../components/Pagination.svelte'

  let cons = $state(null)
  let roles = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)

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
              <th>类型</th>
              <th>收藏</th>
              <th>评论</th>
            </tr>
          </thead>
          <tbody>
            {#each result.items as it (it.id)}
              <tr>
                <td class="text-neutral-500">{it.id}</td>
                <td class="max-w-64 truncate">{it.name}</td>
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