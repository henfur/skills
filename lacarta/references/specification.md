# Stage 1: specification

Goal: a complete specification (four specs and a phase plan), reviewed by an independent agent and approved by the user, before any project code exists. The specs are what every later review measures against, so vague specs make every review weaker.

The stage runs in three steps, and the CLI enforces their order:

1. **Write everything**: the four specs and the plan, asking the user whatever you need along the way.
2. **Independent spec review**: a fresh agent with no memory checks the whole set for gaps and incoherences, including decisions that never reached the specs (`references/spec-review.md`). Fix and re-review with a new agent until it passes.
3. **Approval**: `spec submit` for each spec and `plan submit`. These are refused until step 2 passed on the current text.

## Order

1. **Functional** first. Everything else derives from it.
2. **Architecture**, which needs the functional requirements to know what to design.
3. **Tech stack**, which needs the architecture to know what to choose.
4. **Security**, last because threat modeling needs the components, data flows and stack. Security requirements often push changes back into architecture or stack; edit those files too (they are not approved yet, so that is cheap).
5. **Phase plan**, once the four drafts are stable.

Nothing is submitted for approval spec by spec: the four specs depend on each other, so they are reviewed and approved as a set once all are written. Show the user drafts along the way when it helps (they are visible in the console under Specifications), and ask about anything you are unsure of while writing.

## Gathering what you need

Start from what the user already told you. Do not ask what you can infer or what has a sensible default. Ask what changes the design: who the users are, scale, data sensitivity, hosting constraints, must-have integrations, budget for complexity.

Batch questions. Asking five related questions at once and then waiting for all of them respects the user's time more than five round trips:

```bash
python3 $L ask --kind question --spec functional --title "Who signs in?" --body "..." --option "Only staff" --option "Staff and customers"
python3 $L ask --kind decision --spec stack --title "Database" --body-file /tmp/db-options.md --option "PostgreSQL" --option "SQLite"
python3 $L wait Q-1 Q-2      # background command; end your turn, the answers wake you
```

Good questions for the console:

- Have a title that reads as the question itself.
- Put context and trade-offs in the body (markdown), with your recommendation and why.
- Phrase options as outcomes ("Use PostgreSQL", "Keep it in SQLite"). Leave free text allowed unless the options are truly exhaustive.
- Are tagged with `--spec` so the answer is filed under that spec.

Fold every answer back into the spec file right away. A decision recorded in the log but missing from the spec will be invisible to the final reviewers, who read only the repository.

## Writing the specs

Templates are in `.lacarta/specs/`. Keep their section structure: the user learns where to look, and the reviewers rely on the IDs.

- Every functional requirement gets an ID (`FR-n`) and acceptance criteria a reviewer can verify against running code. "Fast" is not a criterion; "search returns in under 300 ms for 10k items" is.
- Architecture decisions that would be expensive to reverse get an `AD-n` entry with the alternatives you rejected.
- Diagrams are ```` ```mermaid ```` blocks, which the console draws: context and components as a `flowchart`, the data model as an `erDiagram`, key flows as `sequenceDiagram`s, and the threat model's data flows as a `flowchart` with one `subgraph` per trust zone. Keep diagrams and prose consistent: the spec reviewer checks both, and a diagram that disagrees with the text is an incoherence.
- The stack pins versions and names licences.
- The threat model walks each data flow through STRIDE. Every threat at medium or above gets a mitigation that becomes a testable `SR-n` requirement. Mark a section that does not apply as `Not applicable: <reason>` rather than deleting it, so the reader sees it was considered.

The spec review is refused while any `TODO` remains. That is deliberate: a half-filled spec looks finished to a busy reader.

## The phase plan

Split the work into phases that each end in something that runs and can be reviewed on its own: a vertical slice, not "write all models", then "write all views". Three to seven phases suits most projects.

- Put the riskiest or most foundational work first: authentication, data model, the integration nobody has tried.
- Each phase lists the `FR`/`SR` IDs it delivers (`--covers`), so the phase review knows what to check. Every `must` requirement should be covered by some phase.
- Security requirements belong in the phase that builds the thing they protect, not in a "security hardening" phase at the end.

```bash
python3 $L phase add --title "Skeleton and auth" --goal "A user can sign up, sign in and sign out." \
  --scope "Project layout, CI, lint" --scope "Session auth" --covers FR-1 --covers SR-1 --covers SR-2
```

## Before the spec review

Read the four specs once more, end to end, with `python3 $L decisions` open beside them, and make sure every decision is written into the spec it concerns. Then run the independent review as `references/spec-review.md` describes. Do not skip this self-check because a reviewer comes next: the reviewer's time is best spent on what you cannot see, not on what you could.

## Approval

After a passing spec review:

```bash
for k in functional architecture stack security; do python3 $L spec submit $k; done
python3 $L plan submit
python3 $L wait Q-a Q-b Q-c Q-d Q-e   # background command; end your turn
```

## Change requests

When the user answers `Request changes`, their comment says what to change. Edit the spec, run a new fresh spec review (any edit makes the last one stale), then `spec submit` again. Other specs the user already approved stay approved as long as their files do not change; if the change ripples into them, they are withdrawn and resubmitted too. Do not argue in the spec; if you disagree, `ask` a decision question with both options laid out.

When all four specs and the plan are approved and the latest spec review passed on that exact text, the stage moves to implementation by itself.
