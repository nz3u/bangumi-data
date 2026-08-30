<script>
  import { fmtScore } from '../lib/format.js'
  import { openDetail } from '../lib/detail.svelte.js'
  import { externalUrl } from '../lib/settings.svelte.js'
  import EntityPic from './EntityPic.svelte'
  import InfoboxList from './InfoboxList.svelte'

  let { d, id } = $props()
</script>

<div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4" data-sec-label="信息">
  <section class="mb-4 min-w-40 flex-[1_1_16rem]">
    <dl class="grid grid-cols-[repeat(2,minmax(0,1fr))] gap-x-6 gap-y-1.5 text-sm sm:grid-cols-3">
      <div><dt class="label">ID</dt><dd class="text-neutral-500">{d.id}</dd></div>
      <div><dt class="label">收藏</dt><dd>{d.collects}</dd></div>
      <div><dt class="label">评论</dt><dd>{d.comments}</dd></div>
      <div><dt class="label">出演作品数</dt><dd>{d.subjects.length || '—'}</dd></div>
    </dl>
    <p class="mt-1.5 truncate text-sm text-neutral-500 dark:text-neutral-400" title={d.name}>原名：{d.name}</p>
  </section>
  <EntityPic kind="character" {id} href={externalUrl('character', id)} alt={d.name_cn || d.name} class="max-w-40" />
</div>

<InfoboxList fields={d.infobox ?? []} />

<section class="mb-4" data-sec-label="简介">
  <div class="divider-short"></div>
  <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
  <p class="whitespace-pre-wrap text-sm leading-relaxed">{d.summary || '（无简介）'}</p>
</section>

{#if d.relations.length}
  <section class="mb-4" data-sec-label="关联角色">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">关联角色（{d.relations.length}）</h4>
    <ul class="space-y-0.5 text-sm">
      {#each d.relations ?? [] as r}
        <li class="text-neutral-700 dark:text-neutral-300">
          <span class="text-neutral-500">{r.relation_name}</span>
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('character', r.related_person_id)}>{r.related_name || r.related_person_id}</button>
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if d.subjects.length}
  <section class="mb-4" data-sec-label="出演作品">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">出演作品（{d.subjects.length}）</h4>
    <ul class="space-y-0.5 text-sm">
      {#each d.subjects ?? [] as s}
        <li class="text-neutral-700 dark:text-neutral-300">
          <span class="text-neutral-500">{s.date || '—'}</span>
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('subject', s.subject_id)}>{s.name_cn || s.name}</button>
          {#if s.score > 0}<span class="text-amber-600 dark:text-amber-400">{fmtScore(s.score)}</span>{/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if d.cvs.length}
  <section data-sec-label="声优 / 演员">
    <div class="divider-short"></div>
    <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">声优 / 演员（{d.cvs.length}）</h4>
    <ul class="space-y-0.5 text-sm">
      {#each d.cvs ?? [] as c}
        <li class="text-neutral-700 dark:text-neutral-300">
          <button type="button" class="cursor-pointer hover:underline" onclick={() => openDetail('person', c.person_id)}>{c.name}</button>
          {#if c.type_name}<span class="chip">{c.type_name}</span>{/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
