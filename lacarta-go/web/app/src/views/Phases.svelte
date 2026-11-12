<script>
  import Badge from "../components/Badge.svelte";
  import BlockedBanner from "../components/BlockedBanner.svelte";
  import PhaseBlock from "../components/PhaseBlock.svelte";
  import Md from "../components/Md.svelte";
  import { SPEC_REVIEW_TEXT, label } from "../lib/format.js";
  let { st, d } = $props();
  const showFinal = $derived(st.stage === "final-review" || st.stage === "done" || st.reviews.some(r => r.phase === "final"));
  const showSpec = $derived(st.stage === "specification" || st.reviews.some(r => r.phase === "spec"));
</script>

<h1>Phases & reviews</h1>
<p class="lede">Each phase ends with a functional review, a security review and your approval. Plan: <Badge s={st.plan.status} text={label(st.plan.status)} /></p>
<BlockedBanner {d} />
<div class="stack">
  {#if showSpec}
    <PhaseBlock {st} pid="spec">
      <div class="row"><span class="overline">Before implementation</span><Badge s={d.spec_review} text={SPEC_REVIEW_TEXT[d.spec_review][0]} /></div><h3>Independent specification review</h3>
      <p class="small" style="margin:0">{SPEC_REVIEW_TEXT[d.spec_review][1]}</p>
      <p class="small muted" style="margin:0">Run by fresh agents that saw only the specs, the plan and the decision log.</p>
    </PhaseBlock>
  {/if}
  {#each st.phases as p (p.id)}
    <PhaseBlock {st} pid={p.id}>
      <div class="row"><span class="overline">Phase {p.id}</span><Badge s={p.status} text={label(p.status)} /></div><h3>{p.title}</h3>
      <Md text={p.goal} />
      {#if p.scope.length}<ul class="small" style="margin:0;padding-left:20px">{#each p.scope as s, i (i)}<li><Md text={s} inline /></li>{/each}</ul>{/if}
      {#if p.covers.length}<div class="row">{#each p.covers as c (c)}<span class="chip">{c}</span>{/each}</div>{/if}
    </PhaseBlock>
  {:else}
    <div class="card empty">No phases planned yet. The agent proposes the plan once the specs are written.</div>
  {/each}
  {#if showFinal}
    <PhaseBlock {st} pid="final">
      <div class="row"><span class="overline">Final</span><Badge s={st.stage === "done" ? "done" : "in-progress"} text={st.stage === "done" ? "Done" : "In progress"} /></div><h3>Independent final review</h3>
      <p class="small muted" style="margin:0">Run by fresh agents that saw only the repository.</p>
    </PhaseBlock>
  {/if}
</div>
