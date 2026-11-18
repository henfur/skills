<script>
  import { app, modelName, refresh } from "../lib/state.svelte.js";
  import { post } from "../lib/api.js";
  import Icon from "../components/Icon.svelte";
  // Machine-wide. The models and skills of every role are read from the config file and only
  // shown here; this page saves the remote-control default and the auditor's name into that file.
  const cat = $derived(app.hub.catalog);
  const models = $derived(app.hub.settings.models);
  const skills = $derived(app.hub.skills || {});
  const stagesOf = kind => [...new Set(Object.values(kind.roles).map(x => x.stage))];
  const scope = { everyone: "every agent", kind: "every agent of this kind", role: "this role" };
  const skillTone = s => ({ unusable: "danger", missing: "warning" }[s.status] || "plain");
  const skillTitle = s => [`For ${scope[s.scope]}.`, s.path, s.note].filter(Boolean).join(" ");
  let saving = $state(false), error = $state(""), note = $state("");
  let auditor = $state(null);
  const auditorText = $derived(auditor ?? app.hub.settings.auditor ?? "");
  const cfg = $derived(app.hub.settings.config);

  async function save(body) {
    saving = true;
    const r = await post("/api/settings", body);
    error = r.ok ? "" : (r.json.error || "The settings were not saved.");
    note = r.ok ? "Saved." : "";
    saving = false;
    await refresh();
  }
</script>

