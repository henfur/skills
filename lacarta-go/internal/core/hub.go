package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ~/.lacarta/hub.json lists every project the console shows and maps CLI sessions to them.

func Home() string {
	if h := os.Getenv("LACARTA_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lacarta")
}

type Hub struct {
	Projects OMap[*HubProject] `json:"projects"`
	Sessions OMap[string]      `json:"sessions"`
}

type HubProject struct {
	ID        string `json:"id"`
	Template  string `json:"template"`
	Name      string `json:"name"`
	Root      string `json:"root,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
	CreatedAt string `json:"created_at"`
	Archived  bool   `json:"archived"`
}

func hubPath() string { return filepath.Join(Home(), "hub.json") }

func loadHub() (*Hub, error) {
	h := &Hub{}
	b, err := os.ReadFile(hubPath())
	if errors.Is(err, os.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return nil, err
	}
	return h, json.Unmarshal(b, h)
}

func HubRead() (*Hub, error) {
	if _, err := os.Stat(hubPath()); errors.Is(err, os.ErrNotExist) {
		return &Hub{}, nil
	}
	release, err := Lock(filepath.Join(Home(), ".lock"), false)
	if err != nil {
		return nil, err
	}
	defer release()
	return loadHub()
}

// HubEdit runs fn on the hub under the hub lock (which also guards settings.json).
func HubEdit(fn func(*Hub) error) error {
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	release, err := Lock(filepath.Join(Home(), ".lock"), true)
	if err != nil {
		return err
	}
	defer release()
	h, err := loadHub()
	if err != nil {
		return err
	}
	if err := fn(h); err != nil {
		return err
	}
	return WriteJSON(hubPath(), h)
}

func Slug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	parts := []string{}
	for _, p := range strings.Split(b.String(), "-") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	out := strings.Join(parts, "-")
	if r := []rune(out); len(r) > 32 {
		out = string(r[:32])
	}
	if out == "" {
		out = "project"
	}
	return out
}

// RegisterSoftware adds a software project to the hub (idempotent) and returns its id.
func RegisterSoftware(root, name string) (string, error) {
	root, _ = filepath.Abs(root)
	cfg := filepath.Join(root, StateDir, ConfigFile)
	id := configID(cfg)
	var pid string
	copied := false
	err := HubEdit(func(h *Hub) error {
		if id != "" {
			if p, ok := h.Projects.Get(id); ok && p.Template == Software {
				switch {
				case p.Root == root:
				case configID(filepath.Join(p.Root, StateDir, ConfigFile)) == id:
					copied = true // a copy of a registered project: it gets its own id below
				default:
					p.Root = root // the project moved: follow it
				}
				if !copied {
					p.Name, pid = name, id
					return nil
				}
			} else if !ok {
				pid = id // the console forgot it (or a new machine): keep the project's id
				h.Projects.Set(pid, &HubProject{ID: pid, Template: Software, Name: name, Root: root, CreatedAt: Now()})
				return nil
			}
		}
		for _, k := range h.Projects.Keys() {
			p, _ := h.Projects.Get(k)
			if p.Root == root && !copied {
				p.Name = name
				pid = k
				return nil
			}
		}
		pid = NewProjectID(Slug(name))
		h.Projects.Set(pid, &HubProject{ID: pid, Template: Software, Name: name, Root: root, CreatedAt: Now()})
		return nil
	})
	if err != nil {
		return "", err
	}
	if copied {
		setConfigID(cfg, pid)
	} else if _, err := os.Stat(filepath.Join(root, StateDir)); err == nil {
		ensureConfigKeys(cfg, map[string]string{"id": pid, "name": name})
	}
	return pid, nil
}

// IsRegistered reports whether a software project root is already in the hub.
func IsRegistered(root string) bool {
	h, err := HubRead()
	if err != nil {
		return false
	}
	for _, k := range h.Projects.Keys() {
		if p, _ := h.Projects.Get(k); p.Root == root {
			return true
		}
	}
	return false
}

func ProjectStore(pid string) (*Store, error) {
	h, err := HubRead()
	if err != nil {
		return nil, err
	}
	p, ok := h.Projects.Get(pid)
	if !ok {
		return nil, Gate("No project %s in %s.", pid, hubPath())
	}
	if p.Template == Audit {
		return &Store{Root: p.Root, Dir: AuditHome(pid)}, nil
	}
	if p.Template == Single {
		cwd := p.Cwd
		if cwd == "" {
			cwd = Home()
		}
		return &Store{Root: cwd, Dir: filepath.Join(Home(), "tasks", pid)}, nil
	}
	return SoftwareStore(p.Root), nil
}

// ------------------------------------------------------------------ daemon

type DaemonInfo struct {
	URL       string `json:"url"`
	OpenURL   string `json:"open_url"`
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

func DaemonRead() *DaemonInfo {
	b, err := os.ReadFile(filepath.Join(Home(), "daemon.json"))
	if err != nil {
		return nil
	}
	var d DaemonInfo
	if json.Unmarshal(b, &d) != nil || !PIDAlive(d.PID) {
		return nil
	}
	return &d
}
