<script>
  import { onDestroy } from 'svelte'
  import { getPerson, searchPersons } from '../lib/api.js'
  import { goToTab } from '../lib/nav.js'

  // 人物 ID 输入框（带搜索提示）：
  // - 输入文字防抖后请求 /persons/search 列出前 10 条；纯数字时并行查询精确
  //   ID 并置顶，因此直接粘贴/输入 ID 也能看到对应人物。
  // - 选中（点击/回车高亮项）后输入框显示人物名字，bind:pid 同步真实 ID，
  //   父组件提交逻辑仍使用 ID；未选中直接回车则走原生表单提交（纯数字可用）。
  // - 建议中出现「显示名完全相同」的多个人物（常见中文姓名重名，如“刘畅”）时，
  //   列表顶部展示提示行；点选或回车确认后跳转人物搜索页并按该名字发起搜索。
  // - 建议加载后默认高亮第一行（有重名提示时即提示行），回车可直接确认。
  let {
    inputId,
    placeholder = '人物 ID 或名字',
    text = $bindable(''),
    pid = $bindable(null),
    onpick = () => {}
  } = $props()

  const DEBOUNCE_MS = 300
  const LIMIT = 10

  let open = $state(false)
  let loading = $state(false)
  let items = $state([])
  let active = $state(-1)
  let pickedName = '' // 当前 pid 对应的显示名；文本被手动改掉后失效

  let listEl = $state(null) // 建议列表容器（内部滚动）

  let timer = null
  let reqSeq = 0 // 过期响应保护

  // 是否存在重名建议：原名相同，或非空中文名相同（如“刘畅”的多位同名人名）
  function dupExists(list) {
    const count = new Map()
    for (const p of list) {
      if (p.name) count.set(p.name, (count.get(p.name) ?? 0) + 1)
      if (p.name_cn && p.name_cn !== p.name) count.set(p.name_cn, (count.get(p.name_cn) ?? 0) + 1)
    }
    for (const c of count.values()) if (c > 1) return true
    return false
  }

  const hasDup = $derived(dupExists(items))

  // 建议加载后的默认高亮：第一行。存在重名提示时第一行即提示行（-1），
  // 否则为第一个人物（0）；如此回车可直接确认第一行，无需先按方向键。
  function resetActive(list) {
    active = dupExists(list) ? -1 : 0
  }

  function digitsOf(v) {
    const t = String(v ?? '').trim()
    return /^\d+$/.test(t) ? Number(t) : null
  }

  // 确认重名提示：关闭弹层并携带当前关键词跳转人物搜索页
  function goDupSearch() {
    const q = String(text ?? '').trim()
    open = false
    items = []
    active = -1
    reqSeq++
    clearTimeout(timer)
    goToTab('/persons', { q })
  }

  function onInput() {
    const t = String(text ?? '').trim()
    const d = digitsOf(t)
    if (d) {
      pid = d
      pickedName = ''
    } else if (t === pickedName) {
      // 文本仍是选中人物的名字，保持已解析的 ID
    } else {
      pid = null
      pickedName = ''
    }
    reqSeq++
    clearTimeout(timer)
    if (!t) {
      open = false
      items = []
      active = -1
      loading = false
      return
    }
    timer = setTimeout(fetchSuggest, DEBOUNCE_MS)
  }

  async function fetchSuggest() {
    const q = String(text ?? '').trim()
    if (!q) return
    const seq = ++reqSeq
    loading = true
    try {
      const d = digitsOf(q)
      const [direct, res] = await Promise.all([
        d ? getPerson(d).catch(() => null) : Promise.resolve(null),
        searchPersons({ q, page: 1, size: LIMIT })
      ])
      if (seq !== reqSeq) return
      const seen = new Set()
      const merged = []
      for (const p of [direct, ...(res.items ?? [])]) {
        if (!p || seen.has(p.id)) continue
        seen.add(p.id)
        merged.push(p)
        if (merged.length >= LIMIT) break
      }
      items = merged
      resetActive(merged)
      open = merged.length > 0
    } catch {
      if (seq === reqSeq) {
        items = []
        open = false
      }
    } finally {
      if (seq === reqSeq) loading = false
    }
  }

  function pick(p) {
    text = p.name
    pid = p.id
    pickedName = p.name
    open = false
    items = []
    active = -1
    reqSeq++
    onpick?.(p)
  }

  // 高亮项变化时将其滚动进可视范围：自定义列表框不像原生焦点那样自动滚动，
  // 键盘移出可视区（尤其有重名提示行撑高列表时）会导致选中行不可见。
  $effect(() => {
    if (!open || !listEl) return
    void active
    const el = listEl.querySelector('li[aria-selected="true"]')
    if (!el) return
    const lr = listEl.getBoundingClientRect()
    const er = el.getBoundingClientRect()
    if (er.top < lr.top) listEl.scrollTop -= lr.top - er.top
    else if (er.bottom > lr.bottom) listEl.scrollTop += er.bottom - lr.bottom
  })

  function onKeydown(e) {
    if (e.key === 'Escape') {
      open = false
      return
    }
    if (!open || items.length === 0) return // 其余情况交还默认行为（如表单提交）
    // 组合索引：有重名提示时 c===0 为提示行（active=-1）、c>=1 对应 items[c-1]；
    // 无提示时 c 直接对应 items 下标，在首尾间循环。
    const total = items.length + (hasDup ? 1 : 0)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const dir = e.key === 'ArrowDown' ? 1 : -1
      let c = hasDup ? (active < 0 ? 0 : active + 1) : Math.max(active, 0)
      c = (c + dir + total) % total
      active = hasDup ? (c === 0 ? -1 : c - 1) : c
    } else if (e.key === 'Enter' && active >= 0) {
      e.preventDefault() // 拦截表单提交，改走建议选中流程
      pick(items[active])
    } else if (e.key === 'Enter' && hasDup) {
      e.preventDefault() // 未高亮具体人物但存在重名提示：回车即确认跳转
      goDupSearch()
    }
  }
