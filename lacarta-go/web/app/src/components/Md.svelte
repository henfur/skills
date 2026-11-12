<script>
  import { md, mdInline } from "../lib/format.js";
  import { renderDiagrams } from "../lib/diagrams.js";
  import { ui } from "../lib/state.svelte.js";
  // Markdown from agents, sanitised; ```mermaid blocks become diagrams in the current theme.
  let { text = "", cls = "", inline = false } = $props();
  let el = $state();
  const html = $derived(inline ? mdInline(text) : md(text));
  $effect(() => {
    void ui.theme;
    if (!el) return;
    el.innerHTML = html;
    if (!inline) renderDiagrams(el);
  });
</script>

{#if inline}<span bind:this={el}></span>{:else}<div class="md {cls}" bind:this={el}></div>{/if}
