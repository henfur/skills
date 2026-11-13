<script>
  import Badge from "../components/Badge.svelte";
  import { app, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  let { st } = $props();
  const list = $derived(app.data.audit?.plugins || []);
  let error = $state(""), busy = $state("");
  async function toggle(p, on) {
    busy = p.id;
    const r = await post(projectURL(app.data.id, "plugin"), { id: p.id, enabled: on });
    busy = "";
    error = r.ok ? "" : (r.json.error || "Not changed.");
    await refresh();
  }
</script>

<h1>Plugins</h1>
<p class="lede">What the agents may use in this project. A change applies from the next wave or session; the project keeps the version it was enabled with.</p>
<span class="err" role="alert">{error}</span>
<div class="stack">
  {#each list as p (p.id)}
    <section class="card plugin">
      <div class="row">
        <label class="toggle" style="margin:0"><input type="checkbox" checked={p.enabled} disabled={p.need === "required" || busy === p.id || (!p.enabled && (!p.installed || p.problem))}
          onchange={e => toggle(p, e.currentTarget.checked)} aria-label="Enable {p.name}" />
          <span><strong>{p.name}</strong></span></label>
        <Badge s="draft" text={p.type} />
        {#if p.need === "required"}<Badge s="awaiting-approval" text="Required" />{/if}
        {#if p.custom}<Badge s="accepted" text="Custom" />{/if}
        {#if p.tested === false}<Badge s="changed" text="Not the tested version" />{/if}
        {#if p.drift}<Badge s="changed" text="Installed version differs from this project's" />{/if}
      </div>
      <div class="small">{p.about}</div>
      <div class="small muted">{#if p.installed}{p.path}{#if p.ref && p.type !== "check"} · version {p.ref.slice(0, 12)}{/if}{:else}Not installed{#if p.install_hint}: <code>{p.install_hint}</code>{/if}{/if}</div>
      {#if p.problem}<div class="small err">{p.problem}</div>{/if}
    </section>
  {/each}
</div>
<p class="small muted">Add your own skills or attack-class files with <code>lacarta plugin add &lt;folder or git URL&gt;</code> in a terminal: it shows their content before you trust them.</p>
