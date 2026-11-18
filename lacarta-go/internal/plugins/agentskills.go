package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lacarta/internal/core"
)

// How an agent gets a skill, and what lacarta found when it looked for it.
const (
	HowTool = "tool" // the agent CLI's Skill tool
	HowRead = "read" // files to read: a lacarta plugin outside the CLI's skill folders

	SkillFound     = "found"
	SkillUnchecked = "unchecked" // a skill of an agent CLI plugin: lacarta can't see those
	SkillMissing   = "missing"   // not found; the agent is still told to load it
	SkillUnusable  = "unusable"  // never handed to an agent
)

// AgentSkill is a skill the config gives a role, resolved on this machine.
type AgentSkill struct {
	core.RoleSkill
	How    string `json:"how"`
	Status string `json:"status"`
	Path   string `json:"path,omitempty"`
	Type   string `json:"type,omitempty"` // of a lacarta plugin: skill or companion
	Note   string `json:"note,omitempty"`
}

// Resolve finds each skill: in the agent CLI's skill folders, in the project's own .claude/skills,
// then among lacarta's plugins. root is the project's folder ("" for none).
func Resolve(skills []core.RoleSkill, root string) []*AgentSkill {
	out := []*AgentSkill{}
	if len(skills) == 0 {
		return out
	}
	found := Discover()
	known := map[string]*Info{}
	if all, err := List(); err == nil {
		for _, i := range all {
			known[i.ID] = i
		}
	}
	for _, s := range skills {
		a := &AgentSkill{RoleSkill: s, How: HowTool, Status: SkillFound}
		out = append(out, a)
		if strings.Contains(s.Name, ":") {
			a.Status, a.Note = SkillUnchecked, "A skill of an agent CLI plugin: lacarta can't check it is installed."
			continue
		}
		if p, ok := found[s.Name]; ok {
			a.Path = p
			continue
		}
		if root != "" {
			p := filepath.Join(core.LocalRoot(root), ".claude", "skills", s.Name)
			if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err == nil {
				a.Path = p
				continue
			}
		}
		i := known[s.Name]
		switch {
		case i == nil:
			a.Status = SkillMissing
			a.Note = "Not found in " + strings.Join(SkillDirs(), ", ") + " nor among lacarta's plugins. The agent is still told to load it."
		case i.Type == TypeCheck:
			a.Status, a.Note = SkillUnusable, "A check lacarta runs, not a skill: turn it on or off under `plugins`."
		case !i.Installed || i.Problem != "":
			a.Status, a.Type = SkillUnusable, i.Type
			a.Note = strings.TrimSpace(firstNonEmpty(i.Problem, "Not installed.") + " " + i.InstallHint)
		default:
			a.How, a.Path, a.Type = HowRead, i.Path, i.Type
		}
	}
	return out
}

// ForRole: the skills a role loads in a project.
func ForRole(st *core.State, role string) []*AgentSkill {
	return Resolve(core.SkillsFor(st, role), st.Root)
}

// ForSession: the skills of the session itself.
func ForSession(st *core.State) []*AgentSkill { return ForRole(st, core.SessionRole(st)) }

// ByRole lists every role of the project's kind with its skills.
func ByRole(st *core.State) core.OMap[[]*AgentSkill] {
	var out core.OMap[[]*AgentSkill]
	for _, r := range core.KindByID(st.Template).Roles {
		out.Set(r.ID, ForRole(st, r.ID))
	}
	return out
}

// KindSkills: what the home config gives every role of every kind, for the console's Settings.
func KindSkills() core.OMap[core.OMap[[]*AgentSkill]] {
	s := core.SettingsRead()
	var out core.OMap[core.OMap[[]*AgentSkill]]
	for _, k := range core.Kinds {
		var roles core.OMap[[]*AgentSkill]
		for _, r := range k.Roles {
			roles.Set(r.ID, Resolve(s.SkillsOf(k.ID, r.ID), ""))
		}
		out.Set(k.ID, roles)
	}
	return out
}

// Instruction is the paragraph that makes an agent load its skills: it goes in a session's
// context and in a subagent's prompt. "" when the role has none.
func Instruction(skills []*AgentSkill) string {
	var tool, read []string
	for _, s := range skills {
		switch {
		case s.Status == SkillUnusable:
		case s.How == HowTool:
			tool = append(tool, "`"+s.Name+"`")
		case s.Type == TypeCompanion:
			read = append(read, fmt.Sprintf("read the files in %s (`%s`)", s.Path, s.Name))
		default:
			read = append(read, fmt.Sprintf("read %s and follow it (`%s`)", filepath.Join(s.Path, "SKILL.md"), s.Name))
		}
	}
	if len(tool)+len(read) == 0 {
		return ""
	}
	parts := []string{}
	if len(tool) > 0 {
		parts = append(parts, "load "+strings.Join(tool, ", ")+" with the Skill tool")
	}
	parts = append(parts, read...)
	return "Skills set for you in lacarta's config: before you start, " + strings.Join(parts, "; ") +
		". Follow them in your work. Where one conflicts with lacarta's rules for your role (what you may write, run, post or " +
		"approve), lacarta's rules win. If one can't be loaded, say so in your result and carry on."
}

// SkillWarnings: what `config check` says about the skills the config files name. Only an unusable
// one is an error; a missing one is a note, since the agent CLI may know skills lacarta can't see.
func SkillWarnings(st *core.State) (errs, notes []string) {
	seen := map[string]bool{}
	add := func(where string, skills []*AgentSkill) {
		for _, s := range skills {
			if seen[s.Name] {
				continue
			}
			switch s.Status {
			case SkillUnusable:
				seen[s.Name] = true
				errs = append(errs, fmt.Sprintf("skills: %s (%s) is not given to agents. %s", s.Name, where, s.Note))
			case SkillMissing:
				seen[s.Name] = true
				notes = append(notes, fmt.Sprintf("skills: %s (%s): %s", s.Name, where, s.Note))
			}
		}
	}
	if st != nil {
		roles := ByRole(st)
		for _, r := range roles.Keys() {
			l, _ := roles.Get(r)
			add(r, l)
		}
		return
	}
	kinds := KindSkills()
	for _, k := range kinds.Keys() {
		roles, _ := kinds.Get(k)
		for _, r := range roles.Keys() {
			l, _ := roles.Get(r)
			add(k+"."+r, l)
		}
	}
	return
}
