<script>
  import { onMount } from 'svelte'
  import { searchSubjects, getSubject } from '../lib/api.js'
  import { loadConstants, platformsFor, enumList } from '../lib/constants.js'
  import { fmtScore, fmtRank, fmtDate, fmtFavorite } from '../lib/format.js'
  import Pagination from '../components/Pagination.svelte'

  let cons = $state(null)
  let types = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)
  let detail = $state(null)
  let detailLoading = $state(false)

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
  let page = $state(1)

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
    await doSearch()
  })

  async function doSearch() {
    loading = true
    error = ''
    result = null
    try {
      result = await searchSubjects({ ...f, page })
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function submit() {
    page = 1
    doSearch()
  }

  function resetForm() {
    f.q = ''
    f.type = ''
    f.platform = ''
    f.tag = ''
    f.metaTag = ''
    f.rankMin = ''
    f.scoreMin = ''
    f.dateFrom = ''
    f.dateTo = ''
    f.nsfw = ''
    f.series = ''
    f.sort = 'id'
    f.order = 'asc'
    page = 1
    doSearch()
  }

  function changePage(p) {
    page = p
    doSearch()
  }

  async function openDetail(item) {
    detailLoading = true
    detail = null
    try {
      detail = await getSubject(item.id)
    } catch (e) {
      detail = { error: e.message }
    } finally {
      detailLoading = false
    }
  }
</script>

<div class="grid gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-800 bg-neutral-900/60 p-4" onsubmit={(e) => { e.preventDefault(); submit() }}>
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
    <div class="rounded-md border border-red-900 bg-red-950/50 px-3 py-2 text-sm text-red-400">请求失败：{error}</div>
  {/if}

  {#if loading}
    <div class="py-8 text-center text-sm text-neutral-500">加载中…</div>
  {:else if result}
    <div class="rounded-lg border border-neutral-800 bg-neutral-900/60">
      <div class="flex items-center justify-between border-b border-neutral-800 px-4 py-2">
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
              <tr class="cursor-pointer hover:bg-neutral-800/50" onclick={() => openDetail(it)}>
                <td class="text-neutral-500">{it.id}</td>
                <td>{it.type_name}</td>
                <td class="max-w-52 truncate">{it.name_cn || '—'}</td>
                <td class="max-w-52 truncate text-neutral-400">{it.name}</td>
                <td>{it.platform_name}</td>
                <td>{fmtDate(it.date)}</td>
                <td class="text-amber-400">{fmtScore(it.score)}</td>
                <td>{fmtRank(it.rank)}</td>
                <td class="max-w-60">
                  <div class="flex flex-wrap gap-1">
                    {#each (it.tags ?? []).slice(0, 5) as t}
                      <span class="chip">{t.name}</span>
                    {/each}
                  </div>
                </td>
                <td class="max-w-48 text-xs text-neutral-400">{fmtFavorite(it.favorite)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if result.items.length === 0}
        <div class="py-8 text-center text-sm text-neutral-500">无结果</div>
      {/if}
    </div>

    {#if detailLoading}
      <div class="py-4 text-center text-sm text-neutral-500">加载详情…</div>
    {:else if detail}
      <div class="rounded-lg border border-neutral-800 bg-neutral-900/60 p-4">
        <div class="mb-2 flex items-center gap-2">
          <h3 class="text-base font-semibold">{detail.name_cn || detail.name}</h3>
          <span class="chip">{detail.type_name}</span>
          <span class="chip">{detail.platform_name}</span>
          {#if detail.series}<span class="chip">系列</span>{/if}
          {#if detail.nsfw}<span class="chip text-red-400">R18</span>{/if}
          <button class="ml-auto btn-mini" onclick={() => (detail = null)}>关闭</button>
        </div>
        {#if detail.error}
          <p class="text-sm text-red-400">详情加载失败：{detail.error}</p>
        {:else}
          <p class="mb-2 text-sm text-neutral-400">{detail.summary || '（无简介）'}</p>
          <div class="grid gap-4 lg:grid-cols-3">
            <section>
              <h4 class="mb-1 text-xs font-medium text-neutral-400">关联（{detail.relations.length}）</h4>
              <ul class="space-y-0.5 text-sm">
                {#each detail.relations as r}
                  <li class="text-neutral-300">
                    <span class="text-neutral-500">{r.relation_name}</span> →
                    <span class="text-neutral-500">{r.related_type_name}</span> {r.related_name_cn || r.related_name}
                  </li>
                {/each}
              </ul>
            </section>
            <section>
              <h4 class="mb-1 text-xs font-medium text-neutral-400">制作人员（{detail.staff.length}）</h4>
              <ul class="space-y-0.5 text-sm">
                {#each detail.staff as s}
                  <li class="text-neutral-300">
                    <span class="text-neutral-500">{s.position_name}</span> {s.person_name}
                  </li>
                {/each}
              </ul>
            </section>
            <section>
              <h4 class="mb-1 text-xs font-medium text-neutral-400">角色（{detail.characters.length}）</h4>
              <ul class="space-y-0.5 text-sm">
                {#each detail.characters as c}
                  <li class="text-neutral-300">
                    <span class="text-neutral-500">{c.role_name}</span> {c.name}
                  </li>
                {/each}
              </ul>
            </section>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>