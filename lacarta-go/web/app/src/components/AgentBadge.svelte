<script>
  import Badge from "./Badge.svelte";
  import { agentStatus } from "../lib/format.js";
  let { status } = $props();
  const [tone, text] = $derived(agentStatus(status));
</script>

<Badge s={tone} {text} />
