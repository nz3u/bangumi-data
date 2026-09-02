<script>
  // 站点设置面板：齿轮按钮 + 下拉面板。
  // 面板内所有设置项统一为「标签 → 控件 → 说明」三层结构：
  //   · 外部链接 / 图片接口：预设下拉 + 自定义输入的内联组合控件（HostField）
  //   · 高亮功能：一行总开关（统一开关）+ 四项独立开关
  //   · 高亮颜色：三张色卡单选，选中「自定义」时展开取色器；
  //     四项高亮全部关闭时整块隐藏（无高亮可着色）。
  // 所有改动实时写入 localStorage 并全局生效。
  import { onMount } from 'svelte'
  import { fly, fade } from 'svelte/transition'
  import HostField from './HostField.svelte'
  import {
    settings,
    saveSettings,
    EXTERNAL_HOST_PRESETS,
    PIC_HOST_PRESETS,
    HIGHLIGHT_FEATURE_PRESETS,
    isHighlightOn,
    isAnyHighlightOn,
    highlightOnCount,
    toggleHighlightFeature,
    setAllHighlight,
    HIGHLIGHT_COLOR_PRESETS,
    DEFAULT_HIGHLIGHT_COLOR_CUSTOM,
    highlightHex
  } from '../lib/settings.svelte.js'

  let open = $state(false)
  let root = $state(null)

  // 高亮总开关状态：全开 / 部分开启（中间态）/ 全关
  const hiCount = $derived(highlightOnCount())
  const allHi = $derived(hiCount === HIGHLIGHT_FEATURE_PRESETS.length)
  const anyHi = $derived(isAnyHighlightOn())

  // 自定义色卡的展示色：始终显示用户已选的自定义色（而非当前生效色），
  // 非法值时回退默认，保证色卡在未选中时也反映真实配置。
  const customHex = $derived(
    /^#[0-9a-fA-F]{6}$/.test(settings.highlightColorCustom ?? '')
      ? settings.highlightColorCustom
      : DEFAULT_HIGHLIGHT_COLOR_CUSTOM
  )

  // 色卡预览区上的文字色：按相对亮度在深/浅之间切换，
  // 避免自定义深色时预览文字与背景糊在一起。
  function textOn(hex) {
    const m = /^#?([0-9a-f]{6})$/i.exec(String(hex ?? '').trim())
    if (!m) return '#3f3f46'
    const n = parseInt(m[1], 16)
    const lin = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
      const c = v / 255
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
    })
    const l = 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2]
    return l > 0.45 ? '#3f3f46' : '#fafafa'
  }

  function toggle() {
    open = !open
  }

  function close() {
    open = false
  }

  onMount(() => {
    const onDocDown = (e) => {
      if (open && root && !root.contains(e.target)) open = false
    }
    const onKey = (e) => {
      if (e.key === 'Escape') open = false
    }
    document.addEventListener('pointerdown', onDocDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('pointerdown', onDocDown)
      document.removeEventListener('keydown', onKey)
    }
  })
</script>

