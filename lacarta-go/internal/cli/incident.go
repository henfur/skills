package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lacarta/internal/core"
	"lacarta/internal/incident"
	"lacarta/internal/plugins"
	"lacarta/internal/server"
	"lacarta/skill"
)

// Commands of the incident-response kind. Docs: docs/design/incident-response.md.

func incidentCommands() []*Cmd {
	sections := []string{}
	for _, s := range core.IncidentSections {
		sections = append(sections, s.K)
	}
	text := []Opt{{Name: "text", Help: "The text (markdown)."}, {Name: "file", Help: "A file holding the text."}}
	ev := Opt{Name: "evidence", Kind: list, Help: "An evidence id it rests on (EV-n); repeat."}
	return []*Cmd{
		{Name: "incident init", Help: "Open an incident-response case in a folder: state, evidence/, reports/, report templates.",
			Run: cmdIncidentInit, Opts: []Opt{{Name: "folder", Help: "The case folder (default: the current folder)."},
				{Name: "name", Help: "The case's title."}, {Name: "type", Choices: core.IncidentTypes, Help: "Default: incident."},
				{Name: "severity", Required: true, Choices: core.Severities}, {Name: "tlp", Choices: core.TLPs, Help: "Default: AMBER."},
				{Name: "ref", Kind: list, Help: "A CVE id or ticket number; repeat."},
				{Name: "affected", Kind: list, Help: "An affected system, service or account; repeat."}}},
		{Name: "incident guide", Help: "How the coordinator runs a case. Read it first.", Run: cmdIncidentGuide},
		{Name: "incident update", Help: "Change what the case covers: references, affected systems, TLP, report cadence.",
			Run: cmdIncidentUpdate, Opts: []Opt{{Name: "add-ref", Kind: list}, {Name: "remove-ref", Kind: list},
				{Name: "add-affected", Kind: list}, {Name: "remove-affected", Kind: list}, {Name: "tlp", Choices: core.TLPs},
				{Name: "sitrep-every", Help: "How often a situation report is due, like 4h or 30m; `off` for none."}}},
		{Name: "incident severity", Help: "Ask the lead to change the severity.", Run: cmdIncidentSeverity,
			Pos: []Pos{{Name: "severity", Choices: core.Severities}}, Opts: []Opt{{Name: "reason", Required: true}}},
		{Name: "incident note", Help: "Write one narrative section of the case; the reports print them.", Run: cmdIncidentNote,
			Pos: []Pos{{Name: "section", Choices: sections}}, Opts: text},
		{Name: "incident timeline add", Help: "Put an event on the timeline, at the time it happened.", Run: cmdTimelineAdd, Opts: []Opt{
			{Name: "at", Required: true, Help: "When it happened: 2026-11-18T14:05:00Z (UTC without an offset), or `now`."},
			{Name: "text", Required: true}, {Name: "source", Help: "Who or what says so, when there is no evidence item."}, ev,
			{Name: "suspected", Kind: flag, Help: "Not established yet."},
			{Name: "mark", Choices: core.IncidentMarks, Help: "The milestone this event is."}}},
		{Name: "incident timeline retract", Help: "Withdraw an event that turned out wrong (it stays in the record).",
			Run: cmdTimelineRetract, Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "reason", Required: true}}},
		{Name: "incident timeline list", Help: "The timeline, oldest first.", Run: cmdTimelineList},
		{Name: "incident evidence add", Help: "Register evidence: the file is copied into evidence/, made read-only and hashed.",
			Run: cmdEvidenceAdd, Opts: []Opt{{Name: "title", Required: true}, {Name: "kind", Required: true, Choices: core.EvidenceKinds},
				{Name: "source", Required: true, Help: "The host, system or person it comes from."},
				{Name: "file", Help: "The file to register."}, {Name: "text", Help: "A statement or observation to store as a file."},
				{Name: "ref", Help: "Where it is kept, when it stays in another system (a query, a ticket, an image)."},
				{Name: "collected-at", Help: "When it was collected (default: now)."}, {Name: "collected-by", Help: "Who collected it (default: you)."},
				{Name: "note"}}},
		{Name: "incident evidence list", Help: "The evidence registered in the case.", Run: cmdEvidenceList},
		{Name: "incident verify", Help: "Check every evidence file and issued report against its recorded sha256.", Run: cmdIncidentVerify},
		{Name: "incident investigation open", Help: "Open an investigation: one question to answer.", Run: cmdInvOpen,
			Opts: []Opt{{Name: "title", Required: true, Help: "The question."}, {Name: "hypothesis"}}},
		{Name: "incident investigation note", Help: "Record a step of an investigation.", Run: cmdInvNote,
			Pos: []Pos{{Name: "id"}}, Opts: append(append([]Opt{}, text...), ev)},
		{Name: "incident investigation conclude", Help: "Record an investigation's conclusion. Confirmed and refuted need evidence.",
			Run: cmdInvConclude, Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "status", Required: true, Choices: core.InvConclusion},
				{Name: "conclusion", Required: true}, ev}},
		{Name: "incident investigation challenge", Help: "A fresh agent's verdict on a confirmed conclusion.", Run: cmdInvChallenge,
			Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "verdict", Required: true, Choices: []string{"holds", "fails"}}, {Name: "note", Required: true}}},
		{Name: "incident investigation reopen", Run: cmdInvReopen, Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "reason", Required: true}}},
		{Name: "incident investigation list", Help: "Every investigation with its state.", Run: cmdInvList},
		{Name: "incident investigation brief", Help: "The whole prompt of the subagent that works, or challenges, an investigation.",
			Run: cmdInvBrief, Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "role", Choices: []string{"investigator", "challenger"}},
				{Name: "for", Help: "The subagent's id from `agent start`."}}},
		{Name: "incident indicator add", Help: "Add an indicator of compromise (or a CVE, an account, a file name).", Run: cmdIndicatorAdd,
			Opts: []Opt{{Name: "type", Required: true, Choices: core.IndicatorType}, {Name: "value", Required: true}, {Name: "context"}, ev}},
		{Name: "incident indicator retire", Help: "Mark an indicator as not one of this case.", Run: cmdIndicatorRetire,
			Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "reason", Required: true}}},
		{Name: "incident indicator list", Help: "The active indicators, defanged.", Run: cmdIndicatorList,
			Opts: []Opt{{Name: "plain", Kind: flag, Help: "Real values, one `type value` per line, for a blocklist or a search."}}},
		{Name: "incident action propose", Help: "Propose a response action; the lead approves it before anyone carries it out.",
			Run: cmdActionPropose, Opts: []Opt{{Name: "kind", Required: true, Choices: core.ActionKinds}, {Name: "title", Required: true},
				{Name: "detail", Help: "Exactly what will be done, and on what."}, {Name: "risk", Help: "What it could break or alert."},
				{Name: "rollback", Help: "How to undo it."}, {Name: "executor", Help: "Who carries it out (default: agent)."}}},
		{Name: "incident action record", Help: "Record an action a person or a team already took.", Run: cmdActionRecord,
			Opts: []Opt{{Name: "kind", Required: true, Choices: core.ActionKinds}, {Name: "title", Required: true},
				{Name: "by", Required: true, Help: "Who did it."}, {Name: "result", Required: true}, {Name: "at", Help: "When (default: now)."},
				{Name: "mark", Choices: core.IncidentMarks}}},
		{Name: "incident action start", Help: "Start an approved action.", Run: cmdActionStart, Pos: []Pos{{Name: "id"}}},
		{Name: "incident action done", Help: "Close an action with its result; it goes on the timeline.", Run: cmdActionDone,
			Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "result", Required: true, Help: "What was done and how it was checked."},
				{Name: "failed", Kind: flag}, {Name: "mark", Choices: core.IncidentMarks, Help: "The milestone it reaches."}}},
		{Name: "incident action cancel", Run: cmdActionCancel, Pos: []Pos{{Name: "id"}}, Opts: []Opt{{Name: "reason", Required: true}}},
		{Name: "incident action list", Help: "Every action with its state.", Run: cmdActionList},
		{Name: "incident report sitrep", Help: "Draft a situation report from the case record and ask the lead to issue it.",
			Run: cmdReportSitrep, Opts: []Opt{{Name: "unchanged", Kind: flag, Help: "The situation text still holds since the last report."}}},
		{Name: "incident report final", Help: "Draft the final report; the lead's approval issues it and closes the case.",
			Run: cmdReportFinal, Opts: []Opt{{Name: "outcome", Required: true, Choices: core.Outcomes}}},
		{Name: "incident report list", Help: "The reports of the case.", Run: cmdReportList},
		{Name: "incident templates check", Help: "Render both report templates against the case as it is.", Run: cmdIncidentTemplates},
		{Name: "incident reopen", Help: "Open a closed case again.", Run: cmdIncidentReopen, Opts: []Opt{{Name: "reason", Required: true}}},
	}
}

