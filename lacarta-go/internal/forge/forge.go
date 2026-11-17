// Package forge talks to a git forge for review desks. GitLab only for now, through `glab api`.
//
// Lacarta never holds a token: each identity is a glab config folder (filled by `glab auth login`
// run with GLAB_CONFIG_DIR), and every call sets GLAB_CONFIG_DIR to that folder explicitly, removes
// the token variables glab would otherwise prefer, and runs from a neutral folder so no repository's
// .git/glab-cli/config.yml can override it. There is no method to merge, close, push or run a
// pipeline (SR-25). Docs: docs/design/pr-review.md §3.3, §5.3.
package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"lacarta/internal/core"
)

// MR is a merge request as the desk needs it.
type MR struct {
	ProjectID    int      `json:"project_id"`
	Project      string   `json:"project"`
	IID          int      `json:"iid"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Author       string   `json:"author"`
	URL          string   `json:"url"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Labels       []string `json:"labels"`
	Draft        bool     `json:"draft"`
	State        string   `json:"state"`
	HeadSHA      string   `json:"head_sha"`
	BaseSHA      string   `json:"base_sha"`
	StartSHA     string   `json:"start_sha"`
	Pipeline     string   `json:"pipeline"`
	UpdatedAt    string   `json:"updated_at"`
}

type Change struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Diff    string `json:"diff"`
	New     bool   `json:"new_file"`
	Deleted bool   `json:"deleted_file"`
	Renamed bool   `json:"renamed_file"`
}

// Forge: what a desk reads and writes. Nothing to merge, close, push or run.
type Forge interface {
	User() (string, error)
	Scopes() ([]string, error)
	ListOpen(t *core.DeskTarget, since string) ([]*MR, error)
	Get(projectID, iid int) (*MR, error)
	Changes(projectID, iid int) ([]Change, error)
	PostNote(projectID, iid int, body string) (int, error)
	PostInline(mr *MR, file string, line int, body string) (string, error)
	Resolve(projectID, iid int, discussion string) error
	Approve(projectID, iid int, sha string) error
	Unapprove(projectID, iid int) error
	SetLabels(projectID, iid int, add, remove []string) error
}

// GitLab runs `glab api` with one identity.
type GitLab struct {
	Glab      string // the glab program
	ConfigDir string // GLAB_CONFIG_DIR: the identity
	Host      string
	WorkDir   string // a folder outside every repository
	Timeout   time.Duration
}

// tokenVars: environment variables glab reads before its config; they never reach it.
var tokenVars = []string{"GITLAB_TOKEN", "GITLAB_ACCESS_TOKEN", "OAUTH_TOKEN", "GLAB_CONFIG_DIR", "GITLAB_HOST",
	"GL_HOST", "GITLAB_URI", "GITLAB_API_HOST", "CI_JOB_TOKEN"}

// New builds a GitLab forge for one identity folder, already checked by core.CheckCredentialDir.
func New(cfg *core.DeskConfig, configDir, workDir string) (*GitLab, error) {
	if configDir == "" {
		return nil, core.Gate("The desk has no forge identity for this: set desk.forge.write_config and read_config " +
			"in .lacarta/config.json (glab config folders, see `desk guide`).")
	}
	if cfg.Forge.Host == "" {
		return nil, core.Gate("Set desk.forge.host in .lacarta/config.json (the GitLab host name).")
	}
	glab := core.ToolPath("glab")
	if glab == "" {
		return nil, core.Gate("glab is not installed (or set tools.glab in ~/.lacarta/config.json).")
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, err
	}
	return &GitLab{Glab: glab, ConfigDir: configDir, Host: cfg.Forge.Host, WorkDir: workDir, Timeout: 2 * time.Minute}, nil
}

// Env is the environment of every glab call: the caller's, minus token variables, plus the identity.
func Env(configDir string) []string {
	out := []string{}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, t := range tokenVars {
			drop = drop || name == t
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return append(out, "GLAB_CONFIG_DIR="+configDir, "NO_COLOR=1", "GLAB_NO_PROMPT=1", "GLAB_CHECK_UPDATE=false")
}

// APIError is a non-2xx answer (or a failed call).
type APIError struct {
	Endpoint string
	Text     string
}

func (e *APIError) Error() string { return fmt.Sprintf("glab api %s: %s", e.Endpoint, e.Text) }

func (g *GitLab) api(method, endpoint string, fields map[string]string, input any, paginate bool) ([]byte, error) {
	args := []string{"api", "--hostname", g.Host}
	if method != "" {
		args = append(args, "--method", method)
	}
	if paginate {
		args = append(args, "--paginate")
	}
	var tmp string
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		f, err := os.CreateTemp(g.WorkDir, "body-*.json")
		if err != nil {
			return nil, err
		}
		tmp = f.Name()
		defer os.Remove(tmp)
		_, err = f.Write(b)
		f.Close()
		if err != nil {
			return nil, err
		}
		args = append(args, "--input", tmp, "--header", "Content-Type: application/json")
	}
	args = append(args, endpoint)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		v := fields[k]
		if strings.HasPrefix(v, "@") { // a literal value that starts with @ would be read as a file
			f, err := os.CreateTemp(g.WorkDir, "field-*")
			if err != nil {
				return nil, err
			}
			_, _ = f.WriteString(v)
			f.Close()
			defer os.Remove(f.Name())
			v = "@" + f.Name()
		}
		args = append(args, "--raw-field", k+"="+v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), g.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.Glab, args...)
	cmd.Dir, cmd.Env = g.WorkDir, Env(g.ConfigDir)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(errb.String())
		if text == "" {
			text = strings.TrimSpace(out.String())
		}
		if text == "" {
			text = err.Error()
		}
		if len(text) > 500 {
			text = text[:500] + "…"
		}
		return nil, &APIError{Endpoint: endpoint, Text: text}
	}
	return out.Bytes(), nil
}

