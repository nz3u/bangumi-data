// 快速筛选的匹配语义（唯一实现处，Highlight 组件与本页的「命中片段」共用）：
//   1. 字面子串（不区分大小写，同后端 LIKE / FTS 短语），返回全部出现位置；
//   2. 字面未命中时回退拼音全拼/首字母匹配（pinyin-match 返回命中下标区间，
//      如 dy→「导演」）；区间内含非汉字字符时视为伪命中拒绝
//      （避免标点等被误标）。
// 之所以抽到公共模块：卡片上的简介被截断时要用同一套语义切出命中片段，
// 若两处各写一份，容易出现「筛选判定命中、高亮却标不出来」的不一致。
import PinyinMatch from 'pinyin-match'

const isHan = (ch) => ch >= '\u4e00' && ch <= '\u9fff'

// matchRanges 返回命中区间列表 [[start, end], ...]（闭区间下标）；未命中返回 []。
export function matchRanges(text, q) {
  const s = String(text ?? '')
  const kw = String(q ?? '').trim()
  if (!s || !kw) return []

  // 1. 字面子串（全部出现位置）
  const hay = s.toLowerCase()
  const k = kw.toLowerCase()
  const out = []
  let i = 0
  for (;;) {
    const j = hay.indexOf(k, i)
    if (j === -1) break
    out.push([j, j + k.length - 1])
    i = j + k.length
  }
  if (out.length) return out

  // 2. 拼音回退：命中区间 [a, b]（闭区间下标），仅当区间内全部是汉字时才采纳
  const m = PinyinMatch.match(s, kw)
  if (Array.isArray(m) && m.length >= 2) {
    const a = Math.max(Number(m[0]) || 0, 0)
    const b = Math.min(Number(m[1]) || 0, s.length - 1)
    if (b >= a) {
      let allHan = true
      for (let x = a; allHan && x <= b; x++) allHan = isHan(s[x])
      if (allHan) return [[a, b]]
    }
  }
  return []
}

// matchSnippet 把长文本（如人物简介）截成命中词前后各 radius 个字符的一小段，
// 被截断的一端补省略号；未命中时返回空串，调用方回退展示原文。
// 片段内保留命中词原文，因此可直接交给 Highlight 高亮。
export function matchSnippet(text, q, radius = 24) {
  const raw = String(text ?? '')
  const ranges = matchRanges(raw, q)
  if (!ranges.length) return ''
  const [a, b] = ranges[0]
  const start = Math.max(0, a - radius)
  const end = Math.min(raw.length, b + 1 + radius)
  // 简介里含换行与多余空白，压成单行后按两端截断情况补省略号
  const core = raw.slice(start, end).replace(/\s+/g, ' ').trim()
  return (start > 0 ? '…' : '') + core + (end < raw.length ? '…' : '')
}
