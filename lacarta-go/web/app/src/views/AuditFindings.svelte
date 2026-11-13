<script>
  import SevBadge from "../components/SevBadge.svelte";
  import Badge from "../components/Badge.svelte";
  import Md from "../components/Md.svelte";
  import ReviewCard from "../components/ReviewCard.svelte";
  import { app, ui } from "../lib/state.svelte.js";
  import { label } from "../lib/format.js";
  let { st, d } = $props();
  const all = $derived(app.data.audit?.findings || []);
  const reviewing = $derived(st.stage === "review" || st.stage === "report");
  const TABS = [["confirmed", "Confirmed"], ["needs_validation", "Needs validation"], ["out_of_scope", "Out of scope"], ["rejected", "Rejected"]];
  const tab = $derived(TABS.some(t => t[0] === ui.auditTab) ? ui.auditTab : "confirmed");
  const list = $derived(all.filter(f => f.final === tab));
  const pending = f => reviewing && (f.verdict === "confirmed" || f.verdict === "needs_validation") && (!f.review || f.review.action === "recheck");
  let editing = $state({});
</script>

<h1>Findings</h1>
<p class="lede">The agents' records with your review. Severities shown are the final ones: yours where you changed them.
  {#if reviewing}{d.audit.to_review ? `${d.audit.to_review} still to review before the report.` : "Every finding is reviewed."}{/if}</p>
{#if !all.length}
  <div class="card empty">{d.audit.has_findings ? "No findings in this run." : "No findings yet. The agents submit them after hunting and validation."}</div>
{:else}
  <div class="tabs" role="tablist">
    {#each TABS as [k, name] (k)}
      {@const n = all.filter(f => f.final === k).length}
      {#if n || k === "confirmed"}<button type="button" role="tab" aria-selected={tab === k} onclick={() => { ui.auditTab = k; }}>{name} · {n}</button>{/if}
    {/each}
  </div>
  <div class="stack">
    {#each list as f (f.fingerprint)}
      <article class="card finding" class:pending={pending(f)}>
        <div class="row">{#if f.severity}<SevBadge s={f.severity} />{/if}<h3 style="margin:0">{f.title}</h3>
          {#if pending(f)}<Badge s="awaiting-approval" text="To review" />{/if}<code class="small muted" style="margin-left:auto">{f.fingerprint}</code></div>
        {#if f.review?.action === "severity"}<div class="small">Agents' severity: <SevBadge s={f.review.from} /> → yours: <SevBadge s={f.review.severity} /></div>{/if}
        <Md text={f.description} />
        {#if f.root_cause || f.claimed_root_cause}<p class="small"><strong>Root cause.</strong> {f.root_cause || f.claimed_root_cause}</p>{/if}
        {#if f.reason}<p class="small"><strong>Why the verifier rejected it.</strong> {f.reason}</p>{/if}
        <details><summary class="small">Trace, evidence, reproduction and fix</summary>
          {#if f.trace?.length}<div class="overline">Source trace</div><ol class="small">{#each f.trace as t, i (i)}<li><code>{t.file}:{t.line}</code> {t.kind ? `(${t.kind})` : ""} {t.description}</li>{/each}</ol>{/if}
          {#if f.evidence?.length}<div class="overline">Evidence</div><ul class="small">{#each f.evidence as e, i (i)}<li><code>{e.file}:{e.line}</code> {e.description}</li>{/each}</ul>{/if}
          {#if f.conditions?.length}<div class="overline">Conditions</div><ul class="small">{#each f.conditions as c, i (i)}<li>{label(c.kind.replace(/_/g, " "))}: {c.description}</li>{/each}</ul>{/if}
          {#if f.execution}<div class="overline">Reproduction</div>
            <p class="small">Attacker: {f.execution.attacker_perspective}</p>
            <ol class="small">{#each f.execution.instructions as s, i (i)}<li>{s}</li>{/each}</ol>
            <p class="small">Observed: {f.execution.observed_result}</p>{/if}
          {#if f.remediation}<div class="overline">Fix</div><Md text={f.remediation.strategy} />
            {#each f.remediation.code_changes || [] as c, i (i)}<div class="small"><code>{c.file_name}</code></div><pre><code>{c.fixed_code}</code></pre>{/each}{/if}
          {#if f.blockers?.length}<div class="overline">Unresolved</div><ul class="small">{#each f.blockers as b, i (i)}<li>{b}</li>{/each}</ul>{/if}
          {#if f.validation_plan}<div class="overline">Validation plan</div><p class="small">Locally: {f.validation_plan.local}<br />In a test deployment: {f.validation_plan.deployment}</p>{/if}
          {#if f.confidence}<p class="small muted">Agents' confidence: {f.confidence.score}: {f.confidence.reason}</p>{/if}
        </details>
        {#if reviewing && (f.verdict === "confirmed" || f.verdict === "needs_validation")}
          <ReviewCard {f} open={pending(f) || editing[f.fingerprint]} />
          {#if !pending(f)}<div class="row" style="justify-content:flex-end"><button type="button" class="btn" onclick={() => { editing[f.fingerprint] = !editing[f.fingerprint]; }}>{editing[f.fingerprint] ? "Cancel" : "Change my review"}</button></div>{/if}
        {:else if f.review}
          <ReviewCard {f} open={false} />
        {/if}
      </article>
    {/each}
  </div>
{/if}
