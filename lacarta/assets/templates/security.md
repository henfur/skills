# {{PROJECT}}: security specification and threat model

## 1. Assets

What an attacker would want, and what a failure would cost.

| ID | Asset | Confidentiality | Integrity | Availability |
| --- | --- | --- | --- | --- |
| A-1 | TODO | high / medium / low | | |

## 2. Actors and trust levels

| Actor | Trust | Can reach |
| --- | --- | --- |
| Anonymous internet user | none | TODO |
| TODO | | |

## 3. Trust boundaries and data flows

TODO: every place data crosses from a less trusted zone to a more trusted one (network edge, API, file upload, third-party callback, build pipeline). Name the flows: DF-1, DF-2…

## 4. Threats (STRIDE)

Walk each data flow and component through Spoofing, Tampering, Repudiation, Information disclosure, Denial of service and Elevation of privilege. Rate likelihood and impact, then derive the severity with the scale in section 7.

| ID | Component / flow | STRIDE | Threat | Likelihood | Impact | Severity | Mitigation (SR) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| T-1 | TODO | | | | | | |

## 5. Security requirements

Testable statements the reviews check, one per mitigation.

| ID | Requirement | Mitigates | Verified by |
| --- | --- | --- | --- |
| SR-1 | TODO | T-1 | test / review / tool |

Cover at least, or mark "Not applicable: reason":

- Authentication and session handling: TODO
- Authorisation (who may do what, checked where): TODO
- Input validation and output encoding: TODO
- Secrets management (nothing in the repo, how they are injected): TODO
- Cryptography (at rest, in transit, which libraries): TODO
- Logging and audit (what is logged, what never is): TODO
- Dependency and supply chain: TODO
- Error handling (no stack traces or internals to users): TODO

## 6. Accepted risks

Risks deliberately not mitigated. Each one is backed by a decision in the console.

| ID | Risk | Why accepted | Decision |
| --- | --- | --- | --- |
| None yet | | | |

## 7. Severity scale

The reviews use this scale. High and critical stop development until the user decides.

- **Critical**: exploitable remotely with no or trivial preconditions, leading to full compromise, code execution, broad data exposure or leaked production secrets.
- **High**: serious impact with a realistic precondition (an authenticated user, a specific configuration): authorisation bypass, injection, account takeover, sensitive data exposure.
- **Medium**: limited impact, or hard to exploit: a missing hardening measure the spec requires, information leaks that aid an attack, denial of service by a privileged user.
- **Low**: defence in depth or best-practice gaps with no direct exploit path.
- **Info**: observations, no risk on their own.
