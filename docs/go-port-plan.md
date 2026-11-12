# Lacarta: Go port and skill split

Status: proposal, 2026-11-12. Source of truth for behaviour until the port lands: the Python prototype in `lacarta/scripts/lacarta.py` and `lacarta/assets/ui/index.html`.

## Progress

| Phase | State |
| --- | --- |
| 0. Open assumptions | Mostly settled (table below). Fallback in place for the env file: the session context names `--project <id>` |
| 1. State, gates, CLI | **Done** in `lacarta-go/`: every agent-facing command; parity proven by the golden suite (six scenarios, about 1,300 lines of output identical to the prototype, including one scenario where Go and Python alternate on the same files) |
| 2. Hooks, install, daemon API | **Done**: every hook event in the golden suite; listening Stop hook, SIGTERM cleanup, transcript reader and install covered by Go tests; the API answers exactly like the prototype's daemon on the same state files; the console page is the prototype's, embedded unchanged. Not exercised: a hook starting the daemon itself (it would take port 4747) |
| 3. Svelte console | Not started (Bun is not installed on this machine yet) |
| 4. Skill split, releases | Not started; spec templates are already embedded in the binary |
| 5. First new kind | Not started |

## Goal

Turn lacarta into one program per host: a single `lacarta` binary (Go) that is the CLI, the hooks, the daemon and the console (Svelte, built with Bun, embedded in the binary). The skill shrinks to what an agent reads and ships inside the binary, so the instructions always match the program that enforces them.

Non-goals for the port: new features. Parity first; new kinds of project (security audit, DevOps) come after, on the new code.

## What the prototype established

Checked in a real Claude Code session (this repo, 2026-11-12):

| Assumption | Result |
| --- | --- |
| A session started before `install` registers at its first hook | Works: became task `t-8e2d26` on its first prompt |
| A Stop hook can keep the session waiting and hand it a console message | Works: waited about 7 minutes, the reason became the agent's next turn |
| Console messages do not show twice in the chat | Works: Claude Code records them as stop-hook entries (`system/stop_hook_summary`, `attachment`, `api-request-blob`), not as `user` messages |
| The terminal shows a delivered message nicely | No: Claude Code labels it "Stop hook blocking error". Cosmetic, outside our control; say so in the docs |

Checked later the same day:

| Assumption | Result |
| --- | --- |
| SessionStart runs in a fresh session and its context reaches the agent | Works (`t-391543`, source `startup`) |
| SessionEnd fires | Works (`prompt_input_exit`) |
| A listening Stop hook survives a long wait | 40 minutes observed and still alive; hours not yet proven |
| `CLAUDE_ENV_FILE` reaches later Bash commands | Unknown. Fallback: the session context tells the agent to pass `--project <id>` when `$LACARTA_PROJECT` is empty |
| Esc during a listening Stop hook | Unknown. Either way the console recovers: SIGTERM clears `listening` (tested), and a killed hook's dead pid stops it showing as listening (tested) |

Still open: how often the transcript entry format changes between Claude Code releases (fixtures per version as they appear).

## Architecture

```
lacarta (one static binary)
├── CLI           agent-facing commands: status, ask, wait, reply, phase ..., finding ...
├── hooks         `lacarta hook <event>` called by the agent CLI, JSON on stdin
├── daemon        `lacarta serve`: HTTP API + embedded console + live events
├── state         files + locks, shared by all three (one implementation of the gates)
├── kinds         single, software (later: security-audit, devops)
└── embedded      web/dist (console), skill/ (SKILL.md, references, templates)
```

Rules that carry over from the prototype and must not regress:

- **One implementation of the rules.** CLI, hooks and daemon call the same `state` and `gates` packages. No second copy anywhere, in any language.
- **Files are the source of truth**, written under an exclusive lock and replaced atomically. The CLI and the hooks work while the daemon is down; a software project's `.lacarta/` stays committable.
- **The daemon is a reader that also applies the user's actions** (answers, messages, settings) through the same state code. It watches the files and pushes changes to the console.

