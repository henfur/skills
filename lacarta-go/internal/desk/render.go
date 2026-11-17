// Package desk runs a review desk's loop around the core rules: polling the forge, claiming a
// merge request, briefing reviewers, checking their records, building and rendering the plan,
// and publishing it with the desk's write identity. Docs: docs/design/pr-review.md.
package desk

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"lacarta/internal/core"
	"lacarta/skill"
)

// NoteData is what a summary or update template sees.
type NoteData struct {
	Desk        string
	Profile     string
	Title       string
	MR          *core.DeskItem
	Record      *core.DeskRecord
	Findings    []*core.DeskFinding // effective findings, most serious first
	New         []*core.DeskFinding
	Open        []*core.DeskFinding
	Fixed       []string // titles
	VerdictLine string
	Injections  int
	Sections    []*NoteData // combined note: one per profile
}

// InlineData is what an inline template sees.
type InlineData struct {
	Desk    string
	Profile string
	MR      *core.DeskItem
	Finding *core.DeskFinding
}

var sevIcons = map[string]string{"critical": "🔴", "high": "🟠", "medium": "🟡", "low": "🔵", "info": "⚪"}

// funcs is the whole function map templates get: no file access, no commands.
var funcs = template.FuncMap{
	"sevIcon": func(s string) string { return sevIcons[s] },
	"short": func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	},
	"join":  strings.Join,
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"cell": func(s string) string {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " ")
		return strings.ReplaceAll(s, "|", "\\|")
	},
}

// ProfileTitle: "security" -> "Security review".
func ProfileTitle(p string) string {
	if p == "" {
		return "Review"
	}
	return strings.ToUpper(p[:1]) + strings.ReplaceAll(p[1:], "-", " ") + " review"
}

// templateText finds a template: the desk's <name>.md, then its <kind>.md, then the built-in ones.
func templateText(root, name, kind string) (string, string, error) {
	cands := []string{name + ".md"}
	if kind != "" && name != kind {
		cands = append(cands, kind+".md")
	}
	for _, c := range cands {
		p := filepath.Join(root, "templates", c)
		if b, err := os.ReadFile(p); err == nil {
			return string(b), p, nil
		}
	}
	for _, c := range cands {
		if b, err := fs.ReadFile(skill.Desk, "desk/templates/"+c); err == nil {
			return string(b), "built-in " + c, nil
		}
	}
	return "", "", fmt.Errorf("no template %s.md", name)
}

func render(root, name, kind string, data any) (string, error) {
	text, src, err := templateText(root, name, kind)
	if err != nil {
		return "", err
	}
	t, err := template.New(filepath.Base(src)).Funcs(funcs).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", core.Gate("Template %s: %v", src, err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", core.Gate("Template %s: %v", src, err)
	}
	if b.Len() > 60_000 {
		return "", core.Gate("Template %s renders %d characters; GitLab notes hold far less in practice. Shorten it.", src, b.Len())
	}
	return strings.TrimSpace(b.String()), nil
}

func effective(fs []*core.DeskFinding) []*core.DeskFinding {
	out := []*core.DeskFinding{}
	for _, f := range fs {
		if f.Effective() != "" {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return core.AtLeast(out[i].Effective(), out[j].Effective()) && out[i].Effective() != out[j].Effective()
	})
	return out
}

// VerdictLine: "Good to merge", "Merge with caution (2 medium)", "Do not merge (1 high, 1 low)".
func VerdictLine(fs []*core.DeskFinding, threshold string) string {
	if threshold == "" {
		threshold = "high"
	}
	count := map[string]int{}
	blocking := false
	for _, f := range fs {
		count[f.Effective()]++
		blocking = blocking || core.AtLeast(f.Effective(), threshold)
	}
	if len(fs) == 0 {
		return "✅ Good to merge"
	}
	parts := []string{}
	for _, s := range core.Severities {
		if count[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count[s], s))
		}
	}
	if blocking {
		return "🛑 Do not merge (" + strings.Join(parts, ", ") + ")"
	}
	return "⚠️ Merge with caution (" + strings.Join(parts, ", ") + ")"
}

