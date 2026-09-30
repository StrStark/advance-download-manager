<script lang="ts">
  let { checked = $bindable(false), label, hint }: { checked?: boolean; label: string; hint?: string } = $props()
</script>

<label class="flex cursor-pointer items-center justify-between gap-4 py-1.5">
  <span class="min-w-0">
    <span class="block text-[13px] text-fg">{label}</span>
    {#if hint}<span class="block text-[12px] text-fg-3">{hint}</span>{/if}
  </span>
  <span class="relative inline-flex shrink-0">
    <input type="checkbox" class="peer sr-only" bind:checked />
    <span
      class="h-5 w-9 rounded-full bg-surface-3 ring-1 ring-line transition-colors duration-150 peer-checked:bg-accent peer-checked:ring-accent peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent"
    ></span>
    <span
      class="absolute top-0.5 left-0.5 size-4 rounded-full bg-white shadow transition-transform duration-150 peer-checked:translate-x-4"
    ></span>
  </span>
</label>
