<script lang="ts">
  import type { Status } from '../lib/types'

  let { status }: { status: Status } = $props()
  const map: Record<Status, [string, string]> = {
    downloading: ['Downloading', 'bg-accent-soft text-accent-hi'],
    probing: ['Connecting', 'bg-accent-soft text-accent-hi'],
    queued: ['Queued', 'bg-surface-3 text-fg-2'],
    paused: ['Paused', 'bg-warn-soft text-warn'],
    completed: ['Completed', 'bg-ok-soft text-ok'],
    failed: ['Failed', 'bg-bad-soft text-bad'],
  }
</script>

<span class="inline-flex h-5 items-center rounded-md px-1.5 text-[11px] font-semibold {map[status][1]}">{map[status][0]}</span>
