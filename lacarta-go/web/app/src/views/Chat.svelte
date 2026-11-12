<script>
  import { tick } from "svelte";
  import Avatar from "../components/Avatar.svelte";
  import AgentBadge from "../components/AgentBadge.svelte";
  import SessionBanner from "../components/SessionBanner.svelte";
  import ChatBubble from "../components/ChatBubble.svelte";
  import Ago from "../components/Ago.svelte";
  import { app, ui, agentById, refresh } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  let { st, d } = $props();
  const a = $derived(agentById("A-1"));
  const sess = $derived(a.session || null);
  const connected = $derived(!!sess && !sess.ended_at);
  // The transcript plus what you sent from the console; consecutive tool calls fold into one block.
  const blocks = $derived.by(() => {
    const items = [...(app.data.chat || []), ...st.inbox.map(m => ({ role: "console", text: m.text, at: m.at, delivered_at: m.delivered_at }))]
      .sort((x, y) => String(x.at).localeCompare(String(y.at)));
    const out = [];
    for (const m of items) {
      const last = out[out.length - 1];
      if (m.role === "tool" && last?.tools) last.tools.push(m);
      else out.push(m.role === "tool" ? { tools: [m] } : m);
    }
    return out;
  });
  const hint = $derived(!connected ? "No session is connected, so there is nobody to read a message."
    : d.listening ? `${a.name} is listening: your message starts its next turn right away.`
    : `${a.name} reads it after its current step${d.remote ? "" : ", or when its turn ends"}.`);
  let log = $state(), sending = $state(false);

  // Keep the log pinned to the bottom while you are at the bottom; leave it alone otherwise.
  let stick = true;
  $effect.pre(() => {
    void blocks;
    if (log) stick = log.scrollHeight - log.scrollTop - log.clientHeight < 40;
    tick().then(() => { if (log && stick) log.scrollTop = log.scrollHeight; });
  });

  async function send() {
    if (!ui.chatDraft.trim()) { ui.chatError = "Write a message first."; return; }
    sending = true;
    const r = await post(projectURL(app.data.id, "message"), { message: ui.chatDraft });
    if (!r.ok) ui.chatError = r.json.error || "The message was not sent.";
    else { ui.chatDraft = ""; ui.chatError = ""; }
    sending = false;
    stick = true;
    await refresh();
  }
  async function setRemote(e) {
    await post(projectURL(app.data.id, "remote"), { on: e.currentTarget.checked });
    await refresh();
  }
</script>

<h1>Chat</h1>
<div class="chat-head card">
  <div class="row"><Avatar {a} size="md" /><div style="min-width:0"><div class="row"><strong>{a.name}</strong><AgentBadge status={d.agent_status["A-1"]} /></div>
    <div class="small muted">{#if sess}{sess.cli} session · {#if sess.ended_at}ended <Ago iso={sess.ended_at} />{:else}started <Ago iso={sess.started_at} />{/if}{:else}No session has connected yet{/if}{#if sess?.cwd} · <code>{sess.cwd}</code>{/if}</div></div></div>
  <label class="toggle small"><input type="checkbox" checked={d.remote} disabled={!connected} onchange={setRemote} />
    <span><strong>Remote control</strong><br /><span class="muted">{a.name} keeps listening for your messages here, and wakes up when one arrives.</span></span></label>
</div>
<SessionBanner {st} {d} />
<div class="chat-log card" id="chat-log" aria-live="polite" bind:this={log}>
  {#each blocks as b, i (i)}
    {#if b.tools}
      <div class="tools"><div class="small muted">{b.tools.length === 1 ? "1 tool call" : `${b.tools.length} tool calls`}</div>
        {#each b.tools as t, j (j)}<div class="tool"><span class="chip">{t.name}</span> <span class="small">{t.text}</span></div>{/each}</div>
    {:else}<ChatBubble m={b} />{/if}
  {:else}
    <div class="small muted">{sess ? "Nothing said yet." : "The chat appears once a CLI session connects."}</div>
  {/each}
</div>
<div class="composer card">
  <label class="field" for="chat-input">Message {a.name}</label>
  <textarea id="chat-input" placeholder="Write to the agent as you would in its terminal" disabled={!connected}
    bind:value={ui.chatDraft} onkeydown={e => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) send(); }}></textarea>
  <div class="row" style="justify-content:space-between;margin-top:8px"><span class="small muted">{hint}</span>
    <span class="row"><span class="err" role="alert">{ui.chatError}</span><button type="button" class="btn primary" disabled={!connected || sending} onclick={send}>{sending ? "Sending…" : "Send"}</button></span></div>
</div>
