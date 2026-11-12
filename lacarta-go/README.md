# lacarta (Go)

The Go port of lacarta: one binary for the CLI, the hooks, the daemon and the console. Plan and progress: [`../docs/go-port-plan.md`](../docs/go-port-plan.md).

**Status: phases 1 and 2 done.** The binary replaces the Python prototype (`../lacarta/scripts/lacarta.py`) entirely: every command, every hook, `install`, and the console daemon, which serves the prototype's page unchanged until the Svelte console (phase 3).

To switch a machine from the prototype: stop its console, then `go build -o ~/.local/bin/lacarta ./cmd/lacarta && lacarta install` (replaces the prototype's hooks), and restart running sessions. State files are shared, so projects carry over as they are.

```bash
go build -o lacarta ./cmd/lacarta
./lacarta --help
```

## Parity with the prototype

`internal/golden` runs the scenarios in `testdata/scenarios/` and compares the output with recordings made from the prototype:

```bash
go test ./...                                                   # check the Go binary
GOLDEN_IMPL=python GOLDEN_RECORD=1 go test ./internal/golden    # re-record from the prototype
```

Scenario lines: `$ <args>` runs the implementation under test, `% <args>` always runs the prototype (Go and Python then alternate on the same state files), `> <shell>` prepares files, `@env <file>` loads `export K=V` lines. Timestamps, task ids, paths and the CLI's own path are normalised.

Re-record only when the prototype's behaviour changes on purpose, and read the diff of `testdata/golden/` before committing it: the recordings are the specification.

## Layout

```
cmd/lacarta/       main
internal/core/     state files, locking, migration, hub, settings, questions, gates, sessions
internal/cli/      argument parsing and commands (output is a contract: agents read it)
internal/hooks/    hook events, transcript reader, install (Claude Code)
internal/server/   console daemon and API
internal/golden/   parity suite
skill/             embedded files (spec templates now; the whole skill in phase 4)
web/               embedded console (the prototype's page, copied, until phase 3)
testdata/          scenarios and recordings
```

The spec templates in `skill/templates/` and the page in `web/legacy/` are copies of the prototype's files until phase 4 removes it; change both.

Besides the golden suite: `internal/hooks` tests the listening Stop hook (woken by a message, an answer or a "Talk about this" message; ended by remote control off or SIGTERM), the transcript reader and install; `internal/server` runs the prototype's daemon and the Go handler on the same state files and compares their JSON, then tests every action and the auth checks.
