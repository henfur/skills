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
internal/core/     state files, locking, migration, hub, config files, questions, gates, sessions
internal/cli/      argument parsing and commands (output is a contract: agents read it)
internal/hooks/    hook events, transcript reader, install (Claude Code)
internal/server/   console daemon and API
internal/desk/      review desks: poll, claim, briefs, records, plans, publishing, the session's command guard
internal/forge/     GitLab through glab (and forgetest, a fake glab for tests)
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

## Review desks

A review desk watches merge requests on one GitLab instance and reviews them on a loop: `lacarta desk init` makes a
folder a desk (its `.lacarta/config.json` "desk" key holds the forge, targets, profiles with their modes, policy and
schedule; `instructions/` and `templates/` are the team's), `lacarta desk guide` is what the orchestrator agent follows.
Agents read GitLab with a read-only identity and write records; lacarta posts what the policy and the profiles' modes
(shadow by default) allow, and the user publishes the rest from the console. It never merges. Both identities are glab
config folders inside this environment's own config folder. Design and status: [`docs/design/pr-review.md`](docs/design/pr-review.md).
`internal/cli/desk_test.go` drives whole desks against a fake GitLab (the test binary acts as `glab`).

## Configuration

Two JSON files. Built-in defaults < `~/.lacarta/config.json` < the project's `config.json`: `<repo>/.lacarta/config.json`
for a software project; for a single-agent task, `.lacarta/config.json` at the root of the git repository it works in (else
in its folder), shared by every task there; for an audit, beside its state under `~/.lacarta/audits/`, never in the audited code.

The project's file holds its `id`, a random id the console knows it by (`t-3f9a1c` for a task, `<slug>-<6 hex>` for a
software project), so a project is tracked by its folder: a new session in that folder (or its git repository) continues
the same task, even after it ended or was archived; a moved folder keeps its project; a copied folder gets an id of its
own. A second session started while the folder's task still has a live one (written in the last 30 minutes) is a task
of its own; a resumed conversation always rejoins. Leave `id` as it is.

The project's file also names the project. A session's first registration in a folder, and `init`, create it with
`{"name": "<git repository's name, else the folder's>"}` (`init --name` writes that name); edit it to rename the project
everywhere. Tasks are no longer named after their first prompt. A session in the home folder gets no file:
`~/.lacarta` is lacarta's own.
`lacarta config` shows what is in effect and where each value comes from, `lacarta config check` lists what is ignored,
`lacarta config init [--project]` creates a file to fill in. Keys starting with `//` are comments.

```json
{
  "models": {"software": {"implementation": "sonnet"}, "audit": {"hunter": "opus"}},
  "remote_control": true,
  "auditor": "Ada Lovelace",
  "templates": "~/lacarta-templates",
  "plugins": {"gitleaks": true, "osv-scanner": false},
  "tools": {"chromium": "/opt/chromium/chrome", "node": "~/.local/bin/node"}
}
```

| Key | Home | Project | |
| --- | --- | --- | --- |
| `id` | refused | set by lacarta | The project's id in the console. |
| `name` | refused | the project's name | Shown everywhere instead of the name in the state. |
| `models` | `{kind: {role: model}}` | `{role: model}` | Read before every spawn. `models --set role=model [--project]` writes them; the console's Settings page writes the home ones. |
| `remote_control` | default for every session | default for this project | The Chat toggle of a project still wins. |
| `templates` | a folder | a folder inside the project | Spec templates `init` copies (`functional.md`, `architecture.md`, `stack.md`, `security.md`); a missing file falls back to the next layer, then the built-in one. |
| `desk` | refused | a review desk's settings | Forge, targets, filters, profiles and their modes, policy, schedule (see the desk design). |
| `plugins` | on/off in new projects | on/off, locked in the console | The project's Plugins tab can't change a plugin its file sets. Required plugins stay on. |
| `tools` | program paths | refused | `git`, `node`, `bwrap`, `prlimit`, `gitleaks`, `osv-scanner`, `chromium`, `glab`; anything unset is found on `PATH`. |
| `auditor` | the name on signed audit reports | refused | |

A software project's file lives in its repository, so a cloned repository could ship one: it can't choose which programs
lacarta runs or who signs, and its templates folder must stay inside the project. An invalid value is skipped with a
warning (shown by `config check` and in the console) rather than stopping a session. The console's saves keep every key
they don't change. A `~/.lacarta/settings.json` from before is read until the first save, which moves it into `config.json`.

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

