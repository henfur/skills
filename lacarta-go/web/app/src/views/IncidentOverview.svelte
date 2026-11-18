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
  import { L, askerId } from "../lib/state.svelte.js";
  import { utc } from "../lib/format.js";
  let { st, d } = $props();
  const c = $derived(st.incident);
  const k = $derived(d.incident);
  const PHASES = ["contain", "eradicate", "recover", "review"];
  const NAMES = { incident: ["Identify & contain", "Eradicate", "Recover", "Post-incident review"],
    vulnerability: ["Assess & mitigate", "Remediate", "Verify the fix", "Review"] };
  const idx = $derived(PHASES.indexOf(k.phase));
  const steps = $derived(PHASES.map((p, i) => {
    const cls = st.stage === "closed" || i < idx ? "done" : i === idx ? "current" : "";
    return { cls, name: (NAMES[c.type] || NAMES.incident)[i], word: cls === "done" ? "Done" : cls === "current" ? "Current" : "Upcoming" };
  }));
  const openQs = $derived(st.questions.filter(q => q.status === "open"));
  const marks = $derived(k.marks.filter(m => m.at));
  const sit = $derived(c.narrative.situation);
</script>

<h1>Overview</h1>
<p class="lede"><SevBadge s={c.severity} /> {c.type === "vulnerability" ? "Vulnerability response" : "Incident"} · TLP:{c.tlp}
  {#if c.refs.length} · {c.refs.join(", ")}{/if}{#if c.affected.length} · affects {c.affected.join(", ")}{/if}</p>
<SessionBanner {st} {d} />
{#if st.stage === "closed"}<div class="banner info"><Icon name="info" /><div><strong>Closed: {c.outcome}.</strong> The final report is in <a href={L("reports")}>Reports</a>.</div></div>{/if}
{#if k.sitrep_due}<div class="banner warn"><Icon name="warn" /><div>A situation report is due (every {c.sitrep_every}).</div></div>{/if}
<div class="steps" aria-label="Phases">
  {#each steps as s, i (i)}<div class="step {s.cls}"><div class="overline">Phase {i + 1}</div><div class="label">{s.name}</div><div class="state">{s.word}</div></div>{/each}
</div>
<h2>Status</h2>
<div class="grid">
  <a class="card stat" href={L("actions")} style="text-decoration:none;color:inherit"><div class="overline">Actions waiting for you</div>
    <div class="value">{k.waiting.length}</div><div class="small muted">{k.approved.length ? `${k.approved.length} approved, under way` : "nothing under way"}</div></a>
  <a class="card stat" href={L("investigations")} style="text-decoration:none;color:inherit"><div class="overline">Investigations</div>
    <div class="value">{k.investigating.open || 0}<small> open / {k.counts.investigations}</small></div>
    <div class="small muted">{k.unchallenged ? `${k.unchallenged} confirmed, not challenged yet` : `${k.investigating.confirmed || 0} confirmed`}</div></a>
  <a class="card stat" href={L("timeline")} style="text-decoration:none;color:inherit"><div class="overline">Timeline</div>
    <div class="value">{k.counts.timeline}<small> events</small></div><div class="small muted">{k.counts.evidence} evidence · {k.counts.indicators} indicators</div></a>
  <a class="card stat" href={L("reports")} style="text-decoration:none;color:inherit"><div class="overline">Reports issued</div>
    <div class="value">{k.counts.reports}</div>
    <div class="small muted">{#if k.draft}a draft waits for you{:else if k.last_report}last <Ago iso={k.last_report.issued_at} />, {k.changes} records since{:else}none yet{/if}</div></a>
</div>
<h2>Situation</h2>
{#if sit}<div class="card"><Md text={sit.text} cls="wide" /><div class="small muted">Written <Ago iso={sit.at} /></div></div>
{:else}<p class="muted">Not written yet.</p>{/if}
{#if marks.length}
  <h2>Key times</h2>
  <div class="card list">
    {#each marks as m (m.mark)}<div class="row"><span class="overline">{m.label}</span><span>{utc(m.at)}</span></div>{/each}
    {#each k.times as t (t.k)}<div class="row"><span class="overline">{t.k}</span><strong>{t.v}</strong></div>{/each}
  </div>
{/if}
<h2>Next step</h2>
<div class="banner info" style="margin:0"><Icon name="info" /><div><Md text={d.next} inline /></div></div>
<AgentsSection {st} {d} />
{#if openQs.length}
  <h2>Waiting for your answer · {openQs.length}</h2>
  <div class="card list">{#each openQs as q (q.id)}<div class="row"><AgentChip id={askerId(q)} withRole={false} /><span class="chip">{q.id}</span><KindBadge {q} /><a href={L("questions")}>{q.title}</a><span class="small muted"><Ago iso={q.asked_at} /></span></div>{/each}</div>
{/if}
<h2>Recent activity</h2>
<div class="card list"><ActivityRows items={st.activity.slice(-8).reverse()} /></div>
