<script>
  import { app, P, kindOf } from "../lib/state.svelte.js";
  import { STATUS_DOT } from "../lib/format.js";
  const live = $derived(app.hub.projects.filter(p => !p.archived).sort((a, b) => String(b.updated_at).localeCompare(String(a.updated_at))));
  const waiting = $derived(live.reduce((n, p) => n + (p.questions || []).length, 0));
  const r = $derived(app.route);
</script>

<nav class="side" aria-label="Projects">
  <a href="#/" aria-current={r.page === "home" ? "page" : undefined}><span>All projects</span>{#if waiting}<span class="count hot">{waiting}</span>{/if}</a>
  <a href="#/settings" aria-current={r.page === "settings" ? "page" : undefined}><span>Settings</span></a>
  <div class="nav-head overline">Projects · {live.length}</div>
  {#each live as p (p.id)}
    {@const n = (p.questions || []).length}
    <a href={P(p.id)} aria-current={r.pid === p.id ? "page" : undefined}><span class="nav-project"><span class="dot {STATUS_DOT[p.status] || ''}" aria-hidden="true"></span><span class="nav-name">{p.name}</span><span class="nav-kind">{kindOf(p.template).label}</span></span>{#if n}<span class="count hot">{n}</span>{/if}</a>
  {/each}
</nav>
