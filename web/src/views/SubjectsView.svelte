<script>
  import { onMount } from 'svelte'
  import { searchSubjects } from '../lib/api.js'
  import {
    loadConstants,
    enumList,
    SUBJECT_AUTO_SEARCH_DEBOUNCE_MS,
    TAG_BOX_IDLE_SEARCH_MS
  } from '../lib/constants.js'
  import { getPlatforms } from '../lib/platforms.js'
  import { fmtScore, fmtRank, fmtDate, fmtFavorite, fmtCompact } from '../lib/format.js'
 import Pagination from '../components/Pagination.svelte'
 import Highlight from '../components/Highlight.svelte'
 import TagSuggest from '../components/TagSuggest.svelte'
  import { openDetail } from '../lib/detail.svelte.js'
  import { externalUrl, isHighlightEnabled } from '../lib/settings.svelte.js'

  const DEFAULTS = {
    q: '',
    type: '',
    platform: '',
    tag: '',
    rankMin: '',
    scoreMin: '',
    dateFrom: '',
    dateTo: '',
    nsfw: '',
    series: '',
    sort: 'id',
    order: 'asc',
    size: 30
  }
  let cons = $state(null)
  let allPlatforms = $state({})
  let types = $state([])
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)

  let f = $state({ ...DEFAULTS })

  const platforms = $derived(
    f.type
      ? Object.values(allPlatforms[Number(f.type)] ?? {})
          .map((p) => ({ id: p.id, name: p.type_cn }))
          .sort((a, b) => a.id - b.id)
      : []
  )

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
    const [consData, platformsData] = await Promise.all([loadConstants(), getPlatforms()])
    cons = consData
    allPlatforms = platformsData
    types = enumList(cons.subject_types)
    await doSearch({ ...f, page: 1 }) // 挂载即展示第 1 页（空条件 = 全量列表）
  })

  // 搜索由表单状态直接驱动：提交/翻页/重置时按当前表单发起请求。
  let autoTimer = null
  const formSig = () => JSON.stringify(f)
  let appliedFormSig = formSig()

  // 标签框激活状态与最近变动时间：激活期间自动搜索需等待标签内容静默，
  // 避免选词（含拼音首字母检索）过程中频繁触发搜索。
  let tagBoxFocus = $state(false)
  let lastTagBoxChangeAt = $state(0)

  // 自动搜索：表单相对最近一次已执行搜索的快照有任何变更时，停顿
  // SUBJECT_AUTO_SEARCH_DEBOUNCE_MS 后自动提交；若任一标签建议框仍处于激活状态，
  // 则需其内容静默 TAG_BOX_IDLE_SEARCH_MS 后才触发。手动提交会先更新快照，
  // 故不会误触发；输入回退到与快照一致则取消。
  $effect(() => {
    const sig = formSig()
    if (sig === appliedFormSig) return
    clearTimeout(autoTimer)
    let delay = SUBJECT_AUTO_SEARCH_DEBOUNCE_MS
    if (tagBoxFocus) {
      delay = Math.max(delay, TAG_BOX_IDLE_SEARCH_MS - (Date.now() - lastTagBoxChangeAt))
    }
    autoTimer = setTimeout(() => {
      autoTimer = null
      submit()
    }, delay)
    return () => clearTimeout(autoTimer)
  })

  async function doSearch(p) {
    loading = true
    error = ''
    result = null
    try {
      result = await searchSubjects(p)
      tagExpandOverrides = {} // 新结果回到默认收拢状态
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
    doSearch({ ...f, page: 1 })
  }

  function resetForm() {
    clearTimeout(autoTimer)
    autoTimer = null
    Object.assign(f, DEFAULTS)
    appliedFormSig = formSig()
    doSearch({ ...DEFAULTS, page: 1 })
  }

  function changePage(p) {
    doSearch({ ...f, page: p })
  }

  // ---- 标签列展示 ----
  // 元标签在前（着重样式），其余标签在后并附使用次数；默认只展示前
  // TAGS_PREVIEW_N 个，行内可展开/收起，表头另有整体展开/收拢按钮。
  const TAGS_PREVIEW_N = 12

  let allTagsExpanded = $state(false) // 全局展开状态
  let tagExpandOverrides = $state({}) // 单行覆盖：subject_id -> 是否展开

  // 合并元标签与普通标签：元标签在前；同名标签去重（显示元标签样式，附使用次数）
  function rowTags(it) {
    const metaSet = new Set(it.meta_tags ?? [])
    const metaList = (it.meta_tags ?? []).map((name) => {
      const tag = (it.tags ?? []).find((t) => t.name === name)
      return { name, cnt: tag?.count ?? 0, meta: true }
    })
    const tagList = (it.tags ?? [])
      .filter((t) => !metaSet.has(t.name))
      .map((t) => ({ name: t.name, cnt: t.count ?? 0, meta: false }))
    return [...metaList, ...tagList]
  }

  // 当前行实际渲染的标签（收拢时截取前 TAGS_PREVIEW_N 个）
  function rowTagsShown(it) {
    const list = rowTags(it)
    return rowTagsExpanded(it.id) ? list : list.slice(0, TAGS_PREVIEW_N)
  }

  function rowTagsExpanded(id) {
    return tagExpandOverrides[id] ?? allTagsExpanded
  }

  function toggleRowTags(id) {
    tagExpandOverrides = { ...tagExpandOverrides, [id]: !rowTagsExpanded(id) }
  }

  function toggleAllTags() {
    allTagsExpanded = !allTagsExpanded
    tagExpandOverrides = {} // 清空单行覆盖，回到全局一致状态
  }

  // 搜索参数中的正标签集合：用于在结果标签列高亮命中项
  const positiveTagSet = $derived.by(() => {
    const set = new Set()
    for (const part of String(f.tag ?? '').split(/[,，]/)) {
      const t = part.trim()
      if (!t || t === '+' || t === '-' || t.startsWith('-')) continue
      set.add(t.startsWith('+') ? t.slice(1).trim() : t)
    }
    return set
  })

  // 当前页面结果中的所有标签（用于标签搜索建议优先展示）
  const pageTags = $derived.by(() => {
    if (!result?.items) return []
    const tagMap = new Map()
    for (const it of result.items) {
      // 普通标签
      for (const t of it.tags ?? []) {
        if (!tagMap.has(t.name)) {
          tagMap.set(t.name, { name: t.name, cnt: t.count ?? 0 })
        }
      }
      // 元标签
      for (const name of it.meta_tags ?? []) {
        if (!tagMap.has(name)) {
          const tag = (it.tags ?? []).find((t) => t.name === name)
          tagMap.set(name, { name, cnt: tag?.count ?? 0 })
        }
      }
    }
    return Array.from(tagMap.values())
  })

  // 正标签高亮样式（与关键词着重号一致的黄色下划线）
  const POS_MARK =
    'border-2 border-solid border-yellow-400 rounded px-1 py-0'
</script>

<div class="rise grid grid-cols-[minmax(0,1fr)] gap-4">
  <form class="grid gap-3 card p-4" onsubmit={(e) => { e.preventDefault(); submit() }}>
    <div class="grid grid-cols-[repeat(2,minmax(0,1fr))] gap-3 lg:grid-cols-[repeat(4,minmax(0,1fr))]">
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
        <label class="label" for="subject-platform">子类型</label>
        <select id="subject-platform" class="input" bind:value={f.platform} disabled={!f.type}>
          <option value="">全部</option>
          {#each platforms as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
      </div>
      <div class="col-span-2">
        <label class="label" for="subject-tag">标签（+必须包含 / -必须排除，逗号分隔，普通标签与元标签统一检索）</label>
        <TagSuggest
          inputId="subject-tag"
          kind="all"
          placeholder="如：+奇幻,-科幻 或 小说"
          bind:text={f.tag}
          {pageTags}
          onfocus={() => (tagBoxFocus = true)}
          onblur={() => (tagBoxFocus = false)}
          oninput={() => (lastTagBoxChangeAt = Date.now())}
        />
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
      <div class="flex items-center justify-between gap-3 border-b border-neutral-200 px-4 py-2 dark:border-neutral-800">
        <Pagination total={result.total} page={result.page} size={result.size} onchange={changePage} />
        <button
          type="button"
          class="btn-mini shrink-0"
          disabled={!result.items.length}
          onclick={toggleAllTags}
        >{allTagsExpanded ? '收起全部标签' : '展开全部标签'}</button>
      </div>
      <div class="overflow-x-auto p-2">
        <table class="tbl">
          <thead>
            <tr>
              <th>ID</th>
              <th>类型</th>
              <th>中文名</th>
              <th>原名</th>
              <th>子类型</th>
              <th>日期</th>
              <th>评分</th>
              <th>排名</th>
              <th>标签</th>
              <th>收藏</th>
            </tr>
          </thead>
          <tbody class="stagger">
            {#each result.items as it (it.id)}
              <tr class="cursor-pointer transition-colors hover:bg-sakura-50/70 dark:hover:bg-white/[0.04]" onclick={() => openDetail('subject', it.id, it)}>
                <td class="text-neutral-500">{it.id}</td>
                <td>{it.type_name}</td>
                <td class="max-w-52 truncate">
                  {#if it.name_cn}
                    <a href={externalUrl('subject', it.id)} target="_blank" rel="noreferrer" class="text-sakura-600 hover:underline dark:text-sakura-400" onclick={(e) => e.stopPropagation()}>
                      {#if isHighlightEnabled('title')}<Highlight text={it.name_cn} q={f.q} />{:else}{it.name_cn}{/if}
                    </a>
                  {:else}
                    —
                  {/if}
                </td>
                <td class="max-w-52 truncate text-neutral-500 dark:text-neutral-400">
                  {#if isHighlightEnabled('title')}<Highlight text={it.name} q={f.q} />{:else}{it.name}{/if}
                </td>
                <td>{it.platform_name}</td>
                <td>{fmtDate(it.date)}</td>
                <td class="text-amber-600 dark:text-amber-400">{fmtScore(it.score)}</td>
                <td>{fmtRank(it.rank)}</td>
                <td class="max-w-96">
                  <div class="flex flex-wrap items-center gap-1">
                    {#each rowTagsShown(it) as t (`${t.meta ? 'm' : 't'}:${t.name}`)}
                      <span class="{t.meta ? 'chip-meta' : 'chip'} {isHighlightEnabled('tags') && positiveTagSet.has(t.name) ? POS_MARK : ''}">
                        {t.name}{#if t.cnt > 0}<small class="ml-0.5 text-neutral-400 dark:text-neutral-500" title="{t.name} 被引用 {t.cnt} 次">{fmtCompact(t.cnt)}</small>{/if}
                      </span>
                    {/each}
                    {#if rowTags(it).length > TAGS_PREVIEW_N}
                      <button
                        type="button"
                        class="btn-mini"
                        title={rowTagsExpanded(it.id) ? '收起标签' : `展开全部 ${rowTags(it).length} 个标签`}
                        onclick={(e) => {
                          e.stopPropagation() // 不触发行点击的详情抽屉
                          toggleRowTags(it.id)
                        }}
                      >{rowTagsExpanded(it.id) ? '收起' : `+${rowTags(it).length - TAGS_PREVIEW_N}`}</button>
                    {/if}
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
