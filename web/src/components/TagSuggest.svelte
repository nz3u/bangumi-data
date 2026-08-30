<script>
  // 条目搜索的标签/元标签输入框（带实时建议）：
  // - 候选池自 /api/subjects/tags 拉取一次并模块级缓存（普通标签与元标签各一份），
  //   输入时本地过滤：中文/拉丁字面子串，或拼音全拼/首字母命中（如 xs→小说、qh→奇幻）。
  // - 支持多标签组合：当前编辑段（最后一个逗号之后）以 '+'/'-' 开头决定极性，
  //   点选建议后写回「<极性><标签>,」；整框内容形如 “+奇幻,-科幻”。
  // - 键盘：↑↓ 循环高亮，回车确认高亮项（拦截表单提交），Esc 关闭；
  //   无建议时回车交还原生表单提交。
  import PinyinMatch from 'pinyin-match'
  import { fly } from 'svelte/transition'
  import Highlight from './Highlight.svelte'
  import { loadTagPool } from '../lib/api.js'
  import { fmtCompact } from '../lib/format.js'

  let {
    inputId,
    placeholder = '',
    kind = 'tag',
    text = $bindable(''),
    onfocus = () => {},
    onblur = () => {},
    oninput = () => {},
    pageTags = []
  } = $props()

  const LIMIT = 12

  let open = $state(false)
  let pool = $state(null) // [{name, cnt}]，按使用次数降序
  let poolLoading = $state(false)
  let poolError = $state('')
  let active = $state(-1)

  let listEl = $state(null) // 建议列表容器（内部滚动）

  // 当前正在编辑的段：最后一个逗号（中英文皆可）之后的内容
  function lastSepIndex(v) {
    return Math.max(v.lastIndexOf(','), v.lastIndexOf('，'))
  }

  // 极性：'-' 开头为负标签（要求排除），其余为正标签（要求包含）
  const polarity = $derived.by(() => {
    const tok = String(text ?? '').slice(lastSepIndex(String(text ?? '')) + 1).trim()
    return tok.startsWith('-') ? '-' : '+'
  })

  // 过滤词：去掉段首的 +/- 后的剩余部分
  const query = $derived.by(() => {
    const tok = String(text ?? '').slice(lastSepIndex(String(text ?? '')) + 1).trim()
    return tok.replace(/^[+-]/, '').trim()
  })

  async function ensurePool() {
    if (pool || poolLoading) return
    poolLoading = true
    poolError = ''
    try {
      pool = await loadTagPool(kind)
    } catch (e) {
      poolError = e.message
    } finally {
      poolLoading = false
    }
  }

  // 子串（不区分大小写）或拼音全拼/首字母命中；空词视为全部
  function hit(name, q) {
    const s = String(q ?? '').toLowerCase()
    if (!s) return true
    return name.toLowerCase().includes(s) || !!PinyinMatch.match(name, s)
  }

  // 构建当前页面标签名称集合（用于优先展示）
  const pageTagSet = $derived(new Set(pageTags.map((t) => t.name)))

  // 合并页面标签与 API 标签池：页面标签不在池中时补充进去，保证优先排序生效
  const mergedPool = $derived.by(() => {
    if (!pool) return pageTags
    if (pageTags.length === 0) return pool
    const names = new Set(pool.map((t) => t.name))
    const extra = pageTags.filter((t) => !names.has(t.name))
    return extra.length > 0 ? [...extra, ...pool] : pool
  })

  const items = $derived.by(() => {
    const filtered = mergedPool.filter((t) => hit(t.name, query))
    const q = query
    if (!q) {
      // 无输入时：页面标签在前，其余在后
      const page = filtered.filter((t) => pageTagSet.has(t.name))
      const rest = filtered.filter((t) => !pageTagSet.has(t.name))
      return [...page, ...rest].slice(0, LIMIT)
    }
    const k = q.toLowerCase()
    // 按相关性排序：前缀 > 子串/拼音
    const scored = filtered.map((t) => ({
      t,
      pre: t.name.toLowerCase().startsWith(k) ? 0 : 1,
      onPage: pageTagSet.has(t.name) ? 0 : 1
    }))
    // 页面标签优先，再按相关性排序（稳定排序保持次数降序）
    return scored
      .sort((a, b) => a.onPage - b.onPage || a.pre - b.pre)
      .slice(0, LIMIT)
      .map((x) => x.t)
  })

  function resetActive() {
    active = items.length > 0 ? 0 : -1
  }

  function onInput() {
    oninput?.()
    ensurePool()
    open = true
    resetActive()
  }

  // 确认建议：把当前编辑段替换为「<极性><标签名>,」，保留焦点继续追加下一段
  function pick(t) {
    const v = String(text ?? '')
    const i = lastSepIndex(v)
    const head = i >= 0 ? v.slice(0, i + 1) : ''
    text = head + polarity + t.name + ','
    oninput?.()
    resetActive() // 段落清空后展示热门标签，便于连续点选
  }

  // 高亮项变化时滚动进可视范围（同 PersonSuggest）
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
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const dir = e.key === 'ArrowDown' ? 1 : -1
      active = (Math.max(active, 0) + dir + items.length) % items.length
    } else if (e.key === 'Enter' && active >= 0) {
      e.preventDefault() // 拦截表单提交，改走建议选中流程
      pick(items[active])
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
    spellcheck="false"
    bind:value={text}
    oninput={onInput}
    onkeydown={onKeydown}
    onfocus={() => {
      ensurePool()
      onfocus?.()
      if (pool && !poolError) open = true
      resetActive()
    }}
    onblur={() => {
      onblur?.()
      setTimeout(() => (open = false), 150)
    }}
  />
  {#if open}
    {#if poolError}
      <div class="absolute inset-x-0 top-full z-20 mt-1 rounded-xl border border-neutral-200/80 bg-white px-3 py-2 text-xs text-neutral-500 shadow-pop ring-1 ring-black/[0.03] dark:border-white/[0.08] dark:bg-neutral-900 dark:ring-white/[0.06]">
        标签候选加载失败：{poolError}
      </div>
    {:else if poolLoading && !pool}
      <div class="absolute inset-x-0 top-full z-20 mt-1 rounded-xl border border-neutral-200/80 bg-white px-3 py-2 text-xs text-neutral-500 shadow-pop ring-1 ring-black/[0.03] dark:border-white/[0.08] dark:bg-neutral-900 dark:ring-white/[0.06]">
        标签候选加载中…
      </div>
    {:else if items.length > 0}
      <ul
        bind:this={listEl}
        class="absolute inset-x-0 top-full z-20 mt-1 max-h-80 overflow-y-auto rounded-xl border border-neutral-200/80 bg-white py-1 shadow-pop ring-1 ring-black/[0.03] dark:border-white/[0.08] dark:bg-neutral-900 dark:ring-white/[0.06]"
        transition:fly={{ y: -4, duration: 130 }}
        role="listbox"
        aria-label="标签建议"
      >
        {#each items as t, i (t.name)}
          <li role="option" aria-selected={i === active}>
            <button
              type="button"
              class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800 {i === active ? 'bg-sakura-50 dark:bg-sakura-950/60' : ''}"
              title="{polarity === '-' ? '排除' : '包含'}标签「{t.name}」（使用 {t.cnt} 次）"
              onmousedown={(e) => e.preventDefault()}
              onclick={() => pick(t)}
              onmousemove={() => (active = i)}
            >
              <span class="min-w-0 flex-1 truncate"><Highlight text={t.name} q={query} /></span>
              <small class="shrink-0 text-xs text-neutral-400">{fmtCompact(t.cnt)}</small>
              <span class="chip shrink-0 {polarity === '-' ? 'line-through decoration-red-500 dark:decoration-red-400' : ''}">{polarity === '-' ? '排除' : '包含'}</span>
            </button>
          </li>
        {/each}
      </ul>
    {:else if query}
      <div class="absolute inset-x-0 top-full z-20 mt-1 rounded-xl border border-neutral-200/80 bg-white px-3 py-2 text-xs text-neutral-500 shadow-pop ring-1 ring-black/[0.03] dark:border-white/[0.08] dark:bg-neutral-900 dark:ring-white/[0.06]">
        无匹配标签，回车按原词筛选
      </div>
    {/if}
  {/if}
</div>
