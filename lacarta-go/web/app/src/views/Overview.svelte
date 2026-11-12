<script>
  import BlockedBanner from "../components/BlockedBanner.svelte";
  import SessionBanner from "../components/SessionBanner.svelte";
  import AgentsSection from "../components/AgentsSection.svelte";
  import ActivityRows from "../components/ActivityRows.svelte";
  import AgentChip from "../components/AgentChip.svelte";
  import KindBadge from "../components/KindBadge.svelte";
  import SevBadge from "../components/SevBadge.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  import Icon from "../components/Icon.svelte";
  import { L, askerId } from "../lib/state.svelte.js";
  import { STAGES, SEV_ORDER, SPEC_STEP_WORD, label } from "../lib/format.js";
  let { st, d } = $props();
  const stageIdx = $derived(STAGES.findIndex(s => s[0] === st.stage));
  const steps = $derived(STAGES.map(([k, name], i) => {
    const cls = st.stage === "done" || i < stageIdx ? "done" : i === stageIdx ? "current" : "";
    const current = d.blocked ? "Blocked" : k === "specification" ? SPEC_STEP_WORD[d.spec_review] : "Current";
    return { cls, name, word: cls === "done" ? "Done" : cls === "current" ? current : "Upcoming" };
  }));
  const sevs = $derived(SEV_ORDER.filter(s => d.open_findings[s] > 0));
  const totalOpen = $derived(SEV_ORDER.reduce((a, s) => a + d.open_findings[s], 0));
  const openQs = $derived(st.questions.filter(q => q.status === "open"));
</script>

<h1>Overview</h1>
<p class="lede">{st.project.summary || "Project status, open questions and everything decided so far."}</p>
<BlockedBanner {d} />
<SessionBanner {st} {d} />
<div class="steps" aria-label="Stages">
  {#each steps as s, i (i)}<div class="step {s.cls}"><div class="overline">Step {i + 1}</div><div class="label">{s.name}</div><div class="state">{s.word}</div></div>{/each}
</div>
<h2>Status</h2>
<div class="grid">
  <a class="card stat" href={L("specs")} style="text-decoration:none;color:inherit"><div class="overline">Specs approved</div><div class="value">{d.specs_approved}<small> / 4</small></div></a>
  <a class="card stat" href={L("phases")} style="text-decoration:none;color:inherit"><div class="overline">Phases done</div><div class="value">{d.phases_done}<small> / {st.phases.length}</small></div><div class="small muted">Plan {label(st.plan.status).toLowerCase()}</div></a>
  <a class="card stat" href={L("questions")} style="text-decoration:none;color:inherit"><div class="overline">Waiting for you</div><div class="value">{d.open_questions}<small> {d.open_questions === 1 ? "question" : "questions"}</small></div></a>
  <a class="card stat" href={L("findings")} style="text-decoration:none;color:inherit"><div class="overline">Unresolved findings</div><div class="value">{totalOpen}</div>
    <div class="sev-row">{#each sevs as s (s)}<SevBadge {s} /><span class="small">{d.open_findings[s]}</span>{" "}{:else}<span class="small muted">None open</span>{/each}</div></a>
</div>
<h2>Next step</h2>
<div class="banner info" style="margin:0"><Icon name="info" /><div><Md text={d.next} inline /></div></div>
<AgentsSection {st} {d} />
{#if openQs.length}
  <h2>Waiting for your answer · {openQs.length}</h2>
  <div class="card list">{#each openQs as q (q.id)}<div class="row"><AgentChip id={askerId(q)} withRole={false} /><span class="chip">{q.id}</span><KindBadge {q} /><a href={L("questions")}>{q.title}</a><span class="small muted"><Ago iso={q.asked_at} /></span></div>{/each}</div>
{/if}
<h2>Recent activity</h2>
<div class="card list"><ActivityRows items={st.activity.slice(-8).reverse()} /></div>