// incidentEdit runs fn on the case's state, refusing other kinds of project.
func incidentEdit(fn func(st *core.State) error) error {
	return store.Edit(func(st *core.State) error {
		if st.Template != core.Incident || st.Incident == nil {
			return core.Gate("This command belongs to incident-response cases; this project is a %s. `incident init` opens a case in a folder.",
				strings.ToLower(core.KindByID(st.Template).Label))
		}
		return fn(st)
	})
}

func incidentRead() (*core.State, error) {
	st, err := store.Read()
	if err != nil {
		return nil, err
	}
	if st.Template != core.Incident || st.Incident == nil {
		return nil, core.Gate("This command belongs to incident-response cases; this project is a %s. `incident init` opens a case in a folder.",
			strings.ToLower(core.KindByID(st.Template).Label))
	}
	return st, nil
}

// textOpt reads --text or --file.
func textOpt(a *Args) (string, error) {
	if f := a.Str("file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", core.Gate("Can't read %s: %v", f, err)
		}
		return string(b), nil
	}
	return a.Str("text"), nil
}

func cmdIncidentInit(a *Args) error {
	root, _ := filepath.Abs(firstNonEmpty(a.Str("folder"), "."))
	if core.FindRoot(root) == root {
		return core.Gate("%s is already a lacarta project. A case has its own folder: pass `--folder <new folder>`.", root)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	name := firstNonEmpty(a.Str("name"), filepath.Base(root))
	pid, err := incident.Init(root, name, firstNonEmpty(a.Str("type"), "incident"), a.Str("severity"), firstNonEmpty(a.Str("tlp"), "AMBER"))
	if err != nil {
		return err
	}
	s := core.SoftwareStore(root)
	err = s.Edit(func(st *core.State) error {
		st.Incident.Refs = append(st.Incident.Refs, a.List("ref")...)
		st.Incident.Affected = append(st.Incident.Affected, a.List("affected")...)
		return nil
	})
	if err != nil {
		return err
	}
	if sid := os.Getenv("LACARTA_SESSION"); sid != "" {
		if err := core.MoveSession(sid, pid, s, os.Getenv("LACARTA_TRANSCRIPT")); err != nil {
			return err
		}
	}
	console := "not running: ask the user to run `lacarta serve` (do not start it yourself)"
	if info := server.EnsureDaemon(self()); info != nil {
		console = info.OpenURL
	}
	a.Printf("Case %s opened in %s\nConsole: %s\n", pid, root, console)
	a.Printf("Evidence goes to evidence/ (read-only, hashed, out of git), reports to reports/. Edit templates/ to fit your report format.\n")
	a.Printf("Next: read `%s incident guide`, then record what is known: `incident timeline add`, `incident evidence add`, "+
		"`incident investigation open`. Pass `--project %s` to lacarta commands run outside %s.\n", self(), pid, root)
	return nil
}

func cmdIncidentGuide(a *Args) error {
	b, err := fs.ReadFile(skill.Incident, "incident/guide.md")
	if err != nil {
		return err
	}
	a.Printf("%s", b)
	return nil
}

func incidentStatus(a *Args, st *core.State, d *core.Derived) error {
	i, id := st.Incident, d.Incident
	a.Printf("%s: %s %s, TLP:%s, %s (%s)\n", st.Project.Name, i.Severity, i.Type, i.TLP, st.Stage, id.PhaseLabel)
	a.Printf("Case folder: %s\n", st.Root)
	for _, m := range id.Marks {
		if m.At != "" {
			a.Printf("  %-26s %s\n", m.Label, m.At)
		}
	}
	a.Printf("Timeline: %d events · evidence: %d · indicators: %d · reports issued: %d\n", id.Counts["timeline"], id.Counts["evidence"],
		id.Counts["indicators"], id.Counts["reports"])
	if len(i.Investigations) > 0 {
		a.Println("Investigations:")
		for _, v := range i.Investigations {
			extra := ""
			if v.Status == "confirmed" && v.Challenge == nil {
				extra = " (not challenged yet)"
			}
			a.Printf("  %-7s %-13s %s%s\n", v.ID, v.Status, v.Title, extra)
		}
	}
	if len(i.Actions) > 0 {
		a.Println("Actions:")
		for _, x := range i.Actions {
			a.Printf("  %-7s %-12s %-13s %s (by %s)\n", x.ID, x.Status, x.Kind, x.Title, x.Executor)
		}
	}
	for _, q := range core.OpenQuestions(st) {
		a.Printf("  %s [%s] %s\n", q.ID, q.Kind, q.Title)
	}
	a.Printf("\nNext: %s\n", d.Next)
	return nil
}

func editList(xs, add, remove []string) []string {
	out := []string{}
	for _, x := range append(append([]string{}, xs...), add...) {
		x = strings.TrimSpace(x)
		drop := x == ""
		for _, r := range remove {
			drop = drop || strings.EqualFold(r, x)
		}
		for _, o := range out {
			drop = drop || strings.EqualFold(o, x)
		}
		if !drop {
			out = append(out, x)
		}
	}
	return out
}

func cmdIncidentUpdate(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		i := st.Incident
		if st.Stage == "closed" {
			return core.Gate("The case is closed. `incident reopen --reason ...` opens it again.")
		}
		i.Refs = editList(i.Refs, a.List("add-ref"), a.List("remove-ref"))
		i.Affected = editList(i.Affected, a.List("add-affected"), a.List("remove-affected"))
		if v := a.Str("tlp"); v != "" && v != i.TLP {
			core.Log(st, fmt.Sprintf("TLP changed from %s to %s.", i.TLP, v), "incident")
			i.TLP = v
		}
		if v := a.Str("sitrep-every"); v != "" {
			if v == "off" {
				i.SitrepEvery = ""
			} else if d, err := time.ParseDuration(v); err != nil || d < time.Minute {
				return core.Gate("--sitrep-every: a duration like 4h or 30m, or `off`.")
			} else {
				i.SitrepEvery = v
			}
		}
		a.Printf("References: %s\nAffected: %s\nTLP:%s, situation reports: %s\n", firstNonEmpty(strings.Join(i.Refs, ", "), "none"),
			firstNonEmpty(strings.Join(i.Affected, ", "), "none"), i.TLP, firstNonEmpty(i.SitrepEvery, "no cadence"))
		return nil
	})
}