### Repository layout

```
cmd/lacarta/            main
internal/core/          state, locking, migration, hub, settings, questions, gates, sessions
                        (one package: these parts call each other, split packages would cycle)
internal/cli/           argument parsing (the argparse subset the prototype uses), commands
internal/golden/        parity suite: scenarios run against either implementation
internal/hooks/         adapter interface; claudecode/ (events, transcript reader, install)
internal/server/        API, auth, live events, static files
web/                    Svelte + Vite app, built by Bun into web/dist (embedded)
skill/                  SKILL.md, references/, templates/ (embedded; installed by `lacarta install`)
schema/                 JSON Schemas for every state file (the contract)
testdata/               fixtures: state files from the prototype, real transcript lines
```

## State contract

Keep the prototype's file names and field names so existing projects and tasks open unchanged. Write JSON Schemas for each file in `schema/` and validate in tests.

| File | Holds | Writers |
| --- | --- | --- |
| `~/.lacarta/hub.json` | projects (id, template, root or cwd, archived), session id -> project | hooks, CLI (`init`, first use of an old project), daemon (archive) |
| `~/.lacarta/settings.json` | models per kind and role, remote control default | daemon, CLI (`models --set`) |
| `~/.lacarta/daemon.json` (0600) | url, open_url (may hold the key), pid | daemon |
| `~/.lacarta/tasks/<id>/state.json` | a single-agent task | hooks, CLI, daemon |
| `<project>/.lacarta/state.json` | a software project; `specs/`, `reviews/` beside it | CLI, hooks, daemon |

- `$LACARTA_HOME` overrides `~/.lacarta`.
- Every state file gets `"version": 2` on first write by the Go code; the loader migrates version 1 (prototype) in memory exactly as `migrate()` does today: default `template`, drop `models`, withdraw open `models-confirm` questions, add `inbox`, `remote`, `listening`, `attention`.
- Locking: `flock` on a `.lock` file next to each state file (Unix). Windows is out of scope for now; keep the lock behind an interface.
- The daemon watches the state directories (fsnotify) and pushes a change event; it re-reads on demand rather than caching state.

## Kinds

A Go interface, so security audit and DevOps plug in without touching the core. Phase 1 kept the prototype's shape (functions that branch on the template) so parity could be checked line by line; the interface is extracted in phase 5, when the second real kind shows what it must cover:

```go
type Kind interface {
    ID() string                      // "single", "software"
    Label() string
    Roles() []Role                   // id, label, stage, about, default model
    Commands() []Command             // kind-specific CLI commands; others refuse with a clear message
    Refresh(*State, Store) error     // stage transitions after each write
    NextAction(*State, Store) string // the `Next:` line agents follow
    Derived(*State, Store) any       // what the console needs beyond the raw state
}
```

