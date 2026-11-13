// Package auditreport builds a security audit's report from the run's files and the auditor's
// review, with a fixed AI-assistance disclosure, and, once the auditor approves, writes the
// final version: markdown, a print-ready HTML page, a PDF when Chromium is installed, and a
// signed provenance manifest. The report is a deterministic template, never a model's text,
// apart from the executive summary the auditor approves.
package auditreport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"lacarta/internal/core"
)

// Doc is everything the report shows.
type Doc struct {
	ID             string
	Project        string
	Target         string
	Repo           string
	Run            *core.AuditRun
	Execution      string
	Skill          string // the skill's version
	Plugins        []string
	Auditor        string
	ApprovedAt     string // empty for a draft
	Summary        string
	Findings       []*core.FinalFinding
	Units          []*core.LedgerUnit
	Wishes         []string
	AI             AIWork
	Disclosure     string
	Adjustments    []Adjustment
	Hardening      []string
	Incomplete     string
	KeyFingerprint string // the signing key, known at approval
}

// AIWork is what the disclosure states, from the records.
type AIWork struct {
	Models          map[string]string `json:"models"` // role -> model name
	Agents          map[string]int    `json:"agents"` // subagents by role
	Units           int               `json:"units"`
	Candidates      int               `json:"candidates"` // records the agents wrote
	ByAgents        map[string]int    `json:"by_agents"`  // the agents' verdicts
	Reviewed        int               `json:"reviewed_by_auditor"`
	SeverityChanged int               `json:"severity_changed"`
	Rejected        int               `json:"rejected_by_auditor"`
	Confirmed       int               `json:"confirmed_by_auditor"`
	OutOfScope      int               `json:"out_of_scope_by_auditor"`
}

type Adjustment struct {
	Finding, Change, Reason string
}

var severityLabel = map[string]string{"critical": "Critical", "high": "High", "medium": "Medium", "low": "Low", "informational": "Informational"}

// Build assembles the report of the current run. auditor and approvedAt are empty for a draft.
func Build(pid string, st *core.State, auditor, approvedAt string, version int) (*Doc, error) {
	a, run := st.Audit, core.CurrentRun(st)
	if run == nil {
		return nil, core.Gate("No audit run yet.")
	}
	fs, err := core.ReadFindings(run)
	if err != nil {
		return nil, err
	}
	units, err := core.ReadLedger(run)
	if err != nil {
		return nil, err
	}
	d := &Doc{ID: fmt.Sprintf("%s-%s-v%d", pid, run.ID, version), Project: st.Project.Name, Target: a.Target,
		Repo: a.Repo, Run: run, Execution: a.Execution, Auditor: auditor, ApprovedAt: approvedAt,
		Summary: strings.TrimSpace(a.Summary), Findings: core.FinalFindings(st, fs), Units: units}
	ids := make([]string, 0, len(st.Plugins))
	for id, p := range st.Plugins {
		if p.Enabled {
			ids = append(ids, id+"@"+short(p.Ref))
		}
		if id == "security-audit" {
			d.Skill = short(p.Ref)
		}
	}
	sort.Strings(ids)
	d.Plugins = ids
	for _, q := range st.Questions {
		if strings.HasPrefix(q.Title, "Wishlist: ") && q.Answer != nil && q.Answer.Choice != "Provide it" {
			d.Wishes = append(d.Wishes, strings.TrimPrefix(q.Title, "Wishlist: "))
		}
	}
	d.AI = aiWork(st, run, units, fs, d.Findings)
	d.Disclosure = Disclosure(d)
	for _, f := range d.Findings {
		r := f.Review
		if r == nil {
			continue
		}
		switch r.Action {
		case "severity":
			d.Adjustments = append(d.Adjustments, Adjustment{f.Title, fmt.Sprintf("Severity changed from %s to %s",
				label(r.From), label(r.Severity)), r.Reason})
		case "reject":
			d.Adjustments = append(d.Adjustments, Adjustment{f.Title, "Rejected as a false positive (the agents had confirmed it, " +
				label(f.AgentSeverity()) + ")", r.Reason})
		case "confirm":
			d.Adjustments = append(d.Adjustments, Adjustment{f.Title, "Confirmed by the auditor as " + label(r.Severity) +
				" (the agents had left it as needing validation)", r.Reason})
		case "out-of-scope":
			d.Adjustments = append(d.Adjustments, Adjustment{f.Title, "Marked out of scope", r.Reason})
		}
	}
	var meta struct {
		RunStatus        string `json:"run_status"`
		IncompleteReason string `json:"incomplete_reason"`
	}
	if b, err := os.ReadFile(core.RunFile(run, "run-metadata.json")); err == nil && json.Unmarshal(b, &meta) == nil &&
		meta.RunStatus == "incomplete" {
		d.Incomplete = firstOf(meta.IncompleteReason, "the agents marked the run incomplete")
	}
	return d, nil
}

