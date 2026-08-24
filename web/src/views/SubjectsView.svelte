<script>
  import { onMount } from 'svelte'
  import { useLocation } from 'svelte5-router'
  import { searchSubjects } from '../lib/api.js'
  import { loadConstants, platformsFor, enumList } from '../lib/constants.js'
  import { fmtScore, fmtRank, fmtDate, fmtFavorite } from '../lib/format.js'
  import Pagination from '../components/Pagination.svelte'
  import { openDetail } from '../lib/detail.svelte.js'
  import { parseQuery, pushSearch, intParam, AUTO_SEARCH_DEBOUNCE_MS } from '../lib/url.js'

  const BASE = '/subjects'
  let cons = $state(null)
  let types = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)

  let f = $state({
    q: '',
    type: '',
    platform: '',
    tag: '',
    metaTag: '',
    rankMin: '',
    scoreMin: '',
    dateFrom: '',
    dateTo: '',
    nsfw: '',
    series: '',
    sort: 'id',
    order: 'asc',
    size: 30
  })

  const location = useLocation()
  const platforms = $derived(f.type ? platformsFor(cons, f.type) : [])

  const sortOptions = [
    { value: 'id', label: 'ID' },
    { value: 'rank', label: '排名' },
    { value: 'score', label: '评分' },
    { value: 'date', label: '日期' },
    { value: 'favorite', label: '收藏' }
  ]
  const nsfwOptions = [
    { value: '', label: '不限' },
    { value: '0', label: '非 R18' },
    { value: '1', label: '仅 R18' }
  ]
  const seriesOptions = [
    { value: '', label: '不限' },
    { value: '0', label: '非系列' },
    { value: '1', label: '系列' }
  ]

  onMount(async () => {
    cons = await loadConstants()
    types = enumList(cons.subject_types)
  })

  // 搜索以地址栏查询串为唯一驱动：挂载、搜索、翻页、前进后退统一走此路径。
  // 只读 $location.search，显式传参请求，避免依赖回环；签名不变（如仅弹窗锚点变化的
  // 前进/后退）时跳过重复搜索。
  let appliedSig = null
  let appliedFormSig = ''
  $effect(() => {
    const sp = parseQuery($location.search)
    const sig = sp.toString()
    if (sig === appliedSig) return
    appliedSig = sig

    const p = {
      q: sp.get('q') ?? '',
      type: sp.get('type') ?? '',
      platform: sp.get('platform') ?? '',
      tag: sp.get('tag') ?? '',
      metaTag: sp.get('meta_tag') ?? '',
      rankMin: sp.get('rank_min') ?? '',
      scoreMin: sp.get('score_min') ?? '',
      dateFrom: sp.get('date_from') ?? '',
      dateTo: sp.get('date_to') ?? '',
      nsfw: sp.get('nsfw') ?? '',
      series: sp.get('series') ?? '',
      sort: sp.get('sort') || 'id',
      order: sp.get('order') || 'asc',
      page: intParam(sp, 'page', 1),
      size: 30
    }
    Object.assign(f, {
      q: p.q,
      type: p.type,
      platform: p.platform,
      tag: p.tag,
      metaTag: p.metaTag,
      rankMin: p.rankMin,
      scoreMin: p.scoreMin,
      dateFrom: p.dateFrom,
      dateTo: p.dateTo,
      nsfw: p.nsfw,
      series: p.series,
      sort: p.sort,
      order: p.order
    })
    appliedFormSig = formSig()
    doSearch(p)
  })

  // 自动搜索（无搜索建议）：表单相对最近一次已执行搜索的快照有任何变更时，
  // 停顿 AUTO_SEARCH_DEBOUNCE_MS 后自动提交。地址驱动的搜索（挂载/前进后退/
  // 翻页/手动提交）会先更新快照，故不会误触发；输入回退到与快照一致则取消。
  let autoTimer = null
  const formSig = () => JSON.stringify(f)
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
      result = await searchSubjects(p)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  // 当前表单 -> 查询参数（默认值不写入，保持地址简洁）
  function formParams(extra = {}) {
    const p = {
      q: f.q,
      type: f.type,
      platform: f.platform,
      tag: f.tag,
      meta_tag: f.metaTag,
      rank_min: f.rankMin,
      score_min: f.scoreMin,
      date_from: f.dateFrom,
      date_to: f.dateTo,
      nsfw: f.nsfw,
      series: f.series
    }
    if (f.sort !== 'id') p.sort = f.sort
    if (f.order !== 'asc') p.order = f.order
    return { ...p, ...extra }
  }

  function submit() {
    clearTimeout(autoTimer)
    autoTimer = null
    pushSearch(BASE, formParams())
  }

  function resetForm() {
    pushSearch(BASE, {})
  }

  function changePage(p) {
    pushSearch(BASE, formParams({ page: p }))
  }
</script>

