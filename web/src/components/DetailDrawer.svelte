<script>
  import { navigate } from 'svelte5-router'
  import { getSubject, getPerson, getCharacter, getPersonCollaborators } from '../lib/api.js'
  import { fmtScore, fmtRank, fmtDate, fmtFavorite, careerCn } from '../lib/format.js'
  import Drawer from './Drawer.svelte'
  import EntityPic from './EntityPic.svelte'
  import { detailDrawer, closeDetail, detailHref, peekBrief } from '../lib/detail.svelte.js'

  // 全局唯一详情抽屉：依据锚点（#/subject@1 等）加载并渲染对应实体详情，
  // 锚点变化时原地替换内容；内部实体链接统一走锚点跳转。
  const LABELS = { subject: '条目详情', person: '人物详情', character: '角色详情' }
  const FETCHERS = { subject: getSubject, person: getPerson, character: getCharacter }
  const COLLAB_SIZE = 10

  let kind = $derived(detailDrawer.kind)
  let id = $derived(detailDrawer.id)

  let data = $state(null) // { kind, id, error? | detail? }
  let reqSeq = 0

  // 人物详情额外附带前 N 条合作人物（人物合作接口；失败不阻塞主体内容）
  async function loadEntity(k, i) {
    const detail = await FETCHERS[k](i)
    if (k === 'person') {
      try {
        const c = await getPersonCollaborators(i, { page: 1, size: COLLAB_SIZE })
        detail.collaborators = c.items ?? []
        detail.collaborators_total = c.total ?? 0
      } catch {
        detail.collaborators = []
        detail.collaborators_total = 0
      }
    }
    return detail
  }

  // 实体变化时重新加载详情，过期响应直接丢弃。
  // data 必须记录自身所属的 kind/id：跨类别切换时（effect 在 DOM 刷新后才执行）
  // 模板会先以新 kind 渲染一次，若沿用旧类别数据会因字段不一致而报错卡死。
  $effect(() => {
    if (!kind || !id) {
      data = null
      return
    }
    const seq = ++reqSeq
    data = null
    loadEntity(kind, id)
      .then((detail) => {
        if (seq === reqSeq) data = { kind, id, detail }
      })
      .catch((e) => {
        if (seq === reqSeq) data = { kind, id, error: e.message }
      })
  })

  // 仅当数据确实属于当前实体时才可用于渲染
  let view = $derived(data && data.kind === kind && data.id === id ? data : null)

  // 头部展示对象：详情就绪前回退到列表行缓存的基础信息
  let head = $derived(view?.detail ?? peekBrief(kind, id))
</script>

