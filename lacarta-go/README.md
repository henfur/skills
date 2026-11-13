# lacarta (Go)

The Go port of lacarta: one binary for the CLI, the hooks, the daemon and the console. Plan and progress: [`../docs/go-port-plan.md`](../docs/go-port-plan.md). What to do next: [`../docs/next-steps.md`](../docs/next-steps.md). Specification of the system: [`docs/specs/`](docs/specs/) (functional, architecture, stack, security).

**Status: phases 1 to 3 done.** The binary replaces the Python prototype (`../lacarta/scripts/lacarta.py`) entirely: every command, every hook, `install`, and the console daemon with its Svelte console. The prototype's page is still served at `/legacy/` until the new console has been checked by hand.

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
internal/plugins/  plugin registry, the security-audit skill as a required plugin, preflight, validators, check tools
internal/sandbox/  bubblewrap sandbox for target code, check tools and validators
internal/auditreport/  audit report (markdown, print HTML, PDF), approval, verification
internal/provenance/   Ed25519 report signing in OpenSSH's SSHSIG format
internal/golden/   parity suite
skill/             embedded files (spec templates now; the whole skill in phase 4)
web/app/           console source (Svelte 5, Vite, Bun); tests in web/app/tests/
web/dist/          its build, committed and embedded
web/legacy/        the prototype's page, served at /legacy/ until removed
testdata/          scenarios and recordings
```

The spec templates in `skill/templates/` and the page in `web/legacy/` are copies of the prototype's files until phase 4 removes it; change both.

Besides the golden suite: `internal/hooks` tests the listening Stop hook (woken by a message, an answer or a "Talk about this" message; ended by remote control off or SIGTERM), the transcript reader and install; `internal/server` runs the prototype's daemon and the Go handler on the same state files and compares their JSON, then tests every action and the auth checks.

## Security audits

`lacarta audit init --target <repo>` starts an audit after a preflight (Cloudflare's security-audit skill, node, bubblewrap, the
skill's validator tests); `lacarta audit guide` is what the orchestrator agent follows. The auditor approves the rules of
engagement and the coverage plan, reviews every finding and approves the report in the console; the final report is signed
and `lacarta audit verify <final/vN>` checks it. Design and status: [`docs/design/security-audit.md`](docs/design/security-audit.md).
`internal/cli/audit_test.go` drives a whole audit with the real validators (skipped on machines that can't run audits).

## Console

The console's source is `web/app/`; its build in `web/dist/` is committed, so `go build` needs no JavaScript tooling. After changing the console, rebuild it and commit both:

```bash
cd web/app
nix shell nixpkgs#bun -c bun install
nix shell nixpkgs#bun -c bun run build        # writes ../dist
```

While working on it, serve the build from disk instead of rebuilding Go each time: run `bun run build --watch` and, on a scratch home and port, `LACARTA_HOME=$(mktemp -d) LACARTA_WEB_DIR=web/dist lacarta serve --port 4799`.

Smoke tests (Playwright, Chromium from nixpkgs; a daemon seeded through the CLI and hooks by `tests/seed.sh`):

```bash
BR=$(nix build --no-link --print-out-paths nixpkgs#playwright-driver.browsers)
nix shell nixpkgs#bun -c env PLAYWRIGHT_BROWSERS_PATH=$BR PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true bunx playwright test
```

