package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lacarta/internal/core"
	"lacarta/internal/hooks"
	"lacarta/internal/server"
	"lacarta/skill"
)

const intro = `lacarta: one console for every agent session and project on this machine.

A software project's state lives in <project>/.lacarta/ (state.json, specs/, reviews/), and its
gates refuse to move the project forward while the framework's rules are not met. Machine-wide
state lives in ~/.lacarta/ ($LACARTA_HOME). Every write happens under a file lock.

Run ` + "`lacarta <command> --help`" + ` for the options of each command.`

// The project the command acts on; nil when none was found.
var store *core.Store

var softwareOnly = map[string]bool{"spec": true, "plan": true, "phase": true, "review": true, "finding": true, "final": true}

func roleChoices() []string {
	out := []string{}
	for _, r := range core.SoftwareRoles {
		if r.ID != "orchestrator" {
			out = append(out, r.ID)
		}
	}
	return out
}

func specKeys() []string {
	out := []string{}
	for _, s := range core.Specs {
		out = append(out, s.K)
	}
	return out
}

func aliases() string {
	out := []string{}
	for _, m := range core.Models {
		out = append(out, m.Alias)
	}
	return strings.Join(out, ", ")
}

func commands() []*Cmd {
	return []*Cmd{
		{Name: "init", Help: "Create .lacarta/ with spec templates.", Run: cmdInit, Opts: []Opt{
			{Name: "name", Help: "Project name (default: the folder's name)."}, {Name: "summary"}}},
		{Name: "status", Help: "Stage, gates, open items and the next expected action.", Run: cmdStatus,
			Opts: []Opt{{Name: "json", Kind: flag}}},
		{Name: "serve", Help: "Run the console for every project on this machine (loopback by default). Sessions start " +
			"it on their own; this prints its URL if it already runs.", Run: cmdServe, Opts: []Opt{
			{Name: "port", Kind: num, Help: "Default 4747."},
			{Name: "host", Help: "Address to bind. Anything but loopback requires the key printed in the URL."},
			{Name: "allow-host", Kind: list, Help: "Extra host name or IP the browser will use (LAN name, VPN IP); repeat."}}},
		{Name: "install", Help: "Add the hooks that register every Claude Code session in the console.", Run: cmdInstall,
			Opts: []Opt{{Name: "settings", Help: "Claude Code settings file (default: ~/.claude/settings.json)."},
				{Name: "uninstall", Kind: flag, Help: "Remove the hooks instead."}}},
		{Name: "hook", Help: "Called by the CLI's hooks, with its JSON on stdin.", Run: cmdHook,
			Pos: []Pos{{Name: "event", Choices: hookEvents()}}},
		{Name: "inbox", Help: "Read the messages the user sent from the console (CLIs without hooks).", Run: cmdInbox},
		{Name: "spec submit", Help: "Ask the user to approve one spec.", Run: cmdSpecSubmit,
			Pos: []Pos{{Name: "kind", Choices: specKeys()}}, Opts: []Opt{{Name: "note", Help: "What changed or what to look at (markdown)."}}},
		{Name: "plan submit", Help: "Ask the user to approve the phase plan.", Run: cmdPlanSubmit},
		{Name: "plan reopen", Help: "Reopen the plan after a user decision changed it.", Run: cmdPlanReopen,
			Opts: []Opt{{Name: "reason", Required: true}}},
		{Name: "phase add", Help: "Append a phase to the plan.", Run: cmdPhaseAdd, Opts: []Opt{
			{Name: "title", Required: true}, {Name: "goal", Required: true, Help: "What is usable at the end of the phase."},
			{Name: "scope", Kind: list, Help: "One deliverable; repeat."},
			{Name: "covers", Kind: list, Help: "Requirement IDs covered (FR-1, SR-2); repeat."}}},
		{Name: "phase start", Run: cmdPhaseStart, Pos: []Pos{{Name: "id", Int: true}}},
		{Name: "phase complete", Help: "Check gates and ask the user to approve the phase.", Run: cmdPhaseComplete,
			Pos: []Pos{{Name: "id", Int: true}}, Opts: []Opt{{Name: "summary", Help: "What was delivered (markdown)."}}},
		{Name: "review add", Help: "Record a review report: specification, functional or security.", Run: cmdReviewAdd, Opts: []Opt{
			{Name: "phase", Required: true, Help: "Phase number, 'spec' or 'final'."},
			{Name: "type", Required: true, Choices: core.AllReviewTypes},
			{Name: "verdict", Required: true, Choices: []string{"pass", "fail"}},
			{Name: "report", Required: true, Help: "Markdown report file to store."},
			{Name: "summary", Required: true},
			{Name: "reviewer", Help: "Who reviewed, e.g. 'subagent', 'fresh-agent'."}}},
		{Name: "finding add", Help: "Record a finding. High/critical blocks the project.", Run: cmdFindingAdd, Opts: []Opt{
			{Name: "phase", Required: true, Help: "Phase number, 'spec' or 'final'."},
			{Name: "review", Required: true, Choices: core.AllReviewTypes},
			{Name: "severity", Required: true, Choices: core.Severities},
			{Name: "title", Required: true}, {Name: "detail"},
			{Name: "location", Help: "file:line, component, or spec section."},
			{Name: "scenario", Help: "What an input or user does and what goes wrong. Required for high/critical."},
			{Name: "recommendation", Help: "The fix you propose. Required for high/critical."},
			{Name: "spec", Choices: specKeys(), Help: "The spec a specification finding is about."}}},
		{Name: "finding resolve", Run: cmdFindingResolve, Pos: []Pos{{Name: "id"}}, Opts: []Opt{
			{Name: "status", Required: true, Choices: []string{"fixed", "accepted", "wont-fix"}},
			{Name: "note", Required: true, Help: "How it was fixed and verified, or why it is accepted."}}},
		{Name: "ask", Help: "Ask the user a question in the console; prints its id.", Run: cmdAsk, Opts: []Opt{
			{Name: "kind", Choices: []string{"question", "decision", "validation"}},
			{Name: "title", Required: true}, {Name: "body", Help: "Context in markdown."}, {Name: "body-file"},
			{Name: "option", Kind: list, Help: "A choice, as \"Label|what choosing it means\"; repeat. Omit for free text."},
			{Name: "no-free", Kind: flag, Help: "Only the listed options are allowed."},
			{Name: "spec", Choices: specKeys()}, {Name: "phase"}}},
		{Name: "reply", Help: "Answer the user in a question's \"Talk about this\" thread; the question stays open.", Run: cmdReply,
			Pos: []Pos{{Name: "id"}}, Opts: []Opt{
				{Name: "message", Help: "Your reply, in markdown."}, {Name: "message-file"},
				{Name: "body", Help: "Replace the question text, when the discussion showed it was unclear."}, {Name: "body-file"},
				{Name: "option", Kind: list, Help: "Replace the options (\"Label|meaning\"); not allowed on approvals and triage."}}},
		{Name: "wait", Help: "Block until questions are answered in the console; prints the answers. Run it as a background " +
			"command so the answer wakes you.", Run: cmdWait,
			Pos: []Pos{{Name: "ids", Many: true, Help: "Questions to wait for (default: every open question, first answer wins)."}},
			Opts: []Opt{{Name: "any", Kind: flag, Help: "Return as soon as one of the ids is answered."},
				{Name: "timeout", Kind: num, Help: "Seconds before giving up (exit 3); 0 waits forever."}}},
		{Name: "answer", Help: "Record an answer the user gave in the terminal, word for word.", Run: cmdAnswer,
			Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "choice", Help: "One of the question's options."}, {Name: "comment", Help: "The user's words."}}},
		{Name: "decisions", Help: "Print the decision log as markdown (for the spec reviewer).", Run: cmdDecisions},
		{Name: "agent start", Help: "Register a subagent before spawning it; gives its id, name and model.", Run: cmdAgentStart, Opts: []Opt{
			{Name: "role", Required: true, Choices: roleChoices()}, {Name: "task", Required: true, Help: "One line: what it is doing."},
			{Name: "phase", Help: "Phase number, 'spec' or 'final'."}}},
		{Name: "agent update", Help: "Change an agent's current task or status.", Run: cmdAgentUpdate, Pos: []Pos{{Name: "id"}},
			Opts: []Opt{{Name: "task"}, {Name: "status", Choices: []string{"working", "idle"}}}},
		{Name: "agent done", Help: "Mark a subagent finished.", Run: cmdAgentDone, Pos: []Pos{{Name: "id"}},
			Opts: []Opt{{Name: "summary"}, {Name: "failed", Kind: flag}}},
		{Name: "agent rename", Help: "Rename an agent (when the user asks).", Run: cmdAgentRename, Pos: []Pos{{Name: "id"}},
			Opts: []Opt{{Name: "name", Required: true}}},
		{Name: "agent list", Help: "All agents with their status.", Run: cmdAgentList},
		{Name: "models", Help: "Show the model for each role of this kind of project (read it before every spawn).", Run: cmdModels,
			Opts: []Opt{{Name: "json", Kind: flag}, {Name: "set", Kind: list, Metavar: "ROLE=MODEL",
				Help: "Change a role's model as the user asked in the terminal. Models: " + aliases() + "."}}},
		{Name: "decide", Help: "Log a decision you made yourself.", Run: cmdDecide, Opts: []Opt{
			{Name: "title", Required: true}, {Name: "decision", Required: true}, {Name: "rationale"},
			{Name: "spec", Choices: specKeys()}, {Name: "phase"}}},
		{Name: "final start", Run: cmdFinalStart},
		{Name: "final complete", Run: cmdFinalComplete, Opts: []Opt{{Name: "summary"}}},
	}
}

