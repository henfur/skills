<script>
  import Badge from "../components/Badge.svelte";
  import AgentChip from "../components/AgentChip.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  let { st } = $props();
  const list = $derived(st.incident.investigations.slice().sort((a, b) => (a.status === "open" ? 0 : 1) - (b.status === "open" ? 0 : 1)));
</script>

<h1>Investigations</h1>
<p class="lede">One question each. A confirmed or refuted conclusion rests on registered evidence, and a confirmed one is challenged by a fresh agent before the final report.</p>
{#if !list.length}<p class="muted">None opened yet.</p>{/if}
<div class="stack">
  {#each list as v (v.id)}
    <section class="card">
      <div class="row"><span class="chip">{v.id}</span><Badge s={v.status} /><strong>{v.title}</strong>
        <span class="small muted">updated <Ago iso={v.updated_at} /></span></div>
      {#if v.hypothesis}<div class="small muted">Hypothesis: {v.hypothesis}</div>{/if}
      {#if v.status !== "open"}
        <div style="margin-top:8px"><Md text={v.conclusion} cls="wide" /></div>
        <div class="row small">{#if v.concluded_by}<span class="muted">concluded by</span><AgentChip id={v.concluded_by} withRole={false} />{/if}
          {#each v.evidence as id (id)}<span class="chip">{id}</span>{/each}</div>
        {#if v.challenge}<div class="small" style="margin-top:6px"><Badge s={v.challenge.verdict === "holds" ? "approved" : "fail"} text={"Challenge: " + v.challenge.verdict} /> {v.challenge.note}</div>
        {:else if v.status === "confirmed"}<div class="small" style="margin-top:6px"><Badge s="open" text="Not challenged yet" /></div>{/if}
      {/if}
      {#if v.notes.length}
        <details style="margin-top:8px" open={v.status === "open"}><summary class="small">{v.notes.length} note{v.notes.length === 1 ? "" : "s"}</summary>
          <div class="list">{#each v.notes as n, i (i)}<div><div class="row small muted">{#if n.by === "user"}you{:else}<AgentChip id={n.by} withRole={false} />{/if}<Ago iso={n.at} />
            {#each n.evidence as id (id)}<span class="chip">{id}</span>{/each}</div><Md text={n.text} cls="wide" /></div>{/each}</div>
        </details>
      {/if}
    </section>
  {/each}
</div>
