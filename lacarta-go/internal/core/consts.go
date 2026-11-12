// Package core holds lacarta's state, its rules and the kinds of project. The CLI, the hooks
// and the daemon all go through it: there is one implementation of the gates.
package core

const (
	StateDir     = ".lacarta"
	DefaultPort  = 4747
	Orchestrator = "A-1"

	Single   = "single"
	Software = "software"

	SpecReview = "specification" // the fresh-agent review of the whole spec set

	Approve        = "Approve"
	RequestChanges = "Request changes"
	TriageFix      = "Fix it now"
	TriageRedesign = "Change the spec or design"
	TriageAccept   = "Accept the risk"
	TriageStop     = "Stop the project"
	UseModels      = "Use these models" // answer of the setup question older projects still carry
)

// Specs in display order: key, title.
var Specs = []KV{
	{"functional", "Functional"},
	{"architecture", "Architecture & design"},
	{"stack", "Tech stack"},
	{"security", "Security & threat model"},
}

func SpecTitle(k string) string {
	for _, s := range Specs {
		if s.K == k {
			return s.V
		}
	}
	return k
}

func IsSpec(k string) bool {
	for _, s := range Specs {
		if s.K == k {
			return true
		}
	}
	return false
}

type KV struct{ K, V string }

var (
	Severities     = []string{"critical", "high", "medium", "low", "info"}
	Blocking       = map[string]bool{"critical": true, "high": true}
	ReviewTypes    = []string{"functional", "security"} // every phase and the final review
	AllReviewTypes = []string{"functional", "security", SpecReview}
	// A finding in one of these states still needs work.
	Unresolved = map[string]bool{"open": true, "fixing": true, "redesign": true}
)

// Model choices map to the Agent tool's `model` aliases, which resolve to these releases.
type Model struct {
	Alias string `json:"-"`
	Name  string `json:"name"`
	ID    string `json:"id"`
}

var Models = []Model{
	{"opus", "Opus 5.5", "claude-opus-5-5"},
	{"sonnet", "Sonnet 5.5", "claude-sonnet-5-5"},
	{"haiku", "Haiku 5.5", "claude-haiku-5-5"},
	{"fable", "Fable 5.1", "claude-fable-5-1"},
}

func ModelByAlias(a string) (Model, bool) {
	for _, m := range Models {
		if m.Alias == a {
			return m, true
		}
	}
	return Model{}, false
}

func ModelName(a string) string {
	if m, ok := ModelByAlias(a); ok {
		return m.Name
	}
	return a
}

// Role: label, framework stage, what it does, default model.
type Role struct {
	ID, Label, Stage, About, Default string
}

var SoftwareRoles = []Role{
	{"orchestrator", "Orchestrator", "Whole project",
		"Runs the framework, writes the specs, talks to you and coordinates the other agents. " +
			"This is the Claude Code session itself: switch it with /model.", "opus"},
	{"spec_review", "Specification review", "Specification",
		"Fresh agent that checks the complete spec set for gaps and incoherences.", "sonnet"},
	{"implementation", "Implementation", "Implementation",
		"Subagent that builds each phase's scope and its tests.", "opus"},
	{"functional_review", "Functional review", "Implementation",
		"Reviews each phase against the functional spec and architecture.", "sonnet"},
	{"security_review", "Security review", "Implementation",
		"Reviews each phase against the threat model and hunts for vulnerabilities.", "opus"},
	{"final_functional_review", "Final functional review", "Final review",
		"Fresh agent that judges the whole repository against the specs.", "sonnet"},
	{"final_security_review", "Final security review", "Final review",
		"Fresh agent that audits the whole repository.", "opus"},
}

// Kinds of project. Each has its own console view and its own roles; the models for every
// role are set once, machine-wide, on the console's Settings page.
type Kind struct {
	ID, Label, About string
	Roles            []Role
}

var Kinds = []Kind{
	{Single, "Single agent", "One agent session on one task: its chat, questions and activity.",
		[]Role{{"agent", "Agent", "Session", "The CLI session itself. Switch it in the terminal " +
			"(`/model` in Claude Code); helpers it spawns use this model too.", "opus"}}},
	{Software, "Software development",
		"Specs, a phase plan, phases gated by functional and security reviews, a final review.",
		SoftwareRoles},
}

var PlannedKinds = []string{"Security audit", "DevOps (Ansible, Terraform)"}

func KindByID(id string) Kind {
	for _, k := range Kinds {
		if k.ID == id {
			return k
		}
	}
	return Kinds[0]
}

func (k Kind) Role(id string) (Role, bool) {
	for _, r := range k.Roles {
		if r.ID == id {
			return r, true
		}
	}
	return Role{}, false
}

// Agents are named after Totally Spies characters so the user knows who is asking. The first
// name goes to the orchestrator, A-1, so it is always Jerry. The console draws a portrait for
// each of these names (web/app/src/lib/portraits.js); keep the two lists in step.
var AgentNames = []string{"Jerry", "Sam", "Clover", "Alex", "Britney", "Blaine", "Mandy", "Arnold", "Dean",
	"Mike", "Gladis", "Caitlin", "Dominique", "Tim", "Terrence", "Helga", "Geraldine", "Gus"}

// What each fixed choice does, shown under the option in the console.
var ApprovalHelp = map[string]string{
	Approve:        "Accept it as it is. The framework moves on to the next step.",
	RequestChanges: "Send it back. Say what should change in the comment; the agent revises it and asks again.",
}

var TriageHelp = map[string]string{
	TriageFix: "It is a bug in the code. The agent fixes only this finding, adds a test that proves the fix, " +
		"and a fresh review checks it. The specs stay as they are. Work resumes once the fix is verified.",
	TriageRedesign: "Patching the code is not enough: the weakness comes from what was specified or designed. " +
		"The agent first updates the specs (asking you any choice that needs), then fixes the code, " +
		"and a fresh review checks it.",
	TriageAccept: "Leave it unfixed and carry on. Your reason is recorded in the decision log and in the accepted " +
		"risks of the security spec, where the final reviewers will see it. A written reason is required.",
	TriageStop: "Freeze the project as it is now. Nothing advances any more. Pick this when the finding puts " +
		"the whole approach in question.",
}

// What a severity means, per review type, so a triage question can be judged on its own.
var SeverityMeaning = map[string]map[string]string{
	"security": {
		"critical": "exploitable remotely with no or trivial preconditions; full compromise, code execution, " +
			"broad data exposure or leaked production secrets.",
		"high": "serious impact with a realistic precondition (an authenticated user, a specific configuration): " +
			"authorisation bypass, injection, account takeover, sensitive data exposure.",
	},
	"functional": {
		"critical": "data loss or corruption, or the phase's core goal does not work at all.",
		"high":     "a must requirement is unmet or wrong in a common case.",
	},
}
