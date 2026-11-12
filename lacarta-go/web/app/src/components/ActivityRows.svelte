<script>
  import Avatar from "./Avatar.svelte";
  import Ago from "./Ago.svelte";
  import { agentById } from "../lib/state.svelte.js";
  import { label } from "../lib/format.js";
  let { items } = $props();
</script>

{#each items as a, i (i)}
  <div class="row">{#if a.agent && agentById(a.agent)}<Avatar a={agentById(a.agent)} />{/if}<span class="badge plain">{label(a.kind)}</span><span>{a.text}</span><span class="small muted" style="margin-left:auto"><Ago iso={a.at} /></span></div>
{:else}
  <div class="small muted">Nothing yet.</div>
{/each}
