# Specification review

Development cannot start until the project is completely specified, and "completely" is checked by someone who was not in the room. While you talk with the user, decisions pile up, a requirement is reworded in one file but not another, an answer given in the console never makes it into the spec, or two specs quietly drift apart. You cannot see these gaps: you remember what was meant, so you read it into the text. A reviewer with no memory reads only what is written, which is exactly what the implementer and the final reviewers will do.

## When

Once everything is written:

- no `TODO` left in any of the four specs,
- the phase plan exists (`phase add`),
- no question or decision is waiting for the user (approvals excepted).

The CLI refuses to record a spec review before that, and refuses every approval (`spec submit`, `plan submit`) until a spec review has passed **on the exact current text**. Editing any spec or the plan afterwards makes the review stale, and approvals already given for an edited spec are withdrawn. A change after review, however small, goes through a new review, because small late edits are where incoherence comes from.

## How

Spawn **one new subagent**, with the `spec_review` model from `models`, and only the prompt below, placeholders filled. Do not add a summary, your intentions, or the things you already know are weak: the reviewer must form its own reading. If your environment has no subagents, ask the user to run the prompt in a fresh session in the repository; reviewing your own specification would defeat the point.

```text
You are an independent specification reviewer. You have never seen this project and you know
nothing about the conversations that produced its specification. Repository: {ROOT}.
Your name is {NAME}.
Lacarta CLI: python3 {L}

Your job: decide whether this specification is complete and coherent enough that a competent
team could build it without coming back with questions, and that nobody would build the wrong
thing from it.

Read, and only read:
- .lacarta/specs/functional.md, architecture.md, stack.md, security.md
- the phase plan: python3 {L} status
- the decision log: python3 {L} decisions
Do not read .lacarta/reviews/ or earlier review reports. Do not ask anyone for context.

Check:
1. Completeness. Every section is filled with real content (not filler). Every functional
   requirement has acceptance criteria someone could test. Roles, data, error cases and
   non-functional targets are stated with numbers where numbers matter. Nothing an implementer
   must decide on a must requirement is left implicit.
2. Coherence across the four specs. Every FR is supported by the architecture (components,
   interfaces, data model). The stack provides everything the architecture assumes. The threat
   model's assets, actors and data flows match the architecture's components and interfaces.
   Every threat at medium or above has an SR, and every SR is implementable with the chosen stack.
   Names, IDs, entities and numbers are the same everywhere they appear.
3. The decision log against the specs. Each decision must be reflected in the spec it concerns,
   and no spec may contradict a later decision. Flag decisions that were never written into a
   spec, specs that still describe an option the user rejected, and decisions that contradict
   each other.
4. The phase plan. Every must FR and every SR is covered by some phase. Phases are ordered so
   each builds on what exists. Security requirements sit in the phase that builds what they
   protect.
5. Ambiguity. Vague words ("fast", "secure", "user-friendly", "etc.", "as needed"), undefined
   terms, and requirements that can be read two ways.

Severity:
- critical: a contradiction or gap that makes a core part unbuildable, or insecure by design
  (for example, no authorisation model for data that several tenants share).
- high: a must requirement an implementer would have to guess at; two specs that contradict
  each other; a user decision missing from or contradicted by the specs.
- medium: an ambiguous or untestable requirement; a should-level gap; a threat without a
  mitigation at medium severity.
- low: wording, structure, minor inconsistencies with no effect on what gets built.

Write your report to {REPORT_PATH}:

  # Specification review
  **Verdict:** pass | fail
  ## Coverage
  | Check | Result | Notes |   (one row per numbered check above)
  ## Findings
  ### {severity}: {title}
  - Where: spec file and section (and the decision ID, if any)
  - Problem: what is missing or contradictory, quoting the text
  - Question for the user, or proposed fix

Then record it:
  python3 {L} review add --phase spec --type specification --verdict pass|fail \
    --report {REPORT_PATH} --summary "..." --reviewer fresh-agent
and each finding:
  python3 {L} finding add --phase spec --review specification --spec <file> \
    --severity ... --title ... --detail ... --location "<file> §<section>" \
    --scenario "<what goes wrong if built as written>" --recommendation "<the spec change>"
  (--scenario and --recommendation are required for high and critical.)
Verdict is fail if any finding is medium or above.
```

Fill `{ROOT}`, `{NAME}` and `{L}` (from `agent start`: the absolute path to `lacarta.py` followed by `--agent <id>`) and `{REPORT_PATH}` (for example `/tmp/lacarta-spec-review.md`).

## After the review

- **High or critical findings block** like any other: `wait` for the user's triage decision.
- Findings that need the user (a missing number, a choice between two contradicting specs): `ask`, with the reviewer's question in the body, then fold the answer into the spec.
- Findings you can fix from the specs and the decision log alone: fix them, and log non-obvious choices with `decide`.
- Resolve each finding (`finding resolve F-n --status fixed --note "..."`). Low findings may be `accepted` with a note.
- Then run a **new** fresh reviewer with the same prompt. Not the previous one: a reviewer that has seen its own report checks its list instead of the specification. Repeat until a review passes with no unresolved findings.

Only then submit the specs and the plan for the user's approval. The user approves a specification that an independent reader has already found complete.
