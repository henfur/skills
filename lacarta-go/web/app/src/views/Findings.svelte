<script>
  import Badge from "../components/Badge.svelte";
  import SevBadge from "../components/SevBadge.svelte";
  import BlockedBanner from "../components/BlockedBanner.svelte";
  import AgentChip from "../components/AgentChip.svelte";
  import Md from "../components/Md.svelte";
  import { ui } from "../lib/state.svelte.js";
  import { SEV_ORDER, UNRESOLVED, label, phaseName } from "../lib/format.js";
  let { st, d } = $props();
  const list = $derived(st.findings.filter(f => ui.findingFilter === "all" || UNRESOLVED.has(f.status))
    .sort((a, b) => SEV_ORDER.indexOf(a.severity) - SEV_ORDER.indexOf(b.severity)));
</script>

<h1>Findings</h1>
<p class="lede">From functional and security reviews. High and critical stop development until you decide.</p>
<BlockedBanner {d} />
<div class="tabs" role="tablist">
  <button type="button" role="tab" aria-selected={ui.findingFilter === "unresolved"} onclick={() => { ui.findingFilter = "unresolved"; }}>Unresolved</button>
  <button type="button" role="tab" aria-selected={ui.findingFilter === "all"} onclick={() => { ui.findingFilter = "all"; }}>All · {st.findings.length}</button>
</div>
{#if list.length}
  <div class="table-wrap"><table class="data"><thead><tr><th>ID</th><th>Severity</th><th>Finding</th><th>Where</th><th>Status</th></tr></thead><tbody>
    {#each list as f (f.id)}
      <tr><td class="id">{f.id}</td><td><SevBadge s={f.severity} /></td>
        <td><strong>{f.title}</strong>{#if f.detail}<details><summary class="small">Details</summary><Md text={f.detail} cls="wide" /></details>{/if}{#if f.resolution}<div class="small muted">{f.resolution}</div>{/if}</td>
        <td class="small">{phaseName(f.phase)} · {f.review}{#if f.location}<br /><code>{f.location}</code>{/if}{#if f.found_by}<div style="margin-top:4px"><AgentChip id={f.found_by} withRole={false} /></div>{/if}</td>
        <td><Badge s={f.status} text={label(f.status)} /></td></tr>
    {/each}
  </tbody></table></div>
{:else}
  <div class="card empty">{ui.findingFilter === "all" ? "No findings recorded yet." : "No unresolved findings."}</div>
{/if}
