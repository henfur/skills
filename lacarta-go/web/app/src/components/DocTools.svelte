<script>
  import { copyText, downloadText, printElement } from "../lib/doc.js";
  // Copy, download or print a Markdown document. `target` is the element holding its rendered
  // text, printed alone; without one, there is no print button.
  let { text = "", filename = "report.md", target = null, label = "" } = $props();
  let note = $state("");
  async function copy() {
    note = (await copyText(text)) ? "Copied." : "Copy failed: select the text instead.";
    setTimeout(() => { note = ""; }, 2500);
  }
</script>

<div class="doc-tools" role="group" aria-label={label ? `Export ${label}` : "Export"}>
  <button type="button" class="btn sm ghost" disabled={!text} onclick={copy}>Copy Markdown</button>
  <button type="button" class="btn sm ghost" disabled={!text} onclick={() => downloadText(text, filename)}>Download .md</button>
  {#if target}<button type="button" class="btn sm ghost" onclick={() => printElement(target, filename.replace(/\.md$/, ""))}>Print / PDF</button>{/if}
  <span class="small muted" role="status">{note}</span>
</div>
