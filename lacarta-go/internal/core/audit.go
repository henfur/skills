package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The security-audit kind runs Cloudflare's security-audit skill (a required plugin) and records
// around it: the agents follow the skill and write its files into the run directory
// (~/.lacarta/audits/<project>/run-N); lacarta reads those files, gates the stages, and keeps
// what only the human auditor decides: the scope, the coverage plan, the review of every
// finding, and the approval of the report. Docs: docs/design/security-audit.md.

const Audit = "audit"

// Audit stages, in order.
var AuditStages = []KV{
	{"scope", "Scope"},
	{"recon", "Reconnaissance"},
	{"coverage-plan", "Coverage plan"},
	{"hunt", "Hunting & validation"},
	{"review", "Auditor review"},
	{"report", "Report"},
	{"done", "Done"},
}

var AuditProfiles = []string{"quick", "standard", "deep"}

// The skill's severity scale (report-schema.json), lowest last.
var SkillSeverities = []string{"critical", "high", "medium", "low", "informational"}

var AuditRoles = []Role{
	{"orchestrator", "Orchestrator", "Whole audit",
		"The parent agent: runs the skill's workflow, owns the run files, talks to you. This is the Claude Code " +
			"session itself: switch it with /model.", "opus"},
	{"recon", "Reconnaissance", "Recon",
		"Maps the architecture, trust boundaries and entry surfaces, and seeds the coverage ledger.", "sonnet"},
	{"hunter", "Hunter", "Hunting",
		"Hunts one coverage unit at a time for real boundary failures. Most of the audit's spend.", "opus"},
	{"coverage_critic", "Coverage critic", "Hunting",
		"Fresh agent after each wave: finds gaps in the coverage, never findings.", "sonnet"},
	{"candidate_verifier", "Candidate verifier", "Validation",
		"Fresh agent that tries to disprove each candidate. Use a different model from the hunters, so they check each other.", "sonnet"},
	{"record_verifier", "Record verifier", "Validation",
		"Fresh agent that re-checks every final record against the source.", "sonnet"},
}

// AuditState is the audit-specific part of a project's state. Its files live outside the target.
type AuditState struct {
	Target    string                  `json:"target"`
	Repo      string                  `json:"repo"`
	Execution string                  `json:"execution"` // "sandboxed" or "source-only"
	Scope     *AuditScope             `json:"scope"`     // the rules of engagement, proposed then approved
	Runs      []*AuditRun             `json:"runs"`
	Checks    map[string]string       `json:"checks,omitempty"` // validator name -> "ok" or the error, from the last submit
	Reviews   map[string]*AuditReview `json:"reviews"`          // by finding fingerprint, current run
	Recheck   map[string]string       `json:"recheck"`          // fingerprint -> the auditor's question
	Summary   string                  `json:"summary"`          // executive summary (markdown)
	SummaryBy string                  `json:"summary_by"`
	Report    *AuditReport            `json:"report"`
}

type AuditScope struct {
	Profile   string   `json:"profile"`
	Budget    int      `json:"budget"` // agent invocations; 0: none set
	Paths     []string `json:"paths"`  // empty: the whole repository
	Notes     string   `json:"notes"`
	SourceRef string   `json:"source_ref"`
	Dirty     bool     `json:"dirty"`
	Status    string   `json:"status"` // draft, in-review, approved, changes-requested
}

type AuditRun struct {
	ID        string   `json:"id"`
	Dir       string   `json:"dir"`
	Profile   string   `json:"profile"`
	Budget    int      `json:"budget"`
	Paths     []string `json:"paths"`
	SourceRef string   `json:"source_ref"`
	Dirty     bool     `json:"dirty"`
	StartedAt string   `json:"started_at"`
	EndedAt   *string  `json:"ended_at"`
	Plan      string   `json:"plan"` // coverage plan: draft, in-review, approved, changes-requested
}

// AuditReview is the auditor's decision on one finding.
type AuditReview struct {
	Action   string `json:"action"`   // see ReviewActions
	Severity string `json:"severity"` // the final severity when the action sets one
	From     string `json:"from"`     // the agents' severity, when changed
	Reason   string `json:"reason"`
	By       string `json:"by"`
	At       string `json:"at"`
}

