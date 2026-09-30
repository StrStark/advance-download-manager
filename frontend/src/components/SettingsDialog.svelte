<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'
  import Toggle from './Toggle.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { parseBytes, bytes } from '../lib/format'
  import type { Settings } from '../lib/types'
  import type { IconName } from '../lib/icons'

  let s = $state<Settings>(structuredClone($state.snapshot(store.settings)))
  let limitText = $state(store.settings.speedLimit ? bytes(store.settings.speedLimit).replace(' ', '') : '')
  const limitValid = $derived(!limitText.trim() || parseBytes(limitText) !== null)

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
  </div>

  {#snippet footer()}
    <button class="btn btn-ghost" onclick={() => (store.settingsOpen = false)}>Cancel</button>
    <button class="btn btn-primary min-w-[100px]" disabled={!limitValid} onclick={save}>Save</button>
  {/snippet}
</Modal>
