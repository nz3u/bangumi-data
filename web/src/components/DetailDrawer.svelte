<script>
  import { getSubject, getPerson, getCharacter, getPersonCollaborators } from '../lib/api.js'
  import { careerCn } from '../lib/format.js'
  import Drawer from './Drawer.svelte'
  import SubjectDetail from './SubjectDetail.svelte'
  import PersonDetail from './PersonDetail.svelte'
  import CharacterDetail from './CharacterDetail.svelte'
  import SectionNav from './SectionNav.svelte'
  import { detailDrawer, closeDetail, peekBrief } from '../lib/detail.svelte.js'
  import { externalUrl } from '../lib/settings.svelte.js'

  // 全局唯一详情抽屉：依据内部状态（detailDrawer）加载并渲染对应实体详情，
  // 实体切换时原地替换内容；内部实体跳转统一走 openDetail（不写入地址栏）。
  // 各实体内容渲染拆分至 SubjectDetail / PersonDetail / CharacterDetail。
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

  // 滚动容器（供左侧快速跳转导航定位与滚动）
  let scrollEl = $state(null)
</script>

<Drawer open={!!kind} label={LABELS[kind] ?? '详情'} onclose={closeDetail}>
  {#if kind && id}
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-neutral-200/80 px-4 py-3 dark:border-white/[0.06]">
      <h3 class="min-w-0 max-w-full truncate text-base font-semibold">
        <!-- 唯一保留的外链：指向设置中配置的 Bangumi 站点 -->
        <a href={externalUrl(kind, id)} target="_blank" rel="noreferrer" class="text-sakura-600 hover:underline dark:text-sakura-400">
          {head ? head.name_cn || head.name : `${LABELS[kind] ?? ''}加载中…`}
        </a>
      </h3>
      {#if kind === 'subject'}
        {#if head?.type_name}<span class="chip">{head.type_name}</span>{/if}
        {#if head?.platform_name}<span class="chip">{head.platform_name}</span>{/if}
        {#if view?.detail?.series}<span class="chip">系列</span>{/if}
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

    {#if view}
      <SectionNav container={scrollEl} />
    {/if}

    <!-- pl-8 为左侧快速跳转导航预留竖向空槽 -->
    <div bind:this={scrollEl} class="flex-1 overflow-y-auto py-3 pl-8 pr-4">
      {#if !view}
        <div class="py-8 text-center text-sm text-neutral-500">加载详情…</div>
      {:else if view.error}
        <p class="text-sm text-red-600 dark:text-red-400">详情加载失败：{view.error}</p>
      {:else if kind === 'subject'}
        <SubjectDetail d={view.detail} {id} />
      {:else if kind === 'person'}
        <PersonDetail d={view.detail} {id} />
      {:else}
        <CharacterDetail d={view.detail} {id} />
      {/if}
    </div>
  {/if}
</Drawer>
