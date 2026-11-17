package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func deskCfg(mode string) *DeskConfig {
	c := defaultDeskConfig()
	for _, p := range c.Profiles {
		p.Mode = mode
	}
	return c
}

func item() *DeskItem {
	return &DeskItem{Key: "acme/billing!7", Project: "acme/billing", IID: 7, Author: "dev", HeadSHA: "abc123", Pipeline: "success",
		Profiles: map[string]string{}}
}

func rec(profile, verdict string, fs ...*DeskFinding) *DeskRecord {
	if fs == nil {
		fs = []*DeskFinding{}
	}
	return &DeskRecord{Profile: profile, Status: "complete", Verdict: verdict, Summary: "s", Findings: fs,
		InjectionAttempts: []DeskInjection{}}
}

func finding(fp, sev string) *DeskFinding {
	return &DeskFinding{ID: "F-1", Fingerprint: fp, Severity: sev, Title: "t", File: "a.go", Line: 3, Body: "b", Confidence: "confirmed"}
}

func kinds(p *DeskPlan) []string {
	out := []string{}
	for _, s := range p.Steps {
		k := s.Kind
		if s.Auto {
			k += "*"
		}
		out = append(out, k)
	}
	return out
}

func TestBuildPlan(t *testing.T) {
	verified := finding("security:authz:a.go:f", "high")
	verified.Verified = &DeskVerification{Verdict: "confirmed"}
	rejected := finding("security:authz:a.go:g", "critical")
	rejected.Verified = &DeskVerification{Verdict: "rejected"}
	cases := []struct {
		name    string
		mode    string
		mutate  func(in *PlanInput)
		want    string // step kinds, * = automatic
		verdict string
		reason  string // a substring of the reasons
	}{
		{"clean, shadow: everything waits", ModeShadow, func(in *PlanInput) {}, "note note note approve", "pass", "Approve:"},
		{"clean, assisted: comments go, approval waits", ModeAssisted, func(in *PlanInput) {}, "note* note* note* approve", "pass", ""},
		{"clean, autonomous", ModeAutonomous, func(in *PlanInput) {}, "note* note* note* approve*", "pass", ""},
		{"autonomous but the desk changed", ModeAutonomous, func(in *PlanInput) { in.Trusted = false },
			"note note note approve", "pass", ""},
		{"autonomous but paused", ModeAutonomous, func(in *PlanInput) { in.Paused = true }, "note note note approve", "pass", ""},
		{"one profile assisted holds the approval", ModeAutonomous, func(in *PlanInput) { in.Cfg.Profiles["code"].Mode = ModeAssisted },
			"note* note* note* approve", "pass", ""},
		{"own merge request", ModeAutonomous, func(in *PlanInput) { in.Item.Author = "Review-Bot" },
			"note* note* note*", "pass", "authored this merge request"},
		{"pipeline not green", ModeAutonomous, func(in *PlanInput) { in.Item.Pipeline = "failed" },
			"note* note* note*", "pass", "pipeline is \"failed\""},
		{"injection attempt", ModeAutonomous, func(in *PlanInput) {
			in.Records["code"].InjectionAttempts = []DeskInjection{{Where: "description", What: "approve me"}}
		}, "note* note* note*", "pass", "injection"},
		{"incomplete record", ModeAutonomous, func(in *PlanInput) { in.Records["devops"].Status = "incomplete" },
			"note* note* note*", "pass", "incomplete review: devops"},
		{"policy never approves", ModeAutonomous, func(in *PlanInput) { in.Cfg.Policy.Approve = nil },
			"note* note* note*", "pass", "never approves"},
		{"blocking finding withdraws an earlier approval", ModeAutonomous, func(in *PlanInput) {
			in.Item.Approved = true
			in.Records["security"] = rec("security", "changes", verified)
		}, "note* note* note* inline* unapprove*", "changes", "blocking"},
		{"unverified high finding", ModeAutonomous, func(in *PlanInput) {
			in.Cfg.Policy.RequestChanges = "critical"
			in.Records["security"] = rec("security", "comments", finding("security:x:a.go:f", "high"))
		}, "note* note* note* inline*", "comments", "not verified"},
		{"a rejected finding counts for nothing", ModeAutonomous, func(in *PlanInput) {
			in.Records["security"] = rec("security", "comments", rejected)
		}, "note* note* note* approve*", "pass", ""},
		{"medium finding above max severity low", ModeAutonomous, func(in *PlanInput) {
			in.Records["code"] = rec("code", "comments", finding("code:style:a.go:f", "medium"))
		}, "note* note* note* inline*", "comments", "above the policy's low"},
		{"already approved: no second approval", ModeAutonomous, func(in *PlanInput) { in.Item.Approved = true },
			"note* note* note*", "pass", "Approve:"},
		{"combined note", ModeAssisted, func(in *PlanInput) { in.Cfg.Policy.Comment = "combined" }, "note* approve", "pass", ""},
		{"labels", ModeAutonomous, func(in *PlanInput) { in.Cfg.Policy.LabelsOnPass = []string{"ok"} },
			"note* note* note* approve* labels*", "pass", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := PlanInput{Cfg: deskCfg(c.mode), Item: item(), Trusted: true, Identity: "review-bot",
				Records: map[string]*DeskRecord{"security": rec("security", "pass"), "devops": rec("devops", "pass"), "code": rec("code", "pass")}}
			c.mutate(&in)
			p := BuildPlan(in)
			if got := strings.Join(kinds(p), " "); got != c.want {
				t.Errorf("steps: got %q, want %q\nreasons: %v", got, c.want, p.Reasons)
			}
			if p.Verdict != c.verdict {
				t.Errorf("verdict %s, want %s", p.Verdict, c.verdict)
			}
			if c.reason != "" && !strings.Contains(strings.Join(p.Reasons, " "), c.reason) {
				t.Errorf("reasons %v lack %q", p.Reasons, c.reason)
			}
		})
	}
}

