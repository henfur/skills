<script>
  import Icon from "./Icon.svelte";
  import { agentById, L, P } from "../lib/state.svelte.js";
  // Session state shared by both overviews and the chat: terminal prompts, listening, moved.
  let { st, d } = $props();
  const a = $derived(agentById("A-1"));
</script>

{#if st.attention}
  <div class="banner warn" role="status"><Icon name="warn" /><div><strong>{a.name} needs you in the terminal.</strong> {st.attention.text}</div></div>
{:else if st.moved_to}
  <div class="banner info"><Icon name="info" /><div>This session continued as <a href={P(st.moved_to)}>a software project</a>.</div></div>
{:else if d.listening}
  <div class="banner info"><Icon name="info" /><div><strong>{a.name} finished its turn and is listening.</strong> Write to it in <a href={L("chat")}>Chat</a>.</div></div>
{/if}
