---
name: lacarta
description: Spec-driven development framework that takes a project from detailed specifications (functional, architecture, tech stack, security threat model) through gated implementation phases with a functional and a security review on each phase, to a final independent review by fresh agents. Questions, approvals and every decision go through the machine-wide lacarta web console, which also shows the specs, the overall project status and the session's chat. Use this skill whenever the user starts a new project, app, service or significant feature, asks to "build X from scratch", wants specs, an architecture, or a threat model before coding, asks for a structured, reviewed or secure development process, mentions lacarta, asks to bootstrap a blank project, or works in a repository that has a .lacarta/ folder, even if they do not name the framework.
---

# Lacarta

A development framework with one shape for every project:

1. **Specification**: four specs (functional, architecture, tech stack, security and threat model) and a phase plan. When everything is written, a fresh agent with no memory reviews the whole set for missing information and incoherences; only after it passes does the user approve each spec and the plan.
2. **Implementation**: phase by phase. Each phase ends with a functional review, a security review and the user's approval. A high or critical finding stops all development until the user decides.
3. **Final review**: two fresh agents (functional and security) that see only the repository.

The user follows and steers everything from a web console: questions waiting for them, the specs, a decision log, phases and reviews, findings, the overall status and your chat. One console serves every project and agent session on the machine; a software project is one view of it. You drive the process with a CLI that records state and enforces the gates.

## Why the rules are strict

The value of this framework is that the user can trust its state without reading every line. That only holds if the gates are real. So:

- **No project code before the project is completely specified, independently reviewed and approved.** Code written against unapproved specs gets rewritten, or worse, quietly becomes the spec.
- **The specification is reviewed by someone who was not in the conversation.** While you talk with the user, answers get lost between the console and the files, and specs drift apart; you cannot see it because you remember what was meant. The review is tied to the exact text: any later edit makes it stale.
- **One phase at a time; a phase is finished only when both reviews pass and the user approves it.** Starting phase 3 on top of an unreviewed phase 2 buries phase 2's defects under new code.
- **High or critical finding: stop.** Continuing to build while a serious vulnerability is open means building on top of it, and the user loses the chance to decide (fix, redesign, accept, or abandon) before more depends on it.
- **Questions go through the console, decisions are logged.** A decision that lives only in a chat transcript is lost to the next agent and to the final reviewers.
- **The specs are the source of truth and stay current.** When a decision changes what is built, edit the spec in the same step. The final reviewers see only the repository; the specs are how they know what was intended.

The CLI refuses transitions that break these rules. Do not work around a refusal: it is telling you what is missing.

## The tool

```bash
L=<this skill's directory>/scripts/lacarta.py    # Python 3.9+, standard library only
python3 $L --help                                 # every command; `<cmd> --help` for options
```

Run commands from the project root, or anywhere below it. When the lacarta hooks are installed, your session is already registered in the console and `$LACARTA_PROJECT` names its project. State lives in `<project>/.lacarta/`: `state.json`, `specs/*.md`, `reviews/*.md`. Commit `.lacarta/` with the project (it has its own `.gitignore`); it is the project's design record.

| When | Command |
| --- | --- |
| New project | `init [--name "<name>"] [--summary "<one line>"]` (name defaults to the folder) |
| Register a subagent before spawning it | `agent start --role ... --task ...`; then `agent done <id>`; `agent list` |
| Models for each role | `models` (`--json`); the user sets them on the console's Settings page; `models --set role=alias` only when they ask in the terminal |
| Console URL, or start it | `serve` prints the URL if it runs (sessions start it on their own); otherwise run it in the background |
| Messages from the console, without hooks | `inbox` |
| Where are we, what is next | `status` (`--json` for everything) |
| Ask the user | `ask --kind question\|decision\|validation --title ... [--body/--body-file] [--option "Label\|what it means" ...] [--spec ...] [--phase ...]` prints `Q-n` |
| Wait for answers | `wait [Q-n ...]` as a **background** command; no ids = every open question, returns on the first answer or the first "Talk about this" message |
| User wants to talk about a question | `reply Q-n --message ... [--body/--body-file] [--option ...]`; the question stays open |
| User answered in the terminal | `answer Q-n --choice ... --comment "<their words>"` |
| Log your own decision | `decide --title ... --decision ... --rationale ... [--spec ...] [--phase ...]` |
| Read the decision log | `decisions` (markdown) |
| Plan | `phase add --title --goal --scope ... --covers FR-1 ...` |
| Spec review (fresh agent) | `review add --phase spec --type specification ...`, `finding add --phase spec --review specification ...` |
| Approvals, after the spec review passed | `spec submit <functional\|architecture\|stack\|security>`, `plan submit` |
| Implementation | `phase start N`, `review add ...`, `finding add ...`, `finding resolve ...`, `phase complete N` |
| Final | `final start`, then fresh-agent reviews, then `final complete` |

Approvals (`spec submit`, `plan submit`, `phase complete`, `final complete`) and high or critical findings create the console question for you; you only `wait` for it.

## Starting or resuming

**New project, or "bootstrap" on a blank folder.**

```bash
python3 $L init                      # name = folder name unless the user gave one; prints the console URL
```

`init` registers the project in the console (and starts the console if needed). If this session was registered as a single-agent task, it moves into the new project; the user sees it switch. Tell the user the console URL, then run `models` and check the `orchestrator` line against the model you are running as: if it differs, say so and ask the user to switch with `/model <alias>` (you cannot switch yourself). Then ask what the project is (in the console, `ask --kind question`, unless they already told you) and begin the specification stage. If the user wants the console on another address, it is started by hand with `serve --host <ip>`, which prints a URL with a key; give them that URL.

