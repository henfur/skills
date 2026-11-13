// Package plugins knows the skills, attack-class companions and check tools lacarta can give
// agents, installs or adopts them machine-wide (~/.lacarta/plugins.json), and enables them per
// project. A kind can require a plugin: the security audit requires Cloudflare's skill.
package plugins

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"lacarta/internal/core"
)

const (
	TypeSkill     = "skill"
	TypeCompanion = "companion"
	TypeCheck     = "check"

	Required = "required"
	Default  = "default" // on unless the user turns it off
	Optional = "optional"

	SecurityAudit = "security-audit"
)

// Def is a plugin lacarta knows how to find, install and check.
type Def struct {
	ID, Type, Name, About string
	Kinds                 map[string]string // kind -> Required, Default or Optional
	Repo, SubPath, Commit string            // skills: where the tested version comes from
	TestedHash            string            // ContentHash of the tested version
	Files                 []string          // files a skill must have
	Program               string            // checks: the program on the PATH
	InstallHint           string
}

// Builtins are the plugins lacarta ships definitions for.
var Builtins = []Def{
	{
		ID: SecurityAudit, Type: TypeSkill, Name: "Cloudflare security-audit skill",
		About: "The audit methodology: reconnaissance, coverage-led hunting, independent validation, the findings " +
			"schema and its validators.",
		Kinds: map[string]string{core.Audit: Required},
		Repo:  "https://github.com/cloudflare/security-audit-skill", SubPath: "skills/security-audit",
		Commit:     "c1c8a8c1471069fb0e188eeaff69b8e8db6564a8",
		TestedHash: "8513eb3b2c39b8d0", // ContentHash of that commit's skill folder
		Files: []string{"SKILL.md", "RECONNAISSANCE.md", "HUNTING.md", "ATTACK-CLASSES.md",
			"VALIDATION-AND-REPORTING.md", "report-schema.json", "validate-findings.cjs", "validate-coverage-ledger.cjs",
			"validate-findings.test.cjs", "validate-coverage-ledger.test.cjs"},
		InstallHint: "lacarta plugin install security-audit",
	},
	{
		ID: "gitleaks", Type: TypeCheck, Name: "gitleaks",
		About: "Secrets in the tree and in git history. Hits are leads for the hunters, redacted; the report never " +
			"contains a secret value.",
		Kinds: map[string]string{core.Audit: Default}, Program: "gitleaks",
		InstallHint: "nix profile install nixpkgs#gitleaks (or your package manager)",
	},
	{
		ID: "osv-scanner", Type: TypeCheck, Name: "osv-scanner",
		About: "Known-vulnerable dependencies from lockfiles, against an offline copy of the OSV database. A hit is a " +
			"lead until a hunter shows the vulnerable code is reachable.",
		Kinds: map[string]string{core.Audit: Default}, Program: "osv-scanner",
		InstallHint: "nix profile install nixpkgs#osv-scanner (or your package manager)",
	},
}

func DefByID(id string) (Def, bool) {
	for _, d := range Builtins {
		if d.ID == id {
			return d, true
		}
	}
	return Def{}, false
}

// Plugin is an installed (or adopted) plugin in the machine registry.
type Plugin struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	About       string `json:"about,omitempty"`
	Source      string `json:"source"` // git URL, local path, or "adopted"
	Ref         string `json:"ref"`    // content hash (skills, companions) or program path (checks)
	Path        string `json:"path"`
	InstalledAt string `json:"installed_at"`
	TrustedAt   string `json:"trusted_at"`
	Custom      bool   `json:"custom"`
}

type Registry struct {
	Plugins core.OMap[*Plugin] `json:"plugins"`
	// Validator self-tests passed, by skill content hash.
	SelfTests map[string]string `json:"self_tests"`
}

func registryPath() string { return filepath.Join(core.Home(), "plugins.json") }

func Load() (*Registry, error) {
	r := &Registry{SelfTests: map[string]string{}}
	b, err := os.ReadFile(registryPath())
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, r); err != nil {
		return nil, fmt.Errorf("plugins.json: %w", err)
	}
	if r.SelfTests == nil {
		r.SelfTests = map[string]string{}
	}
	return r, nil
}

// Edit changes the registry under its lock.
func Edit(fn func(*Registry) error) error {
	if err := os.MkdirAll(core.Home(), 0o755); err != nil {
		return err
	}
	release, err := core.Lock(filepath.Join(core.Home(), ".plugins.lock"), true)
	if err != nil {
		return err
	}
	defer release()
	r, err := Load()
	if err != nil {
		return err
	}
	if err := fn(r); err != nil {
		return err
	}
	return core.WriteJSON(registryPath(), r)
}

