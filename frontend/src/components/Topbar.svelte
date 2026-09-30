<script lang="ts">
  import Icon from './Icon.svelte'
  import Sparkline from './Sparkline.svelte'
  import { store, isRunning } from '../lib/store.svelte'
  import { speed } from '../lib/format'
  import { extractURLs } from '../lib/batchutil'
  import { mod } from '../lib/platform'

  let { input = $bindable() }: { input?: HTMLInputElement } = $props()

  const urls = $derived(/https?:\/\//i.test(store.search) ? extractURLs(store.search) : [])

  const title = $derived.by(() => {
    const f = store.filter
    if (f.kind === 'all') return 'All downloads'
    if (f.kind === 'status')
      return { active: 'Downloading', queued: 'Queued', paused: 'Paused', completed: 'Completed', failed: 'Failed' }[f.status]
    if (f.kind === 'category') return f.category[0].toUpperCase() + f.category.slice(1)
    return store.batches[f.id]?.name ?? 'Batch'
  })

  const anyRunning = $derived(store.ordered.some((j) => isRunning(j.status) || j.status === 'queued'))
  const anyPaused = $derived(store.ordered.some((j) => j.status === 'paused'))

  function submit(e: KeyboardEvent) {
    if (e.key === 'Enter' && urls.length) {
      if (urls.length === 1) store.addOpen = { url: urls[0] }
      else store.batchOpen = { text: urls.join('\n') }
      store.search = ''
      input?.blur()
    }
    if (e.key === 'Escape') {
      store.search = ''
      input?.blur()
    }
  }
</script>

<header class="@container flex h-[60px] shrink-0 items-center gap-2 border-b border-line px-3 sm:gap-4 sm:px-5">
  <button class="icon-btn md:hidden" onclick={() => (store.navOpen = true)} aria-label="Open menu">
    <Icon name="menu" size={18} />
  </button>
  <h1 class="hidden max-w-[220px] shrink-0 truncate text-[15px] font-semibold tracking-[-0.01em] @xl:block">{title}</h1>

  <div class="relative mx-auto w-full max-w-[460px] min-w-0">
    <Icon name={urls.length ? 'link' : 'search'} size={15} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-fg-3" />
    <input
      bind:this={input}
      bind:value={store.search}
      onkeydown={submit}
      class="field h-9 pr-24 pl-9"
      placeholder="Search downloads, or paste a link…"
      spellcheck="false"
    />
    <div class="absolute top-1/2 right-2 flex -translate-y-1/2 items-center gap-1">
      {#if urls.length}
        <span class="rounded-md bg-accent px-1.5 py-0.5 text-[11px] font-semibold text-white">
          ↵ {urls.length === 1 ? 'Download' : `Add ${urls.length}`}
        </span>
      {:else}
        <span class="hidden gap-1 pointer-fine:flex"><span class="kbd">{mod}</span><span class="kbd">K</span></span>
      {/if}
    </div>
  </div>

  <div class="flex shrink-0 items-center gap-1 sm:gap-3">
    <div class="hidden items-center gap-3 @3xl:flex" title="Total download speed">
      <Sparkline values={store.history} width={110} height={26} />
      <div class="w-[92px] text-right leading-tight">
        <div class="num text-[14px] font-semibold text-fg">{speed(store.totalSpeed)}</div>
        <div class="text-[11px] text-fg-3">
          {store.settings.speedLimit > 0 ? `limit ${speed(store.settings.speedLimit)}` : 'no limit'}
        </div>
      </div>
    </div>
    <div class="hidden h-6 w-px bg-line @3xl:block"></div>
    <button class="icon-btn" title="Pause all" onclick={() => store.pauseAll()} disabled={!anyRunning}>
      <Icon name="pause" size={15} />
    </button>
    <button class="icon-btn" title="Resume all" onclick={() => store.resumeAll()} disabled={!anyPaused}>
      <Icon name="play" size={15} />
    </button>
  </div>
</header>
