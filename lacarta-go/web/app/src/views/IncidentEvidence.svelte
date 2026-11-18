<script>
  import Badge from "../components/Badge.svelte";
  import { defang, utc } from "../lib/format.js";
  let { st } = $props();
  const c = $derived(st.incident);
  let retired = $state(false);
  const indicators = $derived(c.indicators.filter(i => retired || i.status === "active"));
  const nRetired = $derived(c.indicators.filter(i => i.status !== "active").length);
  const size = n => n > 1048576 ? (n / 1048576).toFixed(1) + " MB" : n > 1024 ? Math.round(n / 1024) + " kB" : n + " B";
</script>

<h1>Evidence & indicators</h1>
<p class="lede">Evidence files are kept read-only in the case's <code>evidence/</code> folder with their sha256; <code>lacarta incident verify</code> checks them. Indicators are shown defanged.</p>
<h2>Evidence · {c.evidence.length}</h2>
{#if !c.evidence.length}<p class="muted">Nothing registered yet.</p>
{:else}
  <div class="table-wrap"><table class="data">
    <thead><tr><th>Ref</th><th>Evidence</th><th>From</th><th>Collected</th><th>File or location</th></tr></thead>
    <tbody>{#each c.evidence as e (e.id)}
      <tr><td class="id">{e.id}</td><td><span class="chip">{e.kind}</span> {e.title}{#if e.note}<div class="small muted">{e.note}</div>{/if}</td>
        <td>{e.source}</td><td class="id">{utc(e.collected_at)}<div class="small muted">{e.collected_by}</div></td>
        <td>{#if e.file}<code class="small">{e.file}</code><div class="small muted" title={e.sha256}>sha256 {e.sha256.slice(0, 16)}… · {size(e.size)}</div>{:else}<span class="small">{e.ref}</span>{/if}</td></tr>
    {/each}</tbody>
  </table></div>
{/if}
<h2>Indicators · {c.indicators.length - nRetired}</h2>
{#if nRetired}<label class="small"><input type="checkbox" bind:checked={retired} /> Show {nRetired} retired</label>{/if}
{#if !indicators.length}<p class="muted">None yet.</p>
{:else}
  <div class="table-wrap"><table class="data">
    <thead><tr><th>Ref</th><th>Type</th><th>Value</th><th>Context</th></tr></thead>
    <tbody>{#each indicators as i (i.id)}
      <tr class:muted={i.status !== "active"}><td class="id">{i.id}</td><td>{i.type}</td><td><code>{defang(i.type, i.value)}</code></td>
        <td>{i.context}{#each i.evidence as id (id)}{" "}<span class="chip">{id}</span>{/each}
          {#if i.status !== "active"}<div class="small"><Badge s="retired" /> {i.reason}</div>{/if}</td></tr>
    {/each}</tbody>
  </table></div>
{/if}