// ContentHash identifies a folder's content: every regular file but .git, by relative path.
func ContentHash(dir string) (string, error) {
	type entry struct{ rel, path string }
	var files []entry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, p)
			files = append(files, entry{filepath.ToSlash(rel), p})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f.path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", f.rel, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// SkillDirs are where agent CLIs look for skills.
func SkillDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{}
	if c := os.Getenv("CLAUDE_CONFIG_DIR"); c != "" {
		dirs = append(dirs, filepath.Join(c, "skills"))
	}
	return append(dirs, filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".agents", "skills"))
}

// SkillName reads the `name:` of a SKILL.md front matter.
func SkillName(dir string) string {
	f, err := os.Open(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	in := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "---" {
			if in {
				return ""
			}
			in = true
			continue
		}
		if in && strings.HasPrefix(line, "name:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), `"'`)
		}
	}
	return ""
}

// Discover finds installed skills by name (first location wins), symlinks resolved.
func Discover() map[string]string {
	out := map[string]string{}
	for _, d := range SkillDirs() {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p, err := filepath.EvalSymlinks(filepath.Join(d, e.Name()))
			if err != nil {
				continue
			}
			if name := SkillName(p); name != "" {
				if _, seen := out[name]; !seen {
					out[name] = p
				}
			}
		}
	}
	return out
}

// Info is a plugin's state on this machine, for the console and `plugin list`.
type Info struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	About       string            `json:"about"`
	Kinds       map[string]string `json:"kinds"`
	Installed   bool              `json:"installed"`
	Path        string            `json:"path"`
	Ref         string            `json:"ref"`
	Tested      *bool             `json:"tested"` // skills with a tested version: is this it?
	Problem     string            `json:"problem"`
	InstallHint string            `json:"install_hint"`
	Custom      bool              `json:"custom"`
	Source      string            `json:"source"`
}

// List returns every known plugin: the built-in ones, then custom ones from the registry.
func List() ([]*Info, error) {
	r, err := Load()
	if err != nil {
		return nil, err
	}
	found := Discover()
	out := []*Info{}
	for _, d := range Builtins {
		out = append(out, info(d, r, found))
	}
	for _, id := range r.Plugins.Keys() {
		p, _ := r.Plugins.Get(id)
		if !p.Custom {
			continue
		}
		i := &Info{ID: p.ID, Type: p.Type, Name: p.Name, About: p.About, Kinds: map[string]string{},
			Path: p.Path, Ref: p.Ref, Custom: true, Source: p.Source}
		if _, err := os.Stat(p.Path); err == nil {
			i.Installed = true
			if h, err := ContentHash(p.Path); err == nil && h != p.Ref {
				i.Problem = "Its files changed since it was trusted (" + p.Ref + " → " + h + "): review them and add it again."
			}
		} else {
			i.Problem = "Its folder is gone: " + p.Path
		}
		out = append(out, i)
	}
	return out, nil
}

func Get(id string) (*Info, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	for _, i := range all {
		if i.ID == id {
			return i, nil
		}
	}
	return nil, core.Gate("No plugin %s. `plugin list` shows them.", id)
}

func info(d Def, r *Registry, found map[string]string) *Info {
	i := &Info{ID: d.ID, Type: d.Type, Name: d.Name, About: d.About, Kinds: d.Kinds, InstallHint: d.InstallHint,
		Source: d.Repo}
	switch d.Type {
	case TypeCheck:
		if p, err := exec.LookPath(d.Program); err == nil {
			i.Installed, i.Path, i.Ref = true, p, p
		} else {
			i.Problem = d.Program + " is not installed."
		}
	case TypeSkill:
		path := ""
		if p, ok := r.Plugins.Get(d.ID); ok {
			path = p.Path
		} else if p, ok := found[d.ID]; ok {
			path = p
		}
		if path == "" {
			i.Problem = "Not installed."
			return i
		}
		i.Path = path
		for _, f := range d.Files {
			if _, err := os.Stat(filepath.Join(path, f)); err != nil {
				i.Problem = fmt.Sprintf("Incomplete: %s is missing in %s.", f, path)
				return i
			}
		}
		h, err := ContentHash(path)
		if err != nil {
			i.Problem = err.Error()
			return i
		}
		i.Installed, i.Ref = true, h
		if d.TestedHash != "" {
			t := h == d.TestedHash
			i.Tested = &t
		}
	}
	return i
}

