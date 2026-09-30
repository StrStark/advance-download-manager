<script lang="ts">
  import { flip } from 'svelte/animate'
  import { fly, fade } from 'svelte/transition'
  import Icon from './Icon.svelte'
  import { store } from '../lib/store.svelte'
</script>

<div class="pointer-events-none fixed right-3 bottom-24 left-3 z-50 flex flex-col gap-2 sm:left-auto sm:w-[340px] md:right-4 md:bottom-12" aria-live="polite">
  {#each store.toasts as t (t.id)}
    <div
      class="pointer-events-auto flex items-start gap-3 rounded-xl border border-line bg-surface p-3 shadow-card"
      animate:flip={{ duration: 200 }}
      in:fly={{ y: 12, duration: 200 }}
      out:fade={{ duration: 150 }}
    >
      <div
        class="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-lg
          {t.kind === 'success' ? 'bg-ok-soft text-ok' : t.kind === 'error' ? 'bg-bad-soft text-bad' : 'bg-accent-soft text-accent-hi'}"
      >
        <Icon name={t.kind === 'success' ? 'check' : t.kind === 'error' ? 'alert' : 'info'} size={14} stroke={2.4} />
      </div>
      <div class="min-w-0 flex-1">
        <div class="text-[13px] font-medium text-fg">{t.title}</div>
        {#if t.body}<div class="mt-0.5 truncate text-[12px] text-fg-3" title={t.body}>{t.body}</div>{/if}
      </div>
      {#if t.action}
        <button
          class="btn btn-ghost h-7 px-2 text-[12px] text-accent-hi"
          onclick={() => {
            t.action!.run()
            store.dismiss(t.id)
          }}>{t.action.label}</button
        >
      {/if}
      <button class="icon-btn -mt-1 -mr-1 size-6" onclick={() => store.dismiss(t.id)} aria-label="Dismiss"><Icon name="x" size={13} /></button>
    </div>
  {/each}
</div>
