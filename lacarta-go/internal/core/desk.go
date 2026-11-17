package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The review desk kind watches merge requests on a forge and reviews them on a loop: one
// orchestrator session, one subagent per profile. Agents read the forge with a read-only identity
// and write review records; lacarta checks the records, decides what to do from the desk's policy
// and the profiles' modes, and posts with the desk's write identity. It never merges.
// Docs: docs/design/pr-review.md.

const Desk = "desk"

var DeskRoles = []Role{
	{"orchestrator", "Orchestrator", "Whole desk",
		"Keeps `desk next` running, spawns one reviewer per profile for each merge request, then hands the records " +
			"to lacarta. This is the Claude Code session itself: switch it with /model.", "sonnet"},
	{"reviewer", "Reviewer", "Review",
		"Fresh subagent that reviews one merge request for one profile and writes its record. The profile's model, " +
			"when the desk sets one, wins over this.", "sonnet"},
	{"verifier", "Verifier", "Review",
		"Fresh subagent that tries to disprove a blocking finding before it can block. Use a different model from the " +
			"reviewers, so they check each other.", "opus"},
}

// Item statuses.
const (
	ItemQueued     = "queued"
	ItemReviewing  = "reviewing"
	ItemDrafted    = "drafted"    // a plan waits for a human in the console
	ItemPublishing = "publishing" // approved: lacarta posts the steps
	ItemPublished  = "published"
	ItemSkipped    = "skipped"
	ItemSuperseded = "superseded" // a newer head came before the review ended
	ItemDropped    = "dropped"    // closed, merged or back to draft before publishing
	ItemDiscarded  = "discarded"
)

type DeskState struct {
	Paused bool                              `json:"paused"`
	Hash   string                            `json:"hash"` // the desk content the user accepted
	Seen   string                            `json:"seen"` // the content seen at the last check
	Poll   DeskPoll                          `json:"poll"`
	Items  OMap[*DeskItem]                   `json:"items"` // by "<project>!<iid>"
	Posted map[string]map[string]*DeskPosted `json:"posted"`
	Stats  map[string]*DeskStats             `json:"stats"`
	Budget DeskBudget                        `json:"budget"`
	Active string                            `json:"active"` // the item being reviewed
}

type DeskPoll struct {
	Last   string            `json:"last"`
	Cursor map[string]string `json:"cursor"`
	Error  string            `json:"error"`
	Errors int               `json:"errors"`
}

type DeskBudget struct {
	Date     string `json:"date"`
	Used     int    `json:"used"`
	Hour     string `json:"hour"`
	HourUsed int    `json:"hour_used"`
}

type DeskStats struct {
	Published int `json:"published"`
	Edited    int `json:"edited"`
	Discarded int `json:"discarded"`
}

// DeskItem is one merge request the desk tracks; its review is for HeadSHA.
type DeskItem struct {
	Key             string            `json:"key"`
	Project         string            `json:"project"`
	ProjectID       int               `json:"project_id"`
	IID             int               `json:"iid"`
	Title           string            `json:"title"`
	Author          string            `json:"author"`
	URL             string            `json:"url"`
	SourceBranch    string            `json:"source_branch"`
	TargetBranch    string            `json:"target_branch"`
	Labels          []string          `json:"labels"`
	HeadSHA         string            `json:"head_sha"`
	BaseSHA         string            `json:"base_sha"`
	StartSHA        string            `json:"start_sha"`
	LastReviewedSHA string            `json:"last_reviewed_sha"`
	Status          string            `json:"status"`
	Reason          string            `json:"reason,omitempty"`
	Profiles        map[string]string `json:"profiles"` // profile -> pending, submitted
	Pipeline        string            `json:"pipeline"`
	Approved        bool              `json:"approved"`                // the desk's approval is on the MR
	RecordHashes    map[string]string `json:"record_hashes,omitempty"` // profile -> sha256 of the record lacarta stored
	PlanHash        string            `json:"plan_hash,omitempty"`     // sha256 of the plan lacarta last wrote
	Question        string            `json:"question,omitempty"`
	QueuedAt        string            `json:"queued_at"`
	UpdatedAt       string            `json:"updated_at"`
}