type AuditReport struct {
	Versions []*ReportVersion `json:"versions"`
}

type ReportVersion struct {
	Version    int               `json:"version"`
	Run        string            `json:"run"`
	Dir        string            `json:"dir"`
	Auditor    string            `json:"auditor"`
	ApprovedAt string            `json:"approved_at"`
	Hash       string            `json:"hash"` // sha256 of the approved markdown
	Files      map[string]string `json:"files"`
}

// What the auditor can decide, per verdict.
var ReviewActions = map[string][]KV{
	"confirmed": {
		{"accept", "Accept as it is"},
		{"severity", "Change the severity"},
		{"reject", "Reject as a false positive"},
		{"recheck", "Ask for another check"},
	},
	"needs_validation": {
		{"keep", "Keep as needs validation"},
		{"confirm", "Confirm it myself"},
		{"out-of-scope", "Out of scope"},
	},
}

// ------------------------------------------------------------ the skill's files

type LedgerUnit struct {
	CoverageID     string            `json:"coverage_id"`
	Surface        string            `json:"surface"`
	Boundary       string            `json:"boundary"`
	Subsystem      string            `json:"subsystem"`
	AttackClass    string            `json:"attack_class"`
	Lifecycle      string            `json:"lifecycle,omitempty"`
	StartingPaths  []string          `json:"starting_paths"`
	Companions     []string          `json:"selected_companion_blocks"`
	Excluded       []ExcludedBlock   `json:"excluded_blocks"`
	PriorStatus    string            `json:"prior_status"`
	Wave           int               `json:"wave"`
	Status         string            `json:"status"`
	AgentID        *string           `json:"agent_id"`
	ReviewedPaths  []string          `json:"reviewed_paths"`
	LocalChecks    []json.RawMessage `json:"local_checks"`
	Fingerprints   []string          `json:"result_fingerprints"`
	Unresolved     []json.RawMessage `json:"unresolved"`
	Attempts       []json.RawMessage `json:"attempts"`
	DeferredReason string            `json:"deferred_reason,omitempty"`
}

type ExcludedBlock struct {
	Block  string `json:"block"`
	Reason string `json:"reason"`
}

// Terminal ledger states: the unit needs no more hunting in this run.
var UnitDone = map[string]bool{"covered": true, "candidate": true, "blocked": true, "deferred": true, "out_of_scope": true}

type SourceRef struct {
	Kind        string `json:"kind,omitempty"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Scope       string `json:"scope,omitempty"`
	Description string `json:"description"`
}

type Scored struct {
	Score  string `json:"score"`
	Reason string `json:"reason"`
}

type SkillFinding struct {
	Verdict          string      `json:"verdict"`
	Fingerprint      string      `json:"fingerprint"`
	Title            string      `json:"title"`
	Description      string      `json:"description"`
	RootCause        string      `json:"root_cause,omitempty"`
	ClaimedRootCause string      `json:"claimed_root_cause,omitempty"`
	IntendedBehavior string      `json:"intended_behavior,omitempty"`
	Trace            []SourceRef `json:"trace"`
	Evidence         []SourceRef `json:"evidence"`
	Conditions       []struct {
		Kind        string `json:"kind"`
		Description string `json:"description"`
	} `json:"conditions,omitempty"`
	Execution *struct {
		AttackerPerspective string   `json:"attacker_perspective"`
		Payloads            []string `json:"payloads"`
		Instructions        []string `json:"instructions"`
		ObservedResult      string   `json:"observed_result"`
	} `json:"execution,omitempty"`
	Remediation *struct {
		Strategy    string `json:"strategy"`
		CodeChanges []struct {
			FileName  string `json:"file_name"`
			FixedCode string `json:"fixed_code"`
		} `json:"code_changes,omitempty"`
	} `json:"remediation,omitempty"`
	Severity *struct {
		Likelihood Scored `json:"likelihood"`
		Impact     Scored `json:"impact"`
		Overall    string `json:"overall_severity"`
	} `json:"severity,omitempty"`
	Confidence     *Scored  `json:"confidence,omitempty"`
	Blockers       []string `json:"blockers,omitempty"`
	ValidationPlan *struct {
		Local      string `json:"local"`
		Deployment string `json:"deployment"`
	} `json:"validation_plan,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (f *SkillFinding) AgentSeverity() string {
	if f.Severity == nil {
		return ""
	}
	return f.Severity.Overall
}

