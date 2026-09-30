<script lang="ts">
  import Icon from './Icon.svelte'
  import FileTile from './FileTile.svelte'
  import SegmentBar from './SegmentBar.svelte'
  import { store, isRunning } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { bytes, eta, host, percent, relTime, speed } from '../lib/format'
  import type { Job } from '../lib/types'

  let { job, dense = false }: { job: Job; dense?: boolean } = $props()

  const sp = $derived(store.speed[job.id] ?? 0)
  const running = $derived(isRunning(job.status))
  const pct = $derived(percent(job.downloaded, job.size))
  const selected = $derived(store.selected.has(job.id))
  const showBar = $derived(job.status !== 'completed' && !(job.status === 'queued' && job.downloaded === 0))
</script>

<div
  class="group relative flex cursor-default items-center gap-3 rounded-xl border px-2.5 transition-colors duration-100 sm:gap-3.5 sm:px-3.5
    {dense ? 'py-2' : 'py-3'}
    {selected
    ? 'border-accent/45 bg-accent-soft'
    : store.detailsId === job.id
      ? 'border-line-strong bg-surface-2'
      : 'border-transparent hover:bg-surface-2'}"
  onclick={(e) => {
    store.select(job.id, e)
    if (!e.ctrlKey && !e.shiftKey && !e.metaKey) store.detailsId = job.id
  }}
  ondblclick={() => job.status === 'completed' && api.openFile(job.id)}
  onkeydown={(e) => e.key === 'Enter' && (store.detailsId = job.id)}
  role="row"
  tabindex="0"
  aria-selected={selected}
>
  <FileTile category={job.category} size={dense ? 30 : 38} />

  <div class="min-w-0 flex-1">
    <div class="flex items-center gap-2">
      <span class="truncate font-medium text-fg {dense ? 'text-[13px]' : 'text-[13.5px]'}" title={job.filename}>{job.filename}</span>
      {#if job.status === 'failed'}
        <span class="shrink-0 rounded bg-bad-soft px-1.5 text-[10.5px] font-semibold text-bad">FAILED</span>
      {:else if job.status === 'paused'}
        <span class="shrink-0 rounded bg-warn-soft px-1.5 text-[10.5px] font-semibold text-warn">PAUSED</span>
      {:else if job.status === 'probing'}
        <span class="shrink-0 rounded bg-accent-soft px-1.5 text-[10.5px] font-semibold text-accent-hi">CONNECTING</span>
      {/if}
    </div>

    <div class="mt-0.5 flex items-center gap-1.5 truncate text-[12px] text-fg-3">
      {#if job.status === 'downloading'}
        <span class="num text-fg-2">{bytes(job.downloaded)}</span>
        <span>/</span>
        <span class="num">{bytes(job.size)}</span>
        <span class="text-line-strong">•</span>
        <span class="num text-accent-2">{speed(sp)}</span>
        <span class="text-line-strong">•</span>
        <span class="num">{eta(job.size - job.downloaded, sp)} left</span>
        {#if store.conns[job.id]}
          <span class="text-line-strong">•</span>
          <span class="num">{store.conns[job.id]} conn</span>
        {/if}
      {:else if job.status === 'failed'}
        <span class="truncate text-bad/90">{job.error || 'Download failed'}</span>
      {:else if job.status === 'completed'}
        <span class="num">{bytes(job.size)}</span>
        <span class="text-line-strong">•</span>
        <span class="truncate">{host(job.finalUrl || job.url)}</span>
        <span class="text-line-strong">•</span>
        <span>{relTime(job.completedAt ?? job.createdAt)}</span>
      {:else if job.status === 'paused'}
        <span class="num">{bytes(job.downloaded)} of {bytes(job.size)}</span>
        {#if !job.resumable && job.probed}<span class="text-warn">• restarts from zero</span>{/if}
      {:else if job.status === 'probing'}
        <span class="truncate">Connecting to {host(job.url)}…</span>
      {:else}
        <span>Waiting</span>
        {#if job.size > 0}<span class="text-line-strong">•</span><span class="num">{bytes(job.size)}</span>{/if}
        <span class="text-line-strong">•</span>
        <span class="truncate">{host(job.url)}</span>
      {/if}
    </div>

    {#if showBar}
      <div class="mt-2 max-w-[640px]">
        <SegmentBar segments={job.segments} size={job.size} downloaded={job.downloaded} status={job.status} />
      </div>
    {/if}
  </div>

  <div class="flex shrink-0 items-center gap-1">
    {#if job.status !== 'completed' && job.size > 0 && (running || job.downloaded > 0)}
      <span class="num mr-1 w-12 text-right text-[13px] font-semibold {running ? 'text-fg' : 'text-fg-3'} group-hover:hidden">
        {Math.floor(pct)}%
      </span>
    {:else if job.status === 'completed'}
      <span class="mr-1 flex size-7 items-center justify-center text-ok group-hover:hidden"><Icon name="check" size={16} stroke={2.4} /></span>
    {/if}

    <div class="hidden items-center gap-0.5 group-hover:flex group-focus-within:flex pointer-coarse:flex">
      {#if job.status === 'completed' && api.fileURL(job.id)}
        <a class="icon-btn size-7" title="Save to this computer" href={api.fileURL(job.id)} download onclick={(e) => e.stopPropagation()}>
          <Icon name="download" size={14} />
        </a>
      {:else if job.status === 'completed'}
        <button class="icon-btn size-7" title="Open" onclick={(e) => (e.stopPropagation(), api.openFile(job.id))}>
          <Icon name="external" size={14} />
        </button>
        <button class="icon-btn size-7 pointer-coarse:hidden" title="Show in folder" onclick={(e) => (e.stopPropagation(), api.showInFolder(job.id))}>
          <Icon name="folder" size={14} />
        </button>
      {:else if job.status === 'failed'}
        <button class="icon-btn size-7" title="Retry" onclick={(e) => (e.stopPropagation(), store.resume([job.id]))}>
          <Icon name="retry" size={14} />
        </button>
      {:else}
        <button
          class="icon-btn size-7"
          title={running || job.status === 'queued' ? 'Pause' : 'Resume'}
          onclick={(e) => (e.stopPropagation(), store.toggle(job))}
        >
          <Icon name={running || job.status === 'queued' ? 'pause' : 'play'} size={14} />
        </button>
      {/if}
      <button
        class="icon-btn size-7 hover:text-bad pointer-coarse:hidden"
        title="Remove"
        onclick={(e) => (e.stopPropagation(), store.askRemove(store.targetIds(job.id)))}
      >
        <Icon name="trash" size={14} />
      </button>
    </div>
  </div>
</div>