func aiWork(st *core.State, run *core.AuditRun, units []*core.LedgerUnit, fs []*core.SkillFinding, ff []*core.FinalFinding) AIWork {
	w := AIWork{Models: map[string]string{}, Agents: map[string]int{}, Units: len(units), Candidates: len(fs),
		ByAgents: map[string]int{}}
	for _, ag := range st.Agents {
		if ag.ID == core.Orchestrator || ag.StartedAt >= run.StartedAt {
			r, _ := core.KindByID(core.Audit).Role(ag.Role)
			name := firstOf(r.Label, ag.Role)
			if ag.ID != core.Orchestrator {
				w.Agents[name]++
			}
			if ag.Model != "" {
				w.Models[name] = core.ModelName(ag.Model)
			}
		}
	}
	for _, f := range fs {
		w.ByAgents[f.Verdict]++
	}
	for _, f := range ff {
		if f.Review == nil || f.Review.Action == "recheck" {
			continue
		}
		w.Reviewed++
		switch f.Review.Action {
		case "severity":
			w.SeverityChanged++
		case "reject":
			w.Rejected++
		case "confirm":
			w.Confirmed++
		case "out-of-scope":
			w.OutOfScope++
		}
	}
	return w
}

// Disclosure is the fixed AI-assistance statement. It can't be edited or turned off.
func Disclosure(d *Doc) string {
	models := []string{}
	roles := make([]string, 0, len(d.AI.Models))
	for r := range d.AI.Models {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	for _, r := range roles {
		models = append(models, fmt.Sprintf("%s for %s", d.AI.Models[r], strings.ToLower(r)))
	}
	who := "The auditor named in the sign-off"
	if d.Auditor != "" {
		who = d.Auditor
	}
	modelText := ""
	if len(models) > 0 {
		modelText = " (" + strings.Join(models, ", ") + ")"
	}
	return fmt.Sprintf("This security audit was performed with AI assistance. AI agents%s mapped the code, searched it "+
		"for vulnerabilities, verified the candidates independently and drafted the finding records, using lacarta and "+
		"Cloudflare's security-audit methodology. %s, a human auditor, reviewed every finding (%s reviewed: %s changed, "+
		"%d rejected, %d confirmed by the auditor), approved this report and is responsible for its content.",
		modelText, who, Plural(d.AI.Reviewed, "finding"), Plural(d.AI.SeverityChanged, "severity"), d.AI.Rejected, d.AI.Confirmed)
}

// Plural: "1 finding", "2 findings", "3 severities".
func Plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	if strings.HasSuffix(word, "y") {
		return fmt.Sprintf("%d %sies", n, strings.TrimSuffix(word, "y"))
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func label(s string) string { return firstOf(severityLabel[s], s) }

func short(ref string) string {
	if len(ref) > 12 {
		return ref[:12]
	}
	return ref
}

func firstOf(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// Counts by final verdict and severity.
func (d *Doc) Count(final, severity string) int {
	n := 0
	for _, f := range d.Findings {
		if f.Final == final && (severity == "" || f.Severity == severity) {
			n++
		}
	}
	return n
}

func (d *Doc) Of(final string) []*core.FinalFinding {
	out := []*core.FinalFinding{}
	for _, f := range d.Findings {
		if f.Final == final {
			out = append(out, f)
		}
	}
	return out
}

// Provenance line of one finding: who found, verified and reviewed it.
func Provenance(d *Doc, f *core.FinalFinding) string {
	found := "Found and verified by AI agents"
	if m := d.AI.Models["Hunter"]; m != "" {
		found = "Found by an AI agent (" + m + ")"
		if v := d.AI.Models["Candidate verifier"]; v != "" {
			found += ", verified by an independent AI agent (" + v + ")"
		}
	}
	review := "not reviewed by the auditor"
	if f.Review != nil {
		switch f.Review.Action {
		case "accept", "keep":
			review = "reviewed and accepted by the auditor"
		case "severity":
			review = "reviewed by the auditor, who changed its severity"
		case "confirm":
			review = "confirmed by the auditor"
		case "reject":
			review = "rejected by the auditor"
		case "out-of-scope":
			review = "marked out of scope by the auditor"
		}
	}
	return found + "; " + review + "."
}

func coverageRows(units []*core.LedgerUnit) [][]string {
	type key struct{ sub, class string }
	agg := map[key]map[string]int{}
	for _, u := range units {
		k := key{u.Subsystem, u.AttackClass}
		if agg[k] == nil {
			agg[k] = map[string]int{}
		}
		agg[k][u.Status]++
	}
	keys := make([]key, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].sub != keys[j].sub {
			return keys[i].sub < keys[j].sub
		}
		return keys[i].class < keys[j].class
	})
	rows := [][]string{}
	for _, k := range keys {
		parts := []string{}
		sts := make([]string, 0, len(agg[k]))
		for s := range agg[k] {
			sts = append(sts, s)
		}
		sort.Strings(sts)
		for _, s := range sts {
			parts = append(parts, fmt.Sprintf("%d %s", agg[k][s], strings.ReplaceAll(s, "_", " ")))
		}
		rows = append(rows, []string{k.sub, k.class, strings.Join(parts, ", ")})
	}
	return rows
}

// Gaps: units the run did not cover, with the reason when the ledger has one.
func gaps(units []*core.LedgerUnit) []string {
	out := []string{}
	for _, u := range units {
		if u.Status == "deferred" || u.Status == "blocked" || u.Status == "out_of_scope" {
			why := u.DeferredReason
			if why == "" && len(u.Unresolved) > 0 {
				why = strings.Trim(string(u.Unresolved[0]), `"`)
			}
			line := fmt.Sprintf("%s / %s / %s: %s", u.Subsystem, u.Surface, u.AttackClass, strings.ReplaceAll(u.Status, "_", " "))
			if why != "" {
				line += " (" + why + ")"
			}
			out = append(out, line)
		}
	}
	return out
}

func pathOf(dir string, parts ...string) string {
	return filepath.Join(append([]string{dir}, parts...)...)
}
