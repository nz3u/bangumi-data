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
  // 匹配语义与各页快速筛选一致：
  //   1. 字面子串（不区分大小写，同后端 LIKE / FTS 短语）；
  //   2. 字面未命中时回退拼音全拼/首字母匹配（pinyin-match 返回命中下标区间，
  //      如 dy→「导演」），使拼音筛选命中的汉字同样得到高亮；
  //      区间内含非汉字字符时视为伪命中拒绝（避免标点等被误标）。
  // 关键词为空时原样输出；标签等非关键词筛选不传入 q 即不会高亮。
  import PinyinMatch from 'pinyin-match'
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

    // 1. 字面子串（全部出现位置）
    const hay = s.toLowerCase()
    const k = kw.toLowerCase()
    const out = []
    let i = 0
    for (;;) {
      const j = hay.indexOf(k, i)
      if (j === -1) break
      if (j > i) out.push({ t: s.slice(i, j), m: false })
      out.push({ t: s.slice(j, j + k.length), m: true })
      i = j + k.length
    }
    if (i < s.length) out.push({ t: s.slice(i), m: false })
    if (out.some((p) => p.m)) return out

    // 2. 拼音回退：命中区间 [a, b]（闭区间下标）。
    //    仅当区间内全部是汉字时才采纳：pinyin-match 偶尔会把标点/拉丁字符
    //    卷进区间（如 cp 命中「，英」），这类结果不是真实的拼音对应，直接拒绝。
    const m = PinyinMatch.match(s, kw)
    if (Array.isArray(m) && m.length >= 2) {
      const a = Math.max(Number(m[0]) || 0, 0)
      const b = Math.min(Number(m[1]) || 0, s.length - 1)
      let allHan = b >= a
      for (let x = a; allHan && x <= b; x++) {
        const ch = s[x]
        allHan = ch >= '\u4e00' && ch <= '\u9fff'
      }
      if (allHan) {
        const segs = []
        if (a > 0) segs.push({ t: s.slice(0, a), m: false })
        segs.push({ t: s.slice(a, b + 1), m: true })
        if (b + 1 < s.length) segs.push({ t: s.slice(b + 1), m: false })
        return segs
      }
    }
    return out
  })
</script>

{#each parts as p}{#if p.m}<mark class={MARK} style:--hl={highlightHex()}>{p.t}</mark>{:else}{p.t}{/if}{/each}

