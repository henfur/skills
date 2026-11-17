<script>
  import { app, P, kindOf } from "../lib/state.svelte.js";
  import { STATUS_DOT } from "../lib/format.js";
  const all = $derived(app.hub.projects.filter(p => !p.archived));
  // Only active projects, plus the one open, so the list stays short; Home lists the rest.
  const live = $derived(all.filter(p => p.active || p.error || p.id === app.route.pid)
    .sort((a, b) => String(b.last_seen || b.updated_at).localeCompare(String(a.last_seen || a.updated_at))));
  const hidden = $derived(all.length - live.length);
  const waiting = $derived(live.reduce((n, p) => n + (p.questions || []).length, 0));
  const r = $derived(app.route);
</script>

<nav class="side" aria-label="Projects">
  <a href="#/" aria-current={r.page === "home" ? "page" : undefined}><span>All projects</span>{#if waiting}<span class="count hot">{waiting}</span>{/if}</a>
  <a href="#/settings" aria-current={r.page === "settings" ? "page" : undefined}><span>Settings</span></a>
  <div class="nav-head overline">Active · {live.length}</div>
  {#each live as p (p.id)}
    {@const n = (p.questions || []).length}
    <a href={P(p.id)} aria-current={r.pid === p.id ? "page" : undefined}><span class="nav-project"><span class="dot {STATUS_DOT[p.status] || ''}" aria-hidden="true"></span><span class="nav-name">{p.name}</span><span class="nav-kind">{kindOf(p.template).label}</span></span>{#if n}<span class="count hot">{n}</span>{/if}</a>
  {/each}
  {#if hidden}<a href="#/" class="nav-more"><span class="small muted">{hidden} inactive: see All projects</span></a>{/if}
</nav>