func cmdIncidentSeverity(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		q, err := core.IncidentSeverity(st, a.Positional("severity"), a.Str("reason"))
		if err != nil {
			return err
		}
		a.Printf("%s asks the lead. The severity stays %s until they answer.\n", q.ID, st.Incident.Severity)
		return nil
	})
}

func cmdIncidentNote(a *Args) error {
	text, err := textOpt(a)
	if err != nil {
		return err
	}
	return incidentEdit(func(st *core.State) error {
		if err := core.IncidentNote(st, a.Positional("section"), text); err != nil {
			return err
		}
		a.Printf("%s updated.\n", core.SectionTitle(a.Positional("section")))
		return nil
	})
}

func cmdTimelineAdd(a *Args) error {
	at, err := core.ParseWhen(a.Str("at"))
	if err != nil {
		return err
	}
	return incidentEdit(func(st *core.State) error {
		ev := &core.IncidentEvent{At: at, Text: a.Str("text"), Source: a.Str("source"), Evidence: a.List("evidence"), Mark: a.Str("mark")}
		if a.Flag("suspected") {
			ev.Confidence = "suspected"
		}
		if err := core.TimelineAdd(st, ev); err != nil {
			return err
		}
		a.Printf("%s %s %s\n", ev.ID, ev.At, ev.Confidence)
		return nil
	})
}

