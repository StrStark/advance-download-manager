import { SvelteSet } from 'svelte/reactivity'
import { api } from './api'
import type { Batch, Job, NetLink, PathStat, Progress, Settings, Status } from './types'

export type Filter =
  | { kind: 'all' }
  | { kind: 'status'; status: 'active' | 'queued' | 'completed' | 'failed' | 'paused' }
  | { kind: 'category'; category: string }
  | { kind: 'batch'; id: string }

export type Row = { type: 'job'; job: Job } | { type: 'batch'; batch: Batch; jobs: Job[] }

export interface Toast {
  id: number
  kind: 'info' | 'success' | 'error'
  title: string
  body?: string
  action?: { label: string; run: () => void }
}

export interface BatchStats {
  total: number
  completed: number
  failed: number
  active: number
  paused: number
  queued: number
  size: number
  downloaded: number
  knownSize: boolean
  speed: number
}

export type StatusCounts = { all: number; active: number; queued: number; paused: number; completed: number; failed: number }

const HISTORY = 90

class AppStore {
  jobs = $state<Record<string, Job>>({})
  batches = $state<Record<string, Batch>>({})
  settings = $state<Settings>({
    downloadDir: '',
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
    proxies: [],
    subscriptions: [],
    defaultProxy: '',
    proxyBypass: ['localhost', '<local>'],
    multiLink: false,
    links: [],
  })
  /** The device's network connections (refreshed by the Network panel). */
  links = $state<NetLink[]>([])
  /** Live per-route stats for each running download. */
  paths = $state<Record<string, PathStat[]>>({})
  speed = $state<Record<string, number>>({})
  conns = $state<Record<string, number>>({})
  history = $state<number[]>(Array(HISTORY).fill(0))
  loaded = $state(false)
  online = $state(true)

  filter = $state<Filter>({ kind: 'all' })
  search = $state('')
  selected = new SvelteSet<string>()
  collapsed = new SvelteSet<string>()
  detailsId = $state<string | null>(null)
  /** Sidebar drawer on narrow screens. */
  navOpen = $state(false)

  addOpen = $state<{ url?: string } | null>(null)
  batchOpen = $state<{ text?: string; tab?: 'paste' | 'pattern' | 'import' } | null>(null)
  settingsOpen = $state(false)
  networkOpen = $state(false)
  confirmRemove = $state<{ ids: string[]; label: string } | null>(null)
  toasts = $state<Toast[]>([])
  private toastSeq = 0

  // ---------- derived ----------
  ordered = $derived(Object.values(this.jobs).sort((a, b) => a.position - b.position || a.createdAt - b.createdAt))

  totalSpeed = $derived(Object.values(this.speed).reduce((a, b) => a + b, 0))

  /** Live speed per network link across all downloads. */
  linkSpeed = $derived.by(() => {
    const out: Record<string, number> = {}
    for (const list of Object.values(this.paths)) for (const p of list) out[p.id] = (out[p.id] ?? 0) + p.speed
    return out
  })

  /** Links that multi-link downloads will use right now. */
  activeLinks = $derived(this.settings.multiLink ? this.links.filter((l) => l.enabled) : [])

  proxyName(choice: string | undefined): string {
    const c = choice || this.settings.defaultProxy
    if (!c || c === 'direct') return 'Direct'
    if (c === 'system') return 'System proxy'
    return this.settings.proxies?.find((p) => p.id === c)?.name ?? 'Missing proxy'
  }

  counts = $derived.by(() => {
    const c: StatusCounts = { all: 0, active: 0, queued: 0, paused: 0, completed: 0, failed: 0 }
    const cats: Record<string, number> = {}
    for (const j of this.ordered) {
      c.all++
      const g = group(j.status)
      c[g]++
      cats[j.category] = (cats[j.category] ?? 0) + 1
    }
    return { ...c, cats }
  })

  batchList = $derived(Object.values(this.batches).sort((a, b) => a.createdAt - b.createdAt))

  batchStats = $derived.by(() => {
    const out: Record<string, BatchStats> = {}
    for (const b of Object.values(this.batches)) {
      out[b.id] = { total: 0, completed: 0, failed: 0, active: 0, paused: 0, queued: 0, size: 0, downloaded: 0, knownSize: true, speed: 0 }
    }
    for (const j of this.ordered) {
      const s = j.batchId ? out[j.batchId] : undefined
      if (!s) continue
      s.total++
      const g = group(j.status)
      if (g === 'completed') s.completed++
      else if (g === 'failed') s.failed++
      else if (g === 'active') s.active++
      else if (g === 'paused') s.paused++
      else s.queued++
      if (j.size > 0) s.size += j.size
      else s.knownSize = false
      s.downloaded += j.downloaded
      s.speed += this.speed[j.id] ?? 0
    }
    return out
  })

