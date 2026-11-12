package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"lacarta/internal/core"
	"lacarta/internal/hooks"
)

const token = "test-token"

// fixture builds a home with a software project (session, question, finding) and a
// single-agent task with a transcript, through the same code the CLI and hooks use.
func fixture(t *testing.T) (home, taskID, softID string) {
	t.Helper()
	home = t.TempDir()
	claude := filepath.Join(home, "claude")
	t.Setenv("LACARTA_HOME", home)
	t.Setenv("LACARTA_NO_DAEMON", "1")
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CLAUDE_ENV_FILE", "")
	tr := filepath.Join(claude, "projects", "x", "s-1.jsonl")
	_ = os.MkdirAll(filepath.Dir(tr), 0o755)
	_ = os.WriteFile(tr, []byte(`{"type":"user","message":{"content":"Fix it — vite"},"timestamp":"2026-11-12T01:00:00Z"}
{"type":"assistant","message":{"content":[{"type":"text","text":"On it."},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-11-12T01:00:01Z"}
`), 0o644)
	app := filepath.Join(home, "app")
	_ = os.MkdirAll(app, 0o755)
	var out bytes.Buffer
	hooks.Run("session-start", strings.NewReader(fmt.Sprintf(`{"session_id":"s-1","cwd":%q,"transcript_path":%q}`, app, tr)), &out)
	hooks.Run("prompt", strings.NewReader(`{"session_id":"s-1","prompt":"Fix it — vite"}`), &out)
	h, _ := core.HubRead()
	taskID = h.Projects.Keys()[0]

	proj := filepath.Join(home, "proj")
	init := exec.Command(goBin(t), "--root", proj, "init", "--name", "Proj")
	init.Env = os.Environ()
	if b, err := init.CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, b)
	}
	hooks.Run("session-start", strings.NewReader(fmt.Sprintf(`{"session_id":"s-2","cwd":%q}`, proj)), &out)
	s := core.SoftwareStore(proj)
	core.Actor = core.Orchestrator
	if err := s.Edit(func(st *core.State) error {
		_, err := core.AddQuestion(st, "question", "Stack?", "Pick **one**.", []string{"Go", "Rust"}, nil,
			core.Context{Stage: st.Stage, Spec: "stack"}, true, map[string]string{"Go": "Simple"})
		st.Findings = append(st.Findings, &core.Finding{ID: "F-1", Phase: core.PhaseRef{Name: "spec"}, Review: "specification",
			Severity: "medium", Title: "Gap", Status: "open", At: core.Now()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h, _ = core.HubRead()
	for _, k := range h.Projects.Keys() {
		if p, _ := h.Projects.Get(k); p.Template == core.Software {
			softID = k
		}
	}
	return home, taskID, softID
}

var builtBin string

func goBin(t *testing.T) string {
	if builtBin != "" {
		return builtBin
	}
	_, file, _, _ := runtime.Caller(0)
	dir, err := os.MkdirTemp("", "lacarta-server-test") // shared by every test in the package
	if err != nil {
		t.Fatal(err)
	}
	builtBin = filepath.Join(dir, "lacarta")
	cmd := exec.Command("go", "build", "-o", builtBin, "./cmd/lacarta")
	cmd.Dir = filepath.Join(filepath.Dir(file), "..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, b)
	}
	return builtBin
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func getJSON(t *testing.T, url string) any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v\n%s", url, err, b)
	}
	return v
}

// TestAPIParity runs the prototype's daemon and the Go handler on the same state files and
// compares what the console receives.
func TestAPIParity(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	_, taskID, softID := fixture(t)
	_, file, _, _ := runtime.Caller(0)
	proto := filepath.Join(filepath.Dir(file), "..", "..", "..", "lacarta", "scripts", "lacarta.py")
	port := freePort(t)
	py := exec.Command("python3", proto, "serve", "--port", fmt.Sprint(port))
	py.Env = os.Environ()
	if err := py.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = py.Process.Kill(); _ = py.Wait() }()
	pyURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	for i := 0; ; i++ {
		if resp, err := http.Get(pyURL + "/api/hub"); err == nil {
			resp.Body.Close()
			break
		}
		if i > 100 {
			t.Fatal("the prototype's daemon did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	goSrv := httptest.NewServer(Handler(Config{Token: token, Port: 0}))
	defer goSrv.Close()

	for _, path := range []string{"/api/hub", "/api/p/" + taskID + "/state", "/api/p/" + softID + "/state"} {
		want, got := getJSON(t, pyURL+path), getJSON(t, goSrv.URL+path)
		if diffs := compare("", want, got); len(diffs) > 0 {
			t.Errorf("%s differs from the prototype:\n  %s", path, strings.Join(diffs, "\n  "))
		}
	}
}

// compare lists the paths where two decoded JSON values differ.
func compare(path string, a, b any) []string {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: %v vs %v", path, short(a), short(b))}
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		sorted := []string{}
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		out := []string{}
		for _, k := range sorted {
			x, okA := av[k]
			y, okB := bv[k]
			switch {
			case !okA:
				out = append(out, fmt.Sprintf("%s.%s: only in Go (%v)", path, k, short(y)))
			case !okB:
				out = append(out, fmt.Sprintf("%s.%s: missing in Go (%v)", path, k, short(x)))
			default:
				out = append(out, compare(path+"."+k, x, y)...)
			}
		}
		return out
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return []string{fmt.Sprintf("%s: %v vs %v", path, short(a), short(b))}
		}
		out := []string{}
		for i := range av {
			out = append(out, compare(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i])...)
		}
		return out
	}
	if !reflect.DeepEqual(a, b) {
		return []string{fmt.Sprintf("%s: %v vs %v", path, short(a), short(b))}
	}
	return nil
}

func short(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 120 {
		return string(b[:120]) + "…"
	}
	return string(b)
}

// ------------------------------------------------------------- actions, auth

func doPost(t *testing.T, srv *httptest.Server, path, body string, mutate func(*http.Request)) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(body))
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lacarta-Token", token)
	if mutate != nil {
		mutate(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return resp.StatusCode, v
}

func TestActions(t *testing.T) {
	_, taskID, softID := fixture(t)
	srv := httptest.NewServer(Handler(Config{Token: token}))
	defer srv.Close()
	p := "/api/p/" + softID

	if code, v := doPost(t, srv, p+"/discuss", `{"id":"Q-1","message":"Why Go?"}`, nil); code != 200 {
		t.Fatalf("discuss: %d %v", code, v)
	}
	if code, v := doPost(t, srv, p+"/answer", `{"id":"Q-1","choice":"Haskell"}`, nil); code != 409 ||
		!strings.Contains(v["error"].(string), "Choose one of ['Go', 'Rust']") {
		t.Fatalf("a gate refusal is a 409 with the gate's words: %d %v", code, v)
	}
	if code, _ := doPost(t, srv, p+"/answer", `{"id":"Q-1","choice":"Go","comment":"ok"}`, nil); code != 200 {
		t.Fatal("answer")
	}
	st, _ := core.ProjectStore(softID)
	state, _ := st.Read()
	if state.Questions[0].Answer.By != "user" || state.Decisions[0].By != "user" {
		t.Fatal("console answers are the user's")
	}
	if code, _ := doPost(t, srv, p+"/message", `{"message":"hello"}`, nil); code != 200 {
		t.Fatal("message")
	}
	if code, _ := doPost(t, srv, p+"/remote", `{"on":false}`, nil); code != 200 {
		t.Fatal("remote")
	}
	state, _ = st.Read()
	if len(core.PendingMessages(state)) != 1 || state.Remote == nil || *state.Remote {
		t.Fatal("message queued, remote off")
	}
	if code, _ := doPost(t, srv, "/api/p/"+taskID+"/archive", `{"archived":true}`, nil); code != 200 {
		t.Fatal("archive")
	}
	if h, _ := core.HubRead(); func() bool { x, _ := h.Projects.Get(taskID); return !x.Archived }() {
		t.Fatal("archived")
	}
	if code, v := doPost(t, srv, "/api/settings", `{"models":{"software":{"security_review":"gpt"}}}`, nil); code != 409 {
		t.Fatalf("bad model: %d %v", code, v)
	}
	if code, v := doPost(t, srv, "/api/settings", `{"models":{"software":{"security_review":"haiku"}},"remote_control":false}`, nil); code != 200 {
		t.Fatalf("settings: %d %v", code, v)
	}
	if s := core.SettingsRead(); s.RemoteControl {
		t.Fatal("remote default off")
	} else if m, _ := s.Models.Get("software"); func() string { v, _ := m.Get("security_review"); return v }() != "haiku" {
		t.Fatal("model saved")
	}
	if code, _ := doPost(t, srv, "/api/p/nope/answer", `{}`, nil); code != 404 {
		t.Fatal("unknown project")
	}
	if code, _ := doPost(t, srv, p+"/explode", `{}`, nil); code != 404 {
		t.Fatal("unknown action")
	}
}

func TestAuth(t *testing.T) {
	_, _, softID := fixture(t)
	srv := httptest.NewServer(Handler(Config{Token: token}))
	defer srv.Close()
	path := "/api/p/" + softID + "/message"
	cases := map[string]struct {
		mutate func(*http.Request)
		code   int
	}{
		"foreign origin": {func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, 403},
		"no origin":      {func(r *http.Request) { r.Header.Del("Origin") }, 403},
		"wrong token":    {func(r *http.Request) { r.Header.Set("X-Lacarta-Token", "nope") }, 403},
		"not JSON":       {func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
	}
	for name, c := range cases {
		if code, _ := doPost(t, srv, path, `{"message":"x"}`, c.mutate); code != c.code {
			t.Errorf("%s: got %d, want %d", name, code, c.code)
		}
	}
	if code, _ := doPost(t, srv, path, `{"message":"`+strings.Repeat("x", 70_000)+`"}`, nil); code != 413 {
		t.Errorf("oversized body: got %d", code)
	}

	// DNS rebinding: a foreign Host is refused when the host check is on.
	checked := httptest.NewServer(Handler(Config{Token: token, Port: 4747, CheckHost: true}))
	defer checked.Close()
	if resp, err := http.Get(checked.URL + "/api/hub"); err != nil || resp.StatusCode != 403 {
		t.Errorf("host check: %v %v", resp.StatusCode, err)
	}

	// Bound beyond loopback, reads need the key too.
	keyed := httptest.NewServer(Handler(Config{Token: token, RequireKey: true}))
	defer keyed.Close()
	for _, u := range []string{"/", "/api/hub"} {
		if resp, _ := http.Get(keyed.URL + u); resp.StatusCode != 403 {
			t.Errorf("%s without key: %d", u, resp.StatusCode)
		}
	}
	if resp, _ := http.Get(keyed.URL + "/?key=" + token); resp.StatusCode != 200 {
		t.Errorf("page with key: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", keyed.URL+"/api/hub", nil)
	req.Header.Set("X-Lacarta-Token", token)
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 200 {
		t.Errorf("api with key: %d", resp.StatusCode)
	}
	if resp, _ := http.Get(keyed.URL + "/vendor/marked.min.js"); resp.StatusCode != 200 {
		t.Errorf("vendored scripts need no key: %d", resp.StatusCode)
	}
	if resp, _ := http.Get(srv.URL + "/vendor/../../etc/passwd"); resp.StatusCode == 200 {
		t.Error("path traversal")
	}
}

func TestPageHasToken(t *testing.T) {
	_, _, _ = fixture(t)
	srv := httptest.NewServer(Handler(Config{Token: token}))
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/")
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `const TOKEN = "`+token+`"`) || strings.Contains(string(b), "__LACARTA_TOKEN__") {
		t.Fatal("the page must carry the token")
	}
	if resp.Header.Get("Content-Security-Policy") == "" || resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	if builtBin != "" {
		_ = os.RemoveAll(filepath.Dir(builtBin))
	}
	os.Exit(code)
}
