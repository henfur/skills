package core

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The incident-response kind follows one case: a security incident, or a vulnerability to respond
// to. The case folder holds the state (.lacarta/), the evidence (evidence/) and the reports
// (reports/). lacarta keeps the record: the timeline, the investigations and their conclusions,
// the evidence with its hashes, the indicators, the response actions, and the reports issued
// along the way. The lead (the user) decides what only a person may: every response action an
// agent would carry out, the severity, and every report that leaves the case.
// Docs: docs/design/incident-response.md.

const Incident = "incident"

var IncidentRoles = []Role{
	{"orchestrator", "Coordinator", "Whole case",
		"Coordinates the response: keeps the timeline, opens investigations, proposes actions, drafts the reports and " +
			"talks to you. This is the Claude Code session itself: switch it with /model.", "opus"},
	{"investigator", "Investigator", "Investigation",
		"Subagent that works one investigation: collects evidence, records what it finds, proposes a conclusion.", "sonnet"},
	{"challenger", "Challenger", "Investigation",
		"Fresh agent that tries to disprove a confirmed conclusion. Use a different model from the investigators, " +
			"so they check each other.", "opus"},
}

var (
	IncidentTypes = []string{"incident", "vulnerability"}
	TLPs          = []string{"CLEAR", "GREEN", "AMBER", "AMBER+STRICT", "RED"}

	// Narrative sections the agents draft and the reports print.
	IncidentSections = []KV{
		{"situation", "Situation"},
		{"impact", "Impact"},
		{"root-cause", "Root cause"},
		{"next-steps", "Next steps"},
		{"lessons", "Lessons learned"},
	}

	// Milestones of a case, in order. A timeline event carries at most one; the phase and the
	// response times come from them.
	IncidentMarks = []string{"occurred", "detected", "contained", "eradicated", "recovered"}
	markLabels    = map[string]map[string]string{
		"incident": {"occurred": "First malicious activity", "detected": "Detected", "contained": "Contained",
			"eradicated": "Eradicated", "recovered": "Recovered"},
		"vulnerability": {"occurred": "Introduced or disclosed", "detected": "Reported to us", "contained": "Mitigated",
			"eradicated": "Remediated", "recovered": "Fix verified"},
	}
	phaseLabels = map[string]map[string]string{
		"incident":      {"contain": "Identify & contain", "eradicate": "Eradicate", "recover": "Recover", "review": "Post-incident review"},
		"vulnerability": {"contain": "Assess & mitigate", "eradicate": "Remediate", "recover": "Verify the fix", "review": "Review"},
	}

	InvStatuses   = []string{"open", "confirmed", "refuted", "inconclusive", "parked"}
	InvConclusion = []string{"confirmed", "refuted", "inconclusive", "parked"}
	EvidenceKinds = []string{"log", "file", "image", "command-output", "network", "memory", "statement", "other"}
	IndicatorType = []string{"ip", "domain", "url", "hash", "email", "file", "account", "cve", "other"}
	ActionKinds   = []string{"containment", "eradication", "recovery", "communication", "follow-up"}
	Outcomes      = []string{"resolved", "false-alarm", "not-affected", "risk-accepted"}
	ReportKinds   = []string{"sitrep", "final"}
)

const (
	ActProposed = "proposed"
	ActApproved = "approved"
	ActRejected = "rejected"
	ActRunning  = "in-progress"
	ActDone     = "done"
	ActFailed   = "failed"
	ActCanceled = "cancelled"
	ActPlanned  = "planned" // a follow-up: tracked, not gated
)

type IncidentState struct {
	Type           string                   `json:"type"`
	Severity       string                   `json:"severity"`
	TLP            string                   `json:"tlp"`
	Refs           []string                 `json:"refs"`     // CVE ids, ticket numbers
	Affected       []string                 `json:"affected"` // systems, services, accounts
	SitrepEvery    string                   `json:"sitrep_every,omitempty"`
	Narrative      map[string]*IncidentText `json:"narrative"`
	Timeline       []*IncidentEvent         `json:"timeline"`
	Investigations []*Investigation         `json:"investigations"`
	Evidence       []*Evidence              `json:"evidence"`
	Indicators     []*Indicator             `json:"indicators"`
	Actions        []*IncidentAction        `json:"actions"`
	Reports        []*IncidentReport        `json:"reports"`
	Outcome        string                   `json:"outcome,omitempty"`
	OpenedAt       string                   `json:"opened_at"`
	ClosedAt       *string                  `json:"closed_at"`
}

type IncidentText struct {
	Text string `json:"text"`
	By   string `json:"by"`
	At   string `json:"at"`
}

// IncidentEvent is one line of the timeline. At is when it happened, not when it was recorded.
type IncidentEvent struct {
	ID         string   `json:"id"`
	At         string   `json:"at"`
	Text       string   `json:"text"`
	Source     string   `json:"source"`
	Evidence   []string `json:"evidence"`
	Confidence string   `json:"confidence"` // confirmed, suspected
	Mark       string   `json:"mark,omitempty"`
	By         string   `json:"by"`
	RecordedAt string   `json:"recorded_at"`
	Retracted  string   `json:"retracted,omitempty"` // why it was withdrawn
}

type Investigation struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"` // the question it answers
	Hypothesis  string        `json:"hypothesis"`
	Status      string        `json:"status"`
	Notes       []*InvNote    `json:"notes"`
	Conclusion  string        `json:"conclusion"`
	Evidence    []string      `json:"evidence"`
	ConcludedBy string        `json:"concluded_by,omitempty"`
	ConcludedAt string        `json:"concluded_at,omitempty"`
	Challenge   *InvChallenge `json:"challenge"`
	OpenedBy    string        `json:"opened_by"`
	OpenedAt    string        `json:"opened_at"`
	UpdatedAt   string        `json:"updated_at"`
}

