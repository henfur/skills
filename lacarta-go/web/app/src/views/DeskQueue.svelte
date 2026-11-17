<script>
  import Badge from "../components/Badge.svelte";
  import SevBadge from "../components/SevBadge.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  import { app, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  let { st } = $props();
  const items = $derived(app.data.desk?.items || []);
  const TONE = { queued: "draft", reviewing: "in-progress", drafted: "awaiting-approval", publishing: "in-progress",
    published: "approved", skipped: "draft", superseded: "draft", dropped: "draft", discarded: "rejected" };
  let open = $state(""), editing = $state(""), text = $state(""), error = $state(""), busy = $state(false), filter = $state("active");
  const shown = $derived(items.filter(it => filter === "all" || !["published", "discarded", "dropped", "superseded", "skipped"].includes(it.status)));
  const question = it => st.questions.find(q => q.id === it.question && q.status === "open");
  const findings = it => Object.entries(it.records || {}).flatMap(([p, r]) => (r.findings || []).map(f => ({ ...f, profile: p })));
  const effective = f => f.verified ? (f.verified.verdict === "rejected" ? "" : f.verified.verdict === "downgraded" ? f.verified.severity : f.severity) : f.severity;
  const stepText = s => s.kind === "note" ? `Summary note${s.profile ? " · " + s.profile : ""}` : s.kind === "inline" ? `Inline on ${s.file}:${s.line} · ${s.profile}`
    : s.kind === "resolve" ? `Resolve the fixed thread · ${s.profile}` : s.kind === "labels" ? `Labels +${(s.add || []).join(",")} -${(s.remove || []).join(",")}` : s.kind === "approve" ? "Approve the merge request" : s.kind === "unapprove" ? "Withdraw the desk's approval" : s.kind;
  async function act(action, body, msg) {
    busy = true;
    const r = await post(projectURL(app.data.id, action), body);
    busy = false;
    error = r.ok ? (r.json.warning || "") : (r.json.error || msg);
    await refresh();
    return r.ok;
  }
  async function save(it, s) {
    if (await act("desk-edit", { item: it.key, step: s.id, body: text }, "Not saved.")) editing = "";
  }
  const answer = (it, choice) => act("answer", { id: question(it).id, choice, comment: "" }, "Not answered.");
</script>

<h1>Queue</h1>
<p class="lede">Every merge request the desk tracks, newest first. Drafts wait for you here: read them, edit a comment if needed, then publish or discard.</p>
<div class="row" style="margin-bottom:8px"><label class="small"><input type="radio" bind:group={filter} value="active" /> In progress</label>
  <label class="small"><input type="radio" bind:group={filter} value="all" /> Everything</label></div>
<span class="err" role="alert">{error}</span>
{#if !shown.length}<p class="muted">Nothing here yet.</p>{/if}
<div class="stack">
  {#each shown as it (it.key)}
    {@const q = question(it)}
    {@const fs = findings(it)}
    <section class="card">
      <div class="row">
        <button class="desk-key" onclick={() => open = open === it.key ? "" : it.key} aria-expanded={open === it.key}><strong>{it.key}</strong></button>
        <Badge s={TONE[it.status] || "draft"} text={it.status} />
        <span>{it.title}</span>
        <span class="small muted">by {it.author} · <code>{(it.head_sha || "").slice(0, 8)}</code> · <Ago iso={it.updated_at || it.queued_at} /></span>
        {#if it.url}<a class="small" href={it.url} target="_blank" rel="noopener noreferrer">Open on GitLab</a>{/if}
      </div>
      {#if it.reason}<div class="small muted">{it.reason}</div>{/if}
      {#if it.status === "reviewing"}<div class="small muted">{Object.entries(it.profiles || {}).map(([p, s]) => `${p}: ${s}`).join(" · ")}</div>{/if}
      {#if open === it.key || q}
        {#if it.plan}
          <div class="small" style="margin-top:8px">Verdict: <strong>{it.plan.verdict}</strong>{#each it.plan.reasons as r, i (i)}<div class="muted">{r}</div>{/each}</div>
          {#if fs.length}
            <h3>Findings</h3>
            <div class="list">{#each fs as f (f.profile + f.id)}<div class="row"><SevBadge s={effective(f) || f.severity} /><span class="chip">{f.profile}/{f.id}</span>
              <span class:muted={!effective(f)}>{f.title}</span>{#if f.file}<code class="small">{f.file}{f.line ? ":" + f.line : ""}</code>{/if}
              {#if f.verified}<span class="small muted">verifier: {f.verified.verdict}</span>{/if}</div>{/each}</div>
          {/if}
          <h3>Steps</h3>
          {#each it.plan.steps as s (s.id)}
            <div class="card" style="margin:6px 0">
              <div class="row"><span class="chip">{s.id}</span><strong>{stepText(s)}</strong>
                <Badge s={s.status === "done" ? "approved" : s.status === "failed" ? "rejected" : s.status === "skipped" ? "draft" : s.auto ? "in-progress" : "awaiting-approval"}
                  text={s.status === "pending" ? (s.auto ? "automatic" : "waits for you") : s.status} />
                <span class="small muted">{s.why}</span>{#if s.edited}<span class="small muted">· edited by you</span>{/if}</div>
              {#if s.error}<div class="small err">{s.error}</div>{/if}
              {#if s.body}
                {#if editing === it.key + s.id}
                  <textarea rows="12" style="width:100%" bind:value={text} aria-label="Comment {s.id}"></textarea>
                  <div class="row"><button class="btn" disabled={busy} onclick={() => save(it, s)}>Save</button><button class="btn ghost" onclick={() => editing = ""}>Cancel</button>
                    <span class="small muted">lacarta adds its hidden marker and the AI-review footer when it posts.</span></div>
                {:else}
                  <div class="desk-draft"><Md text={s.body} cls="wide" /></div>
                  {#if q && s.status === "pending" && !s.auto}<button class="btn ghost sm" onclick={() => { editing = it.key + s.id; text = s.body; }}>Edit</button>{/if}
                {/if}
              {/if}
            </div>
          {/each}
          {#if q}
            <div class="row" style="margin-top:8px"><button class="btn primary" disabled={busy} onclick={() => answer(it, "Publish")}>Publish the waiting steps</button>
              <button class="btn ghost" disabled={busy} onclick={() => answer(it, "Discard")}>Discard</button>
              <span class="small muted">Posted as {app.data.derived.desk.config.forge.identity || "the desk's identity"}.</span></div>
          {/if}
        {:else if it.status !== "queued"}<p class="small muted">No plan for this head.</p>{/if}
      {/if}
    </section>
  {/each}
</div>