func cmdTimelineRetract(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		if err := core.TimelineRetract(st, a.Positional("id"), a.Str("reason")); err != nil {
			return err
		}
		a.Printf("%s withdrawn.\n", a.Positional("id"))
		return nil
	})
}

func cmdTimelineList(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	for _, ev := range core.SortedTimeline(st.Incident) {
		mark := ""
		if ev.Mark != "" {
			mark = " [" + core.MarkLabel(st.Incident.Type, ev.Mark) + "]"
		}
		basis := firstNonEmpty(strings.Join(ev.Evidence, ","), ev.Source)
		a.Printf("%s  %-6s %-9s %s%s (%s)\n", ev.At, ev.ID, ev.Confidence, ev.Text, mark, basis)
	}
	return nil
}

func cmdEvidenceAdd(a *Args) error {
	given := 0
	for _, k := range []string{"file", "text", "ref"} {
		if a.Str(k) != "" {
			given++
		}
	}
	if given != 1 {
		return core.Gate("Evidence is one of: a file (`--file`), a text (`--text`) or a pointer to where it is kept (`--ref`).")
	}
	at, err := core.ParseWhen(a.Str("collected-at"))
	if err != nil {
		return err
	}
	return incidentEdit(func(st *core.State) error {
		e := &core.Evidence{ID: core.NextID(st, "EV"), Title: a.Str("title"), Kind: a.Str("kind"), Source: a.Str("source"),
			CollectedAt: at, CollectedBy: a.Str("collected-by"), Ref: a.Str("ref"), Note: a.Str("note")}
		if e.Ref == "" {
			e.File = "pending"
		}
		if err := core.EvidenceAdd(st, e); err != nil {
			return err
		}
		if e.Ref == "" {
			var err error
			if e.File, e.SHA256, e.Size, err = incident.StoreEvidence(st.Root, e.ID, a.Str("file"), a.Str("text")); err != nil {
				return err
			}
			a.Printf("%s %s sha256 %s (%d bytes)\n", e.ID, e.File, e.SHA256, e.Size)
			return nil
		}
		a.Printf("%s kept at %s (no file here: nothing to hash)\n", e.ID, e.Ref)
		return nil
	})
}

