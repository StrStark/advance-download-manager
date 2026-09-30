<script lang="ts">
  import { untrack } from 'svelte'
  import { slide } from 'svelte/transition'
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'
  import FileTile from './FileTile.svelte'
  import Toggle from './Toggle.svelte'
  import ProxySelect from './ProxySelect.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { bytes, host } from '../lib/format'
  import { filenameFromURL } from '../lib/batchutil'
  import type { ProbeResult } from '../lib/types'

  let { initialUrl = '' }: { initialUrl?: string } = $props()

  let url = $state(initialUrl)
  let filename = $state('')
  let nameTouched = $state(false)
  let dir = $state(store.settings.downloadDir)
  let connections = $state(store.settings.defaultConnections)
  let paused = $state(false)
  let advanced = $state(false)
  let referer = $state('')
  let cookie = $state('')
  let userAgent = $state('')
  let extra = $state('')
  let proxy = $state('')

  let probe = $state<ProbeResult | null>(null)
  let probing = $state(false)
  let submitting = $state(false)
  let seq = 0

  const valid = $derived(/^https?:\/\/[^\s/$.?#].[^\s]*$/i.test(url.trim()))

  function headers(): Record<string, string> {
    const h: Record<string, string> = {}
    if (referer.trim()) h['Referer'] = referer.trim()
    if (cookie.trim()) h['Cookie'] = cookie.trim()
    if (userAgent.trim()) h['User-Agent'] = userAgent.trim()
    for (const line of extra.split('\n')) {
      const i = line.indexOf(':')
      if (i > 0) h[line.slice(0, i).trim()] = line.slice(i + 1).trim()
    }
    return h
  }

  // Debounced probe whenever the URL changes.
  $effect(() => {
    const u = url.trim()
    const via = proxy
    probe = null
    if (!valid) return
    untrack(() => {
      if (!nameTouched) filename = filenameFromURL(u)
    })
    const my = ++seq
    probing = true
    const t = setTimeout(async () => {
      try {
        const r = await api.probe(u, headers(), via)
        if (my !== seq) return
        probe = r
        if (!nameTouched && r.filename) filename = r.filename
      } catch (e) {
        if (my === seq) probe = { url: u, finalUrl: u, filename, size: -1, resumable: false, category: 'other', error: String(e) }
      } finally {
        if (my === seq) probing = false
      }
    }, 350)
    return () => clearTimeout(t)
  })

  async function browse() {
    const d = await store.run(api.chooseDirectory(dir))
    if (d) dir = d
  }

  async function submit() {
    if (!valid || submitting) return
    submitting = true
    const j = await store.run(
      api.addDownload({
        url: url.trim(),
        filename: filename.trim(),
        dir,
        connections: probe && !probe.resumable ? 1 : connections,
        headers: headers(),
        paused,
        proxy,
        size: probe?.size && probe.size > 0 ? probe.size : 0,
      }),
      'Could not add download',
    )
    submitting = false
    if (j) {
      store.addOpen = null
      store.filter = { kind: 'all' }
    }
  }
</script>

<Modal title="New download" subtitle="Paste a link. We'll check it before starting." onclose={() => (store.addOpen = null)}>
  <form
    class="space-y-4"
    onsubmit={(e) => {
      e.preventDefault()
      submit()
    }}
  >
    <div>
      <label class="label" for="add-url">URL</label>
      <div class="relative">
        <Icon name="link" size={15} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-fg-3" />
        <input id="add-url" class="field pl-9 font-mono text-[12.5px]" bind:value={url} placeholder="https://example.com/file.zip" autofocus spellcheck="false" />
      </div>
    </div>

    {#if valid}
      <div class="rounded-xl border border-line bg-surface-2 p-3" transition:slide={{ duration: 160 }}>
        <div class="flex items-center gap-3">
          <FileTile category={probe?.category ?? 'other'} size={40} />
          <div class="min-w-0 flex-1">
            <input
              class="w-full truncate rounded-md border border-transparent bg-transparent px-1.5 py-0.5 -ml-1.5 text-[13.5px] font-medium text-fg outline-none hover:border-line focus:border-accent"
              bind:value={filename}
              oninput={() => (nameTouched = true)}
              aria-label="File name"
              spellcheck="false"
            />
            <div class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[12px] text-fg-3">
              {#if probing}
                <span class="flex items-center gap-1.5"><Icon name="loader" size={12} class="animate-spin" />Checking {host(url)}…</span>
              {:else if probe?.error}
                <span class="text-bad">{probe.error}</span>
              {:else if probe}
                <span class="num text-fg-2">{probe.size > 0 ? bytes(probe.size) : 'Unknown size'}</span>
                <span class="text-line-strong">•</span>
                {#if probe.resumable}
                  <span class="flex items-center gap-1 text-ok"><Icon name="check" size={12} stroke={2.6} />Resumable</span>
                {:else}
                  <span class="text-warn">No resume · single connection</span>
                {/if}
                <span class="text-line-strong">•</span>
                <span class="truncate">{host(probe.finalUrl || url)}</span>
              {/if}
            </div>
          </div>
        </div>
      </div>
    {/if}

    <div>
      <label class="label" for="add-dir">Save to</label>
      <div class="flex gap-2">
        <input id="add-dir" class="field font-mono text-[12.5px]" bind:value={dir} spellcheck="false" />
        {#if api.kind === 'desktop'}<button type="button" class="btn btn-outline h-9 shrink-0" onclick={browse}><Icon name="folder" size={14} />Browse</button>{/if}
      </div>
    </div>

    <div>
      <div class="flex items-center justify-between">
        <label class="label mb-0" for="add-conns">Connections</label>
        <span class="num text-[12px] text-fg-2">{probe && !probe.resumable ? 1 : connections}</span>
      </div>
      <input
        id="add-conns"
        type="range"
        min="1"
        max="32"
        bind:value={connections}
        disabled={!!probe && !probe.resumable}
        class="mt-2 w-full accent-[var(--accent)]"
      />
    </div>

    <div>
      <label class="label" for="add-proxy">Connect through</label>
      <ProxySelect id="add-proxy" bind:value={proxy} />
    </div>

    <Toggle bind:checked={paused} label="Add paused" hint="Queue it without starting" />

    <div class="border-t border-line pt-3">
      <button type="button" class="flex items-center gap-1.5 text-[12.5px] font-medium text-fg-2 hover:text-fg" onclick={() => (advanced = !advanced)}>
        <Icon name="chevronRight" size={14} class="transition-transform duration-150 {advanced ? 'rotate-90' : ''}" />
        Advanced: headers, cookies, user agent
      </button>
      {#if advanced}
        <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2" transition:slide={{ duration: 160 }}>
          <div>
            <label class="label" for="add-ref">Referer</label>
            <input id="add-ref" class="field" bind:value={referer} placeholder="https://…" spellcheck="false" />
          </div>
          <div>
            <label class="label" for="add-ua">User agent</label>
            <input id="add-ua" class="field" bind:value={userAgent} placeholder="Default (browser-like)" spellcheck="false" />
          </div>
          <div class="sm:col-span-2">
            <label class="label" for="add-cookie">Cookie</label>
            <input id="add-cookie" class="field font-mono text-[12px]" bind:value={cookie} placeholder="session=…; token=…" spellcheck="false" />
          </div>
          <div class="sm:col-span-2">
            <label class="label" for="add-headers">Extra headers <span class="font-normal text-fg-3">(one per line)</span></label>
            <textarea
              id="add-headers"
              class="field h-20 resize-none py-2 font-mono text-[12px]"
              bind:value={extra}
              placeholder="Authorization: Bearer …"
              spellcheck="false"
            ></textarea>
          </div>
        </div>
      {/if}
    </div>
    <button type="submit" class="hidden" aria-hidden="true" tabindex="-1"></button>
  </form>

  {#snippet footer()}
    <button class="btn btn-ghost" onclick={() => (store.addOpen = null)}>Cancel</button>
    <button class="btn btn-primary min-w-[120px]" disabled={!valid || submitting || !!probe?.error} onclick={submit}>
      <Icon name="download" size={15} />{paused ? 'Add paused' : 'Download'}
    </button>
  {/snippet}
</Modal>
