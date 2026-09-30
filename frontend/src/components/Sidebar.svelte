<script lang="ts">
  import Icon from './Icon.svelte'
  import { store, type Filter, type StatusCounts } from '../lib/store.svelte'
  import { categoryIcon, type IconName } from '../lib/icons'
  import { shortcut } from '../lib/platform'

  const library: { label: string; filter: Filter; icon: IconName; key: keyof StatusCounts }[] = [
    { label: 'All downloads', filter: { kind: 'all' }, icon: 'inbox', key: 'all' },
    { label: 'Downloading', filter: { kind: 'status', status: 'active' }, icon: 'activity', key: 'active' },
    { label: 'Queued', filter: { kind: 'status', status: 'queued' }, icon: 'clock', key: 'queued' },
    { label: 'Paused', filter: { kind: 'status', status: 'paused' }, icon: 'pause', key: 'paused' },
    { label: 'Completed', filter: { kind: 'status', status: 'completed' }, icon: 'checkCircle', key: 'completed' },
    { label: 'Failed', filter: { kind: 'status', status: 'failed' }, icon: 'alert', key: 'failed' },
  ]
  const categories = ['video', 'music', 'archives', 'documents', 'images', 'programs', 'other']

  function same(a: Filter, b: Filter) {
    return JSON.stringify(a) === JSON.stringify(b)
  }
  function pct(bid: string) {
    const s = store.batchStats[bid]
    if (!s || !s.total) return 0
    return s.knownSize && s.size > 0 ? s.downloaded / s.size : s.completed / s.total
  }
</script>

<aside
  class="fixed inset-y-0 left-0 z-40 flex w-[min(17rem,85vw)] shrink-0 flex-col border-r border-line bg-panel transition-transform duration-200 ease-out md:static md:w-60 md:translate-x-0
    {store.navOpen ? 'translate-x-0 shadow-card' : '-translate-x-full'}"
  style="padding-top: var(--inset-top); padding-bottom: var(--inset-bottom)"
