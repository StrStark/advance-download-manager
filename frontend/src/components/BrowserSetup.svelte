<script lang="ts">
  import Icon from './Icon.svelte'
  import { store } from '../lib/store.svelte'

  const releases = $derived(`${store.app?.repoUrl ?? 'https://github.com/StrStark/advance-download-manager'}/releases/latest`)
  let browser = $state<'chromium' | 'firefox'>('chromium')
</script>

<p class="text-[12.5px] leading-relaxed text-fg-2">
  With the ADM extension, downloads you start in the browser open in ADM instead, with your login session included. ADM is already set
  up to receive them; just add the extension:
</p>

<div class="mt-3 grid grid-cols-2 gap-1 rounded-xl bg-surface-2 p-1" role="tablist">
  {#each [['chromium', 'Chrome · Edge · Brave'], ['firefox', 'Firefox']] as [id, label] (id)}
    <button
      class="h-8 rounded-lg text-[12.5px] font-medium transition-all duration-150 {browser === id
        ? 'bg-surface text-fg shadow-card'
        : 'text-fg-3 hover:text-fg-2'}"
      onclick={() => (browser = id as typeof browser)}
      role="tab"
      aria-selected={browser === id}>{label}</button
    >
  {/each}
</div>

<ol class="mt-3 space-y-2 text-[12.5px] text-fg-2">
  {#if browser === 'chromium'}
    <li class="flex gap-2.5">
      <span class="num flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[11px] text-accent-hi">1</span>
      <span>
        Download <span class="font-mono text-fg">adm-browser-extension_…_chrome.zip</span> from the
        <a class="text-accent-hi underline underline-offset-2" href={releases} target="_blank" rel="noreferrer">latest release</a>
        and unzip it.
      </span>
    </li>
    <li class="flex gap-2.5">
      <span class="num flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[11px] text-accent-hi">2</span>
      <span>Open <span class="font-mono text-fg">chrome://extensions</span> (or <span class="font-mono text-fg">edge://extensions</span>) and turn on <b>Developer mode</b>.</span>
    </li>
    <li class="flex gap-2.5">
      <span class="num flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[11px] text-accent-hi">3</span>
      <span>Click <b>Load unpacked</b> and pick the unzipped folder. The ADM icon's popup should say <b>Connected</b>.</span>
    </li>
  {:else}
    <li class="flex gap-2.5">
      <span class="num flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[11px] text-accent-hi">1</span>
      <span>
        Download <span class="font-mono text-fg">adm-browser-extension_…_firefox.zip</span> from the
        <a class="text-accent-hi underline underline-offset-2" href={releases} target="_blank" rel="noreferrer">latest release</a>.
      </span>
    </li>
    <li class="flex gap-2.5">
      <span class="num flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[11px] text-accent-hi">2</span>
      <span>Open <span class="font-mono text-fg">about:debugging#/runtime/this-firefox</span> → <b>Load Temporary Add-on</b> → pick the zip.</span>
    </li>
    <li class="flex gap-2.5 text-fg-3">
      <Icon name="info" size={14} class="mt-0.5 shrink-0" />
      <span>Firefox removes temporary add-ons when it restarts until the extension is published on addons.mozilla.org.</span>
    </li>
  {/if}
</ol>
