<script>
  import SessionBanner from "../components/SessionBanner.svelte";
  import AgentsSection from "../components/AgentsSection.svelte";
  import ActivityRows from "../components/ActivityRows.svelte";
  import AgentChip from "../components/AgentChip.svelte";
  import KindBadge from "../components/KindBadge.svelte";
  import SevBadge from "../components/SevBadge.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  import Icon from "../components/Icon.svelte";
  import { app, L, askerId } from "../lib/state.svelte.js";
  import { AUDIT_STAGES, AUDIT_SEV } from "../lib/format.js";
  let { st, d } = $props();
  const a = $derived(d.audit);
  const stageIdx = $derived(AUDIT_STAGES.findIndex(s => s[0] === st.stage));
  const steps = $derived(AUDIT_STAGES.map(([k, name], i) => {
    const cls = st.stage === "done" || i < stageIdx ? "done" : i === stageIdx ? "current" : "";
    return { cls, name, word: cls === "done" ? "Done" : cls === "current" ? "Current" : "Upcoming" };
  }));
  const covered = $derived(a.units_total - (a.units.planned || 0) - (a.units.in_progress || 0));
  const confirmed = $derived(AUDIT_SEV.reduce((n, s) => n + (a.final[s] || 0), 0));
  const openQs = $derived(st.questions.filter(q => q.status === "open"));
  const skill = $derived((app.data.audit?.plugins || []).find(p => p.id === "security-audit"));
</script>

<h1>Overview</h1>
<p class="lede">Security audit of <code>{st.audit.target}</code>{#if a.run} · {a.run.id}, {a.run.profile} profile{/if}</p>
<SessionBanner {st} {d} />
{#if a.files_error}<div class="banner danger"><Icon name="alert" /><div>The run files can't be read: {a.files_error}</div></div>{/if}
<div class="steps audit-steps" aria-label="Stages">
  {#each steps as s, i (i)}<div class="step {s.cls}"><div class="overline">Step {i + 1}</div><div class="label">{s.name}</div><div class="state">{s.word}</div></div>{/each}
</div>
<h2>Status</h2>
<div class="grid">
  <a class="card stat" href={L("coverage")} style="text-decoration:none;color:inherit"><div class="overline">Coverage</div>
    <div class="value">{covered}<small> / {a.units_total} units</small></div><div class="small muted">{a.has_ledger ? "closed units" : "No ledger yet"}</div></a>
  <a class="card stat" href={L("findings")} style="text-decoration:none;color:inherit"><div class="overline">Confirmed findings</div>
    <div class="value">{a.has_findings ? confirmed : "–"}</div>
    <div class="sev-row">{#each AUDIT_SEV.filter(s => a.final[s]) as s (s)}<SevBadge {s} /><span class="small">{a.final[s]}</span>{" "}{/each}</div></a>
  <a class="card stat" href={L("findings")} style="text-decoration:none;color:inherit"><div class="overline">For you to review</div>
    <div class="value">{a.to_review}<small> {a.to_review === 1 ? "finding" : "findings"}</small></div>{#if a.recheck}<div class="small muted">{a.recheck} sent back for another check</div>{/if}</a>
  <div class="card stat"><div class="overline">Agent budget</div><div class="value">{a.agents_total}<small>{a.budget ? ` / ${a.budget}` : ""} agents</small></div>
    <div class="small muted">{a.budget ? "invocations used this run" : "No budget set"}</div></div>
</div>
<h2>Next step</h2>
<div class="banner info" style="margin:0"><Icon name="info" /><div><Md text={d.next} inline /></div></div>
{#if skill}<p class="small muted">Method: {skill.name}{#if skill.tested === false}, <strong>not the tested version</strong>{/if} · Execution: {st.audit.execution} · <a href={L("plugins")}>Plugins</a></p>{/if}
<AgentsSection {st} {d} />
{#if openQs.length}
  <h2>Waiting for your answer · {openQs.length}</h2>
  <div class="card list">{#each openQs as q (q.id)}<div class="row"><AgentChip id={askerId(q)} withRole={false} /><span class="chip">{q.id}</span><KindBadge {q} /><a href={L("questions")}>{q.title}</a><span class="small muted"><Ago iso={q.asked_at} /></span></div>{/each}</div>
{/if}
<h2>Recent activity</h2>
<div class="card list"><ActivityRows items={st.activity.slice(-8).reverse()} /></div>
