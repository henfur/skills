package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"lacarta/internal/core"
)

// The test binary doubles as a hook process for the signal test.
func TestMain(m *testing.M) {
	switch os.Getenv("HOOKS_TEST_CHILD") {
	case "":
	case "listen":
		_ = Listen(&core.Store{Root: os.Getenv("HOOKS_TEST_ROOT"), Dir: os.Getenv("HOOKS_TEST_DIR")}, os.Stdout, 0)
		os.Exit(0)
	default:
		Run(os.Getenv("HOOKS_TEST_CHILD"), os.Stdin, os.Stdout)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func setup(t *testing.T, remote bool) (*core.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LACARTA_HOME", home)
	t.Setenv("LACARTA_NO_DAEMON", "1")
	t.Setenv("CLAUDE_ENV_FILE", "")
	if !remote {
		if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(`{"remote_control": false}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	PollEvery = 20 * time.Millisecond
	cwd := filepath.Join(home, "work")
	_ = os.MkdirAll(cwd, 0o755)
	_, s, err := core.RegisterSession("s-1", cwd, "", "startup")
	if err != nil {
		t.Fatal(err)
	}
	return s, cwd
}

func runHook(event, payload string) string {
	var out bytes.Buffer
	Run(event, strings.NewReader(payload), &out)
	return out.String()
}

// listenAsync runs `lacarta listen` and returns its output once it ends.
func listenAsync(s *core.Store) <-chan string {
	done := make(chan string, 1)
	go func() {
		var out bytes.Buffer
		_ = Listen(s, &out, 0)
		done <- out.String()
	}()
	return done
}

func waitListener(t *testing.T, s *core.Store, alive bool) {
	t.Helper()
	for range 200 {
		st, err := s.Read()
		if err != nil {
			t.Fatal(err)
		}
		if (st.Listening != nil && st.Listening.PID != 0) == alive {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("a listener never became %v", alive)
}

func edit(t *testing.T, s *core.Store, fn func(*core.State) error) {
	t.Helper()
	core.Actor = ""
	defer func() { core.Actor = core.Orchestrator }()
	if err := s.Edit(fn); err != nil {
		t.Fatal(err)
	}
}

func blockReason(t *testing.T, out string) string {
	t.Helper()
	var b struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(out), &b); err != nil || b.Decision != "block" {
		t.Fatalf("expected a block, got %q", out)
	}
	return b.Reason
}

func receive(t *testing.T, done <-chan string) string {
	t.Helper()
	select {
	case out := <-done:
		return out
	case <-time.After(5 * time.Second):
		t.Fatal("the listener did not return")
		return ""
	}
}

// ---------------------------------------------------------------- Stop hook

func TestStopDeliversQueuedMessageAtOnce(t *testing.T) {
	s, _ := setup(t, true)
	edit(t, s, func(st *core.State) error { _, err := core.AddMessage(st, "hi"); return err })
	if reason := blockReason(t, runHook("stop", `{"session_id":"s-1"}`)); !strings.Contains(reason, "hi") {
		t.Fatal(reason)
	}
}

func TestStopWithoutRemoteDoesNothing(t *testing.T) {
	_, _ = setup(t, false)
	if out := runHook("stop", `{"session_id":"s-1"}`); out != "" {
		t.Fatalf("expected nothing, got %q", out)
	}
}

// The Stop hook never holds the terminal: without a listener it asks for one, once.
func TestStopAsksForAListenerOnce(t *testing.T) {
	_, _ = setup(t, true)
	start := time.Now()
	reason := blockReason(t, runHook("stop", `{"session_id":"s-1"}`))
	if !strings.Contains(reason, "listen` as a background command") || time.Since(start) > time.Second {
		t.Fatalf("%q after %v", reason, time.Since(start))
	}
	if out := runHook("stop", `{"session_id":"s-1","stop_hook_active":true}`); out != "" {
		t.Fatalf("a second stop in a row must not block again: %q", out)
	}
}

func TestStopWithAListenerLetsTheTurnEnd(t *testing.T) {
	s, _ := setup(t, true)
	done := listenAsync(s)
	waitListener(t, s, true)
	if out := runHook("stop", `{"session_id":"s-1"}`); out != "" {
		t.Fatalf("with a listener running the stop must not block: %q", out)
	}
	st, _ := s.Read()
	if core.AgentStatus(st, core.MustAgent(st, core.Orchestrator)) != "listening" || !core.IsListening(st) {
		t.Fatal("the console should show the agent as listening once its turn ended")
	}
	edit(t, s, func(st *core.State) error { off := false; st.Remote = &off; return nil })
	receive(t, done)
}

// ------------------------------------------------------------------ listen

func TestListenWakesOnAMessage(t *testing.T) {
	s, _ := setup(t, true)
	done := listenAsync(s)
	waitListener(t, s, true)
	st, _ := s.Read()
	if core.IsListening(st) {
		t.Fatal("while the agent works, it is not shown as listening")
	}
	edit(t, s, func(st *core.State) error { _, err := core.AddMessage(st, "Write the plan"); return err })
	out := receive(t, done)
	if !strings.HasPrefix(out, core.ConsoleTag) || !strings.Contains(out, "Write the plan") ||
		!strings.Contains(out, "listen` again as a background command") {
		t.Fatal(out)
	}
	waitListener(t, s, false)
	st, _ = s.Read()
	if len(core.PendingMessages(st)) != 0 || core.MustAgent(st, core.Orchestrator).Status != "working" {
		t.Fatal("the message should be delivered and the agent working")
	}
}

func TestListenWakesOnAnswer(t *testing.T) {
	s, _ := setup(t, true)
	edit(t, s, func(st *core.State) error {
		_, err := core.AddQuestion(st, "question", "DB?", "", []string{"A", "B"}, nil, core.Context{}, true, nil)
		return err
	})
	done := listenAsync(s)
	waitListener(t, s, true)
	edit(t, s, func(st *core.State) error {
		_, err := core.AnswerQuestion(st, "Q-1", "B", "why not", "user")
		return err
	})
	if out := receive(t, done); !strings.Contains(out, `Q-1 (DB?) was answered: {"choice": "B", "comment": "why not"`) {
		t.Fatal(out)
	}
}

func TestListenWakesOnTalkAboutThis(t *testing.T) {
	s, _ := setup(t, true)
	edit(t, s, func(st *core.State) error {
		_, err := core.AddQuestion(st, "question", "DB?", "", []string{"A", "B"}, nil, core.Context{}, true, nil)
		return err
	})
	done := listenAsync(s)
	waitListener(t, s, true)
	edit(t, s, func(st *core.State) error { _, err := core.DiscussQuestion(st, "Q-1", "What does B cost?"); return err })
	if out := receive(t, done); !strings.Contains(out, "Talk about this") || !strings.Contains(out, "What does B cost?") {
		t.Fatal(out)
	}
}

func TestListenEndsWhenRemoteIsTurnedOff(t *testing.T) {
	s, _ := setup(t, true)
	done := listenAsync(s)
	waitListener(t, s, true)
	edit(t, s, func(st *core.State) error { off := false; st.Remote = &off; return nil })
	if out := receive(t, done); !strings.Contains(out, "turned remote control off") {
		t.Fatal(out)
	}
	waitListener(t, s, false)
	var out bytes.Buffer
	_ = Listen(s, &out, 0)
	if !strings.Contains(out.String(), "Remote control is off") {
		t.Fatalf("with remote control off, listen returns at once: %q", out.String())
	}
}

func TestListenNewerListenerTakesOver(t *testing.T) {
	s, _ := setup(t, true)
	first := listenAsync(s)
	waitListener(t, s, true)
	edit(t, s, func(st *core.State) error { st.Listening.PID = os.Getppid(); return nil }) // another live process
	if out := receive(t, first); !strings.Contains(out, "Another listener took over") {
		t.Fatal(out)
	}
	st, _ := s.Read()
	if st.Listening == nil || st.Listening.PID != os.Getppid() {
		t.Fatal("the old listener must not unregister the new one")
	}
}

func TestListenTimeout(t *testing.T) {
	s, _ := setup(t, true)
	var out bytes.Buffer
	if err := Listen(s, &out, 100*time.Millisecond); err != nil || !strings.Contains(out.String(), "Nothing from the console yet") {
		t.Fatalf("%v %q", err, out.String())
	}
}

// A listener killed by the CLI (session end, /clear) unregisters on SIGTERM.
func TestListenSIGTERMUnregisters(t *testing.T) {
	s, _ := setup(t, true)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "HOOKS_TEST_CHILD=listen", "HOOKS_TEST_DIR="+s.Dir, "HOOKS_TEST_ROOT="+s.Root)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitListener(t, s, true)
	_ = cmd.Process.Signal(syscall.SIGTERM)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("listener exited with %v", err)
	}
	waitListener(t, s, false)
}

// Killed outright (no chance to clean up), the dead pid still stops it showing as listening.
func TestKilledListenerIsNotListening(t *testing.T) {
	s, _ := setup(t, true)
	edit(t, s, func(st *core.State) error { st.Listening = &core.Listening{Since: core.Now(), PID: 999999}; return nil })
	st, _ := s.Read()
	if core.IsListening(st) {
		t.Fatal("a dead pid is not listening")
	}
}

func TestSessionStartWritesEnvFile(t *testing.T) {
	_, cwd := setup(t, false)
	env := filepath.Join(t.TempDir(), "env")
	t.Setenv("CLAUDE_ENV_FILE", env)
	var out bytes.Buffer
	Run("session-start", strings.NewReader(`{"session_id":"s-9","cwd":"`+cwd+`"}`), &out)
	b, _ := os.ReadFile(env)
	if !strings.Contains(string(b), "export LACARTA_PROJECT=t-") || !strings.Contains(string(b), "export LACARTA_SESSION=s-9") {
		t.Fatalf("env file: %q", b)
	}
	if !strings.Contains(out.String(), "--project t-") {
		t.Fatalf("the context must give the --project fallback: %s", out.String())
	}
}

// --------------------------------------------------------------- transcripts

// Entry types seen in real Claude Code transcripts (2026-11): only user and assistant text and
// tool calls reach the console.
const transcript = `{"type":"user","message":{"role":"user","content":"Fix the login bug please"},"timestamp":"2026-11-12T01:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"..."},{"type":"text","text":"Looking at it."},{"type":"tool_use","name":"Bash","input":{"command":"grep -r login","description":"Search login code"}}]},"timestamp":"2026-11-12T01:00:05Z"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"app.py:3"}]},"timestamp":"2026-11-12T01:00:06Z"}
{"type":"user","isMeta":true,"message":{"role":"user","content":"Caveat: meta"},"timestamp":"2026-11-12T01:00:07Z"}
{"type":"user","message":{"role":"user","content":"<command-name>/resume</command-name>"},"timestamp":"2026-11-12T01:00:08Z"}
{"type":"assistant","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"subagent talk"}]},"timestamp":"2026-11-12T01:00:09Z"}
{"type":"attachment","attachment":{"type":"hook_success","hookEvent":"SessionStart"},"timestamp":"2026-11-12T01:00:10Z"}
{"type":"system","subtype":"stop_hook_summary","hookCount":1,"timestamp":"2026-11-12T01:00:11Z"}
{"type":"api-request-blob","message":{"content":[{"type":"text","text":"Stop hook blocking error from command: [lacarta console] hi"}]}}
{"type":"user","message":{"role":"user","content":"[lacarta console] The user wrote from the lacarta console."},"timestamp":"2026-11-12T01:00:12Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/x/app.py"}}]},"timestamp":"2026-11-12T01:00:13Z"}
not json
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Fixed."}]},"timestamp":"2026-11-12T01:00:14Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"half a li`

func TestReadChat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, "projects", "x", "s.jsonl")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ReadChat(path, 400)
	want := []ChatItem{
		{Role: "user", Text: "Fix the login bug please", At: "2026-11-12T01:00:00Z"},
		{Role: "agent", Text: "Looking at it.", At: "2026-11-12T01:00:05Z"},
		{Role: "tool", Name: "Bash", Text: "Search login code", At: "2026-11-12T01:00:05Z"},
		{Role: "tool", Name: "Read", Text: "/x/app.py", At: "2026-11-12T01:00:13Z"},
		{Role: "agent", Text: "Fixed.", At: "2026-11-12T01:00:14Z"},
	}
	if gb, wb := mustJSON(got), mustJSON(want); gb != wb {
		t.Fatalf("got  %s\nwant %s", gb, wb)
	}
	// The unfinished line is read once it is complete; earlier lines are not read twice.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString(`ne"}]},"timestamp":"2026-11-12T01:00:15Z"}` + "\n")
	_ = f.Close()
	got = ReadChat(path, 400)
	if len(got) != 6 || got[5].Text != "half a line" {
		t.Fatalf("after append: %s", mustJSON(got))
	}
}

