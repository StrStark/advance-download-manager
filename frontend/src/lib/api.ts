import type {
  AddRequest,
  AppInfo,
  ExternalDownload,
  UpdateInfo,
  Batch,
  BatchRequest,
  ImportItem,
  Job,
  LinkCheck,
  NetLink,
  ProbeResult,
  ProxyParse,
  ProxyProfile,
  ProxyTest,
  Settings,
  State,
} from './types'
import { createMockBackend } from './mock'

/**
 * Where the UI is running:
 * - desktop: inside the Wails app, calling Go directly
 * - server:  in a browser, talking to `adm-server` over HTTP + server-sent events
 * - android: in the Android app's WebView; same HTTP API on a loopback port,
 *            plus a small native bridge (window.ADMAndroid)
 * - mock:    in a browser with no backend (UI development)
 */
export type BackendKind = 'desktop' | 'server' | 'android' | 'mock'

export interface Backend {
  readonly kind: BackendKind
  getState(): Promise<State>
  addDownload(req: AddRequest): Promise<Job>
  probe(url: string, headers?: Record<string, string>, proxy?: string): Promise<ProbeResult>
  /** Probes asynchronously; results arrive as `probe:result` events. */
  probeMany(session: string, urls: string[], proxy?: string): Promise<void>
  extractURLs(text: string): Promise<string[]>
  expandPattern(pattern: string): Promise<string[]>
  /** Must be called directly from a click handler (the browser needs a user gesture). */
  importFile(): Promise<ImportItem[]>
  createBatch(req: BatchRequest): Promise<Batch>
  pause(ids: string[]): Promise<void>
  resume(ids: string[]): Promise<void>
  remove(ids: string[], deleteFiles: boolean): Promise<void>
  retryFailed(batchId: string): Promise<void>
  updateBatch(b: Batch): Promise<void>
  openFile(id: string): Promise<void>
  showInFolder(id: string): Promise<void>
  /** URL that downloads a finished file to the viewer's computer (server mode). */
  fileURL(id: string): string | null
  chooseDirectory(current: string): Promise<string>
  updateSettings(s: Settings): Promise<Settings>
  setSpeedLimit(bps: number): Promise<void>
  diskFree(dir: string): Promise<number>
  /** URLs passed on the command line at launch (returned once). */
  pendingURLs(): Promise<string[]>
  // ---- network ----
  listLinks(): Promise<NetLink[]>
  checkLinks(): Promise<Record<string, LinkCheck>>
  parseProxies(text: string): Promise<ProxyParse>
  fetchSubscription(url: string, via: string): Promise<ProxyParse>
  testProxy(p: ProxyProfile): Promise<ProxyTest>
  xrayAvailable(): Promise<boolean>
  // ---- app ----
  appInfo(): Promise<AppInfo>
  checkUpdate(): Promise<UpdateInfo>
  /** Downloads and installs the update; progress arrives as `update:progress`. */
  installUpdate(): Promise<void>
  /** Browser downloads handed over at launch (returned once). */
  pendingDownloads(): Promise<ExternalDownload[]>
  on(event: string, cb: (data: any) => void): () => void
}

/** Native helpers injected by the Android app (MainActivity). */
interface AndroidBridge {
  openFile(id: string): void
  showDownloads(): void
}

declare global {
  interface Window {
    ADMAndroid?: AndroidBridge
    /** Called by Android's back button; returns true if the UI handled it. */
    __admBack?: () => boolean
    go?: { main: { App: Record<string, (...args: any[]) => Promise<any>> } }
    runtime?: {
      EventsOn(name: string, cb: (...data: any[]) => void): () => void
      [k: string]: any
    }
  }
}

function createDesktopBackend(): Backend {
  const app = () => window.go!.main.App
  return {
    kind: 'desktop',
    getState: () => app().GetState(),
    addDownload: (r) => app().AddDownload(r),
    probe: (u, h, p) => app().Probe(u, h ?? {}, p ?? ''),
    probeMany: (s, u, p) => app().ProbeMany(s, u, p ?? ''),
    extractURLs: (t) => app().ExtractURLs(t),
    expandPattern: (p) => app().ExpandPattern(p),
    importFile: () => app().ImportFile(),
    createBatch: (r) => app().CreateBatch(r),
    pause: (ids) => app().Pause(ids),
    resume: (ids) => app().Resume(ids),
    remove: (ids, del) => app().Remove(ids, del),
    retryFailed: (b) => app().RetryFailed(b),
    updateBatch: (b) => app().UpdateBatch(b),
    openFile: (id) => app().OpenFile(id),
    showInFolder: (id) => app().ShowInFolder(id),
    fileURL: () => null,
    chooseDirectory: (c) => app().ChooseDirectory(c),
    updateSettings: (s) => app().UpdateSettings(s),
    setSpeedLimit: (b) => app().SetSpeedLimit(b),
    diskFree: (d) => app().DiskFree(d),
    pendingURLs: () => app().PendingURLs(),
    listLinks: () => app().ListLinks(),
    checkLinks: () => app().CheckLinks(),
    parseProxies: (t) => app().ParseProxies(t),
    fetchSubscription: (u, v) => app().FetchSubscription(u, v),
    testProxy: (p) => app().TestProxy(p),
    xrayAvailable: () => app().XrayAvailable(),
    appInfo: () => app().AppInfo(),
    checkUpdate: () => app().CheckUpdate(),
    installUpdate: () => app().InstallUpdate(),
    pendingDownloads: () => app().PendingDownloads(),
    on: (ev, cb) => window.runtime!.EventsOn(ev, cb),
  }
}

