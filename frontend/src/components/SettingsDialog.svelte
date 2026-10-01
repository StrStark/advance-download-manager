<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'
  import Toggle from './Toggle.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { parseBytes, bytes } from '../lib/format'
  import BrowserSetup from './BrowserSetup.svelte'
  import appIcon from '../assets/appicon.png'
  import type { Settings } from '../lib/types'
  import type { IconName } from '../lib/icons'

  let s = $state<Settings>(structuredClone($state.snapshot(store.settings)))
  let limitText = $state(store.settings.speedLimit ? bytes(store.settings.speedLimit).replace(' ', '') : '')
  const limitValid = $derived(!limitText.trim() || parseBytes(limitText) !== null)

  let checking = $state(false)
  async function check() {
    checking = true
    await store.checkUpdate()
    checking = false
  }

  async function browse() {
    const d = await store.run(api.chooseDirectory(s.downloadDir))
    if (d) s.downloadDir = d
  }

  async function save() {
    if (!limitValid) return
    s.speedLimit = limitText.trim() ? (parseBytes(limitText) ?? 0) : 0
    s.maxActive = Math.min(16, Math.max(1, Number(s.maxActive) || 1))
    s.defaultConnections = Math.min(32, Math.max(1, Number(s.defaultConnections) || 1))
    s.maxRetries = Math.min(20, Math.max(0, Number(s.maxRetries) || 0))
    const r = await store.run(api.updateSettings(s), 'Could not save settings')
    if (r) {
      store.settings = r
      store.settingsOpen = false
    }
  }

  const themes: { id: Settings['theme']; label: string; icon: IconName }[] = [
    { id: 'system', label: 'System', icon: 'monitor' },
    { id: 'dark', label: 'Dark', icon: 'moon' },
    { id: 'light', label: 'Light', icon: 'sun' },
  ]
</script>

