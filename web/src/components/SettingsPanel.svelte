<script>
  // 站点设置面板：齿轮按钮 + 下拉面板。
  // 目前提供「外部链接」配置：下拉选择镜像站（bgm.tv / bangumi.tv / chii.in），
  // 选「自定义…」时出现输入框；所有改动实时写入 localStorage 并全局生效。
  import { onMount } from 'svelte'
  import {
    settings,
    saveSettings,
    EXTERNAL_HOST_PRESETS,
    PIC_HOST_PRESETS,
    HIGHLIGHT_AREA_PRESETS,
    sanitizeHost,
    externalHost,
    picHost
  } from '../lib/settings.svelte.js'

  let open = $state(false)
  let root = $state(null)

  const customValid = $derived(
    settings.externalHost !== 'custom' || sanitizeHost(settings.externalHostCustom) !== ''
  )
  const picValid = $derived(
    settings.picHost !== 'custom' || sanitizeHost(settings.picHostCustom) !== ''
  )
  const hostNow = $derived(externalHost())
  const picNow = $derived(picHost())

  function toggle() {
    open = !open
  }

  function toggleHighlightArea(area) {
    const areas = Array.isArray(settings.highlightAreas) ? [...settings.highlightAreas] : []
    const idx = areas.indexOf(area)
    if (idx >= 0) {
      areas.splice(idx, 1)
    } else {
      areas.push(area)
    }
    settings.highlightAreas = areas
    saveSettings()
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
    class="btn-ghost px-2.5"
    type="button"
    title="设置"
    aria-label="设置"
    aria-expanded={open}
    onclick={toggle}
  >
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="size-4" aria-hidden="true">
      <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  </button>

  {#if open}
    <div
      class="absolute right-0 top-full z-50 mt-2 w-72 rounded-md border border-neutral-200 bg-white p-4 text-sm shadow-lg dark:border-neutral-700 dark:bg-neutral-900"
      role="dialog"
      aria-label="站点设置"
    >
      <h3 class="mb-3 font-medium">设置</h3>

      <!-- 外部链接 -->
      <label class="label" for="ext-host">外部链接</label>
      <select
        id="ext-host"
        class="input"
        bind:value={settings.externalHost}
        onchange={saveSettings}
      >
        {#each EXTERNAL_HOST_PRESETS as p}
          <option value={p.value}>{p.label}</option>
        {/each}
      </select>

      {#if settings.externalHost === 'custom'}
        <input
          class={`input mt-2 ${settings.externalHostCustom && !customValid ? 'border-red-400 focus:border-red-500' : ''}`}
          type="text"
          placeholder="如 mirror.example.com"
          spellcheck="false"
          bind:value={settings.externalHostCustom}
          oninput={saveSettings}
        />
        {#if !customValid}
          <p class="mt-1 text-[11px] text-red-500">主机名无效，跳转将回退为 bgm.tv</p>
        {/if}
      {/if}

      <p class="mt-2 text-[11px] leading-relaxed text-neutral-400">
        条目/人物/角色的外链将跳转到
        <span class="font-medium text-neutral-500 dark:text-neutral-300">{hostNow}</span>。
      </p>

      <!-- 图片接口 -->
      <label class="label mt-4" for="pic-host">图片接口</label>
      <select
        id="pic-host"
        class="input"
        bind:value={settings.picHost}
        onchange={saveSettings}
      >
        {#each PIC_HOST_PRESETS as p}
          <option value={p.value}>{p.label}</option>
        {/each}
      </select>

      {#if settings.picHost === 'custom'}
        <input
          class={`input mt-2 ${settings.picHostCustom && !picValid ? 'border-red-400 focus:border-red-500' : ''}`}
          type="text"
          placeholder="如 cdn.example.com"
          spellcheck="false"
          bind:value={settings.picHostCustom}
          oninput={saveSettings}
        />
        {#if !picValid}
          <p class="mt-1 text-[11px] text-red-500">主机名无效，图片将回退为 lain.bgm.tv</p>
        {/if}
      {/if}

      <p class="mt-2 text-[11px] leading-relaxed text-neutral-400">
        人物头像 / 条目封面 / 角色头像将拼接自
        <span class="font-medium text-neutral-500 dark:text-neutral-300">{picNow}</span>；
        配置实时保存在浏览器本地。
      </p>

      <!-- 高亮区域 -->
      <span class="label mt-4">条目页高亮区域</span>
      <div class="flex flex-wrap gap-3">
        {#each HIGHLIGHT_AREA_PRESETS as area}
          <label class="flex items-center gap-1.5 text-sm">
            <input
              type="checkbox"
              class="size-3.5 rounded border-neutral-300 text-sky-600 focus:ring-sky-500 dark:border-neutral-600"
              checked={settings.highlightAreas?.includes(area.value)}
              onchange={() => toggleHighlightArea(area.value)}
            />
            {area.label}
          </label>
        {/each}
      </div>
      <p class="mt-1 text-[11px] leading-relaxed text-neutral-400">
        选择搜索结果中需要高亮的区域；配置实时保存在浏览器本地。
      </p>
    </div>
  {/if}
</div>