// Install installs a built-in skill: adopts an existing install, else clones the tested commit
// into ~/.lacarta/plugins/<id> and links it into ~/.claude/skills when that name is free.
func Install(id string, out func(string)) error {
	d, ok := DefByID(id)
	if !ok {
		return core.Gate("No built-in plugin %s. Custom ones are added with `plugin add <git-url|path> --trust`.", id)
	}
	if d.Type == TypeCheck {
		if _, err := exec.LookPath(d.Program); err != nil {
			return core.Gate("%s is a program lacarta runs; install it yourself: %s.", d.Program, d.InstallHint)
		}
		out(d.Program + " is installed.")
		return nil
	}
	if p, ok := Discover()[d.ID]; ok {
		h, err := ContentHash(p)
		if err != nil {
			return err
		}
		out(fmt.Sprintf("Adopted the existing install at %s (content %s).", p, h))
		return Edit(func(r *Registry) error {
			r.Plugins.Set(d.ID, &Plugin{ID: d.ID, Type: d.Type, Name: d.Name, Source: "adopted", Ref: h, Path: p,
				InstalledAt: core.Now(), TrustedAt: core.Now()})
			return nil
		})
	}
	dest := filepath.Join(core.Home(), "plugins", d.ID)
	clone := dest + ".clone"
	_ = os.RemoveAll(clone)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out(fmt.Sprintf("Cloning %s at %s…", d.Repo, d.Commit[:7]))
	for _, args := range [][]string{{"clone", "--quiet", d.Repo, clone}, {"-C", clone, "checkout", "--quiet", d.Commit}} {
		if b, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v\n%s", args[0], err, b)
		}
	}
	_ = os.RemoveAll(dest)
	if err := os.Rename(filepath.Join(clone, d.SubPath), dest); err != nil {
		return err
	}
	_ = os.RemoveAll(clone)
	h, err := ContentHash(dest)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	link := filepath.Join(home, ".claude", "skills", d.ID)
	if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
		_ = os.MkdirAll(filepath.Dir(link), 0o755)
		if err := os.Symlink(dest, link); err == nil {
			out("Linked " + link + " → " + dest)
		}
	} else {
		out(link + " exists and lacarta didn't write it: left as it is.")
	}
	out(fmt.Sprintf("Installed %s at %s (content %s).", d.ID, dest, h))
	return Edit(func(r *Registry) error {
		r.Plugins.Set(d.ID, &Plugin{ID: d.ID, Type: d.Type, Name: d.Name, Source: d.Repo + "@" + d.Commit, Ref: h,
			Path: dest, InstalledAt: core.Now(), TrustedAt: core.Now()})
		return nil
	})
}

// Preview is what the user reviews before trusting a custom plugin.
type Preview struct {
	ID, Type, Name, Path string
	Files                []string
	Skill                string // SKILL.md, or the first markdown file, truncated
	Ref                  string
}

// Add registers a custom skill or companion from a local folder or a git URL. Without trust it
// only fetches it and returns the preview: plugins steer agents that run with the user's rights.
func Add(source, ptype string, trust bool) (*Preview, error) {
	if ptype == "" {
		ptype = TypeSkill
	}
	if ptype != TypeSkill && ptype != TypeCompanion {
		return nil, core.Gate("Custom plugins are skills or companions (attack-class files).")
	}
	path := source
	if strings.Contains(source, "://") || strings.HasPrefix(source, "git@") {
		tmp := filepath.Join(core.Home(), "plugins", ".fetch-"+core.Slug(source))
		if _, err := os.Stat(tmp); err != nil {
			if b, err := exec.Command("git", "clone", "--quiet", "--depth", "1", source, tmp).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("git clone: %v\n%s", err, b)
			}
		}
		path = tmp
	} else if abs, err := filepath.Abs(source); err == nil {
		path = abs
	}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return nil, core.Gate("%s is not a folder.", source)
	}
	p := &Preview{Type: ptype, Path: path}
	if ptype == TypeSkill {
		p.ID = SkillName(path)
		if p.ID == "" {
			return nil, core.Gate("%s has no SKILL.md with a `name:` in its front matter.", path)
		}
		if _, builtin := DefByID(p.ID); builtin {
			return nil, core.Gate("%s is a built-in plugin: `plugin install %s`.", p.ID, p.ID)
		}
		b, _ := os.ReadFile(filepath.Join(path, "SKILL.md"))
		p.Skill = string(b)
	} else {
		p.ID = core.Slug(filepath.Base(path))
	}
	p.Name = p.ID
	_ = filepath.WalkDir(path, func(f string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if err == nil && d.Type().IsRegular() {
			rel, _ := filepath.Rel(path, f)
			p.Files = append(p.Files, rel)
			if p.Skill == "" && strings.HasSuffix(f, ".md") {
				b, _ := os.ReadFile(f)
				p.Skill = string(b)
			}
		}
		return nil
	})
	if len(p.Skill) > 6000 {
		p.Skill = p.Skill[:6000] + "\n…"
	}
	if p.Ref, err = ContentHash(path); err != nil {
		return nil, err
	}
	if !trust {
		return p, nil
	}
	if strings.Contains(path, string(filepath.Separator)+".fetch-") {
		dest := filepath.Join(core.Home(), "plugins", p.ID)
		_ = os.RemoveAll(dest)
		if err := os.Rename(path, dest); err != nil {
			return nil, err
		}
		p.Path = dest
	}
	return p, Edit(func(r *Registry) error {
		r.Plugins.Set(p.ID, &Plugin{ID: p.ID, Type: ptype, Name: p.Name, Source: source, Ref: p.Ref, Path: p.Path,
			InstalledAt: core.Now(), TrustedAt: core.Now(), Custom: true})
		return nil
	})
}

