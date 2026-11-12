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
| 3. Svelte console | **Next** |
| 4. Skill split, releases, retire Python | After 3 |
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

## Phase 3: Svelte console

Goal: replace `web/legacy/index.html` with a Svelte app, with the same features, in both themes and at phone width.

1. **Toolchain.** Bun isn't installed; this is NixOS: `nix shell nixpkgs#bun`, or add a `flake.nix` / `shell.nix` to `lacarta-go/` with `go` and `bun`. Svelte 5 + Vite, no SvelteKit (Go serves the pages and the API). Put the source in `lacarta-go/web/app/`, build to `lacarta-go/web/dist/`, and embed `dist` in `web/embed.go`. Decide whether `dist/` is committed (it is simplest for `go install` users) or built by a `go generate` step. Write the choice down in the README.
2. **API: no change needed.** The page uses `GET /api/hub`, `GET /api/p/{id}/state`, `POST /api/p/{id}/{answer,discuss,message,remote,archive}`, `POST /api/settings`, and the token injected as `const TOKEN = "__LACARTA_TOKEN__"` (replaced in `server.Handler`). Keep injecting the token into `index.html` the same way, and keep every request's `X-Lacarta-Token` header and same-origin POSTs.
3. **Live updates.** Add `GET /api/events` (server-sent events) to `internal/server`: send `hub` and `project:{id}` notices when state files change. The module has no dependencies so far. Either poll file modification times every 500 ms in the daemon (stdlib only, fine at this scale) or add `github.com/fsnotify/fsnotify`. Polling is the simpler and recommended choice. Keep the 2.5 s fetch loop as a fallback in the client.
4. **Port the views**, using the prototype page as the reference (`web/legacy/index.html`; its functions are named after the views):
   - Home: projects, "waiting for you", archive and restore.
   - Settings: models per kind, remote-control default, the planned kinds.
   - Single-agent overview.
   - Chat: transcript, queued and delivered console messages, tool calls folded into blocks, remote-control switch, Ctrl+Enter to send.
   - Questions: option help, the "Talk about this" thread, triage styling, "Accept the risk" requiring a reason.
   - Software overview: stages, stats, agents grouped by role with the asking strip.
   - Specifications with Mermaid (lazy-loaded from the embedded vendor file), phases and reviews, findings, decisions, activity.
   - Notifications: toasts, system notifications, the open-question count in the tab title, the agent card flashing on a new question.
   - Keep the colour tokens and the light and dark themes from the legacy page's `:root` block.
5. **Keep behaviour, drop the workarounds.** The legacy page skips re-rendering while you type, restores the chat's scroll position by hand, and works out by hand what changed for notifications. Svelte's reactivity replaces all three; the behaviour must stay.
6. **Tests.** Add Playwright smoke tests (via Bun) against a Go daemon on a temp `LACARTA_HOME` and a free port: answer a question, "Talk about this", send a chat message, change a model in Settings, archive a task. Check both themes and 375 px width by hand, then write down what you checked.
   Where each view lives in the legacy page (`web/legacy/index.html`):

   | Area | Functions (line at handoff) |
   | --- | --- |
   | Colour tokens, light and dark themes | `:root` (13) and the two dark-mode blocks below it |
   | Routing, sidebar, tabs, render loop | `VIEWS` (462), `parseRoute` (476), `renderNav` (492), `projectTabs` (507), `render` (524) |
   | Home | `viewHome` (981), `bindHome` (1017), `projectStatus` (486) |
   | Software overview, agents | `viewOverview` (570), `agentsSection` (631), `AGENT_GROUPS` (621), `STATUS_ORDER` (629), `AGENT_STATUS` (440) |
   | Single-agent overview, session banner | `viewSingleOverview` (1036), `sessionBanner` (1028) |
   | Questions, Talk about this | `viewQuestions` (681), `questionCard` (719), `threadView` (705), `replier` (699), `bindQuestions` (750), `send` (761) |
   | Chat | `viewChat` (1061), `chatBubble` (1053), `bindChat` (1099) |
   | Specs, phases, findings, decisions, activity | `viewSpecs` (795), `specReviewCard` (782), `viewPhases` (845), `reviewRow` (832), `findingRow` (839), `viewFindings` (876), `viewDecisions` (892), `activityRows` (905), `viewActivity` (1123) |
   | Settings | `viewSettings` (909), `saveSettings` (950), `bindSettings` (963) |
   | Polling and notifications | `poll` (1127), `tick` (1234), `hubEvents` (1163), `notifyChanges` (1176), `showToast` (1203), `setupNotifyButton` (1227) |
   | Mermaid | `loadMermaid` (1245), `configureMermaid` (1263), `renderDiagrams` (1295) |

7. **Switch over** when the parity list is done: serve `dist/`, delete `web/legacy/` (keep the vendored `marked`, `DOMPurify` and `mermaid` files, or replace them with npm packages bundled by Vite), update the README.

Exit: the parity list above checked, Playwright green, `go test ./...` green.

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

Phase 1 kept the prototype's shape: functions in `internal/core/workflow.go` and `internal/cli/commands.go` branch on `st.Template`. Extract the interface sketched in the plan (`Roles`, `Commands`, `Refresh`, `NextAction`, `Derived`) once a second real kind shows what it must cover. Build the security-audit kind on it, with its own skill instructions and console views. Exit: added without editing `state`, `hub`, `hooks` or `server` code paths beyond registering the kind.

## Open decisions for the user

- **The console key in transcripts:** when the console listens beyond loopback, the session context includes the URL with `?key=`, so it ends up in `~/.claude` transcripts. Leave it there, or put the URL without the key in the context.
- **Several sessions on one software project:** today the last session to start becomes A-1's session. Decide before the security-audit kind, where parallel agents are likely.
- **Remote control on by default** holds the terminal at the end of every turn. The user chose this; revisit if it surprises them.

## Known gaps and gotchas

- **A hook starting the daemon by itself** (`server.EnsureDaemon`) has no automated test: it always uses port 4747, where the user's console runs. The user's own switch to the Go binary exercised it in practice (their daemon was started that way or by hand; check `~/.lacarta/daemon.log`). Add a `LACARTA_PORT` override before writing a test for it.
- **Usage errors** (bad flags) are worded differently from Python's argparse. They aren't a contract and aren't in the golden suite.
- **Session pruning** keeps the newest 500 entries in `hub.json`; the order is the file's insertion order (`core.OMap`).
- **Test environment:** the machine's `python3` is a wrapper that needs the real `HOME`. Tests isolate lacarta with `LACARTA_HOME` and `CLAUDE_CONFIG_DIR` instead, and set `LANG=C.UTF-8`, without which Python mangles non-ASCII arguments.
- **Don't use `pkill -f lacarta`** (or `lacarta.py`): it matches your own shell command, and the user's live daemon and hooks, and kills them. Stop daemons by the pid in `$LACARTA_HOME/daemon.json`.
- **Don't run the real console on 4747** or touch `~/.lacarta` in tests: the user's live sessions use them. Use a temp `LACARTA_HOME` and another port.
- **Talking to the user:** this session is registered in the lacarta console with remote control on. The user often writes from the console; those messages arrive as Stop hook feedback starting with `[lacarta console]` and are the user's own words. Claude Code labels them "Stop hook blocking error"; that label is cosmetic.
