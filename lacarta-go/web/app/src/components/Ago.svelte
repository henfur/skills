<script>
  import { agoText } from "../lib/format.js";
  import { app } from "../lib/state.svelte.js";
  let { iso } = $props();
</script>

{#if iso}<span title={iso}>{agoText(iso, app.now)}</span>{/if}
