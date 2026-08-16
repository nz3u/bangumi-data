const API_BASE = '/api'

export class ApiError extends Error {}

async function request(path, params) {
  const url = new URL(API_BASE + path, window.location.origin)
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value === undefined || value === null || value === '') continue
    url.searchParams.set(key, String(value))
  }
  const res = await fetch(url)
  let body
  try {
    body = await res.json()
  } catch {
    throw new ApiError(`HTTP ${res.status}: 响应不是 JSON`)
  }
  if (!res.ok || body.ok === false) throw new ApiError(body.error || `HTTP ${res.status}`)
  return body.data
}

export function health() {
  return request('/health')
}

export function stats() {
  return request('/stats')
}

export function constants() {
  return request('/constants')
}

// ---- 条目 ----

// 将表单状态映射为 /subjects/search 查询参数。
// 预留：复杂检索（多标签、多类型、收藏区间、关键词组合等）在此扩展，
// 视图层只需在表单中增加字段并在 buildSubjectQuery 中映射。
export function buildSubjectQuery(f) {
  return {
    q: f.q,
    type: f.type,
    platform: f.platform,
    tag: f.tag,
    meta_tag: f.metaTag,
    rank_min: f.rankMin,
    score_min: f.scoreMin,
    date_from: f.dateFrom,
    date_to: f.dateTo,
    nsfw: f.nsfw,
    series: f.series,
    sort: f.sort,
    order: f.order,
    page: f.page,
    size: f.size
  }
}

export function searchSubjects(filters) {
  return request('/subjects/search', buildSubjectQuery(filters))
}

export function getSubject(id) {
  return request(`/subjects/${id}`)
}

export function getSubjectEpisodes(id, params) {
  return request(`/subjects/${id}/episodes`, params)
}

// ---- 人物 ----

export function searchPersons(filters) {
  return request('/persons/search', {
    q: filters.q,
    type: filters.type,
    page: filters.page,
    size: filters.size
  })
}

export function getPerson(id) {
  return request(`/persons/${id}`)
}

export function getPersonWorks(id, params) {
  return request(`/persons/${id}/works`, params)
}

export function getPersonCollaborators(id, params) {
  return request(`/persons/${id}/collaborators`, params)
}

// ---- 角色 ----

export function searchCharacters(filters) {
  return request('/characters/search', {
    q: filters.q,
    role: filters.role,
    page: filters.page,
    size: filters.size
  })
}

export function getCharacter(id) {
  return request(`/characters/${id}`)
}