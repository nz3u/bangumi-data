<script>
  import { closeDetail, openDetail } from '../lib/detail.svelte.js'
  import { goToTab } from '../lib/nav.js'
  import EntityPic from './EntityPic.svelte'
  import InfoboxList from './InfoboxList.svelte'

  let { d, id } = $props()
</script>

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
  <EntityPic kind="person" {id} href={`https://bgm.tv/person/${id}`} alt={d.name_cn || d.name} class="max-w-40" />
</div>

<InfoboxList fields={d.infobox ?? []} />

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
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('person', c.person_id)}>{c.name}</button>
          <small class="text-xs text-neutral-400">共同作品 x{c.count}</small>
        </li>
      {/each}
    </ul>
    <!-- 跨标签页跳转：内部传参（不写入地址栏），由合作页消费。
         注意：必须先捕获 id 再 closeDetail()——id 派生自 detailDrawer，关闭后即变 null。 -->
    <button
      type="button"
      class="mt-1 inline-block cursor-pointer text-xs text-sky-600 hover:underline dark:text-sky-400"
      onclick={() => { const pid = id; closeDetail(); goToTab('/collaborations', { id: pid }) }}
    >
      查看全部合作人物 →
    </button>
  </section>
{/if}
