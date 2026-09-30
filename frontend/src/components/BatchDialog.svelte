<script lang="ts">
  import { onDestroy } from 'svelte'
  import { SvelteSet } from 'svelte/reactivity'
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'
  import FileTile from './FileTile.svelte'
  import Toggle from './Toggle.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { bytes, ext, host } from '../lib/format'
  import { TEMPLATE_TOKENS, expandPattern, extractURLs, filenameFromURL, hasPattern, renderTemplate } from '../lib/batchutil'
  import type { ImportItem, ProbeResult } from '../lib/types'
  import type { IconName } from '../lib/icons'

  let { initialText = '', initialTab = 'paste' }: { initialText?: string; initialTab?: 'paste' | 'pattern' | 'import' } = $props()

  type Item = {
    url: string
    filename: string
    customName?: string
    dir?: string
    size: number
    category: string
    status: 'pending' | 'ok' | 'error'
    error?: string
    resumable?: boolean
  }

  let step = $state<'source' | 'review'>('source')
  let tab = $state(initialTab)
  let pasteText = $state(initialText)
  let pattern = $state('')
  let imported = $state<ImportItem[]>([])
  let importName = $state('')

  let items = $state<Item[]>([])
  let unchecked = new SvelteSet<number>()
  let exts = new SvelteSet<string>()
  let query = $state('')
  let name = $state('')
  let dir = $state(store.settings.downloadDir)
  let subfolder = $state(true)
  let template = $state('')
  let maxParallel = $state(store.settings.maxActive)
  let connections = $state(store.settings.defaultConnections)
  let sequential = $state(false)
  let paused = $state(false)
  let submitting = $state(false)
  let session = ''
  let unsub: (() => void) | null = null

  // ---------- step 1 ----------
  const pasted = $derived(tab === 'paste' ? extractURLs(pasteText) : [])
  const patternResult = $derived.by((): { urls: string[]; error?: string } => {
    if (tab !== 'pattern' || !pattern.trim()) return { urls: [] }
    if (!/^https?:\/\//i.test(pattern.trim())) return { urls: [], error: 'Must start with http:// or https://' }
    if (!hasPattern(pattern)) return { urls: [], error: 'Add a range like [1-50], [001-120], [a-z] or [0-100:5]' }
    try {
      return { urls: expandPattern(pattern.trim()) }
    } catch (e) {
      return { urls: [], error: (e as Error).message }
    }
  })
  const sourceItems = $derived.by((): ImportItem[] => {
    if (tab === 'paste') return pasted.map((url) => ({ url }))
    if (tab === 'pattern') return patternResult.urls.map((url) => ({ url }))
    return imported
  })

  async function pickFile() {
    const list = await store.run(api.importFile(), 'Could not import file')
    if (list) {
      imported = list.filter((i) => /^https?:\/\//i.test(i.url))
      importName = imported.length ? `${imported.length} links imported` : 'No links found in that file'
    }
  }

  function defaultName(list: ImportItem[]): string {
    if (tab === 'pattern') {
      try {
        // The last folder that isn't itself a range, e.g. /lectures/week[01-12]/x → "lectures".
        const segs = new URL(pattern.trim()).pathname.split('/').filter(Boolean).slice(0, -1)
        const stable = segs.filter((sg) => !sg.includes('[')).pop()
        if (stable) return decodeURIComponent(stable)
      } catch {
        /* fall through */
      }
    }
    const hosts = new Set(list.map((i) => host(i.url)))
    const d = new Date().toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
    return hosts.size === 1 ? `${[...hosts][0]} · ${d}` : `Batch · ${d}`
  }

  function toReview() {
    const list = sourceItems
    if (!list.length) return
    items = list.map((i) => {
      const fn = filenameFromURL(i.url)
      return { url: i.url, filename: fn, customName: i.filename || undefined, dir: i.dir || undefined, size: -1, category: guessCat(fn), status: 'pending' }
    })
    unchecked.clear()
    exts.clear()
    name = defaultName(list)
    step = 'review'
    session = Math.random().toString(36).slice(2)
    unsub?.()
    const mine = session
    unsub = api.on('probe:result', (d: { session: string; index: number; result: ProbeResult }) => {
      if (d.session !== mine) return
      const it = items[d.index]
      if (!it) return
      const r = d.result
      if (r.error) {
        it.status = 'error'
        it.error = r.error
      } else {
        it.status = 'ok'
        it.size = r.size
        it.filename = r.filename || it.filename
        it.category = r.category
        it.resumable = r.resumable
      }
    })
    api.probeMany(mine, items.map((i) => i.url))
  }
  onDestroy(() => unsub?.())

  const CATS: Record<string, string[]> = {
    video: ['mp4', 'mkv', 'webm', 'mov', 'avi', 'm4v'],
    music: ['mp3', 'flac', 'ogg', 'opus', 'wav', 'm4a'],
    archives: ['zip', 'rar', '7z', 'tar', 'gz', 'xz', 'zst', 'iso'],
    documents: ['pdf', 'docx', 'odt', 'xlsx', 'epub', 'txt', 'csv'],
    images: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'avif'],
    programs: ['deb', 'rpm', 'appimage', 'sh', 'run', 'exe'],
  }
  function guessCat(fn: string) {
    const e = ext(fn)
    for (const [c, list] of Object.entries(CATS)) if (list.includes(e)) return c
    return 'other'
  }

  // ---------- step 2 ----------
  const extCounts = $derived.by(() => {
    const m = new Map<string, number>()
    for (const it of items) {
      const e = ext(it.filename) || '—'
      m.set(e, (m.get(e) ?? 0) + 1)
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).slice(0, 12)
  })

  const matcher = $derived.by((): ((s: string) => boolean) | { error: string } => {
    const q = query.trim()
    if (!q) return () => true
    const re = /^\/(.+)\/([a-z]*)$/.exec(q)
    if (re) {
      try {
        const rx = new RegExp(re[1], re[2].includes('i') ? re[2] : re[2] + 'i')
        return (s: string) => rx.test(s)
      } catch {
        return { error: 'Invalid regular expression' }
      }
    }
    const low = q.toLowerCase()
    return (s: string) => s.toLowerCase().includes(low)
  })

  const visible = $derived.by(() => {
    const m = typeof matcher === 'function' ? matcher : () => true
    const out: number[] = []
    items.forEach((it, i) => {
      if (exts.size && !exts.has(ext(it.filename) || '—')) return
      if (!m(it.url) && !m(it.filename)) return
      out.push(i)
    })
    return out
  })
  const chosen = $derived(visible.filter((i) => !unchecked.has(i) && items[i].status !== 'error'))
  const posOf = $derived(new Map(chosen.map((i, p) => [i, p])))
  const allChecked = $derived(visible.length > 0 && visible.every((i) => !unchecked.has(i)))
  const probedCount = $derived(items.filter((i) => i.status !== 'pending').length)
  const failedCount = $derived(items.filter((i) => i.status === 'error').length)
  const totalSize = $derived(chosen.reduce((a, i) => a + Math.max(0, items[i].size), 0))
  const unknownSize = $derived(chosen.some((i) => items[i].size <= 0))

  function finalName(i: number, pos: number): string {
    const it = items[i]
    if (it.customName) return it.customName
    return renderTemplate(template, { filename: it.filename, index: pos + 1, batch: name.trim() || 'batch', host: host(it.url), date: new Date() })
  }
  const preview = $derived(chosen.length ? finalName(chosen[0], 0) : '')
  const dupes = $derived.by(() => {
    const seen = new Set<string>()
    let n = 0
    for (const i of chosen) {
      const k = finalName(i, posOf.get(i) ?? 0).toLowerCase()
      if (seen.has(k)) n++
      else seen.add(k)
    }
    return n
  })

  function toggleAll() {
    if (allChecked) visible.forEach((i) => unchecked.add(i))
    else visible.forEach((i) => unchecked.delete(i))
  }

  function destination(): string {
    const base = dir.replace(/\/+$/, '')
    if (!subfolder || !name.trim()) return base
    return `${base}/${name.trim().replaceAll('/', '_')}`
  }

  async function browse() {
    const d = await store.run(api.chooseDirectory(dir))
    if (d) dir = d
  }

  async function submit() {
    if (!chosen.length || submitting) return
    submitting = true
    const used = new Map<string, number>()
    const reqItems = chosen.map((i, pos) => {
      let fn = finalName(i, pos)
      const n = used.get(fn.toLowerCase()) ?? 0
      used.set(fn.toLowerCase(), n + 1)
      if (n > 0) {
        const dot = fn.lastIndexOf('.')
        fn = dot > 0 ? `${fn.slice(0, dot)} (${n})${fn.slice(dot)}` : `${fn} (${n})`
      }
      const it = items[i]
      return {
        url: it.url,
        filename: fn,
        dir: it.dir,
        size: it.size > 0 ? it.size : 0,
        connections: it.resumable === false ? 1 : 0,
      }
    })
    const b = await store.run(
      api.createBatch({
        name: name.trim(),
        dir: destination(),
        maxParallel: Math.max(1, maxParallel),
        sequential,
        connections,
        paused,
        items: reqItems,
      }),
      'Could not create batch',
    )
    submitting = false
    if (b) {
      store.batchOpen = null
      store.filter = { kind: 'batch', id: b.id }
      store.toast('success', `Added ${reqItems.length} downloads`, b.name)
    }
  }

  const tabs: { id: typeof tab; label: string; icon: IconName }[] = [
    { id: 'paste', label: 'Paste links', icon: 'clipboard' },
    { id: 'pattern', label: 'URL pattern', icon: 'brackets' },
    { id: 'import', label: 'Import file', icon: 'upload' },
  ]
</script>

<Modal
  title={step === 'source' ? 'New batch' : 'Review batch'}
  subtitle={step === 'source'
    ? 'Add many files at once. You’ll review everything before it starts.'
    : `${items.length} links · ${probedCount < items.length ? `checking ${probedCount}/${items.length}…` : 'all checked'}${failedCount ? ` · ${failedCount} unreachable` : ''}`}
  width={step === 'source' ? 'max-w-xl' : 'max-w-3xl'}
  onclose={() => (store.batchOpen = null)}
>
  {#if step === 'source'}
    <div class="mb-4 grid grid-cols-3 gap-1 rounded-xl bg-surface-2 p-1" role="tablist">
      {#each tabs as t (t.id)}
        <button
          class="flex h-8 items-center justify-center gap-2 rounded-lg text-[12.5px] font-medium transition-all duration-150
            {tab === t.id ? 'bg-surface text-fg shadow-card' : 'text-fg-3 hover:text-fg-2'}"
          onclick={() => (tab = t.id)}
          role="tab"
          aria-selected={tab === t.id}
        >
          <Icon name={t.icon} size={14} class="hidden sm:block" />{t.label}
        </button>
      {/each}
    </div>

    {#if tab === 'paste'}
      <textarea
        class="field h-48 resize-none py-2.5 font-mono text-[12px] leading-relaxed"
        bind:value={pasteText}
        placeholder={'Paste links, or any text containing links.\n\nhttps://example.com/files/part1.zip\nhttps://example.com/files/part2.zip\nhttps://cdn.example.org/photos/img[001-040].jpg'}
        spellcheck="false"
        autofocus
      ></textarea>
      <p class="mt-2 text-[12px] text-fg-3">
        {#if pasted.length}
          <span class="font-medium text-ok">{pasted.length} unique link{pasted.length === 1 ? '' : 's'} found.</span> Duplicates and junk are skipped;
          ranges like <code class="font-mono text-fg-2">[1-9]</code> are expanded.
        {:else}
          Links are pulled out of any text. Duplicates are removed automatically.
        {/if}
      </p>
    {:else if tab === 'pattern'}
      <input class="field font-mono text-[12.5px]" bind:value={pattern} placeholder="https://example.com/gallery/img[001-120].jpg" spellcheck="false" autofocus />
      <div class="mt-2.5 flex flex-wrap gap-1.5">
        {#each [['[1-50]', 'numbers'], ['[001-250]', 'zero-padded'], ['[0-100:5]', 'with step'], ['[a-z]', 'letters']] as [ex, desc] (ex)}
          <button class="chip" onclick={() => (pattern = (pattern || 'https://example.com/file') + ex)} title="Insert {desc} range">
            <span class="font-mono text-fg">{ex}</span><span class="text-fg-3">{desc}</span>
          </button>
        {/each}
      </div>
      <div class="mt-4 rounded-xl border border-line bg-surface-2 p-3">
        {#if patternResult.error}
          <p class="text-[12.5px] text-fg-3">{patternResult.error}</p>
        {:else if patternResult.urls.length}
          {@const u = patternResult.urls}
          <div class="mb-2 text-[12px] font-medium text-fg-2">
            <span class="num text-accent-hi">{u.length.toLocaleString()}</span> URLs
          </div>
          <div class="space-y-1 font-mono text-[11.5px] text-fg-3">
            {#each u.length > 5 ? [...u.slice(0, 3), '…', u[u.length - 1]] : u as line, i (i)}
              <div class="truncate {line === '…' ? 'pl-2' : ''}">{line}</div>
            {/each}
          </div>
        {:else}
          <p class="text-[12.5px] text-fg-3">Type a URL with a range to preview the generated links.</p>
        {/if}
      </div>
    {:else}
      <button
        class="flex h-48 w-full flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-line-strong bg-surface-2 text-fg-2 transition-colors hover:border-accent hover:text-fg"
        onclick={pickFile}
      >
        <div class="flex size-11 items-center justify-center rounded-xl bg-accent-soft text-accent-hi"><Icon name="upload" size={20} /></div>
        <div>
          <div class="text-[13px] font-medium">{importName || 'Choose a file…'}</div>
          <div class="mt-0.5 text-[12px] text-fg-3">
            <span class="font-mono">.txt</span> one URL per line · <span class="font-mono">.csv</span> url, filename, folder ·
            <span class="font-mono">.json</span>
          </div>
        </div>
      </button>
    {/if}
  {:else}
    <!-- ================= REVIEW ================= -->
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div>
        <label class="label" for="b-name">Batch name</label>
        <input id="b-name" class="field" bind:value={name} spellcheck="false" />
      </div>
      <div>
        <label class="label" for="b-dir">Save to</label>
        <div class="flex gap-2">
          <input id="b-dir" class="field font-mono text-[12px]" bind:value={dir} spellcheck="false" />
          {#if api.kind === 'desktop'}<button class="btn btn-outline h-9 shrink-0 px-2.5" onclick={browse} title="Browse"><Icon name="folder" size={14} /></button>{/if}
        </div>
      </div>
    </div>
    <label class="mt-2 flex w-fit cursor-pointer items-center gap-2 text-[12px] text-fg-3">
      <input type="checkbox" bind:checked={subfolder} class="accent-[var(--accent)]" />
      Put files in a subfolder named after the batch
      {#if subfolder && name.trim()}<span class="truncate font-mono text-fg-2">→ {destination()}</span>{/if}
    </label>

    <!-- filters -->
    <div class="mt-4 flex flex-wrap items-center gap-2">
      <div class="relative min-w-[180px] flex-1">
        <Icon name="filter" size={13} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-fg-3" />
        <input
          class="field h-8 pl-8 text-[12.5px] {typeof matcher !== 'function' ? 'border-bad' : ''}"
          bind:value={query}
          placeholder="Filter by text, or /regex/"
          spellcheck="false"
        />
      </div>
      <div class="flex flex-wrap gap-1 sm:max-w-[55%] sm:justify-end">
        {#each extCounts as [e, n] (e)}
          <button
            class="chip h-7 {exts.has(e) ? 'chip-on' : ''}"
            onclick={() => (exts.has(e) ? exts.delete(e) : exts.add(e))}
            aria-pressed={exts.has(e)}
          >
            <span class="font-mono">.{e}</span><span class="num text-[11px] opacity-70">{n}</span>
          </button>
        {/each}
      </div>
    </div>

    <!-- table -->
    <div class="mt-3 overflow-hidden rounded-xl border border-line">
      <div class="grid grid-cols-[28px_1fr_64px_56px] sm:grid-cols-[28px_1fr_90px_84px] items-center gap-2 border-b border-line bg-surface-2 px-3 py-2 text-[11px] font-semibold tracking-wide text-fg-3 uppercase">
        <input type="checkbox" checked={allChecked} onchange={toggleAll} class="accent-[var(--accent)]" aria-label="Select all" />
        <span>File <span class="font-normal normal-case">({chosen.length} of {items.length} selected)</span></span>
        <span class="text-right">Size</span>
        <span class="text-right">Status</span>
      </div>
      <div class="max-h-[260px] overflow-y-auto">
        {#each visible.slice(0, 400) as i, pos (i)}
          {@const it = items[i]}
          {@const on = !unchecked.has(i) && it.status !== 'error'}
          <label
            class="grid cursor-pointer grid-cols-[28px_1fr_64px_56px] sm:grid-cols-[28px_1fr_90px_84px] items-center gap-2 border-b border-line/60 px-3 py-1.5 last:border-b-0 hover:bg-surface-2
              {on ? '' : 'opacity-50'}"
          >
            <input
              type="checkbox"
              checked={on}
              disabled={it.status === 'error'}
              onchange={() => (unchecked.has(i) ? unchecked.delete(i) : unchecked.add(i))}
              class="accent-[var(--accent)]"
            />
            <div class="flex min-w-0 items-center gap-2.5">
              <FileTile category={it.category} size={24} />
              <div class="min-w-0">
                <div class="truncate text-[12.5px] text-fg">{on ? finalName(i, posOf.get(i) ?? 0) : it.filename}</div>
                <div class="truncate font-mono text-[10.5px] text-fg-3">{it.url}</div>
              </div>
            </div>
            <span class="num text-right text-[12px] text-fg-2">{it.size > 0 ? bytes(it.size) : '—'}</span>
            <span class="flex justify-end text-[11.5px]">
              {#if it.status === 'pending'}
                <Icon name="loader" size={13} class="animate-spin text-fg-3" />
              {:else if it.status === 'error'}
                <span class="truncate text-bad" title={it.error}>{it.error?.replace('server returned ', '') ?? 'Error'}</span>
              {:else if it.resumable === false}
                <span class="text-warn" title="Server does not support resuming">No resume</span>
              {:else}
                <Icon name="check" size={14} stroke={2.6} class="text-ok" />
              {/if}
            </span>
          </label>
        {:else}
          <div class="px-3 py-8 text-center text-[12.5px] text-fg-3">No links match these filters.</div>
        {/each}
        {#if visible.length > 400}
          <div class="px-3 py-2 text-center text-[11.5px] text-fg-3">+ {(visible.length - 400).toLocaleString()} more</div>
        {/if}
      </div>
    </div>

    <!-- rename + options -->
    <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-[1.3fr_1fr]">
      <div>
        <label class="label" for="b-tpl">Rename template <span class="font-normal text-fg-3">(optional)</span></label>
        <input id="b-tpl" class="field font-mono text-[12px]" bind:value={template} placeholder={'{name}.{ext}'} spellcheck="false" />
        <div class="mt-1.5 flex flex-wrap gap-1">
          {#each TEMPLATE_TOKENS as tok (tok)}
            <button class="chip h-5 px-1.5 font-mono text-[11px]" onclick={() => (template += tok)}>{tok}</button>
          {/each}
        </div>
        {#if dupes > 0}
          <p class="mt-1.5 flex items-center gap-1.5 text-[12px] text-warn">
            <Icon name="alert" size={13} />{dupes} name{dupes === 1 ? '' : 's'} repeat and would get (1), (2)… suffixes.
            <button class="font-medium underline underline-offset-2 hover:text-fg" onclick={() => (template = '{index:03}_{name}.{ext}')}>Number them</button>
          </p>
        {:else if template && preview}
          <p class="mt-1.5 truncate text-[12px] text-fg-3">e.g. <span class="font-mono text-fg-2">{preview}</span></p>
        {/if}
      </div>
      <div class="space-y-2.5">
        <div class="grid grid-cols-2 gap-2">
          <div>
            <label class="label" for="b-par">Files at once</label>
            <input id="b-par" type="number" min="1" max="16" class="field h-8 num" bind:value={maxParallel} disabled={sequential} />
          </div>
          <div>
            <label class="label" for="b-con">Connections each</label>
            <input id="b-con" type="number" min="1" max="32" class="field h-8 num" bind:value={connections} />
          </div>
        </div>
        <Toggle bind:checked={sequential} label="One at a time, in order" />
        <Toggle bind:checked={paused} label="Add paused" />
      </div>
    </div>
  {/if}

  {#snippet footer()}
    {#if step === 'source'}
      <span class="mr-auto text-[12px] text-fg-3">
        {#if sourceItems.length}<span class="num text-fg-2">{sourceItems.length.toLocaleString()}</span> links ready{/if}
      </span>
      <button class="btn btn-ghost" onclick={() => (store.batchOpen = null)}>Cancel</button>
      <button class="btn btn-primary" disabled={!sourceItems.length} onclick={toReview}>
        Review<Icon name="chevronRight" size={15} />
      </button>
    {:else}
      <button class="btn btn-ghost mr-auto" onclick={() => (step = 'source')}>Back</button>
      <span class="text-[12px] text-fg-3">
        {#if chosen.length}
          <span class="num text-fg-2">{bytes(totalSize)}</span>{unknownSize ? '+' : ''} total
        {/if}
      </span>
      <button class="btn btn-primary min-w-[150px]" disabled={!chosen.length || submitting} onclick={submit}>
        <Icon name="download" size={15} />Add {chosen.length} download{chosen.length === 1 ? '' : 's'}
      </button>
    {/if}
  {/snippet}
</Modal>
