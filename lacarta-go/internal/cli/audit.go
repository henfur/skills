package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lacarta/internal/auditreport"
	"lacarta/internal/core"
	"lacarta/internal/plugins"
	"lacarta/internal/sandbox"
	"lacarta/internal/server"
)

// Commands of the security-audit kind, plugins, the wishlist and the sandbox.

// Commands that need no project.
var noProject = map[string]bool{"audit init": true, "audit preflight": true, "audit verify": true,
	"plugin list": true, "plugin install": true, "plugin add": true, "plugin remove": true, "sandbox run": true}

func auditCommands() []*Cmd {
	roles := []string{}
	for _, r := range core.AuditRoles {
		roles = append(roles, r.ID)
	}
	return []*Cmd{
		{Name: "audit init", Help: "Start a security audit of a folder (needs Cloudflare's security-audit skill).", Run: cmdAuditInit,
			Opts: []Opt{{Name: "target", Help: "The repository to audit (default: the current folder)."}, {Name: "name"}}},
		{Name: "audit preflight", Help: "Check what a security audit needs: the skill, node, the sandbox, check tools.", Run: cmdAuditPreflight},
		{Name: "audit guide", Help: "How the orchestrator runs an audit with lacarta. Read it first.", Run: cmdAuditGuide},
		{Name: "audit scope", Help: "Submit the rules of engagement for the auditor's approval.", Run: cmdAuditScope, Opts: []Opt{
			{Name: "profile", Required: true, Choices: core.AuditProfiles},
			{Name: "budget", Kind: num, Help: "Maximum agent invocations; 0 for none."},
			{Name: "path", Kind: list, Help: "A path in scope; repeat. Omit for the whole repository."},
			{Name: "notes", Help: "Anything the auditor should know (markdown)."}}},
		{Name: "audit plan submit", Help: "Validate the coverage ledger and ask the auditor to approve the coverage plan.", Run: cmdAuditPlan},
		{Name: "audit findings submit", Help: "Validate findings.json and the ledger, then hand the findings to the auditor.", Run: cmdAuditFindings},
		{Name: "audit summary", Help: "Write the executive summary draft; the auditor edits and approves it.", Run: cmdAuditSummary,
			Opts: []Opt{{Name: "file"}, {Name: "text"}}},
		{Name: "audit brief", Help: "The lacarta preamble for a subagent's prompt; the skill's own prompt follows it.", Run: cmdAuditBrief,
			Pos: []Pos{{Name: "role", Choices: roles}}, Opts: []Opt{{Name: "unit", Help: "The coverage_id it works on."},
				{Name: "agent", Help: "Its id from `agent start`."}}},
		{Name: "audit checks", Help: "Run the enabled check plugins (gitleaks, osv-scanner) in the sandbox; results are leads.", Run: cmdAuditChecks},
		{Name: "audit rerun", Help: "Start a new run on the same target once the report is approved.", Run: cmdAuditRerun},
		{Name: "audit verify", Help: "Verify a final report folder: signature, file hashes and the AI-assistance disclosure.", Run: cmdAuditVerify,
			Pos: []Pos{{Name: "dir"}}},
		{Name: "wish", Help: "Ask the auditor for a tool or environment you need (the wishlist).", Run: cmdWish,
			Opts: []Opt{{Name: "text", Required: true}, {Name: "unit", Help: "The coverage_id it's for."}}},
		{Name: "plugin list", Help: "Installed and known plugins (and, in a project, which are enabled).", Run: cmdPluginList},
		{Name: "plugin install", Help: "Install a built-in plugin (adopts an existing install).", Run: cmdPluginInstall, Pos: []Pos{{Name: "id"}}},
		{Name: "plugin add", Help: "Add a custom skill or companion from a folder or git URL; shows it first, adds it with --trust.",
			Run: cmdPluginAdd, Pos: []Pos{{Name: "source"}}, Opts: []Opt{{Name: "type", Choices: []string{plugins.TypeSkill, plugins.TypeCompanion}},
				{Name: "trust", Kind: flag, Help: "I read its files and trust it."}}},
		{Name: "plugin remove", Help: "Forget a custom plugin.", Run: cmdPluginRemove, Pos: []Pos{{Name: "id"}}},
		{Name: "plugin enable", Help: "Enable a plugin in this project.", Run: cmdPluginToggle(true), Pos: []Pos{{Name: "id"}}},
		{Name: "plugin disable", Help: "Disable a plugin in this project.", Run: cmdPluginToggle(false), Pos: []Pos{{Name: "id"}}},
		{Name: "sandbox run", Help: "Run a command in the sandbox: no network, read-only target, writes only to --scratch.",
			Run: cmdSandboxRun, Pos: []Pos{{Name: "cmd", Many: true, Help: "After `--`."}}, Opts: []Opt{
				{Name: "scratch", Required: true}, {Name: "ro", Kind: list, Help: "A read-only folder; repeat."},
				{Name: "env", Kind: list, Help: "NAME=value; repeat."}, {Name: "timeout", Kind: num, Help: "Seconds (default 600)."},
				{Name: "mem", Kind: num, Help: "MB of memory (default 4096)."}}},
	}
}