</script>

<div class="relative min-w-0">
  <input
    id={inputId}
    class="input"
    type="text"
    {placeholder}
    autocomplete="off"
    bind:value={text}
    oninput={onInput}
    onkeydown={onKeydown}
    onfocus={() => {
      if (items.length > 0) open = true
    }}
    onblur={() => setTimeout(() => (open = false), 150)}
  />
  {#if open && items.length > 0}
    <ul
      bind:this={listEl}
      class="absolute inset-x-0 top-full z-20 mt-1 max-h-80 overflow-y-auto rounded-md border border-neutral-200 bg-white py-1 shadow-lg dark:border-neutral-700 dark:bg-neutral-900"
      role="listbox"
      aria-label="人物搜索建议"
    >
      {#if hasDup}
        <li role="option" aria-selected={active < 0}>
          <button
            type="button"
            class={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs ${active < 0 ? 'bg-sky-50 text-sky-700 dark:bg-sky-950/60 dark:text-sky-300' : 'text-neutral-500 hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800'}`}
            title="存在同名人物，点击或回车跳转人物搜索页查看全部结果"
            onmousedown={(e) => e.preventDefault()}
            onclick={goDupSearch}
            onmousemove={() => (active = -1)}
          >
            <svg class="size-3.5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span class="min-w-0 flex-1 truncate">发现重名人物，点击或回车到搜索页查看全部 →</span>
          </button>
        </li>
      {/if}
      {#each items as p, i (p.id)}
        <li role="option" aria-selected={i === active}>
          <button
            type="button"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800 {i === active ? 'bg-sky-50 dark:bg-sky-950/60' : ''}"
            onmousedown={(e) => e.preventDefault()}
            onclick={() => pick(p)}
            onmousemove={() => (active = i)}
          >
            <span class="min-w-0 flex-1 truncate">{p.name}{#if p.name_cn && p.name_cn !== p.name}（{p.name_cn}）{/if}</span>
            <small class="shrink-0 text-xs text-neutral-400">#{p.id}</small>
            <span class="chip shrink-0">{p.type_name}</span>
          </button>
        </li>
      {/each}
    </ul>
  {:else if open && loading}
    <div class="absolute inset-x-0 top-full z-20 mt-1 rounded-md border border-neutral-200 bg-white px-3 py-2 text-xs text-neutral-500 shadow-lg dark:border-neutral-700 dark:bg-neutral-900">
      搜索中…
    </div>
  {/if}
</div>
