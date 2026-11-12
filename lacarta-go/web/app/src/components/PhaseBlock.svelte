<script>
  import ReviewRow from "./ReviewRow.svelte";
  import FindingRow from "./FindingRow.svelte";
  let { st, pid, children } = $props();
  const revs = $derived(st.reviews.filter(r => r.phase === pid).reverse());
  const fs = $derived(st.findings.filter(f => f.phase === pid));
</script>

<article class="card stack">
  {@render children()}
  <div class="overline">Reviews · {revs.length}</div>
  <div class="list">{#each revs as r (r.id)}<ReviewRow {r} />{:else}<div class="small muted">No review recorded yet.</div>{/each}</div>
  {#if fs.length}<div class="overline">Findings · {fs.length}</div><div class="list">{#each fs as f (f.id)}<FindingRow {f} />{/each}</div>{/if}
</article>
