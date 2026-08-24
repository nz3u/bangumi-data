<script>
  import { fade } from 'svelte/transition'
  import { getPersonCollaboration, getPersonCollaborationPositions } from '../lib/api.js'
  import { careerCn } from '../lib/format.js'
  import PinyinMatch from 'pinyin-match'
  import Pagination from '../components/Pagination.svelte'
  import PersonSuggest from '../components/PersonSuggest.svelte'
  import PersonAvatar from '../components/PersonAvatar.svelte'
  import { consumeNav } from '../lib/nav.svelte.js'

  const BASE = '/collaborations'

  let pidInput = $state('')
  let pidSel = $state(null) // 搜索提示选中的人物（此时输入框显示名字）
  let loading = $state(false)
  let error = $state('')
  let data = $state(null)
  let page = $state(1)
  const size = 20

  // ---- 棋盘筛选状态 ----
  let currentPid = $state(null) // 当前已查询的人物 ID
  let facets = $state(null) // { self: 当前人物职位标签, other: 合作人物职位标签 }
  let facetsError = $state('')
  let selA = $state([]) // 左侧：当前人物职位（多选 key）
  let selB = $state([]) // 上侧：合作人物职位（多选 key）

  // ---- 前端快速搜索 ----
  let filter = $state('')

  // ---- 职位标签检索 ----
  // 同侧的悬浮轨与窄屏回退框共用一个搜索词；支持中文全文或拼音首字母（如 dy→导演）。
  let tagQA = $state('') // 左列：当前人物职位
  let tagQB = $state('') // 右列：合作人物职位

  // 支持直接输入数字 ID，或粘贴 /person/596 之类的链接
  function extractId(v) {
    const m = String(v ?? '').trim().match(/\d+/)
    return m ? Number(m[0]) : NaN
  }

  function buildParams(p) {
    return { page: p, size, positions_a: selA.join(','), positions_b: selB.join(',') }
  }

  let reqSeq = 0 // 连续点击标签时的过期响应保护
  let renderSeq = $state(0) // 每次数据到达后自增，触发列表区域淡入动画

  async function load(pid, p = 1) {
    const seq = ++reqSeq
    loading = true
    error = ''
    try {
      const d = await getPersonCollaboration(pid, buildParams(p))
      if (seq !== reqSeq) return
      data = d
      page = p
      renderSeq++
      expandAllSubj = false // 新一页数据回到默认收缩状态
      subjOverrides = {}
    } catch (e) {
      if (seq !== reqSeq) return
      error = e.message
    } finally {
      if (seq === reqSeq) loading = false
    }
  }

  async function loadPositions(pid) {
    facetsError = ''
    facets = null
    try {
      facets = await getPersonCollaborationPositions(pid)
    } catch (e) {
      facetsError = e.message
    }
  }

  // 查询新人物：重置棋盘选择与快速搜索后并行加载列表与职位标签
  async function search(pid) {
    currentPid = pid
    selA = []
    selB = []
    filter = ''
    tagQA = ''
    tagQB = ''
    error = ''
    data = null // 切换人物时清除旧内容，避免闪现旧数据
    loadPositions(pid)
    await load(pid, 1)
  }

  function submit(e) {
    e.preventDefault()
    // 选中建议时组件已把真实 ID 同步到 pidSel；否则按原逻辑解析数字
    const pid = pidSel ?? extractId(pidInput)
    if (!pid || pid <= 0) {
      error = '请输入人物 ID（数字）'
      data = null
      return
    }
    search(pid)
  }

  function pickPerson(p) {
    search(p.id)
  }

  // 跨标签页内部传参：其他页面（如人物详情抽屉）跳转过来时携带目标人物 ID。
  // 挂载时消费在途参数；停留本页期间再次收到跳转时因 seq 变化重新消费。
  $effect(() => {
    const params = consumeNav(BASE)
    const pid = Number(params?.id ?? 0)
    if (!pid || pid <= 0 || pid === currentPid) return
    pidInput = String(pid)
    pidSel = null
    search(pid)
  })

  // 点击棋盘标签：切换选中并自动重新请求（回到第 1 页）
  function toggleTag(side, key) {
    if (!currentPid || loading) return
    if (side === 'a') {
      selA = selA.includes(key) ? selA.filter((k) => k !== key) : [...selA, key]
    } else {
      selB = selB.includes(key) ? selB.filter((k) => k !== key) : [...selB, key]
    }
    load(currentPid, 1)
  }

  function clearTags(side) {
    if (!currentPid || loading) return
    if (side === 'a' && selA.length) {
      selA = []
      load(currentPid, 1)
    } else if (side === 'b' && selB.length) {
      selB = []
      load(currentPid, 1)
    }
  }

  function changePage(p) {
    if (!currentPid) return
    load(currentPid, p)
  }

  // 一键重置两组棋盘标签并重新请求
  function clearAllTags() {
    if (!currentPid || loading || (!selA.length && !selB.length)) return
    selA = []
    selB = []
    load(currentPid, 1)
  }

  // 已选 key 列表转标签文本（用于小标题右侧的已选组合展示）
  function selLabelStr(list, keys) {
    return keys.map((k) => list?.find((t) => t.key === k)?.label ?? k).join(' / ') || '不限'
  }

  // 各侧按搜索词过滤后的职位标签（悬浮轨与窄屏框共用）。
  // PinyinMatch 支持中文全文与拼音全拼/首字母匹配（如 dy→导演），空词视为全部命中。
  const tagHit = (label, q) => {
    const s = String(q ?? '').trim()
    return !s || !!PinyinMatch.match(label, s)
  }
  const selfTagsShown = $derived.by(() => (facets?.self ?? []).filter((t) => tagHit(t.label, tagQA)))
  const otherTagsShown = $derived.by(() => (facets?.other ?? []).filter((t) => tagHit(t.label, tagQB)))

  // ---- 合作条目收缩/展开 ----
  // 默认每人只展示前 N 条；卡片按钮单独切换，标题旁按钮全局展开/收起并清空单卡覆盖。
  const COLLAB_PREVIEW_N = 10
  let expandAllSubj = $state(false)
  let subjOverrides = $state({}) // person_id -> 是否展开

  function subjExpanded(pid) {
    return subjOverrides[pid] ?? expandAllSubj
  }

  function toggleSubj(pid) {
    subjOverrides = { ...subjOverrides, [pid]: !subjExpanded(pid) }
  }

  function toggleAllSubj() {
    expandAllSubj = !expandAllSubj
    subjOverrides = {}
  }

  // 当前卡片应渲染的条目：收缩时截断到前 N 条
  function shownSubjects(col) {
    return !subjExpanded(col.person_id) && col.subjects.length > COLLAB_PREVIEW_N
      ? col.subjects.slice(0, COLLAB_PREVIEW_N)
      : col.subjects
  }

  const hasCollapsible = $derived((data?.items ?? []).some((c) => (c.subjects?.length ?? 0) > COLLAB_PREVIEW_N))

  const tagIdleCls =
    'rounded-full border border-neutral-300 bg-white px-2.5 py-0.5 text-xs text-neutral-600 hover:border-sky-400 hover:text-sky-600 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-300 dark:hover:border-sky-500 dark:hover:text-sky-400'
  const tagOnCls = 'rounded-full bg-sky-600 px-2.5 py-0.5 text-xs font-medium text-white hover:bg-sky-500'

  // ---- 前端快速搜索 ----
  // 搜索范围覆盖当前页全部展示内容：名称、类型、职业、简介、共同条目（标题/日期/类型/职位）。
  // 仅作用于当前页，不改变服务端分页与棋盘筛选。
  // 命中判定先走原文子串，未命中再尝试拼音全拼/首字母匹配（如 dy→导演）。
  const visibleItems = $derived.by(() => {
    if (!data?.items) return null
    const q = filter.trim().toLowerCase()
    if (!q) return data.items
    const hit = (text) =>
      String(text).toLowerCase().includes(q) || !!PinyinMatch.match(String(text), q)
    const rowText = (col) =>
      [
        col.name,
        col.type_name,
        col.summary,
        ...(col.career ?? []).map(careerCn),
        ...(col.subjects ?? []).flatMap((s) => [s.name_cn, s.name, s.date, s.type_name, ...(s.roles ?? [])])
      ]
        .join('\n')
    return data.items.filter((col) => hit(rowText(col)))
  })
