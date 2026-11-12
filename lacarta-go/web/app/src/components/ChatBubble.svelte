<script>
  import Avatar from "./Avatar.svelte";
  import Ago from "./Ago.svelte";
  import Md from "./Md.svelte";
  import { agentById } from "../lib/state.svelte.js";
  let { m } = $props();
  const a = $derived(agentById("A-1"));
</script>

{#if m.role === "agent"}
  <div class="msg"><Avatar {a} /><div class="bubble"><div class="meta">{a ? a.name : "Agent"} · <Ago iso={m.at} /></div><Md text={m.text} /></div></div>
{:else}
  <div class="msg user"><div class="bubble"><div class="meta">{m.role === "console" ? "You · console" : "You · terminal"} · <Ago iso={m.at} />
    {#if m.role === "console"}{#if m.delivered_at} · delivered <Ago iso={m.delivered_at} />{:else} · <strong>queued</strong>{/if}{/if}</div>
    <Md text={m.text} /></div></div>
{/if}
