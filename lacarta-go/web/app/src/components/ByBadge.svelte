<script>
  import Badge from "./Badge.svelte";
  import AgentChip from "./AgentChip.svelte";
  import { agentById } from "../lib/state.svelte.js";
  let { by, agent = null } = $props();
</script>

{#if by === "agent"}
  {#if agent && agentById(agent)}<AgentChip id={agent} withRole={false} />{:else}<Badge s="draft" text="Agent" />{/if}
{:else}
  <Badge s="approved" text={by === "user" ? "You" : "You (terminal)"} />
{/if}
