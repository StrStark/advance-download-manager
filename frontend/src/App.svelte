<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import Sidebar from './components/Sidebar.svelte'
  import Topbar from './components/Topbar.svelte'
  import DownloadList from './components/DownloadList.svelte'
  import StatusBar from './components/StatusBar.svelte'
  import DetailsDrawer from './components/DetailsDrawer.svelte'
  import AddDialog from './components/AddDialog.svelte'
  import BatchDialog from './components/BatchDialog.svelte'
  import SettingsDialog from './components/SettingsDialog.svelte'
  import ConfirmRemove from './components/ConfirmRemove.svelte'
  import NetworkDialog from './components/NetworkDialog.svelte'
  import Toasts from './components/Toasts.svelte'
  import Icon from './components/Icon.svelte'
  import { store, isRunning } from './lib/store.svelte'
  import { extractURLs } from './lib/batchutil'

  let search: HTMLInputElement | undefined = $state()
  let dragging = $state(false)
  let dragDepth = 0
  let systemDark = $state(true)

  onMount(() => {
    store.init().catch((e) => store.toast('error', 'Failed to load', String(e)))
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    systemDark = mq.matches
    const onChange = (e: MediaQueryListEvent) => (systemDark = e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  })

  $effect(() => {
    const t = store.settings.theme
    document.documentElement.dataset.theme = t === 'system' ? (systemDark ? 'dark' : 'light') : t
  })

  const modalOpen = $derived(!!(store.addOpen || store.batchOpen || store.settingsOpen || store.networkOpen || store.confirmRemove))

  // Picking a view in the drawer closes it (narrow screens).
  $effect(() => {
    void store.filter
    store.navOpen = false
  })

  // Android back button: close the top-most layer. Returning false lets the
  // app go to the background.
  window.__admBack = () => {
    if (store.confirmRemove) store.confirmRemove = null
    else if (store.addOpen) store.addOpen = null
    else if (store.batchOpen) store.batchOpen = null
    else if (store.settingsOpen) store.settingsOpen = false
    else if (store.networkOpen) store.networkOpen = false
    else if (store.navOpen) store.navOpen = false
    else if (store.detailsId) store.detailsId = null
    else if (store.selected.size) store.selected.clear()
    else if (store.filter.kind !== 'all') store.filter = { kind: 'all' }
    else return false
    return true
  }

  /** Routes a chunk of text: one link → add dialog, many → batch dialog. */
  function takeText(text: string): boolean {
    const urls = extractURLs(text)
    if (!urls.length) return false
    if (urls.length === 1) store.addOpen = { url: urls[0] }
    else store.batchOpen = { text: urls.join('\n') }
    return true
  }

  function typing(e: Event) {
    const t = e.target as HTMLElement
    return t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable
  }

  function onkeydown(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey
    if (modalOpen) return
    if (mod && e.key === 'n') {
      e.preventDefault()
      store.addOpen = {}
    } else if (mod && e.key === 'b') {
      e.preventDefault()
      store.batchOpen = {}
    } else if (mod && (e.key === 'k' || e.key === 'f' || e.key === 'l')) {
      e.preventDefault()
      search?.focus()
      search?.select()
    } else if (mod && e.key === ',') {
      e.preventDefault()
      store.settingsOpen = true
    } else if (typing(e)) {
      return
    } else if (mod && e.key === 'a') {
      e.preventDefault()
      store.visibleIds.forEach((id) => store.selected.add(id))
    } else if (e.key === 'Delete' && store.selected.size) {
      store.askRemove([...store.selected])
    } else if (e.key === ' ' && store.selected.size) {
      e.preventDefault()
      const ids = [...store.selected]
      const anyRunning = ids.some((id) => {
        const s = store.jobs[id]?.status
        return s && (isRunning(s) || s === 'queued')
      })
      if (anyRunning) store.pause(ids)
      else store.resume(ids)
    } else if (e.key === 'Escape') {
      if (store.detailsId) store.detailsId = null
      else store.selected.clear()
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const ids = store.visibleIds
      if (!ids.length) return
      e.preventDefault()
      const cur = store.detailsId ? ids.indexOf(store.detailsId) : -1
      const next = ids[Math.max(0, Math.min(ids.length - 1, cur + (e.key === 'ArrowDown' ? 1 : -1)))]
      store.select(next, e)
      store.detailsId = next
    }
  }

  function onpaste(e: ClipboardEvent) {
    if (modalOpen || typing(e)) return
    const text = e.clipboardData?.getData('text') ?? ''
    if (takeText(text)) e.preventDefault()
  }

  function dropText(e: DragEvent): string {
    const dt = e.dataTransfer
    if (!dt) return ''
    return dt.getData('text/uri-list') || dt.getData('text/plain') || ''
  }
</script>

<svelte:window
  {onkeydown}
  {onpaste}
  ondragenter={(e) => {
    if (!e.dataTransfer?.types.some((t) => t === 'text/uri-list' || t === 'text/plain')) return
    dragDepth++
    dragging = true
  }}
  ondragleave={() => {
    dragDepth = Math.max(0, dragDepth - 1)
    if (!dragDepth) dragging = false
  }}
  ondragover={(e) => dragging && e.preventDefault()}
  ondrop={(e) => {
    dragDepth = 0
    dragging = false
    const text = dropText(e)
    if (text) {
      e.preventDefault()
      if (!takeText(text)) store.toast('info', 'No links found', 'Drop a URL or text that contains links.')
    }
  }}
/>

<div class="flex h-full">
  <Sidebar />
  {#if store.navOpen}
    <div
      class="fixed inset-0 z-30 bg-[var(--overlay)] md:hidden"
      transition:fade={{ duration: 150 }}
      onclick={() => (store.navOpen = false)}
      role="presentation"
    ></div>
  {/if}
  <main class="flex min-w-0 flex-1 flex-col" style="padding-top: var(--inset-top)">
    <Topbar bind:input={search} />
    <DownloadList />
    <StatusBar />
  </main>
  <DetailsDrawer />
</div>

<!-- Phones: primary action within thumb reach. -->
<button
  class="btn btn-primary fixed right-4 z-20 size-14 rounded-2xl p-0 shadow-[0_10px_28px_-8px_var(--accent)] md:hidden"
  style="bottom: calc(3rem + var(--inset-bottom))"
  onclick={() => (store.addOpen = {})}
  aria-label="New download"
>
  <Icon name="plus" size={24} stroke={2.4} />
</button>

{#if store.addOpen}<AddDialog initialUrl={store.addOpen.url} />{/if}
{#if store.batchOpen}<BatchDialog initialText={store.batchOpen.text} initialTab={store.batchOpen.tab} />{/if}
{#if store.settingsOpen}<SettingsDialog />{/if}
{#if store.networkOpen}<NetworkDialog />{/if}
{#if store.confirmRemove}<ConfirmRemove ids={store.confirmRemove.ids} label={store.confirmRemove.label} />{/if}
<Toasts />

{#if dragging}
  <div class="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-[var(--overlay)] p-6 backdrop-blur-sm" transition:fade={{ duration: 120 }}>
    <div class="flex h-full w-full flex-col items-center justify-center rounded-3xl border-2 border-dashed border-accent/70 bg-accent/5">
      <div class="flex size-14 items-center justify-center rounded-2xl bg-accent text-white shadow-[0_12px_32px_-10px_var(--accent)]">
        <Icon name="download" size={26} />
      </div>
      <p class="mt-4 text-[16px] font-semibold">Drop to download</p>
      <p class="mt-1 text-[13px] text-fg-2">Links, or text containing links</p>
    </div>
  </div>
{/if}