func ItemKey(project string, iid int) string { return fmt.Sprintf("%s!%d", project, iid) }

// DeskPosted: a finding the desk posted on a merge request.
type DeskPosted struct {
	Profile    string `json:"profile"`
	Title      string `json:"title"`
	Severity   string `json:"severity"`
	SHA        string `json:"sha"`
	NoteID     int    `json:"note_id,omitempty"`
	Discussion string `json:"discussion,omitempty"`
	State      string `json:"state"` // open, fixed
}

// DeskHome holds a desk's records, drafts and scratch clones, outside the desk folder.
func DeskHome(pid string) string { return filepath.Join(Home(), "reviews", pid) }

// ItemDir: one merge request at one head.
func ItemDir(pid string, it *DeskItem) string {
	return filepath.Join(DeskHome(pid), "items", Slug(it.Project), fmt.Sprint(it.IID), shortSHA(it.HeadSHA))
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

func DeskItemGet(st *State, key string) (*DeskItem, error) {
	if st.Desk == nil {
		return nil, Gate("This is not a review desk.")
	}
	it, ok := st.Desk.Items.Get(key)
	if !ok {
		return nil, Gate("No merge request %s on this desk. `desk status` lists them.", key)
	}
	return it, nil
}

// ------------------------------------------------------------------ records

type DeskRecord struct {
	Profile           string          `json:"profile"`
	MR                DeskRecordMR    `json:"mr"`
	Status            string          `json:"status"` // complete, incomplete
	Reason            string          `json:"reason,omitempty"`
	Verdict           string          `json:"verdict"` // pass, comments, changes
	Summary           string          `json:"summary"`
	Findings          []*DeskFinding  `json:"findings"`
	Notes             []string        `json:"notes"`
	InjectionAttempts []DeskInjection `json:"injection_attempts"`
	Rendered          string          `json:"rendered,omitempty"`
	Agent             string          `json:"agent,omitempty"` // set by lacarta
	Model             string          `json:"model,omitempty"`
	SubmittedAt       string          `json:"submitted_at,omitempty"`
}

type DeskRecordMR struct {
	Project      string `json:"project"`
	IID          int    `json:"iid"`
	HeadSHA      string `json:"head_sha"`
	BaseSHA      string `json:"base_sha,omitempty"`
	ReviewedFrom string `json:"reviewed_from,omitempty"`
}

type DeskInjection struct {
	Where string `json:"where"`
	What  string `json:"what"`
}

type DeskFinding struct {
	ID          string            `json:"id"`
	Fingerprint string            `json:"fingerprint"`
	Class       string            `json:"class"`
	Severity    string            `json:"severity"`
	Title       string            `json:"title"`
	File        string            `json:"file,omitempty"`
	Line        int               `json:"line,omitempty"`
	EndLine     int               `json:"end_line,omitempty"`
	Table       string            `json:"table"`
	Body        string            `json:"body"`
	Fix         string            `json:"fix"`
	Suggestion  string            `json:"suggestion,omitempty"`
	Confidence  string            `json:"confidence"` // confirmed, conditional, theoretical
	Verified    *DeskVerification `json:"verified,omitempty"`
}

type DeskVerification struct {
	Verdict  string `json:"verdict"` // confirmed, downgraded, rejected
	Severity string `json:"severity,omitempty"`
	Note     string `json:"note"`
	Agent    string `json:"agent,omitempty"`
	Model    string `json:"model,omitempty"`
	At       string `json:"at"`
}

var (
	RecordVerdicts      = []string{"pass", "comments", "changes"}
	FindingConfidences  = []string{"confirmed", "conditional", "theoretical"}
	VerificationVerdict = []string{"confirmed", "downgraded", "rejected"}
	fingerprintPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:/#@+-]{2,199}$`)
	// lacarta's own comment marker: agents may not write it (it would fake the desk's bookkeeping).
	markerPattern = regexp.MustCompile(`(?i)<!--\s*lacarta`)
)

// Effective is the finding's severity after verification; "" when the verifier rejected it.
func (f *DeskFinding) Effective() string {
	if f.Verified == nil {
		return f.Severity
	}
	switch f.Verified.Verdict {
	case "rejected":
		return ""
	case "downgraded":
		return f.Verified.Severity
	}
	return f.Severity
}

func sevRank(s string) int { // critical 0 ... info 4; unknown last
	for i, x := range Severities {
		if x == s {
			return i
		}
	}
	return len(Severities)
}

// AtLeast: severity a is as serious as b or more.
func AtLeast(a, b string) bool { return a != "" && sevRank(a) <= sevRank(b) }

// ValidateRecord checks a record against its item and the desk's limits. It normalises
// fingerprints (profile prefix) and fills empty slices.
func ValidateRecord(r *DeskRecord, it *DeskItem, profile string) error {
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	if r.Profile != profile {
		bad("profile is %q, expected %q", r.Profile, profile)
	}
	if r.MR.Project != it.Project || r.MR.IID != it.IID {
		bad("mr is %s!%d, expected %s", r.MR.Project, r.MR.IID, it.Key)
	}
	if r.MR.HeadSHA != it.HeadSHA {
		bad("mr.head_sha is %q, expected the item's head %s (a new push supersedes this review)", r.MR.HeadSHA, it.HeadSHA)
	}
	if r.Status != "complete" && r.Status != "incomplete" {
		bad("status is complete or incomplete")
	}
	if r.Status == "incomplete" && strings.TrimSpace(r.Reason) == "" {
		bad("an incomplete record says why in reason")
	}
	if !contains(RecordVerdicts, r.Verdict) {
		bad("verdict is one of %s", join(RecordVerdicts, ", "))
	}
	if strings.TrimSpace(r.Summary) == "" || len(r.Summary) > 4000 {
		bad("summary: one to a few sentences (at most 4000 characters)")
	}
	if len(r.Findings) > 50 {
		bad("at most 50 findings in one record")
	}
	if len(r.Rendered) > 60_000 {
		bad("rendered: at most 60000 characters")
	}
	if r.Findings == nil {
		r.Findings = []*DeskFinding{}
	}
	if r.Notes == nil {
		r.Notes = []string{}
	}
	if r.InjectionAttempts == nil {
		r.InjectionAttempts = []DeskInjection{}
	}
	ids, fps := map[string]bool{}, map[string]bool{}
	for i, f := range r.Findings {
		w := fmt.Sprintf("findings[%d]", i)
		if f == nil {
			bad("%s is empty", w)
			continue
		}
		if f.ID == "" || ids[f.ID] {
			bad("%s.id: set a unique id (F-1, F-2, ...)", w)
		}
		ids[f.ID] = true
		fp := strings.ToLower(strings.TrimSpace(f.Fingerprint))
		if !strings.HasPrefix(fp, profile+":") {
			fp = profile + ":" + fp
		}
		if !fingerprintPattern.MatchString(fp) {
			bad("%s.fingerprint: class:file:symbol in lowercase, no spaces (got %q)", w, f.Fingerprint)
		} else if fps[fp] {
			bad("%s.fingerprint: %s appears twice", w, fp)
		}
		f.Fingerprint, fps[fp] = fp, true
		if !contains(Severities, f.Severity) {
			bad("%s.severity is one of %s", w, join(Severities, ", "))
		}
		if !contains(FindingConfidences, f.Confidence) {
			bad("%s.confidence is one of %s", w, join(FindingConfidences, ", "))
		}
		if strings.TrimSpace(f.Title) == "" || len(f.Title) > 200 {
			bad("%s.title: one line, at most 200 characters", w)
		}
		if strings.TrimSpace(f.Body) == "" || len(f.Body) > 20_000 || len(f.Table) > 1000 || len(f.Fix) > 20_000 ||
			len(f.Suggestion) > 10_000 {
			bad("%s: body is required; body and fix at most 20000 characters, table 1000, suggestion 10000", w)
		}
		if f.File != "" && (strings.HasPrefix(f.File, "/") || strings.Contains(f.File, "..")) {
			bad("%s.file: a path relative to the repository", w)
		}
		if f.Line < 0 || f.EndLine < 0 || (f.EndLine > 0 && f.EndLine < f.Line) {
			bad("%s: line and end_line are positive, end_line after line", w)
		}
		if f.Verified != nil {
			bad("%s.verified: set by `desk verify`, not in the record", w)
		}
	}
	if r.Verdict == "changes" && len(r.Findings) == 0 {
		bad("verdict changes needs at least one finding")
	}
	if markerPattern.MatchString(recordText(r)) {
		bad("the record contains lacarta's comment marker (<!-- lacarta...): remove it")
	}
	if len(errs) > 0 {
		return Gate("The record is not valid:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

func recordText(r *DeskRecord) string {
	parts := []string{r.Summary, r.Rendered, r.Reason}
	parts = append(parts, r.Notes...)
	for _, f := range r.Findings {
		if f != nil {
			parts = append(parts, f.Title, f.Body, f.Fix, f.Table, f.Suggestion, f.File, f.Class)
		}
	}
	for _, i := range r.InjectionAttempts {
		parts = append(parts, i.Where, i.What)
	}
	return strings.Join(parts, "\n")
}

// NeedsVerification: the findings of a record that a verifier must see before the plan.
func NeedsVerification(r *DeskRecord, verify string) []*DeskFinding {
	out := []*DeskFinding{}
	for _, f := range r.Findings {
		if f.Verified != nil {
			continue
		}
		if verify == "all" || (verify == "blocking" && AtLeast(f.Severity, "high")) {
			out = append(out, f)
		}
	}
	return out
}

// ------------------------------------------------------------------ plan

// A plan is what lacarta will do on the forge for one merge request at one head.
type DeskPlan struct {
	Item     string      `json:"item"`
	SHA      string      `json:"sha"`
	Verdict  string      `json:"verdict"` // pass, comments, changes (merged)
	Reasons  []string    `json:"reasons"`
	Steps    []*PlanStep `json:"steps"`
	Profiles []string    `json:"profiles"`
	Fixed    []string    `json:"fixed"` // fingerprints posted before and gone now
	Open     []string    `json:"open"`  // posted before and still there
	New      []string    `json:"new"`
}

type PlanStep struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"` // note, inline, resolve, approve, unapprove, labels
	Template    string   `json:"template,omitempty"`
	Profile     string   `json:"profile,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	File        string   `json:"file,omitempty"`
	Line        int      `json:"line,omitempty"`
	Discussion  string   `json:"discussion,omitempty"`
	Body        string   `json:"body,omitempty"`
	Edited      bool     `json:"edited,omitempty"`
	Add         []string `json:"add,omitempty"`
	Remove      []string `json:"remove,omitempty"`
	Auto        bool     `json:"auto"`
	Why         string   `json:"why"`
	Status      string   `json:"status"` // pending, done, failed, skipped
	Result      string   `json:"result,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type PlanInput struct {
	Cfg      *DeskConfig
	Item     *DeskItem
	Records  map[string]*DeskRecord
	Posted   map[string]*DeskPosted // this merge request's posted findings
	Trusted  bool                   // the desk content is the one the user accepted
	Paused   bool
	Identity string
}

// BuildPlan applies the hard rules, the policy and the modes. Pure: no IO.
func BuildPlan(in PlanInput) *DeskPlan {
	cfg, it := in.Cfg, in.Item
	p := &DeskPlan{Item: it.Key, SHA: it.HeadSHA, Reasons: []string{}, Steps: []*PlanStep{}, Fixed: []string{},
		Open: []string{}, New: []string{}}
	profiles := sortedKeys(in.Records)
	p.Profiles = profiles
	reason := func(format string, a ...any) { p.Reasons = append(p.Reasons, fmt.Sprintf(format, a...)) }
	add := func(s *PlanStep) {
		s.ID, s.Status = fmt.Sprintf("S-%d", len(p.Steps)+1), "pending"
		p.Steps = append(p.Steps, s)
	}
	mode := func(prof string) string {
		if pr := cfg.Profiles[prof]; pr != nil {
			return pr.Mode
		}
		return ModeShadow
	}
	lowest := ModeAutonomous
	for _, prof := range profiles {
		if modeRank(mode(prof)) < modeRank(lowest) {
			lowest = mode(prof)
		}
	}
	gate := func(need string, m string) (bool, string) {
		switch {
		case in.Paused:
			return false, "the desk is paused"
		case !in.Trusted:
			return false, "the desk changed since you accepted it"
		case modeRank(m) < modeRank(need):
			return false, "mode " + m
		}
		return true, "mode " + m
	}

	// Effective findings, merged across profiles.
	incomplete, injections := []string{}, 0
	worst, blocking, unverified := "", []string{}, []string{}
	current := map[string]*DeskFinding{}
	findingProfile := map[string]string{}
	threshold := cfg.Policy.RequestChanges
	if threshold == "" {
		threshold = "high"
	}
	for _, prof := range profiles {
		r := in.Records[prof]
		if r.Status != "complete" {
			incomplete = append(incomplete, prof)
		}
		injections += len(r.InjectionAttempts)
		verify := "none"
		if pr := cfg.Profiles[prof]; pr != nil {
			verify = pr.Verify
		}
		for _, f := range r.Findings {
			sev := f.Effective()
			if sev == "" {
				continue
			}
			current[f.Fingerprint], findingProfile[f.Fingerprint] = f, prof
			if worst == "" || AtLeast(sev, worst) {
				worst = sev
			}
			if AtLeast(sev, threshold) {
				blocking = append(blocking, f.Fingerprint)
			}
			if f.Verified == nil && (verify == "all" || (verify == "blocking" && AtLeast(f.Severity, "high"))) {
				unverified = append(unverified, f.Fingerprint)
			}
		}
	}
	for _, fp := range sortedKeys(current) {
		if po := in.Posted[fp]; po != nil && po.State == "open" {
			p.Open = append(p.Open, fp)
		} else {
			p.New = append(p.New, fp)
		}
	}
	for _, fp := range sortedKeys(in.Posted) {
		po := in.Posted[fp]
		if po.State == "open" && current[fp] == nil && in.Records[po.Profile] != nil && in.Records[po.Profile].Status == "complete" {
			p.Fixed = append(p.Fixed, fp)
		}
	}
	switch {
	case len(blocking) > 0:
		p.Verdict = "changes"
	case len(current) > 0:
		p.Verdict = "comments"
	default:
		p.Verdict = "pass"
	}

	// Comments.
	comment := cfg.Policy.Comment
	if comment == "combined" {
		auto, why := gate(ModeAssisted, lowest)
		add(&PlanStep{Kind: "note", Template: "combined.summary", Auto: auto, Why: "one note for every profile; " + why})
	} else if comment != "none" {
		for _, prof := range profiles {
			auto, why := gate(ModeAssisted, mode(prof))
			tmpl := "summary"
			if it.LastReviewedSHA != "" && !profileHasNew(p.New, findingProfile, prof) && in.Records[prof].Status == "complete" {
				tmpl = "update"
			}
			add(&PlanStep{Kind: "note", Profile: prof, Template: tmpl, Auto: auto, Why: why})
		}
	}
	if comment == "summary+inline" {
		for _, fp := range p.New {
			f := current[fp]
			if f.File == "" || f.Line == 0 {
				continue // goes in the summary only
			}
			prof := findingProfile[fp]
			auto, why := gate(ModeAssisted, mode(prof))
			add(&PlanStep{Kind: "inline", Profile: prof, Template: "inline", Fingerprint: fp, File: f.File, Line: f.Line,
				Auto: auto, Why: why})
		}
	}
	if cfg.Policy.ResolveFixedThreads {
		for _, fp := range p.Fixed {
			po := in.Posted[fp]
			if po.Discussion == "" {
				continue
			}
			auto, why := gate(ModeAssisted, mode(po.Profile))
			add(&PlanStep{Kind: "resolve", Profile: po.Profile, Fingerprint: fp, Discussion: po.Discussion, Auto: auto,
				Why: "fixed at this head; " + why})
		}
	}

	// Approval: every hard rule, then the policy. The mode decides only who does it.
	approveWhy := []string{}
	if cfg.Policy.Approve == nil {
		approveWhy = append(approveWhy, "the policy never approves")
	}
	if in.Identity != "" && strings.EqualFold(it.Author, in.Identity) {
		approveWhy = append(approveWhy, "the desk's identity authored this merge request")
	}
	if len(incomplete) > 0 {
		approveWhy = append(approveWhy, "incomplete review: "+join(incomplete, ", "))
	}
	if injections > 0 {
		approveWhy = append(approveWhy, fmt.Sprintf("%d injection attempt(s) reported", injections))
	}
	if len(blocking) > 0 {
		approveWhy = append(approveWhy, fmt.Sprintf("%d blocking finding(s)", len(blocking)))
	}
	if len(profiles) == 0 {
		approveWhy = append(approveWhy, "no profile reviewed it")
	}
	if a := cfg.Policy.Approve; a != nil {
		for _, prof := range profiles {
			if in.Records[prof].Verdict == "changes" {
				approveWhy = append(approveWhy, prof+" asks for changes")
			}
		}
		if worst != "" && !AtLeast(a.MaxSeverity, worst) {
			approveWhy = append(approveWhy, fmt.Sprintf("a %s finding is above the policy's %s", worst, a.MaxSeverity))
		}
		if a.Pipeline == "success" && it.Pipeline != "success" {
			approveWhy = append(approveWhy, fmt.Sprintf("pipeline is %q, the policy needs success", firstOf(it.Pipeline, "none")))
		}
		if a.Verified && len(unverified) > 0 {
			approveWhy = append(approveWhy, fmt.Sprintf("%d finding(s) not verified yet", len(unverified)))
		}
	}
	auto, why := gate(ModeAutonomous, lowest)
	if len(approveWhy) == 0 {
		reason("Approve: %d profile(s) pass, worst severity %s, pipeline %s.", len(profiles), firstOf(worst, "none"),
			firstOf(it.Pipeline, "none"))
		if !it.Approved {
			add(&PlanStep{Kind: "approve", Auto: auto, Why: why})
		}
	} else {
		reason("No approval: %s.", strings.Join(approveWhy, "; "))
		if it.Approved && (len(blocking) > 0 || len(incomplete) > 0 || injections > 0) {
			add(&PlanStep{Kind: "unapprove", Auto: auto, Why: "withdraw the desk's earlier approval; " + why})
		}
	}
	if len(blocking) > 0 && cfg.Policy.RequestChanges != "" {
		reason("Request changes: %d finding(s) at %s or above.", len(blocking), threshold)
	}

	// Labels.
	var addL, remL []string
	if len(blocking) > 0 {
		addL, remL = cfg.Policy.LabelsOnBlocking, cfg.Policy.LabelsOnPass
	} else if len(approveWhy) == 0 {
		addL, remL = cfg.Policy.LabelsOnPass, cfg.Policy.LabelsOnBlocking
	}
	if len(addL)+len(remL) > 0 {
		add(&PlanStep{Kind: "labels", Add: nonNil(addL), Remove: nonNil(remL), Auto: auto, Why: why})
	}
	return p
}

func profileHasNew(newFPs []string, owner map[string]string, prof string) bool {
	for _, fp := range newFPs {
		if owner[fp] == prof {
			return true
		}
	}
	return false
}

// NeedsHuman: steps that wait for the user in the console.
func (p *DeskPlan) NeedsHuman() []*PlanStep {
	out := []*PlanStep{}
	for _, s := range p.Steps {
		if s.Status == "pending" && !s.Auto {
			out = append(out, s)
		}
	}
	return out
}

// ------------------------------------------------------------------ files

func ReadPlan(dir string) (*DeskPlan, error) {
	var p DeskPlan
	ok, err := readJSONFile(filepath.Join(dir, "plan.json"), &p)
	if !ok {
		return nil, err
	}
	return &p, err
}

func ReadRecords(dir string) (map[string]*DeskRecord, error) {
	out := map[string]*DeskRecord{}
	entries, _ := os.ReadDir(filepath.Join(dir, "records"))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var r DeskRecord
		if _, err := readJSONFile(filepath.Join(dir, "records", e.Name()), &r); err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = &r
	}
	return out, nil
}

// ------------------------------------------------------------------ desk content

// DeskHash covers everything that steers the agents or decides what is posted: the desk key of
// config.json, instructions/, skills/, tools/, templates/ and the desk's .claude/ settings.
func DeskHash(root string) string {
	h := sha256.New()
	cfg := filepath.Join(root, StateDir, ConfigFile)
	raw, _, _ := readConfig(cfg)
	if d := raw["desk"]; d != nil {
		var v any
		if json.Unmarshal(d, &v) == nil {
			b, _ := json.Marshal(stripComments(v)) // map keys come out sorted
			fmt.Fprintf(h, "config\x00%s\x00", b)
		}
	}
	for _, dir := range []string{"instructions", "skills", "tools", "templates", ".claude"} {
		base := filepath.Join(root, dir)
		files := []string{}
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && !strings.HasSuffix(p, ".lock") {
				files = append(files, p)
			}
			return nil
		})
		sort.Strings(files)
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			rel, _ := filepath.Rel(root, f)
			sum := sha256.Sum256(b)
			fmt.Fprintf(h, "%s\x00%x\x00", rel, sum)
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func stripComments(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			if !strings.HasPrefix(k, "//") {
				out[k] = stripComments(val)
			}
		}
		return out
	case []any:
		for i := range x {
			x[i] = stripComments(x[i])
		}
	}
	return v
}

