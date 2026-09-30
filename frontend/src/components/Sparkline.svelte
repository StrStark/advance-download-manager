<script lang="ts">
  let { values, width = 120, height = 28 }: { values: number[]; width?: number; height?: number } = $props()

  const paths = $derived.by(() => {
    const max = Math.max(1, ...values) * 1.15
    const step = width / Math.max(1, values.length - 1)
    const pts = values.map((v, i) => [i * step, height - (v / max) * (height - 2) - 1])
    const line = pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)} ${y.toFixed(1)}`).join(' ')
    return { line, area: `${line} L${width} ${height} L0 ${height} Z` }
  })
</script>

<svg {width} {height} viewBox="0 0 {width} {height}" class="overflow-visible" aria-hidden="true">
  <defs>
    <linearGradient id="spark-fill" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="var(--accent-2)" stop-opacity="0.28" />
      <stop offset="1" stop-color="var(--accent-2)" stop-opacity="0" />
    </linearGradient>
  </defs>
  <path d={paths.area} fill="url(#spark-fill)" />
  <path d={paths.line} fill="none" stroke="var(--accent-2)" stroke-width="1.5" stroke-linejoin="round" />
</svg>
