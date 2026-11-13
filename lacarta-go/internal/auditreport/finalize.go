package auditreport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lacarta/internal/core"
	"lacarta/internal/provenance"
)

func sortStrings(xs []string) { sort.Strings(xs) }

// Draft renders the current draft and its hash; approval names the hash the auditor read.
func Draft(pid string, st *core.State, auditor string) (string, string, error) {
	d, err := Build(pid, st, auditor, "", nextVersion(st))
	if err != nil {
		return "", "", err
	}
	md := Markdown(d)
	return md, provenance.BytesSHA256([]byte(md)), nil
}

func nextVersion(st *core.State) int {
	if st.Audit.Report == nil {
		return 1
	}
	return len(st.Audit.Report.Versions) + 1
}

// Manifest is the signed provenance record of a final report.
type Manifest struct {
	ReportID         string            `json:"report_id"`
	AIAssisted       bool              `json:"ai_assisted"`
	Disclosure       string            `json:"disclosure"`
	DisclosureSHA256 string            `json:"disclosure_sha256"`
	Files            map[string]string `json:"files"`
	Auditor          string            `json:"auditor"`
	ApprovedAt       string            `json:"approved_at"`
	Generator        string            `json:"generator"`
	Run              map[string]any    `json:"run"`
	AIWork           AIWork            `json:"ai_work"`
	Evidence         map[string]string `json:"evidence"`
	PublicKey        string            `json:"public_key"`
	KeyFingerprint   string            `json:"key_fingerprint"`
}

// Approve writes the final report of the current run. The draft must still hash to what the
// auditor read; the version folder is never overwritten.
func Approve(pid string, st *core.State, auditor, readHash string) (*core.ReportVersion, error) {
	if st.Template != core.Audit {
		return nil, core.Gate("This is not a security audit.")
	}
	if st.Stage != "report" {
		return nil, core.Gate("The report is approved once every finding is reviewed (stage now: %s).", st.Stage)
	}
	auditor = strings.TrimSpace(auditor)
	if auditor == "" {
		return nil, core.Gate("Set your name as the auditor in Settings first: the report is signed off by a person.")
	}
	if strings.TrimSpace(st.Audit.Summary) == "" {
		return nil, core.Gate("Write the executive summary first.")
	}
	_, hash, err := Draft(pid, st, auditor)
	if err != nil {
		return nil, err
	}
	if readHash != hash {
		return nil, core.Gate("The draft changed since you opened it. Read it again, then approve.")
	}
	version := nextVersion(st)
	approvedAt := core.Now()
	d, err := Build(pid, st, auditor, approvedAt, version)
	if err != nil {
		return nil, err
	}
	key, err := provenance.LoadOrCreate(filepath.Join(core.Home(), "keys"))
	if err != nil {
		return nil, err
	}
	d.KeyFingerprint = provenance.Fingerprint(key.Public)
	run := core.CurrentRun(st)
	dir := filepath.Join(run.Dir, "final", fmt.Sprintf("v%d", version))
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("%s already exists", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	md := Markdown(d)
	files := map[string]string{"report.md": md, "report.html": HTML(d, md)}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return nil, err
		}
	}
	out := map[string]string{"md": filepath.Join(dir, "report.md"), "html": filepath.Join(dir, "report.html")}
	if pdf, err := PrintPDF(filepath.Join(dir, "report.html"), filepath.Join(dir, "report.pdf")); err == nil && pdf {
		out["pdf"] = filepath.Join(dir, "report.pdf")
	}
	m := &Manifest{ReportID: d.ID, AIAssisted: true, Disclosure: d.Disclosure,
		DisclosureSHA256: provenance.BytesSHA256([]byte(d.Disclosure)), Files: map[string]string{},
		Auditor: auditor, ApprovedAt: approvedAt, Generator: "lacarta",
		Run: map[string]any{"id": run.ID, "source_ref": run.SourceRef, "dirty": run.Dirty, "profile": run.Profile,
			"target": st.Audit.Target, "skill": d.Skill, "plugins": d.Plugins},
		AIWork: d.AI, Evidence: map[string]string{},
		PublicKey: provenance.AuthorizedKey(key.Public), KeyFingerprint: provenance.Fingerprint(key.Public)}
	for _, name := range []string{"report.md", "report.html", "report.pdf"} {
		if h, err := provenance.FileSHA256(filepath.Join(dir, name)); err == nil {
			m.Files[name] = h
		}
	}
	for _, name := range []string{"findings.json", "coverage-ledger.json", "architecture.md", "run-metadata.json"} {
		if h, err := provenance.FileSHA256(core.RunFile(run, name)); err == nil {
			m.Evidence[name] = h
		}
	}
	if a := core.MustAgent(st, core.Orchestrator); a.Session != nil && a.Session.Transcript != "" {
		if h, err := provenance.FileSHA256(a.Session.Transcript); err == nil {
			m.Evidence["orchestrator_transcript"] = h
		}
	}
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	mb = append(mb, '\n')
	write := map[string]string{
		"report.provenance.json":     string(mb),
		"report.provenance.json.sig": key.Sign(mb),
		"allowed_signers":            provenance.AllowedSigners(auditor, key.Public) + "\n",
	}
	for name, body := range write {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return nil, err
		}
	}
	out["provenance"] = filepath.Join(dir, "report.provenance.json")
	v := &core.ReportVersion{Version: version, Run: run.ID, Dir: dir, Auditor: auditor, ApprovedAt: approvedAt,
		Hash: m.Files["report.md"], Files: out}
	if st.Audit.Report == nil {
		st.Audit.Report = &core.AuditReport{}
	}
	st.Audit.Report.Versions = append(st.Audit.Report.Versions, v)
	now := core.Now()
	run.EndedAt = &now
	st.Stage = "done"
	core.Log(st, fmt.Sprintf("Report v%d approved and signed by %s.", version, auditor), "stage")
	return v, nil
}

