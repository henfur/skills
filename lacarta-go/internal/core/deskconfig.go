package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A review desk's settings: the "desk" key of the desk's config.json. They are read before every
// poll, spawn and publish, so an edit takes effect at the next step. Invalid values fall back to
// their defaults with a warning, like every config key; a profile whose mode is invalid is shadow.
// Docs: docs/design/pr-review.md §3.

const (
	ModeShadow     = "shadow"
	ModeAssisted   = "assisted"
	ModeAutonomous = "autonomous"
)

var DeskModes = []string{ModeShadow, ModeAssisted, ModeAutonomous}

func modeRank(m string) int {
	for i, x := range DeskModes {
		if x == m {
			return i
		}
	}
	return 0
}

// Default profiles: their instructions and templates are built in (skill/desk/).
var DefaultProfiles = []string{"security", "devops", "code"}

type DeskConfig struct {
	Forge    DeskForge               `json:"forge"`
	Targets  []*DeskTarget           `json:"targets"`
	Filters  DeskFilters             `json:"filters"`
	Profiles map[string]*DeskProfile `json:"profiles"`
	Tools    map[string]*DeskTool    `json:"tools"`
	Policy   DeskPolicy              `json:"policy"`
	Schedule DeskSchedule            `json:"schedule"`
	Warnings []string                `json:"warnings"`
}

type DeskForge struct {
	Type        string `json:"type"`
	Host        string `json:"host"`
	Identity    string `json:"identity"`
	WriteConfig string `json:"write_config"` // absolute, checked (§5.3)
	ReadConfig  string `json:"read_config"`
}

type DeskTarget struct {
	Group            string   `json:"group,omitempty"`
	Project          string   `json:"project,omitempty"`
	IncludeSubgroups bool     `json:"include_subgroups,omitempty"`
	Exclude          []string `json:"exclude,omitempty"`
	TargetBranches   []string `json:"target_branches,omitempty"`
}

// Key names a target in the poll cursor and the console.
func (t *DeskTarget) Key() string {
	if t.Group != "" {
		return "group:" + t.Group
	}
	return "project:" + t.Project
}

type DeskFilters struct {
	SkipDrafts      bool     `json:"skip_drafts"`
	SkipLabels      []string `json:"skip_labels"`
	SkipAuthors     []string `json:"skip_authors"`
	MaxChangedLines int      `json:"max_changed_lines"`
}

