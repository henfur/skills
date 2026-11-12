# lacarta: functional specification

Status: describes the system as built in `lacarta-go/` (phases 1 and 2 of [the port](../../../docs/go-port-plan.md)) plus what phases 3 to 5 must add. Each requirement says whether it is **done** (with the test that proves it) or **to do**. Where this spec and the code disagree on a done requirement, the golden recordings in `testdata/golden/` decide: they were recorded from the Python prototype, which was the reference.

## 1. Purpose

One program per machine that lets a person follow and steer every AI coding-agent session on that machine from a browser, and that runs software projects through a gated, reviewed process (specs, independent spec review, phases with functional and security reviews, final review). Agents talk to it through a CLI and hooks; the person uses a local web console.

## 2. Users and roles

| Role | Who | Can |
| --- | --- | --- |
| User | The person at the browser or terminal | Answer questions, talk about a question, message an agent, approve or refuse, triage findings, change settings, archive tasks |
| Orchestrator (agent A-1) | The CLI session itself (Claude Code) | Every CLI command; its answers on the user's behalf are only recorded with `answer`, from the user's own words |
| Subagent (A-2, A-3…) | Agents the orchestrator spawns (implementation, reviewers) | CLI commands with `--agent <id>`; everything they record carries their name |
| Hook | Claude Code calling `lacarta hook <event>` | Registers sessions, delivers console messages, records terminal prompts |

## 3. Scope

In scope: single-agent tasks; software-development projects; one console daemon per host; Claude Code integration through hooks; models per kind of project; remote control of a session from the console.

Out of scope for now: other agent CLIs (Codex, Gemini: an adapter interface is planned), Windows, several users sharing one console, TLS, the security-audit and DevOps kinds (phase 5 and later).

## 4. Functional requirements

### Projects and sessions

### FR-1: Machine-wide project list
- Priority: must. Status: **done**.
- Every project the console shows is listed in `~/.lacarta/hub.json` (`$LACARTA_HOME` overrides the folder). A software project lives in its repository (`<root>/.lacarta/`); a single-agent task lives in `~/.lacarta/tasks/<id>/`.
- Acceptance criteria:
  - [x] `init` registers the project; its id is `<slug of the name>-<first 4 hex of sha256(root path)>` (golden: `software_flow`).
  - [x] A software project created before the hub existed joins it the first time any command runs in it (golden: `migration`).

### FR-2: Every session registers itself
- Priority: must. Status: **done**.
- When a Claude Code session starts, it is attached as agent A-1 to the software project of its folder (nearest `.lacarta/state.json` at or above `cwd`), or else to a new single-agent task named after the folder. A session that started before the hooks were installed registers at its first hook event.
- Acceptance criteria:
  - [x] SessionStart in a folder without `.lacarta/` creates a task `t-<6 hex>` and maps the session to it (golden: `hooks`).
  - [x] SessionStart below a software project's root joins that project (golden: `hooks`, `s-2`).
  - [x] A `prompt` event from an unknown session registers it (golden: `hooks`, `s-3`).
  - [x] Resuming a session (same session id) reattaches it and reopens an ended task (golden: `hooks`).

### FR-3: The session learns about lacarta
- Priority: must. Status: **done**.
- The SessionStart hook gives the agent: the console URL, its project's kind, name and id, the `--project <id>` fallback, when to load the lacarta skill (software projects: always; tasks: when the user starts a project or wants a structured process), how to ask in the console, and the open questions. It also writes `LACARTA_PROJECT` and `LACARTA_SESSION` to `$CLAUDE_ENV_FILE` when set.
- Acceptance criteria:
  - [x] Context text matches the recording (golden: `hooks`); env file written (`hooks_test.go: TestSessionStartWritesEnvFile`).

### FR-4: Single-agent task naming and lifecycle
- Priority: must. Status: **done**.
- A task takes the first line of the user's first prompt as its name (70 characters, then `…`); prompts starting with `/` don't count. When the session ends the task is `ended`, its agent `done`, and its open questions are withdrawn.
- Acceptance criteria:
  - [x] Naming and the end of a session (golden: `hooks`).

