import { getText, TOKEN } from "./api.js";
import { notifyChanges } from "./notify.svelte.js";

// `hub`: every project on this machine plus settings; `data`: the open project's full state.
export const app = $state({
  hub: null,
  data: null,
  route: parseRoute(),
  live: { ok: null, at: null },  // null: connecting
  now: Date.now(),               // re-renders the "3 min ago" labels
});

// #/ home, #/settings, #/p/<project>/<view>
export function parseRoute() {
  const parts = location.hash.replace(/^#\/?/, "").split("/").map(decodeURIComponent);
  if (parts[0] === "settings") return { page: "settings" };
  if (parts[0] === "p" && parts[1]) return { page: "project", pid: parts[1], view: parts[2] || "overview" };
  return { page: "home" };
}
export const P = (pid, view = "overview") => `#/p/${encodeURIComponent(pid)}/${view}`;
export const L = view => P(app.data ? app.data.id : "", view);   // a link inside the open project

let lastHub = "", lastProject = "";
let inflight = null, again = false;

// refresh fetches the hub and the open project; only what changed is replaced, so the page
// keeps its DOM (and whatever is being typed) when nothing moved.
export function refresh() {
  if (inflight) { again = true; return inflight; }
  inflight = (async () => {
    do {
      again = false;
      const r = app.route;
      try {
        const h = await getText("/api/hub");
        const hub = h.ok ? JSON.parse(h.text) : app.hub;
        let p = { ok: true, text: "" };
        // Only projects the hub lists have a state to fetch (an unknown id shows "No project").
        if (r.page === "project" && hub?.projects.some(x => x.id === r.pid)) {
          p = await getText(`/api/p/${encodeURIComponent(r.pid)}/state`);
        }
        app.live = { ok: true, at: new Date() };
        if (h.ok && h.text !== lastHub) {
          lastHub = h.text;
          app.hub = hub;
          notifyChanges(app.hub, app.route);
        }
        const ptext = p.ok ? p.text : "";
        if (ptext !== lastProject || (app.data && r.page !== "project")) {
          lastProject = ptext;
          const data = ptext ? JSON.parse(ptext) : null;
          for (const q of data?.state.questions || []) {
            const k = `${data.id}/${q.id}`;
            if (q.status === "open" && !ui.drafts[k]) ui.drafts[k] = emptyDraft();
          }
          app.data = data;
        }
      } catch (e) {
        app.live = { ok: false, at: new Date() };
      }
    } while (again);
    inflight = null;
  })();
  return inflight;
}

// Live updates: the daemon announces changes over server-sent events; polling stays as the
// fallback (every 2.5 s, as the prototype did, slowed to 15 s while the stream is healthy).
let source = null, streamOK = false;

function connect() {
  if (!("EventSource" in window)) return;
  source = new EventSource(`/api/events?key=${encodeURIComponent(TOKEN)}`);
  source.onopen = () => { streamOK = true; };
  const onChange = () => refresh();
  source.addEventListener("hub", onChange);
  source.addEventListener("project", e => {
    if (app.route.page === "project" && e.data === app.route.pid) refresh();
  });
  source.onerror = () => { streamOK = false; };  // EventSource reconnects on its own
}

export function start() {
  addEventListener("hashchange", () => {
    const r = parseRoute();
    if (app.data && r.pid !== app.data.id) { app.data = null; lastProject = ""; }
    app.route = r;
    scrollTo(0, 0);
    refresh();
  });
  refresh();
  connect();
  let n = 0;
  setInterval(() => { n++; if (!streamOK || n % 6 === 0) refresh(); }, 2500);
  setInterval(() => { app.now = Date.now(); }, 30_000);
}

// ---- helpers over the open project

export const agentById = id => (app.data?.state.agents || []).find(a => a.id === id);
export const askerId = q => q.asked_by || "A-1";
export const kindOf = t => app.hub?.catalog.kinds[t] || { label: t || "Project", roles: {} };
export const roleLabel = a => (kindOf(app.data?.state.template).roles[a.role] || {}).label || a.role;
export const modelName = alias => (app.hub?.catalog.models[alias] || {}).name || alias;

// Who answers a "Talk about this" message: the asker, or the orchestrator once the asker has finished.
export function replier(q) {
  const a = agentById(askerId(q));
  const st = a && app.data.derived.agent_status[a.id];
  return (!a || st === "done" || st === "failed") ? agentById("A-1") : a;
}

export const emptyDraft = () => ({ choice: "", comment: "", talk: false, message: "" });

// What the user is in the middle of: drafts survive refreshes and switching views.
export const ui = $state({
  drafts: {},            // "<project>/<question>" -> { choice, comment, talk, message }
  errors: {},            // "<project>/<question>" -> message
  specTab: "functional",
  findingFilter: "unresolved",
  auditTab: "confirmed",
  modelDraft: {},        // "kind.role" -> alias
  modelError: "",
  settingsNote: "",
  chatDraft: "",
  chatError: "",
  showArchived: false,
  theme: "",             // bumps diagram renders when the theme changes
});