func (f *SkillFinding) Cause() string { return firstOf(f.RootCause, f.ClaimedRootCause) }

func firstOf(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func AuditHome(pid string) string { return filepath.Join(Home(), "audits", pid) }

func CurrentRun(st *State) *AuditRun {
	if st.Audit == nil || len(st.Audit.Runs) == 0 {
		return nil
	}
	return st.Audit.Runs[len(st.Audit.Runs)-1]
}

func readJSONFile(path string, v any) (bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return true, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return true, nil
}

// ReadLedger returns the run's coverage ledger, nil if it doesn't exist yet.
func ReadLedger(run *AuditRun) ([]*LedgerUnit, error) {
	if run == nil {
		return nil, nil
	}
	var units []*LedgerUnit
	_, err := readJSONFile(filepath.Join(run.Dir, "coverage-ledger.json"), &units)
	return units, err
}

// ReadFindings returns the run's findings.json, nil if it doesn't exist yet.
func ReadFindings(run *AuditRun) ([]*SkillFinding, error) {
	if run == nil {
		return nil, nil
	}
	var fs []*SkillFinding
	_, err := readJSONFile(filepath.Join(run.Dir, "findings.json"), &fs)
	return fs, err
}

func RunFile(run *AuditRun, name string) string { return filepath.Join(run.Dir, name) }

// ------------------------------------------------------------ review

// FinalFinding is a finding as the report states it: the agents' record plus the auditor's review.
type FinalFinding struct {
	*SkillFinding
	Review   *AuditReview `json:"review"`
	Final    string       `json:"final"`    // confirmed, needs_validation, rejected, out_of_scope
	Severity string       `json:"severity"` // final severity, confirmed only
}

// NeedsReview: the verdicts the auditor must review before the report.
func NeedsReview(f *SkillFinding) bool {
	return f.Verdict == "confirmed" || f.Verdict == "needs_validation"
}

func FinalOf(st *State, f *SkillFinding) *FinalFinding {
	out := &FinalFinding{SkillFinding: f, Final: f.Verdict}
	if f.Verdict == "confirmed" {
		out.Severity = f.AgentSeverity()
	}
	var r *AuditReview
	if st.Audit != nil {
		r = st.Audit.Reviews[f.Fingerprint]
	}
	out.Review = r
	if r == nil {
		return out
	}
	switch r.Action {
	case "severity":
		out.Severity = r.Severity
	case "reject":
		out.Final, out.Severity = "rejected", ""
	case "confirm":
		out.Final, out.Severity = "confirmed", r.Severity
	case "out-of-scope":
		out.Final = "out_of_scope"
	}
	return out
}

func FinalFindings(st *State, fs []*SkillFinding) []*FinalFinding {
	out := make([]*FinalFinding, 0, len(fs))
	for _, f := range fs {
		out = append(out, FinalOf(st, f))
	}
	rank := map[string]int{}
	for i, s := range SkillSeverities {
		rank[s] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Final != out[j].Final {
			return verdictRank(out[i].Final) < verdictRank(out[j].Final)
		}
		return rank[out[i].Severity] < rank[out[j].Severity]
	})
	return out
}

func verdictRank(v string) int {
	return map[string]int{"confirmed": 0, "needs_validation": 1, "out_of_scope": 2, "rejected": 3}[v]
}

// PendingReviews lists the fingerprints the auditor still has to review (rechecks count as pending).
func PendingReviews(st *State, fs []*SkillFinding) []string {
	out := []string{}
	for _, f := range fs {
		if !NeedsReview(f) {
			continue
		}
		if r := st.Audit.Reviews[f.Fingerprint]; r == nil || r.Action == "recheck" {
			out = append(out, f.Fingerprint)
		}
	}
	return out
}

