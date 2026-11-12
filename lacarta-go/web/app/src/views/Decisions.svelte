<script>
  import ByBadge from "../components/ByBadge.svelte";
  import AnsweredTo from "../components/AnsweredTo.svelte";
  import Ago from "../components/Ago.svelte";
  import { phaseName } from "../lib/format.js";
  let { st } = $props();
  const list = $derived([...st.decisions].reverse());
</script>

<h1>Decisions</h1>
<p class="lede">Everything decided so far: your answers and the choices the agents made and logged.</p>
{#if list.length}
  <div class="table-wrap"><table class="data"><thead><tr><th>ID</th><th>Decision</th><th>By</th><th>Applies to</th><th>When</th></tr></thead><tbody>
    {#each list as x (x.id)}
      {@const applies = [x.spec ? `spec: ${x.spec}` : "", x.phase != null ? phaseName(x.phase) : "", x.source || ""].filter(Boolean)}
      <tr><td class="id">{x.id}</td><td><strong>{x.title}</strong><div>{x.decision}</div>{#if x.rationale}<div class="small muted">{x.rationale}</div>{/if}</td>
        <td><ByBadge by={x.by} agent={x.agent} /><div style="margin-top:4px"><AnsweredTo {x} /></div></td>
        <td class="small">{#each applies as t, i (i)}{#if i}<br />{/if}{t}{:else}<span class="muted">Project</span>{/each}</td>
        <td class="small muted"><Ago iso={x.at} /></td></tr>
    {/each}
  </tbody></table></div>
{:else}
  <div class="card empty">No decisions yet. Your answers to questions will be recorded here.</div>
{/if}