// Chromium finds a headless browser to print the PDF with.
func Chromium() string {
	if p := os.Getenv("LACARTA_CHROMIUM"); p != "" {
		return p
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome-stable", "google-chrome", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// PrintPDF prints the HTML report to PDF with headless Chromium; false when none is installed.
func PrintPDF(htmlPath, pdfPath string) (bool, error) {
	bin := Chromium()
	if bin == "" {
		return false, nil
	}
	profile, err := os.MkdirTemp("", "lacarta-chromium-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(profile)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--headless", "--disable-gpu", "--no-pdf-header-footer",
		"--user-data-dir="+profile, "--print-to-pdf="+pdfPath, "file://"+htmlPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("chromium: %v %s", err, out)
	}
	_, err = os.Stat(pdfPath)
	return err == nil, err
}

// Verify checks a final report folder: the signature over the manifest, then every file hash.
func Verify(dir string) (*Manifest, []string, error) {
	mb, err := os.ReadFile(filepath.Join(dir, "report.provenance.json"))
	if err != nil {
		return nil, nil, core.Gate("No report.provenance.json in %s: this report can't be verified.", dir)
	}
	sig, err := os.ReadFile(filepath.Join(dir, "report.provenance.json.sig"))
	if err != nil {
		return nil, nil, core.Gate("No signature next to the manifest: this report can't be verified.")
	}
	pub, err := provenance.Verify(mb, string(sig))
	if err != nil {
		return nil, nil, core.Gate("The manifest's signature is invalid: %s.", err)
	}
	var m Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, nil, err
	}
	notes := []string{fmt.Sprintf("Signature valid, key %s.", provenance.Fingerprint(pub))}
	if m.PublicKey != provenance.AuthorizedKey(pub) {
		return &m, notes, core.Gate("The manifest names a different key than the one that signed it.")
	}
	if !m.AIAssisted || m.DisclosureSHA256 != provenance.BytesSHA256([]byte(m.Disclosure)) {
		return &m, notes, core.Gate("The manifest's AI-assistance disclosure is missing or altered.")
	}
	names := make([]string, 0, len(m.Files))
	for n := range m.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h, err := provenance.FileSHA256(filepath.Join(dir, n))
		if err != nil {
			return &m, notes, core.Gate("%s is missing.", n)
		}
		if h != m.Files[n] {
			return &m, notes, core.Gate("%s was changed after it was signed.", n)
		}
		notes = append(notes, n+": unchanged.")
	}
	if md, err := os.ReadFile(filepath.Join(dir, "report.md")); err == nil && !strings.Contains(string(md), m.Disclosure) {
		return &m, notes, core.Gate("report.md doesn't carry the AI-assistance disclosure.")
	}
	notes = append(notes, fmt.Sprintf("AI-assisted audit, approved by %s on %s.", m.Auditor, m.ApprovedAt))
	return &m, notes, nil
}