// ReviewFinding records the auditor's decision. Only the console calls it (the user).
func ReviewFinding(st *State, s *Store, fp, action, severity, reason string) error {
	if st.Audit == nil {
		return Gate("This is not a security audit.")
	}
	if st.Stage != "review" && st.Stage != "report" {
		return Gate("Findings are reviewed once the agents submitted them (stage now: %s).", st.Stage)
	}
	fs, err := ReadFindings(CurrentRun(st))
	if err != nil {
		return err
	}
	var f *SkillFinding
	for _, x := range fs {
		if x.Fingerprint == fp {
			f = x
		}
	}
	if f == nil || !NeedsReview(f) {
		return Gate("No confirmed or needs-validation finding %s in this run.", fp)
	}
	ok := false
	for _, a := range ReviewActions[f.Verdict] {
		ok = ok || a.K == action
	}
	if !ok {
		return Gate("%s is not a review of a %s finding.", action, strings.ReplaceAll(f.Verdict, "_", " "))
	}
	reason = strings.TrimSpace(reason)
	if action != "accept" && action != "keep" && reason == "" {
		return Gate("Write the reason: it goes into the report.")
	}
	r := &AuditReview{Action: action, Reason: reason, By: "user", At: Now()}
	if action == "severity" || action == "confirm" {
		if !contains(SkillSeverities, severity) {
			return Gate("Choose a severity: %s.", strings.Join(SkillSeverities, ", "))
		}
		if action == "severity" && severity == f.AgentSeverity() {
			return Gate("That is already its severity; accept it instead.")
		}
		r.Severity = severity
		r.From = f.AgentSeverity()
	}
	if st.Audit.Reviews == nil {
		st.Audit.Reviews = map[string]*AuditReview{}
	}
	if st.Audit.Recheck == nil {
		st.Audit.Recheck = map[string]string{}
	}
	st.Audit.Reviews[fp] = r
	delete(st.Audit.Recheck, fp)
	what := action
	for _, a := range ReviewActions[f.Verdict] {
		if a.K == action {
			what = strings.ToLower(a.V)
		}
	}
	switch action {
	case "severity":
		what = fmt.Sprintf("severity changed from %s to %s", r.From, severity)
	case "confirm":
		what = "confirmed by the auditor as " + severity
	case "recheck":
		st.Audit.Recheck[fp] = reason
		if st.Stage == "report" {
			st.Stage = "review"
		}
	}
	Log(st, fmt.Sprintf("Auditor review of %s: %s.", f.Title, what), "finding")
	return nil
}

// ------------------------------------------------------------ stages

// AuditRefresh moves the audit to the report once every finding is reviewed.
func AuditRefresh(st *State, s *Store) error {
	if st.Stage != "review" {
		return nil
	}
	fs, err := ReadFindings(CurrentRun(st))
	if err != nil {
		return err
	}
	if len(PendingReviews(st, fs)) > 0 || len(st.Audit.Recheck) > 0 {
		return nil
	}
	st.Stage = "report"
	Log(st, "Every finding is reviewed: the report draft is ready for the executive summary and approval.", "stage")
	return nil
}

func unitCounts(units []*LedgerUnit) map[string]int {
	out := map[string]int{}
	for _, u := range units {
		out[u.Status]++
	}
	return out
}

