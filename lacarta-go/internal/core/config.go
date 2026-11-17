package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Config files. ~/.lacarta/config.json holds the machine's choices; the console's Settings page
// writes it too. A project's own config.json (in its state folder: <repo>/.lacarta for a software
// project) overrides some of them for that project. Built-in defaults < home < project.
//
// A value the file can't hold is skipped with a warning, so a typo never stops a session:
// `lacarta config check` and the console show the warnings. Keys starting with "//" are comments.

const ConfigFile = "config.json"

func HomeConfigPath() string { return filepath.Join(Home(), ConfigFile) }

// legacySettingsPath: where the machine's settings lived before config.json. Read when config.json
// doesn't exist yet; the first save moves them.
func legacySettingsPath() string { return filepath.Join(Home(), "settings.json") }

// LegacySettings says whether there is a settings.json and no config.json yet.
func LegacySettings() bool {
	_, errNew := os.Stat(HomeConfigPath())
	_, errOld := os.Stat(legacySettingsPath())
	return errors.Is(errNew, os.ErrNotExist) && errOld == nil
}

// Tools whose path the home config may set; anything else is found on PATH.
var Tools = []KV{
	{"git", "clones plugins"},
	{"node", "runs the security-audit skill's validators"},
	{"bwrap", "the sandbox (bubblewrap)"},
	{"prlimit", "the sandbox's resource limits"},
	{"gitleaks", "the gitleaks check"},
	{"osv-scanner", "the osv-scanner check"},
	{"chromium", "prints audit reports to PDF"},
	{"glab", "talks to GitLab for review desks"},
}

// Where a value comes from.
const (
	FromDefault = "default"
	FromHome    = "home"
	FromProject = "project"
)

// Settings: the machine's configuration, as ~/.lacarta/config.json sets it over the defaults.
type Settings struct {
	Models        OMap[*OMap[string]]        `json:"models"`
	RemoteControl bool                       `json:"remote_control"`
	Auditor       string                     `json:"auditor,omitempty"` // the person who signs audit reports
	Templates     string                     `json:"templates,omitempty"`
	Plugins       map[string]bool            `json:"plugins,omitempty"`
	Tools         map[string]string          `json:"tools,omitempty"`
	Config        ConfigMeta                 `json:"config"`
	explicit      map[string]map[string]bool // kind -> role set in the file
	remoteSet     bool
}

// RemoteSet says whether the home config sets remote_control.
func (s *Settings) RemoteSet() bool { return s.remoteSet }

// ConfigMeta tells the console and `config` which file was read and what was wrong in it.
type ConfigMeta struct {
	Path     string   `json:"path"`
	Exists   bool     `json:"exists"`
	Warnings []string `json:"warnings"`
}

// ProjectConfig: what a project's config.json overrides.
type ProjectConfig struct {
	ConfigMeta
	ID            string            `json:"id,omitempty"`     // the project's id in the console, set by lacarta
	Name          string            `json:"name,omitempty"`   // shown everywhere instead of the state's name
	Models        map[string]string `json:"models,omitempty"` // role -> alias, for the project's kind
	RemoteControl *bool             `json:"remote_control,omitempty"`
	Templates     string            `json:"templates,omitempty"` // absolute, inside the project
	Plugins       map[string]bool   `json:"plugins,omitempty"`
}

// readConfig decodes a config file key by key, so one bad value doesn't lose the others.
func readConfig(path string) (raw map[string]json.RawMessage, exists bool, warns []string) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, []string{err.Error()}
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, true, []string{fmt.Sprintf("not valid JSON, the whole file is ignored: %v", err)}
	}
	for k := range raw {
		if strings.HasPrefix(k, "//") {
			delete(raw, k)
		}
	}
	return raw, true, nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func modelAliases() string {
	a := []string{}
	for _, m := range Models {
		a = append(a, m.Alias)
	}
	return join(a, ", ")
}

// expandPath resolves ~ and makes a relative path relative to base.
func expandPath(p, base string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}

