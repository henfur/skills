<script>
  import ActivityRows from "../components/ActivityRows.svelte";
  let { st } = $props();
</script>

<h1>Activity</h1>
<p class="lede">{st.activity.length} events, newest first.</p>
<div class="card list"><ActivityRows items={[...st.activity].reverse()} /></div>
