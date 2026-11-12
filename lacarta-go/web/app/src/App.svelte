<script>
  import Nav from "./components/Nav.svelte";
  import Toasts from "./components/Toasts.svelte";
  import Badge from "./components/Badge.svelte";
  import Home from "./views/Home.svelte";
  import Settings from "./views/Settings.svelte";
  import Project from "./views/Project.svelte";
  import { app, ui, kindOf } from "./lib/state.svelte.js";
  import { themeName } from "./lib/diagrams.js";
  import { label } from "./lib/format.js";

  const r = $derived(app.route);
  const open = $derived(r.page === "project" && app.data && app.data.id === r.pid ? app.data : null);
  const known = $derived(r.page === "project" ? app.hub?.projects.find(p => p.id === r.pid) : null);
  const waiting = $derived((app.hub?.projects || []).reduce((n, p) => n + (p.archived ? 0 : (p.questions || []).length), 0));

  $effect(() => {
    document.title = `${waiting ? `(${waiting}) ` : ""}${open ? open.state.project.name + ": " : ""}Lacarta console`;
  });

  // System notifications need a secure context (localhost is one) and the user's permission.
  let permission = $state("Notification" in window ? Notification.permission : "unsupported");
  const canAsk = $derived(window.isSecureContext && permission === "default");
  async function askPermission() {
    try { await Notification.requestPermission(); } catch (e) {}
    permission = Notification.permission;
  }

  ui.theme = themeName();
  function toggleTheme() {
    const root = document.documentElement;
    root.dataset.theme = themeName() === "dark" ? "light" : "dark";
    try { localStorage.setItem("lacarta-theme", root.dataset.theme); } catch (e) {}
    ui.theme = themeName();  // diagrams are drawn with the theme's colours
  }
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => { ui.theme = themeName(); });
</script>

<header class="appbar">
  <a class="name" href="#/"><b>Lacarta</b>{#if open} · {open.state.project.name}{/if}</a>
  <span>
    {#if open}
      {#if open.derived.blocked}<Badge s="stopped" text="Blocked" />
      {:else if open.state.template === "single"}<Badge s="draft" text={kindOf("single").label} />
      {:else}<Badge s={open.state.stage} text={label(open.state.stage)} />{/if}
    {/if}
  </span>
  <span class="spacer"></span>
  <span class="live" class:off={app.live.ok === false} role="status">
    {#if app.live.ok === null}Connecting…{:else if app.live.ok}<span class="long">Live · </span>updated {app.live.at.toLocaleTimeString()}{:else}Disconnected: is `lacarta serve` running?{/if}
  </span>
  {#if canAsk}<button class="btn sm" type="button" onclick={askPermission}>Enable alerts</button>{/if}
  <button class="btn sm" type="button" aria-label="Switch theme" onclick={toggleTheme}>Theme</button>
</header>
<Toasts />
<div class="layout">
  {#if app.hub}<Nav />{:else}<nav class="side" aria-label="Projects"></nav>{/if}
  <main><div class="content">
    {#if !app.hub}
      <p class="muted">Loading…</p>
    {:else if r.page === "settings"}
      <Settings />
    {:else if open}
      <Project />
    {:else if r.page === "project"}
      {#if known?.error}<div class="card empty">{known.error}</div>
      {:else if known}<p class="muted">Loading {known.name}…</p>
      {:else}<div class="card empty">No project {r.pid}. <a href="#/">All projects</a></div>{/if}
    {:else}
      <Home />
    {/if}
  </div></main>
</div>
