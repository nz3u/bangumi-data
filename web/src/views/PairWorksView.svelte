<script>
  import { onMount } from 'svelte'
  import { getPairCollaboration } from '../lib/api.js'
  import { careerCn } from '../lib/format.js'
  import { onNavParams } from '../lib/nav.js'
  import PinyinMatch from 'pinyin-match'
 import Highlight from '../components/Highlight.svelte'
  import PersonSuggest from '../components/PersonSuggest.svelte'
  import PersonAvatar from '../components/PersonAvatar.svelte'
  import { openDetail } from '../lib/detail.svelte.js'

  const BASE = '/pairworks'
  let idAInput = $state('')
  let idBInput = $state('')
  let aSel = $state(null) // 搜索提示选中的人物 A（此时输入框显示名字）
  let bSel = $state(null) // 搜索提示选中的人物 B
  let loading = $state(false)
  let error = $state('')
  let data = $state(null)
  let filter = $state('')

  // ---- 职位标签筛选（页内） ----
  // 与「人物合作」页棋盘筛选同构：self = 人物 A（当前人物），other = 人物 B（合作人物）。
  // 数据一次性全量返回，标签统计与点击筛选均在前端完成，不触发额外请求。
  let selA = $state([]) // 人物 A 职位（多选 key）
  let selB = $state([]) // 人物 B 职位（多选 key）

  // 职位标签检索：同侧的悬浮轨与窄屏回退框共用一个搜索词
  let tagQA = $state('') // A 侧：当前人物职位
  let tagQB = $state('') // B 侧：合作人物职位

  // 支持直接输入数字 ID，或粘贴 /person/596 之类的链接
  function extractId(v) {
    const m = String(v ?? '').trim().match(/\d+/)
    return m ? Number(m[0]) : NaN
  }

  async function load(a, b) {
    loading = true
    error = ''
    data = null
    selA = []
    selB = []
    tagQA = ''
    tagQB = ''
    try {
      data = await getPairCollaboration(a, b)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function submit(e) {
    e.preventDefault()
    // 选中建议时组件已把真实 ID 同步到 *Sel；否则按原逻辑解析数字
    const a = aSel ?? extractId(idAInput)
    const b = bSel ?? extractId(idBInput)
    if (!a || a <= 0 || !b || b <= 0) {
      error = '请输入两个人物 ID（数字）'
      data = null
      return
    }
    if (a === b) {
      error = '两个人物 ID 不能相同'
      data = null
      return
    }
    load(a, b)
  }

  // 跨标签页内部传参：其他页面（如人物详情抽屉）跳转过来时携带双方人物 ID，
  // 自动回填输入框（带名字则显示名字）并直接查询。
  onMount(() =>
    onNavParams(BASE, (params) => {
      const a = Number(params?.a ?? 0)
      const b = Number(params?.b ?? 0)
      if (!a || a <= 0 || !b || b <= 0 || a === b) return
      aSel = a
      bSel = b
      idAInput = String(params?.aName ?? a)
      idBInput = String(params?.bName ?? b)
      load(a, b)
    })
  )

  // 职位标签统计：CV 各角色归并为一项（key 'cv'），其余以职务文本为 key，
  // count 为涉及条目数；排序与「人物合作」棋盘接口一致（按 count 倒序、同数按名称）。
  const facets = $derived.by(() => {
    if (!data || data.items.length === 0) return null
    const collect = (field) => {
      const m = new Map()
      for (const w of data.items) {
        const ks = new Set()
        for (const r of w[field] ?? []) {
          if (r.cv) ks.add('cv')
          else if (r.text) ks.add(r.text)
        }
        for (const k of ks) m.set(k, (m.get(k) ?? 0) + 1)
      }
      return [...m.entries()]
        .map(([k, count]) => ({ key: k, label: k === 'cv' ? 'CV' : k, cv: k === 'cv', count }))
        .sort((x, y) => (y.count !== x.count ? y.count - x.count : x.label.localeCompare(y.label, 'zh')))
    }
    return { self: collect('roles_a'), other: collect('roles_b') }
  })

  // 点击标签：切换选中并页内即时过滤（回到全部分组重建）
  function toggleTag(side, k) {
    if (loading) return
    if (side === 'a') selA = selA.includes(k) ? selA.filter((x) => x !== k) : [...selA, k]
    else selB = selB.includes(k) ? selB.filter((x) => x !== k) : [...selB, k]
  }

  function clearTags(side) {
    if (loading) return
    if (side === 'a' && selA.length) selA = []
    else if (side === 'b' && selB.length) selB = []
  }

  // 一键重置两组职位标签
  function clearAllTags() {
    if (loading || (!selA.length && !selB.length)) return
    selA = []
    selB = []
  }

  // 已选 key 列表转标签文本（用于已选组合展示）
  function selLabelStr(list, keys) {
    return keys.map((k) => list?.find((t) => t.key === k)?.label ?? k).join(' / ') || '不限'
  }

  // 标签检索：支持中文全文或拼音全拼/首字母匹配（如 dy→导演），空词视为全部命中
  const tagHit = (label, q) => {
    const s = String(q ?? '').trim()
    return !s || !!PinyinMatch.match(label, s)
  }
  const selfTagsShown = $derived.by(() => (facets?.self ?? []).filter((t) => tagHit(t.label, tagQA)))
  const otherTagsShown = $derived.by(() => (facets?.other ?? []).filter((t) => tagHit(t.label, tagQB)))

  const tagIdleCls =
    'rounded-full border border-neutral-300 bg-white px-2.5 py-0.5 text-xs text-neutral-600 hover:border-sky-400 hover:text-sky-600 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-300 dark:hover:border-sky-500 dark:hover:text-sky-400'
  const tagOnCls = 'rounded-full bg-sky-600 px-2.5 py-0.5 text-xs font-medium text-white hover:bg-sky-500'

  // ---- 前端按职位双向合并 ----
  // 职务总数较少的一方作为分组主轴（仅统计制作职位；若一方只有声优出演，
  // 则以另一方的职位合并展示），另一侧同职位的作品并入同组；
  // 组按作品数倒序排列，无制作职位的条目归入末尾「CV」组。
  function buildGroups(items) {
    const staffLabelsOf = (key) => {
      const s = new Set()
      for (const it of items) for (const r of it[key] ?? []) if (!r.cv) s.add(r.text)
      return s
    }
    const sa = staffLabelsOf('roles_a')
    const sb = staffLabelsOf('roles_b')
    // 主轴候选：排除纯声优一方；平票取 A。双方均纯声优时退化为 A。
    let axis = 'roles_a'
    if (sa.size > 0 && sb.size > 0) axis = sa.size <= sb.size ? 'roles_a' : 'roles_b'
    else if (sa.size === 0 && sb.size > 0) axis = 'roles_b'
    const other = axis === 'roles_a' ? 'roles_b' : 'roles_a'
    const axisPerson = axis === 'roles_a' ? data.person_a : data.person_b

    const map = new Map()
    for (const it of items) {
      const labels = new Set()
      for (const r of it[axis] ?? []) if (!r.cv) labels.add(r.text)
      for (const r of it[other] ?? []) if (!r.cv) labels.add(r.text)
      if (labels.size === 0) labels.add('__cv__')
      for (const l of labels) {
        if (!map.has(l)) map.set(l, [])
        map.get(l).push(it)
      }
    }
    let groups = [...map.entries()]
      .map(([label, works]) => ({ label, works }))
      .sort((x, y) => {
        if (y.works.length !== x.works.length) return y.works.length - x.works.length
        return x.label.localeCompare(y.label, 'zh')
      })
    groups = groups.map((g) =>
      g.label === '__cv__' ? { ...g, label: 'CV（无制作职位）' } : g
    )
    return { groups, axisPerson, otherKey: other }
  }

  function roleText(roles) {
    return (roles ?? []).map((r) => r.text).join(' / ')
  }

  // ---- 前端快速筛选 ----
  // 先做职位标签页内筛选（两侧之间取交集，侧内多选取并集），
  // 再按职位合并分组并应用快速搜索：
  // 搜索范围覆盖全部展示内容：分组名、组内职务（双方）、日期、标题（中文名/原名）、类型标签。
  // 分组名命中时保留整组，否则仅保留行内命中的作品；无命中的分组隐藏。
  // 命中判定先走原文子串，未命中再尝试拼音全拼/首字母匹配（如 dy→导演）。
  const filtered = $derived.by(() => {
    if (!data || data.items.length === 0) return null
    const sideHit = (roles, sel) =>
      sel.length === 0 || (roles ?? []).some((r) => (r.cv ? sel.includes('cv') : sel.includes(r.text)))
    let posItems = data.items
    if (selA.length > 0 || selB.length > 0) {
      posItems = data.items.filter((w) => sideHit(w.roles_a, selA) && sideHit(w.roles_b, selB))
    }
    const base = buildGroups(posItems)
    const q = filter.trim().toLowerCase()
    if (!q) return { ...base, matched: posItems.length }
    const hit = (text) =>
      String(text).toLowerCase().includes(q) || !!PinyinMatch.match(String(text), q)
    const rowText = (w) =>
      [w.name_cn, w.name, w.date, w.type_name,
        ...(w.roles_a ?? []).map((r) => r.text),
        ...(w.roles_b ?? []).map((r) => r.text)].join('\n')
    let matched = 0
    const seen = new Set()
    const groups = base.groups
      .map((g) =>
        hit(g.label)
          ? g
          : { ...g, works: g.works.filter((w) => hit(rowText(w))) }
      )
      .filter((g) => g.works.length > 0)
    for (const g of groups) {
      for (const w of g.works) {
        if (!seen.has(w.id)) {
          seen.add(w.id)
          matched++
        }
      }
    }
    return { groups, axisPerson: base.axisPerson, matched }
  })
</script>

<div class="grid grid-cols-[minmax(0,1fr)] gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-200 bg-white/60 p-4 lg:grid-cols-[1fr_1fr_auto] lg:items-end dark:border-neutral-800 dark:bg-neutral-900/60" onsubmit={submit}>
    <div>
      <label class="label" for="pair-ida">人物 A ID</label>
      <PersonSuggest inputId="pair-ida" placeholder="如：1、名字或粘贴链接" bind:text={idAInput} bind:pid={aSel} />
    </div>
    <div>
      <label class="label" for="pair-idb">人物 B ID</label>
      <PersonSuggest inputId="pair-idb" placeholder="如：5076、名字或粘贴链接" bind:text={idBInput} bind:pid={bSel} />
    </div>
    <div class="flex items-center gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '查询中…' : '查询'}</button>
      <span class="text-xs text-neutral-500">GET /api/persons/:id/collaboration/:other</span>
    </div>
  </form>

  {#if error}
    <div class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-900 dark:bg-red-950/50 dark:text-red-400">请求失败：{error}</div>
  {/if}

  {#if loading}
    <div class="py-8 text-center text-sm text-neutral-500">加载中…</div>
  {:else if data}
    <div class="relative grid grid-cols-[minmax(0,1fr)] gap-4">
      <!-- 左侧悬浮轨：当前人物（A）职位（细长列表，悬浮于页面左侧留白） -->
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
                  title={`筛选人物 A 担任「${t.label}」的共同作品`}
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

        <!-- 右侧悬浮轨：合作人物（B）职位（多选，内部滚动，悬停于整体外侧） -->
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

      <!-- 头部：双方简介 -->
      <div class="rounded-lg border border-neutral-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-900/60">
        <div class="flex flex-wrap items-center justify-center gap-4">
          {#each [data.person_a, data.person_b] as p, i}
            {#if i === 1}
              <span class="text-xl font-bold text-neutral-400">×</span>
            {/if}
            <div class="flex items-center gap-3">
              <PersonAvatar
                pid={p.id}
                name={p.name}
                size="grid"
                class="size-12 rounded-full bg-sky-100 text-lg font-bold text-sky-700 dark:bg-sky-950 dark:text-sky-300"
              />
              <div>
                <button type="button" class="cursor-pointer font-semibold text-sky-600 hover:underline dark:text-sky-400" onclick={() => openDetail('person', p.id, p)}>{p.name}</button>
                <div class="mt-0.5 flex flex-wrap gap-1">
                  <span class="chip">{p.type_name}</span>
                  {#each p.career ?? [] as cb}
                    <span class="chip">{careerCn(cb)}</span>
                  {/each}
                </div>
              </div>
            </div>
          {/each}
          <span class="chip ml-2">共同参与 {data.total} 条</span>
          {#if data.items.length > 0}
            {#if filter.trim()}
              <span class="chip ml-2 text-sky-600 dark:text-sky-400">命中 {filtered?.matched ?? 0} 条</span>
            {/if}
          {/if}
          {#if facets && (selA.length > 0 || selB.length > 0)}
            <span class="chip ml-2 text-sky-600 dark:text-sky-400">
              已选 {selLabelStr(facets.self, selA)} × {selLabelStr(facets.other, selB)}
              <button class="btn-mini ml-1" type="button" onclick={clearAllTags}>重置</button>
            </span>
          {/if}
        </div>
      </div>

      {#if data.items.length === 0}
        <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">未找到两人共同参与的作品</div>
      {:else}
        <!-- 非悬浮回退：职位标签横向排布于列表上方（达到各自阈值后改用两侧悬浮轨） -->
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
                    title={`筛选人物 A 担任「${t.label}」的共同作品`}
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

        <div class="flex items-center gap-2">
          <input
            class="input max-w-md"
            type="search"
            placeholder="快速筛选：职位 / 标题 / 日期 / 类型 / 分组名…（支持拼音首字母）"
            title="支持中文或拼音首字母，如 dy→导演"
            bind:value={filter}
          />
          {#if filter.trim()}
            <button class="btn-mini" type="button" onclick={() => (filter = '')}>清除</button>
          {/if}
          <span class="text-xs text-neutral-400">范围：分组、职务、日期、标题、类型</span>
        </div>

        {#if filtered && filtered.groups.length === 0}
          <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">
            {selA.length > 0 || selB.length > 0 ? '该职位组合下未找到共同作品' : '无匹配内容'}
          </div>
        {:else if filtered}
          <div class="text-xs text-neutral-500 dark:text-neutral-400">
            按「{filtered.axisPerson.name}」的职位合并展示（其职务较少；另一方相同职务的作品已并入对应分组），组内与分组均按倒序排列。
          </div>

          <!-- 按职位合并分组，隔行排列 -->
          <div class="space-y-3">
            {#each filtered.groups as g (g.label)}
              <section class="overflow-hidden rounded-lg border border-neutral-200 dark:border-neutral-800">
                <h3 class="border-b border-neutral-200 bg-neutral-100/80 px-4 py-2 text-sm font-semibold dark:border-neutral-800 dark:bg-neutral-800/60">
                  <Highlight text={g.label} q={filter} />
                  <small class="ml-1 font-normal text-neutral-400">(x{g.works.length})</small>
                </h3>
                <div class="divide-y divide-neutral-100 dark:divide-neutral-900">
                  {#each g.works as w, i (w.id)}
                    <div class="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 px-4 py-2 {i % 2 === 1 ? 'bg-neutral-50 dark:bg-neutral-900/40' : 'bg-white/60 dark:bg-neutral-900/60'}">
                      <span class="shrink-0 text-xs tabular-nums text-neutral-400"><Highlight text={w.date || '—'} q={filter} /></span>
                      <button type="button" onclick={() => openDetail('subject', w.id, w)} class="cursor-pointer min-w-0 truncate text-sm text-sky-600 hover:underline dark:text-sky-400"
                        title="{w.date || ''} {w.type_name}"
                      >{#if w.name_cn}<Highlight text={w.name_cn} q={filter} />{:else}<Highlight text={w.name} q={filter} />{/if}</button>
                      <span class="chip shrink-0"><Highlight text={w.type_name} q={filter} /></span>
                      <span class="ml-auto flex min-w-0 max-w-full items-center gap-x-2 text-xs text-neutral-500 dark:text-neutral-400">
                        <span class="min-w-0 max-w-full truncate leading-7"><b class="font-medium text-neutral-600 dark:text-neutral-300">{data.person_a.name}:</b> <Highlight text={roleText(w.roles_a) || '—'} q={filter} /></span>
                        <span class="min-w-0 max-w-full truncate leading-7"><b class="font-medium text-neutral-600 dark:text-neutral-300">{data.person_b.name}:</b> <Highlight text={roleText(w.roles_b) || '—'} q={filter} /></span>
                      </span>
                    </div>
                  {/each}
                </div>
              </section>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