>
  <div class="flex items-center gap-2.5 px-4 pt-4 pb-5">
    <div
      class="flex size-8 items-center justify-center rounded-[10px] bg-gradient-to-br from-[#6366f1] to-[#22d3ee] text-white shadow-[0_6px_18px_-6px_#6366f1]"
    >
      <Icon name="arrowDown" size={17} stroke={2.6} />
    </div>
    <div class="leading-tight">
      <div class="text-[14px] font-bold tracking-[-0.02em]">ADM</div>
      <div class="text-[11px] text-fg-3">Download Manager</div>
    </div>
  </div>

  <div class="space-y-1.5 px-3">
    <button class="btn btn-primary h-9 w-full justify-between" onclick={() => ((store.addOpen = {}), (store.navOpen = false))}>
      <span class="flex items-center gap-2"><Icon name="plus" size={16} stroke={2.4} />New download</span>
      <span class="text-[11px] font-normal text-white/60 pointer-coarse:hidden">{shortcut('N')}</span>
    </button>
    <button class="btn btn-outline h-9 w-full justify-between" onclick={() => ((store.batchOpen = {}), (store.navOpen = false))}>
      <span class="flex items-center gap-2"><Icon name="layers" size={15} />New batch</span>
      <span class="text-[11px] font-normal text-fg-3 pointer-coarse:hidden">{shortcut('B')}</span>
    </button>
  </div>

  <nav class="mt-5 flex-1 space-y-5 overflow-y-auto px-3 pb-4">
    <div class="space-y-px">
      {#each library as item (item.key)}
        {@const active = same(store.filter, item.filter)}
        {@const n = store.counts[item.key] ?? 0}
        {#if item.key === 'all' || n > 0 || active}
          <button
            class="group relative flex h-8 w-full items-center gap-2.5 rounded-lg px-2.5 text-left text-[13px] transition-colors duration-150
              {active ? 'bg-surface-3 text-fg' : 'text-fg-2 hover:bg-surface-2 hover:text-fg'}"
            onclick={() => (store.filter = item.filter)}
          >
            {#if active}<span class="absolute top-2 bottom-2 -left-3 w-[3px] rounded-r bg-accent"></span>{/if}
            <Icon
              name={item.icon}
              size={15}
              class={item.key === 'failed' && n > 0 ? 'text-bad' : active ? 'text-accent-hi' : 'text-fg-3 group-hover:text-fg-2'}
            />
            <span class="flex-1 truncate">{item.label}</span>
            {#if n > 0}<span class="num text-[11.5px] {active ? 'text-fg-2' : 'text-fg-3'}">{n}</span>{/if}
          </button>
        {/if}
      {/each}
    </div>

    {#if Object.keys(store.counts.cats).length > 0}
      <div>
        <div class="mb-1 px-2.5 text-[10.5px] font-semibold tracking-[0.08em] text-fg-3 uppercase">Categories</div>
        <div class="space-y-px">
          {#each categories.filter((c) => store.counts.cats[c]) as cat (cat)}
            {@const f: Filter = { kind: 'category', category: cat }}
            {@const active = same(store.filter, f)}
            <button
              class="group flex h-8 w-full items-center gap-2.5 rounded-lg px-2.5 text-left text-[13px] capitalize transition-colors duration-150
                {active ? 'bg-surface-3 text-fg' : 'text-fg-2 hover:bg-surface-2 hover:text-fg'}"
              onclick={() => (store.filter = f)}
            >
              <Icon name={categoryIcon[cat]} size={15} class={active ? 'text-accent-hi' : 'text-fg-3 group-hover:text-fg-2'} />
              <span class="flex-1 truncate">{cat}</span>
              <span class="num text-[11.5px] text-fg-3">{store.counts.cats[cat]}</span>
            </button>
          {/each}
        </div>
      </div>
    {/if}

    {#if store.batchList.length > 0}
      <div>
        <div class="mb-1 px-2.5 text-[10.5px] font-semibold tracking-[0.08em] text-fg-3 uppercase">Batches</div>
        <div class="space-y-px">
          {#each store.batchList as b (b.id)}
            {@const f: Filter = { kind: 'batch', id: b.id }}
            {@const active = same(store.filter, f)}
            {@const p = pct(b.id)}
            {@const s = store.batchStats[b.id]}
            <button
              class="group flex h-8 w-full items-center gap-2.5 rounded-lg px-2.5 text-left text-[13px] transition-colors duration-150
                {active ? 'bg-surface-3 text-fg' : 'text-fg-2 hover:bg-surface-2 hover:text-fg'}"
              onclick={() => (store.filter = f)}
              title={b.name}
            >
              <svg width="15" height="15" viewBox="0 0 16 16" class="shrink-0 -rotate-90" aria-hidden="true">
                <circle cx="8" cy="8" r="6" fill="none" stroke="var(--track)" stroke-width="2.5" />
                <circle
                  cx="8"
                  cy="8"
                  r="6"
                  fill="none"
                  stroke={s?.failed ? 'var(--bad)' : p >= 1 ? 'var(--ok)' : 'var(--accent-2)'}
                  stroke-width="2.5"
                  stroke-linecap="round"
                  stroke-dasharray="{Math.max(0.01, p) * 37.7} 37.7"
                  class="transition-[stroke-dasharray] duration-500"
                />
              </svg>
              <span class="flex-1 truncate">{b.name}</span>
              <span class="num text-[11.5px] text-fg-3">{s?.completed ?? 0}/{s?.total ?? 0}</span>
            </button>
          {/each}
        </div>
      </div>
    {/if}
  </nav>

  <div class="space-y-0.5 border-t border-line p-3">
    <button class="btn btn-ghost h-auto min-h-8 w-full justify-start gap-2.5 px-2.5 py-1.5" onclick={() => ((store.networkOpen = true), (store.navOpen = false))}>
      <Icon name="globe" size={15} class="text-fg-3" />
      <span class="min-w-0 flex-1 text-left">
        Network
        <span class="block truncate text-[11px] font-normal text-fg-3">
          {store.activeLinks.length >= 2 ? `${store.activeLinks.length} connections` : 'One connection'} · {store.proxyName('')}
        </span>
      </span>
    </button>
    <button class="btn btn-ghost h-8 w-full justify-start gap-2.5 px-2.5" onclick={() => ((store.settingsOpen = true), (store.navOpen = false))}>
      <Icon name="sliders" size={15} class="text-fg-3" />
      Settings
      <span class="ml-auto text-[11px] text-fg-3 pointer-coarse:hidden">{shortcut(',')}</span>
    </button>
  </div>
</aside>
