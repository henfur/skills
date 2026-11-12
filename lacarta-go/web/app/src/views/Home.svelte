<script>
  import Badge from "../components/Badge.svelte";
  import Ago from "../components/Ago.svelte";
  import { app, ui, P, kindOf, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  import { agentStatus, label } from "../lib/format.js";
  const ps = $derived([...app.hub.projects].sort((a, b) => String(b.updated_at).localeCompare(String(a.updated_at))));
  const live = $derived(ps.filter(p => !p.archived));
  const archived = $derived(ps.filter(p => p.archived));
  const asks = $derived(live.flatMap(p => (p.questions || []).map(q => ({ p, q }))));
  const terminal = $derived(live.filter(p => p.attention));
  const status = p => p.error ? ["fail", "Unreadable"]
    : p.template === "single" && p.stage === "ended" ? ["done", p.moved_to ? "Moved" : "Ended"]
    : p.status ? agentStatus(p.status) : ["draft", label("idle")];
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
    <div class="row" style="justify-content:space-between"><a class="pname" href={P(p.id)}>{p.name}</a><Badge s={tone} {text} /></div>
    <div class="small muted">{kindOf(p.template).label}{#if p.agent} · {p.agent.name}{/if} · <Ago iso={p.updated_at} /></div>
    {#if p.where}<div class="chip path" title={p.where}>{p.where}</div>{/if}
    {#if p.error}<div class="small err">{p.error}</div>{/if}
    {#if p.blocked}<div class="small" style="color:var(--danger)">{p.blocked}</div>{/if}
    {#if p.attention}<div class="small" style="color:var(--warning)">{p.attention.text}</div>{/if}
    {#if p.moved_to}<div class="small muted">Continued as <a href={P(p.moved_to)}>a software project</a>.</div>{/if}
    <div class="row" style="margin-top:auto;padding-top:8px">
      <a class="btn sm" class:primary={n > 0} href={P(p.id, n ? "questions" : p.template === "single" ? "chat" : "overview")}>{n ? `Answer ${n === 1 ? "1 question" : `${n} questions`}` : "Open"}</a>
      {#if p.archived}<button type="button" class="btn sm" onclick={() => archive(p, false)}>Restore</button>
      {:else if ended || p.error}<button type="button" class="btn sm" onclick={() => archive(p, true)}>Archive</button>{/if}
    </div>
  </article>
{/snippet}

<h1>Projects</h1>
<p class="lede">Every agent session and project on this machine. Each Claude Code session registers here on its own.</p>
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
<h2>Active · {live.length}</h2>
{#if live.length}
  <div class="projects">{#each live as p (p.id)}{@render card(p)}{/each}</div>
{:else}
  <div class="card empty">No sessions yet. Run <code>lacarta install</code> once; every Claude Code session started afterwards appears here.</div>
{/if}
{#if archived.length}
  <div class="row" style="margin-top:24px"><button type="button" class="btn sm" onclick={() => { ui.showArchived = !ui.showArchived; }}>{ui.showArchived ? "Hide" : "Show"} archived · {archived.length}</button></div>
  {#if ui.showArchived}<div class="projects" style="margin-top:12px">{#each archived as p (p.id)}{@render card(p)}{/each}</div>{/if}
{/if}
