# Situation report {{.Number}}: {{.Case}}

**TLP:{{.TLP}}** · {{time .Now}} · {{upper .Severity}} {{.Type}} · phase: {{.Phase}}
{{- if .Refs}} · {{join .Refs ", "}}{{end}}

## Situation

{{or .Sections.situation "_Not written yet._"}}
{{- if .Affected}}

**Affected:** {{join .Affected ", "}}
{{- end}}
{{- if .Sections.impact}}

## Impact so far

{{.Sections.impact}}
{{- end}}

## Since {{if .Previous}}the last report ({{time .Previous.At}}){{else}}the case opened ({{time .OpenedAt}}){{end}}
{{if .NewEvents}}
| When (UTC) | What | Basis |
|---|---|---|
{{range .NewEvents}}| {{time .At}} | {{if eq .Confidence "suspected"}}_Suspected:_ {{end}}{{cell .Text}} | {{if .Evidence}}{{join .Evidence ", "}}{{else}}{{cell .Source}}{{end}} |
{{end}}
{{- else}}
Nothing new on the timeline.
{{end}}
## Investigations
{{if .Investigations}}
| Ref | Question | State | Conclusion |
|---|---|---|---|
{{range .Investigations}}| {{.ID}} | {{cell .Title}} | {{.Status}}{{if and (eq .Status "confirmed") (not .Challenge)}} (not challenged yet){{end}} | {{cell .Conclusion}} |
{{end}}
{{- else}}
None opened yet.
{{end}}
## Actions
{{if .Actions}}
| Ref | Kind | Action | By | State |
|---|---|---|---|---|
{{range .Actions}}| {{.ID}} | {{.Kind}} | {{cell .Title}} | {{cell .Executor}} | {{.Status}} |
{{end}}
{{- else}}
None yet.
{{end}}
{{- if .Indicators}}
## Indicators

| Type | Value | Context |
|---|---|---|
{{range .Indicators}}| {{.Type}} | `{{defang .Type .Value}}` | {{cell .Context}} |
{{end}}
{{- end}}
## Next steps

{{or (index .Sections "next-steps") "_Not written yet._"}}
{{- if .Decisions}}

## Decisions needed

{{range .Decisions}}- {{.}}
{{end}}
{{- end}}