// DeskCheckContent compares the desk's content with the accepted one and asks the user when it
// changed. Until they accept it, nothing is published on its own (BuildPlan's Trusted).
func DeskCheckContent(st *State) {
	h := DeskHash(st.Root)
	d := st.Desk
	d.Seen = h
	if allShadow(DeskConfigRead(st)) {
		d.Hash = h // with every profile in shadow nothing is posted on its own: nothing to accept
	}
	if h == d.Hash {
		Supersede(st, &Effect{Type: "desk-content"}, "the desk is back to the accepted content.")
		return
	}
	if q := OpenQuestion(st, &Effect{Type: "desk-content"}); q != nil {
		if q.Effect.Item == h {
			return
		}
		Supersede(st, &Effect{Type: "desk-content"}, "the desk changed again.")
	}
	body := "The desk's settings, instructions, skills, tools, templates or `.claude/` settings changed since you " +
		"accepted them (an edit, or a pulled commit). They decide what the agents do and what is posted under the " +
		"desk's identity.\n\nReviews go on; **nothing is posted on its own until you accept this version**. Check the " +
		"desk's history (`git -C " + st.Root + " log -p`) or the Desk view, then choose."
	_, _ = AddQuestion(st, "validation", "Use the desk's new content?", body, []string{"Use this version", "Keep it paused"},
		&Effect{Type: "desk-content", Item: h}, Context{Stage: "desk"}, false, map[string]string{
			"Use this version": "Publishing follows the profiles' modes again, with the new content.",
			"Keep it paused":   "Every plan waits for you in the console until the desk changes again.",
		})
}

