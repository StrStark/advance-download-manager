<script lang="ts">
  import Icon from './Icon.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { bytes, speed } from '../lib/format'

  let free = $state<number | null>(null)
  let menu = $state(false)

  $effect(() => {
    const dir = store.settings.downloadDir
    if (!dir) return
    const load = () => api.diskFree(dir).then((n) => (free = n)).catch(() => (free = null))
    load()
    const t = setInterval(load, 15000)
    return () => clearInterval(t)
  })

  const MB = 1024 * 1024
  const presets: [string, number][] = [
    ['Unlimited', 0],
    ['50 MB/s', 50 * MB],
    ['20 MB/s', 20 * MB],
    ['10 MB/s', 10 * MB],
    ['5 MB/s', 5 * MB],
    ['1 MB/s', 1 * MB],
    ['256 KB/s', 256 * 1024],
  ]
  function pick(v: number) {
    menu = false
    store.run(api.setSpeedLimit(v))
  }
</script>

<footer class="relative flex h-8 shrink-0 items-center gap-3 border-t border-line px-3 text-[11.5px] whitespace-nowrap text-fg-3 sm:gap-4 sm:px-5" style="box-sizing: content-box; padding-bottom: var(--inset-bottom)">
  <span class="flex items-center gap-1.5">
    <span class="size-1.5 rounded-full {store.counts.active ? 'bg-ok shadow-[0_0_6px_var(--ok)]' : 'bg-fg-3/60'}"></span>
    <span class="num">{store.counts.active}</span> active
  </span>
  <span class="hidden sm:inline"><span class="num">{store.counts.queued}</span> queued</span>
  {#if api.kind === 'mock'}
    <span class="rounded bg-warn-soft px-1.5 py-px font-medium text-warn" title="Running in a browser without the Go engine">Preview<span class="hidden sm:inline"> · simulated engine</span></span>
  {:else if api.kind === 'server' && !store.online}
    <span class="rounded bg-bad-soft px-1.5 py-px font-medium text-bad">Reconnecting to server…</span>
  {/if}
  {#if store.selected.size > 1}
    <span class="text-accent-hi"><span class="num">{store.selected.size}</span> selected</span>
  {/if}

  <span class="ml-auto flex items-center gap-1.5">
    <Icon name="arrowDown" size={12} />
    <span class="num text-fg-2">{speed(store.totalSpeed)}</span>
  </span>

  <div class="relative">
    <button
      class="flex h-6 items-center gap-1.5 rounded-md px-1.5 transition-colors hover:bg-surface-2 hover:text-fg {store.settings.speedLimit
        ? 'text-warn'
        : ''}"
      onclick={() => (menu = !menu)}
      aria-haspopup="menu"
      aria-expanded={menu}
    >
      <Icon name="gauge" size={13} />
      <span class="hidden sm:inline">{store.settings.speedLimit ? `Limit ${speed(store.settings.speedLimit)}` : 'No limit'}</span>
    </button>
    {#if menu}
      <div class="fixed inset-0 z-30" onclick={() => (menu = false)} role="presentation"></div>
      <div
        class="absolute right-0 bottom-8 z-40 w-44 rounded-xl border border-line bg-surface p-1 shadow-card"
        role="menu"
      >
        <div class="px-2.5 pt-1.5 pb-1 text-[10.5px] font-semibold tracking-[0.08em] text-fg-3 uppercase">Speed limit</div>
        {#each presets as [label, v] (v)}
          <button
            class="flex h-8 w-full items-center justify-between rounded-lg px-2.5 text-[12.5px] text-fg-2 hover:bg-surface-2 hover:text-fg"
            onclick={() => pick(v)}
            role="menuitem"
          >
            {label}
            {#if store.settings.speedLimit === v}<Icon name="check" size={14} class="text-accent-hi" />{/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>

  {#if free != null}
    <span class="hidden items-center gap-1.5 sm:flex" title={store.settings.downloadDir}>
      <Icon name="drive" size={13} />
      <span class="num">{bytes(free)}</span> free
    </span>
  {/if}
</footer>
