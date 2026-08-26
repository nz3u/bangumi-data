<script>
  import { closeDetail, openDetail } from '../lib/detail.svelte.js'
  import { goToTab } from '../lib/nav.js'
  import { externalUrl } from '../lib/settings.svelte.js'
  import EntityPic from './EntityPic.svelte'
  import InfoboxList from './InfoboxList.svelte'

  let { d, id } = $props()

  // 「双人合作作品」选中的合作人物 ID（单选）；切换人物时自动清空
  let picked = $state(null)

  $effect(() => {
    void id
    picked = null
  })
</script>

<div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4" data-sec-label="信息">
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
  <EntityPic kind="person" {id} href={externalUrl('person', id)} alt={d.name_cn || d.name} class="max-w-40" />
</div>

<InfoboxList fields={d.infobox ?? []} />

<section class="mb-4" data-sec-label="简介">
  <div class="divider-short"></div>
  <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
  <p class="whitespace-pre-wrap text-sm leading-relaxed">{d.summary || '（无简介）'}</p>
</section>

{#if (d.collaborators ?? []).length > 0}
  <section class="mb-4" data-sec-label="关联人物 / 角色">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">
      关联人物 / 角色（{d.collaborators_total ?? d.collaborators.length}）
    </h4>
    <ul class="space-y-0.5 text-sm">
      {#each d.collaborators as c (c.person_id)}
        <li class="flex items-center gap-1.5 text-neutral-700 dark:text-neutral-300">
          <input
            type="radio"
            name="collab-pick"
            class="size-3 shrink-0 cursor-pointer accent-sky-600 dark:accent-sky-400"
            checked={picked === c.person_id}
            onchange={() => (picked = c.person_id)}
            title={`选中后可查看与 ${c.name} 的双人合作作品`}
            aria-label={`选择 ${c.name} 查看双人合作作品`}
          />
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('person', c.person_id)}>{c.name}</button>
          <small class="text-xs text-neutral-400">共同作品 x{c.count}</small>
        </li>
      {/each}
    </ul>
    <!-- 跨标签页跳转：内部传参（不写入地址栏），由合作页/双人合作页消费。
         注意：id/picked/d 的取值都必须先于 closeDetail() 捕获——
         它们派生自 detailDrawer/view，closeDetail 后立即变 null，再读会抛错并中断跳转。 -->
    <div class="mt-1 flex flex-col items-start gap-0.5">
      <button
        type="button"
        class="inline-block cursor-pointer text-xs text-sky-600 hover:underline dark:text-sky-400"
        onclick={() => { const pid = id; closeDetail(); goToTab('/collaborations', { id: pid }) }}
      >
        查看全部合作人物 →
      </button>
      {#if picked != null && picked !== id}
        <button
          type="button"
          class="inline-block cursor-pointer text-xs text-sky-600 hover:underline dark:text-sky-400"
          onclick={() => {
            const a = id
            const b = picked
            const aName = d.name_cn || d.name
            const bName = d.collaborators.find((c) => c.person_id === b)?.name
            closeDetail()
            goToTab('/pairworks', { a, b, aName, bName })
          }}
        >
          查看双人合作作品 →
        </button>
      {/if}
    </div>
  </section>
{/if}
