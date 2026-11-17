# Security review

Find **exploitable** weaknesses this change introduces or makes reachable. Not a checklist run: a hunt for real
boundary failures, held to an evidence bar.

## What makes a finding

A security finding names all five:

1. the **lower-trust principal** (anonymous user, signed-in user of another tenant, a service with less privilege,
   content from an external system);
2. the **input or action** it controls;
3. the **control** that should stop it (authentication, authorization, validation, isolation, a limit);
4. the **path past that control**, cited `file:line` at the head commit;
5. the concrete **result** (data read or changed, privilege gained, code run, service denied).

Missing one: it is a hardening remark (`low` or `info` at most, `confidence: theoretical`), or nothing.

Look first where trust changes hands: authentication and session handling, authorization and tenant scoping,
queries and command building, outbound requests built from input, file paths, deserialization, secrets in code, logs
or configuration, cryptography, new dependencies, and anything that runs with more privilege than its caller.

## Severity

- `critical`: trivially exploitable with severe impact (code execution, authentication bypass, cross-tenant data
  exposure, a leaked production secret).
- `high`: exploitable under realistic conditions with significant impact (IDOR, injection, privilege escalation).
- `medium`: needs specific conditions, or limited impact.
- `low`: hard to exploit or minor; hygiene with a security angle.
- `info`: no vulnerability now; worth knowing.

The severity never exceeds what the trace shows. When torn between two, pick the lower and say what would raise it.

## Confidence

- `confirmed`: the source shows the whole path.
- `conditional`: one fact lives outside the repository (deployment, proxy, identity provider configuration); name it,
  and rate the severity as if it holds.
- `theoretical`: a hypothesis the source can't settle; `low` or `info` at most.

## Before you submit

Re-open every `file:line` you cite and look for the strongest control on the path. Drop or downgrade what doesn't
survive. Never put secret values in the record: say where a secret is and what kind, with at most its first characters.
