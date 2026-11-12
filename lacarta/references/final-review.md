# Stage 3: final review

The final review answers one question: judged only by what is in the repository, does this project do what its specs say, safely? The reviewers must not inherit anything from the agents that built it: no conversation, no summaries, no earlier review reports, no memory. That is why the specs had to be kept current: they are the only intent the final reviewers will see.

## Preparation (you, the orchestrator)

1. Make sure the specs match what was built, including every decision taken along the way. A decision that never reached a spec file is invisible now.
2. Make sure the README explains how to install, build, test and run the project.
3. Commit everything if the project uses git, so the reviewers see a clean tree.
4. `python3 $L final start`.

## Running the reviewers

Spawn **two new subagents**: one functional, one security, with the `final_functional_review` and `final_security_review` models from `models`. Each gets only the prompt below, with the placeholders filled. Do not add context, hints, a summary of the project, or a list of known issues. If your environment has no subagents, stop and ask the user to open a fresh session in the repository and paste the prompts there; doing the final review yourself would defeat its purpose.

Each reviewer may run the two reviews' worth of work for as long as it needs. Run them in parallel.

### Final functional reviewer

```text
You are an independent functional reviewer. You have never seen this project before. Repository:
{ROOT}. Your name is {NAME}.
Lacarta CLI: python3 {L}

Use only the repository content. Read the specifications in .lacarta/specs/ (they are the contract)
and the code, tests and README. Do not read .lacarta/reviews/, .lacarta/state.json or the
decision log: they record earlier opinions you must not inherit. Do not run `status`.

1. Build and run the project and its tests as the README and the stack spec describe. Report every
   failure verbatim.
2. For every functional requirement (FR) and non-functional requirement (NFR), decide met / partly
   met / not met, with evidence (a test, a command you ran, a file and line).
3. Check the implementation against the architecture spec: components, interfaces, data model,
   repository layout.
4. Report gaps between documentation and behaviour.
5. Write your report to {REPORT_PATH} in this format: a verdict line, a requirements table
   (ID | status | evidence), then findings (severity, location, scenario, recommendation).
   Severity: critical (data loss or core feature broken), high (a must requirement unmet),
   medium (a should unmet or a must failing in edge cases), low (polish).
6. Record: python3 {L} review add --phase final --type functional --verdict pass|fail
   --report {REPORT_PATH} --summary "..." --reviewer fresh-agent
   and for each finding: python3 {L} finding add --phase final --review functional
   --severity ... --title ... --detail ... --location ... --scenario ... --recommendation ...
   (--scenario and --recommendation are required for high and critical.)
Verdict is fail if any must requirement is not fully met or any finding is high or critical.
```

### Final security reviewer

```text
You are an independent security reviewer performing a full security audit. You have never seen
this project before. Your name is {NAME}. Repository: {ROOT}.
Lacarta CLI: python3 {L}

Use only the repository content. Read .lacarta/specs/security.md (threat model, security
requirements, accepted risks, severity scale) and .lacarta/specs/architecture.md, then the whole
codebase. Do not read .lacarta/reviews/, .lacarta/state.json or the decision log. Do not run
`status`.

1. Map the attack surface yourself from the code: every entry point, trust boundary, data store,
   secret and third-party dependency. Compare it with the threat model and note anything the model
   misses.
2. Verify every SR requirement: implemented, tested, and effective.
3. Audit the whole codebase, not a diff: injection of every kind, authentication and session
   handling, authorisation on every route and object, secrets handling, cryptography, input
   limits, error handling and logging, file handling, SSRF, XSS/CSRF, deserialisation,
   dependency vulnerabilities (run the ecosystem's audit tool), CI and build configuration.
4. Every finding needs a concrete scenario (input -> effect). Rate it with the severity scale in
   the security spec. Risks listed under "Accepted risks" are not findings unless the
   implementation is worse than what was accepted.
5. Write your report to {REPORT_PATH}: verdict line, SR table (ID | status | evidence), attack
   surface notes, then findings (severity, location, scenario, recommendation).
6. Record: python3 {L} review add --phase final --type security --verdict pass|fail
   --report {REPORT_PATH} --summary "..." --reviewer fresh-agent
   and each finding with python3 {L} finding add --phase final --review security ...
   (--scenario and --recommendation are required for high and critical.)
Verdict is fail if any SR is unmet or any finding is medium or above.
```

If a `security-audit` skill is available in the reviewer's environment, the security reviewer may use it for the audit method, writing its result in the format above.

## After the reviewers

- High or critical findings block, exactly as in implementation: stop, `wait` for the triage decision, act on it.
- Fix the findings (you, the orchestrator, with full context, is fine for fixing).
- Then review again **with new fresh subagents**, using the same prompts. A reviewer that has seen the previous report would check the fixes instead of the project. Re-run both types if the fixes were substantial, otherwise the type that failed.
- Repeat until both final reviews pass, then `final complete --summary "..."` and `wait` for the user's approval.

When the user approves, the project is done. Tell them where the specs, decisions and reports are.