func cmdEvidenceList(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	for _, e := range st.Incident.Evidence {
		a.Printf("%-6s %-15s %s (from %s, collected %s) %s\n", e.ID, e.Kind, e.Title, e.Source, e.CollectedAt, firstNonEmpty(e.File, "at "+e.Ref))
	}
	return nil
}

func cmdIncidentVerify(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	problems := incident.Verify(st)
	for _, p := range problems {
		a.Println(p)
	}
	if len(problems) > 0 {
		return core.Gate("%d file(s) don't match the record. Tell the lead: evidence that changed can't be relied on as it is.", len(problems))
	}
	a.Println("Every evidence file and issued report matches its recorded sha256.")
	return nil
}

func cmdInvOpen(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		v, err := core.InvestigationOpen(st, a.Str("title"), a.Str("hypothesis"))
		if err != nil {
			return err
		}
		a.Printf("%s opened. Give it to an investigator: `agent start --role investigator --task \"%s\"`, then "+
			"`incident investigation brief %s --for <its id>`.\n", v.ID, v.ID, v.ID)
		return nil
	})
}

func cmdInvNote(a *Args) error {
	text, err := textOpt(a)
	if err != nil {
		return err
	}
	return incidentEdit(func(st *core.State) error {
		if err := core.InvestigationNote(st, a.Positional("id"), text, a.List("evidence")); err != nil {
			return err
		}
		a.Printf("Noted on %s.\n", a.Positional("id"))
		return nil
	})
}

func cmdInvConclude(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		id := a.Positional("id")
		if err := core.InvestigationConclude(st, id, a.Str("status"), a.Str("conclusion"), a.List("evidence")); err != nil {
			return err
		}
		a.Printf("%s %s.\n", id, a.Str("status"))
		if a.Str("status") == "confirmed" {
			a.Printf("Next: a fresh challenger tries to disprove it: `agent start --role challenger --task \"Challenge %s\"`, then "+
				"`incident investigation brief %s --role challenger --for <its id>`.\n", id, id)
		}
		return nil
	})
}

func cmdInvChallenge(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		id := a.Positional("id")
		if err := core.InvestigationChallenge(st, id, a.Str("verdict"), a.Str("note")); err != nil {
			return err
		}
		if a.Str("verdict") == "fails" {
			a.Printf("%s is open again: its conclusion did not hold.\n", id)
		} else {
			a.Printf("%s: the conclusion held.\n", id)
		}
		return nil
	})
}

