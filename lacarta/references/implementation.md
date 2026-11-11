# Stage 2: implementation

One phase at a time. Each phase follows the same loop:

```
phase start N
  build the phase scope (tests included)
  functional review ─┐
  security review  ──┴─> record reviews and findings
  fix findings, re-review until both pass
phase complete N  ->  user approves in the console  ->  next phase
```

## Building

You orchestrate; an implementation subagent builds. After `phase start N`, run `models`, then spawn one subagent with the `implementation` alias as its `model` and this prompt (placeholders filled):

```text
You are implementing phase {N} of a project developed with the lacarta framework.
Your name is {NAME}. Repository: {ROOT}.
Lacarta CLI: python3 {L} (read-only for you: only `status` and `decisions`).

1. Read all four specs in .lacarta/specs/ and `python3 {L} status` for phase {N}'s goal, scope and
   the requirement IDs it covers.
2. Build exactly that scope, following the architecture and stack specs, with the tests the
   architecture's testing strategy calls for. Run the tests and the linters until they pass.
3. Do not change the specs or the plan. If the specs are silent, ambiguous or wrong for something
   you need, stop and report it as a question instead of guessing.
4. If you notice a security problem you cannot fix within the scope, stop and report it.
5. Finish with a report: files changed, how to run what you built, test results (verbatim on
   failure), requirement IDs delivered, and every open question or deviation.
```

Read the report. Questions it raises go to the user through `ask` (spec changes) or are settled with `decide` (small calls), and you send the subagent back (SendMessage, same agent) with the answers. When review findings need fixes, send them to the same implementation agent too, so it keeps its context of the code. If your environment has no subagents, build the phase yourself with the same rules.

- Read the specs at the start of each phase, not from memory: they may have changed since the last phase through decisions.
- Build only the phase scope. Something outside the scope that turns out necessary is a decision: log it with `decide` if it is small and obviously consistent with the specs, `ask` if it changes the spec or the plan.
- Write the tests the architecture's testing strategy calls for as you go, and run them. A phase whose tests do not pass is not ready for review.
- Commit at the end of the phase (if the user wants commits), so the review has a clear diff: `git diff <phase-start>..HEAD`.

## Decisions during implementation

A decision that contradicts or extends a spec goes through the user: `ask --kind decision --spec <kind> --phase N`, then update the spec file to match the answer. Small engineering calls that the specs leave open (a helper library, a file name, an error message) go through `decide`, so the trail stays complete without bothering the user.

## Reviews

Run both reviews when the scope is done. The procedure, checklists and report format are in `references/review.md`. Prefer running each review in a fresh subagent: an author reviewing their own code misses the same things twice.

Record each review and each finding:

```bash
python3 $L review add --phase 2 --type security --verdict fail --report /tmp/review.md \
  --summary "Two issues in the upload endpoint" --reviewer subagent
python3 $L finding add --phase 2 --review security --severity high \
  --title "Upload path traversal" --detail "..." --location "app/upload.py:42"
```

## When a finding is high or critical

`finding add` with high or critical severity blocks the project and asks the user a triage question. From that moment:

1. **Stop writing code.** Not on the finding, not on anything else. Tell the user in the terminal that development is stopped and why, with the question id.
2. `wait` for the triage answer.
3. Act on it:
   - **Fix it now**: fix only that finding, add a test that proves it, re-run the security review on the affected code, then `finding resolve F-n --status fixed --note "<fix and how it was verified>"`.
   - **Change the spec or design**: update the spec(s), ask for any further decision the change needs, then fix and resolve as above.
   - **Accept the risk**: the console already marked it accepted and recorded the rationale. Add it to the "Accepted risks" table of the security spec.
   - **Stop the project**: stop. Report the state and wait for the user.

Only the user can accept a high or critical finding. The CLI refuses an agent's attempt to accept one.

Medium, low and info findings do not block. Fix them in the phase when reasonable; otherwise resolve them as `accepted` or `wont-fix` with a note that says why. All of a phase's findings must be resolved before `phase complete`.

## Completing the phase

`phase complete N --summary "<what the user can now do, what changed in the specs>"` checks that the latest functional and security reviews passed and that no finding is unresolved, then asks the user to approve. `wait` for it. On `Request changes`, the phase returns to in-progress: address the comment, re-run the reviews that the changes touch, and complete again.

The next phase cannot start until this one is approved.
