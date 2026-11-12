package core

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// State is one project's state file. Field names are the contract shared with the Python
// prototype (schema version 1), so its projects and tasks load unchanged.
type State struct {
	Version   int            `json:"version"`
	Template  string         `json:"template"`
	Project   Project        `json:"project"`
	Stage     string         `json:"stage"`
	Specs     OMap[*Spec]    `json:"specs"` // in the order of core.Specs; the prototype lists them in file order
	Plan      Plan           `json:"plan"`
	Phases    []*Phase       `json:"phases"`
	Reviews   []*Review      `json:"reviews"`
	Questions []*Question    `json:"questions"`
	Decisions []*Decision    `json:"decisions"`
	Findings  []*Finding     `json:"findings"`
	Activity  []*Activity    `json:"activity"`
	Counters  map[string]int `json:"counters"`
	Agents    []*Agent       `json:"agents"`
	Inbox     []*InboxMsg    `json:"inbox"`
	Remote    *bool          `json:"remote"` // nil: follow the machine-wide default
	Listening *Listening     `json:"listening"`
	Attention *Attention     `json:"attention"`
	MovedTo   string         `json:"moved_to,omitempty"`
}

// Spec returns one spec's state (nil for a single-agent task).
func (st *State) Spec(k string) *Spec {
	s, _ := st.Specs.Get(k)
	return s
}

