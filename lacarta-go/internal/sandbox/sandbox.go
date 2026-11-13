// Package sandbox runs untrusted work (the audited target's code, check tools, the skill's
// validators) under bubblewrap: no network but loopback, an empty environment plus an
// allowlist, the host's home and other files hidden, the target and the toolchain read-only,
// writes only to one scratch folder, and resource limits. This is the OS-enforced sandbox the
// security-audit skill requires before any target code runs.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Spec is one sandboxed command.
type Spec struct {
	Scratch  string            // the only writable folder; HOME and TMPDIR live in it
	ReadOnly []string          // extra folders visible read-only (the target, a database)
	Env      map[string]string // allowlisted variables, on top of HOME, TMPDIR and PATH
	Dir      string            // working directory (default: Scratch)
	Timeout  time.Duration     // wall clock (default 10 minutes)
	MemMB    int               // address space (default 4096)
	FileMB   int               // largest file it may write (default 256)
	CPUSecs  int               // CPU time (default 600)
	Args     []string          // the command; the program is resolved on the host's PATH
}

// System folders made visible read-only when they exist. Nothing under /home, /root or /var.
var systemDirs = []string{"/nix", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/run/current-system", "/opt"}

func (s *Spec) defaults() {
	if s.Dir == "" {
		s.Dir = s.Scratch
	}
	if s.Timeout == 0 {
		s.Timeout = 10 * time.Minute
	}
	if s.MemMB == 0 {
		s.MemMB = 4096
	}
	if s.FileMB == 0 {
		s.FileMB = 256
	}
	if s.CPUSecs == 0 {
		s.CPUSecs = 600
	}
}

// Available says whether the sandbox can run here, and why not.
func Available() error {
	for _, p := range []string{"bwrap", "prlimit"} {
		if _, err := exec.LookPath(p); err != nil {
			return fmt.Errorf("%s is not installed", p)
		}
	}
	dir, err := os.MkdirTemp("", "lacarta-sandbox-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cmd, cancel, err := Command(context.Background(), &Spec{Scratch: dir, Args: []string{"true"}, Timeout: 20 * time.Second})
	if err != nil {
		return err
	}
	defer cancel()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bubblewrap can't create namespaces here: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resolve finds a program on the PATH. It keeps the name it was found under: multi-call
// binaries (coreutils, busybox) act on the name they are called by.
func resolve(prog string) (string, error) {
	p, err := exec.LookPath(prog)
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

// Command builds the sandboxed command. Call cancel when it is done.
func Command(ctx context.Context, s *Spec) (*exec.Cmd, context.CancelFunc, error) {
	s.defaults()
	if len(s.Args) == 0 {
		return nil, nil, errors.New("sandbox: no command")
	}
	scratch, err := filepath.Abs(s.Scratch)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(filepath.Join(scratch, "tmp"), 0o700); err != nil {
		return nil, nil, err
	}
	prog, err := resolve(s.Args[0])
	if err != nil {
		return nil, nil, fmt.Errorf("sandbox: %s: %w", s.Args[0], err)
	}
	bwrap, err := resolve("bwrap")
	if err != nil {
		return nil, nil, errors.New("sandbox: bwrap is not installed")
	}
	prlimit, err := resolve("prlimit")
	if err != nil {
		return nil, nil, errors.New("sandbox: prlimit is not installed")
	}
	args := []string{
		fmt.Sprintf("--as=%d", s.MemMB<<20), fmt.Sprintf("--fsize=%d", s.FileMB<<20),
		fmt.Sprintf("--cpu=%d", s.CPUSecs), "--nofile=1024", "--core=0", "--",
		bwrap, "--unshare-all", "--die-with-parent", "--new-session", "--clearenv",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	for _, d := range systemDirs {
		if _, err := os.Stat(d); err == nil {
			args = append(args, "--ro-bind", d, d)
		}
	}
	// The program, and what its symlink points to, may live outside the system folders.
	real, _ := filepath.EvalSymlinks(prog)
	for _, p := range []string{prog, real} {
		if p != "" && !under(p, systemDirs) {
			args = append(args, "--ro-bind", filepath.Dir(p), filepath.Dir(p))
		}
	}
	for _, d := range s.ReadOnly {
		abs, err := filepath.Abs(d)
		if err != nil {
			return nil, nil, err
		}
		args = append(args, "--ro-bind", abs, abs)
	}
	args = append(args, "--bind", scratch, scratch, "--chdir", s.Dir,
		"--setenv", "HOME", scratch, "--setenv", "TMPDIR", filepath.Join(scratch, "tmp"),
		"--setenv", "PATH", "/run/current-system/sw/bin:/usr/bin:/bin:"+filepath.Dir(prog), "--setenv", "LANG", "C.UTF-8")
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--setenv", k, s.Env[k])
	}
	args = append(append(args, "--", prog), s.Args[1:]...)
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	cmd := exec.CommandContext(ctx, prlimit, args...)
	cmd.Env = []string{} // bwrap clears it again; nothing from this process leaks either way
	cmd.WaitDelay = 5 * time.Second
	return cmd, cancel, nil
}

func under(p string, dirs []string) bool {
	for _, d := range dirs {
		if p == d || strings.HasPrefix(p, d+"/") {
			if _, err := os.Stat(d); err == nil {
				return true
			}
		}
	}
	return false
}

// Describe is what the skill wants recorded with a local check: the limits and the
// environment variable names, never their values from the host.
func Describe(s *Spec) string {
	s.defaults()
	names := []string{"HOME", "TMPDIR", "PATH", "LANG"}
	for k := range s.Env {
		names = append(names, k)
	}
	sort.Strings(names)
	return fmt.Sprintf("bubblewrap: no network (loopback only), empty environment plus %s; read-only: system folders, %s; "+
		"writable: %s only; limits: %d MB memory, %d MB per file, %d s CPU, %s wall clock",
		strings.Join(names, ", "), strings.Join(append([]string{}, s.ReadOnly...), ", "), s.Scratch,
		s.MemMB, s.FileMB, s.CPUSecs, s.Timeout)
}
