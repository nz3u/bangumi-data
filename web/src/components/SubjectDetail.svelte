<script>
  import { onMount } from 'svelte'
  import { fmtScore, fmtRank, fmtDate, fmtFavorite } from '../lib/format.js'
  import { getSubjectEpisodes } from '../lib/api.js'
  import { loadConstants } from '../lib/constants.js'
  import { openDetail, closeDetail } from '../lib/detail.svelte.js'
  import { goToTab } from '../lib/nav.js'
  import { externalUrl } from '../lib/settings.svelte.js'
  import EntityPic from './EntityPic.svelte'

  let { d, id } = $props()

  // ---- 章节区块 ----
  // 有关章节的条目（动画/电视剧/音乐碟轨等）联合查询按类型+集数排序展示；
  // 游戏/电影等无章节的条目不渲染该区块。长列表只取前 EPS_LIMIT 条，
  // 其余引导到「章节搜索」页（已按条目 ID 预填）。
  const EPS_LIMIT = 200

  let eps = $state(null) // null = 加载中；[] = 加载结束（含失败）
  let epTypes = $state({})

  $effect(() => {
    void id
    eps = null
    if (!d?.episode_count) return
    getSubjectEpisodes(id, { size: EPS_LIMIT, sort: 'type' })
      .then((r) => (eps = r.items ?? []))
      .catch(() => (eps = []))
  })

  onMount(async () => {
    const c = await loadConstants().catch(() => null)
    if (c) epTypes = c.episode_types ?? {}
  })

  // 章节类型分组（0正篇 1SP 2OP 3ED 4Trailer 5MAD 6其他），仅返回到的条目参与分组；
  // 只有正篇时平铺展示，否则按类型分组加小标题。
  // 连续的空章节（无标题且无日期/时长/简介）压缩为一条区间行（如「14–16（无标题 ×3）」），
  // 避免海螺小姐这类长篇条目的空章节把抽屉占满；带任一内容的无标题章节仍单独成行。
  const epGroups = $derived.by(() => {
    if (!eps?.length) return []
    const map = new Map()
    for (const e of eps) {
      if (!map.has(e.type)) map.set(e.type, [])
      map.get(e.type).push(e)
    }
    return [...map.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([type, list]) => ({ type, segments: compressBlankEps(list), count: list.length }))
  })

  function isBlankEp(e) {
    return !e.name && !e.name_cn && !e.description && !e.airdate && !e.duration
  }

  // 顺序扫描展示列表，把连续空章节合并为 { kind: 'run', items } 段
  function compressBlankEps(list) {
    const out = []
    for (const e of list) {
      if (isBlankEp(e)) {
        const last = out[out.length - 1]
        if (last?.kind === 'run') last.items.push(e)
        else out.push({ kind: 'run', items: [e] })
      } else {
        out.push({ kind: 'ep', ep: e })
      }
    }
    return out
  }

  function runLabel(items) {
    const fmt = (s) => (Number.isInteger(s) ? String(s) : s.toFixed(1))
    const first = items[0].sort
    const last = items[items.length - 1].sort
    if (items.length === 1) return `${fmt(first)}（无标题）`
    if (first === last) return `无标题 ×${items.length}`
    return `${fmt(first)}–${fmt(last)}（无标题 ×${items.length}）`
  }

  function epTypeLabel(t) {
    return epTypes[t] ?? (t === 0 ? '正篇' : `类型 ${t}`)
  }

  // 集数显示：整数不带小数点，非整数（如 1.5 话）保留一位小数
  function epSortLabel(s) {
    if (!s) return ''
    return Number.isInteger(s) ? String(s) : s.toFixed(1)
  }

  function openEpisodesTab() {
    const sid = id
    closeDetail()
    goToTab('/episodes', { subjectId: sid })
  }
</script>