type DeskTrigger struct {
	Always         bool     `json:"always,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	TargetBranches []string `json:"target_branches,omitempty"`
}

type DeskProfile struct {
	Instructions string      `json:"instructions"` // relative to the desk; "" uses the built-in one
	Skills       []string    `json:"skills"`
	Tools        []string    `json:"tools"`
	Model        string      `json:"model"`
	Trigger      DeskTrigger `json:"trigger"`
	Verify       string      `json:"verify"` // none, blocking, all
	Template     string      `json:"template"`
	Mode         string      `json:"mode"`
}

type DeskTool struct {
	Run     []string `json:"run"`
	When    string   `json:"when"`              // before, agent
	Sandbox *bool    `json:"sandbox,omitempty"` // nil: true
	Forge   string   `json:"forge,omitempty"`   // "" or "read"
}

type DeskApprove struct {
	When        string `json:"when"`
	MaxSeverity string `json:"max_severity"`
	Pipeline    string `json:"pipeline"` // success, any
	Verified    bool   `json:"verified"`
}

type DeskPolicy struct {
	Comment             string       `json:"comment"`         // none, summary, summary+inline, combined
	Approve             *DeskApprove `json:"approve"`         // nil: never
	RequestChanges      string       `json:"request_changes"` // min severity; "" never
	LabelsOnBlocking    []string     `json:"labels_on_blocking"`
	LabelsOnPass        []string     `json:"labels_on_pass"`
	ResolveFixedThreads bool         `json:"resolve_fixed_threads"`
	CommentOnSkip       bool         `json:"comment_on_skip"`
}

type DeskSchedule struct {
	Poll              string        `json:"poll"`
	PollEvery         time.Duration `json:"-"`
	QuietHours        string        `json:"quiet_hours"`
	MaxReviewsPerHour int           `json:"max_reviews_per_hour"`
	DailyBudget       int           `json:"daily_budget"`
}

var CommentPolicies = []string{"none", "summary", "summary+inline", "combined"}
var VerifyPolicies = []string{"none", "blocking", "all"}

func defaultProfile(name string) *DeskProfile {
	p := &DeskProfile{Skills: []string{}, Tools: []string{}, Verify: "none", Template: name, Mode: ModeShadow,
		Trigger: DeskTrigger{Always: true}}
	switch name {
	case "security":
		p.Model, p.Verify = "opus", "blocking"
	case "devops":
		p.Model = "sonnet"
		p.Trigger = DeskTrigger{Paths: []string{"Dockerfile*", "**/Dockerfile*", "**/*.tf", "**/*.tfvars", "helm/**",
			"charts/**", "k8s/**", "deploy/**", ".gitlab-ci.yml", "**/.gitlab-ci.yml", "**/docker-compose*.yml",
			"**/*.nix", "ansible/**"}}
	default:
		p.Model = "sonnet"
	}
	return p
}

func defaultDeskConfig() *DeskConfig {
	c := &DeskConfig{
		Forge:    DeskForge{Type: "gitlab"},
		Targets:  []*DeskTarget{},
		Filters:  DeskFilters{SkipDrafts: true, SkipLabels: []string{"no-ai-review"}, SkipAuthors: []string{}, MaxChangedLines: 2000},
		Profiles: map[string]*DeskProfile{},
		Tools:    map[string]*DeskTool{},
		Policy: DeskPolicy{Comment: "summary+inline", RequestChanges: "high", ResolveFixedThreads: true,
			Approve:          &DeskApprove{When: "all-profiles-pass", MaxSeverity: "low", Pipeline: "success", Verified: true},
			LabelsOnBlocking: []string{}, LabelsOnPass: []string{}},
		Schedule: DeskSchedule{Poll: "5m", PollEvery: 5 * time.Minute, MaxReviewsPerHour: 20, DailyBudget: 200},
		Warnings: []string{},
	}
	for _, p := range DefaultProfiles {
		c.Profiles[p] = defaultProfile(p)
	}
	return c
}

// ProfileNames in a stable order: the default ones first, then the others by name.
func (c *DeskConfig) ProfileNames() []string {
	out := []string{}
	for _, p := range DefaultProfiles {
		if c.Profiles[p] != nil {
			out = append(out, p)
		}
	}
	extra := []string{}
	for p := range c.Profiles {
		if !contains(DefaultProfiles, p) {
			extra = append(extra, p)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

var (
	deskNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	gitlabPath      = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`)
	hostPattern     = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]+)?$`)
)

// ConfigHome: the config folder of the environment lacarta runs in ($XDG_CONFIG_HOME, else
// $HOME/.config). In a container it is the container's own, never the host's home it mounts.
func ConfigHome() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) {
		return filepath.Clean(x)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// hostMounts: where container tools mount the host's file system. Credentials never come from there.
var hostMounts = []string{"/run/host", "/var/run/host", "/run/flatpak"}

// CheckCredentialDir validates a glab config folder named by a desk (§5.3, D-7): inside this
// environment's config home after resolving symlinks, and never on a host mount.
func CheckCredentialDir(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("not set")
	}
	abs := expandPath(p, ConfigHome())
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%s doesn't exist (create it with `GLAB_CONFIG_DIR=%s glab auth login`)", abs, abs)
	}
	ch, err := filepath.EvalSymlinks(ConfigHome())
	if err != nil {
		ch = ConfigHome()
	}
	for _, h := range hostMounts {
		if real == h || strings.HasPrefix(real, h+"/") {
			return "", fmt.Errorf("%s is on the host's file system (%s): use a folder of this environment's own", abs, real)
		}
	}
	rel, err := filepath.Rel(ch, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s is outside this environment's config folder %s", abs, ch)
	}
	if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a folder", abs)
	}
	return real, nil
}

// DeskConfigRead reads the desk settings of a desk's config.json.
func DeskConfigRead(st *State) *DeskConfig {
	c := defaultDeskConfig()
	path := projectConfigPath(st)
	raw, _, warns := readConfig(path)
	c.Warnings = append(c.Warnings, warns...)
	v, ok := raw["desk"]
	if !ok {
		c.Warnings = append(c.Warnings, "desk: missing; the defaults apply and no target is watched.")
		return c
	}
	parseDesk(c, v)
	return c
}

func parseDesk(c *DeskConfig, v json.RawMessage) {
	warn := func(format string, a ...any) { c.Warnings = append(c.Warnings, fmt.Sprintf(format, a...)) }
	var d map[string]json.RawMessage
	if json.Unmarshal(v, &d) != nil {
		warn("desk: expected an object.")
		return
	}
	for k := range d {
		if strings.HasPrefix(k, "//") {
			delete(d, k)
		}
	}
	for _, key := range sortedKeys(d) {
		v := d[key]
		switch key {
		case "forge":
			parseForge(c, v, warn)
		case "targets":
			var ts []*DeskTarget
			if json.Unmarshal(v, &ts) != nil {
				warn("desk.targets: expected [{\"group\": \"...\"} or {\"project\": \"...\"}].")
				continue
			}
			for i, t := range ts {
				switch {
				case t == nil || (t.Group == "") == (t.Project == ""):
					warn("desk.targets[%d]: set either group or project.", i)
				case !gitlabPath.MatchString(t.Group + t.Project):
					warn("desk.targets[%d]: %q is not a group or project path.", i, t.Group+t.Project)
				default:
					c.Targets = append(c.Targets, t)
				}
			}
		case "filters":
			f := c.Filters
			if json.Unmarshal(v, &f) != nil || f.MaxChangedLines < 0 {
				warn("desk.filters: expected {skip_drafts, skip_labels, skip_authors, max_changed_lines}.")
				continue
			}
			c.Filters = f
		case "profiles":
			parseProfiles(c, v, warn)
		case "tools":
			var ts map[string]*DeskTool
			if json.Unmarshal(v, &ts) != nil {
				warn("desk.tools: expected {\"<name>\": {\"run\": [...], \"when\": \"before\"|\"agent\"}}.")
				continue
			}
			for _, n := range sortedKeys(ts) {
				t := ts[n]
				switch {
				case !deskNamePattern.MatchString(n):
					warn("desk.tools: %q is not a tool name (lowercase letters, digits, - and _).", n)
				case t == nil || len(t.Run) == 0:
					warn("desk.tools.%s: run is missing.", n)
				case t.When != "before" && t.When != "agent":
					warn("desk.tools.%s: when is \"before\" or \"agent\".", n)
				case t.Forge != "" && t.Forge != "read":
					warn("desk.tools.%s: forge can only be \"read\".", n)
				case strings.HasPrefix(t.Run[0], "/") || strings.Contains(t.Run[0], ".."):
					warn("desk.tools.%s: run[0] is a file inside the desk's tools/ folder.", n)
				default:
					c.Tools[n] = t
				}
			}
		case "policy":
			parsePolicy(c, v, warn)
		case "schedule":
			s := c.Schedule
			if json.Unmarshal(v, &s) != nil {
				warn("desk.schedule: expected {poll, quiet_hours, max_reviews_per_hour, daily_budget}.")
				continue
			}
			dur, err := time.ParseDuration(s.Poll)
			if err != nil || dur < time.Minute {
				warn("desk.schedule.poll: expected a duration of at least 1m, like \"5m\".")
				s.Poll, dur = c.Schedule.Poll, c.Schedule.PollEvery
			}
			s.PollEvery = dur
			if s.QuietHours != "" {
				if _, _, ok := parseQuiet(s.QuietHours); !ok {
					warn("desk.schedule.quiet_hours: expected \"HH:MM-HH:MM\".")
					s.QuietHours = ""
				}
			}
			if s.MaxReviewsPerHour < 0 || s.DailyBudget < 0 {
				warn("desk.schedule: limits are 0 (none) or more.")
				s.MaxReviewsPerHour, s.DailyBudget = c.Schedule.MaxReviewsPerHour, c.Schedule.DailyBudget
			}
			c.Schedule = s
		default:
			warn("desk: unknown setting %q. A desk sets: forge, targets, filters, profiles, tools, policy, schedule.", key)
		}
	}
	for _, p := range c.ProfileNames() {
		for _, t := range c.Profiles[p].Tools {
			if c.Tools[t] == nil || c.Tools[t].When != "agent" && c.Tools[t].When != "before" {
				warn("desk.profiles.%s.tools: no tool %q in desk.tools.", p, t)
			}
		}
	}
}

// Sandboxed: a desk tool runs in the sandbox unless its entry says "sandbox": false.
func (t *DeskTool) Sandboxed() bool { return t.Sandbox == nil || *t.Sandbox }

func parseForge(c *DeskConfig, v json.RawMessage, warn func(string, ...any)) {
	f := c.Forge
	if json.Unmarshal(v, &f) != nil {
		warn("desk.forge: expected {type, host, identity, write_config, read_config}.")
		return
	}
	if f.Type != "gitlab" {
		warn("desk.forge.type: only \"gitlab\" for now.")
		f.Type = "gitlab"
	}
	if f.Host != "" && !hostPattern.MatchString(f.Host) {
		warn("desk.forge.host: %q is not a host name.", f.Host)
		f.Host = ""
	}
	for _, x := range []struct {
		name string
		val  *string
	}{{"write_config", &f.WriteConfig}, {"read_config", &f.ReadConfig}} {
		if *x.val == "" {
			continue
		}
		real, err := CheckCredentialDir(*x.val)
		if err != nil {
			warn("desk.forge.%s: %v.", x.name, err)
			*x.val = ""
			continue
		}
		*x.val = real
	}
	if f.WriteConfig != "" && f.WriteConfig == f.ReadConfig {
		warn("desk.forge: read_config and write_config must be two folders: agents only get the read one.")
		f.ReadConfig = ""
	}
	c.Forge = f
}

func parseProfiles(c *DeskConfig, v json.RawMessage, warn func(string, ...any)) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(v, &raw) != nil {
		warn("desk.profiles: expected {\"<name>\": {...}}.")
		return
	}
	c.Profiles = map[string]*DeskProfile{}
	for _, n := range sortedKeys(raw) {
		if !deskNamePattern.MatchString(n) {
			warn("desk.profiles: %q is not a profile name (lowercase letters, digits, - and _).", n)
			continue
		}
		p := defaultProfile(n)
		var keys map[string]json.RawMessage
		if json.Unmarshal(raw[n], &keys) != nil || json.Unmarshal(raw[n], p) != nil {
			warn("desk.profiles.%s: expected an object.", n)
			continue
		}
		if _, ok := keys["trigger"]; ok {
			var t DeskTrigger
			_ = json.Unmarshal(keys["trigger"], &t)
			p.Trigger = t
		}
		if !contains(DeskModes, p.Mode) {
			warn("desk.profiles.%s.mode: %q is not a mode (%s); it stays shadow.", n, p.Mode, join(DeskModes, ", "))
			p.Mode = ModeShadow
		}
		if !contains(VerifyPolicies, p.Verify) {
			warn("desk.profiles.%s.verify: expected %s.", n, join(VerifyPolicies, ", "))
			p.Verify = defaultProfile(n).Verify
		}
		if _, ok := ModelByAlias(p.Model); !ok {
			warn("desk.profiles.%s.model: unknown model %q. Models: %s.", n, p.Model, modelAliases())
			p.Model = defaultProfile(n).Model
		}
		if p.Template != "" && !deskNamePattern.MatchString(p.Template) {
			warn("desk.profiles.%s.template: expected a template name.", n)
			p.Template = n
		}
		if p.Template == "" {
			p.Template = n
		}
		if p.Instructions != "" && (filepath.IsAbs(p.Instructions) || strings.Contains(p.Instructions, "..")) {
			warn("desk.profiles.%s.instructions: expected a file inside the desk.", n)
			p.Instructions = ""
		}
		for _, s := range p.Skills {
			if filepath.IsAbs(s) || strings.Contains(s, "..") {
				warn("desk.profiles.%s.skills: %q must be a folder inside the desk.", n, s)
			}
		}
		if !p.Trigger.Always && len(p.Trigger.Paths)+len(p.Trigger.Labels)+len(p.Trigger.TargetBranches) == 0 {
			warn("desk.profiles.%s.trigger: nothing triggers it; set always, paths, labels or target_branches.", n)
		}
		if p.Skills == nil {
			p.Skills = []string{}
		}
		if p.Tools == nil {
			p.Tools = []string{}
		}
		c.Profiles[n] = p
	}
	if len(c.Profiles) == 0 {
		warn("desk.profiles: no valid profile; the default ones apply.")
		for _, p := range DefaultProfiles {
			c.Profiles[p] = defaultProfile(p)
		}
	}
}

func parsePolicy(c *DeskConfig, v json.RawMessage, warn func(string, ...any)) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(v, &raw) != nil {
		warn("desk.policy: expected an object.")
		return
	}
	p := &c.Policy
	for _, k := range sortedKeys(raw) {
		v := raw[k]
		switch k {
		case "comment":
			var s string
			if json.Unmarshal(v, &s) != nil || !contains(CommentPolicies, s) {
				warn("desk.policy.comment: expected %s.", join(CommentPolicies, ", "))
				continue
			}
			p.Comment = s
		case "approve":
			var s string
			if json.Unmarshal(v, &s) == nil {
				if s != "never" {
					warn("desk.policy.approve: \"never\" or {when, max_severity, pipeline, verified}.")
					continue
				}
				p.Approve = nil
				continue
			}
			a := *p.approveOrDefault()
			if json.Unmarshal(v, &a) != nil || a.When != "all-profiles-pass" || !contains(Severities, a.MaxSeverity) ||
				(a.Pipeline != "success" && a.Pipeline != "any") {
				warn("desk.policy.approve: expected {\"when\": \"all-profiles-pass\", \"max_severity\": \"%s\", "+
					"\"pipeline\": \"success\"|\"any\", \"verified\": true|false} or \"never\".", join(Severities, "|"))
				continue
			}
			p.Approve = &a
		case "request_changes":
			var s string
			if json.Unmarshal(v, &s) == nil && s == "never" {
				p.RequestChanges = ""
				continue
			}
			var o struct {
				MinSeverity string `json:"min_severity"`
			}
			if json.Unmarshal(v, &o) != nil || !contains(Severities, o.MinSeverity) {
				warn("desk.policy.request_changes: \"never\" or {\"min_severity\": \"%s\"}.", join(Severities, "|"))
				continue
			}
			p.RequestChanges = o.MinSeverity
		case "labels":
			var o struct {
				Blocking []string `json:"add_on_blocking"`
				Pass     []string `json:"add_on_pass"`
			}
			if json.Unmarshal(v, &o) != nil {
				warn("desk.policy.labels: expected {add_on_blocking: [...], add_on_pass: [...]}.")
				continue
			}
			p.LabelsOnBlocking, p.LabelsOnPass = nonNil(o.Blocking), nonNil(o.Pass)
		case "resolve_fixed_threads", "comment_on_skip":
			var b bool
			if json.Unmarshal(v, &b) != nil {
				warn("desk.policy.%s: expected true or false.", k)
				continue
			}
			if k == "comment_on_skip" {
				p.CommentOnSkip = b
			} else {
				p.ResolveFixedThreads = b
			}
		default:
			warn("desk.policy: unknown setting %q.", k)
		}
	}
}

func (p *DeskPolicy) approveOrDefault() *DeskApprove {
	if p.Approve != nil {
		return p.Approve
	}
	return &DeskApprove{When: "all-profiles-pass", MaxSeverity: "low", Pipeline: "success", Verified: true}
}

// parseQuiet reads "HH:MM-HH:MM" into minutes of the day.
func parseQuiet(s string) (from, to int, ok bool) {
	var h1, m1, h2, m2 int
	if n, _ := fmt.Sscanf(s, "%d:%d-%d:%d", &h1, &m1, &h2, &m2); n != 4 || h1 > 23 || h2 > 23 || m1 > 59 || m2 > 59 {
		return 0, 0, false
	}
	return h1*60 + m1, h2*60 + m2, true
}

// InQuietHours: no review starts in the desk's quiet hours (local time).
func (s *DeskSchedule) InQuietHours(t time.Time) bool {
	from, to, ok := parseQuiet(s.QuietHours)
	if !ok || from == to {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	if from < to {
		return m >= from && m < to
	}
	return m >= from || m < to
}

// SetDeskMode writes a profile's mode into the desk's config.json, keeping every other key.
func SetDeskMode(st *State, profile, mode string) error {
	if !contains(DeskModes, mode) {
		return Gate("Modes: %s.", join(DeskModes, ", "))
	}
	path := projectConfigPath(st)
	raw, _, _ := readConfig(path)
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	var desk map[string]any
	if raw["desk"] == nil || json.Unmarshal(raw["desk"], &desk) != nil {
		return Gate("%s has no valid \"desk\" key: fix it first.", path)
	}
	profiles, _ := desk["profiles"].(map[string]any)
	if profiles == nil {
		profiles = map[string]any{}
		for _, p := range DefaultProfiles {
			profiles[p] = map[string]any{}
		}
	}
	p, ok := profiles[profile].(map[string]any)
	if !ok {
		names := []string{}
		for n := range profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		return Gate("No profile %s. Profiles: %s.", profile, join(names, ", "))
	}
	p["mode"] = mode
	profiles[profile] = p
	desk["profiles"] = profiles
	return editConfigFile(path, path, map[string]any{"desk": desk})
}

// IsDefaultProfile: one of the profiles lacarta ships instructions for.
func IsDefaultProfile(p string) bool { return contains(DefaultProfiles, p) }
