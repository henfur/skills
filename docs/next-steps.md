# Lacarta: next steps (handoff)

Last updated 2026-11-19. Read in this order:

1. This file: where things stand and what to do next.
2. [`../lacarta-go/docs/specs/`](../lacarta-go/docs/specs/): the specification of the system, in lacarta's own four-spec format. [`functional.md`](../lacarta-go/docs/specs/functional.md) lists every requirement as done (with the test that proves it) or to do. [`architecture.md`](../lacarta-go/docs/specs/architecture.md) gives the state files field by field, the hook protocol, the HTTP API shapes and the design decisions (AD-6: kinds of project). [`stack.md`](../lacarta-go/docs/specs/stack.md) covers tools and environments. [`security.md`](../lacarta-go/docs/specs/security.md) is the threat model, with requirements mapped to tests.
3. [`../lacarta-go/README.md`](../lacarta-go/README.md): building, installing and testing.

## Where things stand

| Phase | State |
| --- | --- |
| 1. State, gates, CLI in Go | Done |
| 2. Hooks, install, daemon API in Go | Done |
| 3. Svelte console | Done; the prototype's page was removed on 2026-11-19 |
| 4. Skill in the binary, retire Python | Done 2026-11-19, except release builds (below) |
| 5. `Kind` interface + security-audit kind | Done (audit 2026-11-13, interface 2026-11-19) |
| 6. Review desk kind | First version, not yet run against a real GitLab |
| 7. Incident-response kind | First version |

- `lacarta-go/` is the whole program: CLI, hooks, daemon, console, and the skill agents read (`lacarta-go/skill/`). The Python prototype is gone; its state files, hooks and projects carry over.
- **The user's machine runs the Go binary**: `~/.local/bin/lacarta`, with hooks in `~/.claude/settings.json` pointing at it and the console on `127.0.0.1:4747`. Every Claude Code session on the machine, including yours, goes through it. A broken build installed there breaks the user's sessions, so test with `go test -count=1 ./...` and a scratch home before replacing that binary, and only replace it when the user asks. Commit only when the user asks.
- `~/.claude/skills/lacarta` on the user's machine is a link to `lacarta-go/skill/` in this checkout (for development). `lacarta install` leaves a link alone; `install --force` replaces it with a copy that sessions keep in step with the binary.

## Before you change anything

```bash
cd lacarta-go
go vet ./... && go test -count=1 ./...          # all green
```

- `internal/golden` replays `testdata/scenarios/*.txt` and compares the output with `testdata/golden/*.out`; `internal/server` compares the console API with `testdata/golden/api/*.json`. The first recordings came from the Python prototype. **The recordings are the behavioural spec.** If a test fails after your change, your change altered behaviour agents or the console rely on, unless it is intended. Then re-record (`GOLDEN_RECORD=1 go test -count=1 ./internal/golden ./internal/server`), read the diff line by line, and say in your summary what changed and why.
- `internal/hooks` tests the listening Stop hook, the transcript reader, `install` and the installed skill.
- The console: `web/app/tests/`, Playwright against a daemon seeded by `tests/seed.sh` (see the Go README for the NixOS browser path).

## Rules that must hold

- **One implementation of the rules.** CLI, hooks and daemon all go through `internal/core`. Don't add gate logic to the server or the frontend.
- **A kind's behaviour goes through its registration** (architecture spec, AD-6): `core.Behaviour` for stages, the `Next:` line and derived fields; `kindBriefs`, `kindPayloads`, `kindStatus` and the Svelte `views` for the layers above. Don't add `if st.Template == ...` dispatch elsewhere.
- **Agent-facing output is a contract.** Agents parse `Next: ...`, the `wait` JSON, `Refused: ...` on stderr with exit 2, and `ask` printing only the id. Don't reword them.
- **State files stay compatible.** Keep field names; key order matters in two places (the `specs` map and the hub's maps, both `core.OMap`). Go structs drop fields they don't know: add a field to the struct before anything writes it.
- **Hooks never fail the session.** They log to `~/.lacarta/hooks.log` and exit 0.
- **The skill matches the binary.** `lacarta-go/skill/SKILL.md` and `references/` are embedded and installed by `lacarta install`; change them together with the commands they describe.

## Phase 3: Svelte console (done)

Built in `lacarta-go/web/app/` (Svelte 5.57, Vite 6.4, built with Bun 1.4 from nixpkgs), output committed in `web/dist/` and embedded, so `go build` needs no Bun. The page has no inline script (token in `<meta name="lacarta-token">`, `script-src 'self'`); live updates come over `GET /api/events` (`internal/server/events.go`). Development: `LACARTA_WEB_DIR=web/dist lacarta serve --port 4799` with a scratch `LACARTA_HOME`, and `bun run build --watch` in `web/app/`. `@playwright/test` is pinned to the version of nixpkgs' `playwright-driver`; keep them equal. Firefox and Safari are unchecked.

## Phase 4: skill in the binary, Python retired (done, releases left)

- The skill is `lacarta-go/skill/SKILL.md` and `references/`, embedded (`skill.Agent`). Commands are written `lacarta ...`; subagents get `{L}` = the path `agent start` prints plus `--agent <id>`.
- `lacarta install` writes it to `~/.claude/skills/lacarta` with a `.lacarta-skill` mark holding its hash; every session start rewrites a marked copy when the binary carries another version (`hooks.SyncSkill`). A link or a folder without the mark is never touched without `--force`. `install --uninstall` removes the hooks and a marked skill.
- The golden suite and the API test no longer run Python: `%` lines became `$` lines, the prototype's hooks became the binary's (`$L`), and two harness directives replace the Python one-liners (`@message`, `@old-models`). Re-recording moved only the `%`/`$` markers and A-1's name in two scenarios (the prototype's hook had named it Ada; the binary names the orchestrator Jerry on purpose).
- `lacarta/` (prototype, its page, its copies of the templates) is deleted.