### FR-5: A task becomes a software project
- Priority: must. Status: **done**.
- `init` run inside a registered single-agent session (`$LACARTA_SESSION` set) moves the session to the new project: the task ends with `moved_to`, is archived, its open questions are withdrawn, and the project's A-1 gets the session.
- Acceptance criteria:
  - [x] golden: `single_to_software`.

### FR-6: Archive
- Priority: should. Status: **done** (API and legacy page).
- The user archives and restores projects from the home page; archived projects leave the sidebar, the counts and the notifications.

### Questions

### FR-7: Ask the user
- Priority: must. Status: **done**.
- `ask --title ... [--body|--body-file] [--option "Label|meaning" ...] [--no-free] [--kind question|decision|validation] [--spec] [--phase]` creates question `Q-n` and prints only its id. Options may carry a meaning shown under the label. Without `--no-free`, the user may answer with free text instead of an option.
- Acceptance criteria:
  - [x] An answer outside the options is refused with `Choose one of ['A', 'B'] or write an answer.` (golden: `software_flow`).

### FR-8: Talk about this
- Priority: must. Status: **done** (core, API, CLI, legacy page).
- Every open question offers "Talk about this": the user writes to the agent instead of answering; the question stays open. The agent replies with `reply Q-n --message ...` and may rewrite the question (`--body`, and `--option` only on questions without an effect: approvals and triage options drive the framework and can't change). While the user spoke last, `next` tells the agent to reply.
- Acceptance criteria:
  - [x] `reply` on an answered question is refused (golden: `software_flow`); changing options of an effect question is refused (Python-era tests; port a golden case if touched).
  - [x] A listening Stop hook wakes on a new Talk message (`hooks_test.go`).

### FR-9: Wait for answers
- Priority: must. Status: **done**.
- `wait [ids] [--any] [--timeout s]` blocks until answered, polling every 2 s. With no ids it watches every open question and returns on the first answer. A "Talk about this" message always returns. Output: one JSON object (or a list) per finished question: `{id, title, choice, comment, by, at}`, `{id, title, withdrawn: true}` or `{id, title, status: "discussing", thread}`, then explanation lines and `Next: ...`. Exit 3 on timeout, 2 on an unknown id.
- Acceptance criteria:
  - [x] golden: `software_flow` (`wait Q-1`, `wait`, `wait Q-99`), `migration`.

### FR-10: Answers become decisions
- Priority: must. Status: **done**.
- Every answer, from the console (`by: "user"`) or recorded from the terminal with `answer` (`by: "user (terminal)"`), adds a decision `D-n` with the choice, the comment as rationale, the question as source, and the question's spec and phase. Agents log their own decisions with `decide`. `decisions` prints the log as markdown, including the question text, its options and any "Talk about this" thread.
- Acceptance criteria:
  - [x] golden: `software_flow` (`decisions`).

### Software workflow (gates)

Stages: `specification` → `implementation` → `final-review` → `done`; `stopped` by a triage decision. The CLI refuses any transition whose rule is not met, on stderr as `Refused: <reason>` with exit 2.

### FR-11: Specification stage
- Priority: must. Status: **done**.
- Four specs (`functional`, `architecture`, `stack`, `security`) start from templates. A spec review (`review add --phase spec --type specification`) is accepted only when no spec has a `TODO` line, the plan has phases, and no non-validation question is open; it stores `specs_hash` (sha256 over the four spec files' hashes plus the plan's hash). Approval questions (`spec submit`, `plan submit`) are only created when the latest spec review passed, has no unresolved spec finding, and its hash matches the current text.
- Acceptance criteria:
  - [x] Each refusal and its wording (golden: `software_flow`, `spec_changes`).

### FR-12: Edits invalidate reviews and approvals
- Priority: must. Status: **done**.
- After every write: a submitted or approved spec whose file changed becomes `changed` and its open approval question is withdrawn; same for the plan (hash over titles, goals, scopes and covers, encoded as Python's `json.dumps`). The spec review becomes `stale`. The stage moves to `implementation` only when all four specs and the plan are approved and the spec review passed on the current text.
- Acceptance criteria:
  - [x] golden: `spec_changes`; hashes agree with the prototype's on non-ASCII text (golden: `interop`).

### FR-13: One phase at a time
- Priority: must. Status: **done**.
- `phase start N` needs the implementation stage, an approved plan, phase N `planned`, no phase `in-progress` or `awaiting-approval`, and every earlier phase `done`. `phase complete N` needs both the functional and security review of N to have passed as their latest review, and no unresolved finding in N; it asks the user to approve. "Request changes" sends the phase back to `in-progress`.
- Acceptance criteria:
  - [x] golden: `software_flow`.

### FR-14: Findings and triage
- Priority: must. Status: **done**.
- `finding add` records a finding with severity `critical|high|medium|low|info`. High and critical need `--scenario` and `--recommendation`, block the project, and create a triage question with four options and their meanings: Fix it now (finding `fixing`), Change the spec or design (`redesign`), Accept the risk (`accepted`, needs a written reason), Stop the project (stage `stopped`). Only the user can accept a high or critical finding; agents resolve it only as `fixed`, and only after the triage answer. The triage question's body explains the severity, what is wrong, how it goes wrong, where, the proposed fix and the report.
- Acceptance criteria:
  - [x] golden: `software_flow`, `spec_changes` (stop).

### FR-15: Final review
- Priority: must. Status: **done**.
- `final start` needs every phase done; `final complete` needs passing final functional and security reviews and no unresolved final finding, then asks the user for final approval; approval sets the stage to `done`.
- Acceptance criteria:
  - [x] golden: `software_flow`.

### FR-16: Next action
- Priority: must. Status: **done**.
- `status` and `wait` end with one `Next:` sentence telling the agent what the framework expects, in this order: stopped; queued console messages; a "Talk about this" waiting for a reply; (tasks: ended / wait / work); triage waiting; blocking finding; then stage-specific steps. The wording is a contract; see `core.NextAction`.

### Agents

### FR-17: Named agents
- Priority: must. Status: **done**.
- Agents are named after Totally Spies characters and get ids `A-n`; A-1 is the orchestrator (software) or the agent (task) and is always Jerry. The console draws a portrait for each built-in name (`web/app/src/lib/portraits.js`); renamed agents show initials. `agent start` registers a subagent with the model from Settings and prints the exact command line it must use; `agent done|update|rename|list` maintain them. A single-agent task refuses `agent start`.
- The console shows an agent's status: `waiting` (has an open question), `terminal` (A-1: a permission prompt in the CLI), `blocked` (A-1: project blocked), `listening` (A-1: turn ended, waiting for a console message), `working`, `idle`, `done`, `failed`.

### Console and remote control

### FR-18: Console messages and remote control
- Priority: must. Status: **done**.
- The user writes to A-1 from the console's Chat. A message is queued in the project's inbox and delivered as `[lacarta console] The user wrote from the lacarta console. Treat it exactly as if they had typed it here:` + the messages: after the agent's next tool call (PostToolUse context), or at the end of its turn (Stop hook block reason). With remote control on (Settings default, per-project override), the agent keeps `lacarta listen` running as a background command: it polls every second and returns, printing what happened, when a message, an answer or a "Talk about this" message arrives; Claude Code then wakes the agent, which acts and starts the listener again. The terminal is never held. The Stop hook never waits: if no listener runs, it asks the agent once (not when `stop_hook_active`) to start one. One listener per project: a newer one takes over; turning remote control off ends it; SIGTERM unregisters it; a dead listener's pid stops the console showing the agent as listening. The console shows "listening" only while the agent is idle and its listener runs. Messages are refused when no session is connected.
- Acceptance criteria:
  - [x] `hooks_test.go` (Stop: delivery, asking for a listener once, not blocking with one; listen: message, answer, Talk, remote off, takeover, timeout, SIGTERM; dead pid); golden `hooks` (delivery).

### FR-19: Chat transcript
- Priority: must. Status: **done** (reader, API, legacy page).
- The console shows A-1's Claude Code transcript: the user's and the agent's text, one line per tool call (name and its description, command, file path, pattern, URL or prompt, 240 characters), skipping sidechains, meta entries, CLI plumbing and console messages (shown from the inbox with queued/delivered state instead). The last 400 items. Only `.jsonl` files inside `~/.claude` (or `$CLAUDE_CONFIG_DIR`) are read.
- Acceptance criteria:
  - [x] `hooks_test.go: TestReadChat, TestTranscriptOutsideClaudeIsRefused`.

### FR-20: Console views
- Priority: must. Status: **done** (Svelte, `web/app/`; Playwright: `web/app/tests/console.spec.js`). The legacy page `web/legacy/index.html` was the reference and stays at `/legacy/` until removed.
- Routes: `#/` home, `#/settings`, `#/p/<id>/<view>`.
- Home: "Waiting for you" (every open question and terminal prompt across projects), active projects as cards (name, kind, agent, last activity, path, blocked reason, terminal prompt, moved-to link, Answer/Open button, Archive for ended tasks), archived projects behind a toggle.
- Sidebar: All projects (total open questions), Settings, then active projects sorted by last activity with a status dot, kind and open-question count. On narrow screens it becomes a horizontal strip.
- Project tabs: single-agent: Overview, Chat, Questions, Activity. Software: Overview, Chat, Questions, Specifications, Phases & reviews, Findings, Decisions, Activity. Tabs show counts (open questions, specs approved, phases done, unresolved findings with red when high/critical, decisions, queued chat messages).
- Software overview: stage steps, stats, next step, agents, open questions, recent activity. Single overview: name, folder, session banner, agent card, open questions, latest three chat messages, recent activity.
- Agents: grouped as Agent, Orchestrator, Developers, Reviewers (spec, functional, security), Final reviewers, Other; within a group ordered waiting, terminal, blocked, listening, working, idle, failed, done; each group's header counts statuses; finished agents greyed, never hidden. An agent with an open question gets an outline, a pulsing dot and a strip with the question and an Answer button.
- Questions: triage questions first; option buttons, or option cards with their meaning when any option has one; an always-present "Talk about this" option that switches the comment box to a message box and the button to "Send to <agent>"; the thread above the options with "Waiting for <agent> to reply…"; "Accept the risk" relabels the comment box as a required reason; drafts survive re-renders; answered questions listed below.
- Chat: agent header (status, CLI, session start or end, folder), remote-control switch (disabled without a session), session banner (terminal prompt, moved, listening), the log (bubbles; consecutive tool calls folded into one block), composer with a hint on when the message will be read, Ctrl/Cmd+Enter sends.
- Specifications: documentation of the specs as they stand: tabs per spec with status dot, status, revision and the number of decisions that shaped it (linked to Decisions), rendered markdown with Mermaid diagrams. No reviews there. Phases & reviews: per phase its goal, scope, covers, reviews (report readable inline) and findings; the spec review (shown from the specification stage on, with its status) and the final review as their own blocks. Findings: unresolved or all, sorted by severity. Decisions table. Activity, newest first.
- Settings: remote-control default; models for every role of every kind, grouped by stage, with unsaved markers, Save and Discard; the planned kinds listed.
- Light and dark themes (system default, toggle remembered in `localStorage` key `lacarta-theme`), usable at 375 px wide.

### FR-21: Notifications
- Priority: must. Status: **done** (Playwright: the live-update test).
- For every non-archived project: a new question, an agent's reply in a "Talk about this" thread, a terminal prompt, an agent finishing its turn while listening. Shown as toasts (bottom right, at most four, red for triage and terminal prompts, staying until opened, dismissed, or the question is answered), and as system notifications when the tab is hidden and permission was given ("Enable alerts" button, shown only in a secure context with permission not yet asked). Nothing is announced for what existed when the page loaded. A finished-turn toast is skipped while the user is on that project's Chat. The tab title starts with the number of open questions.

### FR-22: Models per kind of project
- Priority: must. Status: **done**.
- Settings hold a model alias (`opus`, `sonnet`, `haiku`, `fable`) for every role of every kind; agents read `models` before each spawn; `models --set role=alias` changes the current kind's role when the user asks in the terminal. Unknown kinds, roles or models are refused.

### FR-23: Live updates
- Priority: should. Status: **done** (`server_test.go: TestEvents`, Playwright: the live-update test).
- The console receives change notices over server-sent events (`GET /api/events`) instead of polling; polling every 2.5 s remains the fallback.

### FR-24: Install
- Priority: must. Status: **done for hooks; to do for the skill (phase 4)**.
- `lacarta install [--settings path] [--uninstall]` merges lacarta's six hooks into Claude Code's settings (keeping other hooks, replacing the prototype's and earlier binaries', backup kept). Phase 4: also writes the embedded skill to `~/.claude/skills/lacarta`, without overwriting a symlink or folder it didn't write unless `--force`.

### FR-25: Kinds of project are pluggable
- Priority: should. Status: **to do (phase 5)**. See architecture AD-6.

## 5. User flows

1. **Follow a session**: the user starts `claude` anywhere; the task appears on the home page; they open its Chat and watch; the agent asks a question; a toast appears; they answer; the listening session continues.
2. **Drive remotely**: the agent finishes its turn; "finished its turn" toast; the user writes in Chat; the background listener returns and wakes the agent; the agent works and restarts the listener; repeat. The terminal stays usable throughout, and the user can type there too.
3. **Build a project**: in a task the user asks for a new app; the agent loads the skill and runs `init`; the session moves to the new software project; specs are written, independently reviewed, approved; phases run with reviews; a high finding stops work until the user triages it; the final review closes the project.

## 6. Data

See architecture §3. Retention: activity keeps the last 300 events per project, the inbox the last 200 messages, the hub the last 500 session mappings; transcripts are Claude Code's own files and are only read.

## 7. Non-functional requirements

| ID | Area | Requirement | How it is checked |
| --- | --- | --- | --- |
| NFR-1 | Compatibility | Projects and tasks written by the Python prototype open unchanged; files written by Go open in the prototype | golden `interop`, `migration`; `TestAPIParity` |
| NFR-2 | Contract | CLI output agents parse, exit codes and hook JSON are byte-identical to the prototype | golden suite |
| NFR-3 | Robustness | A hook never fails or blocks the agent's session because of lacarta; failures go to `~/.lacarta/hooks.log` | golden `hooks` (malformed stdin); review |
| NFR-4 | Concurrency | CLI, hooks and daemon can write the same project at once without losing writes | advisory locks + atomic replace; review |
| NFR-5 | Performance | PostToolUse with nothing queued only reads the state (no lock contention); the console stays responsive with transcripts of several MB (incremental reading) | review; manual |
| NFR-6 | Footprint | One static binary, no runtime dependencies; the console works offline (vendored libraries) | build; manual |
| NFR-7 | Accessibility | The console is keyboard-usable, labelled for screen readers, readable in both themes, honours reduced motion | Partly: the Playwright tests find every control by its accessible role and label; keyboard-only use and a screen reader **not yet checked by hand** |

## 8. Assumptions and open questions

- Claude Code's hook protocol and transcript format (2026-11) as described in architecture §4; the format may drift between releases.
- Unknown: whether `CLAUDE_ENV_FILE` reaches later Bash commands (fallback: `--project`), and what Esc does to a listening hook (both outcomes handled).
- Open: several sessions on one software project (today the last one becomes A-1's session); whether the console key should stay in session context (see security spec, accepted risk AR-2).