func auditState() (*core.State, error) {
	st, err := store.Read()
	if err != nil {
		return nil, err
	}
	if st.Template != core.Audit {
		return nil, core.Gate("This command belongs to security audits; this project is a %s.", strings.ToLower(core.KindByID(st.Template).Label))
	}
	return st, nil
}

func auditStatus(a *Args, st *core.State, d *core.Derived) error {
	ad := d.Audit
	a.Printf("%s: security audit of %s, stage %s\n", st.Project.Name, st.Audit.Target, st.Stage)
	if r := ad.Run; r != nil {
		a.Printf("Run %s (%s profile), output directory %s\n", r.ID, r.Profile, r.Dir)
		if ad.UnitsTotal > 0 {
			keys := make([]string, 0, len(ad.Units))
			for k := range ad.Units {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := []string{}
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%d %s", ad.Units[k], k))
			}
			a.Printf("Coverage: %d units (%s)\n", ad.UnitsTotal, strings.Join(parts, ", "))
		}
		if ad.Budget > 0 {
			a.Printf("Budget: %d of %d agent invocations used\n", ad.AgentsTotal, ad.Budget)
		}
		if ad.HasFindings {
			a.Printf("Findings: %d confirmed, %d needing validation, %d rejected; %d to review by the auditor\n",
				ad.Verdicts["confirmed"], ad.Verdicts["needs_validation"], ad.Verdicts["rejected"], ad.ToReview)
		}
	}
	if ad.FilesError != "" {
		a.Printf("Run files unreadable: %s\n", ad.FilesError)
	}
	for _, q := range core.OpenQuestions(st) {
		a.Printf("  %s [%s] %s\n", q.ID, q.Kind, q.Title)
	}
	a.Printf("\nNext: %s\n", d.Next)
	return nil
}

func cmdAuditPreflight(a *Args) error {
	cs, err := plugins.Preflight()
	if err != nil {
		return err
	}
	for _, c := range cs {
		mark := "ok  "
		if !c.OK {
			mark = "MISS"
			if !c.Required {
				mark = "--  "
			}
		}
		a.Printf("%s %-20s %s\n", mark, c.Name, c.Detail)
	}
	if f := plugins.PreflightFailed(cs); f != "" {
		return core.Gate("An audit can't start: %s", f)
	}
	a.Println("Ready for a security audit.")
	return nil
}

func gitRef(dir string) (string, bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false
	}
	st, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return strings.TrimSpace(string(out)), len(strings.TrimSpace(string(st))) > 0
}

