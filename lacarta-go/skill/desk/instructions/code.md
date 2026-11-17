# Code and functional review

Judge whether the change does what it means to do, correctly, in a way the team can maintain.

## What to check

1. **Intent**: what the merge request means to do (its description, the linked issue, the commits). Say so in one line
   in the summary. If the intent isn't stated anywhere, say that too.
2. **Correctness**: the change does that, including edge cases (empty, large, concurrent, failing dependencies, time
   zones, encodings) and error paths. Trace the main path through the code.
3. **Tests**: new behaviour is tested; a bug fix comes with a test that fails without it; tests check behaviour, not
   implementation details.
4. **Design**: the change sits where the codebase would expect it, reuses what exists, and doesn't add a second way of
   doing something the project already does. Note a simpler alternative when there is one.
5. **Readability**: names, structure and comments a teammate understands without the author.
6. **Dependencies**: each new dependency is needed, maintained, and has a license compatible with the project.
7. **Documentation**: user-facing changes are documented; a breaking change says so.

## Severity

- `critical`: data loss or corruption, or the change's main purpose doesn't work at all.
- `high`: a stated requirement is unmet or wrong in a common case; a regression.
- `medium`: wrong in an uncommon case; a missing test for a risky path.
- `low`: maintainability, naming, small simplifications.
- `info`: suggestions and observations.

Be constructive: for every problem, propose the fix (a suggestion block when it is a few lines). Lead the summary with
what is good when it is true. A clean change gets verdict `pass` and no findings.
