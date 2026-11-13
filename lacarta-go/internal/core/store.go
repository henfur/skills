package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// GateError is a framework rule refusing the requested transition.
type GateError struct{ Msg string }

func (e *GateError) Error() string { return e.Msg }

func Gate(format string, a ...any) error { return &GateError{fmt.Sprintf(format, a...)} }

func IsGate(err error) bool {
	var g *GateError
	return errors.As(err, &g)
}

// Actor is who runs this command: --agent, else $LACARTA_AGENT, else the orchestrator.
// The console server sets it to "": everything it records comes from the user.
var Actor = Orchestrator

// Clock is replaceable in tests.
var Clock = func() time.Time { return time.Now() }

func Now() string { return Clock().UTC().Format("2006-01-02T15:04:05Z") }

// FindRoot returns the nearest folder at or above start holding a software project.
func FindRoot(start string) string {
	d, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(d, StateDir, "state.json")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// Lock takes an advisory lock on path (created if missing) until release is called.
func Lock(path string, exclusive bool) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

// WriteJSON replaces path atomically.
func WriteJSON(path string, v any) error {
	b, err := MarshalIndent(v)
	if err != nil {
		return err
	}
	tmp := path[:len(path)-len(filepath.Ext(path))] + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Store is one project's state. Root is the folder the project is about; Dir holds its state
// (<root>/.lacarta for a software project, ~/.lacarta/tasks/<id> for a single-agent task).
type Store struct {
	Root, Dir string
}

func SoftwareStore(root string) *Store { return &Store{Root: root, Dir: filepath.Join(root, StateDir)} }

func (s *Store) Path() string { return filepath.Join(s.Dir, "state.json") }

func (s *Store) load() (*State, error) {
	b, err := os.ReadFile(s.Path())
	if err != nil {
		return nil, err
	}
	st := &State{}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, fmt.Errorf("%s: %w", s.Path(), err)
	}
	if err := Migrate(st); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Store) Read() (*State, error) {
	release, err := Lock(filepath.Join(s.Dir, ".lock"), false)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.load()
}

// Edit runs fn on the state; it is written back only if fn succeeds.
func (s *Store) Edit(fn func(*State) error) error {
	release, err := Lock(filepath.Join(s.Dir, ".lock"), true)
	if err != nil {
		return err
	}
	defer release()
	st, err := s.load()
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	if err := Refresh(st, s); err != nil {
		return err
	}
	return WriteJSON(s.Path(), st)
}

// Migrate brings a state file written by an older version up to date.
func Migrate(st *State) error {
	if st.Template == "" {
		st.Template = Software
	}
	// Models moved to the machine-wide settings; the per-project setup question is obsolete.
	for _, q := range st.Questions {
		if q.Status == "open" && q.Effect != nil && q.Effect.Type == "models-confirm" {
			q.Status = "superseded"
		}
	}
	if st.Counters == nil {
		st.Counters = map[string]int{}
	}
	if st.Inbox == nil {
		st.Inbox = []*InboxMsg{}
	}
	if st.Agents == nil {
		st.Agents = []*Agent{}
		NewAgent(st, "orchestrator", "Runs the framework and talks to you.", nil, false)
	}
	for _, q := range st.Questions {
		if q.Options == nil {
			q.Options = []string{}
		}
		if q.OptionHelp == nil {
			q.OptionHelp = map[string]string{}
		}
		if q.Thread == nil {
			q.Thread = []*Msg{}
		}
		if q.AskedBy == "" {
			q.AskedBy = Orchestrator
		}
	}
	nilSlices(st)
	if a := st.Audit; a != nil {
		if a.Runs == nil {
			a.Runs = []*AuditRun{}
		}
		if a.Reviews == nil {
			a.Reviews = map[string]*AuditReview{}
		}
		if a.Recheck == nil {
			a.Recheck = map[string]string{}
		}
	}
	return nil
}

func nilSlices(st *State) {
	if st.Phases == nil {
		st.Phases = []*Phase{}
	}
	if st.Reviews == nil {
		st.Reviews = []*Review{}
	}
	if st.Questions == nil {
		st.Questions = []*Question{}
	}
	if st.Decisions == nil {
		st.Decisions = []*Decision{}
	}
	if st.Findings == nil {
		st.Findings = []*Finding{}
	}
	if st.Activity == nil {
		st.Activity = []*Activity{}
	}
}

func NextID(st *State, prefix string) string {
	st.Counters[prefix]++
	return fmt.Sprintf("%s-%d", prefix, st.Counters[prefix])
}

func Log(st *State, text, kind string) {
	var agent *string
	for _, a := range st.Agents {
		if a.ID == Actor && Actor != "" {
			agent = ptr(Actor)
			a.UpdatedAt = Now()
		}
	}
	st.Activity = append(st.Activity, &Activity{At: Now(), Kind: kind, Text: text, Agent: agent})
	if n := len(st.Activity); n > 300 {
		st.Activity = st.Activity[n-300:]
	}
}

// ------------------------------------------------------------------ agents

func NewAgent(st *State, role, task string, phase *PhaseRef, logIt bool) *Agent {
	used := map[string]bool{}
	for _, a := range st.Agents {
		used[a.Name] = true
	}
	n := len(st.Agents)
	names := AgentNames[1:] // the first name is the orchestrator's alone
	if n == 0 {
		names = AgentNames
	}
	name := fmt.Sprintf("%s %d", names[n%len(names)], n/len(names)+1)
	for _, x := range names {
		if !used[x] {
			name = x
			break
		}
	}
	a := &Agent{
		ID: fmt.Sprintf("A-%d", n+1), Name: name, Role: role, Model: ModelFor(st, role),
		Task: task, Phase: phase, Status: "working", Avatar: json.RawMessage("null"),
		StartedAt: Now(), UpdatedAt: Now(),
	}
	st.Agents = append(st.Agents, a)
	if logIt {
		Log(st, fmt.Sprintf("%s started as %s on %s: %s", name, lower(RoleLabel(st, role)), ModelName(a.Model), task), "agent")
	}
	return a
}

func RoleLabel(st *State, role string) string {
	if r, ok := KindByID(st.Template).Role(role); ok {
		return r.Label
	}
	return role
}

func GetAgent(st *State, id string) (*Agent, error) {
	for _, a := range st.Agents {
		if a.ID == id {
			return a, nil
		}
	}
	ids := make([]string, len(st.Agents))
	for i, a := range st.Agents {
		ids[i] = a.ID
	}
	return nil, Gate("No agent %s. Agents: %s.", id, join(ids, ", "))
}

func MustAgent(st *State, id string) *Agent {
	a, err := GetAgent(st, id)
	if err != nil {
		panic(err)
	}
	return a
}

// ActorID is the agent running this command ("" in the console server).
func ActorID(st *State) (*string, error) {
	if Actor == "" {
		return nil, nil
	}
	a, err := GetAgent(st, Actor)
	if err != nil {
		return nil, err
	}
	if a.Status == "done" || a.Status == "failed" {
		return nil, Gate("%s (%s) is %s; start a new agent with `agent start`.", a.Name, a.ID, a.Status)
	}
	return ptr(a.ID), nil
}

// AgentStatus is what the console shows: an agent with an unanswered question is waiting for the user.
func AgentStatus(st *State, a *Agent) string {
	if a.Status == "done" || a.Status == "failed" {
		return a.Status
	}
	for _, q := range st.Questions {
		if q.Status == "open" && q.AskedBy == a.ID {
			return "waiting"
		}
	}
	if a.ID == Orchestrator {
		if st.Stage == "done" {
			return "done"
		}
		if BlockedReason(st) != "" {
			return "blocked"
		}
		if st.Attention != nil {
			return "terminal" // the CLI is waiting for the user at the terminal (a permission prompt)
		}
		if IsListening(st) {
			return "listening" // turn finished; the session waits for a console message
		}
	}
	return a.Status
}

func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// ListenerAlive: a `lacarta listen` process is waiting for the console on the agent's behalf.
func ListenerAlive(st *State) bool { return st.Listening != nil && PIDAlive(st.Listening.PID) }

// IsListening: the agent's turn has ended and its listener is running, so a message from the
// console starts its next turn. The console shows it as "listening".
func IsListening(st *State) bool {
	if !ListenerAlive(st) {
		return false
	}
	for _, a := range st.Agents {
		if a.ID == Orchestrator {
			return a.Status == "idle"
		}
	}
	return false
}
