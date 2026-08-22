<script>
  import { getPairCollaboration } from '../lib/api.js'
  import { careerCn } from '../lib/format.js'

  let idAInput = $state('')
  let idBInput = $state('')
  let loading = $state(false)
  let error = $state('')
  let data = $state(null)
  let filter = $state('')

  // 支持直接输入数字 ID，或粘贴 /person/596 之类的链接
  function extractId(v) {
    const m = String(v ?? '').trim().match(/\d+/)
    return m ? Number(m[0]) : NaN
  }

  async function load(a, b) {
    loading = true
    error = ''
    data = null
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
    const a = extractId(idAInput)
    const b = extractId(idBInput)
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
        if (x.label === '__cv__') return 1
        if (y.label === '__cv__') return -1
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
  // 搜索范围覆盖全部展示内容：分组名、组内职务（双方）、日期、标题（中文名/原名）、类型标签。
  // 分组名命中时保留整组，否则仅保留行内命中的作品；无命中的分组隐藏。
  const filtered = $derived.by(() => {
    if (!data || data.items.length === 0) return null
    const base = buildGroups(data.items)
    const q = filter.trim().toLowerCase()
    if (!q) return { ...base, matched: data.total }
    const rowText = (w) =>
      [w.name_cn, w.name, w.date, w.type_name,
        ...(w.roles_a ?? []).map((r) => r.text),
        ...(w.roles_b ?? []).map((r) => r.text)].join('\n').toLowerCase()
    let matched = 0
    const seen = new Set()
    const groups = base.groups
      .map((g) =>
        g.label.toLowerCase().includes(q)
          ? g
          : { ...g, works: g.works.filter((w) => rowText(w).includes(q)) }
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

<div class="grid gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-200 bg-white/60 p-4 lg:grid-cols-[1fr_1fr_auto] lg:items-end dark:border-neutral-800 dark:bg-neutral-900/60" onsubmit={submit}>
    <div>
      <label class="label" for="pair-ida">人物 A ID</label>
      <input id="pair-ida" class="input" type="text" placeholder="如：1 或粘贴链接" bind:value={idAInput} />
    </div>
    <div>
      <label class="label" for="pair-idb">人物 B ID</label>
      <input id="pair-idb" class="input" type="text" placeholder="如：5076 或粘贴链接" bind:value={idBInput} />
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
    <!-- 头部：双方简介 -->
    <div class="rounded-lg border border-neutral-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-900/60">
      <div class="flex flex-wrap items-center justify-center gap-4">
        {#each [data.person_a, data.person_b] as p, i}
          {#if i === 1}
            <span class="text-xl font-bold text-neutral-400">×</span>
          {/if}
          <div class="flex items-center gap-3">
            <div class="flex size-12 shrink-0 items-center justify-center rounded-full bg-sky-100 text-lg font-bold text-sky-700 dark:bg-sky-950 dark:text-sky-300">
              {(p.name || '?').slice(0, 1)}
            </div>
            <div>
              <a href={`https://bgm.tv/person/${p.id}`} target="_blank" rel="noreferrer" class="font-semibold text-sky-600 hover:underline dark:text-sky-400">{p.name}</a>
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
      </div>
    </div>

    {#if data.items.length === 0}
      <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">未找到两人共同参与的作品</div>
    {:else}
      <div class="flex items-center gap-2">
        <input
          class="input max-w-md"
          type="search"
          placeholder="快速筛选：职位 / 标题 / 日期 / 类型 / 分组名…"
          bind:value={filter}
        />
        {#if filter.trim()}
          <button class="btn-mini" type="button" onclick={() => (filter = '')}>清除</button>
        {/if}
        <span class="text-xs text-neutral-400">范围：分组、职务、日期、标题、类型</span>
      </div>

      {#if filtered && filtered.groups.length === 0}
        <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">无匹配内容</div>
      {:else if filtered}
      <div class="text-xs text-neutral-500 dark:text-neutral-400">
        按「{filtered.axisPerson.name}」的职位合并展示（其职务较少；另一方相同职务的作品已并入对应分组），组内与分组均按倒序排列。
      </div>

      <!-- 按职位合并分组，隔行排列 -->
      <div class="space-y-3">
        {#each filtered.groups as g (g.label)}
          <section class="overflow-hidden rounded-lg border border-neutral-200 dark:border-neutral-800">
            <h3 class="border-b border-neutral-200 bg-neutral-100/80 px-4 py-2 text-sm font-semibold dark:border-neutral-800 dark:bg-neutral-800/60">
              {g.label}
              <small class="ml-1 font-normal text-neutral-400">(x{g.works.length})</small>
            </h3>
            <div class="divide-y divide-neutral-100 dark:divide-neutral-900">
              {#each g.works as w, i (w.id)}
                <div class="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 px-4 py-2 {i % 2 === 1 ? 'bg-neutral-50 dark:bg-neutral-900/40' : 'bg-white/60 dark:bg-neutral-900/60'}">
                  <span class="shrink-0 text-xs tabular-nums text-neutral-400">{w.date || '—'}</span>
                  <a href={`https://bgm.tv/subject/${w.id}`} target="_blank" rel="noreferrer" class="min-w-0 truncate text-sm text-sky-600 hover:underline dark:text-sky-400"
                    title="{w.date || ''} {w.type_name}"
                  >{w.name_cn || w.name}</a>
                  <span class="chip shrink-0">{w.type_name}</span>
                  <span class="ml-auto flex min-w-0 flex-wrap gap-x-2 text-xs text-neutral-500 dark:text-neutral-400">
                    <span class="truncate"><b class="font-medium text-neutral-600 dark:text-neutral-300">{data.person_a.name}:</b> {roleText(w.roles_a) || '—'}</span>
                    <span class="truncate"><b class="font-medium text-neutral-600 dark:text-neutral-300">{data.person_b.name}:</b> {roleText(w.roles_b) || '—'}</span>
                  </span>
                </div>
              {/each}
            </div>
          </section>
        {/each}
      </div>
      {/if}
    {/if}
  {/if}
</div>
