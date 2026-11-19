package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"lacarta/internal/core"
	"lacarta/internal/plugins"
)

func configCommands() []*Cmd {
	return []*Cmd{
		{Name: "config", Help: "Show the configuration in effect here: ~/.lacarta/config.json, then this project's config.json.",
			Run: cmdConfig},
		{Name: "config check", Help: "Check both config files; exits 1 if anything in them is ignored.", Run: cmdConfigCheck},
		{Name: "skills", Help: "Show the skills each role loads here. They are set in the config files, under `skills`.",
			Run: cmdSkills, Opts: []Opt{{Name: "json", Kind: flag}, {Name: "role", Help: "Only this role; prints the paragraph for its prompt."}}},
		{Name: "config init", Help: "Create a config file to fill in (the home one, or with --project this project's).",
			Run: cmdConfigInit, Opts: []Opt{{Name: "project", Kind: flag}}},
	}
}

// currentState: the project this command runs in, if any.
func currentState() *core.State {
	if store == nil {
		return nil
	}
	st, err := store.Read()
	if err != nil {
		return nil
	}
	return st
}

func exists(path string) string {
	if _, err := os.Stat(path); err != nil {
		return "not created; `config init` creates it"
	}
	return "found"
}

func cmdConfig(a *Args) error {
	s := core.SettingsRead()
	st := currentState()
	a.Printf("Home config     %s (%s)\n", s.Config.Path, exists(s.Config.Path))
	var pc *core.ProjectConfig
	if st != nil {
		pc = core.ProjectConfigRead(st)
		a.Printf("Project config  %s (%s)\n", firstNonEmpty(pc.Path, "none for a task in the home folder"), exists(pc.Path))
		a.Printf("Name            %s\n", st.Project.Name)
		a.Printf("\nModels for this %s project:\n", strings.ToLower(core.KindByID(st.Template).Label))
		for _, rm := range core.ModelsFor(st) {
			a.Printf("  %-24s %-7s %s\n", rm.Role, rm.Model, source(rm.Source))
		}
		a.Println("\nSkills (`skills` says where each comes from):")
		roles := plugins.ByRole(st)
		for _, r := range roles.Keys() {
			l, _ := roles.Get(r)
			a.Printf("  %-24s %s\n", r, firstNonEmpty(skillNames(l), "none"))
		}
		remote := core.FromDefault
		if st.Remote != nil {
			remote = "the project's Chat toggle"
		} else if pc.RemoteControl != nil {
			remote = core.FromProject
		} else if s.RemoteSet() {
			remote = core.FromHome
		}
		a.Printf("\nRemote control  %v (%s)\n", core.RemoteOn(st), source(remote))
		if st.Template == core.Software {
			dirs := core.TemplateDirs(st)
			a.Printf("Spec templates  %s (used by `init`; a missing file falls back to the built-in one)\n",
				strings.Join(append(dirs, "built-in"), ", then "))
		}
	} else {
		a.Println("Project config  none: no project here")
		a.Println("\nModels and skills: `models` and `skills` list them; the home config sets them per kind of project and role.")
		a.Printf("\nRemote control  %v\n", s.RemoteControl)
		a.Printf("Spec templates  %s\n", firstNonEmpty(s.Templates, "built-in"))
	}
	a.Printf("Auditor         %s\n", firstNonEmpty(s.Auditor, "not set"))
	if st != nil && st.Template == core.Audit { // plugins only apply to audits so far
		if ps, err := plugins.ForProject(st); err == nil && len(ps) > 0 {
			a.Println("\nPlugins in this project:")
			for _, p := range ps {
				on := "off"
				if p.Enabled {
					on = "on"
				}
				from := "default"
				switch {
				case p.Need == plugins.Required:
					from = "required"
				case p.Config != "":
					from = source(p.Config)
				}
				a.Printf("  %-20s %-3s (%s)\n", p.ID, on, from)
			}
		}
	} else if st == nil && len(s.Plugins) > 0 {
		a.Println("\nPlugins turned on or off in every project (a project's config can say otherwise):")
		for id, on := range s.Plugins {
			a.Printf("  %-20s %v\n", id, on)
		}
	}
	a.Println("\nTools:")
	for _, t := range core.Tools {
		p := core.ToolPath(t.K)
		switch {
		case p == "":
			p = "not found"
		case s.Tools[t.K] != "":
			p += " (home config)"
		default:
			p += " (PATH)"
		}
		a.Printf("  %-12s %s: %s\n", t.K, p, t.V)
	}
	warns := configWarnings(s, pc)
	skillErrs, notes := plugins.SkillWarnings(st)
	if warns = append(warns, skillErrs...); len(warns) > 0 {
		a.Println("\nIgnored:")
		for _, w := range warns {
			a.Println("  " + w)
		}
	}
	if len(notes) > 0 {
		a.Println("\nTo check:")
		for _, w := range notes {
			a.Println("  " + w)
		}
	}
	return nil
}

