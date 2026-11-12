<script>
  import QuestionCard from "../components/QuestionCard.svelte";
  import BlockedBanner from "../components/BlockedBanner.svelte";
  import AgentChip from "../components/AgentChip.svelte";
  import KindBadge from "../components/KindBadge.svelte";
  import Ago from "../components/Ago.svelte";
  import { askerId } from "../lib/state.svelte.js";
  let { st, d } = $props();
  const open = $derived(st.questions.filter(q => q.status === "open"));
  const ordered = $derived([...open.filter(q => q.effect?.type === "finding-triage"), ...open.filter(q => q.effect?.type !== "finding-triage")]);
  const answered = $derived(st.questions.filter(q => q.status === "answered").reverse());
</script>

<h1>Questions</h1>
<p class="lede">The agents stop and wait here for your answers. Every answer is recorded under Decisions.</p>
<BlockedBanner {d} />
<div class="stack">
  {#each ordered as q (q.id)}<QuestionCard {q} />{:else}<div class="card empty">No questions waiting. The agents will ask here when they need you.</div>{/each}
</div>
{#if answered.length}
  <h2>Answered · {answered.length}</h2>
  <div class="card list">
    {#each answered as q (q.id)}
      <div><div class="row"><AgentChip id={askerId(q)} withRole={false} /><span class="chip">{q.id}</span><KindBadge {q} /><strong>{q.title}</strong></div>
        <div class="small" style="margin-top:4px"><span class="muted">Answer:</span> {q.answer.choice || q.answer.comment}{#if q.answer.choice && q.answer.comment} · <span class="muted">{q.answer.comment}</span>{/if} <span class="muted">· <Ago iso={q.answer.at} /></span></div></div>
    {/each}
  </div>
{/if}