// Main runs the command line and returns the exit code.
func Main(argv []string, stdout, stderr io.Writer) int {
	cmds := commands()
	a, err := parse(cmds, argv)
	if h, ok := err.(errHelp); ok {
		if h.cmd != nil {
			cmdHelp(h.cmd, stdout)
		} else {
			usage(cmds, stdout, intro)
		}
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "usage: lacarta [--root ROOT] [--project PROJECT] [--agent AGENT] <command> ...")
		fmt.Fprintf(stderr, "lacarta: error: %s\n", err)
		return 2
	}
	a.Out, a.Err = stdout, stderr
	hooks.Self = self()
	hooks.EnsureDaemon = func() *core.DaemonInfo { return server.EnsureDaemon(hooks.Self) }
	core.Actor = firstNonEmpty(a.Agent, os.Getenv("LACARTA_AGENT"), core.Orchestrator)
	if err := run(a); err != nil {
		if core.IsGate(err) {
			fmt.Fprintf(stderr, "Refused: %s\n", err)
			return 2
		}
		if e, ok := err.(exitCode); ok {
			return int(e)
		}
		fmt.Fprintf(stderr, "lacarta: %s\n", err)
		return 1
	}
	return 0
}

type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

func run(a *Args) error {
	switch a.Cmd.Name {
	case "init", "install", "serve", "hook":
		return a.Cmd.Run(a)
	}
	var err error
	if store, err = resolveStore(a); err != nil {
		return err
	}
	if a.Cmd.Name == "models" {
		return a.Cmd.Run(a)
	}
	if store == nil {
		return core.Gate("No project here. `init` creates a software project in this folder; sessions register on " +
			"their own once `install` has added the hooks.")
	}
	if _, err := os.Stat(store.Path()); err != nil {
		return core.Gate("No project here. `init` creates a software project in this folder; sessions register on " +
			"their own once `install` has added the hooks.")
	}
	group := strings.Fields(a.Cmd.Name)[0]
	if softwareOnly[group] {
		st, err := store.Read()
		if err != nil {
			return err
		}
		if st.Template != core.Software {
			return core.Gate("`%s` belongs to software projects; this is a single-agent task. Run `init` to turn this "+
				"folder into a software project.", group)
		}
	}
	return a.Cmd.Run(a)
}

func resolveStore(a *Args) (*core.Store, error) {
	if a.Root != "" {
		root, _ := filepath.Abs(a.Root)
		return core.SoftwareStore(root), nil
	}
	if a.Project != "" {
		return core.ProjectStore(a.Project)
	}
	cwd, _ := os.Getwd()
	if root := core.FindRoot(cwd); root != "" {
		// A project from before the console was machine-wide joins it on first use.
		if !core.IsRegistered(root) {
			name, err := projectName(root)
			if err != nil {
				return nil, err
			}
			if _, err := core.RegisterSoftware(root, name); err != nil {
				return nil, err
			}
		}
		return core.SoftwareStore(root), nil
	}
	if p := os.Getenv("LACARTA_PROJECT"); p != "" {
		return core.ProjectStore(p)
	}
	return nil, nil
}

func projectName(root string) (string, error) {
	var head struct {
		Project core.Project `json:"project"`
	}
	b, err := os.ReadFile(filepath.Join(root, core.StateDir, "state.json"))
	if err != nil {
		return "", err
	}
	return head.Project.Name, json.Unmarshal(b, &head)
}