func skillNames(skills []*plugins.AgentSkill) string {
	names := []string{}
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}

// cmdSkills shows the skills of each role: in a project, its kind's roles with the project's
// config applied; outside one, what the home config gives every kind.
func cmdSkills(a *Args) error {
	st := currentState()
	type group struct {
		kind  core.Kind
		roles core.OMap[[]*plugins.AgentSkill]
	}
	groups := []group{}
	if st != nil {
		groups = append(groups, group{core.KindByID(st.Template), plugins.ByRole(st)})
	} else {
		all := plugins.KindSkills()
		for _, k := range core.Kinds {
			roles, _ := all.Get(k.ID)
			groups = append(groups, group{k, roles})
		}
	}
	if role := a.Str("role"); role != "" {
		if st == nil {
			return core.Gate("No project here: --role shows a role of the current project.")
		}
		l, ok := groups[0].roles.Get(role)
		if !ok {
			return core.Gate("%s is not a role of %s projects. Roles: %s.", role, strings.ToLower(groups[0].kind.Label),
				strings.Join(groups[0].roles.Keys(), ", "))
		}
		var only core.OMap[[]*plugins.AgentSkill]
		only.Set(role, l)
		groups[0].roles = only
		if !a.Flag("json") {
			a.Println(firstNonEmpty(plugins.Instruction(l), "No skills are set for "+role+"."))
			return nil
		}
	}
	if a.Flag("json") {
		var out any = groups[0].roles
		if st == nil {
			var kinds core.OMap[core.OMap[[]*plugins.AgentSkill]]
			for _, g := range groups {
				kinds.Set(g.kind.ID, g.roles)
			}
			out = kinds
		}
		b, err := core.MarshalIndent(out)
		if err != nil {
			return err
		}
		a.Println(string(b))
		return nil
	}
	any := false
	for _, g := range groups {
		a.Printf("Skills for %s projects (config files):\n", strings.ToLower(g.kind.Label))
		for _, r := range g.roles.Keys() {
			l, _ := g.roles.Get(r)
			if len(l) == 0 {
				a.Printf("  %-24s none\n", r)
				continue
			}
			for i, s := range l {
				any = true
				name := ""
				if i == 0 {
					name = r
				}
				a.Printf("  %-24s %-28s %-9s %s\n", name, s.Name, s.Status, skillFrom(s))
			}
		}
		a.Println()
	}
	if any {
		a.Println("`agent start` gives the paragraph that makes a subagent load its role's skills; the session itself gets its own at start.")
	}
	a.Printf("Set them in %s: \"skills\": {\"all\": [...], \"<kind>\": {\"all\": [...], \"<role>\": [...]}}. "+
		"A project's config.json adds its own: \"skills\": {\"all\": [...], \"<role>\": [...]}; \"-name\" drops one.\n", core.HomeConfigPath())
	return nil
}

// skillFrom says which list gave a skill and where lacarta found it.
func skillFrom(s *plugins.AgentSkill) string {
	from := map[string]string{core.ScopeEveryone: "skills.all", core.ScopeKind: "the kind's all", core.ScopeRole: "the role"}[s.Scope]
	if s.Source == core.FromProject {
		from = map[string]string{core.ScopeKind: "all", core.ScopeRole: "the role"}[s.Scope] + ", this project's config.json"
	} else {
		from += ", home config"
	}
	out := "(" + from + ")"
	if s.Path != "" {
		out += " " + s.Path
	}
	if s.Note != "" {
		out += "  " + s.Note
	}
	return out
}

