package desk

import (
	"path/filepath"
	"strings"

	"lacarta/internal/core"
)

// GuardCommand is the desk session's PreToolUse check of a Bash command: it refuses forge writes,
// switching or reading identities, and pushing. It returns why, or "" to let the command run.
//
// It is a second line, not the boundary: shell tricks can hide a command from any parser. The
// boundary is the read-only token the session holds (§5.3); this catches the honest mistakes and the
// plain injected ones, and logs them as refusals the user sees.
func GuardCommand(cmd string, cfg *core.DeskConfig) string {
	low := strings.ToLower(cmd)
	for _, bad := range []string{"glab_config_dir", "glab-cli", "gitlab_token", "private-token", "gitlab_access_token",
		"oauth_token", "--token"} {
		if strings.Contains(low, bad) {
			return "this desk session uses the read identity lacarta gave it; it doesn't switch, read or pass GitLab credentials (" + bad + ")"
		}
	}
	for _, dir := range []string{cfg.Forge.WriteConfig, cfg.Forge.ReadConfig} {
		if dir != "" && strings.Contains(cmd, dir) {
			return "this desk session doesn't touch the desk's identity folders"
		}
	}
	return guardSegments(cmd, cfg, 0)
}

func guardSegments(cmd string, cfg *core.DeskConfig, depth int) string {
	if depth > 3 {
		return "nested shells too deep to check"
	}
	for _, seg := range splitShell(cmd) {
		f := fields(seg)
		// Skip what runs the real program: env assignments and wrappers.
		for len(f) > 0 {
			w := f[0]
			if strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "=") {
				f = f[1:]
				continue
			}
			switch filepath.Base(w) {
			case "sudo", "env", "command", "exec", "nohup", "time", "nice", "stdbuf", "xargs", "builtin":
				f = f[1:]
				for len(f) > 0 && strings.HasPrefix(f[0], "-") {
					f = f[1:]
				}
				continue
			case "timeout":
				f = f[1:]
				for len(f) > 0 && (strings.HasPrefix(f[0], "-") || isDuration(f[0])) {
					f = f[1:]
				}
				continue
			}
			break
		}
		if len(f) == 0 {
			continue
		}
		prog := filepath.Base(f[0])
		switch prog {
		case "bash", "sh", "zsh", "dash", "fish", "eval":
			for i, a := range f {
				if (a == "-c" || prog == "eval") && i+1 < len(f) {
					if r := guardSegments(strings.Join(f[i+1:], " "), cfg, depth+1); r != "" {
						return r
					}
					break
				}
			}
		case "glab":
			if r := guardGlab(f[1:]); r != "" {
				return r
			}
		case "git":
			for _, a := range f[1:] {
				if a == "push" {
					return "the desk never pushes"
				}
				if !strings.HasPrefix(a, "-") {
					break
				}
			}
		case "curl", "wget", "http", "https", "xh", "httpie":
			if cfg.Forge.Host != "" && strings.Contains(seg, cfg.Forge.Host) {
				return "the desk session reads GitLab through glab (read identity) only, not " + prog
			}
		}
	}
	return ""
}

// Read-only glab subcommands the desk's agents may use.
var glabRead = map[string][]string{
	"mr":      {"view", "diff", "list", "checkout"},
	"repo":    {"clone", "view", "list"},
	"ci":      {"list", "view", "status", "trace", "get"},
	"issue":   {"view", "list"},
	"release": {"view", "list"},
}

func guardGlab(args []string) string {
	pos := []string{}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
		}
	}
	if len(pos) == 0 {
		return ""
	}
	sub := pos[0]
	switch sub {
	case "version", "help":
		return ""
	case "api":
		for i, a := range args {
			switch {
			case a == "-X" || a == "--method":
				if i+1 < len(args) && !strings.EqualFold(strings.Trim(args[i+1], `'"`), "GET") {
					return "the desk session only reads GitLab: glab api with method " + args[i+1] + " is refused"
				}
			case strings.HasPrefix(a, "--method=") || strings.HasPrefix(a, "-X") && len(a) > 2:
				m := strings.TrimPrefix(strings.TrimPrefix(a, "--method="), "-X")
				if !strings.EqualFold(strings.Trim(m, `'"=`), "GET") {
					return "the desk session only reads GitLab: glab api with method " + m + " is refused"
				}
			case a == "-f" || a == "-F" || a == "--field" || a == "--raw-field" || a == "--input" || a == "--form" ||
				strings.HasPrefix(a, "--field=") || strings.HasPrefix(a, "--raw-field=") || strings.HasPrefix(a, "--input=") ||
				strings.HasPrefix(a, "--form="):
				return "the desk session only reads GitLab: glab api with a request body (which makes it a write) is refused"
			}
		}
		if len(pos) > 1 && strings.Trim(pos[1], `'"`) == "graphql" {
			return "the desk session doesn't use GraphQL (it can write)"
		}
		return ""
	}
	if ok, known := glabRead[sub]; known && len(pos) > 1 {
		for _, x := range ok {
			if pos[1] == x {
				return ""
			}
		}
	}
	return "the desk session only reads GitLab: `glab " + strings.Join(pos[:min(2, len(pos))], " ") + "` is refused " +
		"(lacarta posts what the desk's policy allows)"
}

func isDuration(s string) bool {
	return strings.TrimRight(s, "0123456789.smhd") == "" && s != ""
}

// splitShell cuts a command line at ; && || | & newlines and command substitutions.
func splitShell(cmd string) []string {
	r := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n", "|", "\n", "&", "\n", "$(", "\n", "`", "\n", "(", "\n", ")", "\n")
	out := []string{}
	for _, s := range strings.Split(r.Replace(cmd), "\n") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// fields splits on blanks and drops quotes (a rough shell word split; enough for the checks above).
func fields(s string) []string {
	out := []string{}
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '\\':
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}
