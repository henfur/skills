package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lacarta/internal/core"
)

func TestTrack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LACARTA_HOME", home)
	t.Setenv("LACARTA_NO_DAEMON", "1")
	t.Setenv("LACARTA_PROJECT", "")
	t.Setenv("LACARTA_AGENT", "")
	t.Setenv("LACARTA_TRANSCRIPT", "")
	work := t.TempDir()

	// From a terminal, outside any session: the folder gets its id, and its next sessions join.
	notes := filepath.Join(work, "notes")
	os.MkdirAll(notes, 0o755)
	t.Setenv("LACARTA_SESSION", "")
	t.Chdir(notes)
	contains(t, must(t, "track", "--name", "Release notes"), `as "Release notes" (t-`)
	id := core.FolderID(notes)
	if id == "" || !core.Tracked(notes) {
		t.Fatalf("track should give the folder an id, got %q", id)
	}
	contains(t, must(t, "track"), "Already tracked", id)

	// From a session the hooks left out: the session joins, and the agent gets its instructions.
	repo := filepath.Join(work, "webshop")
	os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	t.Chdir(repo)
	t.Setenv("LACARTA_SESSION", "s-9")
	t.Setenv("LACARTA_TRANSCRIPT", "/t/s-9.jsonl")
	out := must(t, "track")
	contains(t, out, "Added this session to the console", "Later sessions in "+repo, "is registered in the lacarta console")
	h, _ := core.HubRead()
	pid, ok := h.Sessions.Get("s-9")
	if !ok || core.FolderID(repo) != pid {
		t.Fatalf("session not registered (%q) or folder id %q", pid, core.FolderID(repo))
	}
	// Its later commands find the project by the session, without $LACARTA_PROJECT.
	contains(t, must(t, "status"), "webshop")
	contains(t, must(t, "track"), "already in the console as "+pid)

	// `init` from a session that was never registered attaches it to the new project.
	shop := filepath.Join(work, "shop")
	os.MkdirAll(shop, 0o755)
	t.Chdir(shop)
	t.Setenv("LACARTA_SESSION", "s-10")
	must(t, "init", "--name", "Shop")
	h, _ = core.HubRead()
	spid, ok := h.Sessions.Get("s-10")
	st, err := core.SoftwareStore(shop).Read()
	hp, _ := h.Projects.Get(spid)
	if err != nil || !ok || hp == nil || hp.Root != shop {
		t.Fatalf("init didn't attach the session: %q %v", spid, err)
	}
	if a := core.MustAgent(st, core.Orchestrator); a.Session == nil || a.Session.ID != "s-10" {
		t.Errorf("orchestrator session: %+v", a.Session)
	}
	if out, _ := lac(t, "track"); !strings.Contains(out, "already a lacarta project") {
		t.Errorf("track in a software project: %s", out)
	}
}
