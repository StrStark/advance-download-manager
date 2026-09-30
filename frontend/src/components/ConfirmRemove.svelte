<script lang="ts">
  import Modal from './Modal.svelte'
  import { store } from '../lib/store.svelte'

  let { ids, label }: { ids: string[]; label: string } = $props()
  let deleteFiles = $state(false)
  const done = $derived(ids.filter((i) => store.jobs[i]?.status === 'completed').length)
</script>

<Modal title="Remove {label}?" onclose={() => (store.confirmRemove = null)} width="max-w-md">
  <p class="text-[13px] leading-relaxed text-fg-2">
    {ids.length === 1 ? 'It' : 'They'} will be removed from the list. Unfinished parts are always deleted.
  </p>
  {#if done > 0}
    <label class="mt-4 flex cursor-pointer items-center gap-2.5 rounded-xl border border-line bg-surface-2 px-3 py-2.5 text-[13px]">
      <input type="checkbox" bind:checked={deleteFiles} class="accent-[var(--bad)]" />
      <span>Also delete {done === 1 ? 'the downloaded file' : `${done} downloaded files`} from disk</span>
    </label>
  {/if}
  {#snippet footer()}
    <button class="btn btn-ghost" onclick={() => (store.confirmRemove = null)}>Cancel</button>
    <button class="btn btn-danger min-w-[100px]" onclick={() => store.remove(ids, deleteFiles)} autofocus>
      {deleteFiles ? 'Remove & delete' : 'Remove'}
    </button>
  {/snippet}
</Modal>