<div class="relative" bind:this={root}>
  <button
    class="btn-icon group"
    type="button"
    title="设置"
    aria-label="设置"
    aria-expanded={open}
    onclick={toggle}
  >
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="size-4 transition-transform duration-300 group-hover:rotate-45" aria-hidden="true">
      <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  </button>

  {#if open}
    <!-- 移动端全屏设置面板 -->
    <div
      class="fixed inset-0 z-50 flex flex-col bg-white dark:bg-neutral-900 md:hidden"
      transition:fade={{ duration: 150 }}
      role="dialog"
      aria-label="站点设置"
    >
      <div class="flex items-center justify-between border-b border-neutral-200/80 px-4 py-3 dark:border-white/[0.08]">
        <h3 class="font-medium">设置</h3>
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-lg text-neutral-500 hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800"
          aria-label="关闭设置"
          onclick={close}
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-5" aria-hidden="true">
            <path d="M18 6 6 18" /><path d="m6 6 12 12" />
          </svg>
        </button>
      </div>
      <div class="flex-1 overflow-y-auto p-4 text-sm overscroll-contain">
        {@render settingsContent()}
      </div>
    </div>

    <!-- 桌面端下拉面板 -->
    <div
      class="absolute right-0 top-full z-50 mt-2 hidden max-h-[calc(100vh-6rem)] w-72 origin-top-right transform-gpu overflow-y-auto overscroll-contain rounded-xl border border-neutral-200/80 bg-white p-4 text-sm shadow-pop ring-1 ring-black/[0.03] dark:border-white/[0.08] dark:bg-neutral-900 dark:ring-white/[0.06] md:block"
      transition:fly={{ y: -6, duration: 150 }}
      role="dialog"
      aria-label="站点设置"
    >
      {@render settingsContent()}
    </div>
  {/if}
</div>

<!-- 通用开关行：左侧文案 + 右侧滑块；mixed 表示「部分开启」中间态 -->
{#snippet switchRow({ id, label, hint = '', on = false, mixed = false, onToggle, head = false, sub = false })}
  <div class={`flex items-center justify-between gap-3 ${head ? 'mb-1' : ''}`}>
    <span
      class={`min-w-0 truncate ${
        head
          ? 'text-xs text-neutral-500 dark:text-neutral-400'
          : `text-[13px] text-neutral-600 dark:text-neutral-400${sub ? ' pl-1' : ''}`
      }`}
      id={`${id}-label`}
    >{label}</span>
    <button
      type="button"
      role="switch"
      aria-checked={mixed ? 'mixed' : on ? 'true' : 'false'}
      aria-labelledby={`${id}-label`}
      title={hint || label}
      class={`relative inline-flex h-4.5 w-9 shrink-0 items-center rounded-full transition-colors duration-150 ${mixed ? 'bg-sakura-600/45' : on ? 'bg-sakura-600' : 'bg-neutral-300 dark:bg-neutral-700'}`}
      onclick={onToggle}
    >
      <span
        class={`inline-block size-3.5 transform rounded-full bg-white shadow transition-transform duration-150 ${on ? 'translate-x-[1.15rem]' : mixed ? 'translate-x-[0.72rem]' : 'translate-x-0.5'}`}
      ></span>
    </button>
  </div>
{/snippet}

<!-- 高亮颜色色卡：上半为配色预览（带下划线示意着重线），下半为名称 -->
{#snippet colorCard(p)}
  {@const hex = p.value === 'custom' ? customHex : p.hex}
  {@const picked = settings.highlightColor === p.value}
  <label
    class={`flex cursor-pointer flex-col overflow-hidden rounded-lg border transition-all has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-sakura-500/30 ${
      picked
        ? 'border-sakura-500 ring-2 ring-sakura-500/20 dark:border-sakura-400'
        : 'border-neutral-300/80 hover:border-neutral-400 dark:border-white/[0.09] dark:hover:border-white/20'
    }`}
  >
    <input
      type="radio"
      name="hl-color"
      class="sr-only"
      value={p.value}
      bind:group={settings.highlightColor}
      onchange={saveSettings}
    />
    <span
      class="flex h-8 items-center justify-center text-[13px] underline decoration-2 underline-offset-2"
      style:background={hex}
      style:color={textOn(hex)}
    >示例</span>
    <span
      class={`border-t px-1 py-1 text-center text-[11px] ${
        picked
          ? 'border-sakura-500/40 text-sakura-600 dark:border-sakura-400/40 dark:text-sakura-400'
          : 'border-neutral-200/80 text-neutral-500 dark:border-white/[0.07] dark:text-neutral-400'
      }`}
    >{p.value === 'custom' ? '自定义' : p.label}</span>
  </label>
{/snippet}

{#snippet settingsContent()}
  <h3 class="mb-3 font-medium md:block hidden">设置</h3>

  <!-- 外部链接 -->
  <HostField
    id="ext-host"
    label="外部链接"
    presets={EXTERNAL_HOST_PRESETS}
    bind:value={settings.externalHost}
    bind:customValue={settings.externalHostCustom}
    placeholder="如 mirror.example.com"
    fallback="bgm.tv"
    hintBefore="条目/人物/角色的外链将跳转到 "
    hintAfter="。配置实时保存在浏览器本地。"
    onsave={saveSettings}
  />

  <!-- 图片接口 -->
  <div class="mt-4">
    <HostField
      id="pic-host"
      label="图片接口"
      presets={PIC_HOST_PRESETS}
      bind:value={settings.picHost}
      bind:customValue={settings.picHostCustom}
      placeholder="如 cdn.example.com"
      fallback="lain.bgm.tv"
      hintBefore="人物头像 / 条目封面 / 角色头像将拼接自 "
      hintAfter="。"
      onsave={saveSettings}
    />
  </div>

  <!-- 高亮功能：首行总开关统一控制，随后四项各自独立 -->
  <div class="mt-4">
    {@render switchRow({
      id: 'hl-all',
      label: '全部高亮',
      hint: '统一开启 / 关闭全部高亮',
      on: allHi,
      mixed: anyHi && !allHi,
      onToggle: () => setAllHighlight(!allHi),
      head: true
    })}
    <div class="space-y-1.5">
      {#each HIGHLIGHT_FEATURE_PRESETS as f (f.value)}
        {@render switchRow({
          id: `hl-${f.value}`,
          label: f.label,
          hint: f.hint,
          on: isHighlightOn(f.value),
          onToggle: () => toggleHighlightFeature(f.value),
          sub: true
        })}
      {/each}
    </div>
  </div>
  <p class="mt-2 text-[11px] leading-relaxed text-neutral-400">
    已开启 {hiCount}/{HIGHLIGHT_FEATURE_PRESETS.length} 项；总开关统一控制，单项可独立调整。
  </p>

  <!-- 高亮颜色：全部高亮关闭时无需设置，整块隐藏 -->
  {#if anyHi}
    <div transition:fade={{ duration: 120 }}>
      <span class="label mt-4">高亮颜色</span>
      <div class="grid grid-cols-3 gap-1.5" role="radiogroup" aria-label="高亮颜色">
        {#each HIGHLIGHT_COLOR_PRESETS as p (p.value)}
          {@render colorCard(p)}
        {/each}
      </div>

      {#if settings.highlightColor === 'custom'}
        <div
          class="mt-1.5 flex items-center gap-2 rounded-lg border border-neutral-300/80 bg-white px-2 py-1.5 dark:border-white/[0.09] dark:bg-white/[0.04]"
          transition:fade={{ duration: 120 }}
        >
          <input
            type="color"
            class="h-6 w-8 shrink-0 cursor-pointer rounded border border-neutral-300 bg-transparent p-0.5 dark:border-neutral-600"
            title="自定义高亮颜色"
            bind:value={settings.highlightColorCustom}
            oninput={saveSettings}
          />
          <code class="text-[11px] tabular-nums text-neutral-500 dark:text-neutral-400">{highlightHex()}</code>
          <mark
            class="ml-auto min-w-0 truncate text-inherit underline decoration-2 underline-offset-2 decoration-(--hl) bg-(--hl) dark:bg-transparent dark:decoration-amber-100"
            style:--hl={highlightHex()}
          >高亮效果</mark>
        </div>
      {/if}
    </div>
    <p class="mt-2 text-[11px] leading-relaxed text-neutral-400">
      浅色模式下背景与着重线同色；深色模式背景透明、仅保留着重线。
    </p>
  {/if}
{/snippet}
