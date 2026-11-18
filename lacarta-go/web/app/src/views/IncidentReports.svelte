<script>
  import Badge from "../components/Badge.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  import Icon from "../components/Icon.svelte";
  import { app, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  let { st, d } = $props();
  const c = $derived(st.incident);
  const reports = $derived(c.reports.filter(r => r.status !== "superseded").slice().reverse());
  const texts = $derived(app.data.case?.reports || {});
  const question = r => st.questions.find(q => q.status === "open" && q.effect?.type === "incident-report" && q.effect.item === r.id);
  const name = r => r.kind === "final" ? "Final report" : `Situation report ${r.number}`;
  let open = $state(""), editing = $state(""), text = $state(""), comment = $state(""), error = $state(""), busy = $state(false);
  async function act(action, body) {
    busy = true;
    const r = await post(projectURL(app.data.id, action), body);
    busy = false;
    error = r.ok ? "" : (r.json.error || "Not done.");
    await refresh();
    return r.ok;
  }
  async function save(r) {
    if (await act("incident-report-edit", { id: r.id, text })) editing = "";
  }
  const sections = $derived(Object.entries(d.incident.section_titles).filter(([k]) => c.narrative[k]));
</script>

<h1>Reports</h1>
<p class="lede">Situation reports along the way, then the final report. Each is drafted from the case record into <code>reports/</code>; nothing is issued until you approve it, and you can edit a draft first.</p>
<span class="err" role="alert">{error}</span>
{#if !reports.length}<p class="muted">No report drafted yet.</p>{/if}
<div class="stack">
  {#each reports as r (r.id)}
    {@const q = question(r)}
    {@const t = texts[r.id]}
    <section class="card">
      <div class="row">
        <button class="desk-key" onclick={() => open = open === r.id ? "" : r.id} aria-expanded={open === r.id || !!q}><strong>{name(r)}</strong></button>
        <Badge s={r.status === "in-review" ? "awaiting-approval" : r.status} text={r.status === "in-review" ? "waits for you" : r.status} />
        <code class="small">{r.file}</code><span class="small muted"><Ago iso={r.issued_at || r.at} />{#if r.edited} · edited by you{/if}</span>
      </div>
      {#if t && !t.ok}<div class="banner danger"><Icon name="alert" /><div>This file is not what {r.status === "issued" ? "you issued" : "lacarta drafted"}: it changed outside lacarta.</div></div>{/if}
      {#if open === r.id || q}
        {#if editing === r.id}
          <textarea rows="24" style="width:100%;font-family:var(--mono);font-size:13px" bind:value={text} aria-label="Text of {name(r)}"></textarea>
          <div class="row"><button class="btn" disabled={busy} onclick={() => save(r)}>Save</button><button class="btn ghost" onclick={() => editing = ""}>Cancel</button></div>
        {:else}
          <div class="desk-draft"><Md text={t?.text || "_The file can't be read._"} cls="wide" /></div>
        {/if}
        {#if q && editing !== r.id}
          <div class="row" style="margin-top:8px">
            <input type="text" style="flex:1;min-width:200px" placeholder="What to change (for Request changes)" bind:value={comment} aria-label="Comment on {name(r)}" />
            <button class="btn primary" disabled={busy || (t && !t.ok)} onclick={() => act("answer", { id: q.id, choice: "Approve", comment })}>{r.kind === "final" ? "Issue and close the case" : "Issue"}</button>
            <button class="btn ghost" disabled={busy} onclick={() => act("answer", { id: q.id, choice: "Request changes", comment })}>Request changes</button>
            <button class="btn ghost" disabled={busy} onclick={() => { editing = r.id; text = t?.text || ""; }}>Edit</button>
          </div>
        {/if}
      {/if}
    </section>
  {/each}
</div>
<h2>Case narrative</h2>
<p class="small muted">Written by the agents; the reports print these sections.</p>
{#if !sections.length}<p class="muted">Nothing written yet.</p>{/if}
{#each sections as [k, title] (k)}
  <section class="card" style="margin-bottom:12px"><div class="row"><strong>{title}</strong><span class="small muted">updated <Ago iso={c.narrative[k].at} /></span></div>
    <Md text={c.narrative[k].text} cls="wide" /></section>
{/each}
