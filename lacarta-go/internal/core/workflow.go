package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ------------------------------------------------------------ derived state

func BlockingFindings(st *State) []*Finding {
	out := []*Finding{}
	for _, f := range st.Findings {
		if Blocking[f.Severity] && Unresolved[f.Status] {
			out = append(out, f)
		}
	}
	return out
}

func BlockedReason(st *State) string {
	if st.Stage == "stopped" {
		return "The project was stopped by a decision."
	}
	fs := BlockingFindings(st)
	if len(fs) == 0 {
		return ""
	}
	ids := make([]string, len(fs))
	for i, f := range fs {
		ids[i] = fmt.Sprintf("%s (%s)", f.ID, f.Severity)
	}
	return fmt.Sprintf("Unresolved high or critical finding: %s.", join(ids, ", "))
}

func GetPhase(st *State, id int) (*Phase, error) {
	for _, p := range st.Phases {
		if p.ID == id {
			return p, nil
		}
	}
	ids := make([]int, len(st.Phases))
	for i, p := range st.Phases {
		ids[i] = p.ID
	}
	list := "none yet"
	if len(ids) > 0 {
		list = PyIntList(ids)
	}
	return nil, Gate("No phase %d. Phases: %s.", id, list)
}

func LatestReview(st *State, pid PhaseRef, rtype string) *Review {
	var last *Review
	for _, r := range st.Reviews {
		if r.Phase == pid && r.Type == rtype {
			last = r
		}
	}
	return last
}

func passed(r *Review) bool { return r != nil && r.Verdict == "pass" }

// The spec review and the approvals are tied to the exact text they judged: any edit to a
// spec file or to the plan after that makes them stale. The hashes match the prototype's.

func SpecPath(s *Store, kind string) string { return filepath.Join(s.Dir, "specs", kind+".md") }

