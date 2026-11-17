// Package forgetest is a fake `glab` for tests: a GitLab kept in one JSON file. Tests link a file
// named glab to their own binary and call Main from TestMain when it runs under that name. It
// checks what lacarta must get right: the host, the identity (GLAB_CONFIG_DIR, never a token
// variable), read-only identities refused on writes, and the working folder outside any repository.
package forgetest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const StateVar = "LACARTA_FAKE_GLAB"

type Identity struct {
	User   string   `json:"user"`
	Scopes []string `json:"scopes"`
}

type Note struct {
	ID         int    `json:"id"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	Discussion string `json:"discussion,omitempty"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Resolved   bool   `json:"resolved,omitempty"`
}

type MR struct {
	ProjectID    int              `json:"project_id"`
	Project      string           `json:"project"`
	IID          int              `json:"iid"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	State        string           `json:"state"`
	Draft        bool             `json:"draft"`
	Author       string           `json:"author"`
	SHA          string           `json:"sha"`
	BaseSHA      string           `json:"base_sha"`
	SourceBranch string           `json:"source_branch"`
	TargetBranch string           `json:"target_branch"`
	Labels       []string         `json:"labels"`
	Pipeline     string           `json:"pipeline"`
	UpdatedAt    string           `json:"updated_at"`
	Changes      []map[string]any `json:"changes"`
	Notes        []*Note          `json:"notes"`
	ApprovedBy   []string         `json:"approved_by"`
}

type Call struct {
	Identity string            `json:"identity"`
	Method   string            `json:"method"`
	Endpoint string            `json:"endpoint"`
	Fields   map[string]string `json:"fields,omitempty"`
	Input    json.RawMessage   `json:"input,omitempty"`
}

type State struct {
	Host       string               `json:"host"`
	Identities map[string]*Identity `json:"identities"` // by the base name of GLAB_CONFIG_DIR
	MRs        []*MR                `json:"mrs"`
	Calls      []*Call              `json:"calls"`
	NextID     int                  `json:"next_id"`
}

func Load(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	return &s, json.Unmarshal(b, &s)
}

func Save(path string, s *State) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

func (s *State) Find(project string, iid int) *MR {
	for _, m := range s.MRs {
		if m.Project == project && m.IID == iid {
			return m
		}
	}
	return nil
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

// Main acts as `glab` and exits.
func Main() {
	path := os.Getenv(StateVar)
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		fail("fake glab: %v", err)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	st, err := Load(path)
	if err != nil {
		fail("fake glab: %v", err)
	}
	out := run(st, os.Args[1:])
	if err := Save(path, st); err != nil {
		fail("fake glab: %v", err)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	b, _ := json.Marshal(out)
	os.Stdout.Write(b)
	os.Exit(0)
}

func run(st *State, args []string) any {
	for _, v := range []string{"GITLAB_TOKEN", "GITLAB_ACCESS_TOKEN", "OAUTH_TOKEN", "GITLAB_HOST"} {
		if os.Getenv(v) != "" {
			fail("fake glab: %s reached glab", v)
		}
	}
	if wd, _ := os.Getwd(); hasGlabConfig(wd) {
		fail("fake glab: run where a repository's .git/glab-cli/config.yml applies (%s)", wd)
	}
	dir := os.Getenv("GLAB_CONFIG_DIR")
	id := st.Identities[filepath.Base(dir)]
	if dir == "" || id == nil {
		fail("401 Unauthorized: no identity for GLAB_CONFIG_DIR=%q", dir)
	}
	if len(args) == 0 || args[0] != "api" {
		fail("fake glab: only `api` (got %v)", args)
	}
	c := &Call{Identity: id.User, Fields: map[string]string{}}
	host := ""
	for i := 1; i < len(args); i++ {
		a := args[i]
		next := func() string { i++; return args[i] }
		switch a {
		case "--hostname":
			host = next()
		case "--method", "-X":
			c.Method = next()
		case "--paginate":
		case "--header", "-H":
			next()
		case "--input":
			b, err := os.ReadFile(next())
			if err != nil {
				fail("fake glab: %v", err)
			}
			c.Input = b
		case "--raw-field", "-f", "--field", "-F":
			k, v, _ := strings.Cut(next(), "=")
			if a == "--field" || a == "-F" {
				if strings.HasPrefix(v, "@") {
					b, err := os.ReadFile(v[1:])
					if err != nil {
						fail("fake glab: %v", err)
					}
					v = string(b)
				}
			}
			c.Fields[k] = v
		default:
			c.Endpoint = a
		}
	}
	if host != st.Host {
		fail("fake glab: host %q, expected %q", host, st.Host)
	}
	if c.Method == "" {
		c.Method = "GET"
		if len(c.Fields) > 0 || c.Input != nil {
			c.Method = "POST"
		}
	}
	if c.Method != "GET" {
		write := false
		for _, s := range id.Scopes {
			write = write || s == "api"
		}
		if !write {
			fail("403 Forbidden: insufficient_scope (%s can't %s %s)", id.User, c.Method, c.Endpoint)
		}
		st.Calls = append(st.Calls, c)
	}
	return route(st, id, c)
}

// hasGlabConfig: glab would read a repository-local config here, which overrides GLAB_CONFIG_DIR.
func hasGlabConfig(dir string) bool {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git", "glab-cli", "config.yml")); err == nil {
			return true
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

func apiMR(m *MR) map[string]any {
	return map[string]any{"id": m.ProjectID*1000 + m.IID, "iid": m.IID, "project_id": m.ProjectID, "title": m.Title,
		"description": m.Description, "state": m.State, "draft": m.Draft, "sha": m.SHA, "source_branch": m.SourceBranch,
		"target_branch": m.TargetBranch, "labels": m.Labels, "web_url": "https://example.invalid/" + m.Project + "/-/merge_requests/" + strconv.Itoa(m.IID),
		"updated_at": m.UpdatedAt, "author": map[string]any{"username": m.Author},
		"references":    map[string]any{"full": m.Project + "!" + strconv.Itoa(m.IID)},
		"diff_refs":     map[string]any{"base_sha": m.BaseSHA, "start_sha": m.BaseSHA, "head_sha": m.SHA},
		"head_pipeline": map[string]any{"status": m.Pipeline}}
}

func route(st *State, id *Identity, c *Call) any {
	ep, query, _ := strings.Cut(c.Endpoint, "?")
	q, _ := url.ParseQuery(query)
	parts := strings.Split(ep, "/")
	switch {
	case ep == "user":
		return map[string]any{"username": id.User}
	case ep == "personal_access_tokens/self":
		return map[string]any{"scopes": id.Scopes}
	case len(parts) == 3 && parts[2] == "merge_requests" && (parts[0] == "groups" || parts[0] == "projects"):
		scope, _ := url.QueryUnescape(parts[1])
		out := []any{}
		for _, m := range st.MRs {
			in := m.Project == scope || (parts[0] == "groups" && strings.HasPrefix(m.Project, scope+"/"))
			if in && m.State == q.Get("state") && (q.Get("updated_after") == "" || m.UpdatedAt > q.Get("updated_after")) {
				out = append(out, apiMR(m))
			}
		}
		return out
	case len(parts) >= 4 && parts[0] == "projects" && parts[2] == "merge_requests":
		pid, _ := strconv.Atoi(parts[1])
		iid, _ := strconv.Atoi(parts[3])
		var m *MR
		for _, x := range st.MRs {
			if x.ProjectID == pid && x.IID == iid {
				m = x
			}
		}
		if m == nil {
			fail("404 Not Found")
		}
		rest := strings.Join(parts[4:], "/")
		switch {
		case rest == "" && c.Method == "GET":
			return apiMR(m)
		case rest == "" && c.Method == "PUT":
			for _, l := range splitComma(c.Fields["add_labels"]) {
				m.Labels = append(m.Labels, l)
			}
			keep := []string{}
			for _, l := range m.Labels {
				if !contains(splitComma(c.Fields["remove_labels"]), l) {
					keep = append(keep, l)
				}
			}
			m.Labels = keep
			return apiMR(m)
		case rest == "diffs":
			return m.Changes
		case rest == "notes" && c.Method == "POST":
			st.NextID++
			n := &Note{ID: st.NextID, Author: id.User, Body: c.Fields["body"]}
			m.Notes = append(m.Notes, n)
			return n
		case rest == "discussions" && c.Method == "POST":
			var in struct {
				Body     string `json:"body"`
				Position struct {
					NewPath string `json:"new_path"`
					NewLine int    `json:"new_line"`
					HeadSHA string `json:"head_sha"`
				} `json:"position"`
			}
			_ = json.Unmarshal(c.Input, &in)
			if in.Position.HeadSHA != m.SHA {
				fail("400 Bad Request: position head_sha is not the head")
			}
			if in.Position.NewLine > 1000 {
				fail("400 Bad Request: line_code can't be blank")
			}
			st.NextID++
			n := &Note{ID: st.NextID, Author: id.User, Body: in.Body, Discussion: fmt.Sprintf("d%d", st.NextID),
				File: in.Position.NewPath, Line: in.Position.NewLine}
			m.Notes = append(m.Notes, n)
			return map[string]any{"id": n.Discussion}
		case strings.HasPrefix(rest, "discussions/") && c.Method == "PUT":
			for _, n := range m.Notes {
				if n.Discussion == strings.TrimPrefix(rest, "discussions/") {
					n.Resolved = c.Fields["resolved"] == "true"
					return map[string]any{"id": n.Discussion}
				}
			}
			fail("404 Not Found")
		case rest == "approve":
			if c.Fields["sha"] != m.SHA {
				fail("409 Conflict: SHA does not match HEAD of source branch")
			}
			m.ApprovedBy = append(m.ApprovedBy, id.User)
			return map[string]any{"ok": true}
		case rest == "unapprove":
			keep := []string{}
			for _, u := range m.ApprovedBy {
				if u != id.User {
					keep = append(keep, u)
				}
			}
			m.ApprovedBy = keep
			return map[string]any{"ok": true}
		}
	}
	fail("404 Not Found: %s %s", c.Method, c.Endpoint)
	return nil
}

func splitComma(s string) []string {
	out := []string{}
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
