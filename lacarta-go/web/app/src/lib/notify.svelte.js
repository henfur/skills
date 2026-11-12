import { SvelteSet } from "svelte/reactivity";
import { P } from "./state.svelte.js";

// Notifications, for every project: a new question, an agent's reply in a "Talk about this"
// thread, a session that needs you in its terminal, an agent that finished its turn and listens.
// Shown as a toast in the page and, when the tab is in the background and the user allowed it,
// as a system notification. Whatever is already there when the page loads is not announced.

export const toasts = $state([]);           // { key, base, kind, head, title, href, project, qid, urgent, who }
export const freshQs = new SvelteSet();      // "<project>/<question>" that arrived while the page was open
let seen = null;

function hubEvents(hub) {
  const out = [];
  hub.projects.filter(p => !p.archived).forEach(p => {
    (p.questions || []).forEach(q => {
      out.push({ key: `${p.id}/${q.id}`, p, q, kind: "ask" });
      if (q.replies) out.push({ key: `${p.id}/${q.id}#${q.replies}`, p, q, kind: "reply" });
    });
    if (p.attention) out.push({ key: `${p.id}/attention/${p.attention.at}`, p, kind: "terminal" });
    if (p.listening_since) out.push({ key: `${p.id}/listen/${p.listening_since}`, p, kind: "turn" });
  });
  return out;
}

export function notifyChanges(hub, route) {
  const events = hubEvents(hub);
  if (seen === null) { seen = new Set(events.map(e => e.key)); return; }
  events.filter(e => !seen.has(e.key)).forEach(e => {
    seen.add(e.key);
    const here = route.page === "project" && route.pid === e.p.id;
    if (e.kind === "turn" && here && route.view === "chat") return;   // you are already looking at it
    if (e.kind === "ask") { freshQs.add(e.key); setTimeout(() => freshQs.delete(e.key), 4000); }
    const who = e.q ? e.q.asked_by || "An agent" : (e.p.agent || {}).name || "The agent";
    const head = { ask: `${who} needs you`, reply: `${who} replied`, terminal: `${who} needs you in the terminal`,
                   turn: `${who} finished its turn` }[e.kind];
    const title = e.q ? e.q.title : e.kind === "terminal" ? e.p.attention.text : "Listening for your next message in the console.";
    const href = P(e.p.id, e.q ? "questions" : e.kind === "turn" ? "chat" : "overview");
    while (toasts.length >= 4) toasts.shift();
    toasts.push({ key: e.key, base: e.key.split("#")[0], kind: e.kind, head, title, href, project: e.p.name,
                  qid: e.q?.id, triage: !!e.q?.triage, urgent: !!e.q?.triage || e.kind === "terminal",
                  who: { id: `${e.p.id}:${who}`, name: who } });
    if (document.hidden && "Notification" in window && Notification.permission === "granted") {
      try {
        const n = new Notification(`${head} · ${e.p.name}`, { body: title, tag: e.key });
        n.onclick = () => { window.focus(); location.hash = href; n.close(); };
      } catch (err) {}
    }
  });
  // An answered question, a cleared terminal prompt or a new turn takes its toasts with it.
  const live = new Set(events.map(e => e.key.split("#")[0]));
  for (let i = toasts.length - 1; i >= 0; i--) if (!live.has(toasts[i].base)) toasts.splice(i, 1);
}

export function dismiss(key) {
  const i = toasts.findIndex(t => t.key === key);
  if (i >= 0) toasts.splice(i, 1);
}
