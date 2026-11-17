package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// A CLI session (Claude Code for now) registers through hooks and is attached to agent A-1
// of its project.

const CLIClaude = "claude-code"

// CreateSingle creates a single-agent task; id "" draws a new one.
func CreateSingle(cwd, id string) (string, *Store, error) {
	tid := id
	if tid == "" {
		tid = NewProjectID("t")
	}
	dir := filepath.Join(Home(), "tasks", tid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	name := DefaultName(cwd)
	st := &State{
		Version: 1, Template: Single,
		Project: Project{Name: name, Cwd: cwd, CreatedAt: Now()},
		Stage:   "active", Agents: []*Agent{}, Counters: map[string]int{},
		Root: cwd, Dir: dir,
	}
	EnsureProjectConfig(st, name, tid) // keeps an id already there: then this task runs beside the folder's own
	if n := ProjectConfigRead(st).Name; n != "" {
		name, st.Project.Name = n, n
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
		if st.Template == Single { // tasks from before config files get theirs now
			EnsureProjectConfig(st, DefaultName(st.Root), filepath.Base(st.Dir)) // a task's folder is named by its id
			if n := ProjectConfigRead(st).Name; n != "" {
				st.Project.Name = n
			}
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
	} else if aid := AuditFor(cwd); aid != "" {
		pid = aid
		if s, err = ProjectStore(aid); err != nil {
			return "", nil, err
		}
	} else if pid, s, err = singleFor(h, cwd, source); err != nil {
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

// singleFor finds the task a new session in cwd continues: the one whose id is in the folder's
// config.json (or, for a folder from before ids, its latest task, which then gets the id). A new
// task is created when there is none, when the folder is a copy of another one with the same id
// (the copy gets its own), or when the folder's task still has a live session: two sessions at
// once are two tasks.
func singleFor(h *Hub, cwd, source string) (string, *Store, error) {
	cfg := ProjectConfigPath(Single, &Store{Root: cwd})
	root := LocalRoot(cwd)
	id := configID(cfg)
	if id == "" { // adopt the folder's latest task
		for _, k := range h.Projects.Keys() {
			if p, _ := h.Projects.Get(k); p.Template == Single && p.Cwd != "" && LocalRoot(p.Cwd) == root {
				id = k
			}
		}
	}
	if id != "" {
		if p, ok := h.Projects.Get(id); ok && p.Template == Single {
			oldRoot := LocalRoot(firstNonEmptyStr(p.Cwd, cwd))
			copied := oldRoot != root && configID(ProjectConfigPath(Single, &Store{Root: oldRoot})) == id
			if copied {
				pid, s, err := CreateSingle(cwd, "")
				if err == nil {
					setConfigID(cfg, pid)
				}
				return pid, s, err
			}
			s, err := ProjectStore(id)
			if err != nil {
				return "", nil, err
			}
			st, err := s.Read()
			if err != nil {
				return "", nil, err
			}
			if st.MovedTo == "" && (resumes(source) || !SessionLive(st)) {
				err := HubEdit(func(h *Hub) error {
					if p, ok := h.Projects.Get(id); ok {
						p.Archived = false
						if oldRoot != root { // the folder moved: follow it
							p.Cwd = cwd
						}
					}
					return nil
				})
				if err == nil && oldRoot != root {
					err = s.Edit(func(st *State) error { st.Project.Cwd = cwd; return nil })
					s.Root = cwd
				}
				return id, s, err
			}
			return CreateSingle(cwd, "") // the folder's task is busy with another session
		} else if !ok {
			return CreateSingle(cwd, id) // the console forgot it (or a new machine): keep the folder's id
		}
	}
	return CreateSingle(cwd, "")
}

// resumes: the CLI continues a conversation rather than starting one.
func resumes(source string) bool {
	return source == "resume" || source == "compact" || source == "clear"
}

// SessionStaleAfter: a session that hasn't written to its transcript for this long is taken as
// gone, even if its CLI never reported its end.
var SessionStaleAfter = 30 * time.Minute

// SessionLive says whether the orchestrator's session is still running.
func SessionLive(st *State) bool {
	a := MustAgent(st, Orchestrator)
	if a.Session == nil || a.Session.EndedAt != nil {
		return false
	}
	if st.Listening != nil && st.Listening.PID != 0 {
		return true
	}
	last := a.Session.StartedAt
	if n := len(st.Activity); n > 0 && st.Activity[n-1].At > last {
		last = st.Activity[n-1].At
	}
	if fi, err := os.Stat(a.Session.Transcript); err == nil {
		if m := fi.ModTime().UTC().Format("2006-01-02T15:04:05Z"); m > last {
			last = m
		}
	}
	t, err := time.Parse(time.RFC3339, last)
	return err == nil && Clock().Sub(t) < SessionStaleAfter
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
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
