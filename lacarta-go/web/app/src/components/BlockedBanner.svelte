<script>
  import Icon from "./Icon.svelte";
  import { L } from "../lib/state.svelte.js";
  let { d } = $props();
</script>

{#if d.blocked}
  <div class="banner danger" role="alert"><Icon name="alert" /><div><strong>Development is stopped.</strong> {d.blocked}<br /><span class="small">Answer the decision under <a href={L("questions")}>Questions</a> to unblock it.</span></div></div>
{/if}