type Project struct {
	Name      string `json:"name"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
	Cwd       string `json:"cwd,omitempty"`
	Named     bool   `json:"named,omitempty"`
}

type Spec struct {
	Title            string  `json:"title"`
	Status           string  `json:"status"`
	Revision         int     `json:"revision"`
	ApprovedAt       *string `json:"approved_at"`
	ApprovedRevision *int    `json:"approved_revision"`
	SubmittedHash    string  `json:"submitted_hash,omitempty"`
}

type Plan struct {
	Status        string  `json:"status,omitempty"`
	ApprovedAt    *string `json:"approved_at,omitempty"`
	SubmittedHash string  `json:"submitted_hash,omitempty"`
}

type Phase struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Goal        string   `json:"goal"`
	Scope       []string `json:"scope"`
	Covers      []string `json:"covers"`
	Status      string   `json:"status"`
	StartedAt   *string  `json:"started_at"`
	CompletedAt *string  `json:"completed_at"`
}

type Review struct {
	ID        string   `json:"id"`
	Phase     PhaseRef `json:"phase"`
	Type      string   `json:"type"`
	Verdict   string   `json:"verdict"`
	Summary   string   `json:"summary"`
	Reviewer  string   `json:"reviewer"`
	Agent     *string  `json:"agent"`
	Report    string   `json:"report"`
	At        string   `json:"at"`
	SpecsHash string   `json:"specs_hash,omitempty"`
}

type Question struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	Options    []string          `json:"options"`
	OptionHelp map[string]string `json:"option_help"`
	AllowFree  bool              `json:"allow_free"`
	Context    Context           `json:"context"`
	Effect     *Effect           `json:"effect"`
	Status     string            `json:"status"`
	AskedAt    string            `json:"asked_at"`
	Answer     *Answer           `json:"answer"`
	AskedBy    string            `json:"asked_by"`
	Thread     []*Msg            `json:"thread"`
}

type Context struct {
	Stage   string    `json:"stage,omitempty"`
	Spec    string    `json:"spec,omitempty"`
	Phase   *PhaseRef `json:"phase,omitempty"`
	Finding string    `json:"finding,omitempty"`
}

// Effect ties a question to the framework transition its answer triggers.
type Effect struct {
	Type    string    `json:"type"`
	Spec    string    `json:"spec,omitempty"`
	Phase   *PhaseRef `json:"phase,omitempty"`
	Finding string    `json:"finding,omitempty"`
}

func (e *Effect) Equal(o *Effect) bool {
	if e == nil || o == nil {
		return e == o
	}
	return e.Type == o.Type && e.Spec == o.Spec && e.Finding == o.Finding && e.Phase.Equal(o.Phase)
}

type Answer struct {
	Choice  string `json:"choice"`
	Comment string `json:"comment"`
	By      string `json:"by"`
	At      string `json:"at"`
}

type Msg struct {
	By   string `json:"by"`
	Text string `json:"text"`
	At   string `json:"at"`
}

type Decision struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Decision  string    `json:"decision"`
	Rationale string    `json:"rationale"`
	Source    *string   `json:"source"`
	By        string    `json:"by"`
	Agent     *string   `json:"agent,omitempty"`
	Spec      *string   `json:"spec"`
	Phase     *PhaseRef `json:"phase"`
	At        string    `json:"at"`
}

type Finding struct {
	ID             string   `json:"id"`
	Phase          PhaseRef `json:"phase"`
	Review         string   `json:"review"`
	Severity       string   `json:"severity"`
	Title          string   `json:"title"`
	Detail         string   `json:"detail"`
	Location       string   `json:"location"`
	Scenario       string   `json:"scenario"`
	Recommendation string   `json:"recommendation"`
	Status         string   `json:"status"`
	Resolution     string   `json:"resolution"`
	At             string   `json:"at"`
	ResolvedAt     *string  `json:"resolved_at"`
	FoundBy        *string  `json:"found_by"`
}

type Activity struct {
	At    string  `json:"at"`
	Kind  string  `json:"kind"`
	Text  string  `json:"text"`
	Agent *string `json:"agent"`
}

type Agent struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Role       string          `json:"role"`
	Model      string          `json:"model"`
	Task       string          `json:"task"`
	Phase      *PhaseRef       `json:"phase"`
	Status     string          `json:"status"`
	Summary    string          `json:"summary"`
	Avatar     json.RawMessage `json:"avatar"` // reserved for a custom picture; the console shows initials
	StartedAt  string          `json:"started_at"`
	UpdatedAt  string          `json:"updated_at"`
	FinishedAt *string         `json:"finished_at"`
	Session    *Session        `json:"session,omitempty"`
}

type Session struct {
	CLI        string  `json:"cli"`
	ID         string  `json:"id"`
	Transcript string  `json:"transcript"`
	Cwd        string  `json:"cwd"`
	StartedAt  string  `json:"started_at"`
	EndedAt    *string `json:"ended_at"`
}

type InboxMsg struct {
	ID          string  `json:"id"`
	Text        string  `json:"text"`
	At          string  `json:"at"`
	DeliveredAt *string `json:"delivered_at"`
}

// Listening is the agent's background listener (`lacarta listen`). Since is when the agent's
// last turn ended (the console announces each one); PID is the listener's process.
type Listening struct {
	Since string `json:"since"`
	PID   int    `json:"pid"`
}

type Attention struct {
	Text string `json:"text"`
	At   string `json:"at"`
}

// PhaseRef is a phase number, or "spec" or "final".
type PhaseRef struct {
	Num  int
	Name string
}

func ParsePhase(v string) (PhaseRef, error) {
	if v == "spec" || v == "final" {
		return PhaseRef{Name: v}, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return PhaseRef{}, fmt.Errorf("invalid phase %q: a number, 'spec' or 'final'", v)
	}
	return PhaseRef{Num: n}, nil
}

func NumPhase(n int) *PhaseRef { return &PhaseRef{Num: n} }

func (p PhaseRef) IsNum() bool { return p.Name == "" }

func (p *PhaseRef) Equal(o *PhaseRef) bool {
	if p == nil || o == nil {
		return p == o
	}
	return *p == *o
}

// String is the phase as Python prints it: 3, spec, final.
func (p PhaseRef) String() string {
	if p.Name != "" {
		return p.Name
	}
	return strconv.Itoa(p.Num)
}

func (p PhaseRef) MarshalJSON() ([]byte, error) {
	if p.Name != "" {
		return json.Marshal(p.Name)
	}
	return json.Marshal(p.Num)
}

func (p *PhaseRef) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		r, err := ParsePhase(s)
		*p = r
		return err
	}
	return json.Unmarshal(b, &p.Num)
}

// Where names a phase in prose.
func Where(p PhaseRef) string {
	switch p.Name {
	case "spec":
		return "the specification"
	case "final":
		return "the final review"
	}
	return fmt.Sprintf("phase %d", p.Num)
}

func ptr[T any](v T) *T { return &v }