func cmdInvReopen(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		if err := core.InvestigationReopen(st, a.Positional("id"), a.Str("reason")); err != nil {
			return err
		}
		a.Printf("%s is open again.\n", a.Positional("id"))
		return nil
	})
}

func cmdInvList(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	for _, v := range st.Incident.Investigations {
		a.Printf("%-7s %-13s %s\n", v.ID, v.Status, v.Title)
		if v.Conclusion != "" && v.Status != "open" {
			ch := ""
			if v.Challenge != nil {
				ch = " (challenge: " + v.Challenge.Verdict + ")"
			} else if v.Status == "confirmed" {
				ch = " (not challenged yet)"
			}
			a.Printf("        %s%s\n", v.Conclusion, ch)
		}
	}
	return nil
}

func cmdInvBrief(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	v, err := core.GetInvestigation(st, a.Positional("id"))
	if err != nil {
		return err
	}
	role := firstNonEmpty(a.Str("role"), "investigator")
	if role == "challenger" && v.Status != "confirmed" {
		return core.Gate("Only a confirmed conclusion is challenged (%s is %s).", v.ID, v.Status)
	}
	if id := a.Str("for"); id != "" {
		if _, err := core.GetAgent(st, id); err != nil {
			return err
		}
	}
	a.Printf("%s", incident.Brief(self(), st, v, role, a.Str("for")))
	if s := plugins.Instruction(plugins.ForRole(st, role)); s != "" {
		a.Printf("\n## Skills\n\n%s\n", s)
	}
	return nil
}

func cmdIndicatorAdd(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		in, err := core.IndicatorAdd(st, &core.Indicator{Type: a.Str("type"), Value: a.Str("value"), Context: a.Str("context"), Evidence: a.List("evidence")})
		if err != nil {
			return err
		}
		a.Printf("%s %s %s\n", in.ID, in.Type, core.Defang(in.Type, in.Value))
		return nil
	})
}

func cmdIndicatorRetire(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		if err := core.IndicatorRetire(st, a.Positional("id"), a.Str("reason")); err != nil {
			return err
		}
		a.Printf("%s retired.\n", a.Positional("id"))
		return nil
	})
}

func cmdIndicatorList(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	for _, in := range st.Incident.Indicators {
		if in.Status != "active" {
			continue
		}
		if a.Flag("plain") {
			a.Printf("%s %s\n", in.Type, in.Value)
		} else {
			a.Printf("%-7s %-8s %s  %s\n", in.ID, in.Type, core.Defang(in.Type, in.Value), in.Context)
		}
	}
	return nil
}

func cmdActionPropose(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		act := &core.IncidentAction{Kind: a.Str("kind"), Title: a.Str("title"), Detail: a.Str("detail"), Risk: a.Str("risk"),
			Rollback: a.Str("rollback"), Executor: a.Str("executor")}
		q, err := core.ActionPropose(st, act)
		if err != nil {
			return err
		}
		if q == nil {
			a.Printf("%s noted as a follow-up.\n", act.ID)
			return nil
		}
		a.Printf("%s proposed; %s asks the lead. Don't carry it out before they approve: `wait %s` as a background command, "+
			"and keep investigating meanwhile.\n", act.ID, q.ID, q.ID)
		return nil
	})
}

func cmdActionRecord(a *Args) error {
	at, err := core.ParseWhen(a.Str("at"))
	if err != nil {
		return err
	}
	return incidentEdit(func(st *core.State) error {
		act := &core.IncidentAction{Kind: a.Str("kind"), Title: a.Str("title"), Executor: a.Str("by"), Result: a.Str("result"), DoneAt: at}
		if err := core.ActionRecord(st, act); err != nil {
			return err
		}
		if act.Kind != "follow-up" || a.Str("mark") != "" {
			text := fmt.Sprintf("%s action by %s: %s. %s", strings.ToUpper(act.Kind[:1])+act.Kind[1:], act.Executor, act.Title, act.Result)
			if err := core.TimelineAdd(st, &core.IncidentEvent{At: at, Text: text, Source: act.ID, Mark: a.Str("mark")}); err != nil {
				return err
			}
		}
		a.Printf("%s recorded.\n", act.ID)
		return nil
	})
}