func allShadow(c *DeskConfig) bool {
	for _, p := range c.Profiles {
		if p.Mode != ModeShadow {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------------ kind rules

func NewDeskState() *DeskState {
	return &DeskState{Poll: DeskPoll{Cursor: map[string]string{}}, Posted: map[string]map[string]*DeskPosted{},
		Stats: map[string]*DeskStats{}}
}

func migrateDesk(d *DeskState) {
	if d.Poll.Cursor == nil {
		d.Poll.Cursor = map[string]string{}
	}
	if d.Posted == nil {
		d.Posted = map[string]map[string]*DeskPosted{}
	}
	if d.Stats == nil {
		d.Stats = map[string]*DeskStats{}
	}
}

func applyDeskEffect(st *State, q *Question) {
	d := st.Desk
	switch q.Effect.Type {
	case "desk-content":
		if q.Answer.Choice == "Use this version" {
			d.Hash = q.Effect.Item
			Log(st, "Desk content accepted: publishing follows the profiles' modes.", "stage")
		} else {
			Log(st, "Desk content not accepted: every plan waits for you.", "stage")
		}
	case "desk-publish":
		it, ok := d.Items.Get(q.Effect.Item)
		if !ok || it.Status != ItemDrafted {
			return
		}
		if q.Answer.Choice == "Publish" {
			it.Status, it.UpdatedAt = ItemPublishing, Now()
			Log(st, fmt.Sprintf("You approved the review of %s: lacarta publishes it.", it.Key), "stage")
		} else {
			it.Status, it.UpdatedAt = ItemDiscarded, Now()
			it.LastReviewedSHA = it.HeadSHA
			Log(st, fmt.Sprintf("You discarded the review of %s.", it.Key), "stage")
		}
	}
}

// DeskNext is the `Next:` line for the orchestrator.
func DeskNext(st *State) string {
	d := st.Desk
	if d.Active != "" {
		if it, ok := d.Items.Get(d.Active); ok && it.Status == ItemReviewing {
			pending := []string{}
			for _, p := range sortedKeys(it.Profiles) {
				if it.Profiles[p] == "pending" {
					pending = append(pending, p)
				}
			}
			if len(pending) > 0 {
				return fmt.Sprintf("Review %s at %s: spawn one reviewer per pending profile (%s), each briefed with "+
					"`desk brief <profile> --item %s`; each submits its record with `desk submit`.",
					it.Key, shortSHA(it.HeadSHA), join(pending, ", "), it.Key)
			}
			return fmt.Sprintf("Every profile submitted for %s: verify what `desk done %s` asks for, then run it.", it.Key, it.Key)
		}
	}
	if d.Paused {
		return "The desk is paused: it reviews nothing new until the user resumes it. Keep `desk next` running."
	}
	return "Keep `desk next` running as a background command: it returns the next merge request to review."
}

type DeskDerived struct {
	Paused  bool                  `json:"paused"`
	Trusted bool                  `json:"trusted"`
	Poll    DeskPoll              `json:"poll"`
	Budget  DeskBudget            `json:"budget"`
	Counts  map[string]int        `json:"counts"`
	Drafts  int                   `json:"drafts"`
	Active  string                `json:"active"`
	Config  *DeskConfig           `json:"config"`
	Stats   map[string]*DeskStats `json:"stats"`
}

func DeriveDesk(st *State) *DeskDerived {
	d := st.Desk
	cfg := DeskConfigRead(st)
	out := &DeskDerived{Paused: d.Paused, Trusted: d.Seen == "" || d.Seen == d.Hash, Poll: d.Poll, Budget: d.Budget,
		Counts: map[string]int{}, Active: d.Active, Config: cfg, Stats: d.Stats}
	for _, k := range d.Items.Keys() {
		it, _ := d.Items.Get(k)
		out.Counts[it.Status]++
	}
	out.Drafts = out.Counts[ItemDrafted]
	return out
}

// NewDesk creates a desk's state; its folder is root.
func NewDesk(root, name string) *State {
	st := &State{
		Version: 1, Template: Desk,
		Project: Project{Name: name, Cwd: root, CreatedAt: Now()},
		Stage:   "active", Counters: map[string]int{}, Desk: NewDeskState(),
		Root: root, Dir: filepath.Join(root, StateDir),
	}
	return st
}

// FileHash: sha256 of a file, "" if it can't be read. Records and plans live outside the desk, where
// an agent could write; lacarta keeps their hashes in the state and refuses files it didn't write.
func FileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// CheckItemFiles refuses records or a plan changed since lacarta wrote them.
func CheckItemFiles(it *DeskItem, dir string, plan bool) error {
	for p, h := range it.RecordHashes {
		if FileHash(filepath.Join(dir, "records", p+".json")) != h {
			return Gate("The %s record of %s changed after lacarta stored it: nothing is built or posted from it. "+
				"Submit it again with `desk submit`.", p, it.Key)
		}
	}
	if plan && it.PlanHash != "" && FileHash(filepath.Join(dir, "plan.json")) != it.PlanHash {
		return Gate("The plan of %s changed after lacarta wrote it: nothing is posted from it.", it.Key)
	}
	return nil
}
