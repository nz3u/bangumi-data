import { constants } from './api.js'

let cache = null

export async function loadConstants() {
  if (!cache) cache = await constants()
  return cache
}

export function enumList(map) {
  return Object.entries(map ?? {})
    .map(([id, name]) => ({ id: Number(id), name }))
    .sort((a, b) => a.id - b.id)
}

export function platformsFor(cons, type) {
  const map = cons?.platforms?.[String(type)]
  if (!map) return []
  return Object.entries(map)
    .map(([id, p]) => ({ id: Number(id), name: p.type_cn || p.type || `平台${id}` }))
    .sort((a, b) => a.id - b.id)
}