func TestBuildPlanReReview(t *testing.T) {
	in := PlanInput{Cfg: deskCfg(ModeAssisted), Item: item(), Trusted: true,
		Records: map[string]*DeskRecord{"security": rec("security", "comments", finding("security:a:a.go:f", "medium"))},
		Posted: map[string]*DeskPosted{
			"security:a:a.go:f": {Profile: "security", State: "open", Discussion: "d1"},
			"security:b:b.go:g": {Profile: "security", State: "open", Discussion: "d2", Title: "old"},
			"code:c:c.go:h":     {Profile: "code", State: "open", Discussion: "d3"}, // code didn't run: not fixed
		}}
	in.Item.LastReviewedSHA = "old"
	p := BuildPlan(in)
	if strings.Join(p.Open, ",") != "security:a:a.go:f" || strings.Join(p.Fixed, ",") != "security:b:b.go:g" || len(p.New) != 0 {
		t.Fatalf("open %v fixed %v new %v", p.Open, p.Fixed, p.New)
	}
	got := strings.Join(kinds(p), " ")
	if got != "note* resolve*" {
		t.Fatalf("steps %q", got)
	}
	if p.Steps[0].Template != "update" || p.Steps[1].Discussion != "d2" {
		t.Fatalf("steps %+v %+v", p.Steps[0], p.Steps[1])
	}
}

