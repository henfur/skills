<script>
  import Avatar from "./Avatar.svelte";
  import Ago from "./Ago.svelte";
  import Md from "./Md.svelte";
  import { agentById, replier } from "../lib/state.svelte.js";
  let { q } = $props();
  const r = $derived(replier(q));
</script>

{#if q.thread?.length}
  <section class="thread" aria-label="Conversation about {q.id}">
    {#each q.thread as m, i (i)}
      {@const a = m.by === "user" ? null : agentById(m.by)}
      <div class="msg" class:user={m.by === "user"}>{#if a}<Avatar {a} />{/if}<div class="bubble">
        <div class="meta">{m.by === "user" ? "You" : a ? a.name : m.by} · <Ago iso={m.at} /></div><Md text={m.text} /></div></div>
    {/each}
    {#if q.thread[q.thread.length - 1].by === "user"}<div class="small muted">Waiting for {r ? r.name : "the agent"} to reply…</div>{/if}
  </section>
{/if}