type InvNote struct {
	At       string   `json:"at"`
	By       string   `json:"by"`
	Text     string   `json:"text"`
	Evidence []string `json:"evidence"`
}

type InvChallenge struct {
	Verdict string `json:"verdict"` // holds, fails
	Note    string `json:"note"`
	By      string `json:"by"`
	At      string `json:"at"`
}

// Evidence is one item of the case's evidence. A file is kept in the case folder with its sha256.
type Evidence struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	Source      string `json:"source"` // the host, system or person it comes from
	CollectedAt string `json:"collected_at"`
	CollectedBy string `json:"collected_by"`
	File        string `json:"file,omitempty"` // relative to the case folder
	SHA256      string `json:"sha256,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Ref         string `json:"ref,omitempty"` // where it is kept, when it isn't a file here
	Note        string `json:"note"`
	AddedBy     string `json:"added_by"`
	AddedAt     string `json:"added_at"`
}

type Indicator struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Value    string   `json:"value"`
	Context  string   `json:"context"`
	Evidence []string `json:"evidence"`
	Status   string   `json:"status"` // active, retired
	Reason   string   `json:"reason,omitempty"`
	By       string   `json:"by"`
	At       string   `json:"at"`
}

type IncidentAction struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Risk       string `json:"risk"`
	Rollback   string `json:"rollback"`
	Executor   string `json:"executor"` // "agent", or the person or team who does it
	Status     string `json:"status"`
	Result     string `json:"result"`
	ProposedBy string `json:"proposed_by"`
	ProposedAt string `json:"proposed_at"`
	DecidedAt  string `json:"decided_at,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	DoneAt     string `json:"done_at,omitempty"`
}

type IncidentReport struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Number   int    `json:"number"` // per kind
	Status   string `json:"status"` // in-review, issued, changes-requested, superseded
	File     string `json:"file"`   // relative to the case folder
	Hash     string `json:"hash"`
	Edited   bool   `json:"edited,omitempty"` // the lead edited the draft
	Outcome  string `json:"outcome,omitempty"`
	By       string `json:"by"`
	At       string `json:"at"`
	IssuedAt string `json:"issued_at,omitempty"`
}

func NewIncident(root, name, typ, severity, tlp string) *State {
	return &State{
		Version: 1, Template: Incident,
		Project: Project{Name: name, Cwd: root, CreatedAt: Now()},
		Stage:   "active", Counters: map[string]int{},
		Incident: &IncidentState{Type: typ, Severity: severity, TLP: tlp, OpenedAt: Now()},
		Root:     root, Dir: filepath.Join(root, StateDir),
	}
}

func migrateIncident(i *IncidentState) {
	if i.Narrative == nil {
		i.Narrative = map[string]*IncidentText{}
	}
	if i.Refs == nil {
		i.Refs = []string{}
	}
	if i.Affected == nil {
		i.Affected = []string{}
	}
	if i.Timeline == nil {
		i.Timeline = []*IncidentEvent{}
	}
	if i.Investigations == nil {
		i.Investigations = []*Investigation{}
	}
	if i.Evidence == nil {
		i.Evidence = []*Evidence{}
	}
	if i.Indicators == nil {
		i.Indicators = []*Indicator{}
	}
	if i.Actions == nil {
		i.Actions = []*IncidentAction{}
	}
	if i.Reports == nil {
		i.Reports = []*IncidentReport{}
	}
	for _, v := range i.Investigations {
		if v.Notes == nil {
			v.Notes = []*InvNote{}
		}
		if v.Evidence == nil {
			v.Evidence = []string{}
		}
	}
}

func SectionTitle(k string) string {
	for _, s := range IncidentSections {
		if s.K == k {
			return s.V
		}
	}
	return k
}

func MarkLabel(typ, mark string) string {
	return firstOf(markLabels[typ][mark], markLabels["incident"][mark], mark)
}
func PhaseLabel(typ, phase string) string {
	return firstOf(phaseLabels[typ][phase], phaseLabels["incident"][phase], phase)
}

// actor is who a record comes from: the agent's id, or "user" for the console.
func actor(st *State) string {
	if Actor == "" {
		return "user"
	}
	return Actor
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func incidentOpen(st *State) error {
	if st.Incident == nil {
		return Gate("This is not an incident-response case.")
	}
	if st.Stage == "closed" {
		return Gate("The case is closed. `incident reopen --reason ...` opens it again.")
	}
	return nil
}

// ParseWhen reads a time an agent or a person wrote. Without an offset it is UTC.
func ParseWhen(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "now" {
		return Now(), nil
	}
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(f, v); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05Z"), nil
		}
	}
	return "", Gate("Can't read the time %q: write it like 2026-11-18T14:05:00Z (UTC when no offset is given).", v)
}

func checkEvidence(st *State, ids []string) error {
	for _, id := range ids {
		if GetEvidence(st, id) == nil {
			return Gate("No evidence %s: register it first with `incident evidence add`.", id)
		}
	}
	return nil
}

