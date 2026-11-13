<script>
  import Md from "../components/Md.svelte";
  import Ago from "../components/Ago.svelte";
  import Icon from "../components/Icon.svelte";
  import { app, refresh } from "../lib/state.svelte.js";
  import { post, projectURL, TOKEN } from "../lib/api.js";
  let { st, d } = $props();
  const au = $derived(app.data.audit);
  const versions = $derived((st.audit.report?.versions || []).slice().reverse());
  const ready = $derived(st.stage === "report");
  const url = (q) => `${projectURL(app.data.id, "report")}?${q}&key=${encodeURIComponent(TOKEN)}`;
  let summary = $state(null), note = $state(""), error = $state(""), busy = $state(false), confirmed = $state(false);
  const text = $derived(summary ?? st.audit.summary ?? "");
  async function saveSummary() {
    busy = true;
    const r = await post(projectURL(app.data.id, "summary"), { text });
    busy = false;
    if (!r.ok) { error = r.json.error || "Not saved."; return; }
    summary = null; error = ""; note = "Summary saved."; await refresh();
  }
  async function approve() {
    busy = true;
    const r = await post(projectURL(app.data.id, "approve"), { hash: au.draft_hash });
    busy = false;
    if (!r.ok) { error = r.json.error || "Not approved."; return; }
    error = ""; confirmed = false; await refresh();
  }
</script>

<h1>Report</h1>
<p class="lede">Generated from the records and your reviews, with a fixed AI-assistance disclosure. Only you can approve it; the final version is signed.</p>
{#if versions.length}
  <h2>Approved versions</h2>
  <div class="card list">
    {#each versions as v (v.version)}
      <div class="row"><strong>v{v.version}</strong><span class="small muted">{v.run} · approved by {v.auditor} <Ago iso={v.approved_at} /></span>
        <span style="margin-left:auto" class="row">
          <a class="btn" href={url(`v=${v.version}&f=html`)} target="_blank" rel="noopener">Open (print to PDF)</a>
          <a class="btn" href={url(`v=${v.version}&f=md`)}>Markdown</a>
          {#if v.files.pdf}<a class="btn" href={url(`v=${v.version}&f=pdf`)}>PDF</a>{/if}
          <a class="btn" href={url(`v=${v.version}&f=provenance`)}>Provenance</a>
          <a class="btn" href={url(`v=${v.version}&f=sig`)}>Signature</a>
          <a class="btn" href={url(`v=${v.version}&f=signers`)}>allowed_signers</a></span></div>
    {/each}
    <div class="small muted">Give the client the report with its provenance file, signature and allowed_signers: they can check it with <code>lacarta audit verify</code> or OpenSSH alone (see the report's last section).{#if !versions[0].files.pdf} No PDF was printed: Chromium isn't installed, so use "Open (print to PDF)".{/if}</div>
  </div>
{/if}
{#if st.stage === "review" || ready}
  <h2>Executive summary</h2>
  <section class="card">
    <p class="small muted" style="margin-top:0">{st.audit.summary_by && st.audit.summary_by !== "user" ? "Drafted by the agent; edit it as you see fit." : "Your words, at the top of the report."} The rest of the report is generated from the records and isn't editable here.</p>
    <textarea aria-label="Executive summary" rows="8" value={text} oninput={e => { summary = e.currentTarget.value; note = ""; }}></textarea>
    <div class="row" style="justify-content:flex-end;margin-top:8px"><span class="small muted">{note}</span>
      <button type="button" class="btn" disabled={busy || summary === null} onclick={saveSummary}>Save summary</button></div>
  </section>
  <h2>Draft</h2>
  {#if !ready}<div class="banner info"><Icon name="info" /><div>Review every finding first: {d.audit.to_review} left{d.audit.recheck ? `, ${d.audit.recheck} waiting for another check` : ""}.</div></div>{/if}
  <section class="card">
    <div class="row" style="margin-bottom:8px"><span class="badge warning">Draft</span><a href={url("draft=1")} target="_blank" rel="noopener" style="margin-left:auto">Print view</a></div>
    <Md text={au.draft.replace(/^---\n[\s\S]*?\n---\n/, "")} cls="wide" />
  </section>
  {#if ready}
    <h2>Approve</h2>
    <section class="card stack">
      {#if !au.auditor}
        <div class="banner warn" style="margin:0"><Icon name="warn" /><div>The report is signed off by a person: set your name as the auditor in <a href="#/settings">Settings</a> first.</div></div>
      {:else}
        <label class="toggle"><input type="checkbox" bind:checked={confirmed} />
          <span>I, <strong>{au.auditor}</strong>, reviewed this report and take responsibility for it. It states that the audit was AI-assisted.</span></label>
        <div class="row" style="justify-content:flex-end"><span class="err" role="alert">{error}</span>
          <button type="button" class="btn primary" disabled={!confirmed || busy || !st.audit.summary} onclick={approve}>{busy ? "Signing…" : "Approve and sign the report"}</button></div>
        {#if !st.audit.summary}<div class="small muted">Save an executive summary first.</div>{/if}
      {/if}
    </section>
  {/if}
{:else if !versions.length}
  <div class="card empty">The report comes once the findings are in and you have reviewed them.</div>
{/if}
