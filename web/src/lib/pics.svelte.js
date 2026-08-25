import { resolvePic } from './api.js'

// 统一的图片加载管理器（人物头像 / 条目封面 / 角色头像）。
//
// 所有请求——包括 pending 后的重试与等待——全部经由同一条 FIFO 队列调度：
// 相邻请求保持固定间隔；未到重试时间的项留在队列中，由泵统一休眠到点；
// 需要重试的项回到队尾，不阻塞其他图片。结果全局缓存，同页多组件共享。

const GAP_MS = 250 // 相邻两次请求的最小间隔
const RETRY_BASE_MS = 1200 // 重试基础间隔（随失败次数线性增长）
const RETRY_MAX_MS = 6000 // 单次重试间隔上限
const MAX_ATTEMPTS = 20 // 单张图片最大尝试次数，超过标记失败

// key 形如 `kind/id@size`（默认 l），同图片不同尺寸分别缓存。
// 值为 { status: 'loading' | 'ok' | 'failed', url }
export const picStore = $state({})

// 组件侧读取缓存：与 requestPic 的键规则保持一致。
export function picInfo(kind, id, size = 'l') {
  return picStore[`${kind}/${String(id)}@${size || 'l'}`] ?? null
}

let queue = [] // 待处理的队列 key（包含等待重试的）
const meta = new Map() // key -> { attempts, readyAt }
let running = false

// 请求某张图片；已在队列/已有结果的不会重复入队。
export function requestPic(kind, id, size = 'l') {
  if (!kind || !id || String(id) === 'null') return
  const key = `${kind}/${String(id)}@${size || 'l'}`
  if (picStore[key]) return
  picStore[key] = { status: 'loading', url: '' }
  meta.set(key, { attempts: 0, readyAt: 0 })
  queue.push(key)
  pump()
}

async function pump() {
  if (running) return
  running = true
  try {
    while (queue.length > 0) {
      const now = Date.now()
      const idx = queue.findIndex((k) => (meta.get(k)?.readyAt ?? 0) <= now)
      if (idx === -1) {
        // 队列里都是未到时间的重试项：统一休眠到最早的就绪时刻
        const nextAt = Math.min(...queue.map((k) => meta.get(k)?.readyAt ?? now))
        await sleep(Math.max(nextAt - now, 30))
        continue
      }
      const [key] = queue.splice(idx, 1)
      const outcome = await attemptOnce(key)
      if (outcome === 'retry') {
        const m = meta.get(key)
        m.attempts += 1
        if (m.attempts >= MAX_ATTEMPTS) {
          picStore[key] = { status: 'failed', url: '' }
          meta.delete(key)
        } else {
          m.readyAt = Date.now() + Math.min(RETRY_BASE_MS * m.attempts, RETRY_MAX_MS)
          queue.push(key) // 回队尾稍后重试
        }
      } else {
        meta.delete(key)
      }
      if (queue.length > 0) await sleep(GAP_MS)
    }
  } finally {
    running = false
  }
}

// 发起一次解析请求。返回 'ok' / 'failed'（终态），'retry' 表示需要重新排队。
async function attemptOnce(key) {
  const at = key.lastIndexOf('@')
  const slash = key.indexOf('/')
  const kind = key.slice(0, slash)
  const id = Number(key.slice(slash + 1, at))
  const size = key.slice(at + 1)
  try {
    const d = await resolvePic(kind, id, size)
    if (d?.status === 'ok' && d.url) {
      picStore[key] = { status: 'ok', url: d.url }
      return 'ok'
    }
    if (d?.status === 'failed') {
      picStore[key] = { status: 'failed', url: '' }
      return 'failed'
    }
  } catch {
    /* 网络或服务错误 */
  }
  return 'retry'
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms))
}
