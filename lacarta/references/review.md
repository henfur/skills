# Phase reviews

Two reviews close every phase. They answer different questions, so run them separately even when one agent could do both.

- **Functional**: does the code do what the specs say, for the requirements this phase covers, and does it still do what earlier phases delivered?
- **Security**: does the code meet the security requirements and resist the threats in the threat model, and did it introduce new attack surface the model does not cover?

## Who reviews

Spawn one fresh subagent per review when your environment allows it (the Agent tool in Claude Code), with the model `models` gives for `functional_review` or `security_review`, and give it only what a reviewer needs: the prompt below, the phase number, and the repository. Do not hand it your own summary of what you built or what you think is fine: that summary is exactly the blind spot a second reviewer is meant to remove.

Without subagents, do the review yourself, but in a separate pass: re-read the specs, then read the diff file by file as if someone else wrote it.

## Reviewer prompt (functional)

```text
You are the functional reviewer for phase {N} of a project developed with the lacarta framework.
Your name is {NAME}. Repository: {ROOT}.
Lacarta CLI: python3 {L}

1. Read .lacarta/specs/functional.md and .lacarta/specs/architecture.md. Run `python3 {L} status`
   to see phase {N}'s goal and the requirement IDs it covers.
2. Read the code changed in this phase ({DIFF_HINT}) and the tests.
3. Run the test suite and any build/lint commands in the stack spec. Report failures verbatim.
4. For each covered requirement, check every acceptance criterion against the code and, where you
   can, by running it. Check that earlier phases' behaviour still works.
5. Check the code follows the architecture (components, interfaces, layout) and that any deviation
   is backed by a decision.
6. Write a report in the format of references/review.md "Report format" to {REPORT_PATH}.
7. Record it: python3 {L} review add --phase {N} --type functional --verdict pass|fail
   --report {REPORT_PATH} --summary "..." --reviewer subagent
   and each finding: python3 {L} finding add --phase {N} --review functional --severity ... --title ...
   --detail ... --location ... --scenario ... --recommendation ...
   (--scenario and --recommendation are required for high and critical: the user decides on those
   from what you write, without opening the code.)
Verdict is fail if any must requirement covered by this phase is not met, if tests fail, or if any
finding is high or critical.
```

## Reviewer prompt (security)

```text
You are the security reviewer for phase {N} of a project developed with the lacarta framework.
Your name is {NAME}. Repository: {ROOT}.
Lacarta CLI: python3 {L}

1. Read .lacarta/specs/security.md (threat model, security requirements, severity scale) and
   .lacarta/specs/architecture.md. Run `python3 {L} status` to see phase {N}'s scope.
2. Read the code changed in this phase ({DIFF_HINT}), then follow the data: from every entry point
   this phase added (routes, CLI args, file and network input, third-party callbacks) to where the
   data is stored, rendered or executed.
3. For each SR requirement this phase covers, verify it is implemented and tested.
4. Hunt beyond the threat model: injection (SQL, command, template, path), authn/authz gaps and
   IDOR, secrets in code or logs, unsafe deserialisation, SSRF, XSS/CSRF, crypto misuse, missing
   input limits, verbose errors, vulnerable or unpinned dependencies (run the ecosystem's audit
   tool if available).
5. Rate every finding with the severity scale in the security spec. Include the concrete scenario
   (input -> effect). No scenario, no high/critical.
6. Write the report to {REPORT_PATH} in the format below and record it and each finding with the
   CLI as for the functional review, with --review security.
Verdict is fail if any covered SR is unmet or any finding is medium or above and unresolved.
Recording a high or critical finding blocks the project; that is intended.
```

Fill `{N}`, `{ROOT}`, `{NAME}` and `{L}` (from `agent start`: the absolute path to `lacarta.py` followed by `--agent <id>`), `{REPORT_PATH}` (for example `/tmp/lacarta-phase{N}-security.md`) and `{DIFF_HINT}` (for example `git diff abc1234..HEAD`, or the list of files) before sending.

## Report format

```markdown
# {Functional|Security} review: phase {N}

**Verdict:** pass | fail
**Scope reviewed:** commit range or files; requirement IDs

## Requirements
| ID | Status | Evidence |
| --- | --- | --- |
| FR-3 | met | `tests/test_search.py::test_limit`, manual run |

## Findings
### {severity}: {title}
- Location: `path:line`
- Scenario: what an input or user does and what goes wrong
- Recommendation: the fix

## Notes
Anything worth knowing that is not a finding.
```

## Severity, for functional findings

Functional findings use the same words with their own meaning:

- **Critical**: data loss or corruption, or the phase's core goal does not work at all.
- **High**: a must requirement is unmet or wrong in a common case.
- **Medium**: a should requirement is unmet, or a must requirement fails in an edge case.
- **Low**: polish, naming, small deviations from the architecture with no behavioural impact.

High and critical functional findings block development exactly like security ones.

## After fixes

A fix is not verified until a review has looked at it. Re-run the review type that raised the finding (a fresh subagent again, scoped to the fix and what it touches), record the new review, then resolve the finding with a note naming that review.