  rows = $derived.by((): Row[] => {
    const q = this.search.trim().toLowerCase()
    const f = this.filter
    const match = (j: Job) =>
      (!q || j.filename.toLowerCase().includes(q) || j.url.toLowerCase().includes(q)) &&
      (f.kind === 'all' ||
        f.kind === 'batch' ||
        (f.kind === 'status' && group(j.status) === f.status) ||
        (f.kind === 'category' && j.category === f.category))

    if (f.kind === 'batch') {
      const b = this.batches[f.id]
      if (!b) return []
      return [{ type: 'batch', batch: b, jobs: this.ordered.filter((j) => j.batchId === f.id && match(j)) }]
    }
    // Group batches in "All" (and while searching); otherwise show a flat list.
    const grouped = f.kind === 'all'
    const rows: Row[] = []
    const seen = new Map<string, Job[]>()
    for (const j of this.ordered) {
      if (!match(j)) continue
      if (grouped && j.batchId && this.batches[j.batchId]) {
        let list = seen.get(j.batchId)
        if (!list) {
          list = []
          seen.set(j.batchId, list)
          rows.push({ type: 'batch', batch: this.batches[j.batchId], jobs: list })
        }
        list.push(j)
      } else {
        rows.push({ type: 'job', job: j })
      }
    }
    return rows
  })

  /** Flat list of job ids in display order (for keyboard/shift selection). */
  visibleIds = $derived(
    this.rows.flatMap((r) => (r.type === 'job' ? [r.job.id] : this.collapsed.has(r.batch.id) ? [] : r.jobs.map((j) => j.id))),
  )

  // ---------- lifecycle ----------
  /** Replaces all state with a fresh snapshot from the engine. */
  async resync() {
    const st = await api.getState()
    const jobs: Record<string, Job> = {}
    for (const j of st.jobs) jobs[j.id] = j
    this.jobs = jobs
    const batches: Record<string, Batch> = {}
    for (const b of st.batches) batches[b.id] = b
    this.batches = batches
    this.settings = st.settings
    this.speed = {}
    this.conns = {}
    this.loaded = true
  }

  async init() {
    await this.resync()
    api.on('connection:lost', () => (this.online = false))
    api.on('connection:restored', () => {
      this.online = true
      this.resync()
    })

    api.on('job', (j: Job) => {
      this.jobs[j.id] = j
      if (j.status !== 'downloading' && j.status !== 'probing') {
        delete this.speed[j.id]
        delete this.conns[j.id]
      }
    })
    api.on('progress', (list: Progress[]) => {
      const live = new Set<string>()
      for (const p of list) {
        live.add(p.id)
        const j = this.jobs[p.id]
        if (!j) continue
        j.downloaded = p.downloaded
        if (p.size > 0) j.size = p.size
        j.segments = p.segments
        this.speed[p.id] = p.speed
        this.conns[p.id] = p.conns
        if (p.paths?.length) this.paths[p.id] = p.paths
        else delete this.paths[p.id]
      }
      for (const k of Object.keys(this.speed)) if (!live.has(k)) delete this.speed[k]
      for (const k of Object.keys(this.paths)) if (!live.has(k)) delete this.paths[k]
      this.history = [...this.history.slice(1), list.reduce((a, p) => a + p.speed, 0)]
    })
    api.on('jobs:removed', (ids: string[]) => {
      for (const i of ids) {
        delete this.jobs[i]
        delete this.speed[i]
        this.selected.delete(i)
        if (this.detailsId === i) this.detailsId = null
      }
    })
    api.on('batch', (b: Batch) => (this.batches[b.id] = b))
    api.on('batch:removed', (bid: string) => {
      delete this.batches[bid]
      if (this.filter.kind === 'batch' && this.filter.id === bid) this.filter = { kind: 'all' }
    })
    api.on('settings', (s: Settings) => {
      this.settings = s
      this.refreshLinks()
    })
    api.on('links', () => this.refreshLinks())
    this.refreshLinks()
    api.on('job:done', (j: Job) => {
      const b = j.batchId ? this.batchStats[j.batchId] : null
      if (b && b.completed + b.failed === b.total) {
        const bn = this.batches[j.batchId!]?.name ?? 'Batch'
        this.toast(b.failed ? 'info' : 'success', `${bn} finished`, `${b.completed} of ${b.total} files${b.failed ? ` · ${b.failed} failed` : ''}`)
      } else if (!j.batchId) {
        this.toast('success', 'Download complete', j.filename, { label: 'Open', run: () => api.openFile(j.id) })
      }
    })
    const openUrls = (urls: string[]) => {
      if (!urls?.length) return
      if (urls.length === 1) this.addOpen = { url: urls[0] }
      else this.batchOpen = { text: urls.join('\n') }
    }
    api.on('external:urls', openUrls)
    api.pendingURLs().then(openUrls).catch(() => {})
    api.on('clipboard:url', (url: string) => {
      this.toast('info', 'Link copied', url, { label: 'Download', run: () => (this.addOpen = { url }) })
    })
    // Idle heartbeat so the speed graph decays to zero when nothing runs.
    setInterval(() => {
      if (Object.keys(this.speed).length === 0 && this.history[this.history.length - 1] !== 0) {
        this.history = [...this.history.slice(1), 0]
      }
    }, 400)
  }

