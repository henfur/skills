package plugins

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"lacarta/internal/core"
	"lacarta/internal/sandbox"
)

// SkillPath is where the project's security-audit skill lives.
func SkillPath() (string, error) {
	i, err := Get(SecurityAudit)
	if err != nil {
		return "", err
	}
	if !i.Installed {
		return "", core.Gate("A security audit needs Cloudflare's security-audit skill. %s Install it with: %s", i.Problem, i.InstallHint)
	}
	return i.Path, nil
}

// Validators of the skill, by the file they check.
var Validators = map[string]string{
	"coverage-ledger.json": "validate-coverage-ledger.cjs",
	"findings.json":        "validate-findings.cjs",
}

// Validate runs the skill's validator on a run file, in the sandbox. It returns the
// validator's output and whether the file passed.
func Validate(skill, file string) (string, bool, error) {
	v := Validators[filepath.Base(file)]
	if v == "" {
		return "", false, fmt.Errorf("no validator for %s", file)
	}
	if _, err := os.Stat(file); err != nil {
		return "", false, core.Gate("%s doesn't exist yet.", file)
	}
	scratch, err := os.MkdirTemp("", "lacarta-validate-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(scratch)
	out, code, err := runSandboxed(&sandbox.Spec{Scratch: scratch, ReadOnly: []string{skill, filepath.Dir(file)},
		Timeout: 2 * time.Minute, Args: []string{"node", filepath.Join(skill, v), file}})
	if err != nil {
		return out, false, err
	}
	return out, code == 0, nil
}

func runSandboxed(s *sandbox.Spec) (string, int, error) {
	cmd, cancel, err := sandbox.Command(context.Background(), s)
	if err != nil {
		return "", -1, err
	}
	defer cancel()
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err = cmd.Run()
	out := strings.TrimSpace(buf.String())
	if len(out) > 8000 {
		out = out[:8000] + "\n…"
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return out, ee.ExitCode(), nil
	}
	if err != nil {
		return out, -1, err
	}
	return out, 0, nil
}

// SelfTest runs the validators' own tests once per skill version.
func SelfTest(skill, ref string) error {
	r, err := Load()
	if err != nil {
		return err
	}
	if r.SelfTests[ref] == "ok" {
		return nil
	}
	scratch, err := os.MkdirTemp("", "lacarta-selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	out, code, err := runSandboxed(&sandbox.Spec{Scratch: scratch, ReadOnly: []string{skill}, Dir: skill,
		Timeout: 5 * time.Minute, Args: []string{"node", "--test", "validate-findings.test.cjs", "validate-coverage-ledger.test.cjs"}})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the skill's validator tests fail:\n%s", out)
	}
	return Edit(func(r *Registry) error { r.SelfTests[ref] = "ok"; return nil })
}

// Check is one preflight item.
type Check struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Detail   string `json:"detail"`
}

// Preflight checks what a security audit needs before it starts. The audit can't start while
// a required check fails; the others change how it runs (e.g. source-only without a sandbox).
func Preflight() ([]Check, error) {
	out := []Check{}
	i, err := Get(SecurityAudit)
	if err != nil {
		return nil, err
	}
	skill := Check{Name: "security-audit skill", Required: true, OK: i.Installed && i.Problem == ""}
	if skill.OK {
		skill.Detail = fmt.Sprintf("%s (content %s)", i.Path, i.Ref)
		if i.Tested != nil && !*i.Tested {
			skill.Detail += "; differs from the tested version " + Builtins[0].Commit[:7] + ": allowed, check its changes"
		}
	} else {
		skill.Detail = strings.TrimSpace(i.Problem + " Install: " + i.InstallHint)
	}
	out = append(out, skill)
	node := Check{Name: "node", Required: true}
	if p, err := exec.LookPath("node"); err == nil {
		node.OK, node.Detail = true, p
	} else {
		node.Detail = "The skill's validators need Node.js."
	}
	out = append(out, node)
	sb := Check{Name: "sandbox"}
	if err := sandbox.Available(); err != nil {
		sb.Detail = "No sandbox (" + err.Error() + "): the audit runs source-only, and leads that need running the " +
			"target stay `needs_validation`. Validators can't run either, so this blocks the audit."
		sb.Required = true
	} else {
		sb.OK, sb.Detail = true, "bubblewrap: no network, read-only target, writes only to scratch"
	}
	out = append(out, sb)
	if skill.OK && node.OK && sb.OK {
		st := Check{Name: "validator self-test", Required: true}
		if err := SelfTest(i.Path, i.Ref); err != nil {
			st.Detail = err.Error()
		} else {
			st.OK, st.Detail = true, "the skill's validator tests pass"
		}
		out = append(out, st)
	}
	for _, d := range Builtins {
		if d.Type != TypeCheck {
			continue
		}
		c := Check{Name: d.ID}
		if p, err := exec.LookPath(d.Program); err == nil {
			c.OK, c.Detail = true, p
		} else {
			c.Detail = "Not installed; the audit runs without it. " + d.InstallHint
		}
		out = append(out, c)
	}
	return out, nil
}

