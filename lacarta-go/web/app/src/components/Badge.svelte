<script>
  import { TONE, label } from "../lib/format.js";
  let { s, text = undefined } = $props();
</script>

<span class="badge {TONE[s] ?? ''}">{text ?? label(s)}</span>