// AuditNext is the `Next:` line for the orchestrator.
func AuditNext(st *State, s *Store) (string, error) {
	a, run := st.Audit, CurrentRun(st)
	waiting := OpenQuestions(st)
	switch st.Stage {
	case "scope":
		if a.Scope != nil && a.Scope.Status == "in-review" {
			return "Wait for the auditor to approve the rules of engagement in the console.", nil
		}
		if a.Scope != nil && a.Scope.Status == "changes-requested" {
			return "The auditor asked for changes to the rules of engagement: read their comment (`decisions`), then " +
				"`audit scope` again.", nil
		}
		return "Agree the rules of engagement with the auditor (what is in scope, profile quick/standard/deep, " +
			"budget in agent invocations), then submit them with `audit scope`.", nil
	case "recon":
		return fmt.Sprintf("Run the skill's Phase 1 (RECONNAISSANCE.md) with output directory %s: write "+
			"run-metadata.json, architecture.md and coverage-ledger.json there, then `audit plan submit`.", run.Dir), nil
	case "coverage-plan":
		if run.Plan == "changes-requested" {
			return "The auditor asked for changes to the coverage plan: read their comment (`decisions`), update " +
				"coverage-ledger.json, then `audit plan submit` again.", nil
		}
		return "Wait for the auditor to approve the coverage plan in the console.", nil
	case "hunt":
		units, err := ReadLedger(run)
		if err != nil {
			return "", err
		}
		c := unitCounts(units)
		if c["planned"]+c["in_progress"] > 0 {
			return fmt.Sprintf("Coverage plan approved. Run the hunting waves (HUNTING.md), candidate validation and "+
				"record verification (VALIDATION-AND-REPORTING.md phases 3-5) in %s; %d planned and %d in-progress "+
				"units left. Register each subagent with `agent start` and brief it with `audit brief <role>`.",
				run.Dir, c["planned"], c["in_progress"]), nil
		}
		return fmt.Sprintf("Every unit is closed. Finish validation and record verification, write findings.json in %s, "+
			"then `audit findings submit`.", run.Dir), nil
	case "review":
		if len(a.Recheck) > 0 {
			fps := make([]string, 0, len(a.Recheck))
			for fp := range a.Recheck {
				fps = append(fps, fp)
			}
			sort.Strings(fps)
			lines := []string{}
			for _, fp := range fps {
				lines = append(lines, fmt.Sprintf("%s (%s)", fp, a.Recheck[fp]))
			}
			return fmt.Sprintf("The auditor asked for another check of %s. Give each to a fresh candidate verifier with "+
				"the auditor's question, update findings.json, then `audit findings submit`.", strings.Join(lines, "; ")), nil
		}
		return "Wait for the auditor to review the findings in the console. Meanwhile, write the executive summary " +
			"draft with `audit summary --file <md>` if it isn't written.", nil
	case "report":
		if strings.TrimSpace(a.Summary) == "" {
			return "Write the executive summary draft (counts by final severity, the most serious findings, the overall " +
				"picture) with `audit summary --file <md>`. The auditor edits and approves it with the report.", nil
		}
		return "Wait for the auditor to approve the report in the console. Only they can.", nil
	case "done":
		if len(waiting) > 0 {
			return fmt.Sprintf("Wait for the user to answer %s in the console.", ids(waiting)), nil
		}
		return "The audit is done: its report is approved. `audit rerun` starts a new run on the same target.", nil
	}
	return "", nil
}

// AuditDerived is what the console and `status --json` show beyond the raw state.
type AuditDerived struct {
	Stage        string         `json:"stage"`
	Run          *AuditRun      `json:"run"`
	Units        map[string]int `json:"units"`
	UnitsTotal   int            `json:"units_total"`
	Verdicts     map[string]int `json:"verdicts"` // the agents' verdicts
	Final        map[string]int `json:"final"`    // confirmed findings by final severity
	ToReview     int            `json:"to_review"`
	Recheck      int            `json:"recheck"`
	Agents       map[string]int `json:"agents"` // registered agents by role (budget use)
	AgentsTotal  int            `json:"agents_total"`
	Budget       int            `json:"budget"`
	FilesError   string         `json:"files_error,omitempty"`
	HasLedger    bool           `json:"has_ledger"`
	HasFindings  bool           `json:"has_findings"`
	ReportReady  bool           `json:"report_ready"`
	LatestReport *ReportVersion `json:"latest_report"`
}

