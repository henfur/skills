<script>
  import Overview from "./Overview.svelte";
  import SingleOverview from "./SingleOverview.svelte";
  import Chat from "./Chat.svelte";
  import Questions from "./Questions.svelte";
  import Specs from "./Specs.svelte";
  import Phases from "./Phases.svelte";
  import Findings from "./Findings.svelte";
  import Decisions from "./Decisions.svelte";
  import Activity from "./Activity.svelte";
  import AuditOverview from "./AuditOverview.svelte";
  import Coverage from "./Coverage.svelte";
  import AuditFindings from "./AuditFindings.svelte";
  import Report from "./Report.svelte";
  import Plugins from "./Plugins.svelte";
  import Icon from "../components/Icon.svelte";
  import Badge from "../components/Badge.svelte";
  import { app, L, kindOf } from "../lib/state.svelte.js";
  import { UNRESOLVED, label } from "../lib/format.js";

  // Every kind of project has its own views.
  const VIEWS = {
    software: [["overview", "Overview", Overview], ["chat", "Chat", Chat], ["questions", "Questions", Questions],
      ["specs", "Specifications", Specs], ["phases", "Phases & reviews", Phases], ["findings", "Findings", Findings],
      ["decisions", "Decisions", Decisions], ["activity", "Activity", Activity]],
    audit: [["overview", "Overview", AuditOverview], ["chat", "Chat", Chat], ["questions", "Questions", Questions],
      ["coverage", "Coverage", Coverage], ["findings", "Findings", AuditFindings], ["report", "Report", Report],
      ["plugins", "Plugins", Plugins], ["decisions", "Decisions", Decisions], ["activity", "Activity", Activity]],
    single: [["overview", "Overview", SingleOverview], ["chat", "Chat", Chat], ["questions", "Questions", Questions],
      ["activity", "Activity", Activity]],
  };
  const st = $derived(app.data.state);
  const d = $derived(app.data.derived);
  const views = $derived(VIEWS[st.template] || VIEWS.single);
  const current = $derived(views.find(v => v[0] === app.route.view) || views[0]);
  const counts = $derived({
    questions: [d.open_questions, d.open_questions > 0],
    specs: d.specs_approved != null ? [`${d.specs_approved}/4`, false] : null,
    phases: d.phases_done != null ? [`${d.phases_done}/${st.phases.length}`, false] : null,
    coverage: d.audit?.units_total ? [d.audit.units_total, false] : null,
    report: d.audit?.latest_report ? [`v${d.audit.latest_report.version}`, false] : null,
    findings: d.audit ? (d.audit.has_findings ? [d.audit.to_review ? `${d.audit.to_review} to review` : (app.data.audit?.findings || []).length, d.audit.to_review > 0] : null) : d.open_findings ? [st.findings.filter(f => UNRESOLVED.has(f.status)).length, d.open_findings.critical + d.open_findings.high > 0] : null,
    decisions: [st.decisions.length, false],
    chat: d.pending_messages ? [`${d.pending_messages} queued`, false] : null,
  });
  const View = $derived(current[2]);
</script>

<div class="phead">
  <span class="phead-name">{st.project.name}</span>
  <span class="small muted">{kindOf(st.template).label}</span>
  {#if d.blocked}<Badge s="stopped" text="Blocked" />
  {:else if st.template !== "single"}<Badge s={st.stage} text={label(st.stage)} />{/if}
</div>
<nav class="ptabs" aria-label={st.project.name}>
  {#each views as [k, name] (k)}
    {@const c = counts[k]}
    <a href={L(k)} aria-current={k === current[0] ? "page" : undefined}>{name}{#if c}{" "}<span class="count" class:hot={c[1]}>{c[0]}</span>{/if}</a>
  {/each}
</nav>
{#if app.data.config}
  {@const cfg = app.data.config}
  {#if cfg.warnings.length}<div class="banner warn"><Icon name="warn" /><div><strong>Ignored in <code>{cfg.path}</code></strong>
    <ul class="small" style="margin:4px 0 0">{#each cfg.warnings as w, i (i)}<li>{w}</li>{/each}</ul></div></div>{/if}
{/if}
<View {st} {d} />
{#if app.data.config}
  {@const cfg = app.data.config}
  {#if current[0] === "overview" && (cfg.name || cfg.models || cfg.plugins || cfg.templates || cfg.remote_control != null)}
    <p class="small muted config-note">This project's <code>{cfg.path}</code> sets
      {[cfg.name && "its name", cfg.models && `the models of ${Object.keys(cfg.models).map(r => (app.hub.catalog.kinds[st.template]?.roles[r]?.label || r).toLowerCase()).join(", ")}`,
        cfg.templates && "the spec templates", cfg.plugins && "plugins", cfg.remote_control != null && "remote control"].filter(Boolean).join(", ")}, over the machine's Settings.</p>
  {/if}
{/if}