func hookEvents() []string {
	return []string{"session-start", "prompt", "post-tool", "notification", "stop", "session-end"}
}

func cmdServe(a *Args) error {
	core.Actor = ""
	port := core.DefaultPort
	if a.Has("port") {
		port = a.Int("port")
	}
	return server.Serve(firstNonEmpty(a.Str("host"), "127.0.0.1"), port, a.List("allow-host"), a.Out)
}

func cmdInstall(a *Args) error { return hooks.Install(a.Str("settings"), a.Flag("uninstall"), a.Out) }

func cmdHook(a *Args) error {
	hooks.Run(a.Positional("event"), os.Stdin, a.Out)
	return nil
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func self() string {
	exe, err := os.Executable()
	if err != nil {
		return "lacarta"
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}

// ---------------------------------------------------------------- commands

func cmdInit(a *Args) error {
	root, _ := filepath.Abs(firstNonEmpty(a.Root, "."))
	d := filepath.Join(root, core.StateDir)
	if _, err := os.Stat(filepath.Join(d, "state.json")); err == nil {
		return core.Gate("%s already exists. Use `status` to resume.", d)
	}
	name := firstNonEmpty(a.Str("name"), filepath.Base(root))
	if err := os.MkdirAll(filepath.Join(d, "specs"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(d, "reviews"), 0o755); err != nil {
		return err
	}
	for _, k := range core.Specs {
		target := filepath.Join(d, "specs", k.K+".md")
		if _, err := os.Stat(target); err == nil {
			continue
		}
		tpl, err := skill.Templates.ReadFile("templates/" + k.K + ".md")
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(strings.ReplaceAll(string(tpl), "{{PROJECT}}", name)), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(d, ".gitignore"), []byte(".lock\nstate.tmp\n"), 0o644); err != nil {
		return err
	}
	st := &core.State{
		Version: 1, Template: core.Software,
		Project: core.Project{Name: name, Summary: a.Str("summary"), CreatedAt: core.Now()},
		Stage:   "specification", Plan: core.Plan{Status: "draft"},
		Counters: map[string]int{},
	}
	for _, k := range core.Specs {
		st.Specs.Set(k.K, &core.Spec{Title: k.V, Status: "draft"})
	}
	if err := core.Migrate(st); err != nil { // registers the orchestrator
		return err
	}
	core.Log(st, fmt.Sprintf("Project %s initialised.", name), "stage")
	if err := core.WriteJSON(filepath.Join(d, "state.json"), st); err != nil {
		return err
	}
	pid, err := core.RegisterSoftware(root, name)
	if err != nil {
		return err
	}
	if sid := os.Getenv("LACARTA_SESSION"); sid != "" {
		if err := core.MoveSession(sid, pid, core.SoftwareStore(root)); err != nil {
			return err
		}
	}
	console := "not running; start it with `serve`"
	if info := server.EnsureDaemon(self()); info != nil {
		console = info.OpenURL
	}
	a.Printf("Initialised %s as project %s\nSpec templates: %s\nConsole: %s\n"+
		"Next: tell the user the console URL, then ask what the project is (unless they already said).\n",
		d, pid, filepath.Join(d, "specs"), console)
	return nil
}

func cmdStatus(a *Args) error {
	st, err := store.Read()
	if err != nil {
		return err
	}
	d, err := core.DeriveState(st, store)
	if err != nil {
		return err
	}
	if a.Flag("json") {
		b, err := json.Marshal(st)
		if err != nil {
			return err
		}
		var m core.OMap[json.RawMessage]
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		db, _ := json.Marshal(d)
		m.Set("derived", db)
		out, err := core.MarshalIndent(m)
		if err != nil {
			return err
		}
		a.Println(string(out))
		return nil
	}
	if st.Template == core.Single {
		a.Printf("%s: single-agent task in %s, %s\n", st.Project.Name, st.Project.Cwd, st.Stage)
		a.Printf("Remote control: %s; console messages waiting: %d\n", onOff(d.Remote), d.PendingMessages)
		for _, q := range core.OpenQuestions(st) {
			a.Printf("  %s [%s] %s\n", q.ID, q.Kind, q.Title)
		}
		a.Printf("\nNext: %s\n", d.Next)
		return nil
	}
	a.Printf("%s: stage %s\n", st.Project.Name, st.Stage)
	if d.Blocked != nil {
		a.Printf("BLOCKED: %s\n", *d.Blocked)
	}
	a.Println("\nSpecs:")
	for _, k := range core.Specs {
		s := st.Spec(k.K)
		lines, err := core.TodoLines(store, k.K)
		if err != nil {
			return err
		}
		todo := ""
		if len(lines) > 0 {
			todo = fmt.Sprintf("  (%d TODO lines)", len(lines))
		}
		a.Printf("  %-13s %-18s rev %d%s\n", k.K, s.Status, s.Revision, todo)
	}
	a.Printf("\nSpec review: %s\n", d.SpecReview)
	a.Printf("\nPhase plan: %s\n", st.Plan.Status)
	for _, p := range st.Phases {
		revs := []string{}
		for _, t := range core.ReviewTypes {
			v := "-"
			if r := core.LatestReview(st, *core.NumPhase(p.ID), t); r != nil {
				v = r.Verdict
			}
			revs = append(revs, t+":"+v)
		}
		a.Printf("  %d. %-40s %-18s %s\n", p.ID, p.Title, p.Status, strings.Join(revs, " "))
	}
	if qs := core.OpenQuestions(st); len(qs) > 0 {
		a.Println("\nOpen questions:")
		for _, q := range qs {
			a.Printf("  %s [%s] %s\n", q.ID, q.Kind, q.Title)
		}
	}
	first := true
	for _, f := range st.Findings {
		if !core.Unresolved[f.Status] {
			continue
		}
		if first {
			a.Println("\nUnresolved findings:")
			first = false
		}
		a.Printf("  %s %-8s %-9s phase %s: %s\n", f.ID, f.Severity, f.Status, f.Phase, f.Title)
	}
	a.Printf("\nNext: %s\n", d.Next)
	return nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func requireSpecReview(st *core.State) error {
	for _, k := range core.Specs {
		lines, err := core.TodoLines(store, k.K)
		if err != nil {
			return err
		}
		if len(lines) > 0 {
			return core.Gate("%s.md still has TODO markers on lines %s. The whole specification must be written before "+
				"review and approval.", k.K, core.PyIntList(lines[:min(12, len(lines))]))
		}
	}
	status, err := core.SpecReviewStatus(st, store)
	if err != nil {
		return err
	}
	if status != "passed" {
		return core.Gate("%s Run a fresh spec reviewer (references/spec-review.md) first.", map[string]string{
			"none":   "No specification review yet.",
			"failed": "The latest specification review failed or has unresolved findings.",
			"stale":  "The specs or the plan changed after the last specification review passed.",
		}[status])
	}
	return nil
}

func cmdSpecSubmit(a *Args) error {
	kind := a.Positional("kind")
	var q *core.Question
	err := store.Edit(func(st *core.State) error {
		if st.Stage != "specification" {
			return core.Gate("Specs are submitted during the specification stage. Record later changes with " +
				"`decide --spec` and edit the file.")
		}
		s := st.Spec(kind)
		if s.Status == "approved" || s.Status == "in-review" {
			return core.Gate("The %s spec is already %s.", kind, s.Status)
		}
		if err := requireSpecReview(st); err != nil {
			return err
		}
		h, err := core.SpecFileHash(store, kind)
		if err != nil {
			return err
		}
		s.Revision++
		s.Status, s.SubmittedHash = "in-review", h
		body := firstNonEmpty(a.Str("note"), fmt.Sprintf("Read it under Specifications → %s.", core.SpecTitle(kind)))
		q, err = core.AddQuestion(st, "validation", fmt.Sprintf("Approve the %s spec (revision %d)?", core.SpecTitle(kind), s.Revision),
			body, []string{core.Approve, core.RequestChanges}, &core.Effect{Type: "spec-approval", Spec: kind},
			core.Context{Stage: "specification", Spec: kind}, false, core.ApprovalHelp)
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s asks the user to approve the %s spec. `wait %s`.\n", q.ID, kind, q.ID)
	return nil
}

func cmdPhaseAdd(a *Args) error {
	var pid int
	err := store.Edit(func(st *core.State) error {
		if st.Plan.Status == "approved" || st.Plan.Status == "in-review" {
			return core.Gate("The phase plan is approved. Changing it is a decision: `ask` the user first, then `plan reopen`.")
		}
		pid = len(st.Phases) + 1
		scope, covers := a.List("scope"), a.List("covers")
		if scope == nil {
			scope = []string{}
		}
		if covers == nil {
			covers = []string{}
		}
		st.Phases = append(st.Phases, &core.Phase{ID: pid, Title: a.Str("title"), Goal: a.Str("goal"),
			Scope: scope, Covers: covers, Status: "planned"})
		core.Log(st, fmt.Sprintf("Phase %d planned: %s", pid, a.Str("title")), "plan")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("Phase %d added.\n", pid)
	return nil
}

func cmdPlanSubmit(a *Args) error {
	var q *core.Question
	err := store.Edit(func(st *core.State) error {
		if len(st.Phases) == 0 {
			return core.Gate("Add phases with `phase add` first.")
		}
		if st.Plan.Status == "approved" || st.Plan.Status == "in-review" {
			return core.Gate("The plan is already %s.", st.Plan.Status)
		}
		if st.Stage == "specification" {
			if err := requireSpecReview(st); err != nil {
				return err
			}
		}
		st.Plan.Status, st.Plan.SubmittedHash = "in-review", core.PlanHash(st)
		lines := make([]string, len(st.Phases))
		for i, p := range st.Phases {
			lines[i] = fmt.Sprintf("%d. **%s**: %s", p.ID, p.Title, p.Goal)
		}
		var err error
		q, err = core.AddQuestion(st, "validation", fmt.Sprintf("Approve the phase plan (%d phases)?", len(st.Phases)),
			strings.Join(lines, "\n"), []string{core.Approve, core.RequestChanges}, &core.Effect{Type: "plan-approval"},
			core.Context{Stage: "specification"}, false, core.ApprovalHelp)
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s asks the user to approve the plan. `wait %s`.\n", q.ID, q.ID)
	return nil
}

func cmdPlanReopen(a *Args) error {
	err := store.Edit(func(st *core.State) error {
		st.Plan.Status = "draft"
		core.Log(st, "Phase plan reopened: "+a.Str("reason"), "plan")
		return nil
	})
	if err != nil {
		return err
	}
	a.Println("Plan reopened. Edit phases, then `plan submit`.")
	return nil
}

func cmdPhaseStart(a *Args) error {
	id := a.PosInt("id")
	err := store.Edit(func(st *core.State) error {
		if r := core.BlockedReason(st); r != "" {
			return core.Gate("Blocked. %s", r)
		}
		if st.Stage != "implementation" {
			return core.Gate("Phases start in the implementation stage (now: %s).", st.Stage)
		}
		if st.Plan.Status != "approved" {
			return core.Gate("The phase plan was reopened and is not approved again yet.")
		}
		p, err := core.GetPhase(st, id)
		if err != nil {
			return err
		}
		if p.Status != "planned" {
			return core.Gate("Phase %d is %s.", p.ID, p.Status)
		}
		for _, x := range st.Phases {
			if x.Status == "in-progress" || x.Status == "awaiting-approval" {
				return core.Gate("Phase %d is still %s. One phase at a time.", x.ID, x.Status)
			}
		}
		for _, x := range st.Phases {
			if x.ID < p.ID && x.Status != "done" {
				return core.Gate("Phase %d is not done yet.", x.ID)
			}
		}
		p.Status, p.StartedAt = "in-progress", strPtr(core.Now())
		core.Log(st, fmt.Sprintf("Phase %d started: %s", p.ID, p.Title), "phase")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("Phase %d started.\n", id)
	return nil
}

func cmdPhaseComplete(a *Args) error {
	id := a.PosInt("id")
	var q *core.Question
	err := store.Edit(func(st *core.State) error {
		if r := core.BlockedReason(st); r != "" {
			return core.Gate("Blocked. %s", r)
		}
		p, err := core.GetPhase(st, id)
		if err != nil {
			return err
		}
		if p.Status != "in-progress" {
			return core.Gate("Phase %d is %s, not in-progress.", p.ID, p.Status)
		}
		for _, t := range core.ReviewTypes {
			r := core.LatestReview(st, *core.NumPhase(p.ID), t)
			if r == nil {
				return core.Gate("No %s review recorded for phase %d.", t, p.ID)
			}
			if r.Verdict != "pass" {
				return core.Gate("The latest %s review of phase %d (%s) failed. Fix and review again.", t, p.ID, r.ID)
			}
		}
		left, fs, fixed, accepted := []string{}, 0, 0, 0
		for _, f := range st.Findings {
			if f.Phase != *core.NumPhase(p.ID) {
				continue
			}
			fs++
			if core.Unresolved[f.Status] {
				left = append(left, f.ID)
			}
			switch f.Status {
			case "fixed":
				fixed++
			case "accepted":
				accepted++
			}
		}
		if len(left) > 0 {
			return core.Gate("Unresolved findings in phase %d: %s.", p.ID, strings.Join(left, ", "))
		}
		p.Status = "awaiting-approval"
		body := a.Str("summary") + fmt.Sprintf("\n\nReviews passed. Findings this phase: %d (%d fixed, %d accepted).", fs, fixed, accepted)
		q, err = core.AddQuestion(st, "validation", fmt.Sprintf("Approve phase %d: %s?", p.ID, p.Title), strings.TrimSpace(body),
			[]string{core.Approve, core.RequestChanges}, &core.Effect{Type: "phase-approval", Phase: core.NumPhase(p.ID)},
			core.Context{Stage: "implementation", Phase: core.NumPhase(p.ID)}, false, core.ApprovalHelp)
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s asks the user to approve phase %d. `wait %s`.\n", q.ID, id, q.ID)
	return nil
}

func cmdReviewAdd(a *Args) error {
	pid, err := core.ParsePhase(a.Str("phase"))
	if err != nil {
		return &UsageError{err.Error()}
	}
	report := a.Str("report")
	text, err := os.ReadFile(report)
	if err != nil {
		return core.Gate("Report file %s not found.", report)
	}
	rtype := a.Str("type")
	if (pid.Name == "spec") != (rtype == core.SpecReview) {
		return core.Gate("The specification review is `--phase spec --type specification`; phases and the final review " +
			"take functional and security reviews.")
	}
	var rid string
	verdict := a.Str("verdict")
	err = store.Edit(func(st *core.State) error {
		var specsHash string
		switch pid.Name {
		case "spec":
			if st.Stage != "specification" {
				return core.Gate("The specification review happens in the specification stage.")
			}
			incomplete := []string{}
			for _, k := range core.Specs {
				lines, err := core.TodoLines(store, k.K)
				if err != nil {
					return err
				}
				if len(lines) > 0 {
					incomplete = append(incomplete, k.K)
				}
			}
			if len(incomplete) > 0 {
				return core.Gate("The specification is not complete (TODOs in %s). It is reviewed once everything is written.",
					strings.Join(incomplete, ", "))
			}
			if len(st.Phases) == 0 {
				return core.Gate("The phase plan is part of the specification: add phases first.")
			}
			talks := []string{}
			for _, q := range core.OpenQuestions(st) {
				if q.Kind != "validation" {
					talks = append(talks, q.ID)
				}
			}
			if len(talks) > 0 {
				return core.Gate("Questions still open (%s): the specification is not final yet.", strings.Join(talks, ", "))
			}
			h, err := core.SpecsHash(st, store)
			if err != nil {
				return err
			}
			specsHash = h
		case "final":
			if st.Stage != "final-review" {
				return core.Gate("Final reviews are recorded in the final-review stage (`final start`).")
			}
		default:
			p, err := core.GetPhase(st, pid.Num)
			if err != nil {
				return err
			}
			if p.Status != "in-progress" {
				return core.Gate("Phase %d is %s; reviews are recorded while it is in-progress.", pid.Num, p.Status)
			}
		}
		agent, err := core.ActorID(st)
		if err != nil {
			return err
		}
		rid = core.NextID(st, "R")
		slug := pid.String()
		if pid.IsNum() {
			slug = fmt.Sprintf("phase-%d", pid.Num)
		}
		dest := filepath.Join(store.Dir, "reviews", fmt.Sprintf("%s-%s-%s.md", rid, slug, rtype))
		if err := os.WriteFile(dest, text, 0o644); err != nil {
			return err
		}
		rel, _ := filepath.Rel(store.Root, dest)
		st.Reviews = append(st.Reviews, &core.Review{ID: rid, Phase: pid, Type: rtype, Verdict: verdict,
			Summary: a.Str("summary"), Reviewer: firstNonEmpty(a.Str("reviewer"), "agent"), Agent: agent,
			Report: rel, At: core.Now(), SpecsHash: specsHash})
		core.Log(st, fmt.Sprintf("%s: %s review of %s: %s.", rid, rtype, core.Where(pid), verdict), "review")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("%s recorded (%s). Record each finding with `finding add`.\n", rid, verdict)
	return nil
}

func cmdFindingAdd(a *Args) error {
	pid, err := core.ParsePhase(a.Str("phase"))
	if err != nil {
		return &UsageError{err.Error()}
	}
	review, sev := a.Str("review"), a.Str("severity")
	if (pid.Name == "spec") != (review == core.SpecReview) {
		return core.Gate("Specification findings are `--phase spec --review specification`.")
	}
	if core.Blocking[sev] && (a.Str("scenario") == "" || a.Str("recommendation") == "") {
		return core.Gate("A %s finding blocks the project and the user must decide on it: give --scenario (what an "+
			"input or user does, and what goes wrong) and --recommendation (the fix you propose).", sev)
	}
	var fid string
	var q *core.Question
	err = store.Edit(func(st *core.State) error {
		if pid.IsNum() {
			if _, err := core.GetPhase(st, pid.Num); err != nil {
				return err
			}
		}
		foundBy, err := core.ActorID(st)
		if err != nil {
			return err
		}
		fid = core.NextID(st, "F")
		f := &core.Finding{ID: fid, Phase: pid, Review: review, Severity: sev, Title: a.Str("title"),
			Detail: a.Str("detail"), Location: a.Str("location"), Scenario: a.Str("scenario"),
			Recommendation: a.Str("recommendation"), Status: "open", At: core.Now(), FoundBy: foundBy}
		st.Findings = append(st.Findings, f)
		core.Log(st, fmt.Sprintf("%s (%s, %s): %s", fid, sev, review, a.Str("title")), "finding")
		if !core.Blocking[sev] {
			return nil
		}
		spec := a.Str("spec")
		if spec == "" && review == "security" {
			spec = "security"
		}
		q, err = core.AddQuestion(st, "decision", fmt.Sprintf("%s: %s", fid, a.Str("title")), core.TriageBody(st, f),
			[]string{core.TriageFix, core.TriageRedesign, core.TriageAccept, core.TriageStop},
			&core.Effect{Type: "finding-triage", Finding: fid},
			core.Context{Stage: st.Stage, Phase: &pid, Finding: fid, Spec: spec}, true, core.TriageHelp)
		return err
	})
	if err != nil {
		return err
	}
	if q != nil {
		a.Printf("%s recorded. BLOCKING: development stops. %s asks the user to decide. Stop writing code and `wait %s`.\n",
			fid, q.ID, q.ID)
	} else {
		a.Printf("%s recorded.\n", fid)
	}
	return nil
}

func cmdFindingResolve(a *Args) error {
	id, status, note := a.Positional("id"), a.Str("status"), a.Str("note")
	err := store.Edit(func(st *core.State) error {
		var f *core.Finding
		for _, x := range st.Findings {
			if x.ID == id {
				f = x
			}
		}
		if f == nil {
			return core.Gate("No finding %s.", id)
		}
		if !core.Unresolved[f.Status] {
			return core.Gate("%s is already %s.", f.ID, f.Status)
		}
		if core.Blocking[f.Severity] {
			if status != "fixed" {
				return core.Gate("%s is %s: only the user can accept it, through its triage question.", f.ID, f.Severity)
			}
			for _, q := range st.Questions {
				if q.Status == "open" && q.Effect != nil && q.Effect.Finding == f.ID {
					return core.Gate("The user has not decided on %s yet. Wait for the triage answer.", f.ID)
				}
			}
		}
		f.Status, f.Resolution, f.ResolvedAt = status, note, strPtr(core.Now())
		core.Log(st, fmt.Sprintf("%s %s: %s", f.ID, status, note), "finding")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("%s %s.\n", id, status)
	return nil
}

func cmdAsk(a *Args) error {
	body := a.Str("body")
	if f := a.Str("body-file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		body = string(b)
	}
	var q *core.Question
	err := store.Edit(func(st *core.State) error {
		ctx := core.Context{Stage: st.Stage, Spec: a.Str("spec")}
		if v := a.Str("phase"); v != "" {
			p, err := core.ParsePhase(v)
			if err != nil {
				return &UsageError{err.Error()}
			}
			ctx.Phase = &p
		}
		options, help, err := core.ParseOptions(a.List("option"))
		if err != nil {
			return err
		}
		q, err = core.AddQuestion(st, firstNonEmpty(a.Str("kind"), "question"), a.Str("title"), body, options, nil, ctx,
			!a.Flag("no-free"), help)
		return err
	})
	if err != nil {
		return err
	}
	a.Println(q.ID)
	return nil
}

// cmdReply is the agent's side of a "Talk about this" thread; it can also clarify the question.
func cmdReply(a *Args) error {
	id := a.Positional("id")
	text := a.Str("message")
	if f := a.Str("message-file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		text = string(b)
	}
	if strings.TrimSpace(text) == "" {
		return core.Gate("Give --message or --message-file.")
	}
	err := store.Edit(func(st *core.State) error {
		q, err := core.GetQuestion(st, id)
		if err != nil {
			return err
		}
		if q.Status != "open" {
			return core.Gate("%s is no longer open.", q.ID)
		}
		by := core.Orchestrator
		if who, err := core.ActorID(st); err != nil {
			return err
		} else if who != nil {
			by = *who
		}
		q.Thread = append(q.Thread, &core.Msg{By: by, Text: strings.TrimSpace(text), At: core.Now()})
		changed := []string{}
		if a.Has("body") || a.Has("body-file") {
			q.Body = a.Str("body")
			if f := a.Str("body-file"); f != "" {
				b, err := os.ReadFile(f)
				if err != nil {
					return err
				}
				q.Body = string(b)
			}
			changed = append(changed, "question text")
		}
		if opts := a.List("option"); len(opts) > 0 {
			if q.Effect != nil {
				return core.Gate("%s's options drive the framework and cannot be changed; explain them in the message instead.", q.ID)
			}
			if q.Options, q.OptionHelp, err = core.ParseOptions(opts); err != nil {
				return err
			}
			changed = append(changed, "options")
		}
		msg := q.ID + ": agent replied."
		if len(changed) > 0 {
			msg = fmt.Sprintf("%s: agent replied and revised the %s.", q.ID, strings.Join(changed, " and "))
		}
		core.Log(st, msg, "question")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("Reply added to %s; it is still open. `wait %s` for the answer or the next message.\n", id, id)
	return nil
}

// cmdWait blocks until questions are answered in the console. Meant to run as a background
// command: it exits, and so wakes the agent, as soon as the user answers.
func cmdWait(a *Args) error {
	var deadline time.Time
	if t := a.Int("timeout"); t > 0 {
		deadline = time.Now().Add(time.Duration(t) * time.Second)
	}
	st, err := store.Read()
	if err != nil {
		return err
	}
	ids := a.Positionals("ids")
	first := a.Flag("any") || len(ids) == 0
	if len(ids) == 0 {
		for _, q := range core.OpenQuestions(st) {
			ids = append(ids, q.ID)
		}
	}
	if len(ids) == 0 {
		next, err := core.NextAction(st, store)
		if err != nil {
			return err
		}
		a.Printf("No open questions.\nNext: %s\n", next)
		return nil
	}
	for {
		st, err := store.Read()
		if err != nil {
			return err
		}
		qs := map[string]*core.Question{}
		for _, q := range st.Questions {
			qs[q.ID] = q
		}
		missing, done, pending, talk := []string{}, []string{}, []string{}, []string{}
		for _, i := range ids {
			q, ok := qs[i]
			switch {
			case !ok:
				missing = append(missing, i)
			case q.Status != "open":
				done = append(done, i)
			default:
				pending = append(pending, i)
			}
			// A message in a "Talk about this" thread always wakes the agent: the user is waiting on it.
			if ok && core.Discussing(q) {
				talk = append(talk, i)
			}
		}
		if len(missing) > 0 {
			return core.Gate("No question %s.", strings.Join(missing, ", "))
		}
		if len(talk) > 0 || (len(done) > 0 && (first || len(pending) == 0)) {
			out := []any{}
			superseded := false
			for _, i := range done {
				q := qs[i]
				if q.Answer != nil {
					out = append(out, waitAnswer{q.ID, q.Title, q.Answer.Choice, q.Answer.Comment, q.Answer.By, q.Answer.At})
				} else {
					out = append(out, waitWithdrawn{q.ID, q.Title, true})
				}
				superseded = superseded || q.Status == "superseded"
			}
			for _, i := range talk {
				out = append(out, waitTalk{i, qs[i].Title, "discussing", qs[i].Thread})
			}
			var v any = out
			if len(out) == 1 {
				v = out[0]
			}
			b, err := core.MarshalIndent(v)
			if err != nil {
				return err
			}
			a.Println(string(b))
			if len(talk) > 0 {
				a.Printf("The user chose \"Talk about this\" on %s: it is NOT answered. Read the thread, answer them with "+
					"`reply <id> --message ...` (add --body/--option to clarify the question itself), then `wait` again.\n",
					strings.Join(talk, ", "))
			}
			if superseded {
				a.Println("A question was withdrawn because what it asked about changed. Do not treat it as answered.")
			}
			if len(pending) > 0 {
				a.Printf("Still open: %s. Act on the answers above, then wait again.\n", strings.Join(pending, ", "))
			}
			next, err := core.NextAction(st, store)
			if err != nil {
				return err
			}
			a.Printf("Next: %s\n", next)
			return nil
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			a.Printf("Still waiting on %s. Run `wait` again; do not proceed without the answer, and do not ask for it in the chat.\n",
				strings.Join(pending, ", "))
			return exitCode(3)
		}
		time.Sleep(2 * time.Second)
	}
}

type waitAnswer struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Choice  string `json:"choice"`
	Comment string `json:"comment"`
	By      string `json:"by"`
	At      string `json:"at"`
}

type waitWithdrawn struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Withdrawn bool   `json:"withdrawn"`
}

type waitTalk struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Status string      `json:"status"`
	Thread []*core.Msg `json:"thread"`
}

func cmdAnswer(a *Args) error {
	id := a.Positional("id")
	err := store.Edit(func(st *core.State) error {
		_, err := core.AnswerQuestion(st, id, a.Str("choice"), a.Str("comment"), "user (terminal)")
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s answered on the user's behalf; the console shows it as given in the terminal.\n", id)
	return nil
}

func cmdDecide(a *Args) error {
	var did string
	err := store.Edit(func(st *core.State) error {
		agent, err := core.ActorID(st)
		if err != nil {
			return err
		}
		var phase *core.PhaseRef
		if v := a.Str("phase"); v != "" {
			p, err := core.ParsePhase(v)
			if err != nil {
				return &UsageError{err.Error()}
			}
			phase = &p
		}
		did = core.NextID(st, "D")
		st.Decisions = append(st.Decisions, &core.Decision{ID: did, Title: a.Str("title"), Decision: a.Str("decision"),
			Rationale: a.Str("rationale"), By: "agent", Agent: agent, Spec: strPtr(a.Str("spec")), Phase: phase, At: core.Now()})
		core.Log(st, fmt.Sprintf("%s recorded by agent: %s", did, a.Str("title")), "decision")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("%s recorded.\n", did)
	return nil
}

func cmdDecisions(a *Args) error {
	st, err := store.Read()
	if err != nil {
		return err
	}
	qs := map[string]*core.Question{}
	for _, q := range st.Questions {
		qs[q.ID] = q
	}
	a.Printf("# Decision log: %s\n\n", st.Project.Name)
	if len(st.Decisions) == 0 {
		a.Println("No decisions recorded.")
	}
	for _, d := range st.Decisions {
		about := []string{}
		if d.Spec != nil && *d.Spec != "" {
			about = append(about, "spec: "+*d.Spec)
		}
		if d.Phase != nil {
			about = append(about, core.Where(*d.Phase))
		}
		a.Printf("## %s: %s\n\n", d.ID, d.Title)
		line := fmt.Sprintf("- Decided by: %s, %s", d.By, d.At)
		if len(about) > 0 {
			line += " (" + strings.Join(about, ", ") + ")"
		}
		a.Println(line)
		var q *core.Question
		if d.Source != nil {
			q = qs[*d.Source]
		}
		if q != nil && q.Body != "" {
			lines := []string{}
			for _, l := range strings.Split(strings.TrimSuffix(strings.ReplaceAll(q.Body, "\r\n", "\n"), "\n"), "\n") {
				lines = append(lines, "  > "+l)
			}
			a.Println("- Question asked:\n\n" + strings.Join(lines, "\n"))
			if len(q.Options) > 0 {
				a.Printf("\n  Options: %s\n", strings.Join(q.Options, " / "))
			}
		}
		if q != nil && len(q.Thread) > 0 {
			a.Println("- Discussed before answering:")
			for _, m := range q.Thread {
				who := m.By
				if who == "user" {
					who = "User"
				}
				a.Printf("\n  %s: %s\n", who, strings.Join(strings.Split(strings.TrimSuffix(m.Text, "\n"), "\n"), "\n  "))
			}
		}
		a.Printf("- Decision: %s\n", d.Decision)
		if d.Rationale != "" {
			a.Printf("- Rationale or comment: %s\n", d.Rationale)
		}
		a.Println()
	}
	return nil
}

// cmdModels shows the models of the current project's kind; they are machine-wide.
func cmdModels(a *Args) error {
	kind := core.Software
	if store != nil {
		if st, err := store.Read(); err == nil {
			kind = st.Template
		}
	}
	if sets := a.List("set"); len(sets) > 0 {
		roles := map[string]string{}
		for _, item := range sets {
			role, model, _ := strings.Cut(item, "=")
			roles[strings.TrimSpace(role)] = strings.TrimSpace(model)
		}
		if _, err := core.SettingsUpdate(map[string]map[string]string{kind: roles}, nil); err != nil {
			return err
		}
	}
	models, _ := core.SettingsRead().Models.Get(kind)
	k := core.KindByID(kind)
	if a.Flag("json") {
		var out core.OMap[core.OMap[string]]
		for _, r := range models.Keys() {
			alias, _ := models.Get(r)
			m, _ := core.ModelByAlias(alias)
			var e core.OMap[string]
			e.Set("model", alias)
			e.Set("name", m.Name)
			e.Set("id", m.ID)
			out.Set(r, e)
		}
		b, err := core.MarshalIndent(out)
		if err != nil {
			return err
		}
		a.Println(string(b))
		return nil
	}
	a.Printf("Models for %s projects (console Settings page):\n", strings.ToLower(k.Label))
	for _, r := range models.Keys() {
		alias, _ := models.Get(r)
		role, _ := k.Role(r)
		a.Printf("  %-24s %-7s %-11s %s\n", r, alias, core.ModelName(alias), role.Label)
	}
	a.Println("\nPass the alias (second column) as the Agent tool's `model` when you spawn that role. " +
		"The session itself runs the first role's model: if it differs, ask the user to `/model <alias>`.")
	return nil
}

// cmdInbox: for CLIs without hooks, read (and mark delivered) the messages sent from the console.
func cmdInbox(a *Args) error {
	var msg string
	if err := store.Edit(func(st *core.State) error { msg = core.TakeMessages(st); return nil }); err != nil {
		return err
	}
	a.Println(firstNonEmpty(msg, "No new messages from the console."))
	return nil
}

func cmdAgentStart(a *Args) error {
	var ag *core.Agent
	err := store.Edit(func(st *core.State) error {
		if st.Template != core.Software {
			return core.Gate("A single-agent task has one agent; helpers you spawn are not tracked.")
		}
		var phase *core.PhaseRef
		if v := a.Str("phase"); v != "" {
			p, err := core.ParsePhase(v)
			if err != nil {
				return &UsageError{err.Error()}
			}
			phase = &p
		}
		ag = core.NewAgent(st, a.Str("role"), a.Str("task"), phase, true)
		return nil
	})
	if err != nil {
		return err
	}
	role, _ := core.KindByID(core.Software).Role(ag.Role)
	a.Printf("%s %s: %s on %s (Agent tool model: %s).\n", ag.ID, ag.Name, strings.ToLower(role.Label), core.ModelName(ag.Model), ag.Model)
	a.Printf("Tell the subagent its name is %s and to run every lacarta command as `%s --agent %s <command>`.\n", ag.Name, self(), ag.ID)
	a.Printf("When it returns: `agent done %s --summary \"...\"` (or `--failed`).\n", ag.ID)
	return nil
}

func cmdAgentUpdate(a *Args) error {
	id := a.Positional("id")
	err := store.Edit(func(st *core.State) error {
		ag, err := core.GetAgent(st, id)
		if err != nil {
			return err
		}
		if ag.Status == "done" || ag.Status == "failed" {
			return core.Gate("%s is %s.", ag.Name, ag.Status)
		}
		if t := a.Str("task"); t != "" {
			ag.Task = t
		}
		if s := a.Str("status"); s != "" {
			ag.Status = s
		}
		ag.UpdatedAt = core.Now()
		core.Log(st, fmt.Sprintf("%s: %s, %s", ag.Name, ag.Status, ag.Task), "agent")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("%s updated.\n", id)
	return nil
}

func cmdAgentDone(a *Args) error {
	id := a.Positional("id")
	failed := a.Flag("failed")
	err := store.Edit(func(st *core.State) error {
		ag, err := core.GetAgent(st, id)
		if err != nil {
			return err
		}
		if ag.ID == core.Orchestrator {
			return core.Gate("The orchestrator finishes with the project.")
		}
		if ag.Status == "done" || ag.Status == "failed" {
			return core.Gate("%s is already %s.", ag.Name, ag.Status)
		}
		ag.Status = "done"
		if failed {
			ag.Status = "failed"
		}
		ag.Summary, ag.FinishedAt, ag.UpdatedAt = a.Str("summary"), strPtr(core.Now()), core.Now()
		msg := ag.Name + " " + ag.Status + "."
		if ag.Summary != "" {
			msg = fmt.Sprintf("%s %s: %s", ag.Name, ag.Status, ag.Summary)
		}
		core.Log(st, msg, "agent")
		return nil
	})
	if err != nil {
		return err
	}
	if failed {
		a.Printf("%s failed.\n", id)
	} else {
		a.Printf("%s done.\n", id)
	}
	return nil
}

func cmdAgentRename(a *Args) error {
	id := a.Positional("id")
	name := strings.TrimSpace(a.Str("name"))
	err := store.Edit(func(st *core.State) error {
		ag, err := core.GetAgent(st, id)
		if err != nil {
			return err
		}
		if name == "" || len([]rune(name)) > 40 {
			return core.Gate("A name is 1 to 40 characters.")
		}
		for _, x := range st.Agents {
			if x.Name == name && x.ID != ag.ID {
				return core.Gate("%s is taken.", name)
			}
		}
		old := ag.Name
		ag.Name = name
		core.Log(st, fmt.Sprintf("%s is now called %s.", old, name), "agent")
		return nil
	})
	if err != nil {
		return err
	}
	a.Printf("%s is %s.\n", id, name)
	return nil
}

func cmdAgentList(a *Args) error {
	st, err := store.Read()
	if err != nil {
		return err
	}
	for _, ag := range st.Agents {
		a.Printf("  %-5s %-10s %-24s %-7s %-8s %s\n", ag.ID, ag.Name, ag.Role, ag.Model, core.AgentStatus(st, ag), ag.Task)
	}
	return nil
}

func cmdFinalStart(a *Args) error {
	err := store.Edit(func(st *core.State) error {
		if r := core.BlockedReason(st); r != "" {
			return core.Gate("Blocked. %s", r)
		}
		if st.Stage != "implementation" {
			return core.Gate("The final review follows implementation (now: %s).", st.Stage)
		}
		left := []int{}
		for _, p := range st.Phases {
			if p.Status != "done" {
				left = append(left, p.ID)
			}
		}
		if len(left) > 0 || len(st.Phases) == 0 {
			what := "no phases"
			if len(left) > 0 {
				what = core.PyIntList(left)
			}
			return core.Gate("Phases not done: %s.", what)
		}
		st.Stage = "final-review"
		core.Log(st, "Final review started.", "stage")
		return nil
	})
	if err != nil {
		return err
	}
	a.Println("Stage: final-review. Spawn fresh reviewers as described in references/final-review.md.")
	return nil
}

func cmdFinalComplete(a *Args) error {
	var q *core.Question
	err := store.Edit(func(st *core.State) error {
		if r := core.BlockedReason(st); r != "" {
			return core.Gate("Blocked. %s", r)
		}
		if st.Stage != "final-review" {
			return core.Gate("Not in final review (now: %s).", st.Stage)
		}
		for _, t := range core.ReviewTypes {
			if r := core.LatestReview(st, core.PhaseRef{Name: "final"}, t); r == nil || r.Verdict != "pass" {
				return core.Gate("The final %s review has not passed.", t)
			}
		}
		left := []string{}
		for _, f := range st.Findings {
			if f.Phase == (core.PhaseRef{Name: "final"}) && core.Unresolved[f.Status] {
				left = append(left, f.ID)
			}
		}
		if len(left) > 0 {
			return core.Gate("Unresolved final findings: %s.", strings.Join(left, ", "))
		}
		var err error
		q, err = core.AddQuestion(st, "validation", "Approve the final review and close the project?",
			firstNonEmpty(a.Str("summary"), "Both final reviews passed."), []string{core.Approve, core.RequestChanges},
			&core.Effect{Type: "final-approval"}, core.Context{Stage: "final-review", Phase: &core.PhaseRef{Name: "final"}},
			false, core.ApprovalHelp)
		return err
	})
	if err != nil {
		return err
	}
	a.Printf("%s asks for final approval. `wait %s`.\n", q.ID, q.ID)
	return nil
}
