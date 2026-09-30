<script lang="ts">
  import { onMount } from 'svelte'
  import { slide } from 'svelte/transition'
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'
  import Toggle from './Toggle.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { relTime, speed } from '../lib/format'
  import { linkIcon } from '../lib/icons'
  import type { LinkCheck, ProxyProfile, ProxyTest, Settings, Subscription } from '../lib/types'

  // Edits happen on a draft; nothing is saved until "Save".
  const snap = $state.snapshot(store.settings) as Settings
  let multiLink = $state(snap.multiLink)
  let selected = $state<string[]>(snap.links?.length ? [...snap.links] : store.links.filter((l) => l.enabled).map((l) => l.id))
  let proxies = $state<ProxyProfile[]>([...(snap.proxies ?? [])])
  let subs = $state<Subscription[]>([...(snap.subscriptions ?? [])])
  let defaultProxy = $state(snap.defaultProxy || '')
  let bypass = $state((snap.proxyBypass ?? []).join('\n'))

  let checks = $state<Record<string, LinkCheck | 'checking'>>({})
  let tests = $state<Record<string, ProxyTest | 'testing'>>({})
  let pasteText = $state('')
  let pasteErrors = $state<string[]>([])
  let subURL = $state('')
  let subName = $state('')
  let subBusy = $state<string | null>(null)
  let xray = $state(true)
  let saving = $state(false)
  let tab = $state<'connections' | 'proxies'>('connections')

  const isAndroid = api.kind === 'android'

  onMount(() => {
    store.refreshLinks().then(checkLinks)
    api.xrayAvailable().then((v) => (xray = v)).catch(() => {})
  })

  async function checkLinks() {
    for (const l of store.links) checks[l.id] = 'checking'
    const res = await store.run(api.checkLinks(), 'Could not test connections')
    if (res) for (const l of store.links) checks[l.id] = res[l.id] ?? { latencyMs: 0, error: 'not tested' }
  }

  function toggleLink(id: string) {
    selected = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
  }

  const willCombine = $derived(multiLink && store.links.filter((l) => selected.includes(l.id)).length >= 2)

  // ---------- proxies ----------
  const typeLabel: Record<string, string> = {
    vless: 'VLESS',
    vmess: 'VMess',
    trojan: 'Trojan',
    shadowsocks: 'SS',
    socks5: 'SOCKS5',
    http: 'HTTP',
    https: 'HTTPS',
  }

  async function addPasted() {
    const res = await store.run(api.parseProxies(pasteText), 'Could not read links')
    if (!res) return
    const added = res.profiles ?? []
    proxies = [...proxies, ...added]
    pasteErrors = res.errors ?? []
    if (added.length) {
      pasteText = ''
      if (!defaultProxy && proxies.length === added.length) defaultProxy = added[0].id
      store.toast('success', `Added ${added.length} server${added.length === 1 ? '' : 's'}`, 'Remember to save')
    } else if (!pasteErrors.length) {
      pasteErrors = ['No proxy links found']
    }
  }

  async function test(p: ProxyProfile) {
    tests[p.id] = 'testing'
    tests[p.id] = await api.testProxy($state.snapshot(p) as ProxyProfile).catch((e) => ({ ok: false, latencyMs: 0, error: String(e) }))
  }
  async function testAll() {
    await Promise.all(proxies.map((p) => test(p)))
  }

  function remove(p: ProxyProfile) {
    proxies = proxies.filter((x) => x.id !== p.id)
    if (defaultProxy === p.id) defaultProxy = ''
  }

  /** Subscriptions are often blocked: try the saved proxy first, then direct. */
  async function fetchSub(url: string) {
    const via = store.settings.defaultProxy
    if (via && via !== 'direct') {
      try {
        return await api.fetchSubscription(url, via)
      } catch {
        /* fall back to a direct connection */
      }
    }
    return api.fetchSubscription(url, 'direct')
  }

  async function addSubscription() {
    const url = subURL.trim()
    if (!url) return
    subBusy = 'new'
    const res = await store.run(fetchSub(url), 'Could not load subscription')
    subBusy = null
    if (!res) return
    const id = crypto.randomUUID().slice(0, 16)
    let host = url
    try {
      host = new URL(url).hostname
    } catch {
      /* keep url */
    }
    subs = [...subs, { id, name: subName.trim() || host, url, updatedAt: Date.now() }]
    proxies = [...proxies, ...(res.profiles ?? []).map((p) => ({ ...p, subscriptionId: id }))]
    subURL = ''
    subName = ''
    store.toast('success', `Subscription added: ${res.profiles?.length ?? 0} servers`, 'Remember to save')
  }

  async function updateSubscription(s: Subscription) {
    subBusy = s.id
    const res = await store.run(fetchSub(s.url), `Could not update ${s.name}`)
    subBusy = null
    if (!res) return
    const fresh = (res.profiles ?? []).map((p) => ({ ...p, subscriptionId: s.id }))
    const oldDefault = proxies.find((p) => p.id === defaultProxy)
    proxies = [...proxies.filter((p) => p.subscriptionId !== s.id), ...fresh]
    // Keep the default pointing at the "same" server when it came from this subscription.
    if (oldDefault?.subscriptionId === s.id) defaultProxy = fresh.find((p) => p.name === oldDefault.name)?.id ?? ''
    subs = subs.map((x) => (x.id === s.id ? { ...x, updatedAt: Date.now() } : x))
    store.toast('success', `${s.name}: ${fresh.length} servers`)
  }

  function removeSubscription(s: Subscription) {
    subs = subs.filter((x) => x.id !== s.id)
    proxies = proxies.filter((p) => p.subscriptionId !== s.id)
    if (!proxies.some((p) => p.id === defaultProxy) && !['', 'direct', 'system'].includes(defaultProxy)) defaultProxy = ''
  }

  const groups = $derived.by(() => {
    const byId = new Map(subs.map((s) => [s.id, s]))
    const out: { title: string; sub?: Subscription; items: ProxyProfile[] }[] = []
    const own = proxies.filter((p) => !p.subscriptionId || !byId.has(p.subscriptionId))
    if (own.length) out.push({ title: 'Your servers', items: own })
    for (const s of subs) out.push({ title: s.name, sub: s, items: proxies.filter((p) => p.subscriptionId === s.id) })
    return out
  })

  async function save() {
    saving = true
    const s: Settings = {
      ...(structuredClone($state.snapshot(store.settings)) as Settings),
      multiLink,
      links: selected,
      proxies: $state.snapshot(proxies) as ProxyProfile[],
      subscriptions: $state.snapshot(subs) as Subscription[],
      defaultProxy,
      proxyBypass: bypass
        .split('\n')
        .map((x) => x.trim())
        .filter(Boolean),
    }
    const r = await store.run(api.updateSettings(s), 'Could not save network settings')
    saving = false
    if (r) {
      store.settings = r
      store.refreshLinks()
      store.networkOpen = false
    }
  }
