<script>
  import Badge from "../components/Badge.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  import { app, ui } from "../lib/state.svelte.js";
  import { SPEC_ORDER, label } from "../lib/format.js";
  let { st } = $props();
  const tab = $derived(SPEC_ORDER.includes(ui.specTab) ? ui.specTab : "functional");
  const s = $derived(st.specs[tab]);
  const decs = $derived(st.decisions.filter(x => x.spec === tab).length);
  const tone = sp => sp.status === "approved" ? "success" : sp.status === "in-review" ? "info" : sp.status === "changes-requested" ? "danger" : sp.status === "changed" ? "warning" : "";
</script>

<!-- Documentation of the specs as they stand. Reviews live under Phases & reviews, decisions under Decisions. -->
<h1>Specifications</h1>
<p class="lede">The specs as they stand today: the source of truth for the project.</p>
<div class="tabs" role="tablist">
  {#each SPEC_ORDER as k (k)}
    <button type="button" role="tab" aria-selected={k === tab} onclick={() => { ui.specTab = k; }}><span class="dot {tone(st.specs[k])}" aria-hidden="true"></span>{st.specs[k].title}</button>
  {/each}
</div>
<article class="card">
  <div class="row" style="margin-bottom:16px"><Badge s={s.status} text={label(s.status)} /><span class="small muted">Revision {s.revision}{#if s.approved_at} · approved <Ago iso={s.approved_at} />{/if}{#if decs} · shaped by {decs} {decs === 1 ? "decision" : "decisions"} (<a href="#/p/{app.data.id}/decisions">Decisions</a>){/if}</span><span class="chip" style="margin-left:auto">.lacarta/specs/{tab}.md</span></div>
  <Md text={app.data.specs[tab]} cls="wide" />
</article>
