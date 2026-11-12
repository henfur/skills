<script>
  import Avatar from "./Avatar.svelte";
  import Ago from "./Ago.svelte";
  import { app, agentById, askerId, roleLabel } from "../lib/state.svelte.js";
  let { q } = $props();
  const a = $derived(agentById(askerId(q)));
  const finished = $derived(a && ["done", "failed"].includes(app.data.derived.agent_status[a.id]));
  const orchestrator = $derived(agentById("A-1"));
</script>

{#if !a}
  <span class="small muted">Asked <Ago iso={q.asked_at} /></span>
{:else}
  <div class="asker"><Avatar {a} size="md" /><div>
    <div><span class="who">{a.name}</span> <span class="small muted">asks · {roleLabel(a)} · <Ago iso={q.asked_at} /></span></div>
    {#if finished}<div class="small muted">{a.name} has finished; {orchestrator ? orchestrator.name : "the orchestrator"} acts on your answer.</div>{/if}
  </div></div>
{/if}
