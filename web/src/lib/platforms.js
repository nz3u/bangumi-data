import { load as yamlLoad } from 'js-yaml'

let cache = null

async function loadYaml() {
  if (cache) return cache
  const res = await fetch('/subject_platforms.yml')
  const text = await res.text()
  cache = yamlLoad(text)
  return cache
}

function parsePlatforms(doc) {
  const raw = doc?.platforms
  if (!raw) return {}

  const result = {}
  for (const [typeId, platforms] of Object.entries(raw)) {
    if (typeId === 'game_platforms') continue
    const id = Number(typeId)
    if (isNaN(id)) continue
    const map = {}
    for (const [pid, p] of Object.entries(platforms)) {
      if (pid === 'game_platforms') continue
      const numId = Number(pid)
      if (isNaN(numId)) continue
      map[numId] = {
        id: numId,
        type: p.type ?? '',
        type_cn: p.type_cn ?? p.type ?? `子类型${numId}`,
        alias: p.alias ?? ''
      }
    }
    result[id] = map
  }

  // merge game_platforms into type 4
  const gamePlatforms = raw.game_platforms ?? raw[4]?.game_platforms
  if (gamePlatforms && result[4]) {
    for (const [pid, p] of Object.entries(gamePlatforms)) {
      const numId = Number(pid)
      if (isNaN(numId)) continue
      result[4][numId] = {
        id: numId,
        type: p.type ?? '',
        type_cn: p.type_cn ?? p.type ?? `子类型${numId}`,
        alias: p.alias ?? ''
      }
    }
  }

  return result
}

export async function getPlatforms() {
  const doc = await loadYaml()
  return parsePlatforms(doc)
}

export async function platformsFor(typeId) {
  const all = await getPlatforms()
  const map = all[Number(typeId)]
  if (!map) return []
  return Object.values(map).sort((a, b) => a.id - b.id)
}

export async function platformCN(typeId, platformId) {
  const all = await getPlatforms()
  const map = all[Number(typeId)]
  if (!map) return `子类型${platformId}`
  const p = map[Number(platformId)]
  return p?.type_cn ?? (platformId === 0 ? '其他' : `子类型${platformId}`)
}
