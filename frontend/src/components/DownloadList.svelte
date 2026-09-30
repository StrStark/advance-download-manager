<script lang="ts">
  import { flip } from 'svelte/animate'
  import { fade } from 'svelte/transition'
  import Icon from './Icon.svelte'
  import JobRow from './JobRow.svelte'
  import BatchGroup from './BatchGroup.svelte'
  import { store } from '../lib/store.svelte'
  import { mod } from '../lib/platform'
</script>

<div
  class="min-h-0 flex-1 overflow-y-auto px-2 py-3 pb-24 sm:px-4 md:pb-3"
  role="grid"
  tabindex="-1"
  onclick={(e) => {
    if (e.target === e.currentTarget) {
      store.selected.clear()
      store.detailsId = null
    }
  }}
  onkeydown={() => {}}
>
  {#if !store.loaded}
    <div class="space-y-2 pt-1">
      {#each Array(5) as _, i (i)}
        <div class="h-[68px] animate-pulse rounded-xl bg-surface/70" style="animation-delay:{i * 80}ms"></div>
      {/each}
    </div>
  {:else if store.rows.length === 0}
    <div class="flex h-full flex-col items-center justify-center pb-16 text-center" in:fade={{ duration: 200 }}>
      {#if store.search}
        <div class="mb-4 flex size-12 items-center justify-center rounded-2xl bg-surface-2 text-fg-3">
          <Icon name="search" size={22} />
        </div>
        <p class="text-[14px] font-medium">No matches for “{store.search}”</p>
        <p class="mt-1 text-[12.5px] text-fg-3">Try a different filename or domain.</p>
      {:else if store.filter.kind !== 'all'}
        <div class="mb-4 flex size-12 items-center justify-center rounded-2xl bg-surface-2 text-fg-3">
          <Icon name="inbox" size={22} />
        </div>
        <p class="text-[14px] font-medium">Nothing here</p>
        <button class="btn btn-ghost mt-2 text-accent-hi" onclick={() => (store.filter = { kind: 'all' })}>Show all downloads</button>
      {:else}
        <div class="relative mb-6">
          <div class="absolute inset-0 -z-10 scale-150 rounded-full bg-accent/20 blur-3xl"></div>
          <div
            class="flex size-16 items-center justify-center rounded-[20px] bg-gradient-to-br from-[#6366f1] to-[#22d3ee] text-white shadow-[0_12px_32px_-10px_#6366f1]"
          >
            <Icon name="arrowDown" size={28} stroke={2.4} />
          </div>
        </div>
        <p class="text-[16px] font-semibold tracking-[-0.01em]">Drop a link to start</p>
        <p class="mt-1.5 max-w-[340px] text-[13px] leading-relaxed text-fg-3">
          Paste a URL anywhere with <span class="kbd">{mod}</span> <span class="kbd">V</span>, drag links onto this window, or add
          many at once as a batch.
        </p>
        <div class="mt-5 flex gap-2">
          <button class="btn btn-primary" onclick={() => (store.addOpen = {})}><Icon name="plus" size={15} stroke={2.4} />New download</button>
          <button class="btn btn-outline" onclick={() => (store.batchOpen = {})}><Icon name="layers" size={15} />New batch</button>
        </div>
      {/if}
    </div>
  {:else}
    <div class="mx-auto max-w-[1100px] space-y-1.5">
      {#each store.rows as row (row.type === 'job' ? row.job.id : 'b:' + row.batch.id)}
        <div animate:flip={{ duration: 220 }} class={row.type === 'batch' ? 'py-1' : ''}>
          {#if row.type === 'job'}
            <JobRow job={row.job} />
          {:else}
            <BatchGroup batch={row.batch} jobs={row.jobs} />
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
