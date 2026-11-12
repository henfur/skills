<script>
  import Badge from "./Badge.svelte";
  import AgentChip from "./AgentChip.svelte";
  import Ago from "./Ago.svelte";
  import Md from "./Md.svelte";
  import { app, agentById } from "../lib/state.svelte.js";
  import { label } from "../lib/format.js";
  let { r } = $props();
  const report = $derived(app.data.reports?.[r.id]);
</script>

<div>
  <div class="row"><span class="chip">{r.id}</span><strong>{label(r.type)} review</strong><Badge s={r.verdict} text={r.verdict === "pass" ? "Passed" : "Failed"} />{#if r.agent && agentById(r.agent)}<AgentChip id={r.agent} />{:else}<span class="small muted">{r.reviewer}</span>{/if}<span class="small muted"><Ago iso={r.at} /></span></div>
  <div class="small" style="margin-top:4px">{r.summary}</div>
  {#if report}<details style="margin-top:6px"><summary class="small">Read report</summary><Md text={report} cls="wide" /></details>{/if}
</div>