<div class="grid gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-900/60" onsubmit={(e) => { e.preventDefault(); submit() }}>
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <div class="col-span-2 lg:col-span-4">
        <label class="label" for="subject-q">关键词（FTS 全文搜索，中文子串匹配）</label>
        <input id="subject-q" class="input" type="text" placeholder="如：路人女主的养成方法" bind:value={f.q} />
      </div>

      <div>
        <label class="label" for="subject-type">类型</label>
        <select id="subject-type" class="input" bind:value={f.type} onchange={() => (f.platform = '')}>
          <option value="">全部</option>
          {#each types as t}
            <option value={t.id}>{t.name}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="subject-platform">平台</label>
        <select id="subject-platform" class="input" bind:value={f.platform} disabled={!f.type}>
          <option value="">全部</option>
          {#each platforms as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="subject-tag">标签</label>
        <input id="subject-tag" class="input" type="text" placeholder="如：奇幻" bind:value={f.tag} />
      </div>
      <div>
        <label class="label" for="subject-metatag">Meta 标签</label>
        <input id="subject-metatag" class="input" type="text" placeholder="如：社畜" bind:value={f.metaTag} />
      </div>

      <div>
        <label class="label" for="subject-rank">排名 ≥</label>
        <input id="subject-rank" class="input" type="number" min="1" placeholder="如：1" bind:value={f.rankMin} />
      </div>
      <div>
        <label class="label" for="subject-score">评分 ≥</label>
        <input id="subject-score" class="input" type="number" min="0" max="10" step="0.1" placeholder="如：8" bind:value={f.scoreMin} />
      </div>
      <div>
        <label class="label" for="subject-date-from">日期从</label>
        <input id="subject-date-from" class="input" type="date" bind:value={f.dateFrom} />
      </div>
      <div>
        <label class="label" for="subject-date-to">日期到</label>
        <input id="subject-date-to" class="input" type="date" bind:value={f.dateTo} />
      </div>

      <div>
        <label class="label" for="subject-nsfw">R18</label>
        <select id="subject-nsfw" class="input" bind:value={f.nsfw}>
          {#each nsfwOptions as o}
            <option value={o.value}>{o.label}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="subject-series">系列</label>
        <select id="subject-series" class="input" bind:value={f.series}>
          {#each seriesOptions as o}
            <option value={o.value}>{o.label}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="subject-sort">排序</label>
        <select id="subject-sort" class="input" bind:value={f.sort}>
          {#each sortOptions as o}
            <option value={o.value}>{o.label}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="subject-order">顺序</label>
        <select id="subject-order" class="input" bind:value={f.order}>
          <option value="asc">升序</option>
          <option value="desc">降序</option>
        </select>
      </div>
    </div>

    <div class="flex items-center gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '搜索中…' : '搜索'}</button>
      <button class="btn-ghost" type="button" onclick={resetForm}>重置</button>
      <span class="ml-auto text-xs text-neutral-500">GET /api/subjects/search</span>
    </div>
  </form>

  {#if error}
    <div class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-900 dark:bg-red-950/50 dark:text-red-400">请求失败：{error}</div>
  {/if}

  {#if loading}
    <div class="py-8 text-center text-sm text-neutral-500">加载中…</div>
  {:else if result}
    <div class="rounded-lg border border-neutral-200 bg-white/60 dark:border-neutral-800 dark:bg-neutral-900/60">
      <div class="flex items-center justify-between border-b border-neutral-200 px-4 py-2 dark:border-neutral-800">
        <Pagination total={result.total} page={result.page} size={result.size} onchange={changePage} />
      </div>
      <div class="overflow-x-auto p-2">
        <table class="tbl">
          <thead>
            <tr>
              <th>ID</th>
              <th>类型</th>
              <th>中文名</th>
              <th>原名</th>
              <th>平台</th>
              <th>日期</th>
              <th>评分</th>
              <th>排名</th>
              <th>标签</th>
              <th>收藏</th>
            </tr>
          </thead>
          <tbody>
            {#each result.items as it (it.id)}
              <tr class="cursor-pointer hover:bg-neutral-100 dark:hover:bg-neutral-800/50" onclick={() => openDetail('subject', it.id, it)}>
                <td class="text-neutral-500">{it.id}</td>
                <td>{it.type_name}</td>
                <td class="max-w-52 truncate">
                  {#if it.name_cn}
                    <a href={`https://bgm.tv/subject/${it.id}`} target="_blank" rel="noreferrer" class="text-sky-600 hover:underline dark:text-sky-400" onclick={(e) => e.stopPropagation()}>{it.name_cn}</a>
                  {:else}
                    —
                  {/if}
                </td>
                <td class="max-w-52 truncate text-neutral-500 dark:text-neutral-400">{it.name}</td>
                <td>{it.platform_name}</td>
                <td>{fmtDate(it.date)}</td>
                <td class="text-amber-600 dark:text-amber-400">{fmtScore(it.score)}</td>
                <td>{fmtRank(it.rank)}</td>
                <td class="max-w-60">
                  <div class="flex flex-wrap gap-1">
                    {#each (it.tags ?? []).slice(0, 5) as t}
                      <span class="chip">{t.name}</span>
                    {/each}
                  </div>
                </td>
                <td class="max-w-48 text-xs text-neutral-500 dark:text-neutral-400">{fmtFavorite(it.favorite)}</td>
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
