<script lang="ts">
  import { slide } from 'svelte/transition'
  import Icon from './Icon.svelte'
  import JobRow from './JobRow.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { bytes, eta, speed } from '../lib/format'
  import type { Batch, Job } from '../lib/types'

  let { batch, jobs }: { batch: Batch; jobs: Job[] } = $props()

  const s = $derived(store.batchStats[batch.id])
  const open = $derived(!store.collapsed.has(batch.id))
  const pct = $derived(
    !s || !s.total ? 0 : s.knownSize && s.size > 0 ? Math.min(100, (s.downloaded / s.size) * 100) : (s.completed / s.total) * 100,
  )
  const done = $derived(s && s.completed === s.total)
  const allIds = $derived(store.batchJobIds(batch.id))
  const canPause = $derived(s && s.active + s.queued > 0)
  const canResume = $derived(s && s.paused > 0)

  function toggleOpen() {
    if (open) store.collapsed.add(batch.id)
    else store.collapsed.delete(batch.id)
  }
</script>

<section
  class="overflow-hidden rounded-2xl border bg-surface transition-colors duration-150
    {store.filter.kind === 'batch' ? 'border-line' : 'border-line hover:border-line-strong'}"
>
  <div class="group flex items-center gap-2.5 px-2.5 py-3 sm:gap-3.5 sm:px-3.5">
    <button
      class="icon-btn -ml-1 size-7 text-fg-3"
      onclick={toggleOpen}
      aria-expanded={open}
      aria-label={open ? 'Collapse batch' : 'Expand batch'}
    >
      <Icon name="chevronRight" size={15} class="transition-transform duration-200 {open ? 'rotate-90' : ''}" />
    </button>
    <div
      class="flex size-[38px] shrink-0 items-center justify-center rounded-[10px] {done
        ? 'bg-ok-soft text-ok'
        : 'bg-accent-2-soft text-accent-2'}"
    >
      <Icon name="layers" size={18} />
    </div>

    <button class="min-w-0 flex-1 text-left" onclick={() => (store.filter = { kind: 'batch', id: batch.id })}>
      <div class="flex items-center gap-2">
        <span class="truncate text-[13.5px] font-semibold text-fg">{batch.name}</span>
        <span class="shrink-0 rounded bg-accent-2-soft px-1.5 text-[10.5px] font-semibold tracking-wide text-accent-2">BATCH</span>
        {#if batch.sequential}
          <span class="shrink-0 rounded bg-surface-3 px-1.5 text-[10.5px] font-medium text-fg-2" title="Downloads one file at a time">
            SEQUENTIAL
          </span>
        {/if}
      </div>
      <div class="mt-0.5 flex items-center gap-1.5 truncate text-[12px] text-fg-3">
        <span><span class="num text-fg-2">{s?.completed ?? 0}</span> / <span class="num">{s?.total ?? 0}</span> done</span>
        {#if s?.failed}
          <span class="text-line-strong">•</span><span class="text-bad">{s.failed} failed</span>
        {/if}
        {#if s?.knownSize && s.size > 0}
          <span class="text-line-strong">•</span><span class="num">{bytes(s.downloaded)} of {bytes(s.size)}</span>
        {/if}
        {#if s && s.speed > 0}
          <span class="text-line-strong">•</span><span class="num text-accent-2">{speed(s.speed)}</span>
          {#if s.knownSize}<span class="text-line-strong">•</span><span class="num">{eta(s.size - s.downloaded, s.speed)} left</span>{/if}
        {/if}
      </div>
      <div class="mt-2 h-1.5 max-w-[640px] overflow-hidden rounded-full bg-track">
        <div
          class="h-full rounded-full transition-[width] duration-500 ease-out"
          style="width:{pct}%; background:{done ? 'var(--ok)' : 'var(--accent-2)'}"
        ></div>
      </div>
    </button>

    <div class="flex shrink-0 items-center gap-1">
      <span class="num mr-1 w-12 text-right text-[13px] font-semibold text-fg-2 group-hover:hidden pointer-coarse:hidden">{Math.floor(pct)}%</span>
      <div class="hidden items-center gap-0.5 group-hover:flex group-focus-within:flex pointer-coarse:flex">
        {#if s?.failed}
          <button class="btn btn-ghost h-7 px-2 text-[12px] text-bad hover:text-bad" title="Retry failed" onclick={() => store.run(api.retryFailed(batch.id))}>
            <Icon name="retry" size={13} /><span class="hidden sm:inline">Retry failed</span>
          </button>
        {/if}
        {#if canPause}
          <button class="icon-btn size-7" title="Pause batch" onclick={() => store.pause(allIds)}><Icon name="pause" size={14} /></button>
        {/if}
        {#if canResume}
          <button class="icon-btn size-7" title="Resume batch" onclick={() => store.resume(allIds)}><Icon name="play" size={14} /></button>
        {/if}
        <button class="icon-btn size-7 hover:text-bad pointer-coarse:hidden" title="Remove batch" onclick={() => store.askRemove(allIds)}>
          <Icon name="trash" size={14} />
        </button>
      </div>
    </div>
  </div>

  {#if open && jobs.length}
    <div class="space-y-px border-t border-line bg-bg/40 px-1.5 py-2 sm:px-2 sm:pl-9" transition:slide={{ duration: 180 }} role="rowgroup">
      {#each jobs as job (job.id)}
        <JobRow {job} dense />
      {/each}
    </div>
  {/if}
</section>
