// Package incident does the file work of the incident-response kind around the core rules: making a
// folder a case, keeping evidence files with their hashes, rendering the reports from the case
// record, and writing the briefs of the investigators. Docs: docs/design/incident-response.md.
package incident

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"lacarta/internal/core"
	"lacarta/skill"
)

// Init makes root a case folder: state, evidence/ and reports/, and the report templates to edit.
func Init(root, name, typ, severity, tlp string) (string, error) {
	root, _ = filepath.Abs(root)
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return "", core.Gate("%s is not a folder.", root)
	}
	if _, err := os.Stat(filepath.Join(root, core.StateDir, "state.json")); err == nil {
		return "", core.Gate("%s is already a lacarta project.", root)
	}
	dir := filepath.Join(root, core.StateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cfgPath := filepath.Join(dir, core.ConfigFile)
	raw := map[string]json.RawMessage{}
	if b, err := os.ReadFile(cfgPath); err == nil && json.Unmarshal(b, &raw) != nil {
		return "", core.Gate("%s is not valid JSON: fix it first.", cfgPath)
	}
	if n := ""; raw["name"] != nil && json.Unmarshal(raw["name"], &n) == nil && strings.TrimSpace(n) != "" {
		name = strings.TrimSpace(n) // the file already names the case
	} else {
		b, _ := json.Marshal(name)
		raw["name"] = b
		if err := core.WriteJSON(cfgPath, raw); err != nil {
			return "", err
		}
	}
	for _, d := range []string{"evidence", "reports", "templates"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return "", err
		}
	}
	// The built-in report templates, to edit; a missing file falls back to the built-in one.
	entries, _ := fs.ReadDir(skill.Incident, "incident/templates")
	for _, e := range entries {
		dst := filepath.Join(root, "templates", e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, _ := fs.ReadFile(skill.Incident, "incident/templates/"+e.Name())
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return "", err
		}
	}
	gi := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(gi); err != nil {
		// Evidence may hold secrets and personal data: it stays out of git unless the lead decides otherwise.
		_ = os.WriteFile(gi, []byte(".lacarta/state.json\n.lacarta/.lock\n.lacarta/*.tmp\nevidence/\n"), 0o644)
	}
	st := core.NewIncident(root, name, typ, severity, tlp)
	core.Migrate(st)
	core.Log(st, fmt.Sprintf("Case opened in %s: %s %s.", root, severity, typ), "stage")
	if err := core.WriteJSON(filepath.Join(dir, "state.json"), st); err != nil {
		return "", err
	}
	return core.RegisterFolder(root, name)
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// StoreEvidence puts an evidence file in the case's evidence/ folder (a copy, unless it is already
// there), makes it read-only and returns its path in the case, its sha256 and its size.
func StoreEvidence(root, id, src, text string) (string, string, int64, error) {
	dir := filepath.Join(root, "evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", 0, err
	}
	var dst string
	if src == "" {
		dst = filepath.Join(dir, id+".md")
		if err := os.WriteFile(dst, []byte(strings.TrimSpace(text)+"\n"), 0o444); err != nil {
			return "", "", 0, err
		}
	} else {
		src, _ = filepath.Abs(src)
		fi, err := os.Lstat(src)
		if err != nil {
			return "", "", 0, core.Gate("Can't read %s: %v", src, err)
		}
		if !fi.Mode().IsRegular() {
			return "", "", 0, core.Gate("%s is not a regular file (a link or a folder): register the file itself, or an archive of the folder.", src)
		}
		if rel, err := filepath.Rel(dir, src); err == nil && !strings.HasPrefix(rel, "..") {
			dst = src // already in evidence/: registered in place
		} else {
			dst = filepath.Join(dir, id+"-"+unsafeName.ReplaceAllString(filepath.Base(src), "_"))
			in, err := os.Open(src)
			if err != nil {
				return "", "", 0, err
			}
			defer in.Close()
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return "", "", 0, err
			}
			if _, err := io.Copy(out, in); err != nil {
				out.Close()
				return "", "", 0, err
			}
			if err := out.Close(); err != nil {
				return "", "", 0, err
			}
		}
	}
	_ = os.Chmod(dst, 0o444)
	sum, n, err := hashFile(dst)
	if err != nil {
		return "", "", 0, err
	}
	rel, _ := filepath.Rel(root, dst)
	return filepath.ToSlash(rel), sum, n, nil
}

// Verify compares every evidence file and every issued report with the hash lacarta recorded.
// It returns one line per problem.
func Verify(st *core.State) []string {
	out := []string{}
	for _, e := range st.Incident.Evidence {
		if e.File == "" {
			continue
		}
		sum, _, err := hashFile(filepath.Join(st.Root, e.File))
		switch {
		case err != nil:
			out = append(out, fmt.Sprintf("%s: %s can't be read (%v)", e.ID, e.File, err))
		case sum != e.SHA256:
			out = append(out, fmt.Sprintf("%s: %s changed since it was registered (sha256 %s, recorded %s)", e.ID, e.File, sum, e.SHA256))
		}
	}
	for _, r := range st.Incident.Reports {
		if r.Status != "issued" {
			continue
		}
		if sum, _, err := hashFile(filepath.Join(st.Root, r.File)); err != nil {
			out = append(out, fmt.Sprintf("%s: %s can't be read (%v)", r.ID, r.File, err))
		} else if sum != r.Hash {
			out = append(out, fmt.Sprintf("%s: %s changed since the lead issued it", r.ID, r.File))
		}
	}
	return out
}

// EditDraft replaces a pending report with the lead's own text (the console calls it).
func EditDraft(st *core.State, id, text string) error {
	if st.Incident == nil {
		return core.Gate("This is not an incident-response case.")
	}
	r := core.GetReport(st, id)
	if r == nil || r.Status != "in-review" {
		return core.Gate("No report %s waiting for you.", id)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return core.Gate("The report is empty.")
	}
	path := filepath.Join(st.Root, r.File)
	if err := os.WriteFile(path, []byte(text+"\n"), 0o644); err != nil {
		return err
	}
	r.Hash, r.Edited = core.FileHash(path), true
	core.Log(st, fmt.Sprintf("You edited the draft of %s.", r.ID), "incident")
	return nil
}

// ReadReport returns a report's text and whether it is still what lacarta wrote or the lead issued.
func ReadReport(st *core.State, r *core.IncidentReport) (string, bool) {
	b, err := os.ReadFile(filepath.Join(st.Root, r.File))
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(b)
	return string(b), hex.EncodeToString(sum[:]) == r.Hash
}