</script>

<div class="grid gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-200 bg-white/60 p-4 lg:grid-cols-[1fr_auto] lg:items-end dark:border-neutral-800 dark:bg-neutral-900/60" onsubmit={submit}>
    <div>
      <label class="label" for="collab-pid">人物 ID</label>
      <PersonSuggest inputId="collab-pid" placeholder="如：1、名字或粘贴 https://bgm.tv/person/1" bind:text={pidInput} bind:pid={pidSel} onpick={pickPerson} />
    </div>
    <div class="flex items-center gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '查询中…' : '查询'}</button>
      <span class="text-xs text-neutral-500">GET /api/persons/:id/collaboration</span>
    </div>
  </form>

  {#if error}
    <div class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-900 dark:bg-red-950/50 dark:text-red-400">请求失败：{error}</div>
  {/if}

  {#if loading && !data}
    <div class="py-8 text-center text-sm text-neutral-500">加载中…</div>
  {:else if data}
    <div class="relative">
      <!-- 左侧悬浮轨：当前人物职位（细长列表，悬浮于页面左侧留白） -->
      {#if facets}
        <div class="absolute inset-y-0 left-0 z-10 hidden w-28 -translate-x-full pr-2 min-[1440px]:block">
          <nav
            class="sticky top-6 flex max-h-[90vh] flex-col gap-1 rounded-lg border border-neutral-200 bg-white/95 p-1.5 shadow-sm backdrop-blur dark:border-neutral-800 dark:bg-neutral-900/95"
            aria-label="当前人物职位筛选"
          >
            <div class="flex items-center justify-between gap-1 px-1 pt-0.5">
              <span class="text-[11px] font-semibold leading-tight">当前人物职位</span>
              {#if selA.length > 0}
                <button class="btn-mini shrink-0" type="button" onclick={() => clearTags('a')}>清除</button>
              {/if}
            </div>
            <input
              class="input-xs shrink-0"
              type="search"
              placeholder="搜职位…"
              title="支持中文或拼音首字母，如 dy→导演"
              aria-label="搜索当前人物职位标签"
              bind:value={tagQA}
            />
            <div class="flex min-h-0 flex-1 flex-col items-stretch gap-1 overflow-y-auto pb-0.5">
              {#each selfTagsShown as t (t.key)}
                <button
                  type="button"
                  class={`flex w-full shrink-0 items-baseline justify-between gap-1 rounded-full px-1.5 py-0.5 text-[11px] ${selA.includes(t.key) ? 'bg-sky-600 font-medium text-white hover:bg-sky-500' : 'border border-neutral-300 bg-white text-neutral-600 hover:border-sky-400 hover:text-sky-600 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-300 dark:hover:border-sky-500 dark:hover:text-sky-400'}`}
                  title={`筛选当前人物担任「${t.label}」的共同条目`}
                  onclick={() => toggleTag('a', t.key)}
                >
                  <span class="min-w-0 truncate">{t.label}</span><small class="shrink-0 opacity-70">{t.count}</small>
                </button>
              {:else}
                <span class="px-1 text-xs text-neutral-400">{facets.self.length ? '无匹配标签' : '无可用标签'}</span>
              {/each}
            </div>
          </nav>
        </div>

        <!-- 右侧悬浮轨：合作人物职位（多选，内部滚动，悬停于整体外侧） -->
        <div class="absolute inset-y-0 right-0 z-10 hidden w-40 translate-x-full pl-3 2xl:block">
          <nav
            class="sticky top-6 flex max-h-[90vh] flex-col rounded-lg border border-neutral-200 bg-white/95 p-2 shadow-sm backdrop-blur dark:border-neutral-800 dark:bg-neutral-900/95"
            aria-label="合作人物职位筛选"
          >
            <div class="flex items-center justify-between gap-1 px-1">
              <span class="text-xs font-semibold">合作人物职位</span>
              {#if selB.length > 0}
                <button class="btn-mini" type="button" onclick={() => clearTags('b')}>清除</button>
              {/if}
            </div>
            <p class="mb-1 mt-0.5 px-1 text-[11px] leading-tight text-neutral-400">点击筛选 · 可多选</p>
            <input
              class="input-xs shrink-0"
              type="search"
              placeholder="搜职位…"
              title="支持中文或拼音首字母，如 dy→导演"
              aria-label="搜索合作人物职位标签"
              bind:value={tagQB}
            />
            <div class="mt-1 flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
              {#each otherTagsShown as t (t.key)}
                <button
                  type="button"
                  class={`w-full shrink-0 ${selB.includes(t.key) ? tagOnCls : tagIdleCls}`}
                  title={`筛选担任「${t.label}」的合作人物`}
                  onclick={() => toggleTag('b', t.key)}
                >
                  <span class="min-w-0 truncate">{t.label}</span><small class="ml-1 shrink-0 opacity-70">{t.count}</small>
                </button>
              {:else}
                <span class="px-1 text-xs text-neutral-400">{facets.other.length ? '无匹配标签' : '无可用标签'}</span>
              {/each}
            </div>
          </nav>
        </div>
      {/if}

      <div
        class="grid items-start gap-4 transition-opacity duration-200 lg:grid-cols-[320px_minmax(0,1fr)]"
        class:opacity-40={loading}
        class:pointer-events-none={loading}
      >
      <!-- 左侧：人物简介 -->
      <aside class="rounded-lg border border-neutral-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-900/60">
        <div class="mb-3 flex flex-col items-center">
          <PersonAvatar
            pid={data.person.id}
            name={data.person.name}
            class="size-32 rounded-lg bg-neutral-200 text-4xl font-bold text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400"
          />
          <h2 class="mt-2 text-lg font-bold">{data.person.name}</h2>
          <div class="mt-1 flex flex-wrap justify-center gap-1">
            <span class="chip">{data.person.type_name}</span>
            {#each data.person.career ?? [] as cb}
              <span class="chip">{careerCn(cb)}</span>
            {/each}
          </div>
        </div>

        <dl class="grid grid-cols-2 gap-1 text-xs">
          <div class="rounded bg-neutral-100 px-2 py-1.5 dark:bg-neutral-800/60">
            <dt class="text-neutral-500">参与条目</dt>
            <dd class="text-base font-semibold">{data.person.subjects_count}</dd>
          </div>
          <div class="rounded bg-neutral-100 px-2 py-1.5 dark:bg-neutral-800/60">
            <dt class="text-neutral-500">合作人物</dt>
            <dd class="text-base font-semibold">{data.total}</dd>
          </div>
          <div class="rounded bg-neutral-100 px-2 py-1.5 dark:bg-neutral-800/60">
            <dt class="text-neutral-500">评论</dt>
            <dd class="text-base font-semibold">{data.person.comments}</dd>
          </div>
          <div class="rounded bg-neutral-100 px-2 py-1.5 dark:bg-neutral-800/60">
            <dt class="text-neutral-500">收藏</dt>
            <dd class="text-base font-semibold">{data.person.collects}</dd>
          </div>
        </dl>

        {#if (data.person.infobox ?? []).length > 0}
          <ul class="mt-3 space-y-0.5 text-sm">
            {#each data.person.infobox as f}
              <li class="break-words">
                <span class="tip mr-1 text-neutral-500 dark:text-neutral-400">{f.key}:</span>{String(f.value).replace(/\n+/g, ' / ')}
              </li>
            {/each}
          </ul>
        {/if}

        {#if data.person.summary}
          <p class="mt-3 whitespace-pre-line text-sm leading-relaxed text-neutral-700 dark:text-neutral-300">{data.person.summary}</p>
        {/if}
      </aside>

      <!-- 右侧：快速搜索 + 合作人物列表 -->
      <section class="min-w-0 grid gap-3">
        <!-- 窄屏回退：职位标签横向排布于列表上方（达到各自阈值后改用两侧悬浮轨） -->
        {#if facets}
          <div class="grid gap-y-2 rounded-lg border border-neutral-200 bg-white/60 p-3 min-[1440px]:hidden dark:border-neutral-800 dark:bg-neutral-900/60">
            <div class="min-w-0">
              <div class="mb-1 flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
                <span>当前人物职位{selA.length > 0 ? `（已选 ${selA.length}）` : '（可多选）'}</span>
                {#if selA.length > 0}
                  <button class="btn-mini" type="button" onclick={() => clearTags('a')}>清除</button>
                {/if}
              </div>
              <input
                class="input-xs"
                type="search"
                placeholder="筛选职位，支持拼音首字母，如 dy→导演"
                aria-label="搜索当前人物职位标签"
                bind:value={tagQA}
              />
              <div class="mt-1 flex max-h-24 flex-wrap gap-1 overflow-y-auto">
                {#each selfTagsShown as t (t.key)}
                  <button
                    type="button"
                    class={`shrink-0 ${selA.includes(t.key) ? tagOnCls : tagIdleCls}`}
                    title={`筛选当前人物担任「${t.label}」的共同条目`}
                    onclick={() => toggleTag('a', t.key)}
                  >
                    {t.label}<small class="ml-0.5 opacity-70">{t.count}</small>
                  </button>
                {:else}
                  <span class="text-xs text-neutral-400">{facets.self.length ? '无匹配标签' : '无可用标签'}</span>
                {/each}
              </div>
            </div>
          </div>
          <div class="grid gap-y-2 rounded-lg border border-neutral-200 bg-white/60 p-3 2xl:hidden dark:border-neutral-800 dark:bg-neutral-900/60">
            <div class="min-w-0">
              <div class="mb-1 flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
                <span>合作人物职位{selB.length > 0 ? `（已选 ${selB.length}）` : '（可多选）'}</span>
                {#if selB.length > 0}
                  <button class="btn-mini" type="button" onclick={() => clearTags('b')}>清除</button>
                {/if}
              </div>
              <input
                class="input-xs"
                type="search"
                placeholder="筛选职位，支持拼音首字母，如 dy→导演"
                aria-label="搜索合作人物职位标签"
                bind:value={tagQB}
              />
              <div class="mt-1 flex max-h-24 flex-wrap gap-1 overflow-y-auto">
                {#each otherTagsShown as t (t.key)}
                  <button
                    type="button"
                    class={`shrink-0 ${selB.includes(t.key) ? tagOnCls : tagIdleCls}`}
                    title={`筛选担任「${t.label}」的合作人物`}
                    onclick={() => toggleTag('b', t.key)}
                  >
                    {t.label}<small class="ml-0.5 opacity-70">{t.count}</small>
                  </button>
                {:else}
                  <span class="text-xs text-neutral-400">{facets.other.length ? '无匹配标签' : '无可用标签'}</span>
                {/each}
              </div>
            </div>
          </div>
        {/if}

        {#if facetsError}
          <div class="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900 dark:bg-amber-950/50 dark:text-amber-400">职位标签加载失败：{facetsError}（棋盘筛选不可用）</div>
        {/if}

        <h2 class="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-base font-semibold subtitle">
          <span>与「{data.person.name}」合作的人物（{data.total}）</span>
          {#if hasCollapsible}
            <button
              type="button"
              class="ml-auto inline-flex size-6 shrink-0 items-center justify-center self-center rounded text-neutral-500 hover:bg-neutral-200/70 hover:text-sky-600 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-sky-400"
              title={expandAllSubj ? '全部收起合作条目' : '全部展开合作条目'}
              aria-label={expandAllSubj ? '全部收起合作条目' : '全部展开合作条目'}
              onclick={toggleAllSubj}
            >
              <svg
                class="size-4 transition-transform duration-150"
                class:rotate-180={expandAllSubj}
                viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"
              >
                <path d="m7 6 5 5 5-5" />
                <path d="m7 13 5 5 5-5" />
              </svg>
            </button>
          {/if}
          {#if facets && (selA.length > 0 || selB.length > 0)}
            <span class="text-xs font-normal text-neutral-500 dark:text-neutral-400">
              已选组合：
              <b class="font-medium text-sky-600 dark:text-sky-400">{selLabelStr(facets.self, selA)}</b>
              ×
              <b class="font-medium text-sky-600 dark:text-sky-400">{selLabelStr(facets.other, selB)}</b>
              <button class="btn-mini ml-1" type="button" onclick={clearAllTags}>重置</button>
            </span>
          {/if}
          {#if filter.trim()}
            <span class="chip text-sky-600 dark:text-sky-400">命中 {visibleItems?.length ?? 0}/{data.items.length}</span>
          {/if}
          {#if loading}
            <span class="inline-flex items-center gap-1 text-xs font-normal text-sky-600 dark:text-sky-400">
              <span class="inline-block size-3 animate-spin rounded-full border-2 border-current border-t-transparent"></span>
              更新中…
            </span>
          {/if}
        </h2>

        {#if data.items.length > 0}
          <div class="flex flex-wrap items-center gap-2">
            <input
              class="input max-w-md"
              type="search"
              placeholder="快速筛选：名称 / 职业 / 简介 / 共同作品…（支持拼音首字母）"
              title="支持中文或拼音首字母，如 dy→导演"
              bind:value={filter}
            />
            {#if filter.trim()}
              <button class="btn-mini" type="button" onclick={() => (filter = '')}>清除</button>
            {/if}
            <span class="text-xs text-neutral-400">范围：当前页 · 名称、职业、简介、共同条目</span>
          </div>
        {/if}

        <!-- 列表区域：每次筛选/翻页数据到达后整体淡入，避免闪烁 -->
        {#key renderSeq}
        <div class="grid min-w-0 w-full gap-3" in:fade={{ duration: 150 }}>
        {#if data.items.length === 0}
          <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">
            {(selA.length > 0 || selB.length > 0) ? '该职位组合下未找到合作记录' : '未找到合作记录'}
          </div>
        {:else if visibleItems && visibleItems.length === 0}
          <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">无匹配内容</div>
        {:else}
          <div class="rounded-lg border border-neutral-200 bg-white/60 px-4 py-2 dark:border-neutral-800 dark:bg-neutral-900/60">
            <Pagination total={data.total} page={page} size={size} onchange={changePage} />
          </div>
          <div class="min-w-0 w-full space-y-3">
            {#each visibleItems as col (col.person_id)}
              <div class="rounded-lg border border-neutral-200 bg-white/60 p-3 odd:bg-white even:bg-neutral-100/60 dark:border-neutral-800 dark:bg-neutral-900/60 dark:odd:bg-neutral-900/60 dark:even:bg-neutral-900">
                <div class="flex min-w-0 w-full gap-3">
                  <PersonAvatar
                    pid={col.person_id}
                    name={col.name}
                    size="grid"
                    class="size-14 rounded-full bg-sky-100 text-xl font-bold text-sky-700 hover:bg-sky-200 dark:bg-sky-950 dark:text-sky-300 dark:hover:bg-sky-900"
                    title="查看该人物的 collaborations"
                    onclick={() => { pidInput = String(col.person_id); pidSel = null; search(col.person_id) }}
                  />
                  <div class="min-w-0 flex-1">
                    <h3 class="flex items-center gap-2">
                      <a href={`https://bgm.tv/person/${col.person_id}`} target="_blank" rel="noreferrer" class="font-medium text-sky-600 hover:underline dark:text-sky-400">{col.name}</a>
                      <small class="text-xs text-neutral-400">(x{col.count})</small>
                      {#if col.subjects.length > COLLAB_PREVIEW_N}
                        {#if !subjExpanded(col.person_id)}
                          <small class="ml-auto text-[11px] text-neutral-400">前 {COLLAB_PREVIEW_N} / {col.subjects.length} 条</small>
                        {/if}
                        <button
                          type="button"
                          class="{subjExpanded(col.person_id) ? 'ml-auto' : ''} inline-flex size-5 shrink-0 items-center justify-center rounded text-neutral-400 hover:bg-neutral-200/70 hover:text-sky-600 dark:hover:bg-neutral-800 dark:hover:text-sky-400"
                          title={subjExpanded(col.person_id) ? '收起合作条目' : `展开全部 ${col.subjects.length} 条合作条目`}
                          aria-label={subjExpanded(col.person_id) ? `收起 ${col.name} 的合作条目` : `展开 ${col.name} 的全部 ${col.subjects.length} 条合作条目`}
                          onclick={() => toggleSubj(col.person_id)}
                        >
                          <svg
                            class="size-3.5 transition-transform duration-150"
                            class:rotate-180={subjExpanded(col.person_id)}
                            viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"
                          >
                            <path d="m6 9 6 6 6-6" />
                          </svg>
                        </button>                        
                      {/if}
                    </h3>
                    <div class="mt-1 flex flex-wrap items-center gap-1">
                      <span class="chip">{col.type_name}</span>
                      {#each col.career ?? [] as cb}
                        <span class="chip">{careerCn(cb)}</span>
                      {/each}
                    </div>
                    {#if col.summary}
                      <p class="mt-1 line-clamp-2 text-xs text-neutral-500 dark:text-neutral-400">{col.summary}</p>
                    {/if}
                    <div class="truncate subject_tag_section mt-2 flex min-w-0 w-full flex-wrap gap-x-3 gap-y-1">
                      {#each shownSubjects(col) as s (s.id)}
                        <span class="inline-flex max-w-full items-baseline gap-1">
                          <a
                            href={`https://bgm.tv/subject/${s.id}`}
                            target="_blank"
                            rel="noreferrer"
                            class="truncate text-sm text-sky-600 hover:underline dark:text-sky-400"
                            title={`${s.date || '未知日期'} · ${s.type_name}${s.roles.length ? ' · ' + s.roles.join(' / ') : ''}`}
                          >{s.name_cn || s.name}</a>
                          {#if s.roles.length}
                            <small class="shrink-0 text-[11px] text-neutral-400">{s.roles.join(' / ')}</small>
                            <!--small class="w-full min-w-0 truncate text-[11px] text-neutral-400">{s.roles.join(' / ')}</small-->
                          {/if}
                        </span>
                      {/each}
                    </div>
                  </div>
                </div>
              </div>
            {/each}
          </div>
          <div class="mt-3 rounded-lg border border-neutral-200 bg-white/60 px-4 py-2 dark:border-neutral-800 dark:bg-neutral-900/60">
            <Pagination total={data.total} page={page} size={size} onchange={changePage} />
          </div>
        {/if}
        </div>
        {/key}
      </section>
      </div>
    </div>
  {/if}
</div>
