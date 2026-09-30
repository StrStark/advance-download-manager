// Client-side helpers for the batch dialog. The Go side has equivalent
// implementations (internal/batch); these power the live previews and the
// browser mock.

export const MAX_ITEMS = 10000
const PATTERN_SRC = String.raw`\[([0-9]+|[a-zA-Z])-([0-9]+|[a-zA-Z])(?::([0-9]+))?\]`

export function hasPattern(s: string): boolean {
  return new RegExp(PATTERN_SRC).test(s)
}

function expandRange(from: string, to: string, step: number): string[] {
  const out: string[] = []
  const fa = Number(from)
  const ta = Number(to)
  if (/^\d+$/.test(from) && /^\d+$/.test(to)) {
    const width = from.length > 1 && from[0] === '0' ? from.length : 0
    const inc = ta < fa ? -step : step
    for (let v = fa; inc > 0 ? v <= ta : v >= ta; v += inc) {
      if (out.length >= MAX_ITEMS) throw new Error('Range too large')
      out.push(String(v).padStart(width, '0'))
    }
    return out
  }
  if (/^[a-z]$/i.test(from) && /^[a-z]$/i.test(to)) {
    const a = from.charCodeAt(0)
    const b = to.charCodeAt(0)
    const inc = b < a ? -step : step
    for (let c = a; inc > 0 ? c <= b : c >= b; c += inc) out.push(String.fromCharCode(c))
    return out
  }
  throw new Error(`Invalid range [${from}-${to}]`)
}

export function expandPattern(p: string): string[] {
  const matches = [...p.matchAll(new RegExp(PATTERN_SRC, 'g'))]
  if (matches.length === 0) return [p]
  const lits: string[] = []
  const ranges: string[][] = []
  let prev = 0
  let total = 1
  for (const m of matches) {
    lits.push(p.slice(prev, m.index))
    prev = m.index! + m[0].length
    const step = m[3] ? Number(m[3]) : 1
    if (step <= 0) throw new Error('Step must be positive')
    const vals = expandRange(m[1], m[2], step)
    total *= vals.length
    if (total > MAX_ITEMS) throw new Error(`Pattern expands to more than ${MAX_ITEMS.toLocaleString()} URLs`)
    ranges.push(vals)
  }
  lits.push(p.slice(prev))
  const out: string[] = []
  const idx = new Array(ranges.length).fill(0)
  for (;;) {
    let s = ''
    ranges.forEach((r, i) => (s += lits[i] + r[idx[i]]))
    out.push(s + lits[lits.length - 1])
    let k = idx.length - 1
    while (k >= 0) {
      idx[k]++
      if (idx[k] < ranges[k].length) break
      idx[k] = 0
      k--
    }
    if (k < 0) return out
  }
}

export function extractURLs(text: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (let m of text.match(/https?:\/\/[^\s<>"'`]+/g) ?? []) {
    m = m.replace(/[.,;:!?)}'"]+$/, '')
    while (m.endsWith(']') && (m.match(/]/g)?.length ?? 0) > (m.match(/\[/g)?.length ?? 0)) {
      m = m.slice(0, -1).replace(/[.,;:!?)}'"]+$/, '')
    }
    let list = [m]
    if (hasPattern(m)) {
      try {
        list = expandPattern(m)
      } catch {
        /* keep literal */
      }
    }
    for (const u of list) {
      if (!seen.has(u) && out.length < MAX_ITEMS) {
        seen.add(u)
        out.push(u)
      }
    }
  }
  return out
}

export function splitName(filename: string): [string, string] {
  const lower = filename.toLowerCase()
  for (const d of ['.tar.gz', '.tar.xz', '.tar.bz2', '.tar.zst']) {
    if (lower.endsWith(d)) return [filename.slice(0, -d.length), d.slice(1)]
  }
  const i = filename.lastIndexOf('.')
  if (i <= 0 || filename.length - i > 11) return [filename, '']
  return [filename.slice(0, i), filename.slice(i + 1)]
}

export interface TemplateVars {
  filename: string
  index: number
  batch: string
  host: string
  date: Date
}

export const TEMPLATE_TOKENS = ['{name}', '{ext}', '{index}', '{index:03}', '{batch}', '{date}', '{host}']

export function renderTemplate(tpl: string, v: TemplateVars): string {
  if (!tpl.trim()) return v.filename
  const [name, ext] = splitName(v.filename)
  const pad = (n: number) => String(n).padStart(2, '0')
  const date = `${v.date.getFullYear()}-${pad(v.date.getMonth() + 1)}-${pad(v.date.getDate())}`
  let out = tpl.replace(/\{(name|ext|index|batch|date|host)(?::(0\d+))?\}/g, (_, key: string, width?: string) => {
    switch (key) {
      case 'name':
        return name
      case 'ext':
        return ext
      case 'index':
        return width ? String(v.index).padStart(Number(width), '0') : String(v.index)
      case 'batch':
        return v.batch
      case 'date':
        return date
      case 'host':
        return v.host
    }
    return ''
  })
  if (out.endsWith('.')) out = out.slice(0, -1)
  return out.replaceAll('/', '_')
}

export function filenameFromURL(u: string): string {
  try {
    const url = new URL(u)
    const base = decodeURIComponent(url.pathname.split('/').filter(Boolean).pop() ?? '')
    return base || `${url.hostname}-download`
  } catch {
    return 'download'
  }
}
