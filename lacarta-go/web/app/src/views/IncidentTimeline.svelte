<script>
  import Badge from "../components/Badge.svelte";
  import AgentChip from "../components/AgentChip.svelte";
  import { utc } from "../lib/format.js";
  let { st, d } = $props();
  let withdrawn = $state(false);
  const labels = $derived(Object.fromEntries(d.incident.marks.map(m => [m.mark, m.label])));
  const events = $derived(st.incident.timeline.filter(e => withdrawn || !e.retracted).slice().sort((a, b) => a.at.localeCompare(b.at)));
  const hidden = $derived(st.incident.timeline.filter(e => e.retracted).length);
</script>

<h1>Timeline</h1>
<p class="lede">What happened, in the order it happened (UTC). An event is confirmed when it rests on evidence or a named source; the rest is marked suspected.</p>
{#if hidden}<label class="small"><input type="checkbox" bind:checked={withdrawn} /> Show {hidden} withdrawn event{hidden === 1 ? "" : "s"}</label>{/if}
{#if !events.length}<p class="muted">Nothing recorded yet.</p>
{:else}
  <div class="table-wrap"><table class="data">
    <thead><tr><th>When</th><th>What</th><th>Basis</th><th>Recorded by</th></tr></thead>
    <tbody>
      {#each events as e (e.id)}
        <tr class:muted={!!e.retracted}>
          <td class="id">{utc(e.at)}</td>
          <td>{#if e.mark}<Badge s="approved" text={labels[e.mark] || e.mark} />{" "}{/if}{#if e.confidence === "suspected"}<Badge s="suspected" />{" "}{/if}
            <span style={e.retracted ? "text-decoration:line-through" : ""}>{e.text}</span>
            {#if e.retracted}<div class="small">Withdrawn: {e.retracted}</div>{/if}</td>
          <td>{#each e.evidence as id (id)}<span class="chip">{id}</span>{" "}{/each}<span class="small muted">{e.source}</span></td>
          <td>{#if e.by === "user"}<span class="small muted">you</span>{:else}<AgentChip id={e.by} withRole={false} />{/if} <span class="chip">{e.id}</span></td>
        </tr>
      {/each}
    </tbody>
  </table></div>
{/if}
