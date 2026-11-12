<script>
  import { app, ui, modelName, refresh } from "../lib/state.svelte.js";
  import { post } from "../lib/api.js";
  // Machine-wide: the remote-control default and the model for every role of every kind.
  const cat = $derived(app.hub.catalog);
  const saved = $derived(app.hub.settings.models);
  const key = (k, r) => `${k}.${r}`;
  const current = (k, r) => ui.modelDraft[key(k, r)] ?? saved[k][r];
  const unsaved = (k, r) => ui.modelDraft[key(k, r)] !== undefined && ui.modelDraft[key(k, r)] !== saved[k][r];
  const dirty = $derived(Object.entries(ui.modelDraft).some(([kr, m]) => { const [k, r] = kr.split("."); return saved[k][r] !== m; }));
  const stagesOf = kind => [...new Set(Object.values(kind.roles).map(x => x.stage))];
  let saving = $state(false);

  async function save(body) {
    saving = true;
    const r = await post("/api/settings", body);
    if (!r.ok) ui.modelError = r.json.error || "The settings were not saved.";
    else { if (body.models) ui.modelDraft = {}; ui.modelError = ""; ui.settingsNote = "Saved."; }
    saving = false;
    await refresh();
  }
  function saveModels() {
    const models = {};
    Object.entries(ui.modelDraft).forEach(([kr, m]) => { const [k, r] = kr.split("."); (models[k] = models[k] || {})[r] = m; });
    save({ models });
  }
</script>

<h1>Settings</h1>
<p class="lede">For every project on this machine. Agents read the models before each spawn, so a change applies to the next agent started.</p>
<h2>Remote control</h2>
<section class="card"><label class="toggle"><input type="checkbox" checked={app.hub.settings.remote_control} onchange={e => save({ remote_control: e.currentTarget.checked })} />
  <span><strong>Keep agents listening to the console when their turn ends</strong><br />
  <span class="small muted">Each session keeps a listener running in the background: when you write from the console, answer or ask about a question, the agent wakes up and acts on it, even after its turn ended. You can keep using its terminal meanwhile. Each project's Chat can override this.</span></span></label></section>
<h2>Models by kind of project</h2>
<div class="stack">
  {#each Object.entries(cat.kinds) as [k, kind] (k)}
    {@const stages = stagesOf(kind)}
    <section class="card"><h3>{kind.label}</h3><p class="small muted" style="margin:2px 0 8px">{kind.about}</p>
      {#each stages as stage (stage)}
        {#if stages.length > 1}<div class="overline" style="margin:12px 0 4px">{stage}</div>{/if}
        <div class="list">
          {#each Object.entries(kind.roles).filter(([, x]) => x.stage === stage) as [r, x] (r)}
            <div class="model-row">
              <div><label class="field" for="m-{k}-{r}" style="color:var(--text);font-size:15px;line-height:22px">{x.label}</label>
                <div class="small muted">{x.about}</div>
                <div class="small muted">Default {modelName(x.default)} · <code>{cat.models[current(k, r)].id}</code>{#if unsaved(k, r)} · <strong style="color:var(--warning)">Unsaved</strong>{/if}</div></div>
              <select id="m-{k}-{r}" aria-label="Model for {x.label}" value={current(k, r)}
                onchange={e => { ui.modelDraft[key(k, r)] = e.currentTarget.value; ui.settingsNote = ""; }}>
                {#each Object.entries(cat.models) as [alias, m] (alias)}<option value={alias}>{m.name}</option>{/each}
              </select>
            </div>
          {/each}
        </div>
      {/each}
    </section>
  {/each}
  <section class="card"><div class="overline">Coming later</div><div class="small muted" style="margin-top:4px">{cat.planned.join(" · ")}</div></section>
</div>
<div class="row" style="margin-top:16px;justify-content:flex-end">
  <span class="err" role="alert" style="margin-right:auto">{ui.modelError}</span>
  <span class="small muted">{ui.settingsNote}</span>
  {#if dirty}<button type="button" class="btn" onclick={() => { ui.modelDraft = {}; }}>Discard changes</button>{/if}
  <button type="button" class="btn primary" disabled={!dirty || saving} onclick={saveModels}>{saving ? "Saving…" : "Save models"}</button>
</div>
