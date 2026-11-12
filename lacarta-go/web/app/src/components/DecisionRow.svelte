<script>
  import ByBadge from "./ByBadge.svelte";
  import AnsweredTo from "./AnsweredTo.svelte";
  import Ago from "./Ago.svelte";
  let { x } = $props();
</script>

<div>
  <div class="row"><span class="chip">{x.id}</span><ByBadge by={x.by} agent={x.agent} /><AnsweredTo {x} /></div>
  <div style="margin-top:4px"><strong>{x.title}</strong></div>
  <div class="small">{x.decision}</div>
  {#if x.rationale}<div class="small muted">{x.rationale}</div>{/if}
  <div class="small muted">{#if x.source}{x.source} · {/if}<Ago iso={x.at} /></div>
</div>