<div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4" data-sec-label="信息">
  <section class="mb-4 min-w-40 flex-[1_1_16rem]">
    <dl class="grid grid-cols-[repeat(2,minmax(0,1fr))] gap-x-6 gap-y-1.5 text-sm sm:grid-cols-4">
      <div><dt class="label">ID</dt><dd class="text-neutral-500">{d.id}</dd></div>
      <div><dt class="label">日期</dt><dd>{fmtDate(d.date)}</dd></div>
      <div><dt class="label">评分</dt><dd class="text-amber-600 dark:text-amber-400">{fmtScore(d.score)}</dd></div>
      <div><dt class="label">排名</dt><dd>{fmtRank(d.rank)}</dd></div>
      <div><dt class="label">收藏</dt><dd>{fmtFavorite(d.favorite)}</dd></div>
      <div><dt class="label">集数</dt><dd>{d.episode_count || '—'}</dd></div>
    </dl>
    <p class="mt-1.5 truncate text-sm text-neutral-500 dark:text-neutral-400" title={d.name}>原名：{d.name}</p>
    <div class="mt-1.5 flex flex-wrap items-center gap-1">
      <span class="text-xs text-neutral-500 dark:text-neutral-400">标签：</span>
      {#each d.tags ?? [] as t}
        <span class="chip">{t.name}</span>
      {/each}
    </div>
    <div class="mt-1.5 flex flex-wrap items-center gap-1">
      <span class="text-xs text-neutral-500 dark:text-neutral-400">Meta 标签：</span>
      {#each d.meta_tags ?? [] as m}
        <span class="chip-meta">{m}</span>
      {/each}
    </div>
  </section>
  <EntityPic kind="subject" {id} href={externalUrl('subject', id)} alt={d.name_cn || d.name} class="max-w-40" />
</div>

<section class="mb-4" data-sec-label="简介">
  <div class="divider-short"></div>
  <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
  <p class="whitespace-pre-wrap text-sm leading-relaxed">{d.summary || '（无简介）'}</p>
</section>

{#if d.episode_count > 0}
  {#snippet epSegmentList(segments)}
    <ul class="space-y-0.5 text-sm">
      {#each segments as seg (seg.kind === 'ep' ? seg.ep.id : `run-${seg.items[0].id}`)}
        {#if seg.kind === 'ep'}
          {@const e = seg.ep}
          <li class="text-neutral-700 dark:text-neutral-300" title={e.description}>
            {#if epSortLabel(e.sort)}<span class="mr-1 text-neutral-400 tabular-nums">{epSortLabel(e.sort)}.</span>{/if}
            <span class="text-neutral-500">{e.name_cn || e.name || '（无标题）'}</span>
            {#if e.name_cn && e.name}<small class="ml-1 text-xs text-neutral-400 dark:text-neutral-500">{e.name}</small>{/if}
            {#if e.airdate}<small class="ml-1.5 text-xs text-neutral-400 dark:text-neutral-500">{e.airdate}</small>{/if}
            {#if e.duration}<small class="ml-1.5 text-xs text-neutral-400 dark:text-neutral-500">{e.duration}</small>{/if}
          </li>
        {:else}
          <li class="text-neutral-400 dark:text-neutral-500" title="连续无标题章节（无日期/时长/简介）">
            {runLabel(seg.items)}
          </li>
        {/if}
      {/each}
    </ul>
  {/snippet}

  <section class="mb-4" data-sec-label="章节">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">章节（{d.episode_count}）</h4>
    {#if eps === null}
      <div class="space-y-1.5 py-1">
        {#each Array.from({ length: 5 }) as _, i}
          <div class="skeleton h-4" style:width="{92 - (i % 3) * 12}%"></div>
        {/each}
      </div>
    {:else if eps.length === 0}
      <p class="text-sm text-neutral-500">章节列表加载失败</p>
    {:else}
      {#if epGroups.length === 1 && epGroups[0].type === 0}
        {@render epSegmentList(epGroups[0].segments)}
      {:else}
        {#each epGroups as g (g.type)}
          <div class="mt-2">
            <h5 class="mb-0.5 text-xs text-neutral-400 dark:text-neutral-500">{epTypeLabel(g.type)}（{g.count}）</h5>
            {@render epSegmentList(g.segments)}
          </div>
        {/each}
      {/if}
      {#if d.episode_count > eps.length}
        <button
          type="button"
          class="mt-1.5 inline-block cursor-pointer text-xs text-sakura-600 hover:underline dark:text-sakura-400"
          onclick={openEpisodesTab}
        >已显示前 {eps.length} 条，查看全部 {d.episode_count} 条章节 →</button>
      {/if}
    {/if}
  </section>
{/if}

{#if d.relations.length}
  <section class="mb-4" data-sec-label="关联">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">关联（{d.relations.length}）</h4>
    <ul class="space-y-0.5 text-sm">
      {#each d.relations ?? [] as r}
        <li class="text-neutral-700 dark:text-neutral-300">
          <span class="text-neutral-500">{r.relation_name}</span> →
          <span class="text-neutral-500">{r.related_type_name}</span>
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('subject', r.related_subject_id)}>{r.related_name_cn || r.related_name}</button>
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if d.staff.length}
  <section class="mb-4" data-sec-label="制作人员">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">制作人员（{d.staff.length}）</h4>
    <ul class="space-y-0.5 text-sm">
      {#each d.staff ?? [] as s}
        <li class="text-neutral-700 dark:text-neutral-300">
          <span class="text-neutral-500">{s.position_name}</span>
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('person', s.person_id)}>{s.person_name}</button>
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if d.characters.length}
  <section data-sec-label="角色">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">角色（{d.characters.length}）</h4>
    <ul class="space-y-0.5 text-sm">
      {#each d.characters ?? [] as c}
        <li class="text-neutral-700 dark:text-neutral-300">
          <span class="text-neutral-500">{c.role_name}</span>
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('character', c.id)}>{c.name}</button>
        </li>
      {/each}
    </ul>
  </section>
{/if}
