<script>
  // 主机名选择字段：预设下拉与「自定义…」输入框拼成一个组合控件。
  // 两者共用一套外框、圆角与聚焦态（focus-within），选中「自定义…」时才
  // 在下方展开输入行——避免自定义输入单独占一行、与其他设置项层级不一致。
  import { sanitizeHost } from '../lib/settings.svelte.js'

  let {
    id,
    label,
    presets = [],
    value = $bindable(),
    customValue = $bindable(''),
    placeholder = '',
    fallback = '', // 自定义值非法时实际生效的主机名，用于提示与报错文案
    hintBefore = '',
    hintAfter = '',
    onsave
  } = $props()

  const isCustom = $derived(value === 'custom')
  const invalid = $derived(isCustom && customValue !== '' && sanitizeHost(customValue) === '')
  const effective = $derived(
    isCustom ? sanitizeHost(customValue) || fallback : value || fallback
  )
</script>

<label class="label" for={id}>{label}</label>

<div
  class={`overflow-hidden rounded-lg border bg-white shadow-sm transition-all focus-within:ring-4 dark:bg-white/[0.04] ${
    invalid
      ? 'border-red-400 focus-within:border-red-500 focus-within:ring-red-500/10'
      : 'border-neutral-300/80 focus-within:border-sakura-400 focus-within:ring-sakura-500/10 dark:border-white/[0.09] dark:focus-within:border-sakura-500/50 dark:focus-within:ring-sakura-400/10'
  }`}
>
  <div class="relative">
    <select
      {id}
      class="w-full appearance-none bg-transparent py-1.5 pl-3 pr-8 text-sm text-neutral-900 outline-none dark:text-neutral-100"
      bind:value
      onchange={onsave}
    >
      {#each presets as p}
        <option value={p.value} class="bg-white dark:bg-neutral-900">{p.label}</option>
      {/each}
    </select>
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      class="pointer-events-none absolute right-2.5 top-1/2 size-3.5 -translate-y-1/2 text-neutral-400"
      aria-hidden="true"
    >
      <path d="m6 9 6 6 6-6" />
    </svg>
  </div>

  {#if isCustom}
    <div class="border-t border-neutral-200/80 dark:border-white/[0.07]">
      <input
        type="text"
        class={`w-full bg-transparent px-3 py-1.5 text-sm outline-none placeholder:text-neutral-400 dark:placeholder:text-neutral-500 ${
          invalid ? 'text-red-600 dark:text-red-400' : 'text-neutral-900 dark:text-neutral-100'
        }`}
        {placeholder}
        spellcheck="false"
        bind:value={customValue}
        oninput={onsave}
      />
    </div>
  {/if}
</div>

{#if invalid}
  <p class="mt-1 text-[11px] text-red-500">主机名无效，将回退为 {fallback}</p>
{/if}

{#if hintBefore || hintAfter}
  <p class="mt-2 text-[11px] leading-relaxed text-neutral-400"
    >{hintBefore}<span class="font-medium text-neutral-500 dark:text-neutral-300">{effective}</span
    >{hintAfter}</p
  >
{/if}