func DeriveAudit(st *State, s *Store) *AuditDerived {
	run := CurrentRun(st)
	d := &AuditDerived{Stage: st.Stage, Run: run, Units: map[string]int{}, Verdicts: map[string]int{},
		Final: map[string]int{}, Agents: map[string]int{}}
	for _, ag := range st.Agents {
		if ag.ID != Orchestrator && run != nil && ag.StartedAt >= run.StartedAt {
			d.Agents[ag.Role]++
			d.AgentsTotal++
		}
	}
	if run != nil {
		d.Budget = run.Budget
	}
	if st.Audit.Report != nil && len(st.Audit.Report.Versions) > 0 {
		d.LatestReport = st.Audit.Report.Versions[len(st.Audit.Report.Versions)-1]
	}
	units, err := ReadLedger(run)
	if err != nil {
		d.FilesError = err.Error()
	}
	d.HasLedger = units != nil
	d.Units = unitCounts(units)
	d.UnitsTotal = len(units)
	fs, err := ReadFindings(run)
	if err != nil {
		d.FilesError = err.Error()
	}
	d.HasFindings = fs != nil
	for _, f := range fs {
		d.Verdicts[f.Verdict]++
	}
	if st.Stage == "review" || st.Stage == "report" || st.Stage == "done" {
		for _, ff := range FinalFindings(st, fs) {
			if ff.Final == "confirmed" {
				d.Final[ff.Severity]++
			}
		}
		d.ToReview = len(PendingReviews(st, fs))
	}
	d.Recheck = len(st.Audit.Recheck)
	d.ReportReady = st.Stage == "report" && strings.TrimSpace(st.Audit.Summary) != ""
	return d
}

// ------------------------------------------------------------ transitions

// AuditScopeSubmit records the proposed rules of engagement and asks the auditor.
func AuditScopeSubmit(st *State, sc *AuditScope, body string) (*Question, error) {
	if st.Stage != "scope" {
		return nil, Gate("The rules of engagement are set in the scope stage (now: %s). `audit rerun` starts a new run.", st.Stage)
	}
	if !contains(AuditProfiles, sc.Profile) {
		return nil, Gate("Profile: %s.", strings.Join(AuditProfiles, ", "))
	}
	if sc.Budget < 0 {
		return nil, Gate("The budget is a number of agent invocations, or 0 for none.")
	}
	if sc.Paths == nil {
		sc.Paths = []string{}
	}
	sc.Status = "in-review"
	Supersede(st, &Effect{Type: "audit-scope"}, "new rules of engagement submitted.")
	st.Audit.Scope = sc
	return AddQuestion(st, "validation", "Approve the rules of engagement?", body,
		[]string{Approve, RequestChanges}, &Effect{Type: "audit-scope"}, Context{Stage: "scope"}, false, ApprovalHelp)
}

// AuditPlanSubmit asks the auditor to approve the coverage plan; the CLI has validated the files.
func AuditPlanSubmit(st *State, body string) (*Question, error) {
	if st.Stage != "recon" && st.Stage != "coverage-plan" {
		return nil, Gate("The coverage plan is submitted after reconnaissance (stage now: %s).", st.Stage)
	}
	run := CurrentRun(st)
	if run.Plan == "in-review" {
		return nil, Gate("The coverage plan is already waiting for the auditor.")
	}
	run.Plan = "in-review"
	st.Stage = "coverage-plan"
	return AddQuestion(st, "validation", "Approve the coverage plan?", body, []string{Approve, RequestChanges},
		&Effect{Type: "audit-plan"}, Context{Stage: "coverage-plan"}, false, map[string]string{
			Approve:        "Start hunting with this plan. Units the coverage critics add later appear as they come.",
			RequestChanges: "Say what to add, drop or change (areas, attack classes, profile, budget); the agent updates the ledger and asks again.",
		})
}

