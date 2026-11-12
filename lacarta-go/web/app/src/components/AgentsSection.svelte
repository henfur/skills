<script>
  import Avatar from "./Avatar.svelte";
  import AgentBadge from "./AgentBadge.svelte";
  import Ago from "./Ago.svelte";
  import { app, askerId, roleLabel, modelName, L } from "../lib/state.svelte.js";
  import { freshQs } from "../lib/notify.svelte.js";
  import { AGENT_GROUPS, STATUS_ORDER, agentStatus, phaseName } from "../lib/format.js";
  let { st, d } = $props();
  const status = a => d.agent_status[a.id] || a.status;
  const known = new Set(AGENT_GROUPS.flatMap(g => g[1]));
  const groups = $derived([...AGENT_GROUPS, ["Other", null]].map(([name, roles]) => [name,
    st.agents.filter(a => roles ? roles.includes(a.role) : !known.has(a.role))
      .sort((x, y) => STATUS_ORDER.indexOf(status(x)) - STATUS_ORDER.indexOf(status(y))
        || String(y.updated_at || "").localeCompare(String(x.updated_at || "")))]).filter(g => g[1].length));
  const active = $derived(st.agents.filter(a => !["done", "failed"].includes(status(a))).length);
  const asking = a => st.questions.filter(q => q.status === "open" && askerId(q) === a.id);
  const tally = list => {
    const n = {};
    list.forEach(a => { const s = status(a); n[s] = (n[s] || 0) + 1; });
    return STATUS_ORDER.filter(s => n[s]).map(s => `${n[s]} ${agentStatus(s)[1].toLowerCase()}`);
  };
</script>

{#if st.agents.length}
  <h2>Agents · {active} active of {st.agents.length}</h2>
  {#each groups as [name, list] (name)}
    <section class="agent-group" aria-label={name}>
      <div class="row group-head"><span class="overline">{name}</span>
        {#each tally(list) as t, i (t)}{#if i}<span class="small muted" aria-hidden="true">·</span>{/if}<span class="small muted">{t}</span>{/each}
      </div>
      <div class="agents">
        {#each list as a (a.id)}
          {@const qs = asking(a)}
          {@const s = status(a)}
          <article class="card agent-card" class:finished={["done", "failed"].includes(s)} class:asking={qs.length > 0}
            class:fresh={qs.some(q => freshQs.has(`${app.data.id}/${q.id}`))} aria-label="{a.name}, {roleLabel(a)}">
            <span class="avatar-wrap"><Avatar {a} size="md" />{#if qs.length}<span class="ping" aria-hidden="true"></span>{/if}</span>
            <div style="min-width:0">
              <div class="row" style="justify-content:space-between"><span class="name">{a.name}</span><AgentBadge status={s} /></div>
              <div class="small muted">{roleLabel(a)} · {modelName(a.model)}{#if a.phase != null} · {phaseName(a.phase)}{/if}</div>
              <div class="task small">{a.summary || a.task}</div>
              <div class="small muted" style="margin-top:6px">{#if a.finished_at}Finished <Ago iso={a.finished_at} />{:else}Active <Ago iso={a.updated_at} />{/if}</div>
            </div>
            {#if qs.length}
              <div class="ask-strip">
                <div class="small"><strong>{qs.length === 1 ? "Needs your answer" : `${qs.length} questions for you`}</strong></div>
                {#each qs.slice(0, 2) as q (q.id)}
                  <div class="small"><span class="chip">{q.id}</span> {q.title}{#if q.thread?.length && q.thread[q.thread.length - 1].by === "user"} <span class="muted">· replying to your message</span>{/if}</div>
                {/each}
                <a class="btn sm primary" href={L("questions")}>Answer</a>
              </div>
            {/if}
          </article>
        {/each}
      </div>
    </section>
  {/each}
{/if}
