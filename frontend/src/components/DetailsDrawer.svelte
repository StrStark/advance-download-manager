<script lang="ts">
  import { fly } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import Icon from './Icon.svelte'
  import FileTile from './FileTile.svelte'
  import SegmentBar from './SegmentBar.svelte'
  import StatusPill from './StatusPill.svelte'
  import { store, isRunning } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { bytes, eta, percent, speed } from '../lib/format'

  const job = $derived(store.detailsId ? store.jobs[store.detailsId] : null)
  const sp = $derived(job ? (store.speed[job.id] ?? 0) : 0)
  const segs = $derived(job?.segments ? [...job.segments].sort((a, b) => a.start - b.start) : [])
  const batch = $derived(job?.batchId ? store.batches[job.batchId] : null)
  let copied = $state('')

  function copy(text: string, key: string) {
    navigator.clipboard?.writeText(text)
    copied = key
    setTimeout(() => (copied = ''), 1200)
  }
  function curl() {
    if (!job) return ''
    const h = Object.entries(job.headers ?? {})
      .map(([k, v]) => ` -H '${k}: ${v.replaceAll("'", "'\\''")}'`)
      .join('')
    return `curl -L -C -${h} -o '${job.filename.replaceAll("'", "'\\''")}' '${job.url}'`
  }
</script>

