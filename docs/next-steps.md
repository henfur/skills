# Lacarta: next steps (handoff)

Written 2026-11-12, for the agent taking over the Go port. Read in this order:

1. This file: where things stand and what to do next.
2. [`../lacarta-go/docs/specs/`](../lacarta-go/docs/specs/): the specification of the system, in lacarta's own four-spec format. [`functional.md`](../lacarta-go/docs/specs/functional.md) lists every requirement as done (with the test that proves it) or to do. [`architecture.md`](../lacarta-go/docs/specs/architecture.md) gives the state files field by field, the hook protocol, the HTTP API shapes and the design decisions. [`stack.md`](../lacarta-go/docs/specs/stack.md) covers tools and environments. [`security.md`](../lacarta-go/docs/specs/security.md) is the threat model, with requirements mapped to tests.
3. [`go-port-plan.md`](go-port-plan.md): why the port is shaped this way.
4. [`../lacarta-go/README.md`](../lacarta-go/README.md): building and testing.

## Where things stand

| Phase | State |
| --- | --- |
| 0. Check Claude Code assumptions | Mostly settled; two unknowns have fallbacks (see the plan's table) |
| 1. State, gates, CLI in Go | Done |
| 2. Hooks, install, daemon API in Go | Done |
| 3. Svelte console | Done, except removing the legacy page after the user has checked the new one |
| 4. Skill split, releases, retire Python | **Next** |
| 5. `Kind` interface + security-audit kind | After 4 |

- `lacarta-go/` is a complete replacement for the Python prototype `lacarta/scripts/lacarta.py`: same commands, hooks, API and state files. Its console is the prototype's page, embedded unchanged from `lacarta-go/web/legacy/`.
- **Committed** as `99798da` ("rewrite the lacarta program and cli in go"). Since then, only the docs have changed (this file, the plan, the Go README, `lacarta-go/docs/`). Commit only when the user asks.
- **The user's machine runs the Go binary**: `~/.local/bin/lacarta`, with hooks in `~/.claude/settings.json` pointing at it and the console on `127.0.0.1:4747`. Every Claude Code session on the machine, including yours, goes through it. A broken build installed there breaks the user's sessions, so test with `go test ./...` and a scratch home before replacing that binary, and only replace it when the user asks.

## Before you change anything

```bash
cd lacarta-go
go vet ./... && go test ./...          # all green at handoff
```

- `internal/golden` replays `testdata/scenarios/*.txt` and compares the output with `testdata/golden/*.out`, which were recorded from the Python prototype. **The recordings are the behavioural spec.** If a test fails after your change, your change broke parity, unless the change is intended. Then re-record, read the diff of `testdata/golden/` line by line, and say in your summary what changed and why.
- `internal/hooks` tests the listening Stop hook, the transcript reader and `install`.
- `internal/server` starts the **Python** daemon and the Go handler on the same state files and compares their JSON (`TestAPIParity`), then tests actions and auth.

## Rules that must hold

- **One implementation of the rules.** CLI, hooks and daemon all go through `internal/core`. Don't add gate logic to the server or the frontend.
- **Agent-facing output is a contract.** Agents parse `Next: ...`, the `wait` JSON, `Refused: ...` on stderr with exit 2, and `ask` printing only the id. Don't reword them.
- **State files stay compatible.** Keep field names; key order matters in two places (the `specs` map and the hub's maps, both `core.OMap`), because the prototype lists them in file order. Go structs drop fields they don't know: add a field to the struct before anything writes it.
- **Hooks never fail the session.** They log to `~/.lacarta/hooks.log` and exit 0.
- **The prototype is frozen.** Don't add features to `lacarta.py`. If behaviour must change before phase 4, change Go, re-record the affected scenarios with Go (see "After the prototype is gone" below), and note it.

## Phase 3: Svelte console (done, one step left)

Built in `lacarta-go/web/app/` (Svelte 5.57, Vite 6.4, built with Bun 1.4 from nixpkgs), output committed in `web/dist/` and embedded, so `go build` needs no Bun. What changed, beyond porting every view:

- **Same look**: `src/app.css` is the legacy page's stylesheet, unchanged, plus a short "Fixes" block at the end. Components emit the same markup and classes.
- **Token in `<meta name="lacarta-token">`**: the page has no inline script, so `/` is served with `script-src 'self'` (the legacy page keeps its own CSP at `/legacy/`).
- **Live updates**: `GET /api/events` (`internal/server/events.go`): a watcher compares state-file modification times every 500 ms while someone is subscribed and sends `hub` and `project` events; the key may come as `?key=` because EventSource sends no headers. The page re-fetches on each event and polls every 15 s while the stream is healthy, every 2.5 s otherwise.
- **Fixed on the way**: drafts are keyed by project and question (question ids repeat across projects); a long session folder no longer widens the chat at phone width; the page no longer fetches the state of a project the hub doesn't list.
- **Development**: `LACARTA_WEB_DIR=web/dist lacarta serve --port 4799` (scratch `LACARTA_HOME`) serves the build from disk; `bun run build --watch` in `web/app/` then needs no Go rebuild.
- **Tests**: `web/app/tests/console.spec.js`, 10 Playwright smoke tests in Chromium against a daemon seeded by `tests/seed.sh` through the real CLI and hooks: home, answering, triage with Talk about this, chat, settings, archive, live updates with a toast, Mermaid, every view, dark theme and 375 px width. Every test fails on any console error, which includes CSP violations. Run them on NixOS with:

  ```bash
  cd lacarta-go/web/app
  BR=$(nix build --no-link --print-out-paths nixpkgs#playwright-driver.browsers)
  nix shell nixpkgs#bun -c env PLAYWRIGHT_BROWSERS_PATH=$BR PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true bunx playwright test
  ```

  `@playwright/test` is pinned to 1.64.1, the version of nixpkgs' `playwright-driver`; keep them equal.

**Left to do:**
1. **The user checks the new console** in their own browser (a rebuilt binary on their machine, or a scratch daemon). The automated tests ran Chromium only: Firefox and Safari are unchecked, and nobody has looked at it with human eyes yet.
2. Then **remove the legacy page**: delete `web/legacy/`, the `Legacy` embed, the `/legacy/` and `/vendor/` routes and `cspLegacy` in `internal/server/server.go`, and the legacy assertions in `TestPageHasToken` / `TestAuth` (vendored scripts). `web/legacy/index.html` is still the reference for behaviour until then.

## Phase 4: skill split, releases, retire Python

1. Move the skill into `lacarta-go/skill/`: `SKILL.md`, `references/*.md`, `templates/*.md` from `lacarta/`, with every `python3 $L ...` / `python3 <path>/lacarta.py` rewritten to `lacarta ...`. The session context already tells agents the binary's absolute path (`hooks.Self`).
2. `lacarta install` also writes the embedded skill to `~/.claude/skills/lacarta`. **Careful:** on the user's machine that path is a symlink to this repo's `lacarta/` folder. Don't overwrite a symlink or a folder lacarta didn't write: detect it, say so, and offer `--force`.
3. Release builds for linux and macOS on amd64 and arm64 (goreleaser, or a Makefile with `GOOS`/`GOARCH`), plus an install script that downloads the binary and runs `lacarta install`.
4. **After the prototype is gone:** delete `lacarta/scripts/`, `lacarta/assets/` and the copies they fed. The golden suite keeps working against the frozen recordings, except:
   - `%` lines (always the prototype) in `testdata/scenarios/interop.txt`;
   - `PYLACARTA` hook calls in `single_task.txt` and `single_to_software.txt`;
   - the Python one-liners in `hooks.txt` that queue console messages.

   Replace them with Go equivalents (`$ hook ...` lines; a hidden `lacarta debug message <text>` command, or a small Go helper in the harness for queueing messages), re-record **with Go**, and check the diff is empty apart from those lines. `TestAPIParity` needs Python: skip it when `lacarta.py` is missing, or turn it into a golden JSON comparison.
5. Update the repo `README.md`: install via the binary, the console, remote control.

Exit: a clean machine goes from install to an approved phase 1 of a new project, with no Python.

## Phase 5: `Kind` interface and the security-audit kind

**Design draft:** [`lacarta-go/docs/design/security-audit.md`](../lacarta-go/docs/design/security-audit.md) covers the audit kind built on [cloudflare/security-audit-skill](https://github.com/cloudflare/security-audit-skill) (required to start an audit), plugins enabled per project, and the implementation order. The user decided O-2 to O-5 (output under `~/.lacarta/audits/`, coverage-plan review always, no fixing: the deliverable is a report the human auditor approves, gitleaks and osv-scanner as day-one plugins). Then O-6 and O-7 (the auditor signs with their name; severity changes appear in the report) and an AI-assistance disclosure with tamper-evident provenance (§4.8). Still open: O-1 (technical). O-8 and O-9 are parked: see "Later improvements" below.

Phase 1 kept the prototype's shape: functions in `internal/core/workflow.go` and `internal/cli/commands.go` branch on `st.Template`. Extract the interface sketched in the plan (`Roles`, `Commands`, `Refresh`, `NextAction`, `Derived`) once a second real kind shows what it must cover. Build the security-audit kind on it, with its own skill instructions and console views. Exit: added without editing `state`, `hub`, `hooks` or `server` code paths beyond registering the kind.

## Later improvements: report provenance

Both are parked by the user for later. They build on the security-audit design's §4.8 (AI-assistance disclosure and provenance);
the first version ships without them: disclosure in the template, PDF watermark, XMP/front-matter marking and the signed
manifest (`report.provenance.json`, Ed25519 in SSHSIG format).

1. **O-8: public timestamp of approved reports.** At approval, record the provenance manifest's hash in a public transparency
   log (Sigstore Rekor) or get an RFC 3161 timestamp from a timestamp authority. It proves, independently of the auditor, that a
   report with this id and `ai_assisted: true` existed at that time; it is the only layer an auditor can't redo on their own
   machine. It sends a hash and the public key off the machine (never the report), so it is outward-facing: to decide with the
   user whether it is off, on by default or required, and to confirm before the first use.
2. **O-9: PDF certification signature without a dependency.** A PAdES certification signature with DocMDP level 1 ("no
   changes allowed") makes PDF readers flag any later edit; with a self-signed certificate the signer shows as untrusted but
   changes are still detected. It needs CMS (PKCS#7) SignedData, which the Go standard library lacks. Proposed: implement the
   small subset with `encoding/asn1` and `crypto/x509` rather than take the binary's first third-party dependency. Test the result
   in the readers clients use (Adobe Reader at least): validators differ on incremental updates such as later timestamps.
   Revisit **C2PA Content Credentials** at the same time: it has a PDF binding, but the reference SDK documented PDF as read
   support only when this was designed.

Sources:
- [EU AI Act Article 50: a practical guide](https://artificialintelligenceact.eu/transparency-rules-article-50/) and
  [Orrick: EU AI Act transparency obligations for AI-generated content](https://www.orrick.com/en/insights/2026/08/EU-AI-Act-Transparency-Obligations-for-AI-Generated-Content-Article-50)
  (machine-readable marking; recital 133 lists watermarks, metadata, cryptographic provenance and logging as techniques).
- [Adobe: certificate-based signatures](https://helpx.adobe.com/acrobat/current/certificate-based-signatures.html),
  [Nutrient: certify a PDF document](https://www.nutrient.io/guides/java/editor/certify-a-pdf-document/) and
  [Attacks on PDF certification, and what you can do about them](https://idm.net.au/node/13529) (DocMDP levels, self-signed
  certificates, known viewer differences).
- [C2PA specification 2.3](https://spec.c2pa.org/specifications/specifications/2.3/specs/_attachments/C2PA_Specification.pdf),
  [C2PA Rust SDK](https://docs.rs/crate/c2pa/0.85.2/source/src/lib.rs) and
  [Content Authenticity for PDF (PDF Association)](https://pdfa.org/wp-content/uploads/2021/05/Content-Authenticity-for-PDF.pdf).
- [Build your own vulnerability harness (Cloudflare)](https://blog.cloudflare.com/build-your-own-vulnerability-harness) and
  [cloudflare/security-audit-skill](https://github.com/cloudflare/security-audit-skill), the basis of the audit design.

## Open decisions for the user

- **The console key in transcripts:** when the console listens beyond loopback, the session context includes the URL with `?key=`, so it ends up in `~/.claude` transcripts. Leave it there, or put the URL without the key in the context.
- **Several sessions on one software project:** today the last session to start becomes A-1's session. Decide before the security-audit kind, where parallel agents are likely.
- **Remote control on by default:** it no longer holds the terminal. The agent runs `lacarta listen` as a background command, which wakes it when the console has something; revisit if that surprises the user.

## Known gaps and gotchas

- **A hook starting the daemon by itself** (`server.EnsureDaemon`) has no automated test: it always uses port 4747, where the user's console runs. The user's own switch to the Go binary exercised it in practice (their daemon was started that way or by hand; check `~/.lacarta/daemon.log`). Add a `LACARTA_PORT` override before writing a test for it.
- **Usage errors** (bad flags) are worded differently from Python's argparse. They aren't a contract and aren't in the golden suite.
- **Session pruning** keeps the newest 500 entries in `hub.json`; the order is the file's insertion order (`core.OMap`).
- **Test environment:** the machine's `python3` is a wrapper that needs the real `HOME`. Tests isolate lacarta with `LACARTA_HOME` and `CLAUDE_CONFIG_DIR` instead, and set `LANG=C.UTF-8`, without which Python mangles non-ASCII arguments.
- **Don't use `pkill -f lacarta`** (or `lacarta.py`): it matches your own shell command, and the user's live daemon and hooks, and kills them. Stop daemons by the pid in `$LACARTA_HOME/daemon.json`.
- **Don't run the real console on 4747** or touch `~/.lacarta` in tests: the user's live sessions use them. Use a temp `LACARTA_HOME` and another port.
- **Talking to the user:** this session is registered in the lacarta console with remote control on. The user often writes from the console; those messages arrive as Stop hook feedback starting with `[lacarta console]` and are the user's own words. Claude Code labels them "Stop hook blocking error"; that label is cosmetic.