func GetEvidence(st *State, id string) *Evidence {
	for _, e := range st.Incident.Evidence {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func GetInvestigation(st *State, id string) (*Investigation, error) {
	for _, v := range st.Incident.Investigations {
		if v.ID == id {
			return v, nil
		}
	}
	return nil, Gate("No investigation %s.", id)
}

func GetAction(st *State, id string) (*IncidentAction, error) {
	for _, a := range st.Incident.Actions {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, Gate("No action %s.", id)
}

func GetReport(st *State, id string) *IncidentReport {
	for _, r := range st.Incident.Reports {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// ------------------------------------------------------------------ the case

// IncidentSeverity asks the lead to change the severity: it drives who is told and how fast.
func IncidentSeverity(st *State, severity, reason string) (*Question, error) {
	if err := incidentOpen(st); err != nil {
		return nil, err
	}
	i := st.Incident
	if !contains(Severities, severity) {
		return nil, Gate("Severity: %s.", strings.Join(Severities, ", "))
	}
	if severity == i.Severity {
		return nil, Gate("The severity is already %s.", severity)
	}
	if strings.TrimSpace(reason) == "" {
		return nil, Gate("Say why the severity changes: the lead decides on it.")
	}
	Supersede(st, &Effect{Type: "incident-severity"}, "another severity was proposed.")
	return AddQuestion(st, "decision", fmt.Sprintf("Change the severity from %s to %s?", i.Severity, severity), reason,
		[]string{"Change it", "Keep " + i.Severity}, &Effect{Type: "incident-severity", Item: severity}, Context{}, false,
		map[string]string{"Change it": "The case becomes " + severity + ". Reports and the timeline show the change.",
			"Keep " + i.Severity: "Nothing changes."})
}

// IncidentNote sets one narrative section.
func IncidentNote(st *State, section, text string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	ok := false
	for _, s := range IncidentSections {
		ok = ok || s.K == section
	}
	if !ok {
		return Gate("No section %s.", section)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Gate("Write the text of the section.")
	}
	st.Incident.Narrative[section] = &IncidentText{Text: text, By: actor(st), At: Now()}
	Log(st, SectionTitle(section)+" updated.", "incident")
	return nil
}

// ------------------------------------------------------------------ timeline

func TimelineAdd(st *State, ev *IncidentEvent) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	i := st.Incident
	ev.Text = strings.TrimSpace(ev.Text)
	if ev.Text == "" {
		return Gate("Say what happened.")
	}
	if ev.Evidence == nil {
		ev.Evidence = []string{}
	}
	if err := checkEvidence(st, ev.Evidence); err != nil {
		return err
	}
	if ev.Confidence == "" {
		ev.Confidence = "confirmed"
	}
	if ev.Confidence == "confirmed" && len(ev.Evidence) == 0 && strings.TrimSpace(ev.Source) == "" {
		return Gate("A confirmed event needs what it rests on: `--evidence EV-n`, or `--source` (who or what says so). " +
			"Use `--suspected` for what is not established yet.")
	}
	if ev.Mark != "" {
		if !contains(IncidentMarks, ev.Mark) {
			return Gate("Milestones: %s.", strings.Join(IncidentMarks, ", "))
		}
		if ev.Confidence != "confirmed" {
			return Gate("A milestone is a confirmed event.")
		}
		for _, o := range i.Timeline {
			if o.Mark == ev.Mark && o.Retracted == "" {
				return Gate("%s already marks %q. Retract it first if it was wrong.", o.ID, ev.Mark)
			}
		}
	}
	ev.ID, ev.By, ev.RecordedAt = NextID(st, "TL"), actor(st), Now()
	i.Timeline = append(i.Timeline, ev)
	what := ev.Text
	if ev.Mark != "" {
		what = MarkLabel(i.Type, ev.Mark) + ": " + what
	}
	Log(st, fmt.Sprintf("Timeline %s (%s): %s", ev.ID, ev.At, what), "incident")
	return nil
}

func TimelineRetract(st *State, id, reason string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return Gate("Say why the event is withdrawn.")
	}
	for _, ev := range st.Incident.Timeline {
		if ev.ID == id {
			if ev.Retracted != "" {
				return Gate("%s is already withdrawn.", id)
			}
			ev.Retracted = strings.TrimSpace(reason)
			Log(st, fmt.Sprintf("Timeline %s withdrawn: %s", id, ev.Retracted), "incident")
			return nil
		}
	}
	return Gate("No timeline event %s.", id)
}

// SortedTimeline: the events that stand, oldest first.
func SortedTimeline(i *IncidentState) []*IncidentEvent {
	out := []*IncidentEvent{}
	for _, ev := range i.Timeline {
		if ev.Retracted == "" {
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].At < out[b].At })
	return out
}

func markTime(i *IncidentState, mark string) string {
	for _, ev := range i.Timeline {
		if ev.Mark == mark && ev.Retracted == "" {
			return ev.At
		}
	}
	return ""
}

// ------------------------------------------------------------------ evidence

// EvidenceAdd records an item; the caller has put its file in the case folder and hashed it.
func EvidenceAdd(st *State, e *Evidence) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	e.Title = strings.TrimSpace(e.Title)
	if e.Title == "" {
		return Gate("Give the evidence a title.")
	}
	if !contains(EvidenceKinds, e.Kind) {
		return Gate("Kind: %s.", strings.Join(EvidenceKinds, ", "))
	}
	if e.File == "" && strings.TrimSpace(e.Ref) == "" {
		return Gate("Evidence is a file (`--file`), a text (`--text`) or a pointer to where it is kept (`--ref`).")
	}
	if strings.TrimSpace(e.Source) == "" {
		return Gate("Say where it comes from with `--source`: the host, the system or the person.")
	}
	e.AddedBy, e.AddedAt = actor(st), Now()
	if e.CollectedBy == "" {
		e.CollectedBy = e.AddedBy
	}
	st.Incident.Evidence = append(st.Incident.Evidence, e)
	Log(st, fmt.Sprintf("Evidence %s registered: %s.", e.ID, e.Title), "incident")
	return nil
}

// ------------------------------------------------------------------ investigations

func InvestigationOpen(st *State, title, hypothesis string) (*Investigation, error) {
	if err := incidentOpen(st); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, Gate("An investigation needs the question it answers.")
	}
	v := &Investigation{ID: NextID(st, "INV"), Title: title, Hypothesis: strings.TrimSpace(hypothesis), Status: "open",
		Notes: []*InvNote{}, Evidence: []string{}, OpenedBy: actor(st), OpenedAt: Now(), UpdatedAt: Now()}
	st.Incident.Investigations = append(st.Incident.Investigations, v)
	Log(st, fmt.Sprintf("Investigation %s opened: %s", v.ID, title), "incident")
	return v, nil
}