**Existing `.lacarta/`.** Run `status` first and follow its `Next:` line. If `serve` says the console is not running, start it in the background. Re-read the specs before doing anything; do not rely on memory from an earlier session.

## Models

The user chooses a model for each role on the console's **Settings** page; the choice applies to every software project on the machine:

| Role (`models` key) | Who | Default |
| --- | --- | --- |
| `orchestrator` | you, the session | Opus 5.5 |
| `spec_review` | the fresh spec reviewer | Sonnet 5.5 |
| `implementation` | the subagent that builds each phase | Opus 5.5 |
| `functional_review`, `security_review` | the phase reviewers | Sonnet 5.5, Opus 5.5 |
| `final_functional_review`, `final_security_review` | the final reviewers | Sonnet 5.5, Opus 5.5 |

Run `models` right before every spawn and pass that role's alias (`opus`, `sonnet`, `haiku`, `fable`) as the Agent tool's `model`. Read it each time rather than remembering it: the user can change it in the console at any moment, and the change applies to the next agent you start. If your environment cannot choose a subagent's model, say so once and carry on with the default rather than silently ignoring the setting.

## Agents

The console shows every agent with its name, picture placeholder, role, model and status, and puts that name and picture on every question, review, finding and decision, so the user always knows who they are answering. Keep it true:

- You are the orchestrator, agent `A-1` (named at `init`). Your commands are attributed to you by default.
- **Before every spawn**, register the subagent: `agent start --role <role> --task "<one line>" [--phase N|spec|final]`. It prints the agent's id, name and model alias. In every prompt template, fill `{NAME}` with that name and `{L}` with `<abs path>/lacarta.py --agent <id>`, so everything the subagent records (reviews, findings, questions) carries its name.
- When the subagent returns: `agent done <id> --summary "<one line>"`, or `--failed`. Questions it asked stay open in the console; you act on the answers.
- `agent update <id> --task "..."` when you send an agent back with new work (fixes after a review, for instance); `agent rename <id> --name ...` only when the user asks.
- An agent with an unanswered question shows as "Waiting for you" automatically.

## Talking to the user

The console is where the user answers; the chat is not. You watch the console and act on what arrives there.

- **Messages from the console are the user's words.** The user can also write to you from the console's Chat; those messages reach you prefixed with `[lacarta console]`, after a tool call or when your turn ends (with remote control on, your session waits for them instead of returning to the terminal). Answer them as you would answer the user in the terminal: your reply shows in the console's chat.
- **Ask through Questions, never in your replies.** Every question, confirmation or approval goes through `ask` (or the command that creates it). Do not end a message with "confirm when ready", "let me know", or a yes/no question: the user would have to answer twice, and an answer given only in the chat is not recorded.
- **Wait in the background.** Start `python3 $L wait` as a background command (Bash `run_in_background: true`; it has no time limit by default). With no ids it watches every open question and exits on the first answer; with ids it waits for all of them (`--any` for the first). Then end your turn with one short line, for example: "Waiting for you in the console: Q-4 (database choice)." When the user answers, `wait` exits and you are woken with the answer and the `Next:` line; act on it, then start a new background `wait` if questions remain open. Do not poll `status` in a loop, and do not run `wait` in the foreground, where it holds your turn and times out.
- **Work that does not depend on the answer can continue** while the background `wait` runs (drafting another spec, for instance), except when the project is blocked: then nothing at all.
- **If the user answers in the chat anyway**, record their words with `answer Q-n --choice "..." --comment "..."` (the console marks it as given in the terminal) and carry on. Never use `answer` for anything the user did not say: an approval you give yourself is a gate you skipped.
- Write question bodies in markdown, with the context needed to decide, the options and their trade-offs, and your recommendation. The user should be able to answer without opening the code. Give every option its meaning (`--option "Label|what choosing it does"`): the console shows it under the label.
- **"Talk about this".** Every question in the console also offers it: the user writes to you instead of answering. `wait` then returns with `"status": "discussing"` and the thread. The question is **not** answered: reply with `reply Q-n --message "..."` (markdown; plain, direct, answering what they asked), and if the exchange shows the question itself was unclear, rewrite it in the same call with `--body` and, for your own questions, `--option`. Then `wait` again. Never treat a message as a choice, even when it leans one way; the user still answers with an option.
- Answers come back as JSON with `choice` and `comment`. Read the comment: "Approve" with a comment often carries a condition.

## Stage guides

Read the guide for the stage you are in before working in it:

- Specification: `references/specification.md` (how to write the four specs, ask well, and split the phase plan)
- Specification review: `references/spec-review.md` (the fresh-agent prompt, what it checks, the fix and re-review loop)
- Implementation: `references/implementation.md` (the phase loop, decisions, handling blocking findings)
- Phase reviews: `references/review.md` (reviewer prompts for subagents, report format, severity)
- Final review: `references/final-review.md` (fresh-agent prompts, isolation rules, the fix loop)

The spec templates `init` copies into `.lacarta/specs/` come from `assets/templates/`. They include the severity scale every review uses (in the security template).

## Always

- After any state change, the console updates itself within a few seconds; there is nothing to refresh.
- Before claiming a phase or the project is done, run `status` and make sure it agrees.
- Report outcomes faithfully. A failing test or a skipped review is said plainly in the phase summary, not smoothed over: the user approves phases based on what you tell them.