{#if job}
  <aside
    class="fixed inset-y-0 right-0 z-30 flex w-full shrink-0 flex-col border-l border-line bg-panel shadow-card sm:max-w-[400px] lg:static lg:w-[360px] lg:shadow-none"
    style="padding-top: var(--inset-top); padding-bottom: var(--inset-bottom)"
    transition:fly={{ x: 40, duration: 200, easing: cubicOut }}
    aria-label="Download details"
  >
    <header class="flex items-start gap-3 border-b border-line p-4">
      <FileTile category={job.category} size={42} />
      <div class="min-w-0 flex-1">
        <h2 class="text-[14px] leading-snug font-semibold break-words text-fg">{job.filename}</h2>
        <div class="mt-1.5 flex items-center gap-2">
          <StatusPill status={job.status} />
          {#if batch}
            <button class="truncate text-[12px] text-accent-2 hover:underline" onclick={() => (store.filter = { kind: 'batch', id: batch.id })}>
              {batch.name}
            </button>
          {/if}
        </div>
      </div>
      <button class="icon-btn -mt-1 -mr-1.5" onclick={() => (store.detailsId = null)} aria-label="Close details">
        <Icon name="x" />
      </button>
    </header>

    <div class="flex-1 space-y-5 overflow-y-auto p-4">
      <!-- progress -->
      <div>
        <div class="mb-2 flex items-baseline justify-between">
          <span class="num text-[26px] font-semibold tracking-[-0.03em]">
            {job.size > 0 ? Math.floor(percent(job.downloaded, job.size)) : '—'}<span class="text-[15px] text-fg-3">%</span>
          </span>
          <span class="num text-[12px] text-fg-3">{bytes(job.downloaded)} / {bytes(job.size)}</span>
        </div>
        <SegmentBar segments={job.segments} size={job.size} downloaded={job.downloaded} status={job.status} tall />
        {#if job.error}
          <div class="mt-3 flex gap-2 rounded-lg bg-bad-soft px-3 py-2 text-[12px] text-bad">
            <Icon name="alert" size={14} class="mt-px" />
            <span class="break-words">{job.error}</span>
          </div>
        {/if}
      </div>

      <div class="grid grid-cols-3 gap-2">
        {#each [['Speed', isRunning(job.status) ? speed(sp) : '—'], ['Time left', isRunning(job.status) ? eta(job.size - job.downloaded, sp) : '—'], ['Connections', isRunning(job.status) ? `${store.conns[job.id] ?? 0} / ${job.connections}` : String(job.connections)]] as [k, v] (k)}
          <div class="rounded-xl border border-line bg-surface px-3 py-2.5">
            <div class="text-[11px] text-fg-3">{k}</div>
            <div class="num mt-0.5 truncate text-[13px] font-semibold">{v}</div>
          </div>
        {/each}
      </div>

      <!-- segments -->
      {#if segs.length > 1}
        <div>
          <div class="mb-2 flex items-center justify-between">
            <h3 class="text-[12px] font-semibold text-fg-2">Segments</h3>
            <span class="num text-[11px] text-fg-3">{segs.length}</span>
          </div>
          <div class="max-h-[180px] space-y-1 overflow-y-auto pr-1">
            {#each segs as s, i (s.start)}
              {@const len = s.end - s.start + 1}
              {@const p = Math.min(100, (s.done / len) * 100)}
              <div class="grid grid-cols-[22px_1fr_44px] items-center gap-2 text-[11px]">
                <span class="num text-fg-3">{i + 1}</span>
                <div class="h-1 overflow-hidden rounded-full bg-track">
                  <div
                    class="h-full rounded-full transition-[width] duration-300"
                    style="width:{p}%; background:{p >= 100 ? 'var(--ok)' : i % 2 ? 'var(--seg-b)' : 'var(--seg-a)'}"
                  ></div>
                </div>
                <span class="num text-right text-fg-3">{bytes(len, 0)}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <!-- info -->
      <dl class="space-y-2.5 text-[12px]">
        {#snippet row(label: string, value: string, key: string, mono = false)}
          <div class="group">
            <dt class="text-fg-3">{label}</dt>
            <dd class="mt-0.5 flex items-start gap-1.5">
              <span class="min-w-0 flex-1 break-all text-fg-2 select-text {mono ? 'num' : ''}">{value}</span>
              <button
                class="icon-btn size-6 opacity-0 group-hover:opacity-100 focus:opacity-100"
                title="Copy"
                onclick={() => copy(value, key)}
              >
                <Icon name={copied === key ? 'check' : 'copy'} size={12} />
              </button>
            </dd>
          </div>
        {/snippet}
        {@render row('Saved to', `${job.dir}/${job.filename}`, 'path')}
        {@render row('Source', job.url, 'url')}
        {#if job.finalUrl && job.finalUrl !== job.url}{@render row('Redirected to', job.finalUrl, 'final')}{/if}
        <div class="grid grid-cols-2 gap-2.5">
          <div>
            <dt class="text-fg-3">Resumable</dt>
            <dd class="mt-0.5 {job.resumable ? 'text-ok' : 'text-warn'}">{job.probed ? (job.resumable ? 'Yes' : 'No') : '—'}</dd>
          </div>
          <div>
            <dt class="text-fg-3">Type</dt>
            <dd class="mt-0.5 truncate text-fg-2">{job.contentType?.split(';')[0] || '—'}</dd>
          </div>
          <div>
            <dt class="text-fg-3">Added</dt>
            <dd class="mt-0.5 text-fg-2">{new Date(job.createdAt).toLocaleString()}</dd>
          </div>
          {#if job.completedAt}
            <div>
              <dt class="text-fg-3">Completed</dt>
              <dd class="mt-0.5 text-fg-2">{new Date(job.completedAt).toLocaleString()}</dd>
            </div>
          {/if}
        </div>
        {#if job.etag}{@render row('ETag', job.etag, 'etag', true)}{/if}
      </dl>
    </div>

    <footer class="grid grid-cols-2 gap-2 border-t border-line p-3">
      {#if job.status === 'completed' && api.fileURL(job.id)}
        <a class="btn btn-primary col-span-2" href={api.fileURL(job.id)} download><Icon name="download" size={14} />Save to this computer</a>
      {:else if job.status === 'completed'}
        <button class="btn btn-primary" onclick={() => api.openFile(job.id)}><Icon name="external" size={14} />Open</button>
        <button class="btn btn-outline" onclick={() => api.showInFolder(job.id)}><Icon name="folder" size={14} />Show in folder</button>
      {:else if job.status === 'failed'}
        <button class="btn btn-primary" onclick={() => store.resume([job.id])}><Icon name="retry" size={14} />Retry</button>
        <button class="btn btn-outline" onclick={() => copy(curl(), 'curl')}>
          <Icon name={copied === 'curl' ? 'check' : 'copy'} size={14} />Copy as curl
        </button>
      {:else}
        <button class="btn btn-primary" onclick={() => store.toggle(job)}>
          <Icon name={isRunning(job.status) || job.status === 'queued' ? 'pause' : 'play'} size={14} />
          {isRunning(job.status) || job.status === 'queued' ? 'Pause' : 'Resume'}
        </button>
        <button class="btn btn-outline" onclick={() => copy(curl(), 'curl')}>
          <Icon name={copied === 'curl' ? 'check' : 'copy'} size={14} />Copy as curl
        </button>
      {/if}
      <button class="btn btn-ghost col-span-2 text-bad hover:bg-bad-soft hover:text-bad" onclick={() => store.askRemove([job.id])}>
        <Icon name="trash" size={14} />Remove
      </button>
    </footer>
  </aside>
{/if}
