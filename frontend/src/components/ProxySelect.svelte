<script lang="ts">
  import { store } from '../lib/store.svelte'

  /** "" = the default from the Network panel. */
  let { value = $bindable(''), id }: { value?: string; id?: string } = $props()

  const groups = $derived.by(() => {
    const ps = store.settings.proxies ?? []
    const subs = store.settings.subscriptions ?? []
    const out: { title: string; items: typeof ps }[] = []
    const own = ps.filter((p) => !p.subscriptionId || !subs.some((s) => s.id === p.subscriptionId))
    if (own.length) out.push({ title: 'Your servers', items: own })
    for (const s of subs) out.push({ title: s.name, items: ps.filter((p) => p.subscriptionId === s.id) })
    return out
  })
</script>

<select {id} class="field" bind:value>
  <option value="">Default ({store.proxyName('')})</option>
  <option value="direct">Direct (no proxy)</option>
  <option value="system">System proxy</option>
  {#each groups as g (g.title)}
    <optgroup label={g.title}>
      {#each g.items as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
    </optgroup>
  {/each}
</select>
