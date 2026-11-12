<script>
  import SessionBanner from "../components/SessionBanner.svelte";
  import AgentsSection from "../components/AgentsSection.svelte";
  import ActivityRows from "../components/ActivityRows.svelte";
  import ChatBubble from "../components/ChatBubble.svelte";
  import KindBadge from "../components/KindBadge.svelte";
  import Ago from "../components/Ago.svelte";
  import { app, L } from "../lib/state.svelte.js";
  let { st, d } = $props();
  const openQs = $derived(st.questions.filter(q => q.status === "open"));
  const recent = $derived((app.data.chat || []).filter(m => m.role !== "tool").slice(-3));
</script>

<h1>{st.project.name}</h1>
<p class="lede">Single-agent task in <code>{st.project.cwd || ""}</code></p>
<SessionBanner {st} {d} />
<AgentsSection {st} {d} />
{#if openQs.length}
  <h2>Waiting for your answer · {openQs.length}</h2>
  <div class="card list">{#each openQs as q (q.id)}<div class="row"><span class="chip">{q.id}</span><KindBadge {q} /><a href={L("questions")}>{q.title}</a><span class="small muted"><Ago iso={q.asked_at} /></span></div>{/each}</div>
{/if}
<h2>Latest in the chat</h2>
<div class="card">
  {#if recent.length}<div class="stack">{#each recent as m, i (i)}<ChatBubble {m} />{/each}</div>{:else}<div class="small muted">Nothing yet.</div>{/if}
  <div style="margin-top:12px"><a class="btn sm" href={L("chat")}>Open chat</a></div>
</div>
<h2>Recent activity</h2>
<div class="card list"><ActivityRows items={st.activity.slice(-8).reverse()} /></div>