func source(s string) string {
	switch s {
	case core.FromHome:
		return "home config"
	case core.FromProject:
		return "project config"
	}
	return s
}

// configWarnings: what both files hold that lacarta ignores, plugin ids included.
func configWarnings(s *core.Settings, pc *core.ProjectConfig) []string {
	out := []string{}
	for _, w := range s.Config.Warnings {
		out = append(out, s.Config.Path+": "+w)
	}
	known := map[string]bool{}
	if all, err := plugins.List(); err == nil {
		for _, p := range all {
			known[p.ID] = true
		}
	}
	unknown := func(path string, ps map[string]bool) {
		for id := range ps {
			if !known[id] {
				out = append(out, path+": plugins: no plugin "+id+". `plugin list` shows them.")
			}
		}
	}
	unknown(s.Config.Path, s.Plugins)
	if pc != nil {
		for _, w := range pc.Warnings {
			out = append(out, pc.Path+": "+w)
		}
		unknown(pc.Path, pc.Plugins)
	}
	return out
}

func cmdConfigCheck(a *Args) error {
	var pc *core.ProjectConfig
	if st := currentState(); st != nil {
		pc = core.ProjectConfigRead(st)
	}
	warns := configWarnings(core.SettingsRead(), pc)
	skillErrs, notes := plugins.SkillWarnings(currentState())
	warns = append(warns, skillErrs...)
	for _, w := range notes {
		a.Println("To check: " + w)
	}
	if len(warns) == 0 {
		a.Println("The config files are valid.")
		return nil
	}
	for _, w := range warns {
		a.Println(w)
	}
	return exitCode(1)
}

const homeConfigTemplate = `{
  "//": "lacarta's settings for this machine. A project's .lacarta/config.json overrides models, remote_control, templates and plugins, and adds its own skills. ` + "`lacarta config`" + ` shows what is in effect.",
  "// models": "{\"<kind>\": {\"<role>\": \"<model>\"}}: the model of each role. ` + "`lacarta models`" + ` lists the roles.",
  "models": {},
  "// skills": "{\"all\": [...], \"<kind>\": {\"all\": [...], \"<role>\": [...]}}: the skills every agent loads, every agent of one kind of project, one role. A name is a skill of the agent CLI, plugin:skill for a plugin's, or a lacarta plugin's id. ` + "`lacarta skills`" + ` shows the result.",
  "skills": {"all": []},
  "remote_control": true,
  "auto_track": false,
  "plugins": {},
  "tools": {}
}
`

const projectConfigTemplate = `{
  "//": "lacarta's settings for this project, over ~/.lacarta/config.json. name: the project's name everywhere; models: {\"<role>\": \"<model>\"} (` + "`lacarta models`" + ` lists the roles); skills: {\"all\": [...], \"<role>\": [...]} added to the machine's, \"-name\" drops one (` + "`lacarta skills`" + `); templates: a folder in the project with your own spec templates; plugins: {\"<plugin id>\": true or false}.",
  "name": NAME,
  "models": {},
  "skills": {}
}
`

func cmdConfigInit(a *Args) error {
	path, body := core.HomeConfigPath(), homeConfigTemplate
	if !a.Flag("project") && core.LegacySettings() {
		if _, err := core.SettingsUpdate(nil, nil); err != nil { // rewrites them as config.json
			return err
		}
		a.Printf("Moved the settings of %s into %s.\n", filepath.Join(core.Home(), "settings.json"), path)
		return nil
	}
	if a.Flag("project") {
		st := currentState()
		if st == nil {
			return core.Gate("No project here: run it inside a project, or without --project for the home config.")
		}
		path = core.ProjectConfigPath(st.Template, store)
		if path == "" {
			return core.Gate("A task in the home folder has no config file: ~/.lacarta is lacarta's own.")
		}
		name, _ := json.Marshal(st.Project.Name)
		body = strings.Replace(projectConfigTemplate, "NAME", string(name), 1)
	}
	if _, err := os.Stat(path); err == nil {
		return core.Gate("%s already exists.", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	a.Printf("Created %s.\n", path)
	return nil
}
