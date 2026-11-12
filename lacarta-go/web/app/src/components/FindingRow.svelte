<script>
  import Badge from "./Badge.svelte";
  import SevBadge from "./SevBadge.svelte";
  import AgentChip from "./AgentChip.svelte";
  import { label } from "../lib/format.js";
  let { f } = $props();
</script>

<div>
  <div class="row"><span class="chip">{f.id}</span><SevBadge s={f.severity} /><Badge s={f.status} text={label(f.status)} /><span>{f.title}</span>{#if f.found_by}<AgentChip id={f.found_by} withRole={false} />{/if}</div>
  {#if f.location}<div class="small muted" style="margin-top:4px"><code>{f.location}</code></div>{/if}
  {#if f.resolution}<div class="small" style="margin-top:4px"><span class="muted">Resolution:</span> {f.resolution}</div>{/if}
</div>