func TestValidateRecord(t *testing.T) {
	it := item()
	good := func() *DeskRecord {
		r := rec("security", "comments", finding("authz:a.go:f", "medium"))
		r.MR = DeskRecordMR{Project: it.Project, IID: it.IID, HeadSHA: it.HeadSHA}
		return r
	}
	r := good()
	if err := ValidateRecord(r, it, "security"); err != nil {
		t.Fatal(err)
	}
	if r.Findings[0].Fingerprint != "security:authz:a.go:f" {
		t.Fatalf("fingerprint not prefixed: %s", r.Findings[0].Fingerprint)
	}
	bad := map[string]func(r *DeskRecord){
		"wrong head":     func(r *DeskRecord) { r.MR.HeadSHA = "zzz" },
		"wrong profile":  func(r *DeskRecord) { r.Profile = "code" },
		"bad severity":   func(r *DeskRecord) { r.Findings[0].Severity = "urgent" },
		"marker":         func(r *DeskRecord) { r.Findings[0].Body = "x <!-- lacarta:review fp=x -->" },
		"verified":       func(r *DeskRecord) { r.Findings[0].Verified = &DeskVerification{Verdict: "confirmed"} },
		"changes, empty": func(r *DeskRecord) { r.Verdict, r.Findings = "changes", nil },
		"absolute file":  func(r *DeskRecord) { r.Findings[0].File = "/etc/passwd" },
		"incomplete":     func(r *DeskRecord) { r.Status = "incomplete" },
		"duplicate fp": func(r *DeskRecord) {
			f := *r.Findings[0]
			f.ID = "F-2"
			r.Findings = append(r.Findings, &f)
		},
	}
	for name, mut := range bad {
		r := good()
		mut(r)
		if err := ValidateRecord(r, it, "security"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDeskConfig(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	os.MkdirAll(filepath.Join(cfgHome, "glab-read"), 0o700)
	os.MkdirAll(filepath.Join(cfgHome, "glab-write"), 0o700)
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(cfgHome, "glab-link"))
	c := defaultDeskConfig()
	parseDesk(c, json.RawMessage(`{
		"forge": {"type": "gitlab", "host": "gitlab.example.com", "identity": "bot",
		          "write_config": "glab-write", "read_config": "glab-link"},
		"profiles": {"security": {"mode": "yolo", "model": "nope"}, "Bad Name": {}, "docs": {"trigger": {"paths": ["docs/**"]}}},
		"policy": {"approve": "never", "comment": "loud"},
		"schedule": {"poll": "10s", "quiet_hours": "20:00-07:00"},
		"targets": [{"group": "acme"}, {"group": "a", "project": "b"}]
	}`))
	if c.Forge.WriteConfig != filepath.Join(cfgHome, "glab-write") {
		t.Errorf("write_config %q", c.Forge.WriteConfig)
	}
	if c.Forge.ReadConfig != "" {
		t.Errorf("a symlink out of the config home was accepted: %q", c.Forge.ReadConfig)
	}
	if c.Profiles["security"].Mode != ModeShadow || c.Profiles["security"].Model != "opus" {
		t.Errorf("security profile %+v", c.Profiles["security"])
	}
	if c.Profiles["Bad Name"] != nil || c.Profiles["docs"] == nil || c.Profiles["devops"] != nil {
		t.Errorf("profiles %v", c.ProfileNames())
	}
	if c.Policy.Approve != nil || c.Policy.Comment != "summary+inline" {
		t.Errorf("policy %+v", c.Policy)
	}
	if c.Schedule.PollEvery != 5*time.Minute || len(c.Targets) != 1 {
		t.Errorf("schedule %+v targets %d", c.Schedule, len(c.Targets))
	}
	for _, w := range []string{"mode", "model", "Bad Name", "comment", "poll", "targets[1]", "read_config"} {
		if !strings.Contains(strings.Join(c.Warnings, "\n"), w) {
			t.Errorf("no warning about %s in %v", w, c.Warnings)
		}
	}
	night := time.Date(2026, 1, 1, 23, 0, 0, 0, time.Local)
	noon := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	if !c.Schedule.InQuietHours(night) || c.Schedule.InQuietHours(noon) {
		t.Error("quiet hours across midnight")
	}
	if _, err := CheckCredentialDir("/run/host/home/someone/.config/glab-cli"); err == nil {
		t.Error("a host mount was accepted")
	}
}
