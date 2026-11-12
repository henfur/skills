<script>
  import AskerBlock from "./AskerBlock.svelte";
  import KindBadge from "./KindBadge.svelte";
  import Thread from "./Thread.svelte";
  import Md from "./Md.svelte";
  import Icon from "./Icon.svelte";
  import { app, ui, replier, refresh, emptyDraft } from "../lib/state.svelte.js";
  import { post, projectURL } from "../lib/api.js";
  import { phaseName } from "../lib/format.js";
  let { q } = $props();

  // refresh() creates a draft for every open question before the card renders. Question ids
  // restart at Q-1 in every project, so drafts are keyed by both.
  const k = $derived(`${app.data.id}/${q.id}`);
  const dr = $derived(ui.drafts[k]);
  const triage = $derived(q.effect?.type === "finding-triage");
  const helps = $derived(q.option_help || {});
  const detailed = $derived(q.options.some(o => helps[o]));
  const who = $derived(replier(q)?.name ?? "the agent");
  const needsComment = $derived(triage && dr.choice === "Accept the risk");
  const hint = $derived(q.options.length === 0 ? "Your answer" : q.allow_free ? "Comment, or your own answer" : "Comment (optional)");
  let sending = $state(false);

  function choose(o) {
    const d = ui.drafts[k];
    d.talk = false;
    d.choice = d.choice === o ? "" : o;
  }

  async function send() {
    const d = ui.drafts[k];
    sending = true;
    const r = await post(projectURL(app.data.id, d.talk ? "discuss" : "answer"),
      d.talk ? { id: q.id, message: d.message } : { id: q.id, choice: d.choice, comment: d.comment });
    if (!r.ok) ui.errors[k] = r.json.error || (d.talk ? "The message was not sent." : "The answer was not saved.");
    // A sent message keeps the answer draft and returns to answering mode; the reply appears in the thread.
    else if (d.talk) { d.talk = false; d.message = ""; delete ui.errors[k]; }
    else { ui.drafts[k] = emptyDraft(); delete ui.errors[k]; }
    sending = false;
    await refresh();
  }
</script>

<article class="card q" class:blocking={triage} aria-labelledby="t-{q.id}">
  <header><AskerBlock {q} /><span class="row" style="margin-left:auto"><span class="chip">{q.id}</span><KindBadge {q} />
    {#if q.context?.spec}<span class="chip">spec: {q.context.spec}</span>{/if}
    {#if q.context?.phase != null}<span class="chip">{phaseName(q.context.phase)}</span>{/if}
    {#if q.context?.finding}<span class="chip">{q.context.finding}</span>{/if}
  </span></header>
  <div class="body stack">
    <h3 id="t-{q.id}">{q.title}</h3>
    {#if q.body}<Md text={q.body} />{/if}
    <Thread {q} />
    <div class={detailed ? "choices-detailed" : "row"} role="group" aria-label="Choices">
      {#each q.options as o (o)}
        <button type="button" class="btn choice" aria-pressed={!dr.talk && dr.choice === o} onclick={() => choose(o)}>{o}{#if detailed && helps[o]}<span class="help">{helps[o]}</span>{/if}</button>
      {/each}
      <button type="button" class="btn choice talk" aria-pressed={dr.talk} onclick={() => { ui.drafts[k].talk = !ui.drafts[k].talk; }}>
        {#if !detailed}<Icon name="chat" />{" "}{/if}Talk about this{#if detailed}<span class="help">Not sure yet? Ask {who} about this question first. It stays open until you answer it.</span>{/if}
      </button>
    </div>
    {#if dr.talk}
      <div><label class="field" for="c-{q.id}">Your message to {who}</label>
        <textarea id="c-{q.id}" bind:value={ui.drafts[k].message} placeholder="What is unclear, or what would you like to know before deciding?"></textarea></div>
    {:else}
      <div><label class="field" for="c-{q.id}">{needsComment ? "Why you accept this risk (required)" : hint}</label>
        <textarea id="c-{q.id}" bind:value={ui.drafts[k].comment} placeholder={q.kind === "validation" ? "What should change, if anything" : "Add context for the agent"}></textarea></div>
    {/if}
  </div>
  <footer><span class="err" id="e-{q.id}" role="alert">{ui.errors[k] || ""}</span>
    <button type="button" class="btn primary" disabled={sending} onclick={send}>{sending ? "Sending…" : dr.talk ? `Send to ${who}` : "Send answer"}</button></footer>
</article>