<Modal title="Settings" onclose={() => (store.settingsOpen = false)} width="max-w-xl">
  <div class="space-y-6">
    <section>
      <h3 class="mb-3 text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">General</h3>
      <label class="label" for="s-dir">Default download folder</label>
      <div class="flex gap-2">
        <input id="s-dir" class="field font-mono text-[12.5px]" bind:value={s.downloadDir} spellcheck="false" />
        {#if api.kind === 'desktop'}<button class="btn btn-outline h-9 shrink-0" onclick={browse}><Icon name="folder" size={14} />Browse</button>{/if}
      </div>
      <div class="mt-2">
        <Toggle bind:checked={s.categorizeByType} label="Sort into folders by type" hint="Video, Music, Archives… inside the download folder" />
      </div>
      <div class="mt-3">
        <span class="label">Appearance</span>
        <div class="grid grid-cols-3 gap-1 rounded-xl bg-surface-2 p-1">
          {#each themes as t (t.id)}
            <button
              class="flex h-8 items-center justify-center gap-2 rounded-lg text-[12.5px] font-medium transition-all duration-150
                {s.theme === t.id ? 'bg-surface text-fg shadow-card' : 'text-fg-3 hover:text-fg-2'}"
              onclick={() => {
                s.theme = t.id
                store.settings.theme = t.id
              }}
            >
              <Icon name={t.icon} size={14} />{t.label}
            </button>
          {/each}
        </div>
      </div>
    </section>

    <section>
      <h3 class="mb-3 text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">Performance</h3>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div>
          <label class="label" for="s-active">Downloads at once</label>
          <input id="s-active" type="number" min="1" max="16" class="field num" bind:value={s.maxActive} />
        </div>
        <div>
          <label class="label" for="s-conns">Connections per file</label>
          <input id="s-conns" type="number" min="1" max="32" class="field num" bind:value={s.defaultConnections} />
        </div>
        <div>
          <label class="label" for="s-retry">Retries</label>
          <input id="s-retry" type="number" min="0" max="20" class="field num" bind:value={s.maxRetries} />
        </div>
      </div>
      <div class="mt-3">
        <label class="label" for="s-limit">Global speed limit</label>
        <input
          id="s-limit"
          class="field num {limitValid ? '' : 'border-bad'}"
          bind:value={limitText}
          placeholder="Unlimited, or e.g. 5MB, 800KB"
          spellcheck="false"
        />
        <p class="mt-1 text-[11.5px] text-fg-3">Applies live to all running downloads. Leave empty for no limit.</p>
      </div>
    </section>

    <section>
      <h3 class="mb-1 text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">Behavior</h3>
      {#if api.kind === 'server'}
        <p class="py-1.5 text-[12.5px] text-fg-3">
          Running as a server. Folders are paths on the server, and finished files can be saved to this computer from the list.
        </p>
      {:else if api.kind === 'android'}
        <p class="py-1.5 text-[12.5px] text-fg-3">
          Downloads keep running in the background with a notification. Share a link from any app to ADM to add it.
        </p>
      {:else}
        <Toggle bind:checked={s.clipboardWatch} label="Watch clipboard for links" hint="Offer to download when you copy a URL" />
        <Toggle bind:checked={s.notifyOnComplete} label="Desktop notifications" hint="When a download or batch finishes" />
      {/if}
    </section>

    {#if store.app?.shell === 'desktop'}
      <section>
        <h3 class="mb-1 text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">Startup</h3>
        <Toggle bind:checked={s.autostart} label="Start ADM when I log in" hint="Opens minimised to resume downloads and catch browser links" />
      </section>

      <section>
        <h3 class="mb-2 text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">Browser extension</h3>
        <BrowserSetup />
      </section>
    {/if}

    <section>
      <h3 class="mb-2 text-[11px] font-semibold tracking-[0.08em] text-fg-3 uppercase">About &amp; updates</h3>
      <div class="rounded-xl border border-line bg-surface-2 p-3.5">
        <div class="flex items-center gap-3">
          <img src={appIcon} alt="" class="size-10 rounded-xl" />
          <div class="min-w-0 flex-1">
            <div class="text-[13.5px] font-semibold">ADM <span class="num font-normal text-fg-2">{store.app?.version ?? ''}</span></div>
            <div class="text-[12px] text-fg-3">
              {#if store.update?.available}
                <span class="text-accent-hi">Version {store.update.release?.version} is available</span>
              {:else if store.update}
                You're up to date
              {:else}
                {store.app ? `${store.app.os} · ${store.app.arch}` : ''}
              {/if}
            </div>
          </div>
          {#if store.update?.available && store.app?.canUpdateInApp}
            <button class="btn btn-primary h-8" disabled={!!store.updateProgress} onclick={() => store.installUpdate()}>
              <Icon name="download" size={14} />Update
            </button>
          {:else}
            <button class="btn btn-outline h-8" disabled={checking} onclick={check}>
              <Icon name={checking ? 'loader' : 'refresh'} size={14} class={checking ? 'animate-spin' : ''} />Check
            </button>
          {/if}
        </div>

        {#if store.updateProgress}
          {@const p = store.updateProgress}
          <div class="mt-3">
            <div class="h-1.5 overflow-hidden rounded-full bg-track">
              <div class="h-full rounded-full bg-accent transition-[width] duration-200" style="width:{p.total ? (p.done / p.total) * 100 : 5}%"></div>
            </div>
            <p class="mt-1.5 text-[11.5px] text-fg-3">
              {p.total && p.done >= p.total ? 'Installing… ADM will restart.' : `Downloading update · ${bytes(p.done)} of ${bytes(p.total)}`}
            </p>
          </div>
        {/if}

        {#if store.update?.available && !store.app?.canUpdateInApp}
          <p class="mt-3 rounded-lg bg-surface px-3 py-2 font-mono text-[11.5px] text-fg-2">
            {#if store.app?.install === 'container'}docker compose pull &amp;&amp; docker compose up -d{:else}Download the new version from the release page.{/if}
          </p>
        {/if}

        {#if store.update?.available && store.update.release?.notes}
          <details class="mt-3 text-[12px] text-fg-2">
            <summary class="cursor-pointer text-fg-3">What's new</summary>
            <pre class="mt-2 max-h-40 overflow-y-auto font-sans whitespace-pre-wrap">{store.update.release.notes}</pre>
          </details>
        {/if}

        <div class="mt-3 flex items-center justify-between border-t border-line pt-2.5 text-[12px]">
          <label class="flex cursor-pointer items-center gap-2 text-fg-2">
            <input type="checkbox" checked={!s.noUpdateCheck} onchange={(e) => (s.noUpdateCheck = !e.currentTarget.checked)} class="accent-[var(--accent)]" />
            Check for updates automatically
          </label>
          {#if store.app}
            <a class="text-accent-hi underline-offset-2 hover:underline" href={store.app.repoUrl + '/releases'} target="_blank" rel="noreferrer">Release notes</a>
          {/if}
        </div>
      </div>
    </section>
  </div>

  {#snippet footer()}
    <button class="btn btn-ghost" onclick={() => (store.settingsOpen = false)}>Cancel</button>
    <button class="btn btn-primary min-w-[100px]" disabled={!limitValid} onclick={save}>Save</button>
  {/snippet}
</Modal>
