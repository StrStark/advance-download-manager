<script lang="ts">
  import { icons, type IconName } from '../lib/icons'

  let { name, size = 16, stroke = 2, class: cls = '' }: { name: IconName; size?: number; stroke?: number; class?: string } =
    $props()
</script>

<svg
  xmlns="http://www.w3.org/2000/svg"
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={stroke}
  stroke-linecap="round"
  stroke-linejoin="round"
  class="shrink-0 {cls}"
  aria-hidden="true">{@html icons[name]}</svg
>