// bodyField posts a long text through a file, so it never sits on a command line.
func (g *GitLab) bodyFile(body string) (string, func(), error) {
	f, err := os.CreateTemp(g.WorkDir, "note-*.md")
	if err != nil {
		return "", nil, err
	}
	_, err = f.WriteString(body)
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, err
}

func enc(path string) string { return url.QueryEscape(path) }

func (g *GitLab) User() (string, error) {
	b, err := g.api("", "user", nil, nil, false)
	if err != nil {
		return "", err
	}
	var u struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(b, &u); err != nil || u.Username == "" {
		return "", fmt.Errorf("glab api user: unexpected answer")
	}
	return u.Username, nil
}

func (g *GitLab) Scopes() ([]string, error) {
	b, err := g.api("", "personal_access_tokens/self", nil, nil, false)
	if err != nil {
		return nil, err
	}
	var t struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("personal_access_tokens/self: unexpected answer")
	}
	return t.Scopes, nil
}

type apiMR struct {
	ID           int      `json:"id"`
	IID          int      `json:"iid"`
	ProjectID    int      `json:"project_id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	State        string   `json:"state"`
	Draft        bool     `json:"draft"`
	WIP          bool     `json:"work_in_progress"`
	SHA          string   `json:"sha"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Labels       []string `json:"labels"`
	WebURL       string   `json:"web_url"`
	UpdatedAt    string   `json:"updated_at"`
	Author       struct {
		Username string `json:"username"`
	} `json:"author"`
	References struct {
		Full string `json:"full"`
	} `json:"references"`
	DiffRefs *struct {
		BaseSHA  string `json:"base_sha"`
		HeadSHA  string `json:"head_sha"`
		StartSHA string `json:"start_sha"`
	} `json:"diff_refs"`
	HeadPipeline *struct {
		Status string `json:"status"`
	} `json:"head_pipeline"`
}

func (a *apiMR) mr() *MR {
	m := &MR{ProjectID: a.ProjectID, IID: a.IID, Title: a.Title, Description: a.Description, Author: a.Author.Username,
		URL: a.WebURL, SourceBranch: a.SourceBranch, TargetBranch: a.TargetBranch, Labels: a.Labels,
		Draft: a.Draft || a.WIP, State: a.State, HeadSHA: a.SHA, UpdatedAt: a.UpdatedAt}
	if m.Labels == nil {
		m.Labels = []string{}
	}
	if full := a.References.Full; full != "" {
		m.Project, _, _ = strings.Cut(full, "!")
	}
	if a.DiffRefs != nil {
		m.BaseSHA, m.StartSHA = a.DiffRefs.BaseSHA, a.DiffRefs.StartSHA
		if a.DiffRefs.HeadSHA != "" {
			m.HeadSHA = a.DiffRefs.HeadSHA
		}
	}
	if a.HeadPipeline != nil {
		m.Pipeline = a.HeadPipeline.Status
	}
	return m
}

func (g *GitLab) ListOpen(t *core.DeskTarget, since string) ([]*MR, error) {
	q := url.Values{"state": {"opened"}, "scope": {"all"}, "per_page": {"100"}, "order_by": {"updated_at"}}
	if since != "" {
		q.Set("updated_after", since)
	}
	var ep string
	if t.Group != "" {
		ep = "groups/" + enc(t.Group) + "/merge_requests?" + q.Encode()
	} else {
		ep = "projects/" + enc(t.Project) + "/merge_requests?" + q.Encode()
	}
	b, err := g.api("", ep, nil, nil, true)
	if err != nil {
		return nil, err
	}
	var list []apiMR
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("%s: unexpected answer: %v", ep, err)
	}
	out := []*MR{}
	for i := range list {
		m := list[i].mr()
		if t.Group != "" && !t.IncludeSubgroups && filepath.Dir(m.Project) != t.Group {
			continue
		}
		if !Matches(m.Project, t.Exclude) && (len(t.TargetBranches) == 0 || Matches(m.TargetBranch, t.TargetBranches)) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (g *GitLab) Get(projectID, iid int) (*MR, error) {
	b, err := g.api("", fmt.Sprintf("projects/%d/merge_requests/%d", projectID, iid), nil, nil, false)
	if err != nil {
		return nil, err
	}
	var a apiMR
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("merge request %d!%d: unexpected answer", projectID, iid)
	}
	return a.mr(), nil
}