func cmdAuditInit(a *Args) error {
	target, _ := filepath.Abs(firstNonEmpty(a.Str("target"), "."))
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		return core.Gate("%s is not a folder.", target)
	}
	if pid := core.AuditFor(target); pid != "" {
		return core.Gate("%s already has an audit: %s. `audit rerun` starts a new run of it.", target, pid)
	}
	cs, err := plugins.Preflight()
	if err != nil {
		return err
	}
	if f := plugins.PreflightFailed(cs); f != "" {
		return core.Gate("A security audit can't start: %s", f)
	}
	name := firstNonEmpty(a.Str("name"), filepath.Base(target))
	sum := sha256.Sum256([]byte(target + core.Now()))
	pid := "audit-" + core.Slug(name) + "-" + hex.EncodeToString(sum[:])[:4]
	dir := core.AuditHome(pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	repo := filepath.Base(target)
	if out, err := exec.Command("git", "-C", target, "remote", "get-url", "origin").Output(); err == nil {
		repo = strings.TrimSuffix(filepath.Base(strings.TrimSpace(string(out))), ".git")
	}
	st := core.NewAuditState(pid, name, target, repo, "sandboxed")
	if err := core.Migrate(st); err != nil { // registers the orchestrator
		return err
	}
	if err := plugins.PinAll(st); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(target, ".claude")); err == nil {
		core.Log(st, "The target has a .claude/ folder: its skills and settings are never used by the audit (they come "+
			"from the audited code, which is untrusted).", "stage")
	}
	core.Log(st, fmt.Sprintf("Security audit of %s started.", target), "stage")
	if err := core.WriteJSON(filepath.Join(dir, "state.json"), st); err != nil {
		return err
	}
	if err := core.RegisterAudit(pid, name, target); err != nil {
		return err
	}
	s := &core.Store{Root: target, Dir: dir}
	if sid := os.Getenv("LACARTA_SESSION"); sid != "" {
		if err := core.MoveSession(sid, pid, s); err != nil {
			return err
		}
	}
	console := "not running; start it with `serve`"
	if info := server.EnsureDaemon(self()); info != nil {
		console = info.OpenURL
	}
	a.Printf("Security audit %s of %s\nState and runs: %s\nConsole: %s\n", pid, target, dir, console)
	a.Printf("Next: read `%s audit guide`, tell the user the console URL, then agree the rules of engagement and "+
		"`audit scope`. Pass `--project %s` if $LACARTA_PROJECT isn't set.\n", self(), pid)
	return nil
}

const guide = `How to run a security audit with lacarta

You are the orchestrator (the skill's "parent"). Cloudflare's security-audit skill is the
methodology: load it and follow its full audit mode. Lacarta gates the stages, records who did
what, and keeps what only the human auditor decides. Run %[1]s status after each step:
its Next: line is what lacarta expects.

1. Scope. Agree with the auditor what is in scope, the profile (quick, standard, deep) and a
   budget in agent invocations, then: %[1]s audit scope --profile standard --budget 120 [--path p]...
   Wait for the approval (%[1]s wait).
2. Recon. The run directory (status shows it) is the skill's output directory: never write
   inside the target. Write run-metadata.json, architecture.md and coverage-ledger.json there,
   run %[1]s audit checks for the check plugins' leads (gitleaks, osv-scanner), then
   %[1]s audit plan submit. The auditor approves the coverage plan or asks for changes.
3. Hunt and validate. Follow HUNTING.md and VALIDATION-AND-REPORTING.md phases 2 to 5. Before
   each subagent: %[1]s agent start --role hunter --task "..." and put the output of
   %[1]s audit brief hunter --unit <coverage_id> --agent <id> at the top of its prompt. Use the
   models %[1]s models gives. Run target code only through %[1]s sandbox run.
   When a subagent needs a tool or environment it doesn't have: %[1]s wish --text "...".
4. Findings. When findings.json is written and verified: %[1]s audit findings submit (it runs
   the skill's validators). The auditor then reviews every finding in the console; they may
   change severities, reject, or ask for another check (status tells you which).
5. Report. Write the executive summary draft: %[1]s audit summary --file summary.md. Don't write
   the report yourself: lacarta generates it from the records, with the AI-assistance
   disclosure, and only the auditor can approve it. The skill's own REPORT.md files are drafts.

Never: approve anything for the auditor, edit the target's source, test live systems, or
remove the AI-assistance disclosure from anything you give the user.
`

