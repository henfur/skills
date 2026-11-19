package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lacarta/internal/core"
	"lacarta/internal/desk"
	"lacarta/internal/server"
	"lacarta/skill"
)

// Commands of the review desk kind. Docs: docs/design/pr-review.md.

// DeskPollEvery is how often `desk next` looks at the queue (the forge is polled on the desk's schedule).
var DeskPollEvery = 2 * time.Second

func deskCommands() []*Cmd {
	profiles := "a profile of the desk, or `verify` for a verifier"
	return []*Cmd{
		{Name: "desk init", Help: "Make a folder a review desk: config, default instructions and templates, session settings.",
			Run: cmdDeskInit, Opts: []Opt{{Name: "folder", Help: "The desk's folder (default: the current folder)."}, {Name: "name"}}},
		{Name: "desk guide", Help: "How the orchestrator runs a review desk. Read it first.", Run: cmdDeskGuide},
		{Name: "desk status", Help: "The desk's loop, queue and what waits for the user.", Run: cmdDeskStatus,
			Opts: []Opt{{Name: "json", Kind: flag}}},
		{Name: "desk preflight", Help: "Check glab, both identities, the targets and the templates; rewrite the session settings.",
			Run: cmdDeskPreflight},
		{Name: "desk poll", Help: "Poll the targets now (the desk does it on its own while `desk next` runs).", Run: cmdDeskPoll},
		{Name: "desk next", Help: "Wait for the next merge request to review and print it as JSON. Run it as a background command.",
			Run: cmdDeskNext, Opts: []Opt{{Name: "timeout", Kind: num, Help: "Seconds before giving up (exit 3); 0 waits forever."}}},
		{Name: "desk brief", Help: "A reviewer's (or verifier's) whole prompt for one merge request.", Run: cmdDeskBrief,
			Pos: []Pos{{Name: "profile", Help: profiles}}, Opts: []Opt{{Name: "item", Required: true, Help: "The merge request's key (project!iid)."},
				{Name: "finding", Help: "For a verifier: <profile>/<finding id>."}}},
		{Name: "desk submit", Help: "Check a reviewer's record and store it.", Run: cmdDeskSubmit, Pos: []Pos{{Name: "file"}},
			Opts: []Opt{{Name: "item", Required: true}, {Name: "profile", Required: true}}},
		{Name: "desk verify", Help: "Record a verifier's verdict on one finding.", Run: cmdDeskVerify, Opts: []Opt{
			{Name: "item", Required: true}, {Name: "finding", Required: true, Help: "<profile>/<finding id>"},
			{Name: "verdict", Required: true, Choices: core.VerificationVerdict}, {Name: "severity", Choices: core.Severities},
			{Name: "note", Required: true}}},
		{Name: "desk done", Help: "Build the plan from the records, post what the modes allow, ask the user for the rest.",
			Run: cmdDeskDone, Pos: []Pos{{Name: "item"}}},
		{Name: "desk publish", Help: "Post what may go now for one merge request (what the modes allow, or what the user approved).",
			Run: cmdDeskPublish, Pos: []Pos{{Name: "item"}}},
		{Name: "desk mode", Help: "Set a profile's mode in the desk's config.json (as the user asked). Raising one takes effect once " +
			"the user accepts the change in the console.", Run: cmdDeskMode,
			Pos: []Pos{{Name: "profile"}, {Name: "mode", Choices: core.DeskModes}}},
		{Name: "desk pause", Help: "Stop posting and starting reviews; the user resumes from the console.", Run: cmdDeskPause},
		{Name: "desk templates check", Help: "Render every comment template the desk uses against a fixture.", Run: cmdDeskTemplates},
		{Name: "desk tool", Help: "Run one of the desk's agent tools on a merge request (sandboxed).", Run: cmdDeskTool,
			Pos: []Pos{{Name: "name"}, {Name: "args", Many: true}}, Opts: []Opt{{Name: "item", Required: true}}},
	}
}

func deskState() (*core.State, string, error) {
	st, err := store.Read()
	if err != nil {
		return nil, "", err
	}
	if st.Template != core.Desk || st.Desk == nil {
		return nil, "", core.Gate("This command belongs to review desks; this project is a %s. `desk init` makes a folder a desk.",
			strings.ToLower(core.KindByID(st.Template).Label))
	}
	return st, deskPID(st), nil
}

func deskPID(st *core.State) string {
	if pc := core.ProjectConfigRead(st); pc.ID != "" {
		return pc.ID
	}
	h, _ := core.HubRead()
	for _, k := range h.Projects.Keys() {
		if p, _ := h.Projects.Get(k); p.Root == st.Root {
			return k
		}
	}
	return filepath.Base(st.Root)
}

