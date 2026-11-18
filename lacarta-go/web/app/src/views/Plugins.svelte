<script>
  import Badge from "../components/Badge.svelte";
  import { app } from "../lib/state.svelte.js";
  let { st } = $props();
  // Read-only: the config files turn plugins on and off.
  const list = $derived(app.data.audit?.plugins || []);
  const path = $derived(app.data.config?.path);
  const from = p => p.need === "required" ? "Required for this kind of project"
    : p.config === "project" ? "Set in this project's config.json"
    : p.config === "home" ? "Set in the machine's config.json" : "Default";
</script>

<h1>Plugins</h1>
<p class="lede">What the agents may use in this project. The configuration files decide: a change there applies from the next wave or session; a run keeps the version it started with.</p>
<div class="stack">
  {#each list as p (p.id)}
    <section class="card plugin" data-plugin={p.id}>
      <div class="row">
        <strong>{p.name}</strong>
        <Badge s={p.enabled ? "approved" : "draft"} text={p.enabled ? "On" : "Off"} />
        <Badge s="draft" text={p.type} />
        <span class="small muted">{from(p)}</span>
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
<p class="small muted">Turn one on or off with <code>"plugins": {"{"}"&lt;id&gt;": true{"}"}</code> in {#if path}<code>{path}</code>{:else}this project's config.json{/if}, or for every project in the machine's file (Settings).
  Add your own skills or attack-class files with <code>lacarta plugin add &lt;folder or git URL&gt;</code> in a terminal: it shows their content before you trust them.</p>
