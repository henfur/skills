<script>
  import { sevTone, label } from "../lib/format.js";
  let { s } = $props();
</script>

<span class="badge {sevTone(s)}">{label(s)}</span>
