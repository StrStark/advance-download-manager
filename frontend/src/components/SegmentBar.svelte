<script lang="ts">
  import type { Segment, Status } from '../lib/types'
  import { routeColors } from '../lib/icons'

  let {
    segments,
    size,
    downloaded,
    status,
    tall = false,
    routes = [],
  }: { segments: Segment[] | null; size: number; downloaded: number; status: Status; tall?: boolean; routes?: string[] } = $props()

  // With several connections, each segment takes its connection's color.
  const routeIndex = $derived.by(() => {
    const order = [...routes]
    for (const s of segments ?? []) if (s.path && !order.includes(s.path)) order.push(s.path)
    return order.length > 1 ? new Map(order.map((id, i) => [id, i])) : null
  })
  const colorFor = (s: Segment, i: number) =>
    routeIndex && s.path !== undefined && routeIndex.has(s.path)
      ? routeColors[routeIndex.get(s.path)! % routeColors.length]
      : i % 2
        ? 'var(--seg-b)'
        : 'var(--seg-a)'

  const tone = $derived(
    status === 'failed' ? 'var(--bad)' : status === 'paused' ? 'var(--warn)' : status === 'completed' ? 'var(--ok)' : null,
  )
  const segs = $derived(size > 0 && segments?.length ? [...segments].sort((a, b) => a.start - b.start) : [])
  const indeterminate = $derived(size <= 0 && (status === 'downloading' || status === 'probing'))
</script>

<div
  class="relative w-full overflow-hidden rounded-full bg-track {tall ? 'h-2.5' : 'h-1.5'}"
  role="progressbar"
  aria-valuemin={0}
  aria-valuemax={100}
  aria-valuenow={size > 0 ? Math.round((downloaded / size) * 100) : undefined}
>
  {#if indeterminate}
    <div class="indeterminate absolute inset-y-0 w-1/3 rounded-full bg-accent"></div>
  {:else if segs.length}
    {#each segs as s, i (s.start)}
      {@const left = (s.start / size) * 100}
      {@const w = (Math.min(s.done, s.end - s.start + 1) / size) * 100}
      <div
        class="absolute inset-y-0 transition-[width] duration-300 ease-out"
        style="left:{left}%; width:{w}%; background:{tone ?? colorFor(s, i)}; opacity:{tone ? 0.9 : 1}"
      ></div>
      {#if i > 0 && tall}
        <div class="absolute inset-y-0 w-px bg-bg/70" style="left:{left}%"></div>
      {/if}
    {/each}
  {:else if size > 0}
    <div
      class="absolute inset-y-0 left-0 rounded-full transition-[width] duration-300 ease-out"
      style="width:{Math.min(100, (downloaded / size) * 100)}%; background:{tone ?? 'var(--accent)'}"
    ></div>
  {/if}
</div>

<style>
  .indeterminate {
    animation: slide 1.3s ease-in-out infinite;
  }
  @keyframes slide {
    from {
      left: -33%;
    }
    to {
      left: 100%;
    }
  }
</style>
