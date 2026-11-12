// Package hooks connects agent CLIs to lacarta: `lacarta hook <event>` with the CLI's JSON on
// stdin. A hook never fails the session: errors go to ~/.lacarta/hooks.log and it exits 0.
package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"lacarta/internal/core"
)

// PollEvery is how often `lacarta listen` looks for news.
var PollEvery = time.Second

// Self is the command that runs this binary, for the context given to agents.
var Self = "lacarta"

// EnsureDaemon starts the console if it is not running; set by the server package.
var EnsureDaemon = func() *core.DaemonInfo { return core.DaemonRead() }

// Payload is what Claude Code sends; unknown fields are ignored.
type Payload struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
	Source         string `json:"source"`
	Prompt         string `json:"prompt"`
	Message        string `json:"message"`
	Reason         string `json:"reason"`
	StopHookActive bool   `json:"stop_hook_active"` // this stop follows a block by a Stop hook
}

var Events = map[string]func(*Payload, io.Writer) error{
	"session-start": sessionStart, "prompt": prompt, "post-tool": postTool,
	"notification": notification, "stop": stop, "session-end": sessionEnd,
}

// Run handles one hook call; it always succeeds from the CLI's point of view.
func Run(event string, stdin io.Reader, stdout io.Writer) {
	core.Actor = core.Orchestrator
	p := &Payload{}
	if b, err := io.ReadAll(stdin); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		_ = json.Unmarshal(b, p)
	}
	defer func() {
		if r := recover(); r != nil {
			logFailure(event, fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
		}
	}()
	fn, ok := Events[event]
	if !ok {
		logFailure(event, "unknown event")
		return
	}
	if err := fn(p, stdout); err != nil {
		logFailure(event, err.Error())
	}
}

func logFailure(event, text string) {
	if os.MkdirAll(core.Home(), 0o755) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(core.Home(), "hooks.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n%s\n\n", core.Now(), event, text)
}

// The CLI reads one JSON object; the prototype printed it with json.dumps.
type contextOut struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

type blockOut struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func printContext(w io.Writer, event, context string) {
	if context == "" {
		return
	}
	var o contextOut
	o.HookSpecificOutput.HookEventName, o.HookSpecificOutput.AdditionalContext = event, context
	fmt.Fprintln(w, core.PyDumps(o))
}

func printBlock(w io.Writer, reason string) {
	if reason != "" {
		fmt.Fprintln(w, core.PyDumps(blockOut{"block", reason}))
	}
}

func cwdOf(p *Payload) string {
	if p.Cwd != "" {
		return p.Cwd
	}
	wd, _ := os.Getwd()
	return wd
}

func register(p *Payload) (string, *core.Store, error) {
	return core.RegisterSession(p.SessionID, cwdOf(p), p.TranscriptPath, p.Source)
}

// sessionStore finds the session's project; hooks installed mid-session register on first contact.
func sessionStore(p *Payload) (*core.Store, error) {
	h, err := core.HubRead()
	if err != nil {
		return nil, err
	}
	if pid, ok := h.Sessions.Get(p.SessionID); ok {
		if s, err := core.ProjectStore(pid); err == nil {
			return s, nil
		}
	}
	_, s, err := register(p)
	return s, err
}

func sessionContext(pid string, s *core.Store) (string, error) {
	st, err := s.Read()
	if err != nil {
		return "", err
	}
	url := "not running (start it with `serve`)"
	if info := core.DaemonRead(); info != nil {
		url = info.OpenURL
	}
	kind := strings.ToLower(core.KindByID(st.Template).Label)
	lines := []string{fmt.Sprintf("lacarta: this session is registered in the lacarta console (%s) as the %s project "+
		"\"%s\" (%s). The user can follow it there and write to you from it: those messages start with \"%s\" and are "+
		"the user's own words. If `$LACARTA_PROJECT` is empty in your shell, pass `--project %s` to lacarta commands.",
		url, kind, st.Project.Name, pid, core.ConsoleTag, pid)}
	if st.Template == core.Software {
		lines = append(lines, fmt.Sprintf("This folder is a lacarta software project: load the `lacarta` skill now and "+
			"follow it. Run `%s status` first.", Self))
	} else {
		lines = append(lines, fmt.Sprintf("If the user asks to build a new project, app or service, or wants specs or a "+
			"structured, reviewed process, load the `lacarta` skill. When you need a decision while the user may be away "+
			"from the terminal, ask in the console: `%s ask --title \"...\" --option \"Label|what it means\"`, then run "+
			"`%s wait` as a background command.", Self, Self))
	}
	if core.RemoteOn(st) {
		lines = append(lines, fmt.Sprintf("Remote control is on: at your first turn, start `%s listen` as a background "+
			"command and leave it running. It returns when the user writes from the console, answers a question or asks "+
			"about one; act on what it prints, then start it again. The user can keep typing in this terminal meanwhile.", Self))
	}
	if qs := core.OpenQuestions(st); len(qs) > 0 {
		ids := make([]string, len(qs))
		for i, q := range qs {
			ids[i] = q.ID
		}
		lines = append(lines, fmt.Sprintf("Questions still open in the console: %s.", strings.Join(ids, ", ")))
	}
	return strings.Join(lines, "\n"), nil
}

