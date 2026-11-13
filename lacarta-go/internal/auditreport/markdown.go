package auditreport

import (
	"fmt"
	"strings"
)

// Markdown renders the report. A draft (no ApprovedAt) says so at the top and has no sign-off.
func Markdown(d *Doc) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	draft := d.ApprovedAt == ""
	w("---\nlacarta_report: %s\nai_assisted: true\nauditor: %s\napproved_at: %s\nmodels: [%s]\n"+
		"verify: report.provenance.json and report.provenance.json.sig, see \"How to verify this report\"\n---\n\n",
		d.ID, yamlStr(d.Auditor), yamlStr(d.ApprovedAt), strings.Join(modelList(d), ", "))
	if draft {
		w("> **DRAFT: not approved.** This draft is for the auditor's review; it is not a report.\n\n")
	}
	w("# Security audit report: %s\n\n", d.Project)
	w("> **AI-assisted audit.** %s\n\n", d.Disclosure)
	w("| | |\n| --- | --- |\n")
	w("| Report | `%s` |\n| Target | `%s` |\n", d.ID, d.Target)
	ref := d.Run.SourceRef
	if d.Run.Dirty {
		ref += " (with uncommitted changes)"
	}
	w("| Source revision | `%s` |\n| Run | %s, %s profile, started %s |\n", firstOf(ref, "unknown"), d.Run.ID, d.Run.Profile, d.Run.StartedAt)
	w("| Execution | %s |\n", execution(d.Execution))
	w("| Methodology | Cloudflare security-audit skill %s; plugins: %s |\n", firstOf(d.Skill, "?"), firstOf(strings.Join(d.Plugins, ", "), "none"))
	if draft {
		w("| Approval | not approved yet |\n\n")
	} else {
		w("| Approval | **Reviewed and approved by %s on %s, who is responsible for this report.** |\n\n", d.Auditor, d.ApprovedAt)
	}
	if d.Incomplete != "" {
		w("> **Incomplete run:** %s. Coverage and findings below are partial.\n\n", d.Incomplete)
	}

	w("## 1. Executive summary\n\n")
	w("| Critical | High | Medium | Low | Informational | Needs validation |\n| --- | --- | --- | --- | --- | --- |\n")
	w("| %d | %d | %d | %d | %d | %d |\n\n", d.Count("confirmed", "critical"), d.Count("confirmed", "high"),
		d.Count("confirmed", "medium"), d.Count("confirmed", "low"), d.Count("confirmed", "informational"), d.Count("needs_validation", ""))
	w("%s\n\n", firstOf(d.Summary, "_The executive summary is not written yet._"))

	w("## 2. Scope and method\n\n")
	if len(d.Run.Paths) > 0 {
		w("In scope: %s. Everything else in the repository was out of scope.\n\n", codeList(d.Run.Paths))
	} else {
		w("In scope: the whole repository at the revision above.\n\n")
	}
	w("Method: AI agents mapped the architecture and trust boundaries and seeded a coverage plan, which the auditor " +
		"approved. Hunters then worked through it unit by unit (surface × boundary × attack class), coverage critics " +
		"looked for gaps after each wave, and every candidate went to a fresh verifier that tried to disprove it. " +
		"Every confirmed finding and every lead needing validation was then reviewed by the auditor. " +
		"No live or deployed system was tested; target code ran only in a sandbox without network access.\n\n")

	w("## 3. Coverage\n\n")
	rows := coverageRows(d.Units)
	if len(rows) == 0 {
		w("_No coverage ledger._\n\n")
	} else {
		w("| Area | Attack class | Units |\n| --- | --- | --- |\n")
		for _, r := range rows {
			w("| %s | %s | %s |\n", cell(r[0]), cell(r[1]), cell(r[2]))
		}
		w("\n")
	}
	if g := gaps(d.Units); len(g) > 0 {
		w("Not covered in this run:\n\n")
		for _, x := range g {
			w("- %s\n", x)
		}
		w("\n")
	}
	w("One audit run never exhausts a target: repeated runs find more. This report covers the units above only.\n\n")

	w("## 4. Findings\n\n")
	conf := d.Of("confirmed")
	if len(conf) == 0 {
		w("No confirmed vulnerability in the covered units.\n\n")
	}
	for i, f := range conf {
		w("### 4.%d %s: %s\n\n", i+1, label(f.Severity), f.Title)
		w("- **Severity:** %s", label(f.Severity))
		if f.SkillFinding.Severity != nil {
			w(" (likelihood %s, impact %s)", f.SkillFinding.Severity.Likelihood.Score, f.SkillFinding.Severity.Impact.Score)
		}
		if f.Review != nil && f.Review.Action == "severity" {
			w(". The agents rated it %s; the auditor changed it: %s", label(f.Review.From), f.Review.Reason)
		}
		w("\n- **Id:** `%s`\n- **Provenance:** %s\n\n", f.Fingerprint, Provenance(d, f))
		w("**What is wrong.** %s\n\n", f.Description)
		if f.Cause() != "" {
			w("**Root cause.** %s\n\n", f.Cause())
		}
		if f.IntendedBehavior != "" {
			w("**Intended behaviour.** %s\n\n", f.IntendedBehavior)
		}
		if len(f.Trace) > 0 {
			w("**Source trace.**\n\n")
			for j, t := range f.Trace {
				w("%d. `%s:%d`%s: %s\n", j+1, t.File, t.Line, kind(t.Kind), t.Description)
			}
			w("\n")
		}
		if len(f.Conditions) > 0 {
			w("**Conditions.**\n\n")
			for _, c := range f.Conditions {
				w("- %s: %s\n", strings.ReplaceAll(c.Kind, "_", " "), c.Description)
			}
			w("\n")
		}
		if e := f.Execution; e != nil {
			w("**Reproduction procedure** (local and sandboxed; never against a live system).\n\n")
			w("- Attacker: %s\n", e.AttackerPerspective)
			for _, p := range e.Payloads {
				w("- Input: %s\n", p)
			}
			w("\nSteps:\n\n")
			for j, s := range e.Instructions {
				w("%d. %s\n", j+1, s)
			}
			w("\nObserved result: %s\n\n", e.ObservedResult)
		}
		if len(f.Evidence) > 0 {
			w("**Evidence.**\n\n")
			for _, e := range f.Evidence {
				w("- `%s:%d`: %s\n", e.File, e.Line, e.Description)
			}
			w("\n")
		}
		if f.SkillFinding.Severity != nil {
			w("**Impact.** %s\n\n", f.SkillFinding.Severity.Impact.Reason)
		}
		if r := f.Remediation; r != nil {
			w("**How to fix it.** %s\n\n", r.Strategy)
			for _, c := range r.CodeChanges {
				w("In `%s`:\n\n```\n%s\n```\n\n", c.FileName, c.FixedCode)
			}
			w("Add a regression test that reproduces the steps above and fails before the fix.\n\n")
		}
	}

	w("## 5. Needs validation\n\n")
	nv := d.Of("needs_validation")
	if len(nv) == 0 {
		w("None.\n\n")
	}
	for _, f := range nv {
		w("### %s\n\n%s\n\n", f.Title, f.Description)
		if f.Cause() != "" {
			w("Claimed root cause: %s\n\n", f.Cause())
		}
		for _, x := range f.Blockers {
			w("- Unresolved: %s\n", x)
		}
		if p := f.ValidationPlan; p != nil {
			w("- How to check locally: %s\n- How to check in a test deployment: %s\n", p.Local, p.Deployment)
		}
		w("- Provenance: %s\n\n", Provenance(d, f))
	}

	if len(d.Wishes) > 0 {
		w("## 6. Limitations\n\nThe agents asked for these and didn't get them; their leads stay open:\n\n")
		for _, x := range d.Wishes {
			w("- %s\n", x)
		}
		w("\n")
	}

	w("## How this audit was done\n\n%s\n\n", d.Disclosure)
	w("| Work | By |\n| --- | --- |\n")
	w("| Architecture map and coverage plan (%s) | AI agents; plan approved by the auditor |\n", Plural(d.AI.Units, "unit"))
	w("| Hunting and candidate validation (%s: %d confirmed, %d needing validation, %d rejected) | AI agents |\n",
		Plural(d.AI.Candidates, "record"), d.AI.ByAgents["confirmed"], d.AI.ByAgents["needs_validation"], d.AI.ByAgents["rejected"])
	w("| Review of every finding (%d) and the final severities | %s |\n", d.AI.Reviewed, firstOf(d.Auditor, "the auditor"))
	w("| This report's text | generated from the records; executive summary approved by the auditor |\n\n")
	if len(d.AI.Agents) > 0 {
		w("Agents: %s.\n\n", agentList(d))
	}

	w("## Appendix A: auditor adjustments\n\n")
	if len(d.Adjustments) == 0 {
		w("None: the auditor accepted the agents' records as they were.\n\n")
	} else {
		w("| Finding | Change | Reason |\n| --- | --- | --- |\n")
		for _, a := range d.Adjustments {
			w("| %s | %s | %s |\n", cell(a.Finding), cell(a.Change), cell(a.Reason))
		}
		w("\n")
	}
	if rej := d.Of("rejected"); len(rej) > 0 {
		w("## Appendix B: rejected candidates\n\n")
		for _, f := range rej {
			why := f.Reason
			if f.Review != nil && f.Review.Action == "reject" {
				why = "the auditor: " + f.Review.Reason
			}
			w("- %s: %s\n", f.Title, why)
		}
		w("\n")
	}
	w("## How to verify this report\n\n")
	w("This report was produced with AI assistance by lacarta. It comes with `report.provenance.json`, which states that, " +
		"lists the files' SHA-256 hashes and the hashes of the agents' records, and `report.provenance.json.sig`, its " +
		"signature (Ed25519, OpenSSH format). To check it, with OpenSSH only:\n\n")
	w("```\nssh-keygen -Y verify -f allowed_signers -I %s -n lacarta-report -s report.provenance.json.sig < report.provenance.json\nsha256sum report.md report.pdf   # compare with \"files\" in report.provenance.json\n```\n\n",
		strings.ReplaceAll(firstOf(d.Auditor, "AUDITOR"), " ", "_"))
	if d.KeyFingerprint != "" {
		w("The signing key's fingerprint is `%s`. ", d.KeyFingerprint)
	}
	w("`allowed_signers` holds the auditor's public key; ask them for it, or compare its fingerprint with one they " +
		"published. A copy of this report without this section, without the AI-assistance notice or without a valid " +
		"signature has not been verified.\n")
	return b.String()
}

func execution(e string) string {
	if e == "source-only" {
		return "source only: no target code was run"
	}
	return "sandboxed: target code ran only in bubblewrap, without network, with the target read-only"
}

func kind(k string) string {
	if k == "" {
		return ""
	}
	return " (" + k + ")"
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

func codeList(xs []string) string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = "`" + x + "`"
	}
	return strings.Join(out, ", ")
}

func yamlStr(s string) string { return fmt.Sprintf("%q", s) }

func modelList(d *Doc) []string {
	set := map[string]bool{}
	out := []string{}
	for _, m := range d.AI.Models {
		if !set[m] {
			set[m] = true
			out = append(out, fmt.Sprintf("%q", m))
		}
	}
	sortStrings(out)
	return out
}

func agentList(d *Doc) string {
	keys := make([]string, 0, len(d.AI.Agents))
	for k := range d.AI.Agents {
		keys = append(keys, k)
	}
	sortStrings(keys)
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", d.AI.Agents[k], strings.ToLower(k)))
	}
	return strings.Join(parts, ", ")
}
