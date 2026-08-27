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

// 数据库版本与上游最新导出对比（右下角徽标与更新提醒）
export function dbInfo() {
  return request('/dbinfo')
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

// 标签/元标签候选池（条目搜索的实时建议数据源）。
// kind: 'tag'（普通标签）| 'meta'（元标签）| 'all'（合并不区分）；按使用次数降序，最多 5000 条。
// 前端一次拉取后本地做子串 + 拼音首字母过滤（见 components/TagSuggest.svelte），
// 模块级缓存避免重复请求。
const tagPoolCache = new Map()
export function loadTagPool(kind) {
  if (!tagPoolCache.has(kind)) {
    tagPoolCache.set(
      kind,
      request('/subjects/tags', { kind, limit: 5000 })
        .then((d) => d.items ?? [])
        .catch((e) => {
          tagPoolCache.delete(kind)
          throw e
        })
    )
  }
  return tagPoolCache.get(kind)
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

// 图片解析（轮询接口）：data = {status:'ok'|'pending'|'failed', url?}
// kind: 'person'（人物头像）| 'subject'（条目封面）| 'character'（角色头像）
// size: 'l'|'m'|'s'|'grid'，决定返回的 CDN 尺寸（小头像建议 grid）
export function resolvePic(kind, id, size) {
  return request(`/pics/${kind}/${id}`, { size })
}

export function getPersonWorks(id, params) {
  return request(`/persons/${id}/works`, params)
}

export function getPersonCollaborators(id, params) {
  return request(`/persons/${id}/collaborators`, params)
}

export function getPersonCollaboration(id, params) {
  return request(`/persons/${id}/collaboration`, params)
}

// 棋盘筛选职位标签：self = 当前人物在共同条目中的职位，other = 合作人物的职位
export function getPersonCollaborationPositions(id) {
  return request(`/persons/${id}/collaboration/positions`)
}

export function getPairCollaboration(idA, idB) {
  return request(`/persons/${idA}/collaboration/${idB}`)
}

export function getPersonRoles(id) {
  return request(`/persons/${id}/roles`)
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