func sessionStart(p *Payload, w io.Writer) error {
	EnsureDaemon()
	pid, s, err := register(p)
	if err != nil {
		return err
	}
	if env := os.Getenv("CLAUDE_ENV_FILE"); env != "" {
		// Later Bash commands in this session resolve the project without a .lacarta/ folder.
		f, err := os.OpenFile(env, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		fmt.Fprintf(f, "export LACARTA_PROJECT=%s\nexport LACARTA_SESSION=%s\n", shellQuote(pid), shellQuote(p.SessionID))
		f.Close()
	}
	ctx, err := sessionContext(pid, s)
	if err != nil {
		return err
	}
	printContext(w, "SessionStart", ctx)
	return nil
}

func prompt(p *Payload, w io.Writer) error {
	s, err := sessionStore(p)
	if err != nil {
		return err
	}
	return s.Edit(func(st *core.State) error {
		st.Attention = nil
		a := core.MustAgent(st, core.Orchestrator)
		a.Status, a.UpdatedAt = "working", core.Now()
		text := strings.TrimSpace(p.Prompt)
		if st.Template == core.Single && !st.Project.Named && text != "" && !strings.HasPrefix(text, "/") {
			first := []rune(strings.SplitN(text, "\n", 2)[0])
			name := string(first)
			if len(first) > 70 {
				name = string(first[:70]) + "…"
			}
			st.Project.Name, st.Project.Named = name, true
		}
		return nil
	})
}

func postTool(p *Payload, w io.Writer) error {
	s, err := sessionStore(p)
	if err != nil {
		return err
	}
	st, err := s.Read()
	if err != nil {
		return err
	}
	if len(core.PendingMessages(st)) == 0 && st.Attention == nil {
		return nil // the common case: read only, nothing to do
	}
	var msg string
	if err := s.Edit(func(st *core.State) error {
		st.Attention = nil
		msg = core.TakeMessages(st)
		return nil
	}); err != nil {
		return err
	}
	printContext(w, "PostToolUse", msg)
	return nil
}

func notification(p *Payload, w io.Writer) error {
	s, err := sessionStore(p)
	if err != nil {
		return err
	}
	return s.Edit(func(st *core.State) error {
		text := p.Message
		if text == "" {
			text = "The agent needs you in the terminal."
		}
		st.Attention = &core.Attention{Text: text, At: core.Now()}
		core.Log(st, "Needs you in the terminal: "+text, "agent")
		return nil
	})
}

// stop ends a turn. It never waits: queued console messages are handed to the agent at once;
// otherwise, with remote control on, a background `lacarta listen` keeps watching the console
// while the terminal stays free. If none runs, the agent is asked once to start it.
func stop(p *Payload, w io.Writer) error {
	s, err := sessionStore(p)
	if err != nil {
		return err
	}
	var msg string
	var askListener bool
	err = s.Edit(func(st *core.State) error {
		st.Attention = nil
		a := core.MustAgent(st, core.Orchestrator)
		msg = core.TakeMessages(st)
		a.Status, a.UpdatedAt = "idle", core.Now()
		if msg != "" {
			a.Status = "working"
		}
		active := st.Stage != "ended" && st.Stage != "done"
		if core.ListenerAlive(st) {
			st.Listening.Since = core.Now() // the console announces each finished turn
		} else {
			st.Listening = nil
			askListener = msg == "" && active && core.RemoteOn(st) && !p.StopHookActive
		}
		return nil
	})
	if err != nil {
		return err
	}
	switch {
	case msg != "":
		printBlock(w, msg)
	case askListener:
		printBlock(w, fmt.Sprintf("%s Remote control is on for this project but no listener is running. Start `%s listen` "+
			"as a background command (it returns when the user writes from the console, answers a question or asks "+
			"about one), then end your turn. Do nothing else.", core.ConsoleTag, Self))
	}
	return nil
}

func sessionEnd(p *Payload, w io.Writer) error {
	s, err := sessionStore(p)
	if err != nil {
		return err
	}
	return s.Edit(func(st *core.State) error {
		a := core.MustAgent(st, core.Orchestrator)
		if a.Session != nil {
			now := core.Now()
			a.Session.EndedAt = &now
		}
		st.Listening, st.Attention = nil, nil
		if st.Template == core.Single {
			now := core.Now()
			st.Stage = "ended"
			a.Status, a.FinishedAt, a.UpdatedAt = "done", &now, now
			core.WithdrawOpenQuestions(st, "the session ended.")
		} else {
			a.Status, a.UpdatedAt = "idle", core.Now()
		}
		reason := p.Reason
		if reason == "" {
			reason = "exit"
		}
		core.Log(st, fmt.Sprintf("%s's session ended (%s).", a.Name, reason), "agent")
		return nil
	})
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./-_") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