func InvestigationNote(st *State, id, text string, evidence []string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	v, err := GetInvestigation(st, id)
	if err != nil {
		return err
	}
	if v.Status != "open" {
		return Gate("%s is %s. `incident investigation reopen %s --reason ...` to work on it again.", id, v.Status, id)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Gate("Write the note.")
	}
	if evidence == nil {
		evidence = []string{}
	}
	if err := checkEvidence(st, evidence); err != nil {
		return err
	}
	v.Notes = append(v.Notes, &InvNote{At: Now(), By: actor(st), Text: text, Evidence: evidence})
	v.UpdatedAt = Now()
	return nil
}

// InvestigationConclude records a conclusion. What is confirmed or refuted rests on evidence.
func InvestigationConclude(st *State, id, status, conclusion string, evidence []string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	v, err := GetInvestigation(st, id)
	if err != nil {
		return err
	}
	if v.Status != "open" {
		return Gate("%s is already %s.", id, v.Status)
	}
	if !contains(InvConclusion, status) {
		return Gate("A conclusion is: %s.", strings.Join(InvConclusion, ", "))
	}
	conclusion = strings.TrimSpace(conclusion)
	if conclusion == "" {
		return Gate("Write the conclusion: what is established, or why the investigation stops.")
	}
	if err := checkEvidence(st, evidence); err != nil {
		return err
	}
	all := append([]string{}, evidence...)
	for _, n := range v.Notes {
		for _, e := range n.Evidence {
			if !contains(all, e) {
				all = append(all, e)
			}
		}
	}
	if (status == "confirmed" || status == "refuted") && len(all) == 0 {
		return Gate("A %s conclusion rests on evidence: register it (`incident evidence add`) and pass `--evidence EV-n`. "+
			"Without evidence the investigation is inconclusive.", status)
	}
	v.Status, v.Conclusion, v.Evidence = status, conclusion, all
	v.ConcludedBy, v.ConcludedAt, v.UpdatedAt, v.Challenge = actor(st), Now(), Now(), nil
	Log(st, fmt.Sprintf("Investigation %s %s: %s", id, status, conclusion), "incident")
	return nil
}

// InvestigationChallenge records a fresh agent's attempt to disprove a confirmed conclusion.
func InvestigationChallenge(st *State, id, verdict, note string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	v, err := GetInvestigation(st, id)
	if err != nil {
		return err
	}
	if v.Status != "confirmed" {
		return Gate("Only a confirmed conclusion is challenged (%s is %s).", id, v.Status)
	}
	if verdict != "holds" && verdict != "fails" {
		return Gate("Verdict: holds or fails.")
	}
	if strings.TrimSpace(note) == "" {
		return Gate("Write what was checked and what it showed.")
	}
	by := actor(st)
	if by == v.ConcludedBy {
		return Gate("%s concluded %s: a different agent challenges it (`agent start --role challenger`).", by, id)
	}
	v.Challenge = &InvChallenge{Verdict: verdict, Note: strings.TrimSpace(note), By: by, At: Now()}
	v.UpdatedAt = Now()
	if verdict == "fails" {
		v.Status = "open"
		v.Notes = append(v.Notes, &InvNote{At: Now(), By: by, Text: "Challenge failed the conclusion \"" + v.Conclusion + "\": " + v.Challenge.Note, Evidence: []string{}})
		Log(st, fmt.Sprintf("Investigation %s reopened: the challenge disproved its conclusion.", id), "incident")
		return nil
	}
	Log(st, fmt.Sprintf("Investigation %s: the conclusion held against the challenge.", id), "incident")
	return nil
}

func InvestigationReopen(st *State, id, reason string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	v, err := GetInvestigation(st, id)
	if err != nil {
		return err
	}
	if v.Status == "open" {
		return Gate("%s is open.", id)
	}
	if strings.TrimSpace(reason) == "" {
		return Gate("Say why it is reopened.")
	}
	v.Notes = append(v.Notes, &InvNote{At: Now(), By: actor(st), Text: fmt.Sprintf("Reopened (was %s: %s). %s", v.Status, v.Conclusion, strings.TrimSpace(reason)), Evidence: []string{}})
	v.Status, v.Challenge, v.UpdatedAt = "open", nil, Now()
	Log(st, fmt.Sprintf("Investigation %s reopened: %s", id, strings.TrimSpace(reason)), "incident")
	return nil
}

// Unchallenged: confirmed conclusions no fresh agent has tried to disprove yet.
func Unchallenged(i *IncidentState) []*Investigation {
	out := []*Investigation{}
	for _, v := range i.Investigations {
		if v.Status == "confirmed" && v.Challenge == nil {
			out = append(out, v)
		}
	}
	return out
}

// ------------------------------------------------------------------ indicators

func IndicatorAdd(st *State, in *Indicator) (*Indicator, error) {
	if err := incidentOpen(st); err != nil {
		return nil, err
	}
	if !contains(IndicatorType, in.Type) {
		return nil, Gate("Type: %s.", strings.Join(IndicatorType, ", "))
	}
	in.Value = Refang(strings.TrimSpace(in.Value))
	if in.Value == "" || strings.ContainsAny(in.Value, "\n\r") {
		return nil, Gate("An indicator is one value on one line.")
	}
	if in.Evidence == nil {
		in.Evidence = []string{}
	}
	if err := checkEvidence(st, in.Evidence); err != nil {
		return nil, err
	}
	for _, o := range st.Incident.Indicators {
		if o.Type == in.Type && strings.EqualFold(o.Value, in.Value) {
			if o.Status == "retired" {
				return nil, Gate("%s was retired (%s). Add it under another context only if that reason no longer holds.", o.ID, o.Reason)
			}
			return o, nil // already known
		}
	}
	in.ID, in.Status, in.By, in.At = NextID(st, "IOC"), "active", actor(st), Now()
	st.Incident.Indicators = append(st.Incident.Indicators, in)
	Log(st, fmt.Sprintf("Indicator %s added: %s %s", in.ID, in.Type, Defang(in.Type, in.Value)), "incident")
	return in, nil
}

