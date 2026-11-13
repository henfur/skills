<script>
  import Badge from "../components/Badge.svelte";
  import Md from "../components/Md.svelte";
  import { app } from "../lib/state.svelte.js";
  import { UNIT_TONE } from "../lib/format.js";
  let { st, d } = $props();
  const units = $derived(app.data.audit?.units || []);
  const rows = $derived([...new Set(units.map(u => u.subsystem))].sort());
  const cols = $derived([...new Set(units.map(u => u.attack_class))].sort());
  const cell = (r, c) => units.filter(u => u.subsystem === r && u.attack_class === c);
  // The cell's colour is its least finished unit's: open work shows first.
  const ORDER = ["in_progress", "planned", "candidate", "blocked", "deferred", "covered", "out_of_scope"];
  const worst = us => ORDER.find(s => us.some(u => u.status === s));
  const counts = $derived(units.reduce((m, u) => (m[u.status] = (m[u.status] || 0) + 1, m), {}));
  let open = $state(null);
  const name = s => s.replace(/_/g, " ");
</script>

<h1>Coverage</h1>
<p class="lede">The run's coverage ledger: every area of the code against every attack class it was hunted for. A cell opens its units.</p>
{#if !units.length}
  <div class="card empty">No coverage ledger yet. The agents write it during reconnaissance; you approve it as the coverage plan.</div>
{:else}
  <div class="row" style="margin-bottom:12px">{#each ORDER.filter(s => counts[s]) as s (s)}<span class="badge {UNIT_TONE[s]}">{counts[s]} {name(s)}</span>{/each}</div>
  <div class="table-wrap"><table class="data coverage"><thead><tr><th>Area</th>{#each cols as c (c)}<th>{c}</th>{/each}</tr></thead><tbody>
    {#each rows as r (r)}
      <tr><td><strong>{r}</strong></td>
        {#each cols as c (c)}
          {@const us = cell(r, c)}
          <td>{#if us.length}<button type="button" class="cell badge {UNIT_TONE[worst(us)]}" aria-expanded={open === r + c}
            onclick={() => { open = open === r + c ? null : r + c; }}>{us.length} · {name(worst(us))}</button>{/if}</td>
        {/each}</tr>
      {#if rows.some(x => x === r) && cols.some(c => open === r + c)}
        {@const c = cols.find(c => open === r + c)}
        <tr><td colspan={cols.length + 1}><div class="list">
          {#each cell(r, c) as u (u.coverage_id)}
            <div class="unit"><div class="row"><Badge s={u.status} text={name(u.status)} /><strong>{u.surface}</strong><span class="small muted">· {u.boundary}{#if u.lifecycle} · {u.lifecycle}{/if} · wave {u.wave}</span></div>
              <div class="small">Starts from {#each u.starting_paths as p, i (i)}{#if i}, {/if}<code>{p}</code>{/each}{#if u.agent_id} · hunter <code>{u.agent_id}</code>{/if}</div>
              {#if u.reviewed_paths?.length}<div class="small muted">Reviewed {u.reviewed_paths.length} {u.reviewed_paths.length === 1 ? "path" : "paths"}, {u.local_checks?.length || 0} checks</div>{/if}
              {#if u.result_fingerprints?.length}<div class="small">Candidates: {#each u.result_fingerprints as f, i (f)}{#if i}, {/if}<code>{f}</code>{/each}</div>{/if}
              {#if u.excluded_blocks?.length}<details><summary class="small">{u.excluded_blocks.length} excluded attack-class {u.excluded_blocks.length === 1 ? "block" : "blocks"}</summary>
                <ul class="small">{#each u.excluded_blocks as x, i (i)}<li><code>{x.block}</code>: <Md text={x.reason} inline /></li>{/each}</ul></details>{/if}
            </div>
          {/each}
        </div></td></tr>
      {/if}
    {/each}
  </tbody></table></div>
{/if}
{#if app.data.audit?.architecture}
  <h2>Architecture map</h2>
  <details class="card"><summary>architecture.md, written during reconnaissance</summary><Md text={app.data.audit.architecture} cls="wide" /></details>
{/if}
