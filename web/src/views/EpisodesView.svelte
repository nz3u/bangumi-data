<script>
  import { onMount } from 'svelte'
  import { searchEpisodes } from '../lib/api.js'
  import { loadConstants, enumList, SUBJECT_AUTO_SEARCH_DEBOUNCE_MS } from '../lib/constants.js'
  import { fmtDate } from '../lib/format.js'
  import Pagination from '../components/Pagination.svelte'
  import Highlight from '../components/Highlight.svelte'
  import { openDetail } from '../lib/detail.svelte.js'
  import { onNavParams } from '../lib/nav.js'
  import { externalUrl } from '../lib/settings.svelte.js'

  const BASE = '/episodes'

  const DEFAULTS = {
    q: '',
    subjectId: '',
    type: '',
    epType: '',
    disc: '',
    airdateFrom: '',
    airdateTo: '',
    sort: '',
    order: 'asc',
    size: 30
  }
  let cons = $state(null)
  let types = $state([]) // 作品类型（所属条目）
  let epTypes = $state([]) // 章节类型（正篇/SP/OP/ED…）
  let loading = $state(false)
  let error = $state('')
  let result = $state(null)

  let f = $state({ ...DEFAULTS })

  const sortOptions = [
    { value: '', label: '默认' },
    { value: 'id', label: 'ID' },
    { value: 'sort', label: '集数' },
    { value: 'airdate', label: '播出日期' },
    { value: 'subject', label: '条目' },
    { value: 'popularity', label: '条目人气' }
  ]

  onMount(async () => {
    cons = await loadConstants()
    types = enumList(cons.subject_types)
    epTypes = enumList(cons.episode_types)
    await doSearch({ ...f, page: 1 }) // 挂载即展示第 1 页（空条件 = 全量列表）
    // 跨页跳转：条目抽屉「查看全部章节」携带条目 ID 直达
    onNavParams(BASE, (params) => {
      const sid = Number(params?.subjectId ?? 0)
      if (!sid || sid <= 0) return
      Object.assign(f, DEFAULTS, { subjectId: String(sid) })
      appliedFormSig = formSig()
      doSearch({ ...f, page: 1 })
    })
  })

  // 搜索由表单状态直接驱动：提交/翻页/重置时按当前表单发起请求。
  let autoTimer = null
  const formSig = () => JSON.stringify(f)
  let appliedFormSig = formSig()

  // 自动搜索：表单相对最近一次已执行搜索的快照有任何变更时，停顿后自动提交；
  // 手动提交会先更新快照，故不会误触发；输入回退到与快照一致则取消。
  $effect(() => {
    const sig = formSig()
    if (sig === appliedFormSig) return
    clearTimeout(autoTimer)
    autoTimer = setTimeout(() => {
      autoTimer = null
      submit()
    }, SUBJECT_AUTO_SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(autoTimer)
  })

  async function doSearch(p) {
    loading = true
    error = ''
    result = null
    try {
      result = await searchEpisodes(p)
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

  // 行点击打开所属条目的详情抽屉（章节本身无详情页）
  function openSubject(s) {
    if (!s?.id) return
    openDetail('subject', s.id, s)
  }
</script>

<div class="rise grid grid-cols-[minmax(0,1fr)] gap-4">
  <form class="grid gap-3 card p-4" onsubmit={(e) => { e.preventDefault(); submit() }}>
    <div class="grid grid-cols-[repeat(2,minmax(0,1fr))] gap-3 lg:grid-cols-[repeat(4,minmax(0,1fr))]">
      <div class="col-span-2 lg:col-span-4">
        <label class="label" for="episode-q">关键词（FTS 全文搜索，命中章节标题或所属条目标题，中文名/原名）</label>
        <input id="episode-q" class="input" type="text" placeholder="如：光るなら、少女歌剧" bind:value={f.q} />
      </div>

      <div>
        <label class="label" for="episode-subject">条目 ID</label>
        <input id="episode-subject" class="input" type="number" min="1" placeholder="如：265" bind:value={f.subjectId} />
      </div>
      <div>
        <label class="label" for="episode-type">作品类型</label>
        <select id="episode-type" class="input" bind:value={f.type}>
          <option value="">全部</option>
          {#each types as t}
            <option value={t.id}>{t.name}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="episode-ep-type">章节类型</label>
        <select id="episode-ep-type" class="input" bind:value={f.epType}>
          <option value="">全部</option>
          {#each epTypes as t}
            <option value={t.id}>{t.name}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="episode-disc">光盘</label>
        <input id="episode-disc" class="input" type="number" min="0" placeholder="如：1" bind:value={f.disc} />
      </div>

      <div>
        <label class="label" for="episode-airdate-from" title="播出时间为原文文本，范围匹配对 ISO 日期格式有效">播出时间从</label>
        <input id="episode-airdate-from" class="input" type="date" bind:value={f.airdateFrom} />
      </div>
      <div>
        <label class="label" for="episode-airdate-to">播出时间到</label>
        <input id="episode-airdate-to" class="input" type="date" bind:value={f.airdateTo} />
      </div>
      <div>
        <label class="label" for="episode-sort">排序</label>
        <select id="episode-sort" class="input" bind:value={f.sort}>
          {#each sortOptions as o}
            <option
              value={o.value}
              title={o.value === '' ? '搜索时按匹配分级与条目人气排序，浏览时按 ID 排序' : undefined}
            >
              {o.label}
            </option>
          {/each}
        </select>
      </div>
      <div>
        <label class="label" for="episode-order">顺序</label>
        <select id="episode-order" class="input" bind:value={f.order}>
          <option value="asc">升序</option>
          <option value="desc">降序</option>
        </select>
      </div>
    </div>

    <div class="flex items-center gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '搜索中…' : '搜索'}</button>
      <button class="btn-ghost" type="button" onclick={resetForm}>重置</button>
      <span class="ml-auto text-xs text-neutral-500">GET /api/episodes/search</span>
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
            <tr class="whitespace-nowrap">
              <th>ID</th>
              <th>章节类型</th>
              <th>集数</th>
              <th>中文名</th>
              <th>原名</th>
              <th>播出时间</th>
              <th>时长</th>
              <th>光盘</th>
              <th>所属条目</th>
              <th>简介</th>
            </tr>
          </thead>
          <tbody class="stagger">
            {#each result.items as it (it.id)}
              <tr
                class="cursor-pointer transition-colors hover:bg-sakura-50/70 dark:hover:bg-white/[0.04]"
                title={it.subject?.id ? '查看所属条目详情' : undefined}
                onclick={() => openSubject(it.subject)}
              >
                <td class="text-neutral-500">{it.id}</td>
                <td class="whitespace-nowrap">{#if it.type}{it.type_name}{:else}正篇{/if}</td>
                <td class="tabular-nums">{it.sort || '—'}</td>
                <td class="max-w-52 truncate">
                  {#if it.name_cn}
                    <Highlight text={it.name_cn} q={f.q} scope="title" />
                  {:else}
                    —
                  {/if}
                </td>
                <td class="max-w-52 truncate text-neutral-500 dark:text-neutral-400">
                  {#if it.name}
                    <Highlight text={it.name} q={f.q} scope="title" />
                  {:else}
                    —
                  {/if}
                </td>
                <td class="whitespace-nowrap">{fmtDate(it.airdate)}</td>
                <td class="text-neutral-500 dark:text-neutral-400">{it.duration || '—'}</td>
                <td class="tabular-nums">{it.disc > 0 ? it.disc : '—'}</td>
                <td class="max-w-64">
                  {#if it.subject?.id}
                    <div class="flex items-baseline gap-1">
                      <a
                        href={externalUrl('subject', it.subject.id)}
                        target="_blank"
                        rel="noreferrer"
                        class="min-w-0 truncate text-sakura-600 hover:underline dark:text-sakura-400"
                        onclick={(e) => e.stopPropagation()}
                      >
                        <Highlight text={it.subject.name_cn || it.subject.name} q={f.q} scope="title" />
                      </a>
                      <span class="shrink-0 text-xs text-neutral-400 dark:text-neutral-500">{it.subject.type_name}{#if it.subject.platform_name} · {it.subject.platform_name}{/if}</span>
                    </div>
                  {:else}
                    —
                  {/if}
                </td>
                <td class="max-w-80 truncate text-xs text-neutral-500 dark:text-neutral-400" title={it.description}>{it.description || '—'}</td>
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