func IndicatorRetire(st *State, id, reason string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return Gate("Say why it is no indicator of this case.")
	}
	for _, in := range st.Incident.Indicators {
		if in.ID == id {
			in.Status, in.Reason = "retired", strings.TrimSpace(reason)
			Log(st, fmt.Sprintf("Indicator %s retired: %s", id, in.Reason), "incident")
			return nil
		}
	}
	return Gate("No indicator %s.", id)
}

// Defang writes an address so nothing turns it into a link: reports and the console show this form.
func Defang(typ, v string) string {
	switch typ {
	case "ip", "domain", "url", "email":
		v = strings.NewReplacer("http://", "hxxp://", "https://", "hxxps://", "HTTP://", "hxxp://", "HTTPS://", "hxxps://").Replace(v)
		v = strings.ReplaceAll(v, ".", "[.]")
		v = strings.ReplaceAll(v, "@", "[@]")
	}
	return v
}

// Refang undoes the usual defanging, so the stored value is the real one.
func Refang(v string) string {
	return strings.NewReplacer("[.]", ".", "(.)", ".", "[@]", "@", "hxxps://", "https://", "hxxp://", "http://", "[:]", ":").Replace(v)
}

// ------------------------------------------------------------------ actions

// ActionPropose records a response action. Anything but a follow-up waits for the lead's approval.
func ActionPropose(st *State, a *IncidentAction) (*Question, error) {
	if err := incidentOpen(st); err != nil {
		return nil, err
	}
	if !contains(ActionKinds, a.Kind) {
		return nil, Gate("Kind: %s.", strings.Join(ActionKinds, ", "))
	}
	a.Title, a.Detail = strings.TrimSpace(a.Title), strings.TrimSpace(a.Detail)
	if a.Title == "" {
		return nil, Gate("Give the action a title.")
	}
	if a.Executor == "" {
		a.Executor = "agent"
	}
	if a.Kind != "follow-up" && a.Detail == "" {
		return nil, Gate("Say exactly what will be done and on what (`--detail`): the lead approves that, not the title.")
	}
	if a.Kind != "follow-up" && strings.TrimSpace(a.Risk) == "" {
		return nil, Gate("Say what the action could break or alert (`--risk`), even if it is \"none known\".")
	}
	a.ID, a.ProposedBy, a.ProposedAt = NextID(st, "ACT"), actor(st), Now()
	st.Incident.Actions = append(st.Incident.Actions, a)
	if a.Kind == "follow-up" {
		a.Status = ActPlanned
		Log(st, fmt.Sprintf("Follow-up %s noted: %s", a.ID, a.Title), "incident")
		return nil, nil
	}
	a.Status = ActProposed
	who := "The agent carries it out once you approve."
	if a.Executor != "agent" {
		who = a.Executor + " carries it out; the agent only records the result."
	}
	body := []string{fmt.Sprintf("**%s** action proposed by the agents. %s", upperFirst(a.Kind), who), "### What will be done\n\n" + a.Detail,
		"### What it could break\n\n" + a.Risk}
	if strings.TrimSpace(a.Rollback) != "" {
		body = append(body, "### How to undo it\n\n"+a.Rollback)
	}
	return AddQuestion(st, "validation", "Approve "+a.ID+": "+a.Title+"?", strings.Join(body, "\n\n"), []string{Approve, "Reject"},
		&Effect{Type: "incident-action", Item: a.ID}, Context{}, false, map[string]string{
			Approve:  "The action may be carried out exactly as described.",
			"Reject": "The action is not carried out. Say why in the comment, or what to do instead.",
		})
}

// ActionRecord notes an action somebody already took.
func ActionRecord(st *State, a *IncidentAction) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	if !contains(ActionKinds, a.Kind) {
		return Gate("Kind: %s.", strings.Join(ActionKinds, ", "))
	}
	a.Title = strings.TrimSpace(a.Title)
	if a.Title == "" {
		return Gate("Give the action a title.")
	}
	if strings.TrimSpace(a.Executor) == "" || a.Executor == "agent" {
		return Gate("`action record` is for what a person or a team already did: say who with `--by`. An agent's own " +
			"action is proposed first (`incident action propose`).")
	}
	a.ID, a.ProposedBy, a.ProposedAt, a.Status = NextID(st, "ACT"), actor(st), Now(), ActDone
	st.Incident.Actions = append(st.Incident.Actions, a)
	Log(st, fmt.Sprintf("Action %s recorded (done by %s): %s", a.ID, a.Executor, a.Title), "incident")
	return nil
}

func ActionStart(st *State, id string) (*IncidentAction, error) {
	if err := incidentOpen(st); err != nil {
		return nil, err
	}
	a, err := GetAction(st, id)
	if err != nil {
		return nil, err
	}
	switch a.Status {
	case ActProposed:
		return nil, Gate("%s waits for the lead's approval in the console. Don't carry it out.", id)
	case ActRejected:
		return nil, Gate("The lead rejected %s. Don't carry it out.", id)
	case ActApproved, ActPlanned:
	default:
		return nil, Gate("%s is %s.", id, a.Status)
	}
	a.Status, a.StartedAt = ActRunning, Now()
	Log(st, fmt.Sprintf("Action %s started: %s", id, a.Title), "incident")
	return a, nil
}

