const API_BASE = '/api/admin'

function tokenHeaders() {
  let t = ''
  try {
    t = localStorage.getItem('bgm.admin_token') || ''
  } catch {}
  // URL 携带的 token 优先级更高（开箱场景分享链接）
  try {
    const u = new URL(window.location.href)
    const q = u.searchParams.get('token')
    if (q) t = q
  } catch {}
  const h = {}
  if (t) {
    h['X-Admin-Token'] = t
    h['Authorization'] = `Bearer ${t}`
  }
  return h
}

export function getStoredToken() {
  try {
    return localStorage.getItem('bgm.admin_token') || ''
  } catch { return '' }
}
export function setStoredToken(v) {
  try {
    if (v) localStorage.setItem('bgm.admin_token', v)
    else localStorage.removeItem('bgm.admin_token')
  } catch {}
}

async function adminRequest(path, { method = 'GET', body, params } = {}) {
  const url = new URL(API_BASE + path, window.location.origin)
  for (const [k, v] of Object.entries(params ?? {})) {
    if (v === undefined || v === null || v === '') continue
    url.searchParams.set(k, String(v))
  }
  // 若 URL 未带 token 且存储有 token，自动附加 query（便于 SSE 链接）
  // 同时也通过 header 发送
  const headers = { ...tokenHeaders() }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(url, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined
  })
  let data
  try { data = await res.json() } catch { throw new Error(`HTTP ${res.status}`) }
  if (!res.ok || data.ok === false) {
    const err = new Error(data.error || `HTTP ${res.status}`)
    err.status = res.status
    throw err
  }
  return data.data
}

export function adminStatus() { return adminRequest('/status') }
export function adminPublicStatus() {
  // 无需鉴权的轻量状态（同样走 adminRequest 但后端有 publicStatus 接口）
  // 直接 fetch /api/admin/public-status
  return fetch('/api/admin/public-status').then(async r => {
    const j = await r.json()
    if (!r.ok || j.ok === false) throw new Error(j.error || `HTTP ${r.status}`)
    return j.data
  })
}
export function adminConfig() { return adminRequest('/config') }
export function adminSaveConfig(payload) { return adminRequest('/config', { method: 'PUT', body: payload }) }
export function adminTriggerUpdate(force = false) { return adminRequest('/update', { method: 'POST', body: { force } }) }
export function adminCancel() { return adminRequest('/cancel', { method: 'POST' }) }
export function adminReset() { return adminRequest('/reset', { method: 'POST' }) }
export function adminLogs() { return adminRequest('/logs') }

export function openLogStream(onLine, onError) {
  let t = ''
  try { t = localStorage.getItem('bgm.admin_token') || '' } catch {}
  try {
    const u = new URL(window.location.href)
    const q = u.searchParams.get('token')
    if (q) t = q
  } catch {}
  const url = new URL('/api/admin/logs/stream', window.location.origin)
  if (t) url.searchParams.set('token', t)
  const es = new EventSource(url.toString())
  es.addEventListener('log', (e) => { try { onLine(e.data) } catch {} })
  es.onerror = (e) => { if (onError) onError(e); }
  return () => es.close()
}

export function openStatusStream(onStatus, onError) {
  let t = ''
  try { t = localStorage.getItem('bgm.admin_token') || '' } catch {}
  try { const u = new URL(window.location.href); const q = u.searchParams.get('token'); if (q) t = q } catch {}
  const url = new URL('/api/admin/status/stream', window.location.origin)
  if (t) url.searchParams.set('token', t)
  const es = new EventSource(url.toString())
  es.addEventListener('status', (e) => {
    try { const d = JSON.parse(e.data); onStatus(d) } catch {}
  })
  es.onerror = (e) => { if (onError) onError(e) }
  return () => es.close()
}

export function openPublicStatusStream(onStatus, onError) {
  const url = new URL('/api/admin/public-status/stream', window.location.origin)
  const es = new EventSource(url.toString())
  es.addEventListener('status', (e) => {
    try { const d = JSON.parse(e.data); onStatus(d) } catch {}
  })
  es.onerror = (e) => { if (onError) onError(e) }
  return () => es.close()
}

export function openHealthStream(onHealth, onError) {
  const url = new URL('/api/health/stream', window.location.origin)
  const es = new EventSource(url.toString())
  es.addEventListener('health', (e) => {
    try { const d = JSON.parse(e.data); onHealth(d) } catch {}
  })
  es.onerror = (e) => { if (onError) onError(e) }
  return () => es.close()
}