<h1>Settings</h1>
<p class="lede">For every project on this machine. The configuration file is the source of truth: the models and skills of each role are set there and shown here.</p>
<h2>Configuration file</h2>
<section class="card">
  <div class="row"><code>{cfg.path}</code><span class="small muted">{cfg.exists ? "" : "not created yet: `lacarta config init` creates it"}</span></div>
  <p class="small muted" style="margin:6px 0 0">Edit it to set the models and skills of each role, the plugins, spec templates and tool paths; the two settings below are saved into it too. <code>lacarta config</code> shows what is in effect and <code>lacarta config check</code> what is ignored.
    A project's own <code>.lacarta/config.json</code> names it, can override models, remote control, spec templates and plugins, and adds its own skills. Tool paths and the auditor are set only here: a project's file can be in a repository you cloned.</p>
  {#if cfg.warnings.length}<div class="banner warn" style="margin:8px 0 0"><Icon name="warn" /><div><strong>Ignored in this file</strong>
    <ul class="small" style="margin:4px 0 0">{#each cfg.warnings as w, i (i)}<li>{w}</li>{/each}</ul></div></div>{/if}
  <div class="list" style="margin-top:8px">
    <div class="row"><span class="field" style="margin:0;min-width:120px">Spec templates</span>{#if app.hub.settings.templates}<code class="small">{app.hub.settings.templates}</code>{:else}<span class="small muted">built-in</span>{/if}</div>
    {#each app.hub.tools || [] as t (t.name)}
      <div class="row"><span class="field" style="margin:0;min-width:120px">{t.name}</span>
        {#if t.path}<code class="small" style="overflow-wrap:anywhere;min-width:0">{t.path}</code>{:else}<span class="small muted">not found</span>{/if}
        {#if t.configured}<span class="badge plain">set in config</span>{/if}<span class="small muted" style="margin-left:auto">{t.about}</span></div>
    {/each}
  </div>
</section>
<span class="err" role="alert">{error}</span>
<h2>Remote control</h2>
<section class="card"><label class="toggle"><input type="checkbox" checked={app.hub.settings.remote_control} onchange={e => save({ remote_control: e.currentTarget.checked })} />
  <span><strong>Keep agents listening to the console when their turn ends</strong><br />
  <span class="small muted">Each session keeps a listener running in the background: when you write from the console, answer or ask about a question, the agent wakes up and acts on it, even after its turn ended. You can keep using its terminal meanwhile. Each project's Chat can override this.</span></span></label></section>
<h2>Auditor</h2>
<section class="card"><label class="field" for="auditor">Your name, as it appears on the security audit reports you sign</label>
  <div class="row"><input id="auditor" type="text" value={auditorText} oninput={e => { auditor = e.currentTarget.value; }} placeholder="First and last name" style="flex:1;min-width:0" />
    <button type="button" class="btn" disabled={auditor === null || saving} onclick={async () => { await save({ auditor: auditorText }); auditor = null; }}>Save name</button><span class="small muted">{note}</span></div>
  <p class="small muted" style="margin:6px 0 0">Reports are owned by a person: approving one needs your name. Every report states that the audit was AI-assisted, and is signed so a client can check it wasn't changed.</p></section>
<h2>Plugins on this machine</h2>
<section class="card list">
  {#each app.hub.plugins || [] as p (p.id)}
    <div class="row"><strong>{p.name}</strong><span class="badge plain">{p.type}</span>{#if p.custom}<span class="badge accent">Custom</span>{/if}
      <span class="small muted">{p.installed ? p.path : "Not installed"}{#if p.tested === false} · not the tested version{/if}{#if app.hub.settings.plugins?.[p.id] !== undefined} · turned {app.hub.settings.plugins[p.id] ? "on" : "off"} in the config file{/if}</span>
      {#if !p.installed && p.install_hint}<code class="small" style="margin-left:auto">{p.install_hint}</code>{/if}</div>
    {#if p.problem && p.installed}<div class="small err">{p.problem}</div>{/if}
  {/each}
  <div class="small muted">Turn one on or off with <code>"plugins": {"{"}"&lt;id&gt;": true{"}"}</code> in the configuration file, or in a project's own. Add your own with <code>lacarta plugin add &lt;folder or git URL&gt;</code>, then name it under <code>skills</code> for the roles that load it.</div>
</section>
<h2>Models and skills by kind of project</h2>
<p class="small muted" style="margin:0 0 8px">Read from the configuration file before each agent starts, so a change there applies to the next one. Set them like this:</p>
<pre class="small config-sample">{`"models": { "software": { "security_review": "opus" } },
"skills": {
  "all": ["a-skill-every-agent-loads"],
  "software": { "all": ["for-every-agent-of-this-kind"], "security_review": ["for-one-role", "plugin:skill"] }
}`}</pre>
<div class="stack">
  {#each Object.entries(cat.kinds) as [k, kind] (k)}
    {@const stages = stagesOf(kind)}
    <section class="card"><h3>{kind.label} <code class="small muted">{k}</code></h3><p class="small muted" style="margin:2px 0 8px">{kind.about}</p>
      {#each stages as stage (stage)}
        {#if stages.length > 1}<div class="overline" style="margin:12px 0 4px">{stage}</div>{/if}
        <div class="list">
          {#each Object.entries(kind.roles).filter(([, x]) => x.stage === stage) as [r, x] (r)}
            {@const m = models[k][r]}
            <div class="model-row" data-role="{k}.{r}">
              <div><div style="font-weight:600">{x.label} <code class="small muted">{r}</code></div>
                <div class="small muted">{x.about}</div>
                <div class="row skills" style="margin-top:4px;gap:4px">
                  {#each skills[k]?.[r] || [] as s (s.name)}
                    <span class="badge {skillTone(s)}" title={skillTitle(s)}>{s.name}{s.status === "missing" ? " · not found" : s.status === "unusable" ? " · not given" : ""}</span>
                  {:else}<span class="small muted">No skills</span>{/each}
                </div></div>
              <div class="model"><strong>{modelName(m)}</strong>
                <div class="small muted">{m === x.default ? "default" : `default ${modelName(x.default)}`} · <code>{cat.models[m].id}</code></div></div>
            </div>
          {/each}
        </div>
      {/each}
    </section>
  {/each}
  <section class="card"><div class="overline">Coming later</div><div class="small muted" style="margin-top:4px">{cat.planned.join(" · ")}</div></section>
</div>
