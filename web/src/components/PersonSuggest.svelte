<script>
  import { onDestroy } from 'svelte'
  import { getPerson, searchPersons } from '../lib/api.js'

  // 人物 ID 输入框（带搜索提示）：
  // - 输入文字防抖后请求 /persons/search 列出前 10 条；纯数字时并行查询精确
  //   ID 并置顶，因此直接粘贴/输入 ID 也能看到对应人物。
  // - 选中（点击/回车高亮项）后输入框显示人物名字，bind:pid 同步真实 ID，
  //   父组件提交逻辑仍使用 ID；未选中直接回车则走原生表单提交（纯数字可用）。
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

  let timer = null
  let reqSeq = 0 // 过期响应保护

  function digitsOf(v) {
    const t = String(v ?? '').trim()
    return /^\d+$/.test(t) ? Number(t) : null
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
      active = -1
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

  function onKeydown(e) {
    if (e.key === 'Escape') {
      open = false
      return
    }
    if (!open || items.length === 0) return // 其余情况交还默认行为（如表单提交）
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const dir = e.key === 'ArrowDown' ? 1 : -1
      active = (active + dir + items.length) % items.length
    } else if (e.key === 'Enter' && active >= 0) {
      e.preventDefault() // 拦截表单提交，改走建议选中流程
      pick(items[active])
    }
  }

  onDestroy(() => clearTimeout(timer))
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
      class="absolute inset-x-0 top-full z-20 mt-1 max-h-80 overflow-y-auto rounded-md border border-neutral-200 bg-white py-1 shadow-lg dark:border-neutral-700 dark:bg-neutral-900"
      role="listbox"
      aria-label="人物搜索建议"
    >
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
