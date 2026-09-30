<script lang="ts">
  import type { Snippet } from 'svelte'
  import { fade, scale } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import Icon from './Icon.svelte'

  let {
    title,
    subtitle,
    width = 'max-w-lg',
    onclose,
    children,
    footer,
  }: { title: string; subtitle?: string; width?: string; onclose: () => void; children: Snippet; footer?: Snippet } = $props()

  let panel: HTMLDivElement

  $effect(() => {
    const el = panel.querySelector<HTMLElement>('[autofocus], input, textarea, button.btn-primary')
    el?.focus()
  })

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onclose()
    }
  }
</script>

<div
  class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-[var(--overlay)] backdrop-blur-[3px] sm:px-4 sm:pt-[9vh] sm:pb-8"
  transition:fade={{ duration: 140 }}
  onmousedown={(e) => e.target === e.currentTarget && onclose()}
  {onkeydown}
  role="presentation"
>
  <div
    bind:this={panel}
    class="flex min-h-full w-full {width} flex-col border-line bg-surface shadow-card sm:min-h-0 sm:rounded-2xl sm:border"
    style="padding-top: var(--inset-top); padding-bottom: var(--inset-bottom)"
    transition:scale={{ start: 0.97, duration: 180, easing: cubicOut }}
    role="dialog"
    aria-modal="true"
    aria-label={title}
  >
    <header class="flex items-start justify-between gap-4 px-4 pt-5 pb-3 sm:px-5">
      <div>
        <h2 class="text-[15px] font-semibold tracking-[-0.01em] text-fg">{title}</h2>
        {#if subtitle}<p class="mt-0.5 text-[12.5px] text-fg-3">{subtitle}</p>{/if}
      </div>
      <button class="icon-btn -mt-1 -mr-2" onclick={onclose} aria-label="Close"><Icon name="x" /></button>
    </header>
    <div class="flex-1 px-4 pb-5 sm:px-5">
      {@render children()}
    </div>
    {#if footer}
      <footer class="sticky bottom-0 flex flex-wrap items-center justify-end gap-2 border-t border-line bg-surface px-4 py-3 sm:static sm:rounded-b-2xl sm:bg-surface-2/60 sm:px-5">
        {@render footer()}
      </footer>
    {/if}
  </div>
</div>