func TestTranscriptOutsideClaudeIsRefused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "claude"))
	_ = os.MkdirAll(filepath.Join(dir, "claude"), 0o755)
	outside := filepath.Join(dir, "secret.jsonl")
	_ = os.WriteFile(outside, []byte(transcript), 0o644)
	link := filepath.Join(dir, "claude", "link.jsonl")
	_ = os.Symlink(outside, link)
	notJSONL := filepath.Join(dir, "claude", "x.txt")
	_ = os.WriteFile(notJSONL, []byte("x"), 0o644)
	for _, p := range []string{outside, link, notJSONL, filepath.Join(dir, "claude", "..", "secret.jsonl"), ""} {
		if TranscriptAllowed(p) || len(ReadChat(p, 10)) != 0 {
			t.Errorf("%q must not be served", p)
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ------------------------------------------------------------------ install

func TestInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-send done"}]},` +
		`{"hooks":[{"type":"command","command":"python3 /old/lacarta.py hook stop","timeout":86400}]}]}}`
	_ = os.WriteFile(path, []byte(original), 0o644)
	Self = "/opt/lacarta"
	var out bytes.Buffer
	for range 2 { // idempotent
		if err := Install(path, false, &out); err != nil {
			t.Fatal(err)
		}
	}
	var cfg struct {
		Model string
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct {
				Command string
				Timeout int
			}
		}
	}
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "opus" || len(cfg.Hooks) != 6 {
		t.Fatalf("settings: %s", b)
	}
	stop := cfg.Hooks["Stop"]
	if len(stop) != 2 || stop[0].Hooks[0].Command != "notify-send done" || stop[1].Hooks[0].Command != "/opt/lacarta hook stop" ||
		stop[1].Hooks[0].Timeout != 30 {
		t.Fatalf("the prototype's hook must be replaced and others kept: %s", b)
	}
	if cfg.Hooks["PostToolUse"][0].Matcher != "*" {
		t.Fatal("PostToolUse needs the * matcher")
	}
	if err := Install(path, true, &out); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "lacarta") || !strings.Contains(string(b), "notify-send done") {
		t.Fatalf("after uninstall: %s", b)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatal("a backup must be kept")
	}
}
