// A simulated engine so the UI can be developed and previewed in a normal
// browser (`npm run dev`) without the Go backend.
import type { Backend } from './api'
import type { Batch, Job, Progress, ProbeResult, Segment, Settings, State } from './types'
import { expandPattern, extractURLs, filenameFromURL } from './batchutil'

const MB = 1024 * 1024
const GB = 1024 * MB

const CATS: Record<string, string[]> = {
  video: ['mp4', 'mkv', 'webm', 'mov', 'avi'],
  music: ['mp3', 'flac', 'ogg', 'opus', 'wav', 'm4a'],
  archives: ['zip', 'rar', '7z', 'tar', 'gz', 'xz', 'zst', 'iso', 'img'],
  documents: ['pdf', 'docx', 'odt', 'xlsx', 'epub', 'txt', 'md', 'csv'],
  images: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'avif'],
  programs: ['deb', 'rpm', 'appimage', 'flatpak', 'sh', 'run', 'exe'],
}
function category(name: string): string {
  const e = name.toLowerCase().split('.').pop() ?? ''
  for (const [c, exts] of Object.entries(CATS)) if (exts.includes(e)) return c
  return 'other'
}

function hash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619)
  return h >>> 0
}

let seq = 0
const id = () => (Date.now().toString(36) + (seq++).toString(36) + Math.random().toString(36).slice(2, 6)).slice(-12)

function split(size: number, n: number, doneFrac = 0): Segment[] {
  const part = Math.floor(size / n)
  return Array.from({ length: n }, (_, i) => {
    const start = i * part
    const end = i === n - 1 ? size - 1 : start + part - 1
    const len = end - start + 1
    const jitter = Math.min(1, Math.max(0, doneFrac + (Math.sin(i * 7.3) * 0.18)))
    return { start, end, done: Math.floor(len * jitter) }
  })
}