// ActionDone closes an action and puts it on the timeline.
func ActionDone(st *State, id, result string, failed bool, mark string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	a, err := GetAction(st, id)
	if err != nil {
		return err
	}
	switch a.Status {
	case ActProposed:
		return Gate("%s waits for the lead's approval in the console.", id)
	case ActRejected:
		return Gate("The lead rejected %s.", id)
	case ActApproved, ActRunning, ActPlanned:
	default:
		return Gate("%s is already %s.", id, a.Status)
	}
	result = strings.TrimSpace(result)
	if result == "" {
		return Gate("Say what was done and how you checked it worked (`--result`).")
	}
	if failed && mark != "" {
		return Gate("A failed action reaches no milestone.")
	}
	a.Status, a.Result, a.DoneAt = ActDone, result, Now()
	if failed {
		a.Status = ActFailed
	}
	text := fmt.Sprintf("%s action %s: %s. %s", upperFirst(a.Kind), a.Status, a.Title, result)
	if a.Kind == "follow-up" && mark == "" {
		Log(st, fmt.Sprintf("Follow-up %s %s.", id, a.Status), "incident")
		return nil
	}
	return TimelineAdd(st, &IncidentEvent{At: Now(), Text: text, Source: id, Mark: mark})
}

func ActionCancel(st *State, id, reason string) error {
	if err := incidentOpen(st); err != nil {
		return err
	}
	a, err := GetAction(st, id)
	if err != nil {
		return err
	}
	if a.Status == ActDone || a.Status == ActFailed || a.Status == ActCanceled || a.Status == ActRejected {
		return Gate("%s is already %s.", id, a.Status)
	}
	if strings.TrimSpace(reason) == "" {
		return Gate("Say why it is cancelled.")
	}
	Supersede(st, &Effect{Type: "incident-action", Item: id}, "the action was cancelled.")
	a.Status, a.Result, a.DoneAt = ActCanceled, strings.TrimSpace(reason), Now()
	Log(st, fmt.Sprintf("Action %s cancelled: %s", id, a.Result), "incident")
	return nil
}

// pendingActions: response actions not settled yet (follow-ups outlive the case).
func pendingActions(i *IncidentState) []*IncidentAction {
	out := []*IncidentAction{}
	for _, a := range i.Actions {
		if a.Kind != "follow-up" && (a.Status == ActProposed || a.Status == ActApproved || a.Status == ActRunning) {
			out = append(out, a)
		}
	}
	return out
}

// ------------------------------------------------------------------ reports

// LastIssued: the latest issued report of a kind ("" for any), nil if none.
func LastIssued(i *IncidentState, kind string) *IncidentReport {
	for n := len(i.Reports) - 1; n >= 0; n-- {
		if r := i.Reports[n]; r.Status == "issued" && (kind == "" || r.Kind == kind) {
			return r
		}
	}
	return nil
}

// FinalBlockers lists what keeps the final report from being drafted.
func FinalBlockers(st *State, outcome string) []string {
	i := st.Incident
	out := []string{}
	open := []string{}
	for _, v := range i.Investigations {
		if v.Status == "open" {
			open = append(open, v.ID)
		}
	}
	if len(open) > 0 {
		out = append(out, fmt.Sprintf("%s still open: conclude them (confirmed, refuted, inconclusive or parked)", strings.Join(open, ", ")))
	}
	if u := Unchallenged(i); len(u) > 0 {
		ids := []string{}
		for _, v := range u {
			ids = append(ids, v.ID)
		}
		out = append(out, fmt.Sprintf("the confirmed conclusion of %s was never challenged: give each to a fresh challenger", strings.Join(ids, ", ")))
	}
	if p := pendingActions(i); len(p) > 0 {
		ids := []string{}
		for _, a := range p {
			ids = append(ids, a.ID+" ("+a.Status+")")
		}
		out = append(out, "actions not settled: "+strings.Join(ids, ", "))
	}
	need := []string{"situation", "impact"}
	if outcome == "resolved" || outcome == "risk-accepted" {
		need = append(need, "root-cause", "lessons")
	}
	for _, k := range need {
		if t := i.Narrative[k]; t == nil || strings.TrimSpace(t.Text) == "" {
			out = append(out, fmt.Sprintf("the %s section is empty (`incident note %s`)", strings.ToLower(SectionTitle(k)), k))
		}
	}
	if outcome == "resolved" && markTime(i, "contained") == "" {
		out = append(out, fmt.Sprintf("no timeline event marks when it was %s (`--mark contained`)", strings.ToLower(MarkLabel(i.Type, "contained"))))
	}
	return out
}

// ReportSubmit records a drafted report (the caller rendered and wrote its file) and asks the lead.
func ReportSubmit(st *State, kind, file, hash, body, outcome string) (*IncidentReport, *Question, error) {
	if err := incidentOpen(st); err != nil {
		return nil, nil, err
	}
	i := st.Incident
	for _, r := range i.Reports {
		if r.Status == "in-review" {
			r.Status = "superseded"
			Supersede(st, &Effect{Type: "incident-report", Item: r.ID}, "a newer draft replaces it.")
		}
	}
	n := ReportNumber(i, kind)
	r := &IncidentReport{ID: NextID(st, "REP"), Kind: kind, Number: n, Status: "in-review", File: file, Hash: hash,
		Outcome: outcome, By: actor(st), At: Now()}
	i.Reports = append(i.Reports, r)
	title := fmt.Sprintf("Issue situation report %d?", n)
	help := map[string]string{
		Approve:        "The report is issued as shown: it is yours to send. Edit it in the Reports view first if needed.",
		RequestChanges: "Say what to change; the agent updates the case and drafts it again.",
	}
	if kind == "final" {
		title = "Issue the final report and close the case?"
		help[Approve] = "The report is issued as shown and the case is closed (" + outcome + ")."
		st.Stage = "review"
	}
	q, err := AddQuestion(st, "validation", title, body, []string{Approve, RequestChanges},
		&Effect{Type: "incident-report", Item: r.ID}, Context{}, false, help)
	return r, q, err
}

