<script>
  import { onMount } from 'svelte'
  import { searchCharacters } from '../lib/api.js'
  import { loadConstants, enumList, AUTO_SEARCH_DEBOUNCE_MS } from '../lib/constants.js'
import Pagination from '../components/Pagination.svelte'
import Highlight from '../components/Highlight.svelte'
import { openDetail } from '../lib/detail.svelte.js'
import { externalUrl } from '../lib/settings.svelte.js'

  let cons = $state(null)
  let roles = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)

  let f = $state({ q: '', role: '' })

  onMount(async () => {
    cons = await loadConstants()
    roles = enumList(cons.character_roles)
    await doSearch({ q: f.q, role: f.role, page: 1, size: 30 }) // 挂载即展示第 1 页
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
      result = await searchCharacters(p)
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
    doSearch({ q: f.q, role: f.role, page: 1, size: 30 })
  }

  function changePage(p) {
    if (!result) return
    doSearch({ q: f.q, role: f.role, page: p, size: 30 })
  }
</script>

<div class="rise grid grid-cols-[minmax(0,1fr)] gap-4">
  <form class="grid gap-3 card p-4 lg:grid-cols-3" onsubmit={(e) => { e.preventDefault(); submit() }}>
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
    <div class="card p-4">
      <div class="skeleton mb-3 h-4 w-40"></div>
      <div class="space-y-2.5">
        {#each Array.from({ length: 8 }) as _, i}
          <div class="skeleton h-5" style:width="{96 - (i % 4) * 9}%"></div>
        {/each}
      </div>
    </div>
  {:else if result}
    <div class="card">
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
          <tbody class="stagger">
            {#each result.items as it (it.id)}
              <tr class="cursor-pointer transition-colors hover:bg-sakura-50/70 dark:hover:bg-white/[0.04]" onclick={() => openDetail('character', it.id, it)}>
                <td class="text-neutral-500"><a href={externalUrl('character', it.id)} target="_blank" rel="noreferrer" class="hover:underline" onclick={(e) => e.stopPropagation()}>{it.id}</a></td>
                <td class="max-w-64 truncate"><a href={externalUrl('character', it.id)} target="_blank" rel="noreferrer" class="text-sakura-600 hover:underline dark:text-sakura-400" onclick={(e) => e.stopPropagation()}><Highlight text={it.name} q={f.q} scope="suggest" /></a></td>
                <td class="max-w-52 truncate"><Highlight text={it.name_cn} q={f.q} scope="suggest" />{#if !it.name_cn}—{/if}</td>
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