func defaultSettings() *Settings {
	s := &Settings{RemoteControl: true, explicit: map[string]map[string]bool{}}
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
	path := HomeConfigPath()
	raw, exists, warns := readConfig(path)
	if !exists {
		if r, ok, w := readConfig(legacySettingsPath()); ok {
			raw, warns = r, w
		}
	}
	s.Config = ConfigMeta{Path: path, Exists: exists, Warnings: []string{}}
	warn := func(format string, a ...any) {
		s.Config.Warnings = append(s.Config.Warnings, fmt.Sprintf(format, a...))
	}
	s.Config.Warnings = append(s.Config.Warnings, warns...)
	for _, key := range sortedKeys(raw) {
		v := raw[key]
		switch key {
		case "models":
			var models map[string]map[string]string
			if json.Unmarshal(v, &models) != nil {
				warn("models: expected {\"<kind>\": {\"<role>\": \"<model>\"}}.")
				continue
			}
			for _, k := range sortedKeys(models) {
				cur, ok := s.Models.Get(k)
				if !ok {
					warn("models: no kind of project %q. Kinds: %s.", k, kindIDs())
					continue
				}
				for _, r := range sortedKeys(models[k]) {
					m := models[k][r]
					if _, known := cur.Get(r); !known {
						warn("models.%s: no role %q. Roles: %s.", k, r, join(cur.Keys(), ", "))
					} else if _, valid := ModelByAlias(m); !valid {
						warn("models.%s.%s: unknown model %q. Models: %s.", k, r, m, modelAliases())
					} else {
						cur.Set(r, m)
						if s.explicit[k] == nil {
							s.explicit[k] = map[string]bool{}
						}
						s.explicit[k][r] = true
					}
				}
			}
		case "remote_control":
			if json.Unmarshal(v, &s.RemoteControl) != nil {
				s.RemoteControl = true
				warn("remote_control: expected true or false.")
			} else {
				s.remoteSet = true
			}
		case "auditor":
			if json.Unmarshal(v, &s.Auditor) != nil || len([]rune(s.Auditor)) > 120 {
				s.Auditor = ""
				warn("auditor: expected a name of at most 120 characters.")
			}
		case "templates":
			var dir string
			if json.Unmarshal(v, &dir) != nil || dir == "" {
				warn("templates: expected the path of a folder.")
				continue
			}
			dir = expandPath(dir, Home())
			if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
				warn("templates: %s is not a folder.", dir)
				continue
			}
			s.Templates = dir
		case "plugins":
			if json.Unmarshal(v, &s.Plugins) != nil {
				s.Plugins = nil
				warn("plugins: expected {\"<plugin id>\": true or false}.")
			}
		case "tools":
			var tools map[string]string
			if json.Unmarshal(v, &tools) != nil {
				warn("tools: expected {\"<tool>\": \"<path>\"}.")
				continue
			}
			for _, name := range sortedKeys(tools) {
				if !isTool(name) {
					warn("tools: no tool %q. Tools: %s.", name, toolNames())
					continue
				}
				p := expandPath(tools[name], Home())
				if fi, err := os.Stat(p); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
					warn("tools.%s: %s is not an executable file.", name, p)
					continue
				}
				if s.Tools == nil {
					s.Tools = map[string]string{}
				}
				s.Tools[name] = p
			}
		default:
			warn("unknown setting %q. Settings: models, remote_control, auditor, templates, plugins, tools.", key)
		}
	}
	return s
}

func kindIDs() string {
	ids := []string{}
	for _, x := range Kinds {
		ids = append(ids, x.ID)
	}
	return join(ids, ", ")
}

func isTool(name string) bool {
	for _, t := range Tools {
		if t.K == name {
			return true
		}
	}
	return false
}

func toolNames() string {
	n := []string{}
	for _, t := range Tools {
		n = append(n, t.K)
	}
	return join(n, ", ")
}

