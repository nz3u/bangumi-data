<script>
  import { onMount } from 'svelte'
  import { searchPersons } from '../lib/api.js'
  import { loadConstants, enumList, AUTO_SEARCH_DEBOUNCE_MS } from '../lib/constants.js'
  import { careerCn } from '../lib/format.js'
  import { onNavParams } from '../lib/nav.js'
  import Pagination from '../components/Pagination.svelte'
  import { openDetail } from '../lib/detail.svelte.js'

  const BASE = '/persons'

  let cons = $state(null)
  let types = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)

  let f = $state({ q: '', type: '' })

  // 首次搜索是否已完成：完成前收到跨页传参只回填表单（由下方首次搜索消费），
  // 完成后收到则立即重新搜索。
  let ready = false

  onMount(async () => {
    // 跨标签页内部传参：搜索建议的「发现重名人物」提示等场景跳转过来时携带关键词。
    onNavParams(BASE, (params) => {
      const q = String(params?.q ?? '').trim()
      if (!q) return
      f.q = q
      appliedFormSig = formSig() // 同步快照，避免自动搜索重复触发
      if (ready) submit()
    })
    cons = await loadConstants()
    types = enumList(cons.person_types)
    await doSearch({ q: f.q, type: f.type, page: 1, size: 30 }) // 挂载即展示第 1 页
    ready = true
  })

  // 搜索由表单状态直接驱动：提交/翻页时按当前表单发起请求。
  let autoTimer = null
  const formSig = () => JSON.stringify(f)
  let appliedFormSig = formSig()

  // 自动搜索（无搜索建议）：表单相对最近一次已执行搜索的快照有任何变更时，
  // 停顿 AUTO_SEARCH_DEBOUNCE_MS 后自动提交。手动提交会先更新快照，故不会误触发；
  // 输入回退到与快照一致则取消。
  $effect(() => {
    const sig = formSig()
    if (sig === appliedFormSig) return
    clearTimeout(autoTimer)
    autoTimer = setTimeout(() => {
      autoTimer = null
      submit()
    }, AUTO_SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(autoTimer)
  })

  async function doSearch(p) {
    loading = true
    error = ''
    result = null
    try {
      result = await searchPersons(p)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function submit() {
    clearTimeout(autoTimer)
    autoTimer = null
    appliedFormSig = formSig()
    doSearch({ q: f.q, type: f.type, page: 1, size: 30 })
  }

  function changePage(p) {
    if (!result) return
    doSearch({ q: f.q, type: f.type, page: p, size: 30 })
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
              <tr class="cursor-pointer hover:bg-neutral-100 dark:hover:bg-neutral-800/50" onclick={() => openDetail('person', it.id, it)}>
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
