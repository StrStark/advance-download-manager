// Mirrors internal/core types (JSON field names).
export type Status = 'queued' | 'probing' | 'downloading' | 'paused' | 'completed' | 'failed'

export interface Segment {
  start: number
  end: number
  done: number
  /** Route (network link) this segment uses when downloading over several. */
  path?: string
}

export interface Job {
  id: string
  batchId?: string
  url: string
  finalUrl?: string
  filename: string
  dir: string
  category: string
  size: number
  downloaded: number
  status: Status
  error?: string
  resumable: boolean
  singleConn?: boolean
  probed: boolean
  etag?: string
  lastModified?: string
  contentType?: string
  connections: number
  segments: Segment[] | null
  headers?: Record<string, string>
  /** "" = default, "direct", "system", or a proxy profile ID */
  proxy?: string
  position: number
  retries: number
  createdAt: number
  completedAt?: number
}

export interface Batch {
  id: string
  name: string
  dir: string
  maxParallel: number
  sequential: boolean
  createdAt: number
}

export interface Settings {
  downloadDir: string
  maxActive: number
  defaultConnections: number
  speedLimit: number
  maxRetries: number
  perHostLimit: number
  userAgent: string
  theme: 'system' | 'dark' | 'light'
  categorizeByType: boolean
  clipboardWatch: boolean
  notifyOnComplete: boolean
  proxies: ProxyProfile[] | null
  subscriptions: Subscription[] | null
  defaultProxy: string
  proxyBypass: string[] | null
  multiLink: boolean
  links: string[] | null
}

export type ProxyType = 'http' | 'https' | 'socks5' | 'vmess' | 'vless' | 'trojan' | 'shadowsocks'

export interface ProxyProfile {
  id: string
  name: string
  type: ProxyType
  url: string
  server?: string
  subscriptionId?: string
}

export interface Subscription {
  id: string
  name: string
  url: string
  updatedAt: number
}

export type LinkKind = 'ethernet' | 'wifi' | 'cellular' | 'usb' | 'vpn' | 'other'

export interface NetLink {
  id: string
  name: string
  label: string
  kind: LinkKind
  addrs: string[] | null
  enabled: boolean
}

export interface LinkCheck {
  latencyMs: number
  error?: string
}

export interface ProxyTest {
  ok: boolean
  latencyMs: number
  ip?: string
  error?: string
}

export interface ProxyParse {
  profiles: ProxyProfile[] | null
  errors: string[] | null
}

export interface PathStat {
  id: string
  label: string
  kind: string
  speed: number
  conns: number
}

export interface Progress {
  id: string
  status: Status
  downloaded: number
  size: number
  speed: number
  segments: Segment[] | null
  conns: number
  paths?: PathStat[] | null
}

export interface ProbeResult {
  url: string
  finalUrl: string
  filename: string
  size: number
  resumable: boolean
  etag?: string
  contentType?: string
  category: string
  error?: string
}

export interface AddRequest {
  url: string
  filename?: string
  dir?: string
  connections?: number
  headers?: Record<string, string>
  paused?: boolean
  size?: number
  proxy?: string
}

export interface BatchRequest {
  name: string
  dir: string
  maxParallel: number
  sequential: boolean
  connections: number
  paused: boolean
  proxy?: string
  items: AddRequest[]
}

export interface ImportItem {
  url: string
  filename?: string
  dir?: string
}

export interface State {
  jobs: Job[]
  batches: Batch[]
  settings: Settings
}
