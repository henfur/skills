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
  import { app, L } from "../lib/state.svelte.js";
  import { UNRESOLVED } from "../lib/format.js";

  // Every kind of project has its own views.
  const VIEWS = {
    software: [["overview", "Overview", Overview], ["chat", "Chat", Chat], ["questions", "Questions", Questions],
      ["specs", "Specifications", Specs], ["phases", "Phases & reviews", Phases], ["findings", "Findings", Findings],
      ["decisions", "Decisions", Decisions], ["activity", "Activity", Activity]],
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
    findings: d.open_findings ? [st.findings.filter(f => UNRESOLVED.has(f.status)).length, d.open_findings.critical + d.open_findings.high > 0] : null,
    decisions: [st.decisions.length, false],
    chat: d.pending_messages ? [`${d.pending_messages} queued`, false] : null,
  });
  const View = $derived(current[2]);
</script>

<nav class="ptabs" aria-label={st.project.name}>
  {#each views as [k, name] (k)}
    {@const c = counts[k]}
    <a href={L(k)} aria-current={k === current[0] ? "page" : undefined}>{name}{#if c}{" "}<span class="count" class:hot={c[1]}>{c[0]}</span>{/if}</a>
  {/each}
</nav>
<View {st} {d} />