func Remove(id string) error {
	if _, builtin := DefByID(id); builtin {
		return core.Gate("%s is built in; it can be disabled per project, not removed.", id)
	}
	return Edit(func(r *Registry) error {
		if _, ok := r.Plugins.Get(id); !ok {
			return core.Gate("No custom plugin %s.", id)
		}
		r.Plugins.Delete(id)
		return nil
	})
}

// ------------------------------------------------------------ per project

// ProjectPlugin is a plugin as one project sees it.
type ProjectPlugin struct {
	*Info
	Enabled bool   `json:"enabled"`
	Need    string `json:"need"` // Required, Default or Optional for this project's kind
	Pinned  string `json:"pinned"`
	Drift   bool   `json:"drift"` // the installed version differs from the project's pin
}

// ForProject lists the plugins of a project with their state there.
func ForProject(st *core.State) ([]*ProjectPlugin, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	out := []*ProjectPlugin{}
	for _, i := range all {
		need := i.Kinds[st.Template]
		if need == "" {
			need = Optional
		}
		p := &ProjectPlugin{Info: i, Need: need}
		if pp := st.Plugins[i.ID]; pp != nil {
			p.Enabled, p.Pinned = pp.Enabled, pp.Ref
		} else {
			p.Enabled = (need == Required || need == Default) && i.Installed && i.Problem == ""
		}
		if need == Required {
			p.Enabled = true
		}
		p.Drift = p.Pinned != "" && i.Ref != "" && p.Pinned != i.Ref && i.Type != TypeCheck
		out = append(out, p)
	}
	return out, nil
}

// Enabled returns the project's enabled plugins.
func Enabled(st *core.State) ([]*ProjectPlugin, error) {
	all, err := ForProject(st)
	if err != nil {
		return nil, err
	}
	out := []*ProjectPlugin{}
	for _, p := range all {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out, nil
}

// SetEnabled turns a plugin on or off for a project and pins its current version.
func SetEnabled(st *core.State, id string, on bool, by string) error {
	all, err := ForProject(st)
	if err != nil {
		return err
	}
	for _, p := range all {
		if p.ID != id {
			continue
		}
		if !on && p.Need == Required {
			return core.Gate("%s is required for this kind of project.", p.Name)
		}
		if on && (!p.Installed || p.Problem != "") {
			hint := p.Problem
			if p.InstallHint != "" {
				hint += " Install: " + p.InstallHint
			}
			return core.Gate("%s can't be enabled: %s", p.Name, strings.TrimSpace(hint))
		}
		if st.Plugins == nil {
			st.Plugins = map[string]*core.ProjectPlugin{}
		}
		st.Plugins[id] = &core.ProjectPlugin{Enabled: on, Ref: p.Ref, By: by, At: core.Now()}
		state := "disabled"
		if on {
			state = "enabled"
		}
		core.Log(st, fmt.Sprintf("Plugin %s %s (version %s). It applies from the next wave or session.", p.Name, state, short(p.Ref)), "stage")
		return nil
	}
	return core.Gate("No plugin %s.", id)
}

// PinAll pins the enabled plugins' current versions (at the start of an audit run).
func PinAll(st *core.State) error {
	all, err := ForProject(st)
	if err != nil {
		return err
	}
	if st.Plugins == nil {
		st.Plugins = map[string]*core.ProjectPlugin{}
	}
	for _, p := range all {
		if pp := st.Plugins[p.ID]; pp == nil {
			st.Plugins[p.ID] = &core.ProjectPlugin{Enabled: p.Enabled, Ref: p.Ref, By: "lacarta", At: core.Now()}
		} else if pp.Ref == "" {
			pp.Ref = p.Ref
		}
	}
	return nil
}

func short(ref string) string {
	if len(ref) > 12 && !strings.Contains(ref, "/") {
		return ref[:12]
	}
	return ref
}
