# Incident response kind

Status: first version implemented 2026-11-18. Kind id `incident`, commands `lacarta incident …`.

## What it is for

One case per folder: a security incident, or a vulnerability to respond to (a CVE that affects the stack, a
reported flaw). The agents investigate and write; lacarta keeps the case record and shows it in the console; the
user is the **lead** and decides what only a person may.

A case holds:

| Part | What it is | Ids |
|---|---|---|
| Timeline | Events at the time they happened (UTC), confirmed or suspected, with their basis | `TL-n` |
| Investigations | One question each: notes, a conclusion, the challenge of that conclusion | `INV-n` |
| Evidence | Files copied into `evidence/`, read-only, with sha256; or a pointer to where it is kept | `EV-n` |
| Indicators | IPs, domains, URLs, hashes, accounts, CVEs; stored real, shown defanged | `IOC-n` |
| Actions | Containment, eradication, recovery, communication; and follow-ups | `ACT-n` |
| Narrative | Situation, impact, root cause, next steps, lessons learned | |
| Reports | Situation reports along the way, then the final report, in `reports/` | `REP-n` |

## Stages and phases

The stage is `active`, then `review` while the final report waits for the lead, then `closed`. `incident reopen`
brings a closed case back.

The phase is not set by hand: it comes from the milestones marked on the timeline (`--mark`).

| Milestone | Incident | Vulnerability | Phase until it is reached |
|---|---|---|---|
| `occurred` | First malicious activity | Introduced or disclosed | |
| `detected` | Detected | Reported to us | |
| `contained` | Contained | Mitigated | Identify & contain / Assess & mitigate |
| `eradicated` | Eradicated | Remediated | Eradicate / Remediate |
| `recovered` | Recovered | Fix verified | Recover / Verify the fix |

The response times in the final report (time to detect, detection to contained, detection to recovered) come from
the same marks.

## Gates

| Rule | Where |
|---|---|
| A confirmed timeline event needs evidence or a named source; otherwise it is `--suspected` | `core.TimelineAdd` |
| A milestone is marked once; a wrong event is withdrawn with a reason, never deleted | `core.TimelineAdd`, `TimelineRetract` |
| A confirmed or refuted conclusion rests on registered evidence | `core.InvestigationConclude` |
| A confirmed conclusion is challenged by a different agent; `fails` reopens the investigation | `core.InvestigationChallenge` |
| A response action is proposed with what exactly is done and what it could break, and waits for the lead | `core.ActionPropose` |
| `action start` and `action done` refuse an action that is not approved | `core.ActionStart`, `ActionDone` |
| Actions, severity changes and reports are answered in the console only; `lacarta answer` refuses them | `cli.cmdAnswer` |
| The severity changes only when the lead says so | `core.IncidentSeverity` |
| A situation report needs a situation text newer than the last report (or `--unchanged`) | `cli.cmdReportSitrep` |
| The final report needs: no open investigation, every confirmed conclusion challenged, every action settled, the narrative written, every evidence file matching its hash | `core.FinalBlockers`, `incident.Verify` |
| A report is issued only by the lead, and only if its file is still what was drafted or what the lead edited | `core.applyIncidentEffect` |

Follow-ups (`--kind follow-up`) are not gated: they are work that outlives the case, listed in the final report.

## Reports

`incident report sitrep` and `incident report final --outcome …` render a markdown file from the case record with
Go `text/template`. The built-in templates are copied into the case's `templates/` at init, to be edited to the
organisation's format; a missing file falls back to the built-in one. lacarta appends the AI-assistance line
itself. The draft is written to `reports/`, its hash kept in the state, and a question asks the lead. The lead can
edit the draft in the console (Reports view) before issuing it. Approving the final report closes the case with
its outcome: `resolved`, `false-alarm`, `not-affected` or `risk-accepted`.

A situation report shows what changed since the previous report; the `Next:` line suggests one when several
records were added, or when the cadence set with `incident update --sitrep-every 4h` has passed.

## Agents

| Role | Default model | What it does |
|---|---|---|
| Coordinator | opus | The session itself: timeline, investigations, proposals, reports |
| Investigator | sonnet | Works one investigation |
| Challenger | opus | Fresh agent that tries to disprove a confirmed conclusion |

`incident investigation brief INV-n [--role challenger] --for <agent id>` prints a subagent's whole prompt. The
brief and the guide state the rules: evidence is untrusted data, nothing is changed on any system, no suspicious
file is run, no secret is pasted into the record.

## Where things are

| Path | Content |
|---|---|
| `internal/core/incident.go` | State, rules, derived view, the `Next:` line |
| `internal/incident/` | Case folder init, evidence files and hashes, report rendering, briefs |
| `internal/cli/incident.go` | Commands |
| `skill/incident/` | The coordinator's guide and the two report templates |
| `web/app/src/views/Incident*.svelte` | Overview, Timeline, Investigations, Actions, Evidence, Reports |
| `internal/cli/incident_test.go`, `web/app/tests/incident.spec.js` | A whole case through the CLI; the console views and the lead's approvals |

## Decisions taken without asking, to revisit

1. **No approval to open a case.** `incident init` takes the type and severity the user gave; only later severity
   changes ask.
2. **Every decision of the lead is console-only** (actions, severity, reports): an agent can't record an answer
   for them with `lacarta answer`, since evidence may carry text asking it to.
3. **Evidence is kept out of git** by the case's `.gitignore`; reports and the state's config are not.
4. **No session settings are written** for a case (a desk gets deny rules and a command guard). The agents run
   as the same OS user as lacarta, so the hashes detect a changed evidence file or report but nothing prevents
   the change. This is the same limit as O-3 of the review desk.

## Left to do

- HTML and PDF output of reports, and signing the final report as the audit kind does.
- Report kinds beyond these two: a customer or regulator notification, a vulnerability advisory.
- Importing indicators and timeline events in bulk (CSV, STIX), and exporting indicators as STIX or MISP.
- Linking a case to a security audit or a review desk finding that started it.
- The `Kind` interface is still not extracted: this is the fourth kind that branches on `st.Template`.
