<script>
  // 快速筛选结果的关键词着重号：把文本切分为命中/未命中片段，
  // 命中片段渲染为带颜色的下划线 + 同色背景（文字保持原色）。
  // 颜色来自站点设置的「高亮颜色」（settings.highlightColor，CSS 变量传入，
  // Tailwind 任意值类是构建期编译的，运行时颜色只能走变量）：
  //   浅色模式 bg 与着重线同色；深色模式背景透明、仅保留琥珀色线。
  // 各高亮功能的独立开关（settings.highlightFeatures）也在此统一管理：
  //   scope 指定本组件属于哪一项功能（suggest | filter | title | tags），
  //   关闭时命中片段按普通文本输出，各调用方（建议列表、筛选结果、搜索结果表）
  //   无需各自判断，只管声明自己属于哪个功能即可。
  // 匹配语义与各页快速筛选一致（实现在 lib/match.js，与「命中片段」共用）：
  //   字面子串（不区分大小写，同后端 LIKE / FTS 短语）优先，
  //   未命中时回退拼音全拼/首字母匹配（如 dy→「导演」），
  //   使拼音筛选命中的汉字同样得到高亮。
  // 关键词为空时原样输出；标签等非关键词筛选不传入 q 即不会高亮。
  import { matchRanges } from '../lib/match.js'
  import { highlightHex, isHighlightOn } from '../lib/settings.svelte.js'

  // scope：本组件归属的高亮功能，对应设置面板里的四个独立开关之一
  let { text = '', q = '', scope = 'suggest' } = $props()

  const MARK =
    'text-inherit underline decoration-2 underline-offset-2 decoration-(--hl) dark:decoration-amber-100 bg-(--hl) dark:bg-transparent'

  const parts = $derived.by(() => {
    const s = String(text ?? '')
    const kw = String(q ?? '').trim()
    // 关键词为空或本项高亮关闭：整段按普通文本输出，不做任何匹配计算
    if (!s || !kw || !isHighlightOn(scope)) return s ? [{ t: s, m: false }] : []

    const ranges = matchRanges(s, kw)
    if (!ranges.length) return [{ t: s, m: false }]
    // 按命中区间切分为命中/未命中片段
    const out = []
    let i = 0
    for (const [a, b] of ranges) {
      if (a > i) out.push({ t: s.slice(i, a), m: false })
      out.push({ t: s.slice(a, b + 1), m: true })
      i = b + 1
    }
    if (i < s.length) out.push({ t: s.slice(i), m: false })
    return out
  })
</script>

{#each parts as p}{#if p.m}<mark class={MARK} style:--hl={highlightHex()}>{p.t}</mark>{:else}{p.t}{/if}{/each}