func cmdDeskInit(a *Args) error {
	root, _ := filepath.Abs(firstNonEmpty(a.Str("folder"), "."))
	if core.FindRoot(root) == root {
		return core.Gate("%s is already a lacarta project.", root)
	}
	name := firstNonEmpty(a.Str("name"), filepath.Base(root))
	if a.Str("name") != "" {
		_ = os.MkdirAll(filepath.Join(root, core.StateDir), 0o755)
		cfg := filepath.Join(root, core.StateDir, core.ConfigFile)
		raw := map[string]json.RawMessage{}
		if b, err := os.ReadFile(cfg); err == nil && json.Unmarshal(b, &raw) != nil {
			return core.Gate("%s is not valid JSON: fix it first.", cfg)
		}
		b, _ := json.Marshal(name)
		raw["name"] = b
		if err := core.WriteJSON(cfg, raw); err != nil {
			return err
		}
	}
	pid, err := desk.Init(root, name, self())
	if err != nil {
		return err
	}
	s := core.SoftwareStore(root)
	if sid := os.Getenv("LACARTA_SESSION"); sid != "" {
		if err := core.MoveSession(sid, pid, s, os.Getenv("LACARTA_TRANSCRIPT")); err != nil {
			return err
		}
	}
	console := "not running; start it with `serve`"
	if info := server.EnsureDaemon(self()); info != nil {
		console = info.OpenURL
	}
	a.Printf("Review desk %s in %s\nConsole: %s\n", pid, root, console)
	a.Printf("Edit .lacarta/config.json (\"desk\": forge, targets, profiles, policy), instructions/ and templates/. Every profile " +
		"starts in shadow mode: nothing is posted until the user publishes it.\n")
	a.Printf("Next: read `%s desk guide`, help the user set the forge identities (see its Setup section), then "+
		"`desk preflight`. Restart the session in %s so it picks up the desk's .claude/settings.json.\n", self(), root)
	return nil
}

func cmdDeskGuide(a *Args) error {
	b, err := fs.ReadFile(skill.Desk, "desk/guide.md")
	if err != nil {
		return err
	}
	a.Printf("%s\n%s", b, fmt.Sprintf(deskSetup, self(), core.ConfigHome()))
	return nil
}

const deskSetup = `## Setup (once, with the user)

The desk needs two GitLab identities, each a glab config folder inside this environment's own config folder
(%[2]s, never a host home mounted into a container):

1. The account the desk posts as (a service account is best), with an "api" token:
   GLAB_CONFIG_DIR=~/.config/glab-desk-write glab auth login --hostname <host>
2. A read-only token (read_api) for the agents, of any account that can see the targets:
   GLAB_CONFIG_DIR=~/.config/glab-desk-read glab auth login --hostname <host>

The user runs these themselves (tokens never go through you). Then in .lacarta/config.json, "desk.forge":
{"host": "<host>", "identity": "<username of 1>", "write_config": "~/.config/glab-desk-write",
"read_config": "~/.config/glab-desk-read"}, and "desk.targets": the groups and projects to watch.
Run %[1]s desk preflight: it checks both identities and rewrites .claude/settings.json so this session
uses the read one. Restart the session afterwards.
`

func cmdDeskStatus(a *Args) error {
	st, pid, err := deskState()
	if err != nil {
		return err
	}
	if err := store.Edit(func(st *core.State) error { core.DeskCheckContent(st); return nil }); err != nil {
		return err
	}
	if st, err = store.Read(); err != nil {
		return err
	}
	if a.Flag("json") {
		d, err := core.DeriveState(st, store)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(map[string]any{"project": pid, "derived": d, "items": st.Desk.Items}, "", "  ")
		a.Println(string(b))
		return nil
	}
	cfg := core.DeskConfigRead(st)
	d := st.Desk
	a.Printf("%s: review desk %s on %s\n", st.Project.Name, pid, firstNonEmpty(cfg.Forge.Host, "(no host set)"))
	state := "running"
	if d.Paused {
		state = "paused"
	}
	a.Printf("Loop: %s, %d target(s), last poll %s", state, len(cfg.Targets), firstNonEmpty(d.Poll.Last, "never"))
	if d.Poll.Error != "" {
		a.Printf(" (failed: %s)", d.Poll.Error)
	}
	a.Println("")
	modes := []string{}
	for _, p := range cfg.ProfileNames() {
		modes = append(modes, p+" "+cfg.Profiles[p].Mode)
	}
	a.Printf("Profiles: %s\n", strings.Join(modes, ", "))
	if d.Seen != "" && d.Seen != d.Hash {
		a.Println("The desk's content changed since the user accepted it: nothing is posted on its own until they do.")
	}
	for _, k := range d.Items.Keys() {
		it, _ := d.Items.Get(k)
		if it.Status == core.ItemPublished || it.Status == core.ItemDiscarded || it.Status == core.ItemDropped {
			continue
		}
		extra := ""
		if it.Reason != "" {
			extra = " (" + it.Reason + ")"
		}
		if it.Status == core.ItemReviewing {
			ps := []string{}
			for _, p := range sortedKeys(it.Profiles) {
				ps = append(ps, p+" "+it.Profiles[p])
			}
			extra = " [" + strings.Join(ps, ", ") + "]"
		}
		a.Printf("  %s %-10s %s%s\n", k, it.Status, it.Title, extra)
	}
	for _, w := range cfg.Warnings {
		a.Printf("Config: %s\n", w)
	}
	for _, q := range core.OpenQuestions(st) {
		a.Printf("  %s [%s] %s\n", q.ID, q.Kind, q.Title)
	}
	a.Printf("\nNext: %s\n", core.DeskNext(st))
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && ks[j] < ks[j-1]; j-- {
			ks[j], ks[j-1] = ks[j-1], ks[j]
		}
	}
	return ks
}

