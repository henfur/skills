// Package golden runs scripted scenarios against a lacarta implementation and compares the
// output with recordings. Recordings come from the Python prototype, so passing means the Go
// binary behaves exactly like it:
//
//	GOLDEN_IMPL=python GOLDEN_RECORD=1 go test ./internal/golden   # record from the prototype
//	go test ./internal/golden                                      # check the Go binary
//
// Scenario lines: `$ <lacarta args>` runs lacarta, `% <lacarta args>` always runs the prototype
// (so Go and Python read each other's state files), `> <shell>` runs sh in the work folder,
// `@env <file>` loads `export K=V` lines into the environment, `@set K=V` and `@unset K` change
// it, `#` starts a comment. A `$`/`%` line may end with ` <<< <text>`: the command's stdin.
// $VARS are expanded from that environment.
package golden

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var bin string

func repoDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func pythonCLI() []string {
	return []string{"python3", filepath.Join(repoDir(), "..", "lacarta", "scripts", "lacarta.py")}
}

func TestMain(m *testing.M) {
	if os.Getenv("GOLDEN_IMPL") != "python" {
		dir, err := os.MkdirTemp("", "lacarta-bin")
		if err != nil {
			panic(err)
		}
		bin = filepath.Join(dir, "lacarta")
		build := exec.Command("go", "build", "-o", bin, "./cmd/lacarta")
		build.Dir = repoDir()
		if out, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build failed: %s\n%s", err, out)
			os.Exit(1)
		}
		code := m.Run()
		_ = os.RemoveAll(dir) // a deferred call would never run before os.Exit
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestScenarios(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(repoDir(), "testdata", "scenarios", "*.txt"))
	if len(files) == 0 {
		t.Fatal("no scenarios")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".txt")
		t.Run(name, func(t *testing.T) {
			got := runScenario(t, name, f)
			goldenPath := filepath.Join(repoDir(), "testdata", "golden", name+".out")
			if os.Getenv("GOLDEN_RECORD") != "" {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("no recording (run with GOLDEN_IMPL=python GOLDEN_RECORD=1): %v", err)
			}
			if got != string(want) {
				t.Errorf("output differs from the prototype's recording:\n%s", diff(string(want), got))
			}
		})
	}
}

func runScenario(t *testing.T, name, path string) string {
	// A fixed folder per scenario: project ids hash the path, so both implementations see the same one.
	work := filepath.Join(os.TempDir(), "lacarta-golden", name)
	_ = os.RemoveAll(work)
	home := filepath.Join(work, ".home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"LACARTA_HOME": home, "LACARTA_NO_DAEMON": "1", "W": work, "HOME": os.Getenv("HOME"),
		"PATH": os.Getenv("PATH"), "LANG": "C.UTF-8", "CLAUDE_CONFIG_DIR": filepath.Join(home, "claude"),
		"PYLACARTA": pythonCLI()[1], // scenarios use the prototype's hooks until the Go ones exist
	}
	cli := pythonCLI()
	if bin != "" {
		cli = []string{bin}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out strings.Builder
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "$ "), strings.HasPrefix(line, "% "):
			text, stdin, _ := strings.Cut(line[2:], " <<< ")
			expand := func(v string) string { return os.Expand(v, func(k string) string { return env[k] }) }
			args, err := split(expand(text))
			if err != nil {
				t.Fatalf("%s: %v", line, err)
			}
			run := cli
			if line[0] == '%' { // always the prototype: the two implementations share state files
				run = pythonCLI()
			}
			cmd := exec.Command(run[0], append(run[1:], args...)...)
			cmd.Dir, cmd.Env = work, envList(env)
			cmd.Stdin = strings.NewReader(expand(stdin))
			var so, se bytes.Buffer
			cmd.Stdout, cmd.Stderr = &so, &se
			code := 0
			if err := cmd.Run(); err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("%s: %v", line, err)
				}
				code = ee.ExitCode()
			}
			fmt.Fprintf(&out, "%s\n%s", line, so.String())
			for _, l := range strings.Split(strings.TrimSuffix(se.String(), "\n"), "\n") {
				if l != "" {
					fmt.Fprintf(&out, "stderr: %s\n", l)
				}
			}
			fmt.Fprintf(&out, "[exit %d]\n\n", code)
		case strings.HasPrefix(line, "> "):
			cmd := exec.Command("sh", "-c", line[2:])
			cmd.Dir, cmd.Env = work, envList(env)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", line, err, b)
			}
		case strings.HasPrefix(line, "@set "):
			k, v, _ := strings.Cut(line[5:], "=")
			env[k] = os.Expand(v, func(k string) string { return env[k] })
		case strings.HasPrefix(line, "@unset "):
			delete(env, strings.TrimSpace(line[7:]))
		case strings.HasPrefix(line, "@env "):
			b, err := os.ReadFile(filepath.Join(work, strings.TrimSpace(line[5:])))
			if err != nil {
				t.Fatalf("%s: %v", line, err)
			}
			for _, l := range strings.Split(string(b), "\n") {
				if k, v, ok := strings.Cut(strings.TrimPrefix(l, "export "), "="); ok {
					env[k] = strings.Trim(v, "'")
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return normalize(out.String(), work, cli)
}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

var (
	reTime = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ`)
	reTask = regexp.MustCompile(`\bt-[0-9a-f]{6}\b`)
)

func normalize(s, work string, cli []string) string {
	s = strings.ReplaceAll(s, strings.Join(cli, " "), "<L>")
	s = strings.ReplaceAll(s, strings.Join(pythonCLI(), " "), "<L>")
	s = strings.ReplaceAll(s, work, "<W>")
	s = reTime.ReplaceAllString(s, "<TS>")
	return reTask.ReplaceAllString(s, "<TASK>")
}

// split is a small shell-words splitter: quotes and backslashes, no expansion.
func split(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord, quote := false, rune(0)
	for i := 0; i < len(s); i++ {
		c := rune(s[i])
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else {
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			inWord = true
		case c == ' ' || c == '\t':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out, nil
}

// diff shows the first differing lines with some context.
func diff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			from := max(0, i-6)
			ctx := strings.Join(w[from:i], "\n")
			return fmt.Sprintf("line %d\n...\n%s\n- %s\n+ %s", i+1, ctx, a, b)
		}
	}
	return "(no line difference)"
}
