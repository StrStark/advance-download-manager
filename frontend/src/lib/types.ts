// Mirrors internal/core types (JSON field names).
export type Status = 'queued' | 'probing' | 'downloading' | 'paused' | 'completed' | 'failed'

export interface Segment {
  start: number
  end: number
  done: number
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
}

export interface Progress {
  id: string
  status: Status
  downloaded: number
  size: number
  speed: number
  segments: Segment[] | null
  conns: number
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
}

export interface BatchRequest {
  name: string
  dir: string
  maxParallel: number
  sequential: boolean
  connections: number
  paused: boolean
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