func cmdDeskPreflight(a *Args) error {
	st, pid, err := deskState()
	if err != nil {
		return err
	}
	if err := desk.WriteSessionSettings(store, self()); err != nil {
		return err
	}
	failed := 0
	for _, c := range desk.Preflight(pid, st) {
		mark := "ok  "
		if !c.OK {
			mark, failed = "MISS", failed+1
		}
		a.Printf("%s %-18s %s\n", mark, c.Name, c.Detail)
	}
	if err := store.Edit(func(st *core.State) error { core.DeskCheckContent(st); return nil }); err != nil {
		return err
	}
	a.Printf("Session settings: %s (restart the session to apply them).\n", filepath.Join(st.Root, ".claude", "settings.json"))
	if failed > 0 {
		return core.Gate("%d check(s) failed: the desk can't run until they pass.", failed)
	}
	a.Println("Ready: keep `desk next` running as a background command.")
	return nil
}

func cmdDeskPoll(a *Args) error {
	_, pid, err := deskState()
	if err != nil {
		return err
	}
	if err := desk.Poll(pid, store); err != nil {
		return err
	}
	st, _ := store.Read()
	if st.Desk.Poll.Error != "" {
		return core.Gate("Poll failed: %s", st.Desk.Poll.Error)
	}
	n := 0
	for _, k := range st.Desk.Items.Keys() {
		if it, _ := st.Desk.Items.Get(k); it.Status == core.ItemQueued {
			n++
		}
	}
	a.Printf("Polled: %d merge request(s) queued.\n", n)
	return nil
}

func cmdDeskNext(a *Args) error {
	_, pid, err := deskState()
	if err != nil {
		return err
	}
	var deadline time.Time
	if t := a.Int("timeout"); t > 0 {
		deadline = time.Now().Add(time.Duration(t) * time.Second)
	}
	for {
		if err := desk.Settle(pid, store); err != nil {
			fmt.Fprintf(a.Err, "lacarta: publishing: %v\n", err)
		}
		if err := store.Edit(func(st *core.State) error { core.DeskCheckContent(st); return nil }); err != nil {
			return err
		}
		c, err := desk.Claim(pid, store)
		if err != nil {
			return err
		}
		if c == nil {
			st, err := store.Read()
			if err != nil {
				return err
			}
			if !st.Desk.Paused && desk.PollDue(st, core.DeskConfigRead(st)) {
				if err := desk.Poll(pid, store); err != nil {
					return err
				}
				if c, err = desk.Claim(pid, store); err != nil {
					return err
				}
			}
		}
		if c != nil {
			b, _ := json.MarshalIndent(c, "", "  ")
			a.Println(string(b))
			a.Printf("Next: for each profile, `agent start --role reviewer`, then give a fresh subagent on the profile's model "+
				"the output of its `desk brief` as its whole prompt. When every record is in: `desk done '%s'`.\n", c.Item.Key)
			return nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			a.Println("No merge request to review yet.")
			return exitCode(3)
		}
		time.Sleep(DeskPollEvery)
	}
}

func cmdDeskBrief(a *Args) error {
	st, pid, err := deskState()
	if err != nil {
		return err
	}
	agent := a.Agent
	if agent == "" || agent == core.Orchestrator {
		agent = ""
	}
	text, err := desk.Brief(pid, st, a.Positional("profile"), a.Str("item"), agent, a.Str("finding"))
	if err != nil {
		return err
	}
	a.Printf("%s", text)
	return nil
}

