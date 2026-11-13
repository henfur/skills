package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"lacarta/internal/auditreport"
	"lacarta/internal/core"
	"lacarta/internal/plugins"
)

// The run files an audit's agents would write, built with the skill's own helpers so the real
// validators accept them.
const fixtureJS = `
const v = require(process.argv[1]);
const fs = require("fs"), path = require("path");
const out = process.argv[2];
const refs = { surface: "src/api.js#POST /orders/:id", boundary: "src/authz.js#requireOwner",
  subsystem: "api", attack_class: "ATTACK-CLASSES.md#Access control" };
const unit = (status, agent, fps) => ({ coverage_id: v.canonicalCoverageId(refs), canonical_refs: refs,
  surface: "Update order", boundary: "Order ownership", subsystem: "api", attack_class: "Access control",
  starting_paths: ["src/api.js"], ordinary_attack_class_block: "ATTACK-CLASSES.md#Access control",
  selected_companion_blocks: [], excluded_blocks: [], prior_status: "none", attempts: [], wave: 1, status,
  agent_id: agent, reviewed_paths: agent ? ["src/api.js"] : [], local_checks: agent ? [{ agent_id: agent,
  reviewed_paths: ["src/api.js"], invariant: "Only the owner updates an order.", method: "source",
  result: "The handler updates without the owner check.", artifact: null }] : [], result_fingerprints: fps, unresolved: [] });
fs.writeFileSync(path.join(out, process.argv[3]), JSON.stringify([process.argv[3] === "plan.json"
  ? unit("planned", null, []) : unit("candidate", "hunter-1", ["api-order-owner"])], null, 2));
const src = (kind, line) => ({ kind, file: "src/api.js", line, scope: "updateOrder", description: "The order id reaches the update." });
const sorted = xs => xs.sort((a, b) => a.fingerprint < b.fingerprint ? -1 : 1);
fs.writeFileSync(path.join(out, "findings.json"), JSON.stringify(sorted([
  { verdict: "confirmed", fingerprint: "api-order-owner", title: "Any user can update any order",
    description: "The update route changes an order without checking that it belongs to the caller.",
    root_cause: "updateOrder skips requireOwner.", intended_behavior: "Only the order's owner can change it.",
    trace: [src("entrypoint", 3), src("sink", 9)], evidence: [{ file: "src/api.js", line: 9, description: "No owner check." }],
    conditions: [], execution: { attacker_perspective: "A user with their own account.", payloads: ["Another user's order id."],
      instructions: ["Call the update route with that id."], observed_result: "The other user's order changes." },
    remediation: { strategy: "Call requireOwner before the update." },
    severity: { likelihood: { score: "high", reason: "Any user can reach it." }, impact: { score: "high", reason: "Orders of every user." },
      overall_severity: "high" }, confidence: { score: "high", reason: "Reproduced in a fixture." } },
  { verdict: "needs_validation", fingerprint: "api-size", title: "Unbounded body size", description: "Bodies may be unbounded.",
    claimed_root_cause: "No limit in the parser setup.", trace: [src("entrypoint", 1)], evidence: [{ file: "src/api.js", line: 1, description: "Parser setup." }],
    blockers: ["The proxy's limit isn't in the repository."], validation_plan: { local: "Send a large body to a local server.",
      deployment: "Check the proxy configuration." } },
  { verdict: "rejected", fingerprint: "api-auth", title: "Login bypass", description: "Claimed bypass.",
    claimed_root_cause: "Claimed missing check.", trace: [src("sink", 2)], evidence: [{ file: "src/api.js", line: 2, description: "Check present." }],
    reason: "The middleware checks every route." }]), null, 2));
`

func lac(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(args, &out, &errb)
	return out.String() + errb.String(), code
}

