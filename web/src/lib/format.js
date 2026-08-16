export function fmtScore(s) {
  return s > 0 ? s.toFixed(2) : '—'
}

export function fmtRank(r) {
  return r > 0 ? `#${r}` : '—'
}

export function fmtDate(d) {
  return d || '—'
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