const UNITS = ['B', 'KB', 'MB', 'GB', 'TB']

export function bytes(n: number, digits = 1): string {
  if (n == null || n < 0 || !Number.isFinite(n)) return '—'
  if (n < 1024) return `${n} B`
  let i = 0
  let v = n
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 ? 0 : digits)} ${UNITS[i]}`
}

export function speed(bps: number): string {
  if (!bps || bps < 1) return '0 B/s'
  return `${bytes(bps)}/s`
}

export function eta(remaining: number, bps: number): string {
  if (!bps || bps < 1 || remaining <= 0) return '—'
  const s = Math.round(remaining / bps)
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`
  const h = Math.floor(s / 3600)
  if (h >= 48) return `${Math.floor(h / 24)}d`
  return `${h}h ${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}m`
}

export function percent(done: number, size: number): number {
  if (!size || size <= 0) return 0
  return Math.min(100, (done / size) * 100)
}

export function relTime(ms: number): string {
  if (!ms) return ''
  const d = (Date.now() - ms) / 1000
  if (d < 60) return 'just now'
  if (d < 3600) return `${Math.floor(d / 60)}m ago`
  if (d < 86400) return `${Math.floor(d / 3600)}h ago`
  if (d < 86400 * 7) return `${Math.floor(d / 86400)}d ago`
  return new Date(ms).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function host(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    return ''
  }
}

export function ext(name: string): string {
  const m = /\.([a-z0-9]{1,8})$/i.exec(name)
  return m ? m[1].toLowerCase() : ''
}

/** Parses "10 MB", "500k", "2m" into bytes; returns null if invalid. */
export function parseBytes(input: string): number | null {
  const m = /^\s*(\d+(?:\.\d+)?)\s*([kmgt]?)(?:i?b)?\s*$/i.exec(input)
  if (!m) return null
  const mult = { '': 1, k: 1024, m: 1024 ** 2, g: 1024 ** 3, t: 1024 ** 4 }[m[2].toLowerCase()] ?? 1
  return Math.round(parseFloat(m[1]) * mult)
}