func must(t *testing.T, args ...string) string {
	t.Helper()
	out, code := lac(t, args...)
	if code != 0 {
		t.Fatalf("lacarta %s: exit %d\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

// TestAuditEndToEnd drives an audit from scope to a signed report, with the real skill's
// validators in the real sandbox.
func TestAuditEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available")
	}
	t.Setenv("LACARTA_HOME", t.TempDir())
	t.Setenv("LACARTA_NO_DAEMON", "1")
	t.Setenv("LACARTA_PROJECT", "")
	t.Setenv("LACARTA_SESSION", "")
	t.Setenv("LACARTA_AGENT", "")
	if cs, err := plugins.Preflight(); err != nil || plugins.PreflightFailed(cs) != "" {
		t.Skipf("this machine can't run audits: %v %s", err, plugins.PreflightFailed(cs))
	}
	skill, _ := plugins.SkillPath()
	target := t.TempDir()
	os.MkdirAll(filepath.Join(target, "src"), 0o755)
	os.WriteFile(filepath.Join(target, "src", "api.js"), []byte("// api\n"), 0o644)

	out := must(t, "audit", "init", "--target", target, "--name", "Shop")
	pid := regexp.MustCompile(`audit-shop-[0-9a-f]{4}`).FindString(out)
	if pid == "" {
		t.Fatalf("no project id in:\n%s", out)
	}
	p := func(args ...string) string { return must(t, append([]string{"--project", pid}, args...)...) }
	if _, code := lac(t, "audit", "init", "--target", target); code != 2 {
		t.Fatal("a second audit of the same target was accepted")
	}
	p("audit", "scope", "--profile", "quick", "--budget", "20")
	p("answer", "Q-1", "--choice", core.Approve)
	runDir := filepath.Join(core.AuditHome(pid), "run-1")
	if !strings.Contains(p("status"), "stage recon") {
		t.Fatal("not in recon after the scope approval")
	}
	os.MkdirAll(runDir, 0o755)
	node := func(name string) {
		cmd := exec.Command("node", "-e", fixtureJS, filepath.Join(skill, "validate-coverage-ledger.cjs"), runDir, name)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixtures: %v\n%s", err, b)
		}
	}
	node("plan.json")
	os.Rename(filepath.Join(runDir, "plan.json"), filepath.Join(runDir, "coverage-ledger.json"))
	if _, code := lac(t, "--project", pid, "audit", "plan", "submit"); code != 2 {
		t.Fatal("plan submitted without architecture.md and run-metadata.json")
	}
	os.WriteFile(filepath.Join(runDir, "architecture.md"), []byte("# Architecture\n"), 0o644)
	os.WriteFile(filepath.Join(runDir, "run-metadata.json"), []byte(`{"run_status":"in_progress"}`), 0o644)
	os.WriteFile(filepath.Join(runDir, "coverage-ledger.json"), []byte(`[{"coverage_id": 1}]`), 0o644)
	if out, code := lac(t, "--project", pid, "audit", "plan", "submit"); code != 2 || !strings.Contains(out, "validator") {
		t.Fatalf("an invalid ledger passed: %d\n%s", code, out)
	}
	node("plan.json")
	os.Rename(filepath.Join(runDir, "plan.json"), filepath.Join(runDir, "coverage-ledger.json"))
	p("audit", "plan", "submit")
	p("answer", "Q-2", "--choice", core.Approve)
	if _, code := lac(t, "--project", pid, "audit", "findings", "submit"); code != 2 {
		t.Fatal("findings submitted with a planned unit")
	}
	node("coverage-ledger.json")
	p("audit", "findings", "submit")
	brief := p("audit", "brief", "hunter", "--unit", "x")
	if !strings.Contains(brief, "sandbox run") || !strings.Contains(brief, "security-audit") {
		t.Fatalf("brief:\n%s", brief)
	}

	s, _ := core.ProjectStore(pid)
	edit := func(fn func(*core.State) error) error { return s.Edit(fn) }
	if err := edit(func(st *core.State) error {
		return core.ReviewFinding(st, s, "api-order-owner", "severity", "medium", "")
	}); !core.IsGate(err) {
		t.Fatalf("a severity change without a reason: %v", err)
	}
	if err := edit(func(st *core.State) error {
		return core.ReviewFinding(st, s, "api-order-owner", "severity", "medium", "Orders hold no payment data.")
	}); err != nil {
		t.Fatal(err)
	}
	if err := edit(func(st *core.State) error { return core.ReviewFinding(st, s, "api-size", "keep", "", "") }); err != nil {
		t.Fatal(err)
	}
	p("audit", "summary", "--text", "One medium finding: order ownership.")
	st, _ := s.Read()
	if st.Stage != "report" {
		t.Fatalf("stage %s after every review", st.Stage)
	}
	_, hash, err := auditreport.Draft(pid, st, "Ada Auditor")
	if err != nil {
		t.Fatal(err)
	}
	if err := edit(func(st *core.State) error { _, err := auditreport.Approve(pid, st, "", hash); return err }); !core.IsGate(err) {
		t.Fatalf("approved without an auditor name: %v", err)
	}
	if err := edit(func(st *core.State) error { _, err := auditreport.Approve(pid, st, "Ada Auditor", "old"); return err }); !core.IsGate(err) {
		t.Fatalf("approved a draft the auditor didn't read: %v", err)
	}
	if err := edit(func(st *core.State) error { _, err := auditreport.Approve(pid, st, "Ada Auditor", hash); return err }); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(runDir, "final", "v1")
	md, _ := os.ReadFile(filepath.Join(final, "report.md"))
	if dump := os.Getenv("AUDIT_REPORT_DUMP"); dump != "" { // to look at a sample report
		os.CopyFS(dump, os.DirFS(final))
	}
	for _, want := range []string{"performed with AI assistance", "Reviewed and approved by Ada Auditor",
		"Severity changed from High to Medium", "Orders hold no payment data.", "### 4.1 Medium: Any user can update any order",
		"Unbounded body size", "AI-ASSISTED"} {
		if !strings.Contains(string(md), want) && !strings.Contains(readFile(final, "report.html"), want) {
			t.Errorf("the report lacks %q", want)
		}
	}
	if out := must(t, "audit", "verify", final); !strings.Contains(out, "Verified") {
		t.Fatalf("verify:\n%s", out)
	}
	os.WriteFile(filepath.Join(final, "report.md"), []byte(strings.Replace(string(md), "performed with AI assistance", "performed", 1)), 0o644)
	if out, code := lac(t, "audit", "verify", final); code != 2 || !strings.Contains(out, "changed") {
		t.Fatalf("a tampered report verified: %d\n%s", code, out)
	}
	if st, _ := s.Read(); st.Stage != "done" {
		t.Fatalf("stage %s after approval", st.Stage)
	}
}

func readFile(dir, name string) string {
	b, _ := os.ReadFile(filepath.Join(dir, name))
	return string(b)
}
