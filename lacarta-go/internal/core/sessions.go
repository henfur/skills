package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// A CLI session (Claude Code for now) registers through hooks and is attached to agent A-1
// of its project.

const CLIClaude = "claude-code"

func CreateSingle(cwd string) (string, *Store, error) {
	b := make([]byte, 3)
	rand.Read(b)
	tid := "t-" + hex.EncodeToString(b)
	dir := filepath.Join(Home(), "tasks", tid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	name := filepath.Base(cwd)
	if name == "" || name == "/" || name == "." {
		name = "Task"
	}
	st := &State{
		Version: 1, Template: Single,
		Project: Project{Name: name, Cwd: cwd, CreatedAt: Now()},
		Stage:   "active", Agents: []*Agent{}, Counters: map[string]int{},
	}
	if err := Migrate(st); err != nil {
		return "", nil, err
	}
	NewAgent(st, "agent", "Works on the task with you.", nil, false)
	Log(st, fmt.Sprintf("Task started in %s.", cwd), "stage")
	if err := WriteJSON(filepath.Join(dir, "state.json"), st); err != nil {
		return "", nil, err
	}
	err := HubEdit(func(h *Hub) error {
		h.Projects.Set(tid, &HubProject{ID: tid, Template: Single, Name: name, Cwd: cwd, CreatedAt: Now()})
		return nil
	})
	return tid, &Store{Root: cwd, Dir: dir}, err
}

func AttachSession(s *Store, sid, cli, transcript, cwd, source string) error {
	return s.Edit(func(st *State) error {
		a, err := GetAgent(st, Orchestrator)
		if err != nil {
			return err
		}
		a.Session = &Session{CLI: cli, ID: sid, Transcript: transcript, Cwd: cwd, StartedAt: Now()}
		a.Status, a.UpdatedAt = "working", Now()
		st.Attention = nil
		if st.Template == Single && st.Stage == "ended" {
			st.Stage = "active"
			a.FinishedAt, a.Summary = nil, ""
		}
		extra := ""
		if source != "" {
			extra = ", " + source
		}
		Log(st, fmt.Sprintf("%s connected (%s session%s).", a.Name, cli, extra), "agent")
		return nil
	})
}

// RegisterSession finds or creates the project of a CLI session.
func RegisterSession(sid, cwd, transcript, source string) (string, *Store, error) {
	h, err := HubRead()
	if err != nil {
		return "", nil, err
	}
	if pid, ok := h.Sessions.Get(sid); ok {
		if _, ok := h.Projects.Get(pid); ok {
			s, err := ProjectStore(pid)
			return pid, s, err
		}
	}
	var pid string
	var s *Store
	if root := FindRoot(cwd); root != "" {
		var head struct {
			Project Project `json:"project"`
		}
		b, err := os.ReadFile(filepath.Join(root, StateDir, "state.json"))
		if err != nil {
			return "", nil, err
		}
		if err := json.Unmarshal(b, &head); err != nil {
			return "", nil, err
		}
		if pid, err = RegisterSoftware(root, head.Project.Name); err != nil {
			return "", nil, err
		}
		s = SoftwareStore(root)
	} else if pid, s, err = CreateSingle(cwd); err != nil {
		return "", nil, err
	}
	err = HubEdit(func(h *Hub) error {
		h.Sessions.Set(sid, pid)
		for keys := h.Sessions.Keys(); len(keys) > 500; keys = keys[1:] {
			h.Sessions.Delete(keys[0])
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	return pid, s, AttachSession(s, sid, CLIClaude, transcript, cwd, source)
}

// MoveSession: `init` inside a registered single-agent session; the session now belongs to
// the new project.
func MoveSession(sid, toPID string, to *Store) error {
	h, err := HubRead()
	if err != nil {
		return err
	}
	old, ok := h.Sessions.Get(sid)
	if !ok || old == toPID {
		return nil
	}
	if p, ok := h.Projects.Get(old); !ok || p.Template != Single {
		return nil
	}
	from, err := ProjectStore(old)
	if err != nil {
		return err
	}
	var sess Session
	err = from.Edit(func(st *State) error {
		a, err := GetAgent(st, Orchestrator)
		if err != nil {
			return err
		}
		if a.Session != nil {
			sess = *a.Session
		}
		a.Status, a.FinishedAt = "done", ptr(Now())
		a.Summary = fmt.Sprintf("Continued as the software project %s.", toPID)
		st.Stage, st.MovedTo = "ended", toPID
		Log(st, fmt.Sprintf("The session became the software project %s.", toPID), "stage")
		WithdrawOpenQuestions(st, "the session moved to a software project.")
		return nil
	})
	if err != nil {
		return err
	}
	if err := HubEdit(func(h *Hub) error {
		h.Sessions.Set(sid, toPID)
		if p, ok := h.Projects.Get(old); ok {
			p.Archived = true
		}
		return nil
	}); err != nil {
		return err
	}
	cli, cwd := sess.CLI, sess.Cwd
	if cli == "" {
		cli = CLIClaude
	}
	if cwd == "" {
		cwd = to.Root
	}
	return AttachSession(to, sid, cli, sess.Transcript, cwd, "")
}
