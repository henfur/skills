<script>
  import SessionBanner from "../components/SessionBanner.svelte";
  import AgentsSection from "../components/AgentsSection.svelte";
  import ActivityRows from "../components/ActivityRows.svelte";
  import Badge from "../components/Badge.svelte";
  import Ago from "../components/Ago.svelte";
  import Md from "../components/Md.svelte";
  import Icon from "../components/Icon.svelte";
  import { L } from "../lib/state.svelte.js";
  let { st, d } = $props();
  const k = $derived(d.desk);
  const cfg = $derived(k.config);
  const profiles = $derived(Object.keys(cfg.profiles).sort());
  const queued = $derived((k.counts.queued || 0) + (k.counts.reviewing || 0));
  const target = t => t.group ? `${t.group}${t.include_subgroups ? " (with subgroups)" : ""}` : t.project;
</script>

<h1>Overview</h1>
<p class="lede">Review desk on <code>{cfg.forge.host || "no host set"}</code>{#if cfg.forge.identity} · posts as <strong>{cfg.forge.identity}</strong>{/if}</p>
<SessionBanner {st} {d} />
{#if k.paused}<div class="banner warn"><Icon name="warn" /><div><strong>Paused.</strong> Nothing is posted and no review starts. <a href={L("desk")}>Resume it</a> in the Desk view.</div></div>{/if}
{#if !k.trusted}<div class="banner warn"><Icon name="warn" /><div><strong>The desk changed since you accepted it.</strong> Reviews go on, but nothing is posted on its own until you accept the new content in <a href={L("questions")}>Questions</a>.</div></div>{/if}
{#if k.poll.error}<div class="banner danger"><Icon name="alert" /><div>The last poll failed: {k.poll.error}</div></div>{/if}
<h2>Status</h2>
<div class="grid">
  <a class="card stat" href={L("queue")} style="text-decoration:none;color:inherit"><div class="overline">Drafts for you</div>
    <div class="value">{k.drafts}</div><div class="small muted">reviews waiting to be published</div></a>
  <a class="card stat" href={L("queue")} style="text-decoration:none;color:inherit"><div class="overline">In the queue</div>
    <div class="value">{queued}</div><div class="small muted">{k.active ? `reviewing ${k.active}` : "nothing in review"}</div></a>
  <div class="card stat"><div class="overline">Reviews today</div>
    <div class="value">{k.budget.used || 0}<small>{cfg.schedule.daily_budget ? ` / ${cfg.schedule.daily_budget}` : ""}</small></div>
    <div class="small muted">{cfg.schedule.quiet_hours ? `quiet hours ${cfg.schedule.quiet_hours}` : "no quiet hours"}</div></div>
  <div class="card stat"><div class="overline">Last poll</div>
    <div class="value small">{#if k.poll.last}<Ago iso={k.poll.last} />{:else}never{/if}</div>
    <div class="small muted">every {cfg.schedule.poll} · {cfg.targets.length} target{cfg.targets.length === 1 ? "" : "s"}</div></div>
</div>
<h2>Profiles</h2>
<div class="card list">
  {#each profiles as p (p)}
    {@const pr = cfg.profiles[p]}
    {@const s = k.stats?.[p]}
    <div class="row"><strong>{p}</strong><Badge s={pr.mode === "shadow" ? "draft" : pr.mode === "assisted" ? "awaiting-approval" : "approved"} text={pr.mode} />
      <span class="small muted">{pr.model} · {pr.trigger.always ? "every merge request" : "when " + [...(pr.trigger.paths || []), ...(pr.trigger.labels || []).map(l => "label " + l)].slice(0, 4).join(", ")}{pr.verify !== "none" ? ` · verifies ${pr.verify} findings` : ""}</span>
      {#if s}<span class="small muted">· you published {s.published} draft{s.published === 1 ? "" : "s"} ({s.edited} edited), discarded {s.discarded}</span>{/if}
    </div>
  {/each}
</div>
<p class="small muted">Targets: {cfg.targets.length ? cfg.targets.map(target).join(", ") : "none yet: add them to desk.targets in .lacarta/config.json"}. Change modes in the <a href={L("desk")}>Desk</a> view or in the config file.</p>
<h2>Next step</h2>
<div class="banner info" style="margin:0"><Icon name="info" /><div><Md text={d.next} inline /></div></div>
<AgentsSection {st} {d} />
<h2>Recent activity</h2>
<div class="card list"><ActivityRows items={st.activity.slice(-10).reverse()} /></div>