<Drawer open={!!kind} label={LABELS[kind] ?? '详情'} onclose={closeDetail}>
  {#if kind && id}
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-neutral-200 px-4 py-3 dark:border-neutral-800">
      <h3 class="min-w-0 max-w-full truncate text-base font-semibold">
        <!-- 唯一保留的外链：指向 bgm.tv 的标题 -->
        <a href={`https://bgm.tv/${kind}/${id}`} target="_blank" rel="noreferrer" class="text-sky-600 hover:underline dark:text-sky-400">
          {head ? head.name_cn || head.name : `${LABELS[kind] ?? ''}加载中…`}
        </a>
      </h3>
      {#if kind === 'subject'}
        {#if head?.type_name}<span class="chip">{head.type_name}</span>{/if}
        {#if head?.platform_name}<span class="chip">{head.platform_name}</span>{/if}
        {#if data?.detail?.series}<span class="chip">系列</span>{/if}
        {#if head?.nsfw}<span class="chip text-red-600 dark:text-red-400">R18</span>{/if}
      {:else if kind === 'person'}
        {#if head?.type_name}<span class="chip">{head.type_name}</span>{/if}
        {#each head?.career ?? [] as c}
          <span class="chip">{careerCn(c)}</span>
        {/each}
      {:else if head?.role_name}
        <span class="chip">{head.role_name}</span>
      {/if}
      <button class="btn-mini ml-auto shrink-0" onclick={closeDetail}>关闭（Esc）</button>
    </div>

    <div class="flex-1 overflow-y-auto px-4 py-3">
      {#if !view}
        <div class="py-8 text-center text-sm text-neutral-500">加载详情…</div>
      {:else if view.error}
        <p class="text-sm text-red-600 dark:text-red-400">详情加载失败：{view.error}</p>
      {:else if kind === 'subject'}
        {@const d = view.detail}
        <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4">
          <section class="mb-4 min-w-40 flex-[1_1_16rem]">
            <dl class="grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-4">
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
                <span class="chip">{m}</span>
              {/each}
            </div>
          </section>
          <EntityPic kind="subject" id={id} href={`https://bgm.tv/subject/${id}`} alt={d.name_cn || d.name} class="max-w-40" />
        </div>

        <section class="mb-4">
          <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
          <p class="whitespace-pre-wrap text-sm leading-relaxed">{d.summary || '（无简介）'}</p>
        </section>

        {#if d.relations.length}
          <section class="mb-4">
            <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">关联（{d.relations.length}）</h4>
            <ul class="space-y-0.5 text-sm">
              {#each d.relations ?? [] as r}
                <li class="text-neutral-700 dark:text-neutral-300">
                  <span class="text-neutral-500">{r.relation_name}</span> →
                  <span class="text-neutral-500">{r.related_type_name}</span>
                  <a href={detailHref('subject', r.related_subject_id)} class="hover:underline">{r.related_name_cn || r.related_name}</a>
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if d.staff.length}
          <section class="mb-4">
            <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">制作人员（{d.staff.length}）</h4>
            <ul class="space-y-0.5 text-sm">
              {#each d.staff ?? [] as s}
                <li class="text-neutral-700 dark:text-neutral-300">
                  <span class="text-neutral-500">{s.position_name}</span>
                  <a href={detailHref('person', s.person_id)} class="hover:underline">{s.person_name}</a>
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if d.characters.length}
          <section>
            <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">角色（{d.characters.length}）</h4>
            <ul class="space-y-0.5 text-sm">
              {#each d.characters ?? [] as c}
                <li class="text-neutral-700 dark:text-neutral-300">
                  <span class="text-neutral-500">{c.role_name}</span>
                  <a href={detailHref('character', c.id)} class="hover:underline">{c.name}</a>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
      {:else if kind === 'person'}
        {@const d = view.detail}
        <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4">
          <section class="mb-4 min-w-40 flex-[1_1_16rem]">
            <dl class="grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-5">
              <div><dt class="label">ID</dt><dd class="text-neutral-500">{d.id}</dd></div>
              <div><dt class="label">参与作品</dt><dd>{d.works_count || '—'}</dd></div>
              <div><dt class="label">出演角色</dt><dd>{d.roles_count || '—'}</dd></div>
              <div><dt class="label">评论</dt><dd>{d.comments}</dd></div>
              <div><dt class="label">收藏</dt><dd>{d.collects}</dd></div>
            </dl>
            <p class="mt-1.5 truncate text-sm text-neutral-500 dark:text-neutral-400" title={d.name}>原名：{d.name}</p>
          </section>
          <EntityPic kind="person" id={id} href={`https://bgm.tv/person/${id}`} alt={d.name_cn || d.name} class="max-w-40" />
        </div>

        <section class="mb-4">
          <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
          <p class="whitespace-pre-wrap text-sm leading-relaxed">{d.summary || '（无简介）'}</p>
        </section>

        {#if (d.collaborators ?? []).length > 0}
          <section class="mb-4">
            <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">
              关联人物 / 角色（{d.collaborators_total ?? d.collaborators.length}）
            </h4>
            <ul class="space-y-0.5 text-sm">
              {#each d.collaborators as c (c.person_id)}
                <li class="text-neutral-700 dark:text-neutral-300">
                  <a href={detailHref('person', c.person_id)} class="hover:underline">{c.name}</a>
                  <small class="text-xs text-neutral-400">共同作品 x{c.count}</small>
                </li>
              {/each}
            </ul>
            <a
              href={`/collaborations?id=${id}`}
              class="mt-1 inline-block text-xs text-sky-600 hover:underline dark:text-sky-400"
              onclick={(e) => { e.preventDefault(); closeDetail(); navigate(`/collaborations?pid=${id}`) }}
            >
              查看全部合作人物 →
            </a>
          </section>
        {/if}
      {:else}
        {@const d = view.detail}
        <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4">
          <section class="mb-4 min-w-40 flex-[1_1_16rem]">
            <dl class="grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-3">
              <div><dt class="label">ID</dt><dd class="text-neutral-500">{d.id}</dd></div>
              <div><dt class="label">收藏</dt><dd>{d.collects}</dd></div>
              <div><dt class="label">评论</dt><dd>{d.comments}</dd></div>
              <div><dt class="label">出演作品数</dt><dd>{d.subjects.length || '—'}</dd></div>
            </dl>
            <p class="mt-1.5 truncate text-sm text-neutral-500 dark:text-neutral-400" title={d.name}>原名：{d.name}</p>
          </section>
          <EntityPic kind="character" id={id} href={`https://bgm.tv/character/${id}`} alt={d.name_cn || d.name} class="max-w-40" />
        </div>

        <section class="mb-4">
          <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
          <p class="whitespace-pre-wrap text-sm leading-relaxed">{d.summary || '（无简介）'}</p>
        </section>

        {#if d.relations.length}
          <section class="mb-4">
            <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">关联角色（{d.relations.length}）</h4>
            <ul class="space-y-0.5 text-sm">
              {#each d.relations ?? [] as r}
                <li class="text-neutral-700 dark:text-neutral-300">
                  <span class="text-neutral-500">{r.relation_name}</span>
                  <a href={detailHref('character', r.related_person_id)} class="hover:underline">{r.related_name || r.related_person_id}</a>
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if d.subjects.length}
          <section class="mb-4">
            <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">出演作品（{d.subjects.length}）</h4>
            <ul class="space-y-0.5 text-sm">
              {#each d.subjects ?? [] as s}
                <li class="text-neutral-700 dark:text-neutral-300">
                  <span class="text-neutral-500">{s.date || '—'}</span>
                  <a href={detailHref('subject', s.subject_id)} class="hover:underline">{s.name_cn || s.name}</a>
                  {#if s.score > 0}<span class="text-amber-600 dark:text-amber-400">{fmtScore(s.score)}</span>{/if}
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if d.cvs.length}
          <section>
            <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">声优 / 演员（{d.cvs.length}）</h4>
            <ul class="space-y-0.5 text-sm">
              {#each d.cvs ?? [] as c}
                <li class="text-neutral-700 dark:text-neutral-300">
                  <a href={detailHref('person', c.person_id)} class="hover:underline">{c.name}</a>
                  {#if c.type_name}<span class="chip">{c.type_name}</span>{/if}
                </li>
              {/each}
            </ul>
          </section>
        {/if}
      {/if}
    </div>
  {/if}
</Drawer>