func SpecFileHash(s *Store, kind string) (string, error) {
	b, err := os.ReadFile(SpecPath(s, kind))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func PlanHash(st *State) string {
	plan := make([][]any, len(st.Phases))
	for i, p := range st.Phases {
		plan[i] = []any{p.Title, p.Goal, nonNil(p.Scope), nonNil(p.Covers)}
	}
	sum := sha256.Sum256([]byte(pyDumpsCompact(plan)))
	return hex.EncodeToString(sum[:])
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func SpecsHash(st *State, s *Store) (string, error) {
	var b strings.Builder
	for _, k := range Specs {
		h, err := SpecFileHash(s, k.K)
		if err != nil {
			return "", err
		}
		b.WriteString(h)
	}
	b.WriteString(PlanHash(st))
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

func TodoLines(s *Store, kind string) ([]int, error) {
	b, err := os.ReadFile(SpecPath(s, kind))
	if err != nil {
		return nil, err
	}
	out := []int{}
	for i, line := range splitLines(string(b)) {
		if strings.Contains(line, "TODO") {
			out = append(out, i+1)
		}
	}
	return out, nil
}

// splitLines is str.splitlines() for the separators text files use.
func splitLines(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// SpecReviewStatus is "none", "failed", "stale" (specs changed since it passed) or "passed".
func SpecReviewStatus(st *State, s *Store) (string, error) {
	r := LatestReview(st, PhaseRef{Name: "spec"}, SpecReview)
	if r == nil {
		return "none", nil
	}
	if r.Verdict != "pass" {
		return "failed", nil
	}
	for _, f := range st.Findings {
		if f.Phase == (PhaseRef{Name: "spec"}) && Unresolved[f.Status] {
			return "failed", nil
		}
	}
	h, err := SpecsHash(st, s)
	if err != nil {
		return "", err
	}
	if r.SpecsHash == h {
		return "passed", nil
	}
	return "stale", nil
}

// Refresh invalidates approvals of edited specs, then advances the stage when its exit
// conditions are met. It runs after every write.
func Refresh(st *State, s *Store) error {
	if st.Stage != "specification" {
		return nil
	}
	for _, k := range Specs {
		sp := st.Spec(k.K)
		if sp == nil || (sp.Status != "in-review" && sp.Status != "approved") {
			continue
		}
		h, err := SpecFileHash(s, k.K)
		if err != nil {
			return err
		}
		if sp.SubmittedHash != h {
			sp.Status = "changed"
			Supersede(st, &Effect{Type: "spec-approval", Spec: k.K}, k.K+".md changed after submission.")
			Log(st, k.V+" spec edited after submission: needs a new spec review and approval.", "spec")
		}
	}
	if (st.Plan.Status == "in-review" || st.Plan.Status == "approved") && st.Plan.SubmittedHash != PlanHash(st) {
		st.Plan.Status = "changed"
		Supersede(st, &Effect{Type: "plan-approval"}, "the plan changed after submission.")
		Log(st, "Phase plan edited after submission: needs a new spec review and approval.", "plan")
	}
	for _, k := range Specs {
		if sp := st.Spec(k.K); sp == nil || sp.Status != "approved" {
			return nil
		}
	}
	if st.Plan.Status != "approved" {
		return nil
	}
	if r, err := SpecReviewStatus(st, s); err != nil || r != "passed" {
		return err
	}
	st.Stage = "implementation"
	Log(st, "Specification reviewed and approved. Implementation can start.", "stage")
	return nil
}

// NextAction is one sentence telling an agent what the framework expects next.
func NextAction(st *State, s *Store) (string, error) {
	stage := st.Stage
	if stage == "stopped" {
		return "The project is stopped. Nothing advances until the user restarts it.", nil
	}
	if len(PendingMessages(st)) > 0 {
		return "The user sent messages from the console: `inbox` shows them. Treat them as the user's words.", nil
	}
	for _, q := range st.Questions {
		if Discussing(q) {
			return fmt.Sprintf("The user wants to talk about %s before answering: read the thread and "+
				"`reply %s --message ...`, then `wait` again. The question stays open.", q.ID, q.ID), nil
		}
	}
	waiting := OpenQuestions(st)
	if st.Template == Single {
		if stage == "ended" {
			return "The session ended.", nil
		}
		if len(waiting) > 0 {
			return fmt.Sprintf("Wait for the user to answer %s in the console.", ids(waiting)), nil
		}
		return "Work on the task with the user.", nil
	}
	for _, q := range waiting {
		if q.Effect != nil && q.Effect.Type == "finding-triage" {
			return fmt.Sprintf("Blocked: wait for the user's decision on %s (`wait %s`). Do not write code.", q.ID, q.ID), nil
		}
	}
	if fs := BlockingFindings(st); len(fs) > 0 {
		f := fs[0]
		return fmt.Sprintf("Blocked on %s (%s): fix it, re-run the %s review, then `finding resolve %s --status fixed`. "+
			"Nothing else advances.", f.ID, f.Status, f.Review, f.ID), nil
	}
	switch stage {
	case "specification":
		incomplete := []string{}
		for _, k := range Specs {
			lines, err := TodoLines(s, k.K)
			if err != nil {
				return "", err
			}
			if len(lines) > 0 {
				incomplete = append(incomplete, ".lacarta/specs/"+k.K+".md")
			}
		}
		if len(incomplete) > 0 {
			return fmt.Sprintf("Finish writing the specs: %s still have TODOs.", join(incomplete, ", ")), nil
		}
		if len(st.Phases) == 0 {
			return "Define the phase plan with `phase add`.", nil
		}
		talks := []*Question{}
		for _, q := range waiting {
			if q.Kind != "validation" {
				talks = append(talks, q)
			}
		}
		if len(talks) > 0 {
			return fmt.Sprintf("Wait for the user to answer %s, then fold the answers into the specs.", ids(talks)), nil
		}
		review, err := SpecReviewStatus(st, s)
		if err != nil {
			return "", err
		}
		if review != "passed" {
			hint := map[string]string{"none": "Everything is written", "failed": "The last spec review failed: fix its findings",
				"stale": "The specs changed since the last spec review"}[review]
			return hint + ". Spawn a fresh spec reviewer with no memory (references/spec-review.md) and " +
				"record it with `review add --phase spec --type specification`.", nil
		}
		unsubmitted := []string{}
		for _, k := range Specs {
			if sp := st.Spec(k.K); sp != nil && (sp.Status == "draft" || sp.Status == "changes-requested" || sp.Status == "changed") {
				unsubmitted = append(unsubmitted, k.K)
			}
		}
		if len(unsubmitted) > 0 {
			return fmt.Sprintf("Spec review passed: submit for approval with `spec submit` (%s).", join(unsubmitted, ", ")), nil
		}
		if ps := st.Plan.Status; ps == "draft" || ps == "changes-requested" || ps == "changed" {
			return "Spec review passed: `plan submit`.", nil
		}
		if len(waiting) > 0 {
			return fmt.Sprintf("Wait for the user to answer %s in the console.", ids(waiting)), nil
		}
		return "Specifications are in review.", nil
	case "implementation":
		for _, p := range st.Phases {
			if p.Status == "planned" || p.Status == "done" {
				continue
			}
			if p.Status == "awaiting-approval" {
				return fmt.Sprintf("Wait for the user to approve phase %d in the console.", p.ID), nil
			}
			if missing := missingReviews(st, *NumPhase(p.ID)); len(missing) > 0 {
				return fmt.Sprintf("Phase %d in progress: finish its scope, then run the %s review(s) and record them "+
					"with `review add`.", p.ID, join(missing, " and ")), nil
			}
			return fmt.Sprintf("Phase %d reviews passed: `phase complete %d`.", p.ID, p.ID), nil
		}
		for _, p := range st.Phases {
			if p.Status == "planned" {
				return fmt.Sprintf("`phase start %d`.", p.ID), nil
			}
		}
		return "All phases done: `final start`, then run the final review with fresh agents.", nil
	case "final-review":
		if OpenQuestion(st, &Effect{Type: "final-approval"}) != nil {
			return "Wait for the user's final approval in the console.", nil
		}
		if missing := missingReviews(st, PhaseRef{Name: "final"}); len(missing) > 0 {
			return fmt.Sprintf("Run the final %s review(s) with fresh agents (references/final-review.md), then fix "+
				"and re-review until they pass.", join(missing, " and ")), nil
		}
		return "Final reviews passed: `final complete`.", nil
	}
	return "The project is done.", nil
}

func missingReviews(st *State, pid PhaseRef) []string {
	out := []string{}
	for _, t := range ReviewTypes {
		if !passed(LatestReview(st, pid, t)) {
			out = append(out, t)
		}
	}
	return out
}

func ids(qs []*Question) string {
	out := make([]string, len(qs))
	for i, q := range qs {
		out[i] = q.ID
	}
	return join(out, ", ")
}

// Derived is what the console and `status --json` add to the raw state.
type Derived struct {
	Next            string            `json:"next"`
	OpenQuestions   int               `json:"open_questions"`
	AgentStatus     map[string]string `json:"agent_status"`
	Listening       bool              `json:"listening"`
	Remote          bool              `json:"remote"`
	PendingMessages int               `json:"pending_messages"`
	Blocked         *string           `json:"blocked"`
	SpecReview      string            `json:"spec_review,omitempty"`
	OpenFindings    map[string]int    `json:"open_findings,omitempty"`
	SpecsApproved   *int              `json:"specs_approved,omitempty"`
	PhasesDone      *int              `json:"phases_done,omitempty"`
}

func DeriveState(st *State, s *Store) (*Derived, error) {
	next, err := NextAction(st, s)
	if err != nil {
		return nil, err
	}
	d := &Derived{Next: next, OpenQuestions: len(OpenQuestions(st)), AgentStatus: map[string]string{},
		Listening: IsListening(st), Remote: RemoteOn(st), PendingMessages: len(PendingMessages(st))}
	for _, a := range st.Agents {
		d.AgentStatus[a.ID] = AgentStatus(st, a)
	}
	if st.Template == Single {
		return d, nil
	}
	if b := BlockedReason(st); b != "" {
		d.Blocked = &b
	}
	if d.SpecReview, err = SpecReviewStatus(st, s); err != nil {
		return nil, err
	}
	d.OpenFindings = map[string]int{}
	for _, sev := range Severities {
		d.OpenFindings[sev] = 0
	}
	for _, f := range st.Findings {
		if Unresolved[f.Status] {
			d.OpenFindings[f.Severity]++
		}
	}
	approved, done := 0, 0
	for _, k := range st.Specs.Keys() {
		if st.Spec(k).Status == "approved" {
			approved++
		}
	}
	for _, p := range st.Phases {
		if p.Status == "done" {
			done++
		}
	}
	d.SpecsApproved, d.PhasesDone = &approved, &done
	return d, nil
}

// TriageBody is the triage question: enough to decide without opening the code or the report.
func TriageBody(st *State, f *Finding) string {
	meaning := SeverityMeaning[f.Review][f.Severity]
	source := "the specification review"
	if f.Review != SpecReview {
		source = fmt.Sprintf("the %s review of %s", f.Review, Where(f.Phase))
	}
	out := []string{fmt.Sprintf("**%s** finding from %s. Development is stopped until you decide.", strings.ToUpper(f.Severity), source)}
	if meaning != "" {
		out = append(out, fmt.Sprintf("_%s means: %s_", Capitalize(f.Severity), meaning))
	}
	if f.Detail != "" {
		out = append(out, "### What is wrong\n\n"+f.Detail)
	}
	out = append(out, "### How it goes wrong\n\n"+f.Scenario)
	if f.Location != "" {
		out = append(out, "### Where\n\n`"+f.Location+"`")
	}
	out = append(out, "### Proposed fix\n\n"+f.Recommendation)
	if r := LatestReview(st, f.Phase, f.Review); r != nil {
		out = append(out, fmt.Sprintf("Full review report: `%s` (%s).", r.Report, r.ID))
	}
	out = append(out, "Not sure which option fits? Use **Talk about this** to ask the agent first; "+
		"development stays stopped meanwhile.")
	return strings.Join(out, "\n\n")
}