function createServerBackend(kind: 'server' | 'android'): Backend {
  async function call<T = any>(method: string, ...args: unknown[]): Promise<T> {
    const res = await fetch(`api/call/${method}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(args),
    })
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
    const body = await res.json()
    if (body.error) throw new Error(body.error)
    return body.result as T
  }

  const listeners = new Map<string, Set<(d: any) => void>>()
  const dispatch = (name: string, data: any) => listeners.get(name)?.forEach((cb) => cb(data))
  let connectedOnce = false
  const es = new EventSource('api/events')
  es.onmessage = (e) => {
    const msg = JSON.parse(e.data) as { name: string; data: any }
    dispatch(msg.name, msg.data)
  }
  es.onopen = () => {
    // After a reconnect we may have missed events: ask the store to resync.
    if (connectedOnce) dispatch('connection:restored', null)
    connectedOnce = true
  }
  es.onerror = () => dispatch('connection:lost', null)

  const android = kind === 'android' ? window.ADMAndroid : undefined

  return {
    kind,
    getState: () => call('GetState'),
    addDownload: (r) => call('AddDownload', r),
    probe: (u, h, p) => call('Probe', u, h ?? {}, p ?? ''),
    probeMany: (s, u, p) => call('ProbeMany', s, u, p ?? ''),
    extractURLs: (t) => call('ExtractURLs', t),
    expandPattern: (p) => call('ExpandPattern', p),
    importFile: () =>
      new Promise((resolve, reject) => {
        const input = document.createElement('input')
        input.type = 'file'
        input.accept = '.txt,.csv,.json,.list,text/plain'
        input.onchange = async () => {
          const f = input.files?.[0]
          if (!f) return resolve([])
          try {
            resolve(await call('ParseImport', f.name, await f.text()))
          } catch (e) {
            reject(e)
          }
        }
        input.oncancel = () => resolve([])
        input.click()
      }),
    createBatch: (r) => call('CreateBatch', r),
    pause: (ids) => call('Pause', ids),
    resume: (ids) => call('Resume', ids),
    remove: (ids, del) => call('Remove', ids, del),
    retryFailed: (b) => call('RetryFailed', b),
    updateBatch: (b) => call('UpdateBatch', b),
    openFile: async (id) => {
      if (android) android.openFile(id)
      else window.open(`api/file/${encodeURIComponent(id)}`, '_blank')
    },
    showInFolder: async () => android?.showDownloads(),
    // On a phone the file is already on the device, so offer Open instead.
    fileURL: (id) => (android ? null : `api/file/${encodeURIComponent(id)}`),
    chooseDirectory: async (c) => c,
    updateSettings: (s) => call('UpdateSettings', s),
    setSpeedLimit: (b) => call('SetSpeedLimit', b),
    diskFree: (d) => call('DiskFree', d),
    pendingURLs: async () => [],
    listLinks: () => call('ListLinks'),
    checkLinks: () => call('CheckLinks'),
    parseProxies: (t) => call('ParseProxies', t),
    fetchSubscription: (u, v) => call('FetchSubscription', u, v),
    testProxy: (p) => call('TestProxy', p),
    xrayAvailable: () => call('XrayAvailable'),
    appInfo: () => call('AppInfo'),
    checkUpdate: () => call('CheckUpdate'),
    installUpdate: () => call('InstallUpdate'),
    pendingDownloads: async () => [],
    on(ev, cb) {
      if (!listeners.has(ev)) listeners.set(ev, new Set())
      listeners.get(ev)!.add(cb)
      return () => listeners.get(ev)?.delete(cb)
    },
  }
}

async function detect(): Promise<Backend> {
  if (window.go?.main?.App) return createDesktopBackend()
  try {
    const res = await fetch('api/ping', { cache: 'no-store' })
    const kind = res.ok ? (await res.json()).kind : null
    if (kind === 'server' || kind === 'android') return createServerBackend(kind)
  } catch {
    /* no server */
  }
  return createMockBackend()
}

/** The active backend; assigned by initApi() before the app mounts. */
export let api: Backend

export async function initApi() {
  api = await detect()
}
