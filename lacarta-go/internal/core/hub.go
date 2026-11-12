package core

import (
	"crypto/sha256"
	"encoding/hex"
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
	var pid string
	err := HubEdit(func(h *Hub) error {
		for _, k := range h.Projects.Keys() {
			p, _ := h.Projects.Get(k)
			if p.Root == root {
				p.Name = name
				pid = k
				return nil
			}
		}
		sum := sha256.Sum256([]byte(root))
		pid = Slug(name) + "-" + hex.EncodeToString(sum[:])[:4]
		h.Projects.Set(pid, &HubProject{ID: pid, Template: Software, Name: name, Root: root, CreatedAt: Now()})
		return nil
	})
	return pid, err
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
	if p.Template == Single {
		cwd := p.Cwd
		if cwd == "" {
			cwd = Home()
		}
		return &Store{Root: cwd, Dir: filepath.Join(Home(), "tasks", pid)}, nil
	}
	return SoftwareStore(p.Root), nil
}

// ------------------------------------------------------------------ settings

type Settings struct {
	Models        OMap[*OMap[string]] `json:"models"`
	RemoteControl bool                `json:"remote_control"`
}

func settingsPath() string { return filepath.Join(Home(), "settings.json") }

func defaultSettings() *Settings {
	s := &Settings{RemoteControl: true}
	for _, k := range Kinds {
		roles := &OMap[string]{}
		for _, r := range k.Roles {
			roles.Set(r.ID, r.Default)
		}
		s.Models.Set(k.ID, roles)
	}
	return s
}

func SettingsRead() *Settings {
	s := defaultSettings()
	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return s
	}
	var saved struct {
		Models        map[string]map[string]string `json:"models"`
		RemoteControl *bool                        `json:"remote_control"`
	}
	if json.Unmarshal(b, &saved) != nil {
		return s
	}
	for k, roles := range saved.Models {
		cur, ok := s.Models.Get(k)
		if !ok {
			continue
		}
		for r, m := range roles {
			_, known := cur.Get(r)
			if _, valid := ModelByAlias(m); known && valid {
				cur.Set(r, m)
			}
		}
	}
	if saved.RemoteControl != nil {
		s.RemoteControl = *saved.RemoteControl
	}
	return s
}

// SettingsUpdate changes models ({kind: {role: alias}}) and, if non-nil, the remote control default.
func SettingsUpdate(models map[string]map[string]string, remote *bool) (*Settings, error) {
	var out *Settings
	err := HubEdit(func(*Hub) error {
		s := SettingsRead()
		for k, roles := range models {
			cur, ok := s.Models.Get(k)
			if !ok {
				ids := []string{}
				for _, x := range Kinds {
					ids = append(ids, x.ID)
				}
				return Gate("Unknown kind of project %s. Kinds: %s.", k, join(ids, ", "))
			}
			for r, m := range roles {
				if _, ok := cur.Get(r); !ok {
					return Gate("Unknown role %s for %s. Roles: %s.", r, k, join(cur.Keys(), ", "))
				}
				if _, ok := ModelByAlias(m); !ok {
					aliases := []string{}
					for _, x := range Models {
						aliases = append(aliases, x.Alias)
					}
					return Gate("Unknown model %s. Models: %s.", m, join(aliases, ", "))
				}
				cur.Set(r, m)
			}
		}
		if remote != nil {
			s.RemoteControl = *remote
		}
		out = s
		return WriteJSON(settingsPath(), s)
	})
	return out, err
}

func ModelFor(st *State, role string) string {
	if roles, ok := SettingsRead().Models.Get(st.Template); ok {
		if m, ok := roles.Get(role); ok && m != "" {
			return m
		}
	}
	r, _ := KindByID(st.Template).Role(role)
	return r.Default
}

func RemoteOn(st *State) bool {
	if st.Remote == nil {
		return SettingsRead().RemoteControl
	}
	return *st.Remote
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
