<script>
  import Badge from "../components/Badge.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  import { app, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  let { st } = $props();
  const TONE = { proposed: "awaiting-approval", approved: "in-progress", "in-progress": "in-progress", done: "done", failed: "fail",
    rejected: "rejected", cancelled: "cancelled", planned: "planned" };
  const order = ["proposed", "approved", "in-progress", "planned", "done", "failed", "rejected", "cancelled"];
  const list = $derived(st.incident.actions.slice().sort((a, b) => order.indexOf(a.status) - order.indexOf(b.status)));
  const question = a => st.questions.find(q => q.status === "open" && q.effect?.type === "incident-action" && q.effect.item === a.id);
  let comment = $state({}), error = $state(""), busy = $state(false);
  async function answer(a, choice) {
    busy = true;
    const r = await post(projectURL(app.data.id, "answer"), { id: question(a).id, choice, comment: comment[a.id] || "" });
    busy = false;
    error = r.ok ? "" : (r.json.error || "Not answered.");
    await refresh();
  }
</script>

<h1>Actions</h1>
<p class="lede">What is done to contain, eradicate and recover, and who is told. An agent carries out nothing before you approve it here; follow-ups are work that outlives the case.</p>
<span class="err" role="alert">{error}</span>
{#if !list.length}<p class="muted">None proposed yet.</p>{/if}
<div class="stack">
  {#each list as a (a.id)}
    {@const q = question(a)}
    <section class="card">
      <div class="row"><span class="chip">{a.id}</span><Badge s={TONE[a.status] || "draft"} text={a.status === "proposed" ? "waits for you" : a.status} />
        <span class="chip">{a.kind}</span><strong>{a.title}</strong>
        <span class="small muted">by {a.executor === "agent" ? "the agent" : a.executor} · <Ago iso={a.done_at || a.proposed_at} /></span></div>
      {#if a.detail}<h3>What will be done</h3><Md text={a.detail} cls="wide" />{/if}
      {#if a.risk}<div class="small"><strong>Could break:</strong> {a.risk}</div>{/if}
      {#if a.rollback}<div class="small"><strong>To undo:</strong> {a.rollback}</div>{/if}
      {#if a.result}<div class="small" style="margin-top:6px"><strong>{a.status === "rejected" ? "Your reason" : a.status === "cancelled" ? "Cancelled" : "Result"}:</strong> {a.result}</div>{/if}
      {#if q}
        <div class="row" style="margin-top:8px">
          <input type="text" style="flex:1;min-width:200px" placeholder="Comment (why, or what to do instead)" bind:value={comment[a.id]} aria-label="Comment on {a.id}" />
          <button class="btn primary" disabled={busy} onclick={() => answer(a, "Approve")}>Approve</button>
          <button class="btn ghost" disabled={busy} onclick={() => answer(a, "Reject")}>Reject</button>
        </div>
      {/if}
    </section>
  {/each}
</div>