func cmdAuditGuide(a *Args) error {
	a.Printf(guide, self())
	return nil
}

func cmdAuditScope(a *Args) error {
	st0, err := auditState()
	if err != nil {
		return err
	}
	ref, dirty := gitRef(st0.Audit.Target)
	sc := &core.AuditScope{Profile: a.Str("profile"), Budget: a.Int("budget"), Paths: a.List("path"),
		Notes: a.Str("notes"), SourceRef: ref, Dirty: dirty}
	for _, p := range sc.Paths {
		if _, err := os.Stat(filepath.Join(st0.Audit.Target, p)); err != nil {
			return core.Gate("%s is not in %s.", p, st0.Audit.Target)
		}
	}
	var q *core.Question
	err = store.Edit(func(st *core.State) error {
		ens, err := plugins.Enabled(st)
		if err != nil {
			return err
		}
		names := []string{}
		for _, p := range ens {
			names = append(names, p.Name)
		}
		paths := "the whole repository"
		if len(sc.Paths) > 0 {
			paths = "`" + strings.Join(sc.Paths, "`, `") + "`"
		}
		budget := "none set"
		if sc.Budget > 0 {
			budget = fmt.Sprintf("%d agent invocations", sc.Budget)
		}
		rev := firstNonEmpty(ref, "not a git repository")
		if dirty {
			rev += " (with uncommitted changes: they are audited as they are)"
		}
		body := fmt.Sprintf("- **Target:** `%s`\n- **Revision:** `%s`\n- **In scope:** %s\n- **Profile:** %s\n- **Budget:** %s\n"+
			"- **Execution:** %s\n- **Plugins:** %s\n- **Output:** `%s`, outside the target\n\nNo live or deployed system is "+
			"tested. Target code runs only in the sandbox, without network.",
			st.Audit.Target, rev, paths, sc.Profile, budget, st.Audit.Execution, strings.Join(names, ", "), st.Project.Cwd)
		if sc.Notes != "" {
			body += "\n\n" + sc.Notes
		}
		q, err = core.AuditScopeSubmit(st, sc, body)
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s asks the auditor to approve the rules of engagement. `wait %s`.\n", q.ID, q.ID)
	return nil
}

func validate(skill, file string) error {
	out, ok, err := plugins.Validate(skill, file)
	if err != nil {
		return err
	}
	if !ok {
		return core.Gate("%s fails the skill's validator:\n%s", filepath.Base(file), out)
	}
	return nil
}

func cmdAuditPlan(a *Args) error {
	st0, err := auditState()
	if err != nil {
		return err
	}
	run := core.CurrentRun(st0)
	if run == nil {
		return core.Gate("No run yet: the rules of engagement aren't approved.")
	}
	for _, f := range []string{"run-metadata.json", "architecture.md", "coverage-ledger.json"} {
		if _, err := os.Stat(core.RunFile(run, f)); err != nil {
			return core.Gate("%s is missing in %s.", f, run.Dir)
		}
	}
	skill, err := plugins.SkillPath()
	if err != nil {
		return err
	}
	if err := validate(skill, core.RunFile(run, "coverage-ledger.json")); err != nil {
		return err
	}
	units, err := core.ReadLedger(run)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return core.Gate("The coverage ledger is empty.")
	}
	type key struct{ sub, class string }
	cells := map[key]int{}
	excluded := 0
	for _, u := range units {
		cells[key{u.Subsystem, u.AttackClass}]++
		excluded += len(u.Excluded)
	}
	keys := make([]key, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].sub+keys[i].class < keys[j].sub+keys[j].class })
	var b strings.Builder
	fmt.Fprintf(&b, "**%d units** to hunt (%s profile", len(units), run.Profile)
	if run.Budget > 0 {
		fmt.Fprintf(&b, ", budget %d agent invocations", run.Budget)
	}
	fmt.Fprintf(&b, "). The Coverage view shows the matrix; %d considered blocks were excluded, each with its reason.\n\n"+
		"| Area | Attack class | Units |\n| --- | --- | --- |\n", excluded)
	for _, k := range keys {
		fmt.Fprintf(&b, "| %s | %s | %d |\n", k.sub, k.class, cells[k])
	}
	var q *core.Question
	err = store.Edit(func(st *core.State) error {
		var err error
		q, err = core.AuditPlanSubmit(st, b.String())
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s asks the auditor to approve the coverage plan (%d units). `wait %s`.\n", q.ID, len(units), q.ID)
	return nil
}

func cmdAuditFindings(a *Args) error {
	st0, err := auditState()
	if err != nil {
		return err
	}
	run := core.CurrentRun(st0)
	if run == nil {
		return core.Gate("No run yet.")
	}
	skill, err := plugins.SkillPath()
	if err != nil {
		return err
	}
	for _, f := range []string{"coverage-ledger.json", "findings.json"} {
		if err := validate(skill, core.RunFile(run, f)); err != nil {
			return err
		}
	}
	units, err := core.ReadLedger(run)
	if err != nil {
		return err
	}
	open := 0
	for _, u := range units {
		if !core.UnitDone[u.Status] {
			open++
		}
	}
	if open > 0 {
		return core.Gate("%d ledger units are still planned or in progress: finish them, or mark them deferred with a reason.", open)
	}
	fs, err := core.ReadFindings(run)
	if err != nil {
		return err
	}
	err = store.Edit(func(st *core.State) error {
		if err := core.AuditFindingsSubmit(st, fs); err != nil {
			return err
		}
		st.Audit.Checks = map[string]string{"coverage-ledger.json": "ok", "findings.json": "ok"}
		return nil
	})
	if err != nil {
		return err
	}
	review := 0
	for _, f := range fs {
		if core.NeedsReview(f) {
			review++
		}
	}
	a.Printf("Findings submitted: %d records, %d for the auditor to review in the console.\n", len(fs), review)
	return nil
}

func cmdAuditSummary(a *Args) error {
	text := a.Str("text")
	if f := a.Str("file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		text = string(b)
	}
	if strings.TrimSpace(text) == "" {
		return core.Gate("Give the summary with --file or --text.")
	}
	err := store.Edit(func(st *core.State) error {
		if st.Template != core.Audit {
			return core.Gate("Only security audits have an executive summary.")
		}
		if st.Stage != "review" && st.Stage != "report" {
			return core.Gate("The summary is written once the findings are submitted (stage now: %s).", st.Stage)
		}
		st.Audit.Summary, st.Audit.SummaryBy = strings.TrimSpace(text), core.Actor
		core.Log(st, "Executive summary drafted; the auditor edits and approves it with the report.", "stage")
		return nil
	})
	if err != nil {
		return err
	}
	a.Println("Executive summary saved as a draft.")
	return nil
}

func cmdAuditBrief(a *Args) error {
	st, err := auditState()
	if err != nil {
		return err
	}
	role, _ := core.KindByID(core.Audit).Role(a.Positional("role"))
	run := core.CurrentRun(st)
	if run == nil {
		return core.Gate("No run yet.")
	}
	ens, err := plugins.Enabled(st)
	if err != nil {
		return err
	}
	who := "You"
	if id := a.Str("agent"); id != "" {
		if ag, err := core.GetAgent(st, id); err == nil {
			who = fmt.Sprintf("You are %s (%s)", ag.Name, ag.ID)
		}
	}
	a.Printf("[lacarta brief] %s, the %s of a security audit of %s. Run every lacarta command as `%s --agent %s <command>`.\n",
		who, strings.ToLower(role.Label), st.Audit.Target, self(), firstNonEmpty(a.Str("agent"), "<your id>"))
	a.Printf("- Run directory (the skill's output directory): %s. Write only to your own agents/<id>/scratch there.\n", run.Dir)
	a.Printf("- The target is read-only for you. Run its code only with `%s sandbox run --scratch <your scratch> --ro %s -- <cmd>`; "+
		"never against live systems.\n", self(), st.Audit.Target)
	if len(run.Paths) > 0 {
		a.Printf("- In scope: %s. Anything else is out of scope.\n", strings.Join(run.Paths, ", "))
	}
	a.Println("- Enabled plugins for this project (use these, and only these):")
	for _, p := range ens {
		switch p.Type {
		case plugins.TypeCheck:
			a.Printf("  - %s: results in %s (leads, not findings)\n", p.Name, filepath.Join(run.Dir, "checks", p.ID+".json"))
		default:
			a.Printf("  - %s (%s): %s\n", p.Name, p.Type, p.Path)
		}
	}
	if u := a.Str("unit"); u != "" {
		units, _ := core.ReadLedger(run)
		for _, x := range units {
			if x.CoverageID == u {
				a.Printf("- Your unit: %s / %s / %s / %s. Start from: %s.\n", x.Subsystem, x.Surface, x.Boundary, x.AttackClass,
					strings.Join(x.StartingPaths, ", "))
			}
		}
	}
	a.Printf("- Missing a tool or an environment to confirm something? `%s --agent <id> wish --text \"...\"`, then carry on; "+
		"the lead stays needs_validation until the auditor provides it.\n", self())
	a.Println("The skill's prompt for your role follows.")
	return nil
}

func cmdAuditChecks(a *Args) error {
	st, err := auditState()
	if err != nil {
		return err
	}
	run := core.CurrentRun(st)
	if run == nil {
		return core.Gate("No run yet.")
	}
	ens, err := plugins.Enabled(st)
	if err != nil {
		return err
	}
	ran := 0
	for _, p := range ens {
		if p.Type != plugins.TypeCheck {
			continue
		}
		ran++
		msg, err := plugins.RunCheck(p.ID, st.Audit.Target, filepath.Join(run.Dir, "checks", p.ID+".json"))
		if err != nil {
			a.Printf("%s failed: %s\n", p.ID, err)
			continue
		}
		a.Println(msg)
	}
	if ran == 0 {
		a.Println("No check plugin is enabled (or installed) for this project.")
	}
	return nil
}

func cmdAuditRerun(a *Args) error {
	err := store.Edit(func(st *core.State) error {
		if st.Template != core.Audit {
			return core.Gate("Only security audits have runs.")
		}
		if st.Stage != "done" {
			return core.Gate("The current run isn't finished (stage %s).", st.Stage)
		}
		st.Stage = "scope"
		if st.Audit.Scope != nil {
			st.Audit.Scope.Status = "draft"
		}
		core.Log(st, "New run: the rules of engagement are agreed again. Earlier runs' ledgers and findings feed it.", "stage")
		return nil
	})
	if err != nil {
		return err
	}
	a.Println("New run started at the scope stage. Previous runs stay in the audit's folder; the skill reads them as prior runs.")
	return nil
}

func cmdAuditVerify(a *Args) error {
	m, notes, err := auditreport.Verify(a.Positional("dir"))
	for _, n := range notes {
		a.Println(n)
	}
	if err != nil {
		return err
	}
	a.Printf("Verified: %s.\n", m.ReportID)
	return nil
}

func cmdWish(a *Args) error {
	var q *core.Question
	err := store.Edit(func(st *core.State) error {
		if st.Template != core.Audit {
			return core.Gate("The wishlist belongs to security audits; ask the user with `ask`.")
		}
		body := a.Str("text")
		if u := a.Str("unit"); u != "" {
			body += "\n\nFor coverage unit `" + u + "`."
		}
		var err error
		q, err = core.AddQuestion(st, "question", "Wishlist: "+a.Str("text"), body, []string{"Provide it", "Not possible"}, nil,
			core.Context{Stage: st.Stage}, true, map[string]string{
				"Provide it":   "Say how in the comment (where the tool is, how to reach the environment). The unit is hunted again.",
				"Not possible": "The lead stays needs validation, with this as its blocker; the report lists it under limitations.",
			})
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s is on the wishlist. Carry on; `wait %s` later if it blocks you.\n", q.ID, q.ID)
	return nil
}

func cmdPluginList(a *Args) error {
	var st *core.State
	if store != nil {
		if s, err := store.Read(); err == nil {
			st = s
		}
	}
	if st != nil {
		ps, err := plugins.ForProject(st)
		if err != nil {
			return err
		}
		for _, p := range ps {
			on := "off"
			if p.Enabled {
				on = "ON "
			}
			a.Printf("%s %-16s %-9s %-8s %s%s\n", on, p.ID, p.Type, p.Need, firstNonEmpty(p.Path, "-"), problem(p.Problem))
		}
		return nil
	}
	all, err := plugins.List()
	if err != nil {
		return err
	}
	for _, p := range all {
		inst := "installed"
		if !p.Installed {
			inst = "missing"
		}
		a.Printf("%-16s %-9s %-9s %s%s\n", p.ID, p.Type, inst, firstNonEmpty(p.Path, "-"), problem(p.Problem))
	}
	return nil
}

func problem(p string) string {
	if p == "" {
		return ""
	}
	return "  (" + p + ")"
}

func cmdPluginInstall(a *Args) error {
	return plugins.Install(a.Positional("id"), func(s string) { a.Println(s) })
}

func cmdPluginAdd(a *Args) error {
	p, err := plugins.Add(a.Positional("source"), a.Str("type"), a.Flag("trust"))
	if err != nil {
		return err
	}
	if !a.Flag("trust") {
		a.Printf("Plugin %s (%s) from %s, content %s. Files:\n", p.ID, p.Type, p.Path, p.Ref)
		for _, f := range p.Files {
			a.Printf("  %s\n", f)
		}
		a.Printf("\n%s\n\nThis text will steer agents that run with your rights. Read it all; if you trust it, run the "+
			"same command with --trust.\n", p.Skill)
		return nil
	}
	a.Printf("Added %s (%s, content %s). Enable it per project with `plugin enable %s` or in the console.\n", p.ID, p.Type, p.Ref, p.ID)
	return nil
}

func cmdPluginRemove(a *Args) error {
	if err := plugins.Remove(a.Positional("id")); err != nil {
		return err
	}
	a.Println("Removed.")
	return nil
}

func cmdPluginToggle(on bool) func(*Args) error {
	return func(a *Args) error {
		id := a.Positional("id")
		err := store.Edit(func(st *core.State) error { return plugins.SetEnabled(st, id, on, "user (terminal)") })
		if err != nil {
			return err
		}
		a.Printf("%s %s for this project.\n", id, map[bool]string{true: "enabled", false: "disabled"}[on])
		return nil
	}
}

func cmdSandboxRun(a *Args) error {
	args := a.Positionals("cmd")
	if len(args) == 0 {
		return &UsageError{"give the command after --"}
	}
	env := map[string]string{}
	for _, kv := range a.List("env") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return &UsageError{"--env takes NAME=value"}
		}
		env[k] = v
	}
	spec := &sandbox.Spec{Scratch: a.Str("scratch"), ReadOnly: a.List("ro"), Env: env, Args: args,
		Timeout: time.Duration(a.Int("timeout")) * time.Second, MemMB: a.Int("mem")}
	cmd, cancel, err := sandbox.Command(context.Background(), spec)
	if err != nil {
		return err
	}
	defer cancel()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, a.Out, a.Err
	fmt.Fprintf(a.Err, "[sandbox] %s\n", sandbox.Describe(spec))
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return exitCode(ee.ExitCode())
		}
		return err
	}
	return nil
}