  async refreshLinks() {
    try {
      this.links = (await api.listLinks()) ?? []
    } catch {
      /* older backend */
    }
  }

  // ---------- actions ----------
  toast(kind: Toast['kind'], title: string, body?: string, action?: Toast['action']) {
    const t: Toast = { id: ++this.toastSeq, kind, title, body, action }
    this.toasts = [...this.toasts.slice(-3), t]
    setTimeout(() => this.dismiss(t.id), action ? 7000 : 4500)
  }
  dismiss(id: number) {
    this.toasts = this.toasts.filter((t) => t.id !== id)
  }

  async run<T>(p: Promise<T>, err = 'Something went wrong'): Promise<T | undefined> {
    try {
      return await p
    } catch (e) {
      this.toast('error', err, String((e as Error)?.message ?? e))
    }
  }

  targetIds(id?: string): string[] {
    if (id && !this.selected.has(id)) return [id]
    return [...this.selected]
  }

  pause(ids: string[]) {
    return this.run(api.pause(ids))
  }
  resume(ids: string[]) {
    return this.run(api.resume(ids))
  }
  toggle(j: Job) {
    return isRunning(j.status) || j.status === 'queued' ? this.pause([j.id]) : this.resume([j.id])
  }
  pauseAll() {
    return this.pause(this.ordered.filter((j) => isRunning(j.status) || j.status === 'queued').map((j) => j.id))
  }
  resumeAll() {
    return this.resume(this.ordered.filter((j) => j.status === 'paused').map((j) => j.id))
  }
  askRemove(ids: string[]) {
    if (!ids.length) return
    const label = ids.length === 1 ? (this.jobs[ids[0]]?.filename ?? '1 download') : `${ids.length} downloads`
    this.confirmRemove = { ids, label }
  }
  async remove(ids: string[], deleteFiles: boolean) {
    await this.run(api.remove(ids, deleteFiles))
    this.confirmRemove = null
  }
  batchJobIds(bid: string) {
    return this.ordered.filter((j) => j.batchId === bid).map((j) => j.id)
  }

  select(id: string, e?: MouseEvent | KeyboardEvent) {
    if (e && (e.ctrlKey || e.metaKey)) {
      if (this.selected.has(id)) this.selected.delete(id)
      else this.selected.add(id)
      return
    }
    if (e?.shiftKey && this.selected.size) {
      const ids = this.visibleIds
      const last = [...this.selected].pop()!
      const a = ids.indexOf(last)
      const b = ids.indexOf(id)
      if (a >= 0 && b >= 0) {
        for (let i = Math.min(a, b); i <= Math.max(a, b); i++) this.selected.add(ids[i])
        return
      }
    }
    this.selected.clear()
    this.selected.add(id)
  }
}

export function group(s: Status): 'active' | 'queued' | 'paused' | 'completed' | 'failed' {
  if (s === 'downloading' || s === 'probing') return 'active'
  return s
}

export function isRunning(s: Status) {
  return s === 'downloading' || s === 'probing'
}

export const store = new AppStore()
