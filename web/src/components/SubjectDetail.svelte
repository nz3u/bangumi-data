<script>
  import { fmtScore, fmtRank, fmtDate, fmtFavorite } from '../lib/format.js'
  import { openDetail } from '../lib/detail.svelte.js'
  import { externalUrl } from '../lib/settings.svelte.js'
  import EntityPic from './EntityPic.svelte'

  let { d, id } = $props()
</script>

<div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4" data-sec-label="信息">
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
  <EntityPic kind="subject" {id} href={externalUrl('subject', id)} alt={d.name_cn || d.name} class="max-w-40" />
</div>

<section class="mb-4" data-sec-label="简介">
  <div class="divider-short"></div>
  <h4 class="mb-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">简介</h4>
  <p class="whitespace-pre-wrap text-sm leading-relaxed">{d.summary || '（无简介）'}</p>
</section>

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
