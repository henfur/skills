<script>
  import Badge from "../components/Badge.svelte";
  import { app, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  let { d } = $props();
  const k = $derived(d.desk);
  const cfg = $derived(k.config);
  const profiles = $derived(Object.keys(cfg.profiles).sort());
  const MODES = [["shadow", "Shadow", "Drafts only: you publish or discard each one."],
    ["assisted", "Assisted", "Comments are posted; approvals and labels wait for you."],
    ["autonomous", "Autonomous", "Comments, approvals and labels follow the policy."]];
  let error = $state(""), busy = $state(false);
  async function act(action, body) {
    busy = true;
    const r = await post(projectURL(app.data.id, action), body);
    busy = false;
    error = r.ok ? (r.json.warning || "") : (r.json.error || "Not changed.");
    await refresh();
  }
  const a = $derived(cfg.policy.approve);
</script>

<h1>Desk</h1>
<p class="lede">What the desk may do. Everything here lives in <code>{app.data.config?.path || ".lacarta/config.json"}</code>: changes made here are written there, and edits made there show here.</p>
<span class="err" role="alert">{error}</span>
<h2>Loop</h2>
<div class="row">
  {#if k.paused}<Badge s="stopped" text="Paused" /><button class="btn" disabled={busy} onclick={() => act("desk-pause", { paused: false })}>Resume</button>
  {:else}<Badge s="approved" text="Running" /><button class="btn ghost" disabled={busy} onclick={() => act("desk-pause", { paused: true })}>Pause</button>{/if}
  <span class="small muted">Paused: nothing is posted and no review starts; reviews in progress finish as drafts.</span>
</div>
<h2>Modes</h2>
<div class="stack">
  {#each profiles as p (p)}
    {@const pr = cfg.profiles[p]}
    <section class="card">
      <div class="row"><strong>{p}</strong><span class="small muted">{pr.instructions || "built-in instructions"} · {pr.model}</span></div>
      <div class="row" role="radiogroup" aria-label="Mode of {p}">
        {#each MODES as [m, name, help] (m)}
          <label class="small" title={help}><input type="radio" name="mode-{p}" checked={pr.mode === m} disabled={busy}
            onchange={() => act("desk-mode", { profile: p, mode: m })} /> {name}</label>
        {/each}
      </div>
      <div class="small muted">{MODES.find(m => m[0] === pr.mode)?.[2]}</div>
    </section>
  {/each}
</div>
<p class="small muted">An approval needs every profile that reviewed the merge request to be autonomous. A mode raised in the file (or by an agent) only counts once you accept the desk's new content.</p>
<h2>Policy</h2>
<div class="card list">
  <div class="row"><span class="overline">Comments</span><span>{cfg.policy.comment}</span></div>
  <div class="row"><span class="overline">Approve</span><span>{a ? `when every profile passes, nothing above ${a.max_severity}, pipeline ${a.pipeline}${a.verified ? ", blocking findings verified" : ""}` : "never"}</span></div>
  <div class="row"><span class="overline">Request changes</span><span>{cfg.policy.request_changes ? `at ${cfg.policy.request_changes} or above` : "never"}</span></div>
  <div class="row"><span class="overline">Fixed threads</span><span>{cfg.policy.resolve_fixed_threads ? "resolved" : "left open"}</span></div>
  <div class="row"><span class="overline">Never</span><span>merge, close, push, run pipelines, approve its own merge requests, or approve when a reviewer reported an instruction aimed at it</span></div>
</div>
<h2>Forge</h2>
<div class="card list">
  <div class="row"><span class="overline">Host</span><code>{cfg.forge.host || "not set"}</code></div>
  <div class="row"><span class="overline">Posts as</span><span>{cfg.forge.identity || "not set"}</span><code class="small">{cfg.forge.write_config || "write_config not set"}</code></div>
  <div class="row"><span class="overline">Agents read as</span><code class="small">{cfg.forge.read_config || "read_config not set"}</code></div>
  <div class="row"><span class="overline">Targets</span><span>{cfg.targets.map(t => t.group || t.project).join(", ") || "none"}</span></div>
</div>
