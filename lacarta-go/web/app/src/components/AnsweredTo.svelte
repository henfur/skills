<script>
  import AgentChip from "./AgentChip.svelte";
  import { app, askerId } from "../lib/state.svelte.js";
  let { x } = $props();
  const q = $derived(x.by === "agent" || !x.source ? null : app.data.state.questions.find(q => q.id === x.source));
</script>

{#if q}<span class="small muted">answering <AgentChip id={askerId(q)} withRole={false} /></span>{/if}
