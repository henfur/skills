package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lacarta/internal/core"
	"lacarta/internal/plugins"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestConfigFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LACARTA_HOME", home)
	t.Setenv("LACARTA_NO_DAEMON", "1")
	t.Setenv("LACARTA_PROJECT", "")
	t.Setenv("LACARTA_SESSION", "")
	t.Setenv("LACARTA_AGENT", "")

	// A legacy settings.json is read until the first save moves it into config.json.
	writeFile(t, filepath.Join(home, "settings.json"), `{"models": {"software": {"spec_review": "haiku"}}, "remote_control": false}`)
	if s := core.SettingsRead(); s.RemoteControl || s.Config.Exists {
		t.Fatalf("legacy settings not read: %+v", s)
	}

	fakeNode := filepath.Join(home, "bin", "node")
	writeFile(t, fakeNode, "#!/bin/sh\n")
	os.Chmod(fakeNode, 0o755)
	tpl := filepath.Join(home, "my-templates")
	writeFile(t, filepath.Join(tpl, "functional.md"), "# {{PROJECT}}: functional, from the home templates\n")
	contains(t, must(t, "config", "init"), "Moved")
	if _, err := os.Stat(filepath.Join(home, "settings.json")); err == nil {
		t.Error("config init should move the legacy settings.json")
	}
	contains(t, must(t, "config"), filepath.Join(home, "config.json")+" (found)", "Remote control  false")
	writeFile(t, filepath.Join(home, "config.json"), `{
  "// note": "kept on save",
  "models": {"software": {"implementation": "sonnet", "nope": "opus", "security_review": "gpt"}, "devops": {}},
  "templates": "my-templates",
  "plugins": {"gitleaks": false, "unknown-plugin": true},
  "tools": {"node": "bin/node", "chromium": "/does/not/exist", "make": "/usr/bin/make"},
  "colour": "blue"
}`)
	s := core.SettingsRead()
	if m, _ := s.Models.Get(core.Software); func() string { v, _ := m.Get("implementation"); return v }() != "sonnet" {
		t.Error("home model not applied")
	}
	if s.Templates != tpl || s.Tools["node"] != fakeNode || core.ToolPath("node") != fakeNode {
		t.Errorf("templates/tools not read: %q %v", s.Templates, s.Tools)
	}
	out, code := lac(t, "config", "check")
	if code != 1 {
		t.Fatalf("config check should fail, got %d:\n%s", code, out)
	}
	contains(t, out, `no role "nope"`, `unknown model "gpt"`, `no kind of project "devops"`,
		"/does/not/exist is not an executable file", `no tool "make"`, `unknown setting "colour"`,
		"no plugin unknown-plugin")

	// The console's saves keep what they don't change, comments included.
	if _, err := core.SettingsUpdate(map[string]map[string]string{core.Software: {"spec_review": "opus"}}, nil); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	b, _ := os.ReadFile(filepath.Join(home, "config.json"))
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["// note"] == nil || raw["templates"] != "my-templates" || raw["tools"] == nil {
		t.Errorf("save lost keys: %s", b)
	}
	sw := raw["models"].(map[string]any)["software"].(map[string]any)
	if sw["implementation"] != "sonnet" || sw["spec_review"] != "opus" || sw["orchestrator"] != nil {
		t.Errorf("models saved wrong (only the overrides belong in the file): %v", sw)
	}

	// A project's config.json, there before `init`: its templates and models win over the home ones.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "tpl", "stack.md"), "# {{PROJECT}}: stack, from the project\n")
	writeFile(t, filepath.Join(root, ".lacarta", "config.json"), `{
  "models": {"implementation": "haiku", "hunter": "opus"},
  "templates": "docs/tpl",
  "plugins": {"gitleaks": true},
  "tools": {"node": "/tmp/evil"},
  "auditor": "Mallory"
}`)
	must(t, "--root", root, "init", "--name", "Shop")
	read := func(k string) string {
		b, _ := os.ReadFile(filepath.Join(root, ".lacarta", "specs", k+".md"))
		return string(b)
	}
	contains(t, read("stack"), "Shop: stack, from the project")
	contains(t, read("functional"), "Shop: functional, from the home templates")
	contains(t, read("security"), "Threat") // built-in

	out = must(t, "--root", root, "models")
	contains(t, out, "implementation           haiku   Haiku 5.5   Implementation (this project's config.json)",
		"spec_review              opus ")
	contains(t, must(t, "--root", root, "agent", "start", "--role", "implementation", "--task", "Phase 1"),
		"Agent tool model: haiku")
	out, _ = lac(t, "--root", root, "config", "check")
	contains(t, out, `no role "hunter" in software development projects`, "tools: can only be set in",
		"auditor: can only be set in")
	if core.ToolPath("node") != fakeNode {
		t.Error("a project's config must not choose the tools")
	}

	// The project's config decides its plugins.
	st, err := core.SoftwareStore(root).Read()
	if err != nil {
		t.Fatal(err)
	}
	ps, err := plugins.ForProject(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.ID == "gitleaks" && p.Config != core.FromProject {
			t.Errorf("gitleaks should be decided by the project config: %+v", p)
		}
	}

	// models --set --project writes the project's file; `default` removes the override.
	must(t, "--root", root, "models", "--set", "functional_review=opus", "--project")
	must(t, "--root", root, "models", "--set", "implementation=default", "--project")
	out = must(t, "--root", root, "models")
	contains(t, out, "functional_review        opus    Opus 5.5    Functional review (this project's config.json)",
		"implementation           sonnet  Sonnet 5.5  Implementation\n")

	// A templates folder outside the project is refused.
	writeFile(t, filepath.Join(root, ".lacarta", "config.json"), `{"templates": "../elsewhere", "remote_control": false}`)
	out, _ = lac(t, "--root", root, "config", "check")
	contains(t, out, "templates: ")
	contains(t, must(t, "--root", root, "config"), "Remote control  false (project config)")
}

func TestProjectNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LACARTA_HOME", home)
	t.Setenv("LACARTA_NO_DAEMON", "1")
	t.Setenv("LACARTA_PROJECT", "")
	t.Setenv("LACARTA_SESSION", "")
	t.Setenv("LACARTA_AGENT", "")
	work := t.TempDir()

	// A session in a subfolder of a git repository: the task is named after the repository, and
	// its config.json is created at the repository's root. The first prompt no longer names it.
	repo := filepath.Join(work, "webshop")
	os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	os.MkdirAll(filepath.Join(repo, "api"), 0o755)
	pid, s, err := core.RegisterSession("s1", filepath.Join(repo, "api"), "", "startup")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(repo, ".lacarta", "config.json"))
	if err != nil || !strings.Contains(string(b), `"name": "webshop"`) {
		t.Fatalf("config not created at the repository root: %s %v", b, err)
	}
	st, _ := s.Read()
	if st.Project.Name != "webshop" {
		t.Errorf("name = %q", st.Project.Name)
	}

	// The config's name wins, for every task in that folder, and survives edits.
	writeFile(t, filepath.Join(repo, ".lacarta", "config.json"), `{"name": "Web shop", "models": {"agent": "sonnet"}}`)
	if st, _ = s.Read(); st.Project.Name != "Web shop" {
		t.Errorf("config name not applied: %q", st.Project.Name)
	}
	_, s2, _ := core.RegisterSession("s2", repo, "", "startup")
	if st2, _ := s2.Read(); st2.Project.Name != "Web shop" {
		t.Errorf("second task: %q", st2.Project.Name)
	}
	if !strings.Contains(must(t, "--project", pid, "status"), "Web shop") {
		t.Error("status should show the config's name")
	}

	// No git repository: the folder's name. The home folder gets no config file (its .lacarta is lacarta's own).
	plain := filepath.Join(work, "notes")
	os.MkdirAll(plain, 0o755)
	_, s3, _ := core.RegisterSession("s3", plain, "", "startup")
	if st3, _ := s3.Read(); st3.Project.Name != "notes" {
		t.Errorf("plain folder: %q", st3.Project.Name)
	}
	userHome, _ := os.UserHomeDir()
	if p := core.ProjectConfigPath(core.Single, &core.Store{Root: userHome}); p != "" {
		t.Errorf("a task in the home folder must not get a config file, got %s", p)
	}

	// init writes the name into the project's config.json; without --name it takes the one there.
	shop := filepath.Join(work, "shop")
	must(t, "--root", shop, "init", "--name", "Shop")
	if b, _ := os.ReadFile(filepath.Join(shop, ".lacarta", "config.json")); !strings.Contains(string(b), `"name": "Shop"`) {
		t.Errorf("init should write the name: %s", b)
	}
	other := filepath.Join(work, "other")
	writeFile(t, filepath.Join(other, ".lacarta", "config.json"), `{"name": "Billing"}`)
	contains(t, must(t, "--root", other, "init"), "project billing-")
}