func noteData(name, prof string, cfg *core.DeskConfig, it *core.DeskItem, r *core.DeskRecord, plan *core.DeskPlan,
	posted map[string]*core.DeskPosted) *NoteData {
	d := &NoteData{Desk: name, Profile: prof, Title: ProfileTitle(prof), MR: it, Record: r, Fixed: []string{},
		New: []*core.DeskFinding{}, Open: []*core.DeskFinding{}}
	d.Findings = effective(r.Findings)
	for _, f := range d.Findings {
		if contains(plan.Open, f.Fingerprint) {
			d.Open = append(d.Open, f)
		} else {
			d.New = append(d.New, f)
		}
	}
	for _, fp := range plan.Fixed {
		if po := posted[fp]; po != nil && po.Profile == prof {
			d.Fixed = append(d.Fixed, po.Title)
		}
	}
	d.Injections = len(r.InjectionAttempts)
	d.VerdictLine = VerdictLine(d.Findings, cfg.Policy.RequestChanges)
	return d
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// RenderPlan writes each step's body (the part the desk's templates control).
func RenderPlan(root, name string, cfg *core.DeskConfig, it *core.DeskItem, recs map[string]*core.DeskRecord,
	plan *core.DeskPlan, posted map[string]*core.DeskPosted) error {
	for _, s := range plan.Steps {
		var err error
		switch s.Kind {
		case "note":
			if s.Template == "combined.summary" {
				all := []*core.DeskFinding{}
				d := &NoteData{Desk: name, MR: it, Fixed: []string{}}
				for _, p := range plan.Profiles {
					sec := noteData(name, p, cfg, it, recs[p], plan, posted)
					d.Sections = append(d.Sections, sec)
					all = append(all, sec.Findings...)
					d.Fixed = append(d.Fixed, sec.Fixed...)
					d.Injections += sec.Injections
				}
				d.Findings = effective(all)
				d.VerdictLine = VerdictLine(d.Findings, cfg.Policy.RequestChanges)
				s.Body, err = render(root, "combined.summary", "", d)
				break
			}
			tmpl := cfg.Profiles[s.Profile].Template
			s.Body, err = render(root, tmpl+"."+s.Template, s.Template, noteData(name, s.Profile, cfg, it, recs[s.Profile], plan, posted))
		case "inline":
			var f *core.DeskFinding
			for _, x := range recs[s.Profile].Findings {
				if x.Fingerprint == s.Fingerprint {
					f = x
				}
			}
			s.Body, err = render(root, cfg.Profiles[s.Profile].Template+".inline", "inline",
				&InlineData{Desk: name, Profile: s.Profile, MR: it, Finding: f})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Envelope wraps a body in lacarta's fixed parts: the hidden marker, and the footer with the
// AI-assistance disclosure and the reviewed head. No template, setting or edit removes it (SR-29).
func Envelope(pid, deskName string, it *core.DeskItem, s *core.PlanStep, models []string) string {
	marker := fmt.Sprintf("<!-- lacarta:review desk=%s mr=%s sha=%s kind=%s", pid, it.Key, it.HeadSHA, s.Kind)
	if s.Profile != "" {
		marker += " profile=" + s.Profile
	}
	if s.Fingerprint != "" {
		marker += " fp=" + s.Fingerprint
	}
	marker += " -->"
	who := "AI agents"
	if len(models) > 0 {
		who += " (" + strings.Join(models, ", ") + ")"
	}
	footer := fmt.Sprintf("<sub>Reviewed at `%s` · automated review by %s, desk %s · reply to discuss; a human owns the merge.</sub>",
		shortSHA(it.HeadSHA), who, deskName)
	if s.Kind == "inline" {
		footer = fmt.Sprintf("<sub>Automated review by %s · desk %s</sub>", who, deskName)
	}
	return marker + "\n" + strings.TrimSpace(s.Body) + "\n\n" + footer
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// CheckTemplates renders every template the desk uses against a fixture, so a typo never reaches the forge.
func CheckTemplates(root string, cfg *core.DeskConfig) []string {
	it := &core.DeskItem{Key: "group/project!1", Project: "group/project", IID: 1, Title: "Fixture", Author: "someone",
		HeadSHA: "0123456789abcdef", LastReviewedSHA: "fedcba9876543210"}
	f := &core.DeskFinding{ID: "F-1", Fingerprint: "x:class:file:symbol", Class: "class", Severity: "medium", Title: "A finding",
		File: "src/a.go", Line: 3, Table: "What is wrong.", Body: "Why.", Fix: "How.", Suggestion: "code", Confidence: "confirmed"}
	errs := []string{}
	for _, p := range cfg.ProfileNames() {
		f.Fingerprint = p + ":class:file:symbol"
		r := &core.DeskRecord{Profile: p, Status: "complete", Verdict: "comments", Summary: "Summary.",
			Findings: []*core.DeskFinding{f}, Notes: []string{}, InjectionAttempts: []core.DeskInjection{}}
		plan := &core.DeskPlan{Profiles: []string{p}, New: []string{f.Fingerprint}, Open: []string{}, Fixed: []string{}}
		for _, k := range []string{"summary", "update"} {
			if _, err := render(root, cfg.Profiles[p].Template+"."+k, k, noteData("desk", p, cfg, it, r, plan, nil)); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if _, err := render(root, cfg.Profiles[p].Template+".inline", "inline", &InlineData{Desk: "desk", Profile: p, MR: it, Finding: f}); err != nil {
			errs = append(errs, err.Error())
		}
	}
	return errs
}
