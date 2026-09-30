<script lang="ts">
  import Icon from './Icon.svelte'
  import { categoryIcon } from '../lib/icons'

  let { category, size = 36 }: { category: string; size?: number } = $props()

  const tones: Record<string, string> = {
    video: 'bg-[#8b5cf6]/15 text-[#a78bfa]',
    music: 'bg-[#ec4899]/15 text-[#f472b6]',
    archives: 'bg-[#f59e0b]/15 text-[#fbbf24]',
    documents: 'bg-[#3b82f6]/15 text-[#60a5fa]',
    images: 'bg-[#10b981]/15 text-[#34d399]',
    programs: 'bg-[#06b6d4]/15 text-[#22d3ee]',
    other: 'bg-surface-3 text-fg-2',
  }
</script>

<div
  class="flex shrink-0 items-center justify-center rounded-[10px] {tones[category] ?? tones.other}"
  style="width:{size}px;height:{size}px"
>
  <Icon name={categoryIcon[category] ?? 'file'} size={Math.round(size * 0.47)} />
</div>