</script>

<Modal title="Network" subtitle="Connections and proxies used for downloads" width="max-w-2xl" onclose={() => (store.networkOpen = false)}>
  <div class="mb-4 grid grid-cols-2 gap-1 rounded-xl bg-surface-2 p-1" role="tablist">
    {#each [['connections', 'Connections', 'merge'], ['proxies', 'Proxies', 'globe']] as [id, label, icon] (id)}
      <button
        class="flex h-8 items-center justify-center gap-2 rounded-lg text-[12.5px] font-medium transition-all duration-150
          {tab === id ? 'bg-surface text-fg shadow-card' : 'text-fg-3 hover:text-fg-2'}"
        onclick={() => (tab = id as typeof tab)}
        role="tab"
        aria-selected={tab === id}
      >
        <Icon name={icon as 'merge' | 'globe'} size={14} />{label}
        {#if id === 'proxies' && proxies.length}<span class="num text-[11px] text-fg-3">{proxies.length}</span>{/if}
      </button>
    {/each}
  </div>

  {#if tab === 'connections'}
    <div class="rounded-xl border border-line bg-surface-2 px-3.5 py-1.5">
      <Toggle
        bind:checked={multiLink}
        label="Combine connections"
        hint={isAndroid
          ? 'Use Wi-Fi and mobile data together for faster downloads (uses your data plan)'
          : 'Split each download across several connections, e.g. Ethernet + a phone hotspot'}
      />
    </div>

    <div class="mt-4 mb-2 flex items-center justify-between">
      <h3 class="text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">This device</h3>
      <button class="btn btn-ghost h-7 px-2 text-[12px]" onclick={checkLinks}><Icon name="refresh" size={13} />Test</button>
    </div>

    <div class="space-y-1.5">
      {#each store.links as l (l.id)}
        {@const c = checks[l.id]}
        {@const on = selected.includes(l.id)}
        {@const sp = store.linkSpeed[l.id] ?? 0}
        <div class="flex items-center gap-3 rounded-xl border border-line px-3 py-2.5 {multiLink && !on ? 'opacity-55' : ''}">
          <div class="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-accent-soft text-accent-hi">
            <Icon name={linkIcon[l.kind] ?? 'globe'} size={17} />
          </div>
          <div class="min-w-0 flex-1">
            <div class="truncate text-[13px] font-medium">{l.label}</div>
            <div class="flex items-center gap-1.5 truncate text-[11.5px] text-fg-3">
              <span class="num truncate">{(l.addrs ?? []).join(', ')}</span>
              {#if c === 'checking'}
                <span class="text-line-strong">•</span><Icon name="loader" size={11} class="animate-spin" />
              {:else if c?.error}
                <span class="text-line-strong">•</span><span class="text-bad">{c.error}</span>
              {:else if c}
                <span class="text-line-strong">•</span><span class="num text-ok">{c.latencyMs} ms</span>
              {/if}
              {#if sp > 0}
                <span class="text-line-strong">•</span><span class="num text-accent-2">{speed(sp)}</span>
              {/if}
            </div>
          </div>
          {#if multiLink}
            <label class="relative inline-flex shrink-0 cursor-pointer" title={on ? 'Used for downloads' : 'Not used'}>
              <input type="checkbox" class="peer sr-only" checked={on} onchange={() => toggleLink(l.id)} />
              <span class="h-5 w-9 rounded-full bg-surface-3 ring-1 ring-line transition-colors peer-checked:bg-accent peer-checked:ring-accent"></span>
              <span class="absolute top-0.5 left-0.5 size-4 rounded-full bg-white shadow transition-transform peer-checked:translate-x-4"></span>
            </label>
          {/if}
        </div>
      {:else}
        <p class="rounded-xl border border-dashed border-line px-3 py-6 text-center text-[12.5px] text-fg-3">No network connections found.</p>
      {/each}
    </div>

    {#if multiLink}
      <p class="mt-3 flex gap-2 text-[12px] leading-relaxed {willCombine ? 'text-fg-3' : 'text-warn'}" transition:slide={{ duration: 150 }}>
        <Icon name="info" size={14} class="mt-px" />
        {#if willCombine}
          Downloads will be split across {selected.filter((id) => store.links.some((l) => l.id === id)).length} connections. It only speeds
          things up when each one reaches the internet separately (for example your home line and a phone hotspot), not several cables to
          the same router.
        {:else}
          Pick at least two connections. Until then downloads use the normal route.
        {/if}
      </p>
    {/if}
  {:else}
    <!-- ================= PROXIES ================= -->
    <div class="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
      <div>
        <label class="label" for="net-default">Use for downloads</label>
        <select id="net-default" class="field" bind:value={defaultProxy}>
          <option value="">Direct (no proxy)</option>
          <option value="system">System proxy settings</option>
          {#each groups as g (g.title)}
            <optgroup label={g.title}>
              {#each g.items as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
            </optgroup>
          {/each}
        </select>
      </div>
      {#if proxies.length}
        <button class="btn btn-outline h-9" onclick={testAll}><Icon name="zap" size={14} />Test all</button>
      {/if}
    </div>

    {#if !xray}
      <p class="mt-2 text-[12px] text-warn">This build has no V2Ray/Xray support; only HTTP and SOCKS5 proxies work.</p>
    {/if}

    <div class="mt-4 max-h-[300px] space-y-3 overflow-y-auto pr-1">
      {#each groups as g (g.title)}
        <div>
          <div class="mb-1.5 flex items-center gap-2">
            <h3 class="flex-1 truncate text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">{g.title}</h3>
            {#if g.sub}
              <span class="text-[11px] text-fg-3">updated {relTime(g.sub.updatedAt)}</span>
              <button class="icon-btn size-6" title="Update subscription" disabled={subBusy !== null} onclick={() => updateSubscription(g.sub!)}>
                <Icon name={subBusy === g.sub.id ? 'loader' : 'refresh'} size={12} class={subBusy === g.sub.id ? 'animate-spin' : ''} />
              </button>
              <button class="icon-btn size-6 hover:text-bad" title="Remove subscription" onclick={() => removeSubscription(g.sub!)}>
                <Icon name="trash" size={12} />
              </button>
            {/if}
          </div>
          <div class="space-y-1">
            {#each g.items as p (p.id)}
              {@const t = tests[p.id]}
              <div
                class="group flex items-center gap-2.5 rounded-lg border px-2.5 py-2 {defaultProxy === p.id
                  ? 'border-accent/50 bg-accent-soft'
                  : 'border-line'}"
              >
                <span class="w-[58px] shrink-0 rounded bg-surface-3 px-1.5 py-0.5 text-center font-mono text-[10.5px] font-semibold text-fg-2">
                  {typeLabel[p.type] ?? p.type}
                </span>
                <button class="min-w-0 flex-1 text-left" onclick={() => (defaultProxy = p.id)} title="Use for downloads">
                  <div class="truncate text-[12.5px] font-medium">{p.name}</div>
                  <div class="num truncate text-[11px] text-fg-3">{p.server}</div>
                </button>
                <span class="w-[92px] shrink-0 text-right text-[11.5px]">
                  {#if t === 'testing'}
                    <Icon name="loader" size={12} class="ml-auto animate-spin text-fg-3" />
                  {:else if t?.ok}
                    <span class="num text-ok" title={t.ip ? `Exit IP ${t.ip}` : ''}>{t.latencyMs} ms</span>
                  {:else if t}
                    <span class="truncate text-bad" title={t.error}>{t.error ?? 'failed'}</span>
                  {/if}
                </span>
                <button class="icon-btn size-7" title="Test" onclick={() => test(p)}><Icon name="zap" size={13} /></button>
                <button class="icon-btn size-7 hover:text-bad" title="Remove" onclick={() => remove(p)}><Icon name="trash" size={13} /></button>
              </div>
            {:else}
              <p class="px-1 text-[12px] text-fg-3">No servers.</p>
            {/each}
          </div>
        </div>
      {:else}
        <p class="rounded-xl border border-dashed border-line px-3 py-6 text-center text-[12.5px] text-fg-3">
          No proxies yet. Paste a link or add a subscription below.
        </p>
      {/each}
    </div>

    <div class="mt-4 space-y-3 border-t border-line pt-4">
      <div>
        <label class="label" for="net-paste">Add servers</label>
        <textarea
          id="net-paste"
          class="field h-20 resize-none py-2 font-mono text-[11.5px]"
          bind:value={pasteText}
          placeholder={'vless://…  vmess://…  trojan://…  ss://…\nsocks5://127.0.0.1:1080   http://proxy:3128'}
          spellcheck="false"
        ></textarea>
        <div class="mt-1.5 flex items-start justify-between gap-3">
          <div class="min-w-0 text-[11.5px] text-bad">
            {#each pasteErrors.slice(0, 3) as e (e)}<div class="truncate">{e}</div>{/each}
          </div>
          <button class="btn btn-outline h-8 shrink-0" disabled={!pasteText.trim()} onclick={addPasted}><Icon name="plus" size={14} />Add</button>
        </div>
      </div>
      <div>
        <span class="label">Subscription</span>
        <div class="flex flex-wrap gap-2">
          <input class="field h-8 w-36 shrink-0 text-[12.5px]" bind:value={subName} placeholder="Name (optional)" />
          <input class="field h-8 min-w-[200px] flex-1 font-mono text-[11.5px]" bind:value={subURL} placeholder="https://…/sub/…" spellcheck="false" />
          <button class="btn btn-outline h-8" disabled={!subURL.trim() || subBusy !== null} onclick={addSubscription}>
            <Icon name={subBusy === 'new' ? 'loader' : 'plus'} size={14} class={subBusy === 'new' ? 'animate-spin' : ''} />Add
          </button>
        </div>
      </div>
      <div>
        <label class="label" for="net-bypass">Don't use the proxy for <span class="font-normal text-fg-3">(one per line)</span></label>
        <textarea id="net-bypass" class="field h-16 resize-none py-2 font-mono text-[11.5px]" bind:value={bypass} spellcheck="false"></textarea>
        <p class="mt-1 text-[11px] text-fg-3">Domains (also match subdomains), IPs, CIDR ranges, <code class="font-mono">&lt;local&gt;</code> for your local network.</p>
      </div>
    </div>
  {/if}

  {#snippet footer()}
    <button class="btn btn-ghost" onclick={() => (store.networkOpen = false)}>Cancel</button>
    <button class="btn btn-primary min-w-[100px]" disabled={saving} onclick={save}>Save</button>
  {/snippet}
</Modal>