export function createMockBackend(): Backend {
  const listeners = new Map<string, Set<(d: any) => void>>()
  const emit = (ev: string, d: any) => listeners.get(ev)?.forEach((cb) => cb(structuredClone(d)))

  const settings: Settings = {
    downloadDir: '~/Downloads',
    maxActive: 3,
    defaultConnections: 8,
    speedLimit: 0,
    maxRetries: 6,
    perHostLimit: 16,
    userAgent: '',
    theme: 'system',
    categorizeByType: false,
    clipboardWatch: false,
    notifyOnComplete: true,
  }
  const jobs: Job[] = []
  const batches: Batch[] = []
  const speed = new Map<string, number>()
  let pos = 0

  function mk(p: Partial<Job> & { url: string; size: number }): Job {
    const filename = p.filename ?? filenameFromURL(p.url)
    const j: Job = {
      id: id(),
      finalUrl: p.url,
      filename,
      dir: settings.downloadDir,
      category: category(filename),
      downloaded: 0,
      status: 'queued',
      resumable: true,
      probed: true,
      connections: 8,
      segments: [],
      position: pos++,
      retries: 0,
      createdAt: Date.now() - 1000 * 60 * (60 - pos),
      contentType: 'application/octet-stream',
      etag: `"${hash(p.url).toString(16)}"`,
      ...p,
    } as Job
    j.category = category(j.filename)
    return j
  }

  // ----- demo data -----
  const iso = mk({ url: 'https://releases.ubuntu.com/26.04/ubuntu-26.04-desktop-amd64.iso', size: 5.8 * GB, connections: 16, status: 'downloading' })
  iso.segments = split(iso.size, 16, 0.53)
  jobs.push(iso)

  const season: Batch = { id: id(), name: 'Studio Shorts — Season 1', dir: '~/Videos/Studio Shorts', maxParallel: 2, sequential: false, createdAt: Date.now() - 3600e3 }
  batches.push(season)
  const epStates: Job['status'][] = ['completed', 'completed', 'completed', 'downloading', 'failed', 'queued', 'queued', 'queued']
  epStates.forEach((st, i) => {
    const size = Math.round((0.9 + ((hash('ep' + i) % 100) / 100) * 0.7) * GB)
    const j = mk({
      url: `https://cdn.example.org/shorts/s01/S01E${String(i + 1).padStart(2, '0')}.mkv`,
      size,
      batchId: season.id,
      dir: season.dir,
      status: st,
      connections: 8,
    })
    if (st === 'completed') {
      j.downloaded = size
      j.segments = split(size, 8, 1)
      j.completedAt = Date.now() - (8 - i) * 600e3
    } else if (st === 'downloading') {
      j.segments = split(size, 8, 0.41)
    } else if (st === 'failed') {
      j.segments = split(size, 8, 0.12)
      j.error = 'server returned 403 Forbidden'
      j.status = 'failed'
    }
    j.downloaded = (j.segments ?? []).reduce((a, s) => a + s.done, 0)
    jobs.push(j)
  })

  const data = mk({ url: 'https://data.example.net/exports/dataset-2026.tar.zst', size: 40.2 * GB, status: 'paused', connections: 12 })
  data.segments = split(data.size, 12, 0.3)
  data.downloaded = data.segments.reduce((a, s) => a + s.done, 0)
  jobs.push(data)

  const walls: Batch = { id: id(), name: 'Wallpapers', dir: '~/Pictures/Wallpapers', maxParallel: 4, sequential: false, createdAt: Date.now() - 86400e3 }
  batches.push(walls)
  for (let i = 1; i <= 6; i++) {
    const size = Math.round((3 + (hash('w' + i) % 70) / 10) * MB)
    jobs.push(mk({ url: `https://img.example.com/walls/aurora-${String(i).padStart(3, '0')}.jpg`, size, batchId: walls.id, dir: walls.dir, status: 'completed', downloaded: size, segments: split(size, 4, 1), completedAt: Date.now() - 86000e3, connections: 4 }))
  }

  const done = [
    mk({ url: 'https://example.com/reports/report-final.pdf', size: 4.2 * MB, status: 'completed' }),
    mk({ url: 'https://nodejs.org/dist/v24.9.0/node-v24.9.0-linux-x64.tar.xz', size: 29.6 * MB, status: 'completed' }),
    mk({ url: 'https://downloads.example.dev/Toolbox-2.4.1.AppImage', size: 212 * MB, status: 'completed' }),
    mk({ url: 'https://podcasts.example.fm/episodes/episode-112-the-long-tail.mp3', size: 88 * MB, status: 'queued' }),
  ]
  done.forEach((j, i) => {
    if (j.status === 'completed') {
      j.downloaded = j.size
      j.segments = split(j.size, Math.max(1, Math.min(8, Math.floor(j.size / MB))), 1)
      j.completedAt = Date.now() - (i + 1) * 5400e3
    }
    jobs.push(j)
  })

  // ----- simulation -----
  const find = (jid: string) => jobs.find((j) => j.id === jid)
  const active = () => jobs.filter((j) => j.status === 'downloading' || j.status === 'probing')

  function schedule() {
    let n = active().length
    const perBatch = new Map<string, number>()
    active().forEach((j) => j.batchId && perBatch.set(j.batchId, (perBatch.get(j.batchId) ?? 0) + 1))
    for (const j of [...jobs].sort((a, b) => a.position - b.position)) {
      if (n >= settings.maxActive) break
      if (j.status !== 'queued') continue
      const b = batches.find((x) => x.id === j.batchId)
      if (b) {
        const lim = b.sequential ? 1 : b.maxParallel
        if ((perBatch.get(b.id) ?? 0) >= lim) continue
        perBatch.set(b.id, (perBatch.get(b.id) ?? 0) + 1)
      }
      j.status = 'probing'
      n++
      emit('job', j)
      setTimeout(() => {
        if (j.status !== 'probing') return
        if (!j.segments?.length) j.segments = split(j.size, Math.max(1, Math.min(j.connections, Math.floor(j.size / MB))))
        j.status = 'downloading'
        emit('job', j)
      }, 500 + Math.random() * 600)
    }
  }

  const TICK = 400
  setInterval(() => {
    const prog: Progress[] = []
    const limit = settings.speedLimit
    const act = active()
    for (const j of act) {
      if (j.status !== 'downloading') {
        prog.push({ id: j.id, status: j.status, downloaded: j.downloaded, size: j.size, speed: 0, segments: j.segments, conns: 0 })
        continue
      }
      const base = speed.get(j.id) ?? (6 + (hash(j.id) % 20)) * MB
      let s = Math.max(1.5 * MB, base * (0.9 + Math.random() * 0.2))
      if (limit > 0) s = Math.min(s, limit / Math.max(1, act.length))
      speed.set(j.id, s)
      let budget = (s * TICK) / 1000
      const live = (j.segments ?? []).filter((x) => x.done < x.end - x.start + 1)
      for (const seg of live) {
        const want = Math.min(budget / live.length * (0.6 + Math.random() * 0.8), seg.end - seg.start + 1 - seg.done)
        seg.done += Math.floor(want)
      }
      // work stealing: split the biggest remaining segment when one finishes
      const segs = j.segments ?? []
      const finished = segs.filter((x) => x.done >= x.end - x.start + 1).length
      const running = segs.length - finished
      if (running < j.connections && segs.length < 32) {
        const big = segs.reduce<Segment | null>((a, x) => {
          const r = x.end - x.start + 1 - x.done
          return r > 2 * MB && (!a || r > a.end - a.start + 1 - a.done) ? x : a
        }, null)
        if (big) {
          const pos = big.start + big.done
          const mid = pos + Math.floor((big.end - pos + 1) / 2)
          segs.push({ start: mid, end: big.end, done: 0 })
          big.end = mid - 1
          segs.sort((a, b) => a.start - b.start)
        }
      }
      j.downloaded = segs.reduce((a, x) => a + x.done, 0)
      prog.push({ id: j.id, status: j.status, downloaded: j.downloaded, size: j.size, speed: s, segments: segs, conns: Math.min(j.connections, segs.filter((x) => x.done < x.end - x.start + 1).length) })
      if (j.downloaded >= j.size) {
        j.downloaded = j.size
        j.status = 'completed'
        j.completedAt = Date.now()
        speed.delete(j.id)
        emit('job', j)
        emit('job:done', j)
      }
    }
    if (prog.length) emit('progress', prog)
    schedule()
  }, TICK)

  const snapshot = (): State => structuredClone({ jobs: [...jobs].sort((a, b) => a.position - b.position), batches, settings })
  const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

  function fakeProbe(url: string): ProbeResult {
    const filename = filenameFromURL(url)
    const h = hash(url)
    if (/404|missing/.test(url)) return { url, finalUrl: url, filename, size: -1, resumable: false, category: category(filename), error: 'server returned 404 Not Found' }
    const cat = category(filename)
    const scale = cat === 'video' ? 900 * MB : cat === 'archives' ? 600 * MB : cat === 'images' ? 4 * MB : cat === 'music' ? 60 * MB : 25 * MB
    return { url, finalUrl: url, filename, size: Math.round(scale * (0.3 + (h % 1000) / 700)), resumable: h % 9 !== 0, category: cat, contentType: 'application/octet-stream' }
  }

  function add(r: { url: string; filename?: string; dir?: string; connections?: number; paused?: boolean; size?: number; headers?: Record<string, string> }, batchId?: string): Job {
    const pr = fakeProbe(r.url)
    const j = mk({
      url: r.url,
      size: r.size && r.size > 0 ? r.size : pr.size > 0 ? pr.size : 50 * MB,
      filename: r.filename || pr.filename,
      dir: r.dir || settings.downloadDir,
      connections: r.connections || settings.defaultConnections,
      status: r.paused ? 'paused' : 'queued',
      batchId,
      headers: r.headers,
      createdAt: Date.now(),
    })
    j.resumable = pr.resumable
    jobs.push(j)
    return j
  }

  return {
    kind: 'mock',
    fileURL: () => null,
    async getState() {
      return snapshot()
    },
    async addDownload(r) {
      const j = add(r)
      emit('job', j)
      return structuredClone(j)
    },
    async probe(url) {
      await wait(350 + Math.random() * 500)
      return fakeProbe(url)
    },
    async probeMany(session, urls) {
      urls.forEach((u, index) =>
        setTimeout(() => emit('probe:result', { session, index, result: fakeProbe(u) }), 150 + index * 35 + Math.random() * 400),
      )
    },
    async extractURLs(t) {
      return extractURLs(t)
    },
    async expandPattern(p) {
      return expandPattern(p)
    },
    async importFile() {
      await wait(200)
      return Array.from({ length: 12 }, (_, i) => ({ url: `https://files.example.org/lectures/lecture-${String(i + 1).padStart(2, '0')}.mp4` }))
    },
    async createBatch(req) {
      const b: Batch = { id: id(), name: req.name || 'New batch', dir: req.dir || settings.downloadDir, maxParallel: req.maxParallel || 3, sequential: req.sequential, createdAt: Date.now() }
      batches.push(b)
      emit('batch', b)
      for (const it of req.items) {
        const j = add({ ...it, dir: it.dir || b.dir, connections: it.connections || req.connections, paused: req.paused || it.paused }, b.id)
        emit('job', j)
      }
      return b
    },
    async pause(ids) {
      ids.forEach((i) => {
        const j = find(i)
        if (j && j.status !== 'completed') {
          j.status = 'paused'
          emit('job', j)
        }
      })
    },
    async resume(ids) {
      ids.forEach((i) => {
        const j = find(i)
        if (j && (j.status === 'paused' || j.status === 'failed')) {
          j.status = 'queued'
          j.error = undefined
          emit('job', j)
        }
      })
    },
    async remove(ids) {
      for (const i of ids) {
        const k = jobs.findIndex((j) => j.id === i)
        if (k >= 0) jobs.splice(k, 1)
      }
      emit('jobs:removed', ids)
      for (let k = batches.length - 1; k >= 0; k--) {
        if (!jobs.some((j) => j.batchId === batches[k].id)) {
          emit('batch:removed', batches[k].id)
          batches.splice(k, 1)
        }
      }
    },
    async retryFailed(bid) {
      const ids = jobs.filter((j) => j.status === 'failed' && (!bid || j.batchId === bid)).map((j) => j.id)
      await this.resume(ids)
    },
    async updateBatch(b) {
      const cur = batches.find((x) => x.id === b.id)
      if (cur) {
        Object.assign(cur, { name: b.name || cur.name, maxParallel: b.maxParallel || cur.maxParallel, sequential: b.sequential })
        emit('batch', cur)
      }
    },
    async openFile() {},
    async showInFolder() {},
    async chooseDirectory(current) {
      return current
    },
    async updateSettings(s) {
      Object.assign(settings, s)
      emit('settings', settings)
      return structuredClone(settings)
    },
    async setSpeedLimit(bps) {
      settings.speedLimit = bps
      emit('settings', settings)
    },
    async diskFree() {
      return 412.6 * GB
    },
    async pendingURLs() {
      return []
    },
    on(ev, cb) {
      if (!listeners.has(ev)) listeners.set(ev, new Set())
      listeners.get(ev)!.add(cb)
      return () => listeners.get(ev)?.delete(cb)
    },
  }
}
