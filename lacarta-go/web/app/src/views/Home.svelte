<script>
  import Badge from "../components/Badge.svelte";
  import Ago from "../components/Ago.svelte";
  import { app, ui, P, kindOf, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  import { agentStatus, label } from "../lib/format.js";
  const ps = $derived([...app.hub.projects].sort((a, b) => String(b.updated_at).localeCompare(String(a.updated_at))));
  const live = $derived(ps.filter(p => !p.archived));
  const archived = $derived(ps.filter(p => p.archived));
  // The server decides what is active: a project until it is done, a task while its session is in use,
  // anything waiting for the user. The rest is listed compactly; sessions that ended before their
  // first prompt are folded into one line.
  const active = $derived(live.filter(p => p.active || p.error));
  const inactive = $derived(live.filter(p => !p.active && !p.error));
  const ended = $derived(inactive.filter(p => !p.empty));
  const empty = $derived(inactive.filter(p => p.empty));
  const asks = $derived(live.flatMap(p => (p.questions || []).map(q => ({ p, q }))));
  const terminal = $derived(live.filter(p => p.attention));
  const status = p => p.error ? ["fail", "Unreadable"]
    : p.template === "single" && p.stage === "ended" ? ["done", p.moved_to ? "Moved" : "Ended"]
    : p.template === "single" && !p.active ? ["draft", "Idle"]
    : p.stage === "done" || p.stage === "stopped" ? ["done", label(p.stage)]
    : p.status ? agentStatus(p.status) : ["draft", label("idle")];
  let busy = $state(false);
  async function archiveAll(list) {
    busy = true;
    for (const p of list) await post(projectURL(p.id, "archive"), { archived: true });
    busy = false;
    await refresh();
  }
  async function archive(p, on) {
    await post(projectURL(p.id, "archive"), { archived: on });
    await refresh();
  }
</script>

{#snippet card(p)}
  {@const [tone, text] = status(p)}
  {@const n = (p.questions || []).length}
  {@const ended = p.template === "single" && p.stage === "ended"}
  <article class="card project-card" class:asking={n > 0} class:finished={ended || p.archived}>
    <div class="pc-head">
      <div class="pc-title"><a class="pname" href={P(p.id)}>{p.name}</a>
        <div class="small muted">{kindOf(p.template).label}{#if p.agent}{" · "}{p.agent.name}{/if}{" · "}<Ago iso={p.updated_at} /></div></div>
      <Badge s={tone} {text} />
    </div>
    <div class="pc-body">
      {#if p.where}<div class="chip path" title={p.where}>{p.where}</div>{/if}
      {#if p.error}<div class="small err">{p.error}</div>{/if}
      {#if p.blocked}<div class="small" style="color:var(--danger)">{p.blocked}</div>{/if}
      {#if p.attention}<div class="small" style="color:var(--warning)">{p.attention.text}</div>{/if}
      {#if p.moved_to}<div class="small muted">Continued as <a href={P(p.moved_to)}>a software project</a>.</div>{/if}
    </div>
    <div class="pc-foot">
      <a class="btn sm" class:primary={n > 0} href={P(p.id, n ? "questions" : p.template === "single" ? "chat" : "overview")}>{n ? `Answer ${n === 1 ? "1 question" : `${n} questions`}` : "Open"}</a>
      {#if p.archived}<button type="button" class="btn sm ghost" onclick={() => archive(p, false)}>Restore</button>
      {:else if ended || p.error || !p.active}<button type="button" class="btn sm ghost" onclick={() => archive(p, true)}>Archive</button>{/if}
    </div>
  </article>
{/snippet}

{#snippet row(p)}
  {@const [tone, text] = status(p)}
  <div class="row inactive-row">
    <a class="pname" href={P(p.id, p.template === "single" ? "chat" : "overview")}>{p.name}</a>
    <span class="small muted">{kindOf(p.template).label} · <Ago iso={p.last_seen || p.updated_at} /></span>
    {#if p.where}<span class="chip path" title={p.where}>{p.where}</span>{/if}
    {#if p.moved_to}<a class="small" href={P(p.moved_to)}>continued as a software project</a>{/if}
    <span class="row-end"><Badge s={tone} {text} />
      {#if p.archived}<button type="button" class="btn sm ghost" onclick={() => archive(p, false)}>Restore</button>
      {:else}<button type="button" class="btn sm ghost" onclick={() => archive(p, true)}>Archive</button>{/if}</span>
  </div>
{/snippet}

<h1>Projects</h1>
<p class="lede">Your projects, and the agent sessions you added. A session joins when you ask its agent to track it (<code>lacarta track</code>), or when it starts in a tracked folder.</p>
{#if asks.length || terminal.length}
  <h2>Waiting for you · {asks.length + terminal.length}</h2>
  <div class="card list">
    {#each asks as { p, q } (p.id + q.id)}
      <div class="row"><span class="chip">{q.id}</span>{#if q.triage}<Badge s="fail" text="Security decision" />{/if}<a href={P(p.id, "questions")}>{q.title}</a><span class="small muted">{q.asked_by} · {p.name}</span></div>
    {/each}
    {#each terminal as p (p.id)}
      <div class="row"><Badge s="open" text="Terminal" /><a href={P(p.id)}>{p.attention.text}</a><span class="small muted">{p.name}</span></div>
    {/each}
  </div>
{/if}
<h2>Active · {active.length}</h2>
{#if active.length}
  <div class="projects">{#each active as p (p.id)}{@render card(p)}{/each}</div>
{:else if !live.length}
  <div class="card empty">No sessions yet. Run <code>lacarta install</code> once, then ask an agent to "track this session" (it runs <code>lacarta track</code>), or start one in a lacarta project.</div>
{:else}
  <div class="card empty">Nothing is running. Sessions you start appear here.</div>
{/if}
{#if inactive.length}
  <div class="row section-head"><h2>Inactive · {inactive.length}</h2>
    <button type="button" class="btn sm" disabled={busy} onclick={() => archiveAll(inactive.filter(p => p.template === "single"))}
      title="Archive every inactive single-agent task; projects stay">Archive inactive tasks</button></div>
  <p class="small muted" style="margin:0 0 8px">Ended or idle sessions (nothing written for 12 hours), and finished projects.</p>
  <div class="card list">
    {#each ended as p (p.id)}{@render row(p)}{/each}
    {#if empty.length}
      <div class="row inactive-row"><span class="muted">{empty.length} empty {empty.length === 1 ? "session" : "sessions"}: ended or idle before their first prompt</span>
        <span class="row-end"><button type="button" class="btn sm ghost" onclick={() => { ui.showEmpty = !ui.showEmpty; }}>{ui.showEmpty ? "Hide" : "Show"}</button>
          <button type="button" class="btn sm ghost" disabled={busy} onclick={() => archiveAll(empty)}>Archive them</button></span></div>
      {#if ui.showEmpty}{#each empty as p (p.id)}{@render row(p)}{/each}{/if}
    {/if}
  </div>
{/if}
{#if archived.length}
  <div class="row" style="margin-top:24px"><button type="button" class="btn sm" onclick={() => { ui.showArchived = !ui.showArchived; }}>{ui.showArchived ? "Hide" : "Show"} archived · {archived.length}</button></div>
  {#if ui.showArchived}<div class="card list" style="margin-top:12px">{#each archived as p (p.id)}{@render row(p)}{/each}</div>{/if}
{/if}
