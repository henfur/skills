<script>
  import Avatar from "./Avatar.svelte";
  import { toasts, dismiss } from "../lib/notify.svelte.js";
  const action = { ask: "Answer", reply: "Read reply", terminal: "Open", turn: "Open chat" };
</script>

<div class="toasts" role="region" aria-label="Notifications" aria-live="polite">
  {#each toasts as t (t.key)}
    <div class="toast" class:danger={t.urgent}>
      <Avatar a={t.who} size="md" />
      <div style="min-width:0">
        <div class="small muted">{t.head} · {t.project}{#if t.qid} · {t.qid}{/if}{#if t.triage} · development stopped{/if}</div>
        <div class="t-title">{t.title}</div>
        <div class="t-actions"><a class="btn sm primary" href={t.href} onclick={() => dismiss(t.key)}>{action[t.kind]}</a></div>
      </div>
      <button type="button" class="close" aria-label="Dismiss" onclick={() => dismiss(t.key)}>×</button>
    </div>
  {/each}
</div>
