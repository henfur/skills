package core

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

// Skills an agent loads, by role. Like the models, they are set in the config files and nowhere
// else: ~/.lacarta/config.json for the machine, a project's config.json over it.
//
//	"skills": {
//	  "all": ["house-style"],                       every agent of every kind of project
//	  "software": {
//	    "all": ["go-conventions"],                  every agent of a software project
//	    "security_review": ["threat-modeling"]      one role, as in `models`
//	  }
//	}
//
// A project's file has one level less, for its own kind: {"all": [...], "<role>": [...]}.
// Lists add up, widest first. An entry "-name" in a narrower list drops a skill a wider one added.
//
// An entry is the name of a skill the agent CLI knows (`name`, or `plugin:skill` for a skill of
// one of its plugins) or the id of a lacarta plugin (`plugin add`). A config file never gives a
// path: a project's file can come from a cloned repository, and must not choose which files steer
// an agent.

const SkillsAll = "all"

// Which list gave a skill to a role.
const (
	ScopeEveryone = "everyone" // skills.all of the home config
	ScopeKind     = "kind"     // the kind's or the project's "all"
	ScopeRole     = "role"
)

// Names go into agents' prompts, so they can hold nothing but a name.
var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}(:[A-Za-z0-9][A-Za-z0-9._-]{0,63})?$`)

const maxSkillsPerList = 32

// HomeSkills is the home config's `skills`.
type HomeSkills struct {
	All   []string
	Kinds map[string]map[string][]string // kind -> role or "all" -> names
}

// RoleSkill is a skill a role loads, and the list that gave it.
type RoleSkill struct {
	Name   string `json:"name"`
	Source string `json:"source"` // FromHome or FromProject
	Scope  string `json:"scope"`
}

// skillList checks one list. What it can't hold is skipped with a warning.
func skillList(v json.RawMessage, where string, warn func(string, ...any)) []string {
	var names []string
	if json.Unmarshal(v, &names) != nil {
		warn("%s: expected a list of skill names.", where)
		return nil
	}
	out := []string{}
	for _, n := range names {
		if !skillNamePattern.MatchString(strings.TrimPrefix(n, "-")) || strings.Contains(n, "..") {
			warn("%s: %q is not a skill name. Give the skill's name, `plugin:skill` for a plugin's, or -name to drop one.", where, n)
			continue
		}
		if len(out) == maxSkillsPerList {
			warn("%s: more than %d skills, the rest is ignored.", where, maxSkillsPerList)
			break
		}
		out = append(out, n)
	}
	return out
}

// roleSkillLists reads {"all": [...], "<role>": [...]} for one kind.
func roleSkillLists(v json.RawMessage, kind Kind, where string, warn func(string, ...any)) map[string][]string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(v, &raw) != nil {
		warn("%s: expected {\"all\": [...], \"<role>\": [...]}.", where)
		return nil
	}
	out := map[string][]string{}
	for _, r := range sortedKeys(raw) {
		if _, ok := kind.Role(r); !ok && r != SkillsAll {
			warn("%s: no role %q in %s projects. Roles: %s, or all.", where, r, strings.ToLower(kind.Label), join(roleIDs(kind), ", "))
			continue
		}
		if l := skillList(raw[r], where+"."+r, warn); len(l) > 0 {
			out[r] = l
		}
	}
	return out
}

func roleIDs(k Kind) []string {
	ids := []string{}
	for _, r := range k.Roles {
		ids = append(ids, r.ID)
	}
	return ids
}

// homeSkills reads the home config's `skills`.
func homeSkills(v json.RawMessage, warn func(string, ...any)) HomeSkills {
	hs := HomeSkills{Kinds: map[string]map[string][]string{}}
	var raw map[string]json.RawMessage
	if json.Unmarshal(v, &raw) != nil {
		warn("skills: expected {\"all\": [...], \"<kind>\": {\"all\": [...], \"<role>\": [...]}}.")
		return hs
	}
	for _, k := range sortedKeys(raw) {
		if k == SkillsAll {
			hs.All = skillList(raw[k], "skills.all", warn)
			continue
		}
		kind := KindByID(k)
		if kind.ID != k {
			warn("skills: no kind of project %q. Kinds: %s, or all.", k, kindIDs())
			continue
		}
		if l := roleSkillLists(raw[k], kind, "skills."+k, warn); len(l) > 0 {
			hs.Kinds[k] = l
		}
	}
	return hs
}

// addSkills applies one list over the wider ones.
func addSkills(out []RoleSkill, names []string, source, scope string) []RoleSkill {
	for _, n := range names {
		if drop, ok := strings.CutPrefix(n, "-"); ok {
			out = slices.DeleteFunc(out, func(x RoleSkill) bool { return x.Name == drop })
		} else if !slices.ContainsFunc(out, func(x RoleSkill) bool { return x.Name == n }) {
			out = append(out, RoleSkill{Name: n, Source: source, Scope: scope})
		}
	}
	return out
}

// SkillsOf: the skills the home config gives a role of a kind of project.
func (s *Settings) SkillsOf(kind, role string) []RoleSkill {
	out := addSkills([]RoleSkill{}, s.Skills.All, FromHome, ScopeEveryone)
	out = addSkills(out, s.Skills.Kinds[kind][SkillsAll], FromHome, ScopeKind)
	return addSkills(out, s.Skills.Kinds[kind][role], FromHome, ScopeRole)
}

// SkillsFor: the skills a role loads in a project, the project's config over the machine's.
func SkillsFor(st *State, role string) []RoleSkill {
	out := SettingsRead().SkillsOf(st.Template, role)
	pc := ProjectConfigRead(st)
	out = addSkills(out, pc.Skills[SkillsAll], FromProject, ScopeKind)
	return addSkills(out, pc.Skills[role], FromProject, ScopeRole)
}

// SessionRole is the role of the session itself: the first of its kind.
func SessionRole(st *State) string { return KindByID(st.Template).Roles[0].ID }