// ReportNumber: the number the next report of a kind takes (issued and pending ones count).
func ReportNumber(i *IncidentState, kind string) int {
	n := 1
	for _, r := range i.Reports {
		if r.Kind == kind && (r.Status == "issued" || r.Status == "in-review") {
			n = r.Number + 1
		}
	}
	for _, r := range i.Reports {
		if r.Kind == kind && r.Status == "in-review" {
			n = r.Number // a new draft replaces the pending one
		}
	}
	return n
}

// ReportFile: where a report of a kind and number is written, relative to the case folder.
func ReportFile(kind string, n int) string {
	if kind == "final" {
		if n > 1 {
			return fmt.Sprintf("reports/final-report-%d.md", n)
		}
		return "reports/final-report.md"
	}
	return fmt.Sprintf("reports/sitrep-%03d.md", n)
}

func applyIncidentEffect(st *State, q *Question) error {
	i := st.Incident
	switch q.Effect.Type {
	case "incident-severity":
		if q.Answer.Choice != "Change it" {
			Log(st, "Severity kept at "+i.Severity+".", "incident")
			return nil
		}
		from := i.Severity
		i.Severity = q.Effect.Item
		Log(st, fmt.Sprintf("Severity changed from %s to %s.", from, i.Severity), "incident")
		i.Timeline = append(i.Timeline, &IncidentEvent{ID: NextID(st, "TL"), At: Now(), Text: fmt.Sprintf("Severity changed from %s to %s. %s", from, i.Severity, q.Body),
			Source: "the lead's decision " + q.ID, Evidence: []string{}, Confidence: "confirmed", By: "user", RecordedAt: Now()})
	case "incident-action":
		a, err := GetAction(st, q.Effect.Item)
		if err != nil {
			return err
		}
		a.DecidedAt = Now()
		if q.Answer.Choice == Approve {
			a.Status = ActApproved
			Log(st, fmt.Sprintf("Action %s approved: %s", a.ID, a.Title), "incident")
		} else {
			a.Status, a.Result = ActRejected, q.Answer.Comment
			Log(st, fmt.Sprintf("Action %s rejected.", a.ID), "incident")
		}
	case "incident-report":
		r := GetReport(st, q.Effect.Item)
		if r == nil {
			return Gate("No report %s.", q.Effect.Item)
		}
		if q.Answer.Choice != Approve {
			r.Status = "changes-requested"
			if r.Kind == "final" {
				st.Stage = "active"
			}
			Log(st, fmt.Sprintf("Report %s: changes requested.", r.ID), "incident")
			return nil
		}
		if FileHash(filepath.Join(st.Root, r.File)) != r.Hash {
			return Gate("%s changed since it was drafted, outside lacarta. Ask the agent to draft the report again.", r.File)
		}
		r.Status, r.IssuedAt = "issued", Now()
		if r.Kind == "final" {
			st.Stage, i.Outcome, i.ClosedAt = "closed", r.Outcome, ptr(Now())
			Log(st, fmt.Sprintf("Final report issued (%s). The case is closed: %s.", r.File, i.Outcome), "stage")
			return nil
		}
		Log(st, fmt.Sprintf("Situation report %d issued: %s.", r.Number, r.File), "incident")
	}
	return nil
}

// IncidentReopen opens a closed case again.
func IncidentReopen(st *State, reason string) error {
	if st.Incident == nil {
		return Gate("This is not an incident-response case.")
	}
	if st.Stage != "closed" {
		return Gate("The case is not closed.")
	}
	if strings.TrimSpace(reason) == "" {
		return Gate("Say why the case is reopened.")
	}
	st.Stage, st.Incident.ClosedAt, st.Incident.Outcome = "active", nil, ""
	Log(st, "Case reopened: "+strings.TrimSpace(reason), "stage")
	return nil
}

// ------------------------------------------------------------------ derived

type IncidentDerived struct {
	Phase         string            `json:"phase"`
	PhaseLabel    string            `json:"phase_label"`
	Marks         []IncidentMark    `json:"marks"`
	Times         []KVJ             `json:"times"` // response times, where both ends are known
	Counts        map[string]int    `json:"counts"`
	Investigating map[string]int    `json:"investigating"` // by status
	Unchallenged  int               `json:"unchallenged"`
	Waiting       []string          `json:"waiting"`  // actions waiting for the lead
	Approved      []string          `json:"approved"` // approved, not done yet
	Changes       int               `json:"changes"`  // records since the last issued report
	LastReport    *IncidentReport   `json:"last_report"`
	Draft         *IncidentReport   `json:"draft"`
	SitrepDue     bool              `json:"sitrep_due"`
	SectionTitles map[string]string `json:"section_titles"`
}

type IncidentMark struct {
	Mark  string `json:"mark"`
	Label string `json:"label"`
	At    string `json:"at"`
}

type KVJ struct {
	K string `json:"k"`
	V string `json:"v"`
}

// IncidentPhase comes from the milestones reached.
func IncidentPhase(st *State) string {
	i := st.Incident
	switch {
	case st.Stage == "closed" || st.Stage == "review" || markTime(i, "recovered") != "":
		return "review"
	case markTime(i, "eradicated") != "":
		return "recover"
	case markTime(i, "contained") != "":
		return "eradicate"
	}
	return "contain"
}

// Span writes the time between two instants like "3h 12m".
func Span(from, to string) string {
	a, err1 := time.Parse(time.RFC3339, from)
	b, err2 := time.Parse(time.RFC3339, to)
	if err1 != nil || err2 != nil || b.Before(a) {
		return ""
	}
	d := b.Sub(a)
	days, h, m := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, h)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// ResponseTimes: the usual measures, for the ends the timeline marks.
