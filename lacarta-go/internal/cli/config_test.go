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

	// The project's plugin choice is locked against the console.
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
	if err := plugins.SetEnabled(st, "gitleaks", false, "user"); err == nil || !strings.Contains(err.Error(), "change it there") {
		t.Errorf("console toggle should be refused, got %v", err)
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
