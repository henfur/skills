package cli

import (
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
	if st != nil && st.Dir != "" {
		pc = core.ProjectConfigRead(st)
		a.Printf("Project config  %s (%s)\n", pc.Path, exists(pc.Path))
		a.Printf("\nModels for this %s project:\n", strings.ToLower(core.KindByID(st.Template).Label))
		for _, rm := range core.ModelsFor(st) {
			a.Printf("  %-24s %-7s %s\n", rm.Role, rm.Model, source(rm.Source))
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
		a.Println("\nModels: `models` lists them inside a project; the home config sets them per kind of project.")
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
				from := "the console"
				switch {
				case p.Config != "":
					from = source(p.Config)
				case p.Need == plugins.Required:
					from = "required"
				case st.Plugins[p.ID] == nil:
					from = "default"
				}
				a.Printf("  %-20s %-3s (%s)\n", p.ID, on, from)
			}
		}
	} else if st == nil && len(s.Plugins) > 0 {
		a.Println("\nPlugins enabled by default in new projects:")
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
	if len(warns) > 0 {
		a.Println("\nIgnored:")
		for _, w := range warns {
			a.Println("  " + w)
		}
	}
	return nil
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
	if st := currentState(); st != nil && st.Dir != "" {
		pc = core.ProjectConfigRead(st)
	}
	warns := configWarnings(core.SettingsRead(), pc)
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
  "//": "lacarta's settings for this machine. A project's .lacarta/config.json overrides models, remote_control, templates and plugins. ` + "`lacarta config`" + ` shows what is in effect.",
  "models": {},
  "remote_control": true,
  "plugins": {},
  "tools": {}
}
`

const projectConfigTemplate = `{
  "//": "lacarta's settings for this project, over ~/.lacarta/config.json. models: {\"<role>\": \"<model>\"} (` + "`lacarta models`" + ` lists the roles); templates: a folder in the project with your own spec templates; plugins: {\"<plugin id>\": true or false}.",
  "models": {}
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
		if st == nil || st.Dir == "" {
			return core.Gate("No project here: run it inside a project, or without --project for the home config.")
		}
		path, body = filepath.Join(st.Dir, core.ConfigFile), projectConfigTemplate
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
