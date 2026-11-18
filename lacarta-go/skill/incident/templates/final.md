# Final report: {{.Case}}

**TLP:{{.TLP}}** · {{time .Now}} · {{upper .Severity}} {{.Type}} · outcome: {{.Outcome}}
{{- if .Refs}} · {{join .Refs ", "}}{{end}}

## Summary

{{.Sections.situation}}
{{- if .Affected}}

**Affected:** {{join .Affected ", "}}
{{- end}}

## Impact

{{or .Sections.impact "_Not assessed._"}}

## Root cause

{{or (index .Sections "root-cause") "_Not established._"}}
{{- if .Milestones}}

## Key times

| Milestone | When (UTC) |
|---|---|
{{range .Milestones}}| {{.Label}} | {{time .At}} |
{{end}}
{{- range .Times}}
- {{.K}}: {{.V}}
{{- end}}
{{- end}}

## Timeline
{{if .Timeline}}
| When (UTC) | What | Basis |
|---|---|---|
{{range .Timeline}}| {{time .At}} | {{if eq .Confidence "suspected"}}_Suspected:_ {{end}}{{cell .Text}} | {{if .Evidence}}{{join .Evidence ", "}}{{else}}{{cell .Source}}{{end}} |
{{end}}
{{- else}}
No event recorded.
{{end}}
## Investigations
{{range .Investigations}}
### {{.ID}}: {{.Title}}

**{{upper .Status}}.** {{.Conclusion}}
{{- if .Evidence}}

Rests on: {{join .Evidence ", "}}.
{{- end}}
{{- if .Challenge}}

Challenged by {{who .Challenge.By}}: the conclusion {{.Challenge.Verdict}}. {{.Challenge.Note}}
{{- end}}
{{else}}
None.
{{end}}
## Actions taken
{{if .Actions}}
| Ref | Kind | Action | By | State | Result |
|---|---|---|---|---|---|
{{range .Actions}}| {{.ID}} | {{.Kind}} | {{cell .Title}} | {{cell .Executor}} | {{.Status}} | {{cell .Result}} |
{{end}}
{{- else}}
None.
{{end}}
{{- if .Indicators}}
## Indicators

| Type | Value | Context |
|---|---|---|
{{range .Indicators}}| {{.Type}} | `{{defang .Type .Value}}` | {{cell .Context}} |
{{end}}
{{- end}}
## Lessons learned

{{or .Sections.lessons "_None recorded._"}}
{{- if .FollowUps}}

## Follow-ups

| Ref | What | Owner | State |
|---|---|---|---|
{{range .FollowUps}}| {{.ID}} | {{cell .Title}} | {{cell .Executor}} | {{.Status}} |
{{end}}
{{- end}}
{{- if .Evidence}}

## Evidence

| Ref | Title | Kind | From | Collected (UTC) | File or location | sha256 |
|---|---|---|---|---|---|---|
{{range .Evidence}}| {{.ID}} | {{cell .Title}} | {{.Kind}} | {{cell .Source}} | {{time .CollectedAt}} | {{cell (or .File .Ref)}} | {{if .SHA256}}`{{short .SHA256}}…`{{end}} |
{{end}}
{{- end}}
