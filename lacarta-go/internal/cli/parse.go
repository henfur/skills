// Package cli is lacarta's command line: the commands agents run. Its flags, output and exit
// codes are a contract (agents and the skill depend on them), kept identical to the prototype.
package cli

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

type optKind int

const (
	str optKind = iota
	flag
	list
	num
)

type Opt struct {
	Name     string // without the leading dashes
	Kind     optKind
	Required bool
	Choices  []string
	Help     string
	Metavar  string
}

type Pos struct {
	Name    string
	Many    bool // nargs="*"
	Int     bool
	Choices []string
	Help    string
}

type Cmd struct {
	Name string // "status", "spec submit"
	Help string
	Pos  []Pos
	Opts []Opt
	Run  func(*Args) error
}

// Args are the parsed command line.
type Args struct {
	Cmd     *Cmd
	Root    string
	Project string
	Agent   string
	strs    map[string]string
	flags   map[string]bool
	lists   map[string][]string
	pos     map[string][]string
	Out     io.Writer
	Err     io.Writer
}

func (a *Args) Str(name string) string    { return a.strs[name] }
func (a *Args) Has(name string) bool      { _, ok := a.strs[name]; return ok }
func (a *Args) Flag(name string) bool     { return a.flags[name] }
func (a *Args) List(name string) []string { return a.lists[name] }
func (a *Args) Positional(name string) string {
	if v := a.pos[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}
func (a *Args) Positionals(name string) []string { return a.pos[name] }
func (a *Args) Int(name string) int {
	n, _ := strconv.Atoi(a.strs[name])
	return n
}
func (a *Args) PosInt(name string) int {
	n, _ := strconv.Atoi(a.Positional(name))
	return n
}
func (a *Args) Printf(format string, v ...any) { fmt.Fprintf(a.Out, format, v...) }
func (a *Args) Println(v ...any)               { fmt.Fprintln(a.Out, v...) }

// UsageError is a malformed command line (exit 2, like argparse).
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

var globalOpts = []Opt{
	{Name: "root", Help: "Software project root (default: nearest folder with .lacarta/)."},
	{Name: "project", Help: "Project id from the console (default: the folder's .lacarta/, else $LACARTA_PROJECT, " +
		"which a registered session sets)."},
	{Name: "agent", Help: "Your agent id from `agent start` (default: $LACARTA_AGENT, else the orchestrator A-1). " +
		"Goes before the command: `lacarta --agent A-3 ask ...`."},
}

// errHelp asks the caller to print help and exit 0.
type errHelp struct{ cmd *Cmd }

func (errHelp) Error() string { return "help" }

func parse(cmds []*Cmd, argv []string) (*Args, error) {
	a := &Args{strs: map[string]string{}, flags: map[string]bool{}, lists: map[string][]string{}, pos: map[string][]string{}}
	i := 0
	// Global options come before the command.
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		name, val, hasVal := strings.Cut(strings.TrimLeft(argv[i], "-"), "=")
		if name == "h" || name == "help" {
			return nil, errHelp{}
		}
		if !slices.ContainsFunc(globalOpts, func(o Opt) bool { return o.Name == name }) {
			return nil, &UsageError{fmt.Sprintf("unrecognized arguments: %s", argv[i])}
		}
		if !hasVal {
			i++
			if i >= len(argv) {
				return nil, &UsageError{fmt.Sprintf("argument --%s: expected one argument", name)}
			}
			val = argv[i]
		}
		switch name {
		case "root":
			a.Root = val
		case "project":
			a.Project = val
		case "agent":
			a.Agent = val
		}
		i++
	}
	if i >= len(argv) {
		return nil, &UsageError{"the following arguments are required: cmd"}
	}
	// Longest command name that matches ("spec submit" before "spec").
	var cmd *Cmd
	for _, c := range cmds {
		words := strings.Fields(c.Name)
		if len(argv)-i >= len(words) && slices.Equal(argv[i:i+len(words)], words) {
			if cmd == nil || len(words) > len(strings.Fields(cmd.Name)) {
				cmd = c
			}
		}
	}
	if cmd == nil {
		group := argv[i]
		subs := []string{}
		for _, c := range cmds {
			if w := strings.Fields(c.Name); len(w) > 1 && w[0] == group {
				subs = append(subs, w[1])
			}
		}
		if len(subs) > 0 && (i+1 >= len(argv) || argv[i+1] == "-h" || argv[i+1] == "--help") {
			if i+1 < len(argv) {
				return nil, errHelp{&Cmd{Name: group, Help: "Subcommands: " + strings.Join(subs, ", ")}}
			}
			return nil, &UsageError{fmt.Sprintf("%s: the following arguments are required: sub (%s)", group, strings.Join(subs, ", "))}
		}
		return nil, &UsageError{fmt.Sprintf("invalid choice: %q", group)}
	}
	a.Cmd = cmd
	i += len(strings.Fields(cmd.Name))
	posIdx := 0
	for ; i < len(argv); i++ {
		tok := argv[i]
		if tok == "-h" || tok == "--help" {
			return nil, errHelp{cmd}
		}
		if tok == "--" && len(cmd.Pos) > 0 && cmd.Pos[len(cmd.Pos)-1].Many {
			// Everything after `--` belongs to the last positional, dashes included (`sandbox run -- cmd -x`).
			last := cmd.Pos[len(cmd.Pos)-1].Name
			a.pos[last] = append(a.pos[last], argv[i+1:]...)
			break
		}
		if strings.HasPrefix(tok, "--") && len(tok) > 2 {
			name, val, hasVal := strings.Cut(tok[2:], "=")
			idx := slices.IndexFunc(cmd.Opts, func(o Opt) bool { return o.Name == name })
			if idx < 0 {
				return nil, &UsageError{fmt.Sprintf("unrecognized arguments: %s", tok)}
			}
			o := cmd.Opts[idx]
			if o.Kind == flag {
				a.flags[name] = true
				continue
			}
			if !hasVal {
				i++
				if i >= len(argv) {
					return nil, &UsageError{fmt.Sprintf("argument --%s: expected one argument", name)}
				}
				val = argv[i]
			}
			if len(o.Choices) > 0 && !slices.Contains(o.Choices, val) {
				return nil, &UsageError{fmt.Sprintf("argument --%s: invalid choice: %q (choose from %s)", name, val, strings.Join(o.Choices, ", "))}
			}
			if o.Kind == num {
				if _, err := strconv.Atoi(val); err != nil {
					return nil, &UsageError{fmt.Sprintf("argument --%s: invalid int value: %q", name, val)}
				}
			}
			if o.Kind == list {
				a.lists[name] = append(a.lists[name], val)
			} else {
				a.strs[name] = val
			}
			continue
		}
		if posIdx >= len(cmd.Pos) {
			return nil, &UsageError{fmt.Sprintf("unrecognized arguments: %s", tok)}
		}
		p := cmd.Pos[posIdx]
		if len(p.Choices) > 0 && !slices.Contains(p.Choices, tok) {
			return nil, &UsageError{fmt.Sprintf("argument %s: invalid choice: %q (choose from %s)", p.Name, tok, strings.Join(p.Choices, ", "))}
		}
		if p.Int {
			if _, err := strconv.Atoi(tok); err != nil {
				return nil, &UsageError{fmt.Sprintf("argument %s: invalid int value: %q", p.Name, tok)}
			}
		}
		a.pos[p.Name] = append(a.pos[p.Name], tok)
		if !p.Many {
			posIdx++
		}
	}
	for _, p := range cmd.Pos[min(posIdx, len(cmd.Pos)):] {
		if !p.Many && len(a.pos[p.Name]) == 0 {
			return nil, &UsageError{fmt.Sprintf("the following arguments are required: %s", p.Name)}
		}
	}
	for _, o := range cmd.Opts {
		if o.Required && !a.Has(o.Name) && len(a.lists[o.Name]) == 0 {
			return nil, &UsageError{fmt.Sprintf("the following arguments are required: --%s", o.Name)}
		}
	}
	return a, nil
}