func cmdActionStart(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		act, err := core.ActionStart(st, a.Positional("id"))
		if err != nil {
			return err
		}
		a.Printf("%s started. Do exactly this, nothing more:\n%s\nThen `incident action done %s --result \"...\"`.\n", act.ID,
			firstNonEmpty(act.Detail, act.Title), act.ID)
		return nil
	})
}

func cmdActionDone(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		if err := core.ActionDone(st, a.Positional("id"), a.Str("result"), a.Flag("failed"), a.Str("mark")); err != nil {
			return err
		}
		a.Printf("%s closed.\n", a.Positional("id"))
		return nil
	})
}

func cmdActionCancel(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		if err := core.ActionCancel(st, a.Positional("id"), a.Str("reason")); err != nil {
			return err
		}
		a.Printf("%s cancelled.\n", a.Positional("id"))
		return nil
	})
}

func cmdActionList(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	for _, x := range st.Incident.Actions {
		a.Printf("%-7s %-12s %-13s %s (by %s)\n", x.ID, x.Status, x.Kind, x.Title, x.Executor)
	}
	return nil
}

// draftReport renders a report into reports/ and asks the lead to issue it.
func draftReport(a *Args, st *core.State, kind, outcome string) error {
	pid := deskPID(st)
	md, err := incident.Render(pid, st, kind, outcome)
	if err != nil {
		return err
	}
	rel := core.ReportFile(kind, core.ReportNumber(st.Incident, kind))
	path := filepath.Join(st.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return err
	}
	body := fmt.Sprintf("Drafted in `%s` from the case record. Read it in the **Reports** view, edit it there if needed, then decide.", rel)
	r, q, err := core.ReportSubmit(st, kind, rel, core.FileHash(path), body, outcome)
	if err != nil {
		return err
	}
	a.Printf("%s drafted in %s; %s asks the lead to issue it. `wait %s` as a background command.\n", r.ID, rel, q.ID, q.ID)
	return nil
}

func cmdReportSitrep(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		i := st.Incident
		if st.Stage != "active" {
			return core.Gate("Situation reports are for an open case (stage now: %s).", st.Stage)
		}
		sit := i.Narrative["situation"]
		if sit == nil {
			return core.Gate("Write the situation first: `incident note situation --file <md>` (what is known now, in plain words).")
		}
		if i.Narrative["next-steps"] == nil {
			return core.Gate("Write the next steps first: `incident note next-steps --text ...`.")
		}
		if last := core.LastIssued(i, ""); last != nil && sit.At <= last.At && !a.Flag("unchanged") {
			return core.Gate("The situation text is older than the last report (%s). Update it with `incident note situation`, "+
				"or pass `--unchanged` if it still holds.", last.ID)
		}
		return draftReport(a, st, "sitrep", "")
	})
}

func cmdReportFinal(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		if st.Stage == "closed" {
			return core.Gate("The case is closed.")
		}
		outcome := a.Str("outcome")
		blockers := core.FinalBlockers(st, outcome)
		if p := incident.Verify(st); len(p) > 0 {
			blockers = append(blockers, fmt.Sprintf("%d evidence or report file(s) don't match their recorded sha256 (`incident verify`)", len(p)))
		}
		if len(blockers) > 0 {
			return core.Gate("The final report can't be drafted yet:\n- %s", strings.Join(blockers, "\n- "))
		}
		return draftReport(a, st, "final", outcome)
	})
}

func cmdReportList(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	for _, r := range st.Incident.Reports {
		a.Printf("%-7s %-7s %-18s %s %s\n", r.ID, r.Kind, r.Status, r.File, firstNonEmpty(r.IssuedAt, r.At))
	}
	return nil
}

func cmdIncidentTemplates(a *Args) error {
	st, err := incidentRead()
	if err != nil {
		return err
	}
	if errs := incident.CheckTemplates(deskPID(st), st); len(errs) > 0 {
		return core.Gate("%s", strings.Join(errs, "\n"))
	}
	a.Println("Both report templates render.")
	return nil
}

func cmdIncidentReopen(a *Args) error {
	return incidentEdit(func(st *core.State) error {
		if err := core.IncidentReopen(st, a.Str("reason")); err != nil {
			return err
		}
		a.Println("The case is open again.")
		return nil
	})
}