// ToolPath finds a program: the path the home config sets, else the first name found on PATH.
func ToolPath(name string, alts ...string) string {
	if p := SettingsRead().Tools[name]; p != "" {
		return p
	}
	for _, n := range append([]string{name}, alts...) {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// editHomeConfig changes some keys of ~/.lacarta/config.json and keeps the others as they are,
// comments included. A legacy settings.json is folded in on the first save, then removed.
func editHomeConfig(set map[string]any) error {
	path := HomeConfigPath()
	src := path
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		src = legacySettingsPath()
	}
	if err := editConfigFile(src, path, set); err != nil {
		return err
	}
	if src != path {
		_ = os.Remove(src)
	}
	return nil
}

// editConfigFile reads src (if it exists), sets the given keys (nil removes one) and writes dst.
func editConfigFile(src, dst string, set map[string]any) error {
	raw := map[string]json.RawMessage{}
	if b, err := os.ReadFile(src); err == nil {
		if err := json.Unmarshal(b, &raw); err != nil {
			return Gate("%s is not valid JSON (%v): fix it by hand first, nothing was saved.", src, err)
		}
	}
	for k, v := range set {
		if v == nil {
			delete(raw, k)
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		raw[k] = b
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return WriteJSON(dst, raw)
}

// modelOverrides: the roles whose model differs from the default or that the file set before.
func (s *Settings) modelOverrides() map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, k := range Kinds {
		cur, _ := s.Models.Get(k.ID)
		for _, r := range k.Roles {
			m, _ := cur.Get(r.ID)
			if m != r.Default || s.explicit[k.ID][r.ID] {
				if out[k.ID] == nil {
					out[k.ID] = map[string]string{}
				}
				out[k.ID][r.ID] = m
			}
		}
	}
	return out
}

// SettingsUpdate changes models ({kind: {role: alias}}) and, if non-nil, the remote control default.
func SettingsUpdate(models map[string]map[string]string, remote *bool) (*Settings, error) {
	err := HubEdit(func(*Hub) error {
		s := SettingsRead()
		for k, roles := range models {
			cur, ok := s.Models.Get(k)
			if !ok {
				return Gate("Unknown kind of project %s. Kinds: %s.", k, kindIDs())
			}
			for r, m := range roles {
				if _, ok := cur.Get(r); !ok {
					return Gate("Unknown role %s for %s. Roles: %s.", r, k, join(cur.Keys(), ", "))
				}
				if _, ok := ModelByAlias(m); !ok {
					return Gate("Unknown model %s. Models: %s.", m, modelAliases())
				}
				cur.Set(r, m)
			}
		}
		set := map[string]any{}
		if len(models) > 0 {
			set["models"] = s.modelOverrides()
		}
		if remote != nil {
			set["remote_control"] = *remote
		}
		return editHomeConfig(set)
	})
	if err != nil {
		return nil, err
	}
	return SettingsRead(), nil
}

// SetAuditor records the name of the person who signs audit reports. Only the user sets it.
func SetAuditor(name string) (*Settings, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 120 {
		return nil, Gate("That name is too long.")
	}
	err := HubEdit(func(*Hub) error {
		var v any = name
		if name == "" {
			v = nil
		}
		return editHomeConfig(map[string]any{"auditor": v})
	})
	if err != nil {
		return nil, err
	}
	return SettingsRead(), nil
}

// ------------------------------------------------------------------ project config

// LocalRoot is the folder a session is about: its git repository, else the folder itself.
func LocalRoot(cwd string) string {
	d, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	for x := d; ; {
		if _, err := os.Stat(filepath.Join(x, ".git")); err == nil {
			return x
		}
		parent := filepath.Dir(x)
		if parent == x {
			return d
		}
		x = parent
	}
}

// DefaultName names a session's project after its git repository, else its folder.
func DefaultName(cwd string) string {
	name := filepath.Base(LocalRoot(cwd))
	if name == "" || name == "/" || name == "." {
		return "Task"
	}
	return name
}

// ProjectConfigPath: where a project's config.json is. A software project's is in its repository;
// a single-agent task's in the folder it works in (its git repository's root, else the folder),
// shared by every task there; an audit's beside its state, never inside the audited code. ""
// when there is none: a task in the home folder, whose .lacarta is lacarta's own.
func ProjectConfigPath(template string, s *Store) string {
	switch template {
	case Single:
		if s.Root == "" {
			return ""
		}
		root := LocalRoot(s.Root)
		home, _ := os.UserHomeDir()
		dir := filepath.Join(root, StateDir)
		if root == "/" || root == home || dir == Home() {
			return ""
		}
		return filepath.Join(dir, ConfigFile)
	default:
		if s.Dir == "" {
			return ""
		}
		return filepath.Join(s.Dir, ConfigFile)
	}
}

func projectConfigPath(st *State) string {
	return ProjectConfigPath(st.Template, &Store{Root: st.Root, Dir: st.Dir})
}

// EnsureProjectConfig gives a project's config.json the keys it lacks: its id (which the console
// and the next sessions in that folder know it by) and its name. It creates the file if needed and
// never changes a value already there. Best effort: a folder lacarta can't write to has no file.
func EnsureProjectConfig(st *State, name, id string) {
	ensureConfigKeys(projectConfigPath(st), map[string]string{"name": name, "id": id})
}

func ensureConfigKeys(path string, kv map[string]string) {
	if path == "" {
		return
	}
	raw, exists, warns := readConfig(path)
	if exists && raw == nil && len(warns) > 0 {
		return // unreadable or not JSON: leave the user's file alone
	}
	set := map[string]any{}
	for k, v := range kv {
		if _, ok := raw[k]; !ok && v != "" {
			set[k] = v
		}
	}
	if len(set) > 0 {
		_ = editConfigFile(path, path, set)
	}
}

// Project ids: lowercase letters, digits and dashes, as in the console's URLs.
var projectIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

// NewProjectID: a random id with a readable prefix ("t" for a task, the slug of a project's name).
func NewProjectID(prefix string) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

// configID reads the project id a config file holds, "" if none or invalid.
func configID(path string) string {
	if path == "" {
		return ""
	}
	raw, _, _ := readConfig(path)
	var id string
	if json.Unmarshal(raw["id"], &id) != nil || !projectIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// setConfigID replaces the id in a config file: a copied folder gets its own.
func setConfigID(path, id string) {
	if path != "" {
		_ = editConfigFile(path, path, map[string]any{"id": id})
	}
}

// ProjectConfigSetName writes the project's name into its config.json.
func ProjectConfigSetName(st *State, name string) error {
	path := projectConfigPath(st)
	if path == "" {
		return Gate("This project has no folder for a config file.")
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return Gate("A name has 1 to 120 characters.")
	}
	return editConfigFile(path, path, map[string]any{"name": name})
}

// ProjectConfigRead reads a project's config.json. Tool paths and the auditor are refused there:
// a software project's config is in its repository, and a cloned repository must not choose
// which programs lacarta runs or who signs.
func ProjectConfigRead(st *State) *ProjectConfig {
	pc := &ProjectConfig{ConfigMeta: ConfigMeta{Path: projectConfigPath(st)}}
	if pc.Path == "" {
		return pc
	}
	raw, exists, warns := readConfig(pc.Path)
	pc.Exists, pc.Warnings = exists, append([]string{}, warns...)
	warn := func(format string, a ...any) { pc.Warnings = append(pc.Warnings, fmt.Sprintf(format, a...)) }
	kind := KindByID(st.Template)
	for _, key := range sortedKeys(raw) {
		v := raw[key]
		switch key {
		case "id":
			var id string
			if json.Unmarshal(v, &id) != nil || !projectIDPattern.MatchString(id) {
				warn("id: expected lowercase letters, digits and dashes (3 to 63). It is set by lacarta: keep it as it is.")
				continue
			}
			pc.ID = id
		case "name":
			var name string
			if json.Unmarshal(v, &name) != nil || strings.TrimSpace(name) == "" || len([]rune(name)) > 120 {
				warn("name: expected a name of at most 120 characters.")
				continue
			}
			pc.Name = strings.TrimSpace(name)
		case "models":
			var models map[string]string
			if json.Unmarshal(v, &models) != nil {
				warn("models: expected {\"<role>\": \"<model>\"} for this %s project.", strings.ToLower(kind.Label))
				continue
			}
			for _, r := range sortedKeys(models) {
				if _, ok := kind.Role(r); !ok {
					ids := []string{}
					for _, x := range kind.Roles {
						ids = append(ids, x.ID)
					}
					warn("models: no role %q in %s projects. Roles: %s.", r, strings.ToLower(kind.Label), join(ids, ", "))
				} else if _, ok := ModelByAlias(models[r]); !ok {
					warn("models.%s: unknown model %q. Models: %s.", r, models[r], modelAliases())
				} else {
					if pc.Models == nil {
						pc.Models = map[string]string{}
					}
					pc.Models[r] = models[r]
				}
			}
		case "remote_control":
			var b bool
			if json.Unmarshal(v, &b) != nil {
				warn("remote_control: expected true or false.")
				continue
			}
			pc.RemoteControl = &b
		case "templates":
			if st.Template != Software {
				warn("templates: only software projects have spec templates.")
				continue
			}
			var dir string
			if json.Unmarshal(v, &dir) != nil || dir == "" || strings.HasPrefix(dir, "~") || filepath.IsAbs(dir) {
				warn("templates: expected a folder inside the project, relative to its root.")
				continue
			}
			root := st.Root
			abs := filepath.Clean(filepath.Join(root, dir))
			real, err := filepath.EvalSymlinks(abs)
			realRoot, _ := filepath.EvalSymlinks(root)
			if err != nil {
				warn("templates: %s doesn't exist.", abs)
				continue
			}
			if rel, err := filepath.Rel(realRoot, real); err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
				warn("templates: %s is outside the project.", dir)
				continue
			}
			if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
				warn("templates: %s is not a folder.", abs)
				continue
			}
			pc.Templates = real
		case "plugins":
			if json.Unmarshal(v, &pc.Plugins) != nil {
				pc.Plugins = nil
				warn("plugins: expected {\"<plugin id>\": true or false}.")
			}
		case "desk":
			if st.Template != Desk {
				warn("desk: only review desks have desk settings.")
				continue
			}
			for _, w := range DeskConfigRead(st).Warnings {
				warn("%s", w)
			}
		case "tools", "auditor":
			warn("%s: can only be set in %s, not in a project.", key, HomeConfigPath())
		default:
			warn("unknown setting %q. A project can set: id, name, models, remote_control, templates, plugins (and desk, for a review desk).", key)
		}
	}
	return pc
}

// ProjectConfigSetModels writes role models into the project's config.json (alias "" removes one).
func ProjectConfigSetModels(st *State, roles map[string]string) error {
	path := projectConfigPath(st)
	if path == "" {
		return Gate("This project has no folder for a config file.")
	}
	kind := KindByID(st.Template)
	models := map[string]string{}
	if raw, _, _ := readConfig(path); raw["models"] != nil {
		_ = json.Unmarshal(raw["models"], &models)
	}
	for r, m := range roles {
		if _, ok := kind.Role(r); !ok {
			return Gate("Unknown role %s for %s. `models` lists them.", r, kind.ID)
		}
		if m == "" || m == "default" {
			delete(models, r)
			continue
		}
		if _, ok := ModelByAlias(m); !ok {
			return Gate("Unknown model %s. Models: %s.", m, modelAliases())
		}
		models[r] = m
	}
	var v any = models
	if len(models) == 0 {
		v = nil
	}
	return editConfigFile(path, path, map[string]any{"models": v})
}

// RoleModel: the model a role gets in a project, and which layer chose it.
type RoleModel struct {
	Role   string `json:"role"`
	Model  string `json:"model"`
	Source string `json:"source"`
}

// ModelsFor lists every role of the project's kind with its effective model.
func ModelsFor(st *State) []RoleModel {
	s := SettingsRead()
	pc := ProjectConfigRead(st)
	home, _ := s.Models.Get(st.Template)
	out := []RoleModel{}
	for _, r := range KindByID(st.Template).Roles {
		rm := RoleModel{r.ID, r.Default, FromDefault}
		if m, ok := home.Get(r.ID); ok && (m != r.Default || s.explicit[st.Template][r.ID]) {
			rm.Model, rm.Source = m, FromHome
		}
		if m := pc.Models[r.ID]; m != "" {
			rm.Model, rm.Source = m, FromProject
		}
		out = append(out, rm)
	}
	return out
}

func ModelFor(st *State, role string) string {
	for _, rm := range ModelsFor(st) {
		if rm.Role == role {
			return rm.Model
		}
	}
	r, _ := KindByID(st.Template).Role(role)
	return r.Default
}

// RemoteOn: the project's console toggle, else its config, else the machine's.
func RemoteOn(st *State) bool {
	if st.Remote != nil {
		return *st.Remote
	}
	if pc := ProjectConfigRead(st); pc.RemoteControl != nil {
		return *pc.RemoteControl
	}
	return SettingsRead().RemoteControl
}

// TemplateDirs: where a new software project's spec templates are looked for, most specific
// first: the project's config, then the home config. The built-in templates come last.
func TemplateDirs(st *State) []string {
	dirs := []string{}
	if d := ProjectConfigRead(st).Templates; d != "" {
		dirs = append(dirs, d)
	}
	if d := SettingsRead().Templates; d != "" {
		dirs = append(dirs, d)
	}
	return dirs
}