// PreflightFailed returns the first required check that failed, or "".
func PreflightFailed(cs []Check) string {
	for _, c := range cs {
		if c.Required && !c.OK {
			return c.Name + ": " + c.Detail
		}
	}
	return ""
}

// ------------------------------------------------------------ check plugins

// RunCheck runs a check plugin over the target and writes its results (JSON) to out. The tool
// runs in the sandbox with the target read-only. Results are leads, never findings.
func RunCheck(id, target, out string) (string, error) {
	d, ok := DefByID(id)
	if !ok || d.Type != TypeCheck {
		return "", core.Gate("%s is not a check plugin.", id)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	scratch, err := os.MkdirTemp("", "lacarta-check-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(scratch)
	report := filepath.Join(scratch, "report.json")
	spec := &sandbox.Spec{Scratch: scratch, ReadOnly: []string{target}, Timeout: 20 * time.Minute}
	switch id {
	case "gitleaks":
		// --redact: the secret value never reaches the report or the agents.
		spec.Args = []string{"gitleaks", "detect", "--source", target, "--redact", "--no-banner",
			"--report-format", "json", "--report-path", report, "--exit-code", "0"}
	case "osv-scanner":
		// The database is fetched outside the sandbox (it needs the network) from copies of the
		// lockfiles only, then the scan runs offline inside.
		db := filepath.Join(core.Home(), "cache", "osv")
		if err := os.MkdirAll(db, 0o755); err != nil {
			return "", err
		}
		locks, err := copyLockfiles(target, filepath.Join(scratch, "locks"))
		if err != nil {
			return "", err
		}
		if locks == 0 {
			return "No lockfiles found: nothing to scan.", core.WriteJSON(out, map[string]any{"results": []any{}})
		}
		dl := exec.Command("osv-scanner", "scan", "source", "--offline-vulnerabilities", "--download-offline-databases",
			"--format", "json", "-r", filepath.Join(scratch, "locks"))
		dl.Env = append(os.Environ(), "OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY="+db)
		_, _ = dl.CombinedOutput() // its findings are recomputed below; only the download matters
		spec.ReadOnly = append(spec.ReadOnly, db)
		spec.Env = map[string]string{"OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY": db}
		spec.Args = []string{"osv-scanner", "scan", "source", "--offline-vulnerabilities", "--format", "json",
			"--output", report, "-r", target}
	}
	log, code, err := runSandboxed(spec)
	if err != nil {
		return log, err
	}
	b, rerr := os.ReadFile(report)
	if rerr != nil {
		return log, fmt.Errorf("%s produced no report (exit %d): %s", id, code, log)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return log, err
	}
	return fmt.Sprintf("%s: report written to %s (exit %d). %s", id, out, code, sandbox.Describe(spec)), nil
}

var lockfiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "bun.lock": true, "go.sum": true, "go.mod": true,
	"Cargo.lock": true, "poetry.lock": true, "Pipfile.lock": true, "requirements.txt": true, "uv.lock": true,
	"Gemfile.lock": true, "composer.lock": true, "pom.xml": true, "gradle.lockfile": true, "packages.lock.json": true,
	"mix.lock": true, "pubspec.lock": true,
}

func copyLockfiles(root, dest string) (int, error) {
	n := 0
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() && (fi.Name() == ".git" || fi.Name() == "node_modules" || fi.Name() == "vendor") {
			return filepath.SkipDir
		}
		if !fi.Mode().IsRegular() || !lockfiles[fi.Name()] || fi.Size() > 50<<20 {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		to := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		n++
		return os.WriteFile(to, b, 0o644)
	})
	return n, err
}
