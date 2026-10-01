<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'
  import BrowserSetup from './BrowserSetup.svelte'
  import { store } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import type { Settings } from '../lib/types'

  let step = $state<'startup' | 'browser'>('startup')
  let saving = $state(false)

  async function finish(autostart: boolean) {
    saving = true
    const s = { ...($state.snapshot(store.settings) as Settings), autostart, onboarded: true }
    const r = await store.run(api.updateSettings(s), 'Could not save')
    saving = false
    if (r) {
      store.settings = r
      step = 'browser'
    }
  }
</script>

<Modal
  title={step === 'startup' ? 'Welcome to ADM' : 'Catch downloads from your browser'}
  subtitle={step === 'startup' ? 'Two quick questions, then you are set.' : 'Optional: you can do this later from Settings.'}
  width="max-w-lg"
  onclose={() => (step === 'startup' ? finish(false) : (store.onboardingOpen = false))}
>
  {#if step === 'startup'}
    <div class="flex gap-4 rounded-xl border border-line bg-surface-2 p-4">
      <div class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-soft text-accent-hi">
        <Icon name="zap" size={20} />
      </div>
      <div>
        <h3 class="text-[14px] font-semibold">Start ADM when you log in?</h3>
        <p class="mt-1 text-[12.5px] leading-relaxed text-fg-2">
          ADM opens quietly in the background, so it can resume unfinished downloads and catch new ones from your browser right
          away. You can change this anytime in Settings.
        </p>
      </div>
    </div>
  {:else}
    <BrowserSetup />
  {/if}

  {#snippet footer()}
    {#if step === 'startup'}
      <button class="btn btn-ghost" disabled={saving} onclick={() => finish(false)}>Not now</button>
      <button class="btn btn-primary" disabled={saving} onclick={() => finish(true)}>Yes, start at login</button>
    {:else}
      <button class="btn btn-primary min-w-[100px]" onclick={() => (store.onboardingOpen = false)}>Done</button>
    {/if}
  {/snippet}
</Modal>
