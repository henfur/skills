<script>
  import Badge from "./Badge.svelte";
  let { q } = $props();
</script>

{#if q.effect?.type === "finding-triage"}<Badge s="fail" text="Security decision" />
{:else if q.kind === "validation"}<Badge s="awaiting-approval" text="Validation" />
{:else if q.kind === "decision"}<Badge s="open" text="Decision" />
{:else}<Badge s="in-review" text="Question" />{/if}