Each kind has its console views in `web/src/kinds/<id>/` and its instructions in `skill/` (the software kind keeps today's stage guides). Common to all kinds: agents, questions (with "Talk about this"), decisions, activity, inbox, chat, remote control.

## CLI surface

Agents and the skill depend on it: keep every command, flag, exit code and output line of the prototype, with `lacarta` replacing `python3 <path>/lacarta.py`.

- Commands: `init`, `status`, `serve`, `install`, `hook`, `inbox`, `ask`, `wait`, `reply`, `answer`, `decide`, `decisions`, `models`, `agent {start,update,done,rename,list}`, `spec submit`, `plan {submit,reopen}`, `phase {add,start,complete}`, `review add`, `finding {add,resolve}`, `final {start,complete}`.
- Global flags: `--root`, `--project`, `--agent`; env `LACARTA_PROJECT`, `LACARTA_SESSION`, `LACARTA_AGENT`, `LACARTA_HOME`.
- Exit codes: 0 ok, 2 refused by a gate (message on stderr starting `Refused:`), 3 `wait` timed out.
- Text that agents act on stays word for word: `Next: ...`, the `wait` JSON and its follow-up lines, the blocking-finding message, `ask` printing only the id.
- Add `--json` to every read command for tooling; text stays the default.

## Hooks

`lacarta hook <event>`, JSON on stdin, JSON (or nothing) on stdout, always exit 0; failures go to `~/.lacarta/hooks.log`. An adapter per agent CLI:

```go
type Adapter interface {
    Name() string                                   // "claude-code"
    Install(settingsPath string, binary string) error
    Uninstall(settingsPath string) error
    Session(payload []byte) (SessionInfo, error)    // id, cwd, transcript path, source
    Chat(transcriptPath string, from Offset) ([]ChatItem, Offset, error)
    Output(event string, r Result) []byte           // context to add, or a block reason
}
```

Claude Code adapter, behaviour as in the prototype:

| Event | Timeout | Does |
| --- | --- | --- |
| SessionStart | 30 s | start the daemon if needed; register the session (software project if the folder has `.lacarta/`, else a new single task); write `LACARTA_PROJECT`/`LACARTA_SESSION` to `CLAUDE_ENV_FILE`; add context: console URL, when to load the skill, open questions |
| UserPromptSubmit | 15 s | clear attention; agent working; name a single task after its first prompt |
| PostToolUse (`*`) | 15 s | if messages are queued: mark delivered, add them as context. Cheap path: read only, no lock, when nothing is queued |
| Notification | 15 s | set attention ("needs you in the terminal") |
| Stop | 86400 s | deliver queued messages as a block reason; else, with remote control on, listen: record `listening {since, pid}`, poll (or watch) for a message, an answer, or a "Talk about this" message; give up after 23 h; always clear `listening` on exit, including SIGTERM |
| SessionEnd | 15 s | close the session; a single task ends and its open questions are withdrawn |

Any hook registers an unknown session on first contact (sessions started before `install`).

`lacarta install`: merge the hooks into `~/.claude/settings.json` (backup kept), replacing entries from the Python version (marker `lacarta.py hook`) and from earlier binaries (marker `lacarta hook`); write the embedded skill to `~/.claude/skills/lacarta`. Hook commands use the binary's absolute path. `--uninstall` reverses both.

Transcript reader: incremental by byte offset, complete lines only; keep `user` and `assistant` text and one line per `tool_use`; skip sidechains, meta entries, CLI plumbing (`<command-`, `<local-command`, `<system-reminder`, ...) and everything else. Serve only `.jsonl` files under `~/.claude` (or `$CLAUDE_CONFIG_DIR`), whatever a state file claims. Fixtures from real transcripts, one per Claude Code version seen.

## Daemon and API

One per host: `daemon.json` with pid, plus a port probe before starting a second one. Started by SessionStart, `init`, or by hand with `serve`.

Endpoints (prototype parity), all JSON:

| Method | Path | |
| --- | --- | --- |
| GET | `/` | console (token injected), `?key=` when exposed |
| GET | `/api/hub` | project summaries, settings, catalog of kinds, roles and models |
| GET | `/api/p/{id}/state` | full state, derived, chat; specs and reports for software |
| POST | `/api/p/{id}/answer` `discuss` `message` `remote` `archive` | user actions |
| POST | `/api/settings` | models per kind, remote control default |
| GET | `/api/events` | new: server-sent events, `hub` and `project:{id}` change notices |

Security, unchanged: loopback by default; Host allow-list (DNS rebinding); same-origin and token on every POST; key on every request when bound elsewhere; strict CSP; no-store; 64 KB body limit; transcript path restriction. Plain HTTP: document SSH tunnels for remote use.

## Console (Svelte)

- Svelte 5 with Vite, built by Bun to `web/dist`, embedded with `go:embed`. No Bun or Node at runtime.
- Routes: `/` home, `/settings`, `/p/{id}/{view}`; views come from the project's kind.
- State: one store fed by `/api/events` (fallback: polling every 2.5 s, as today). Re-fetch the hub, or the open project, on its notice.
- Parity list: home (projects, waiting for you, archive), settings, single overview, chat (transcript, queued and delivered console messages, folded tool calls, remote-control switch), questions (option help, Talk about this thread, triage), software overview (stages, stats, agents by group with asking strip), specs with Mermaid, phases and reviews, findings, decisions, activity; toasts, system notifications, tab title count; light and dark themes with today's tokens; phone width.
- The prototype's string templates and DOM workarounds (skip render while typing, manual scroll keeping, diffing for notifications) go away; keep the behaviour, not the mechanism.
- Vendored libraries stay local (marked, DOMPurify, Mermaid lazily loaded): the console works offline.

## Order of work

Each phase ends with its tests green and a short note of anything that changed in behaviour.

0. **Close the open assumptions** with the Python version: a fresh session after `install` (env file), an hours-long listen, Esc while listening, a transcript from the current Claude Code version as a fixture. Exit: the table above has no open rows, or the design has a fallback for each.
1. **State, gates, CLI.** `internal/state`, `gates`, `questions`, `hub`, `kinds`; every CLI command. Exit: the golden test suite (below) passes against both the prototype and the binary, on copies of real prototype state files.
2. **Hooks, install, daemon API.** The binary serves the prototype's `index.html` unchanged at first: the console works on the new backend before any frontend work. Exit: the hook fixtures pass; a real session registers, chats, and is driven remotely through the Go daemon.
3. **Svelte console** at parity, then `/api/events`. Exit: the parity list checked by hand in both themes and at phone width; Playwright smoke tests for answer, talk, message, settings.
4. **Skill split.** `skill/` with commands rewritten to `lacarta`; embedded and installed by `lacarta install`; release builds (linux and macOS, amd64 and arm64) and an install script. Remove the Python prototype. Exit: install on a clean machine, start a project from a blank folder through to phase 1 approval.
5. **First new kind** (security audit) to prove the `Kind` interface. Exit: added without changes to `state`, `hub`, `hooks` or `server`.

## Tests

- **Golden CLI tests**: scripted scenarios with expected stdout, stderr and exit codes, run against both implementations in phase 1. Cover at least: init and spec flow to implementation; every gate refusal; blocking finding without scenario refused; triage with each option; accept-risk without reason refused; ask, discuss, reply, answer; `wait` returning on an answer, a discussion, a timeout; software-only commands refused in a single task; models from settings.
- **Hook tests**: fake payloads for every event; a listening Stop hook woken by a message, an answer, a discussion, remote control turned off; SIGTERM clears `listening`; registration of unknown sessions; `init` moving a session out of a single task.
- **Transcript fixtures**: real lines per Claude Code version, including stop-hook entries, tool results, sidechains, plumbing.
- **API tests**: auth (host, origin, token, key), each action, path restriction for transcripts.

## Risks and decisions to take

- **A long Stop hook holds the terminal.** Default on (the user's choice); per-project switch in Chat; machine-wide in Settings. Revisit if it surprises people.
- **Several sessions on one software project**: today the last one to start becomes A-1's session. Decide before the security-audit kind, where parallel agents are likely.
- **Transcript format drift**: parse defensively, ignore unknown entry types, keep fixtures per version.
- **The skill must not assume `lacarta` is on PATH**: the SessionStart context gives the absolute path, as today.
- **The console key ends up in transcripts**: when the console is bound beyond loopback, the session context includes the URL with its key, and Claude Code stores that context in `~/.claude`. Fine on a single-user machine; on a shared one, give the context the URL without the key.
- **Dogfooding**: the port is a good first real software project for lacarta itself; this plan can seed its functional spec.