**Left:** release builds for linux and macOS on amd64 and arm64 (goreleaser, or a Makefile with `GOOS`/`GOARCH`), and an install script that downloads the binary and runs `lacarta install`. Exit: a clean machine goes from install to an approved phase 1 of a new project.

## Phase 5: `Kind` interface and the security-audit kind (done)

**Audit kind (2026-11-13):** the security-audit kind, plugins, the sandbox, auditor review and the signed report. Design and what's left: [`lacarta-go/docs/design/security-audit.md`](../lacarta-go/docs/design/security-audit.md), built on [cloudflare/security-audit-skill](https://github.com/cloudflare/security-audit-skill). The user decided O-2 to O-7 and the AI-assistance disclosure with tamper-evident provenance (§4.8). Still open: O-1 (technical). O-8 and O-9 are parked: see "Later improvements" below.

**Interface (2026-11-19):** `internal/core/kinds.go`. Each kind is a catalog entry plus a `Behaviour` (`Refresh`, `Next`, `Derive`); the hooks, server and CLI register their per-kind parts in maps keyed by kind. Behaviour is unchanged: no golden recording moved. The per-command kind checks (`auditState`, `deskState`, `incidentRead`, `softwareOnly`) stay in each kind's commands; their refusal messages differ slightly by kind, and `softwareOnly` still calls every other kind "a single-agent task" (worth fixing with the golden diff it brings).

## Phase 6: the review kind (MR / PR review desk)

**First version implemented (2026-11-17), not committed, not yet run against a real GitLab:** [`lacarta-go/docs/design/pr-review.md`](../lacarta-go/docs/design/pr-review.md).
A desk is a folder with its own instructions, skills, tools and comment templates. It watches projects and groups on one
GitLab instance and reviews new and updated MRs on a loop: one orchestrator session, one subagent per profile (security,
devops, code). Agents get a read-only identity and write records; lacarta posts from templates according to a per-profile
mode (shadow by default, set in the desk's `config.json`) and an action policy. Never merges. Kind id `desk`, commands
`lacarta desk …`. The design doc's "Implementation status" lists what was built, the differences and what is left. The
`Kind` interface is still not extracted (the desk branches like the audit): do it next, with three kinds to cover.
**The binary in `~/.local/bin` is not rebuilt**: replace it only when the user asks.
**Open security item O-3** (design doc §13): agents run as the same OS user as the publisher, so the write identity
isn't isolated from them. The user chose to record it and not act now; proposed fix: a separate OS user for publishing.

## Phase 7: the incident-response kind

**First version implemented (2026-11-18), not committed:** [`lacarta-go/docs/design/incident-response.md`](../lacarta-go/docs/design/incident-response.md).
One case per folder, for a security incident or a vulnerability to respond to: timeline, investigations (with a
challenger for confirmed conclusions), evidence with hashes, indicators, response actions approved by the lead in the
console, situation reports along the way and a final report that closes the case. Kind id `incident`, commands
`lacarta incident …`. The design doc lists the gates, four decisions taken without asking and what is left. Go tests
and the 18 Playwright tests pass; the golden recordings did not move. **The binary in `~/.local/bin` is not rebuilt.**

## Skills per role, config files as the only source (done)

- `skills` in both config files gives each role its skills, beside `models`: `all`, per kind, per role; a project's file adds its own and `-name` drops one. Code: `internal/core/skills.go` (parsing, layering), `internal/plugins/agentskills.go` (finding them, the paragraph for prompts). README: "Skills per role".
- The console no longer changes models or plugins: Settings shows each role's model and skills, the audit's Plugins tab is read-only, `POST /api/settings` refuses `models`, `skills` and `plugins`, and `plugin enable|disable` are gone. The state's `plugins.<id>.enabled` is ignored; only the pinned `ref` is used.
- Left as it is: `models` still prints "(console Settings page)" in its header, because the golden recordings pin that line. Change it together with the recordings.
- Not done: a desk profile's own `skills` are still folders inside the desk (`desk.profiles.<name>.skills`), separate from the role lists; a project view of the effective skills in the console (`lacarta skills` shows them).

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
- **Usage errors** (bad flags) aren't a contract and aren't in the golden suite.
- **Session pruning** keeps the newest 500 entries in `hub.json`; the order is the file's insertion order (`core.OMap`).
- **Test environment:** tests isolate lacarta with `LACARTA_HOME` and `CLAUDE_CONFIG_DIR` (never the real `~/.lacarta` or `~/.claude`), and set `LANG=C.UTF-8`.
- **Don't use `pkill -f lacarta`**: it matches your own shell command, and the user's live daemon and hooks, and kills them. Stop daemons by the pid in `$LACARTA_HOME/daemon.json`.
- **Don't run the real console on 4747** or touch `~/.lacarta` in tests: the user's live sessions use them. Use a temp `LACARTA_HOME` and another port.
- **Talking to the user:** this session is registered in the lacarta console with remote control on. The user often writes from the console; those messages arrive as Stop hook feedback starting with `[lacarta console]` and are the user's own words. Claude Code labels them "Stop hook blocking error"; that label is cosmetic.