func cmdDeskSubmit(a *Args) error {
	_, pid, err := deskState()
	if err != nil {
		return err
	}
	file, _ := filepath.Abs(a.Positional("file"))
	if err := desk.Submit(pid, store, a.Str("item"), a.Str("profile"), file); err != nil {
		return err
	}
	a.Printf("Record stored for %s (%s).\n", a.Str("item"), a.Str("profile"))
	return nil
}

func cmdDeskVerify(a *Args) error {
	_, pid, err := deskState()
	if err != nil {
		return err
	}
	if err := desk.Verify(pid, store, a.Str("item"), a.Str("finding"), a.Str("verdict"), a.Str("severity"), a.Str("note")); err != nil {
		return err
	}
	a.Printf("Verdict recorded on %s %s: %s.\n", a.Str("item"), a.Str("finding"), a.Str("verdict"))
	return nil
}

func cmdDeskDone(a *Args) error {
	_, pid, err := deskState()
	if err != nil {
		return err
	}
	key := a.Positional("item")
	plan, err := desk.Done(pid, store, key)
	if plan != nil {
		posted, waiting := 0, 0
		for _, s := range plan.Steps {
			switch {
			case s.Status == "done":
				posted++
			case s.Status == "pending":
				waiting++
			}
		}
		a.Printf("Plan for %s: %s. %d step(s) posted, %d waiting for the user in the console.\n", key, plan.Verdict, posted, waiting)
		for _, r := range plan.Reasons {
			a.Printf("  %s\n", r)
		}
	}
	if err != nil {
		return err
	}
	a.Println("Next: back to `desk next` (as a background command).")
	return nil
}

func cmdDeskPublish(a *Args) error {
	_, pid, err := deskState()
	if err != nil {
		return err
	}
	n, err := desk.Publish(pid, store, a.Positional("item"))
	if err != nil {
		return err
	}
	a.Printf("%d step(s) posted.\n", n)
	return nil
}

func cmdDeskMode(a *Args) error {
	st, _, err := deskState()
	if err != nil {
		return err
	}
	prof, mode := a.Positional("profile"), a.Positional("mode")
	old := core.DeskConfigRead(st).Profiles[prof]
	if err := core.SetDeskMode(st, prof, mode); err != nil {
		return err
	}
	lower := old != nil && modeLower(mode, old.Mode)
	err = store.Edit(func(st *core.State) error {
		core.Log(st, fmt.Sprintf("Mode of %s set to %s in config.json (terminal).", prof, mode), "stage")
		if lower && st.Desk.Seen == st.Desk.Hash {
			st.Desk.Hash = core.DeskHash(st.Root) // lowering a mode never needs the user's acceptance
		}
		core.DeskCheckContent(st)
		return nil
	})
	if err != nil {
		return err
	}
	if lower {
		a.Printf("%s is now %s.\n", prof, mode)
	} else {
		a.Printf("%s is set to %s in config.json; it takes effect once the user accepts the desk's new content in the console.\n", prof, mode)
	}
	return nil
}

func modeLower(n, o string) bool {
	rank := map[string]int{core.ModeShadow: 0, core.ModeAssisted: 1, core.ModeAutonomous: 2}
	return rank[n] <= rank[o]
}

func cmdDeskPause(a *Args) error {
	if _, _, err := deskState(); err != nil {
		return err
	}
	err := store.Edit(func(st *core.State) error {
		st.Desk.Paused = true
		core.Log(st, "Desk paused from the terminal: nothing is posted or started until it is resumed in the console.", "stage")
		return nil
	})
	if err != nil {
		return err
	}
	a.Println("Paused. The user resumes the desk from the console.")
	return nil
}

func cmdDeskTemplates(a *Args) error {
	st, _, err := deskState()
	if err != nil {
		return err
	}
	errs := desk.CheckTemplates(st.Root, core.DeskConfigRead(st))
	for _, e := range errs {
		a.Println(e)
	}
	if len(errs) > 0 {
		return core.Gate("%d template error(s).", len(errs))
	}
	a.Println("Every template renders.")
	return nil
}

func cmdDeskTool(a *Args) error {
	st, pid, err := deskState()
	if err != nil {
		return err
	}
	cfg := core.DeskConfigRead(st)
	it, err := core.DeskItemGet(st, a.Str("item"))
	if err != nil {
		return err
	}
	name := a.Positional("name")
	if t := cfg.Tools[name]; t == nil || t.When != "agent" {
		return core.Gate("No agent tool %s in desk.tools.", name)
	}
	out, err := desk.RunTool(st.Root, core.ItemDir(pid, it), cfg, name, a.Positionals("args"))
	a.Printf("%s", out)
	return err
}
