<script>
  import Avatar from "./Avatar.svelte";
  import { agentById, roleLabel } from "../lib/state.svelte.js";
  let { id, withRole = true } = $props();
  const a = $derived(agentById(id));
</script>

{#if a}<span class="agent-chip"><Avatar {a} />{a.name}{#if withRole}{" "}<span class="role">{roleLabel(a)}</span>{/if}</span>{/if}