// AuditFindingsSubmit moves to the auditor's review; the CLI has validated the files.
func AuditFindingsSubmit(st *State, fs []*SkillFinding) error {
	if st.Stage != "hunt" && st.Stage != "review" && st.Stage != "report" {
		return Gate("Findings are submitted after hunting (stage now: %s).", st.Stage)
	}
	have := map[string]bool{}
	for _, f := range fs {
		have[f.Fingerprint] = true
	}
	// A recheck is answered by the new record: the auditor reviews it again.
	for fp := range st.Audit.Recheck {
		delete(st.Audit.Reviews, fp)
		delete(st.Audit.Recheck, fp)
	}
	for fp := range st.Audit.Reviews {
		if !have[fp] {
			delete(st.Audit.Reviews, fp)
		}
	}
	counts := map[string]int{}
	for _, f := range fs {
		counts[f.Verdict]++
	}
	st.Stage = "review"
	Log(st, fmt.Sprintf("Findings submitted: %d confirmed, %d needing validation, %d rejected. The auditor reviews them.",
		counts["confirmed"], counts["needs_validation"], counts["rejected"]), "finding")
	return nil
}

func applyAuditEffect(st *State, q *Question) {
	approved := q.Answer.Choice == Approve
	switch q.Effect.Type {
	case "audit-scope":
		sc := st.Audit.Scope
		if !approved {
			sc.Status = "changes-requested"
			Log(st, "Rules of engagement: changes requested.", "stage")
			return
		}
		sc.Status = "approved"
		n := len(st.Audit.Runs) + 1
		run := &AuditRun{ID: fmt.Sprintf("run-%d", n), Profile: sc.Profile, Budget: sc.Budget, Paths: sc.Paths,
			SourceRef: sc.SourceRef, Dirty: sc.Dirty, StartedAt: Now(), Plan: "draft"}
		run.Dir = filepath.Join(st.Project.Cwd, run.ID) // an audit's Cwd is its folder under ~/.lacarta/audits
		st.Audit.Runs = append(st.Audit.Runs, run)
		st.Audit.Reviews, st.Audit.Recheck, st.Audit.Summary, st.Audit.SummaryBy = map[string]*AuditReview{}, map[string]string{}, "", ""
		st.Stage = "recon"
		Log(st, fmt.Sprintf("Rules of engagement approved: %s starts (%s profile).", run.ID, run.Profile), "stage")
	case "audit-plan":
		run := CurrentRun(st)
		if !approved {
			run.Plan = "changes-requested"
			Log(st, "Coverage plan: changes requested.", "stage")
			return
		}
		run.Plan = "approved"
		st.Stage = "hunt"
		Log(st, "Coverage plan approved: hunting starts.", "stage")
	}
}

// NewAuditState: an audit's state and runs live in ~/.lacarta/audits/<pid>, kept in Project.Cwd.
func NewAuditState(pid, name, target, repo, execution string) *State {
	st := &State{
		Version: 1, Template: Audit,
		Project: Project{Name: name, Cwd: AuditHome(pid), CreatedAt: Now()},
		Stage:   "scope", Counters: map[string]int{},
		Audit: &AuditState{Target: target, Repo: repo, Execution: execution, Runs: []*AuditRun{},
			Reviews: map[string]*AuditReview{}, Recheck: map[string]string{}},
	}
	return st
}

// RegisterAudit adds an audit project to the hub.
func RegisterAudit(pid, name, target string) error {
	return HubEdit(func(h *Hub) error {
		h.Projects.Set(pid, &HubProject{ID: pid, Template: Audit, Name: name, Root: target, CreatedAt: Now()})
		return nil
	})
}

// AuditFor finds the open audit of a target folder (the session's cwd or a parent of it).
func AuditFor(cwd string) string {
	h, err := HubRead()
	if err != nil {
		return ""
	}
	cwd, _ = filepath.Abs(cwd)
	best := ""
	for _, k := range h.Projects.Keys() {
		p, _ := h.Projects.Get(k)
		if p.Template != Audit || p.Archived || p.Root == "" {
			continue
		}
		if cwd == p.Root || strings.HasPrefix(cwd, p.Root+string(filepath.Separator)) {
			if best == "" || len(p.Root) > len(bestRoot(h, best)) {
				best = k
			}
		}
	}
	return best
}

func bestRoot(h *Hub, pid string) string {
	p, _ := h.Projects.Get(pid)
	return p.Root
}
