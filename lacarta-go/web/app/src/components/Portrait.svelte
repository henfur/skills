<script>
  // A drawn agent portrait (see lib/portraits.js); fills the avatar disc.
  let { p } = $props();
  const top = {
    short: "M11.5 17 C11 9 16 7.5 20 7.5 C25 7.5 29 9.5 28.5 17 C27 13 24 12 20 12 C16 12 13 13 11.5 17Z",
    bob: "M11 25 C9 14 13 8 20 8 C27 8 31 14 29 25 L27 25 C27.5 18 26 13 20 12.5 C14 13 12.5 18 13 25Z",
    receding: "M11.8 19 C11 12 14 9.5 20 9.5 C26 9.5 29 12 28.2 19 C27.5 15.5 27 14 26 13.5 C24 12 16 12 14 13.5 C13 14 12.5 15.5 11.8 19Z",
  };
  const hairTop = $derived(top[p.hairStyle] || top.short);
</script>

<svg viewBox="0 0 40 40" aria-hidden="true">
  <rect width="40" height="40" fill={p.bg} />
  {#if p.robot}
    <path d="M8 40 C8 33 13 31 20 31 C27 31 32 33 32 40Z" fill={p.outfit} />
    <line x1="20" y1="9" x2="20" y2="5" stroke={p.outfit} stroke-width="1.5" />
    <circle cx="20" cy="4.5" r="1.8" fill="#22d3ee" />
    <rect x="9" y="9" width="22" height="19" rx="5" fill={p.outfit} />
    <rect x="12" y="12" width="16" height="12" rx="3" fill="#ecfeff" />
    <circle cx="16.5" cy="17.5" r="1.8" fill="#06b6d4" /><circle cx="23.5" cy="17.5" r="1.8" fill="#06b6d4" />
    <path d="M17 21.5 H23" stroke="#0e7490" stroke-width="1.2" stroke-linecap="round" />
  {:else}
    {#if p.hairStyle === "long"}<path d="M9 19 A11 11 0 0 1 31 19 V34 H9Z" fill={p.hair} />{/if}
    <path d="M6 40 C6 31 12 28 20 28 C28 28 34 31 34 40Z" fill={p.outfit} />
    {#if p.tie}<path d="M16.5 28.4 L20 33 L23.5 28.4Z" fill="#f8fafc" /><path d="M20 30.5 L18.6 32.2 L20 39 L21.4 32.2Z" fill={p.tie} />{/if}
    <rect x="17" y="23" width="6" height="6" fill={p.skin} />
    <ellipse cx="20" cy="18.5" rx="8" ry="9" fill={p.skin} />
    {#if p.hairStyle === "bun"}<circle cx="20" cy="7" r="4" fill={p.hair} />{/if}
    <path d={p.hairStyle === "long" ? top.bob : hairTop} fill={p.hair} />
    <circle cx="17" cy="18.5" r="1" fill="#1f2937" /><circle cx="23" cy="18.5" r="1" fill="#1f2937" />
    {#if p.glasses}<g fill="none" stroke="#1f2937" stroke-width="0.8"><circle cx="17" cy="18.5" r="2.4" /><circle cx="23" cy="18.5" r="2.4" /><path d="M19.4 18.5 H20.6" /></g>{/if}
    <path d="M17.6 22.6 Q20 24.3 22.4 22.6" fill="none" stroke="#7c2d12" stroke-width="0.9" stroke-linecap="round" />
  {/if}
</svg>
