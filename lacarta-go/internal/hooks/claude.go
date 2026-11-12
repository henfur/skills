package hooks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"lacarta/internal/core"
)

// Transcripts: Claude Code appends one JSON object per line. The console shows the user's and
// the agent's text and one line per tool call; the CLI's own plumbing is left out.

type ChatItem struct {
	Role string `json:"role"`
	Name string `json:"name,omitempty"`
	Text string `json:"text"`
	At   string `json:"at"`
}

var cliPlumbing = []string{"<command-", "<local-command", "<system-reminder", "<bash-", "<user-memory", "Caveat:"}

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// TranscriptAllowed: only session transcripts are served, whatever a state file claims.
func TranscriptAllowed(path string) bool {
	if path == "" || filepath.Ext(path) != ".jsonl" {
		return false
	}
	base, err := filepath.EvalSymlinks(claudeDir())
	if err != nil {
		return false
	}
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

type entry struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	Timestamp   string `json:"timestamp"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func ChatItems(line []byte) []ChatItem {
	var e entry
	if json.Unmarshal(line, &e) != nil || e.IsSidechain || e.IsMeta || (e.Type != "user" && e.Type != "assistant") {
		return nil
	}
	var blocks []block
	var s string
	if json.Unmarshal(e.Message.Content, &s) == nil {
		blocks = []block{{Type: "text", Text: s}}
	} else {
		var raw []json.RawMessage
		_ = json.Unmarshal(e.Message.Content, &raw)
		for _, r := range raw {
			var b block
			if json.Unmarshal(r, &b) == nil {
				blocks = append(blocks, b)
			}
		}
	}
	out := []ChatItem{}
	for _, b := range blocks {
		switch {
		case b.Type == "text":
			text := strings.TrimSpace(b.Text)
			if text == "" || (e.Type == "user" && (plumbing(text) || strings.Contains(text, core.ConsoleTag))) {
				continue // console messages are shown from the inbox, with their delivery state
			}
			role := "agent"
			if e.Type == "user" {
				role = "user"
			}
			out = append(out, ChatItem{Role: role, Text: text, At: e.Timestamp})
		case b.Type == "tool_use" && e.Type == "assistant":
			hint := ""
			for _, k := range []string{"description", "command", "file_path", "pattern", "url", "prompt"} {
				if v, ok := b.Input[k].(string); ok && v != "" {
					hint = v
					break
				}
			}
			if r := []rune(hint); len(r) > 240 {
				hint = string(r[:240])
			}
			out = append(out, ChatItem{Role: "tool", Name: b.Name, Text: hint, At: e.Timestamp})
		}
	}
	return out
}

func plumbing(text string) bool {
	for _, p := range cliPlumbing {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

// Reading is incremental: each transcript is parsed once, then only what was appended.
type chatCache struct {
	ino    uint64
	offset int64
	items  []ChatItem
}

var (
	chatMu sync.Mutex
	chats  = map[string]*chatCache{}
)

func ReadChat(path string, limit int) []ChatItem {
	if !TranscriptAllowed(path) {
		return []ChatItem{}
	}
	chatMu.Lock()
	defer chatMu.Unlock()
	info, err := os.Stat(path)
	if err != nil {
		return []ChatItem{}
	}
	ino := inode(info)
	c := chats[path]
	if c == nil || c.ino != ino || c.offset > info.Size() {
		c = &chatCache{ino: ino}
	}
	f, err := os.Open(path)
	if err != nil {
		return []ChatItem{}
	}
	defer f.Close()
	if _, err := f.Seek(c.offset, io.SeekStart); err != nil {
		return []ChatItem{}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return []ChatItem{}
	}
	end := bytes.LastIndexByte(data, '\n') + 1 // a line still being written waits for the next read
	sc := bufio.NewScanner(bytes.NewReader(data[:end]))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		c.items = append(c.items, ChatItems(sc.Bytes())...)
	}
	if len(c.items) > limit {
		c.items = c.items[len(c.items)-limit:]
	}
	c.offset += int64(end)
	chats[path] = c
	return append([]ChatItem{}, c.items...)
}

// ---------------------------------------------------------------- install

type event struct {
	name    string
	timeout int
	matcher string
}

// Claude Code event -> our hook, its timeout in seconds, and its matcher.
var claudeEvents = []struct {
	Event string
	event
}{
	{"SessionStart", event{"session-start", 30, ""}},
	{"UserPromptSubmit", event{"prompt", 15, ""}},
	{"PostToolUse", event{"post-tool", 15, "*"}},
	{"Notification", event{"notification", 15, ""}},
	{"Stop", event{"stop", 86400, ""}},
	{"SessionEnd", event{"session-end", 15, ""}},
}

// Hooks written by the prototype or by an earlier binary are replaced.
var hookMarks = []string{"lacarta.py hook", "lacarta hook"}

func ours(cmd string) bool {
	for _, m := range hookMarks {
		if strings.Contains(cmd, m) {
			return true
		}
	}
	return false
}

// Install adds (or with uninstall, removes) the hooks in a Claude Code settings file.
func Install(settings string, uninstall bool, w io.Writer) error {
	if settings == "" {
		settings = filepath.Join(claudeDir(), "settings.json")
	}
	var cfg core.OMap[json.RawMessage]
	old, err := os.ReadFile(settings)
	hadFile := err == nil
	if hadFile && len(bytes.TrimSpace(old)) > 0 {
		if err := json.Unmarshal(old, &cfg); err != nil {
			return fmt.Errorf("%s: %w", settings, err)
		}
	}
	var hooks core.OMap[[]map[string]any]
	if raw, ok := cfg.Get("hooks"); ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return fmt.Errorf("%s: hooks: %w", settings, err)
		}
	}
	base := shellQuote(Self) + " hook"
	for _, e := range claudeEvents {
		groups, _ := hooks.Get(e.Event)
		kept := []map[string]any{}
		for _, g := range groups {
			list, _ := g["hooks"].([]any)
			mine := false
			for _, h := range list {
				if m, ok := h.(map[string]any); ok {
					if c, _ := m["command"].(string); ours(c) {
						mine = true
					}
				}
			}
			if !mine {
				kept = append(kept, g)
			}
		}
		if !uninstall {
			g := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": base + " " + e.name, "timeout": e.timeout}}}
			if e.matcher != "" {
				g["matcher"] = e.matcher
			}
			kept = append(kept, g)
		}
		if len(kept) > 0 {
			hooks.Set(e.Event, kept)
		} else {
			hooks.Delete(e.Event)
		}
	}
	if hooks.Len() > 0 {
		b, _ := json.Marshal(hooks)
		cfg.Set("hooks", b)
	} else {
		cfg.Delete("hooks")
	}
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		return err
	}
	if hadFile {
		if err := os.WriteFile(settings+".bak", old, 0o644); err != nil {
			return err
		}
	}
	out, err := core.MarshalIndent(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(settings, append(out, '\n'), 0o644); err != nil {
		return err
	}
	name := filepath.Base(settings)
	if uninstall {
		fmt.Fprintf(w, "Removed the lacarta hooks from %s (backup: %s.bak).\n", settings, name)
		return nil
	}
	fmt.Fprintf(w, "Installed the lacarta hooks in %s (backup: %s.bak). New Claude Code sessions register in the console; "+
		"restart running sessions to include them.\n", settings, name)
	if _, err := os.Stat(filepath.Join(filepath.Dir(settings), "skills", "lacarta")); err != nil {
		fmt.Fprintf(w, "The skill itself is not in %s: link the lacarta skill folder there so sessions can load it.\n",
			filepath.Join(filepath.Dir(settings), "skills"))
	}
	return nil
}

func inode(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}
