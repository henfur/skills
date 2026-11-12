<script>
  import { avatarTone, initials, avatarPicture } from "../lib/format.js";
  import { portrait } from "../lib/portraits.js";
  import Portrait from "./Portrait.svelte";
  let { a, size = "" } = $props();
  const pic = $derived(avatarPicture(a));
  const drawn = $derived(pic ? null : portrait(a?.name));
</script>

{#if a}
  <span class="avatar {size} t-{avatarTone(a.id)}" class:drawn role="img" aria-label={a.name} title={a.name}>
    {#if pic}<img src={pic} alt="" />{:else if drawn}<Portrait p={drawn} />{:else}{initials(a.name)}{/if}
  </span>
{/if}