func ResponseTimes(i *IncidentState) []KVJ {
	out := []KVJ{}
	add := func(label, from, to string) {
		if s := Span(from, to); s != "" && from != "" && to != "" {
			out = append(out, KVJ{label, s})
		}
	}
	t := func(m string) string { return markTime(i, m) }
	add("Time to detect", t("occurred"), t("detected"))
	add("Detection to "+strings.ToLower(MarkLabel(i.Type, "contained")), t("detected"), t("contained"))
	add("Detection to "+strings.ToLower(MarkLabel(i.Type, "recovered")), t("detected"), t("recovered"))
	if i.ClosedAt != nil {
		add("Case open for", i.OpenedAt, *i.ClosedAt)
	}
	return out
}

func changesSince(i *IncidentState, since string) int {
	n := 0
	for _, ev := range i.Timeline {
		if ev.RecordedAt > since {
			n++
		}
	}
	for _, v := range i.Investigations {
		if v.OpenedAt > since {
			n++
		}
		if v.ConcludedAt > since {
			n++
		}
	}
	for _, in := range i.Indicators {
		if in.At > since {
			n++
		}
	}
	return n
}

func sitrepDue(i *IncidentState, last string) bool {
	every, err := time.ParseDuration(i.SitrepEvery)
	if err != nil || every <= 0 {
		return false
	}
	t, err := time.Parse(time.RFC3339, last)
	return err == nil && Clock().Sub(t) >= every
}

func DeriveIncident(st *State) *IncidentDerived {
	i := st.Incident
	d := &IncidentDerived{Phase: IncidentPhase(st), Marks: []IncidentMark{}, Counts: map[string]int{}, Investigating: map[string]int{},
		Waiting: []string{}, Approved: []string{}, SectionTitles: map[string]string{}}
	d.PhaseLabel = PhaseLabel(i.Type, d.Phase)
	for _, s := range IncidentSections {
		d.SectionTitles[s.K] = s.V
	}
	for _, m := range IncidentMarks {
		d.Marks = append(d.Marks, IncidentMark{m, MarkLabel(i.Type, m), markTime(i, m)})
	}
	d.Times = ResponseTimes(i)
	for _, v := range i.Investigations {
		d.Investigating[v.Status]++
	}
	d.Unchallenged = len(Unchallenged(i))
	for _, a := range i.Actions {
		switch a.Status {
		case ActProposed:
			d.Waiting = append(d.Waiting, a.ID)
		case ActApproved, ActRunning:
			d.Approved = append(d.Approved, a.ID)
		}
	}
	active := 0
	for _, in := range i.Indicators {
		if in.Status == "active" {
			active++
		}
	}
	d.Counts = map[string]int{"timeline": len(SortedTimeline(i)), "investigations": len(i.Investigations), "evidence": len(i.Evidence),
		"indicators": active, "actions": len(i.Actions), "reports": 0}
	for _, r := range i.Reports {
		if r.Status == "issued" {
			d.Counts["reports"]++
		}
		if r.Status == "in-review" {
			d.Draft = r
		}
	}
	d.LastReport = LastIssued(i, "")
	since := i.OpenedAt
	if d.LastReport != nil {
		since = d.LastReport.At
	}
	d.Changes = changesSince(i, since)
	d.SitrepDue = st.Stage == "active" && d.Draft == nil && sitrepDue(i, since)
	return d
}

// IncidentNext is the `Next:` line for the coordinator.
func IncidentNext(st *State) string {
	i := st.Incident
	d := DeriveIncident(st)
	switch st.Stage {
	case "closed":
		return "The case is closed: its final report is issued. Follow-ups stay listed; `incident reopen --reason ...` if it comes back."
	case "review":
		return "Wait for the lead to approve the final report in the console. Only they can close the case."
	}
	parts := []string{}
	if len(d.Approved) > 0 {
		parts = append(parts, fmt.Sprintf("%s approved: carry out exactly what was approved (or record it when its owner did), then "+
			"`incident action done <id> --result ...`", strings.Join(d.Approved, ", ")))
	}
	if len(d.Waiting) > 0 {
		parts = append(parts, fmt.Sprintf("%s not approved yet: wait for the lead in the console and carry out nothing before", strings.Join(d.Waiting, ", ")))
	}
	for _, r := range i.Reports {
		if r.Status == "changes-requested" && r == i.Reports[len(i.Reports)-1] {
			parts = append(parts, fmt.Sprintf("the lead asked for changes to %s: read their comment (`decisions`), update the case, draft it again", r.ID))
		}
	}
	if d.Draft != nil {
		parts = append(parts, fmt.Sprintf("%s waits for the lead in the console", d.Draft.ID))
	}
	if d.Unchallenged > 0 {
		ids := []string{}
		for _, v := range Unchallenged(i) {
			ids = append(ids, v.ID)
		}
		parts = append(parts, fmt.Sprintf("give the confirmed conclusion of %s to a fresh challenger (`incident investigation brief <id> --role challenger`)", strings.Join(ids, ", ")))
	}
	if n := d.Investigating["open"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d open investigation%s to work: notes, evidence, then a conclusion", n, plural(n)))
	}
	if d.Draft == nil {
		switch {
		case d.SitrepDue:
			parts = append(parts, "a situation report is due (every "+i.SitrepEvery+"): update the situation and next steps, then `incident report sitrep`")
		case d.Changes >= 5 && d.LastReport != nil:
			parts = append(parts, fmt.Sprintf("%d records since the last report: consider `incident report sitrep`", d.Changes))
		case d.LastReport == nil && d.Changes >= 3:
			parts = append(parts, "no report issued yet: write the situation and next steps, then `incident report sitrep`")
		}
	}
	if len(parts) == 0 {
		if len(i.Investigations) == 0 {
			return "Establish the facts with the lead: record what is known on the timeline (`incident timeline add`), register " +
				"the evidence (`incident evidence add`), open an investigation for each question to answer (`incident investigation open`)."
		}
		return "Nothing is open. If the response is over, write the impact, root cause and lessons (`incident note`), then " +
			"`incident report final --outcome ...`; otherwise open the next investigation or propose the next action."
	}
	return upperFirst(strings.Join(parts, "; ")) + "."
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
