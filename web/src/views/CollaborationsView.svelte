<script>
  import { getPersonCollaboration } from '../lib/api.js'
  import Pagination from '../components/Pagination.svelte'

  let pidInput = $state('')
  let loading = $state(false)
  let error = $state('')
  let data = $state(null)
  let page = $state(1)
  const size = 20

  // 支持直接输入数字 ID，或粘贴 /person/596 之类的链接
  function extractId(v) {
    const m = String(v ?? '').trim().match(/\d+/)
    return m ? Number(m[0]) : NaN
  }

  async function load(pid, p = 1) {
    loading = true
    error = ''
    data = null
    try {
      data = await getPersonCollaboration(pid, { page: p, size })
      page = p
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function submit(e) {
    e.preventDefault()
    const pid = extractId(pidInput)
    if (!pid || pid <= 0) {
      error = '请输入人物 ID（数字）'
      data = null
      return
    }
    load(pid)
  }

  function changePage(p) {
    if (!data?.person?.id) return
    load(data.person.id, p)
  }
</script>

<div class="grid gap-4">
  <form class="grid gap-3 rounded-lg border border-neutral-200 bg-white/60 p-4 lg:grid-cols-[1fr_auto] lg:items-end dark:border-neutral-800 dark:bg-neutral-900/60" onsubmit={submit}>
    <div>
      <label class="label" for="collab-pid">人物 ID</label>
      <input id="collab-pid" class="input" type="text" placeholder="如：7906 或粘贴 https://bgm.tv/person/7906" bind:value={pidInput} />
    </div>
    <div class="flex items-center gap-2">
      <button class="btn" type="submit" disabled={loading}>{loading ? '查询中…' : '查询'}</button>
      <span class="text-xs text-neutral-500">GET /api/persons/:id/collaboration</span>
    </div>
  </form>

  {#if error}
    <div class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-900 dark:bg-red-950/50 dark:text-red-400">请求失败：{error}</div>
  {/if}

  {#if loading}
    <div class="py-8 text-center text-sm text-neutral-500">加载中…</div>
  {:else if data}
    <div class="grid items-start gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
      <!-- 左侧：人物简介 -->
      <aside class="rounded-lg border border-neutral-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-900/60">
        <div class="mb-3 flex flex-col items-center">
          <div class="flex size-32 items-center justify-center rounded-lg bg-neutral-200 text-4xl font-bold text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
            {(data.person.name || '?').slice(0, 1)}
          </div>
          <h2 class="mt-2 text-lg font-bold">{data.person.name}</h2>
          <div class="mt-1 flex flex-wrap justify-center gap-1">
            <span class="chip">{data.person.type_name}</span>
            {#each data.person.career ?? [] as cb}
              <span class="chip">{cb}</span>
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

      <!-- 右侧：合作人物列表（按共同条目数倒序） -->
      <section class="min-w-0">
        <h2 class="subtitle mb-3 text-base font-semibold">与「{data.person.name}」合作的人物（{data.total}）</h2>

        {#if data.items.length === 0}
          <div class="rounded-lg border border-neutral-200 bg-white/60 py-8 text-center text-sm text-neutral-500 dark:border-neutral-800 dark:bg-neutral-900/60">未找到合作记录</div>
        {:else}
          <div class="rounded-lg border border-neutral-200 bg-white/60 px-4 py-2 dark:border-neutral-800 dark:bg-neutral-900/60">
            <Pagination total={data.total} page={page} size={size} onchange={changePage} />
          </div>
          <div class="mt-3 space-y-3">
            {#each data.items as col (col.person_id)}
              <div class="rounded-lg border border-neutral-200 bg-white/60 p-3 odd:bg-white even:bg-neutral-100/60 dark:border-neutral-800 dark:bg-neutral-900/60 dark:odd:bg-neutral-900/60 dark:even:bg-neutral-900">
                <div class="flex gap-3">
                  <button
                    class="flex size-14 shrink-0 items-center justify-center rounded-full bg-sky-100 text-xl font-bold text-sky-700 hover:bg-sky-200 dark:bg-sky-950 dark:text-sky-300 dark:hover:bg-sky-900"
                    title="查看该人物的 collaborations"
                    onclick={() => load(col.person_id)}
                  >
                    {(col.name || '?').slice(0, 1)}
                  </button>
                  <div class="min-w-0 flex-1">
                    <h3 class="flex items-baseline gap-2">
                      <a href={`https://bgm.tv/person/${col.person_id}`} target="_blank" rel="noreferrer" class="font-medium text-sky-600 hover:underline dark:text-sky-400">{col.name}</a>
                      <small class="text-xs text-neutral-400">(x{col.count})</small>
                    </h3>
                    <div class="mt-1 flex flex-wrap items-center gap-1">
                      <span class="chip">{col.type_name}</span>
                      {#each col.career ?? [] as cb}
                        <span class="chip">{cb}</span>
                      {/each}
                    </div>
                    {#if col.summary}
                      <p class="mt-1 line-clamp-2 text-xs text-neutral-500 dark:text-neutral-400">{col.summary}</p>
                    {/if}
                    <div class="subject_tag_section mt-2 flex flex-wrap gap-x-3 gap-y-1">
                      {#each col.subjects as s (s.id)}
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
      </section>
    </div>
  {/if}
</div>