func (g *GitLab) Changes(projectID, iid int) ([]Change, error) {
	b, err := g.api("", fmt.Sprintf("projects/%d/merge_requests/%d/diffs?per_page=100", projectID, iid), nil, nil, true)
	if err == nil {
		var cs []Change
		if json.Unmarshal(b, &cs) == nil {
			return cs, nil
		}
	}
	// Older GitLab: the changes endpoint.
	b, err2 := g.api("", fmt.Sprintf("projects/%d/merge_requests/%d/changes", projectID, iid), nil, nil, false)
	if err2 != nil {
		return nil, errors.Join(err, err2)
	}
	var c struct {
		Changes []Change `json:"changes"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("changes of %d!%d: unexpected answer", projectID, iid)
	}
	return c.Changes, nil
}

func (g *GitLab) PostNote(projectID, iid int, body string) (int, error) {
	f, done, err := g.bodyFile(body)
	if err != nil {
		return 0, err
	}
	defer done()
	b, err := g.apiFieldFile("POST", fmt.Sprintf("projects/%d/merge_requests/%d/notes", projectID, iid), "body", f)
	if err != nil {
		return 0, err
	}
	var n struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(b, &n)
	return n.ID, nil
}

// apiFieldFile sends one field read from a file (glab's --field key=@file).
func (g *GitLab) apiFieldFile(method, endpoint, key, file string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.Glab, "api", "--hostname", g.Host, "--method", method, endpoint, "--field", key+"=@"+file)
	cmd.Dir, cmd.Env = g.WorkDir, Env(g.ConfigDir)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(errb.String() + " " + out.String())
		if len(text) > 500 {
			text = text[:500] + "…"
		}
		return nil, &APIError{Endpoint: endpoint, Text: text}
	}
	return out.Bytes(), nil
}

func (g *GitLab) PostInline(mr *MR, file string, line int, body string) (string, error) {
	in := map[string]any{"body": body, "position": map[string]any{
		"position_type": "text", "base_sha": mr.BaseSHA, "start_sha": mr.StartSHA, "head_sha": mr.HeadSHA,
		"new_path": file, "old_path": file, "new_line": line}}
	b, err := g.api("POST", fmt.Sprintf("projects/%d/merge_requests/%d/discussions", mr.ProjectID, mr.IID), nil, in, false)
	if err != nil {
		return "", err
	}
	var d struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(b, &d) != nil || d.ID == "" {
		return "", fmt.Errorf("discussion: unexpected answer")
	}
	return d.ID, nil
}

func (g *GitLab) Resolve(projectID, iid int, discussion string) error {
	_, err := g.api("PUT", fmt.Sprintf("projects/%d/merge_requests/%d/discussions/%s", projectID, iid, url.PathEscape(discussion)),
		map[string]string{"resolved": "true"}, nil, false)
	return err
}

func (g *GitLab) Approve(projectID, iid int, sha string) error {
	_, err := g.api("POST", fmt.Sprintf("projects/%d/merge_requests/%d/approve", projectID, iid),
		map[string]string{"sha": sha}, nil, false)
	return err
}

func (g *GitLab) Unapprove(projectID, iid int) error {
	_, err := g.api("POST", fmt.Sprintf("projects/%d/merge_requests/%d/unapprove", projectID, iid), nil, nil, false)
	return err
}

func (g *GitLab) SetLabels(projectID, iid int, add, remove []string) error {
	f := map[string]string{}
	if len(add) > 0 {
		f["add_labels"] = strings.Join(add, ",")
	}
	if len(remove) > 0 {
		f["remove_labels"] = strings.Join(remove, ",")
	}
	if len(f) == 0 {
		return nil
	}
	_, err := g.api("PUT", fmt.Sprintf("projects/%d/merge_requests/%d", projectID, iid), f, nil, false)
	return err
}

// Matches: a path matches one of the globs (`**` crosses folders, `*` doesn't).
func Matches(p string, globs []string) bool {
	for _, g := range globs {
		if Glob(g, p) {
			return true
		}
	}
	return false
}

// Glob matches a slash-separated path against a pattern with `*`, `?` and `**`.
func Glob(pattern, p string) bool {
	return globParts(strings.Split(pattern, "/"), strings.Split(p, "/"))
}

func globParts(pat, parts []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(parts); i++ {
				if globParts(pat[1:], parts[i:]) {
					return true
				}
			}
			return false
		}
		if len(parts) == 0 {
			return false
		}
		if ok, _ := filepath.Match(pat[0], parts[0]); !ok {
			return false
		}
		pat, parts = pat[1:], parts[1:]
	}
	return len(parts) == 0
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}
