const CAREER_CN = {
  seiyu: '声优',
  mangaka: '漫画家',
  illustrator: '绘师',
  producer: '制作人',
  artist: '音乐人',
  writer: '作家',
  actor: '演员'
}

export function careerCn(c) {
  return CAREER_CN[c] ?? c
}

export function fmtScore(s) {
  return s > 0 ? s.toFixed(2) : '—'
}

export function fmtRank(r) {
  return r > 0 ? `#${r}` : '—'
}

export function fmtDate(d) {
  return d || '—'
}

const compactFmt = new Intl.NumberFormat('zh-CN', { notation: 'compact' })

// 紧凑数字（中文万/千单位）；0 或空返回空串，便于直接内联到标签后缀
export function fmtCompact(n) {
  return n > 0 ? compactFmt.format(n) : ''
}

export function fmtFavorite(f) {
  if (!f) return '—'
  const parts = []
  if (f.wish) parts.push(`想看 ${f.wish}`)
  if (f.done) parts.push(`看过 ${f.done}`)
  if (f.doing) parts.push(`在看 ${f.doing}`)
  if (f.on_hold) parts.push(`搁置 ${f.on_hold}`)
  if (f.dropped) parts.push(`抛弃 ${f.dropped}`)
  return parts.join(' · ') || '—'
}