func usage(cmds []*Cmd, w io.Writer, intro string) {
	fmt.Fprintln(w, "usage: lacarta [--root ROOT] [--project PROJECT] [--agent AGENT] <command> ...")
	fmt.Fprintln(w)
	fmt.Fprintln(w, intro)
	fmt.Fprintln(w, "\ncommands:")
	for _, c := range cmds {
		fmt.Fprintf(w, "  %-18s %s\n", c.Name, c.Help)
	}
	fmt.Fprintln(w, "\nglobal options:")
	for _, o := range globalOpts {
		fmt.Fprintf(w, "  --%-16s %s\n", o.Name, o.Help)
	}
}

func cmdHelp(c *Cmd, w io.Writer) {
	line := "usage: lacarta " + c.Name
	for _, p := range c.Pos {
		if p.Many {
			line += " [" + p.Name + " ...]"
		} else {
			line += " " + p.Name
		}
	}
	for _, o := range c.Opts {
		v := strings.ToUpper(strings.ReplaceAll(o.Name, "-", "_"))
		if o.Metavar != "" {
			v = o.Metavar
		}
		part := "--" + o.Name
		if o.Kind != flag {
			part += " " + v
		}
		if !o.Required {
			part = "[" + part + "]"
		}
		line += " " + part
	}
	fmt.Fprintln(w, line)
	if c.Help != "" {
		fmt.Fprintln(w, "\n"+c.Help)
	}
	if len(c.Pos) > 0 || len(c.Opts) > 0 {
		fmt.Fprintln(w)
	}
	for _, p := range c.Pos {
		fmt.Fprintf(w, "  %-18s %s%s\n", p.Name, p.Help, choices(p.Choices))
	}
	for _, o := range c.Opts {
		fmt.Fprintf(w, "  --%-16s %s%s\n", o.Name, o.Help, choices(o.Choices))
	}
}

func choices(c []string) string {
	if len(c) == 0 {
		return ""
	}
	return " {" + strings.Join(c, ",") + "}"
}
