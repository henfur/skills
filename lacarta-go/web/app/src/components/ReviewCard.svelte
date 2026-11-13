<script>
  // The auditor's decision on one finding. Only the console records it.
  import { app, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  import { AUDIT_SEV, label } from "../lib/format.js";
  let { f, open } = $props();
  const actions = $derived(app.data.audit.actions[f.verdict] || []);
  const help = {
    accept: "The finding goes into the report as the agents recorded it.",
    severity: "Set the severity you stand behind. The report shows the agents' rating, yours, and your reason.",
    reject: "It is not a vulnerability. It moves to the rejected list with your reason; the agents' record stays in the appendix.",
    recheck: "A fresh verifier checks it again with your question; it comes back for review.",
    keep: "It goes into the report's Needs validation section with its blocker and validation plan.",
    confirm: "You checked the missing fact yourself: give a severity and what you established.",
    "out-of-scope": "It doesn't belong in this audit's scope; your reason is shown in the appendix.",
  };
  let action = $state(""), severity = $state(""), reason = $state(""), error = $state(""), busy = $state(false);
  const needsSev = $derived(action === "severity" || action === "confirm");
  const needsReason = $derived(action && action !== "accept" && action !== "keep");
  async function save() {
    busy = true;
    const r = await post(projectURL(app.data.id, "review"), { fingerprint: f.fingerprint, action, severity, reason });
    busy = false;
    if (!r.ok) { error = r.json.error || "Not saved."; return; }
    error = ""; action = ""; reason = ""; severity = "";
    await refresh();
  }
</script>

<div class="review-card" class:done={f.review && f.review.action !== "recheck"}>
  {#if f.review && !open}
    <div class="row"><strong>Your review:</strong>
      {#if f.review.action === "severity"}severity changed from {label(f.review.from)} to {label(f.review.severity)}
      {:else if f.review.action === "confirm"}confirmed as {label(f.review.severity)}
      {:else}{(actions.find(a => a.K === f.review.action) || {}).V ?? f.review.action}{/if}
      {#if f.review.action === "recheck"}<span class="badge warning">Waiting for another check</span>{/if}</div>
    {#if f.review.reason}<div class="small muted">{f.review.reason}</div>{/if}
  {/if}
  {#if open}
    <div class="field">Your review</div>
    <div class="choices-detailed" role="group" aria-label="Your review of {f.title}">
      {#each actions as a (a.K)}
        <button type="button" class="btn choice" aria-pressed={action === a.K} onclick={() => { action = a.K; error = ""; }}>{a.V}<span class="help">{help[a.K]}</span></button>
      {/each}
    </div>
    {#if needsSev}
      <label class="field" for="sev-{f.fingerprint}">Severity</label>
      <select id="sev-{f.fingerprint}" bind:value={severity}><option value="" disabled>Choose…</option>
        {#each AUDIT_SEV.filter(s => action !== "severity" || s !== f.severity) as s (s)}<option value={s}>{label(s)}</option>{/each}</select>
    {/if}
    {#if needsReason}
      <label class="field" for="why-{f.fingerprint}">Reason <span class="muted">(goes into the report)</span></label>
      <textarea id="why-{f.fingerprint}" bind:value={reason} placeholder="Why, in a sentence or two"></textarea>
    {/if}
    <div class="row" style="justify-content:flex-end;margin-top:8px"><span class="err" role="alert">{error}</span>
      <button type="button" class="btn primary" disabled={!action || busy || (needsSev && !severity) || (needsReason && !reason.trim())} onclick={save}>Save review</button></div>
  {/if}
</div>
