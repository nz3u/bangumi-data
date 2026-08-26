<script>
  import { getPersonRoles } from '../lib/api.js'
  import { careerCn } from '../lib/format.js'
  import PinyinMatch from 'pinyin-match'
 import Highlight from '../components/Highlight.svelte'
  import PersonSuggest from '../components/PersonSuggest.svelte'
  import PersonAvatar from '../components/PersonAvatar.svelte'
  import { openDetail } from '../lib/detail.svelte.js'

  let idInput = $state('')
  let pidSel = $state(null) // 搜索提示选中的人物（此时输入框显示名字）
  let loading = $state(false)
  let error = $state('')
  let data = $state(null)
  let filter = $state('')

  // ---- 当前人物职位筛选（同「人物合作」页棋盘，纯前端页内过滤） ----
  // 数据一次性全量返回，职位统计与筛选均在前端完成，不产生额外请求。
  let selPos = $state([]) // 选中的职位标签（多选 key）
  let tagQ = $state('') // 职位标签检索词（支持中文或拼音首字母）

  // 支持直接输入数字 ID，或粘贴 /person/596 之类的链接
  function extractId(v) {
    const m = String(v ?? '').trim().match(/\d+/)
    return m ? Number(m[0]) : NaN
  }

  async function load(pid) {
    loading = true
    error = ''
    data = null
    selPos = []
    tagQ = ''
    try {
      data = await getPersonRoles(pid)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function submit(e) {
    e.preventDefault()
    // 选中建议时组件已把真实 ID 同步到 pidSel；否则按原逻辑解析数字
    const pid = pidSel ?? extractId(idInput)
    if (!pid || pid <= 0) {
      error = '请输入人物 ID（数字）'
      data = null
      return
    }
    load(pid)
  }

  // ---- 前端按职务分组 ----
  // 每个制作职位各成组，多职位的条目会在多个组中出现；
  // 无制作职位（仅声优出演）的条目归入末尾「CV」组。
  // 组按作品数倒序排列，组内按日期倒序。
  // 职位筛选生效时基于筛选后的作品重建分组：命中的作品仍按其全部职务归组，
  // 兼任多职位的作品（含 CV 与制作并存）会出现在对应各组中。
  function buildGroups(items) {
    const map = new Map()
    for (const it of items) {
      const labels = new Set()
      for (const r of it.roles ?? []) if (!r.cv) labels.add(r.text)
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
    return { groups }
  }

  function roleText(roles) {
    return (roles ?? []).map((r) => r.text).join(' / ')
  }

  // ---- 当前人物职位标签（与「人物合作」棋盘 self 侧口径一致） ----
  // 制作职位按职位名统计涉及作品数；「CV」只要有声优出演即计（可与制作职位并存，
  // 与合作页棋盘、双人合作页一致）。排序：按作品数倒序，同数按名称。
  const facets = $derived.by(() => {
    if (!data) return null
    const map = new Map()
    for (const w of data.items) {
      const labels = new Set()
      let hasCv = false
      for (const r of w.roles ?? []) {
        if (r.cv) hasCv = true
        else labels.add(r.text)
      }
      for (const l of labels) map.set(l, (map.get(l) ?? 0) + 1)
      if (hasCv) map.set('__cv__', (map.get('__cv__') ?? 0) + 1)
    }
    return [...map.entries()]
      .map(([key, count]) => ({ key, label: key === '__cv__' ? 'CV' : key, count }))
      .sort((x, y) => (y.count !== x.count ? y.count - x.count : x.label.localeCompare(y.label, 'zh')))
  })

  function togglePos(key) {
    selPos = selPos.includes(key) ? selPos.filter((k) => k !== key) : [...selPos, key]
  }

  function clearPos() {
    selPos = []
  }

  // 职位标签检索：支持中文全文或拼音全拼/首字母匹配（如 dy→导演），空词视为全部命中
  const tagHit = (label, q) => {
    const s = String(q ?? '').trim()
    return !s || !!PinyinMatch.match(label, s)
  }
  const posTagsShown = $derived.by(() => (facets ?? []).filter((t) => tagHit(t.label, tagQ)))

  // 选定职位后在作品级过滤：任一角色命中所选标签即保留；
  // 「CV」命中该作品的任一声优出演（含兼任制作职位的作品）
  const posItems = $derived.by(() => {
    if (!data || selPos.length === 0) return data?.items ?? []
    const sel = new Set(selPos)
    return data.items.filter((w) =>
      (w.roles ?? []).some((r) => (r.cv ? sel.has('__cv__') : sel.has(r.text)))
    )
  })

  const tagIdleCls =
    'rounded-full border border-neutral-300 bg-white px-2.5 py-0.5 text-xs text-neutral-600 hover:border-sky-400 hover:text-sky-600 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-300 dark:hover:border-sky-500 dark:hover:text-sky-400'
  const tagOnCls = 'rounded-full bg-sky-600 px-2.5 py-0.5 text-xs font-medium text-white hover:bg-sky-500'

  // ---- 前端快速筛选 ----
  // 搜索范围覆盖全部展示内容：分组名、职务、日期、标题（中文名/原名）、类型标签。
  // 分组名命中时保留整组，否则仅保留行内命中的作品；无命中的分组隐藏。
  // 命中判定先走原文子串，未命中再尝试拼音全拼/首字母匹配（如 dy→导演）。
  // 职位筛选生效时：先在作品级过滤（posItems），再基于筛选结果重建分组。
  const filtered = $derived.by(() => {
    if (!data || data.items.length === 0) return null
    const base = buildGroups(posItems)
    const q = filter.trim().toLowerCase()
    if (!q) return { ...base, matched: posItems.length }
    const hit = (text) =>
      String(text).toLowerCase().includes(q) || !!PinyinMatch.match(String(text), q)
    const rowTextOf = (w) =>
      [w.name_cn, w.name, w.date, w.type_name,
        ...(w.roles ?? []).map((r) => r.text)].join('\n')
    let matched = 0
    const seen = new Set()
    const groups = base.groups
      .map((g) =>
        hit(g.label)
          ? g
          : { ...g, works: g.works.filter((w) => hit(rowTextOf(w))) }
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
    return { groups, matched }
  })
</script>

<div class="grid grid-cols-[minmax(0,1fr)] gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-200 bg-white/60 p-4 lg:grid-cols-[1fr_auto] lg:items-end dark:border-neutral-800 dark:bg-neutral-900/60" onsubmit={submit}>
    <div>
      <label class="label" for="roles-pid">人物 ID</label>
      <PersonSuggest inputId="roles-pid" placeholder="如：1、名字或粘贴 https://bgm.tv/person/1" bind:text={idInput} bind:pid={pidSel} onpick={(p) => load(p.id)} />
    </div>
    <div class="flex items-center gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '查询中…' : '查询'}</button>
      <span class="text-xs text-neutral-500">GET /api/persons/:id/roles</span>
    </div>
  </form>

  {#if error}
    <div class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-900 dark:bg-red-950/50 dark:text-red-400">请求失败：{error}</div>
  {/if}

  {#if loading}
    <div class="py-8 text-center text-sm text-neutral-500">加载中…</div>
  {:else if data}
    <!-- 头部：人物简介 -->
    <div class="rounded-lg border border-neutral-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-900/60">
      <div class="flex flex-wrap items-center justify-center gap-4">
        <div class="flex items-center gap-3">
          <PersonAvatar
            pid={data.person.id}
            name={data.person.name}
            size="grid"
            class="size-12 rounded-full bg-sky-100 text-lg font-bold text-sky-700 dark:bg-sky-950 dark:text-sky-300"
          />
          <div>
            <button type="button" class="cursor-pointer font-semibold text-sky-600 hover:underline dark:text-sky-400" onclick={() => openDetail('person', data.person.id, data.person)}>{data.person.name}</button>
            <div class="mt-0.5 flex flex-wrap gap-1">
              <span class="chip">{data.person.type_name}</span>
              {#each data.person.career ?? [] as cb}
                <span class="chip">{careerCn(cb)}</span>
              {/each}
            </div>
          </div>
        </div>
        <span class="chip ml-2">参与作品 {data.total} 条</span>
        {#if data.items.length > 0 && (filter.trim() || selPos.length > 0)}
          <span class="chip ml-2 text-sky-600 dark:text-sky-400">命中 {filtered?.matched ?? 0} 条</span>
        {/if}
      </div>
    </div>

    {#if data.items.length === 0}
      <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">未找到该人物参与的作品</div>
    {:else}
      <div class="relative">
        <!-- 左侧悬浮轨：当前人物职位（同「人物合作」页，悬浮于左侧留白） -->
        {#if facets}
          <div class="absolute inset-y-0 left-0 z-10 hidden w-28 -translate-x-full pr-2 min-[1440px]:block">
            <nav
              class="sticky top-6 flex max-h-[90vh] flex-col gap-1 rounded-lg border border-neutral-200 bg-white/95 p-1.5 shadow-sm backdrop-blur dark:border-neutral-800 dark:bg-neutral-900/95"
              aria-label="当前人物职位筛选"
            >
              <div class="flex items-center justify-between gap-1 px-1 pt-0.5">
                <span class="text-[11px] font-semibold leading-tight">当前人物职位</span>
                {#if selPos.length > 0}
                  <button class="btn-mini shrink-0" type="button" onclick={clearPos}>清除</button>
                {/if}
              </div>
              <input
                class="input-xs shrink-0"
                type="search"
                placeholder="搜职位…"
                title="支持中文或拼音首字母，如 dy→导演"
                aria-label="搜索当前人物职位标签"
                bind:value={tagQ}
              />
              <div class="flex min-h-0 flex-1 flex-col items-stretch gap-1 overflow-y-auto pb-0.5">
                {#each posTagsShown as t (t.key)}
                  <button
                    type="button"
                    class={`flex w-full shrink-0 items-baseline justify-between gap-1 rounded-full px-1.5 py-0.5 text-[11px] ${selPos.includes(t.key) ? 'bg-sky-600 font-medium text-white hover:bg-sky-500' : 'border border-neutral-300 bg-white text-neutral-600 hover:border-sky-400 hover:text-sky-600 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-300 dark:hover:border-sky-500 dark:hover:text-sky-400'}`}
                    title={`筛选该人物担任「${t.label}」的作品`}
                    onclick={() => togglePos(t.key)}
                  >
                    <span class="min-w-0 truncate">{t.label}</span><small class="shrink-0 opacity-70">{t.count}</small>
                  </button>
                {:else}
                  <span class="px-1 text-xs text-neutral-400">{facets.length ? '无匹配标签' : '无可用标签'}</span>
                {/each}
              </div>
            </nav>
          </div>
        {/if}

        <div class="grid min-w-0 gap-3">
          <!-- 窄屏回退：职位标签横向排布于列表上方（1440px 及以上改用左侧悬浮轨） -->
          {#if facets}
            <div class="grid gap-y-2 rounded-lg border border-neutral-200 bg-white/60 p-3 min-[1440px]:hidden dark:border-neutral-800 dark:bg-neutral-900/60">
              <div class="min-w-0">
                <div class="mb-1 flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
                  <span>当前人物职位{selPos.length > 0 ? `（已选 ${selPos.length}）` : '（可多选）'}</span>
                  {#if selPos.length > 0}
                    <button class="btn-mini" type="button" onclick={clearPos}>清除</button>
                  {/if}
                </div>
                <input
                  class="input-xs"
                  type="search"
                  placeholder="筛选职位，支持拼音首字母，如 dy→导演"
                  aria-label="搜索当前人物职位标签"
                  bind:value={tagQ}
                />
                <div class="mt-1 flex max-h-24 flex-wrap gap-1 overflow-y-auto">
                  {#each posTagsShown as t (t.key)}
                    <button
                      type="button"
                      class={`shrink-0 ${selPos.includes(t.key) ? tagOnCls : tagIdleCls}`}
                      title={`筛选该人物担任「${t.label}」的作品`}
                      onclick={() => togglePos(t.key)}
                    >
                      {t.label}<small class="ml-0.5 opacity-70">{t.count}</small>
                    </button>
                  {:else}
                    <span class="text-xs text-neutral-400">{facets.length ? '无匹配标签' : '无可用标签'}</span>
                  {/each}
                </div>
              </div>
            </div>
          {/if}

          <div class="flex flex-wrap items-center gap-2">
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
            <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">{selPos.length > 0 ? '所选职位下未找到作品' : '无匹配内容'}</div>
          {:else if filtered}
            <!-- 按职务分组，隔行排列 -->
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
                        <span class="ml-auto flex min-w-0 flex-wrap gap-x-2 text-xs text-neutral-500 dark:text-neutral-400">
                          <span class="min-w-0 max-w-full truncate leading-7"><Highlight text={roleText(w.roles) || '—'} q={filter} /></span>
                        </span>
                      </div>
                    {/each}
                  </div>
                </section>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</div>
