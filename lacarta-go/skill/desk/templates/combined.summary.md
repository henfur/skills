## Automated review · !{{.MR.IID}}

**{{.VerdictLine}}**
{{range .Sections}}
### {{.Title}}

{{.Record.Summary}}
{{- if .Findings}}

| Ref | Severity | Finding |
|---|---|---|
{{range .Findings}}| {{.ID}} | {{sevIcon .Effective}} {{.Effective}} | {{cell (or .Table .Title)}} |
{{end}}
<details><summary>Details</summary>
{{range .Findings}}
#### {{.ID}} · {{.Title}}

{{if .File}}`{{.File}}{{if .Line}}:{{.Line}}{{end}}` · {{end}}confidence: {{.Confidence}}

{{.Body}}
{{if .Fix}}
**Fix:** {{.Fix}}
{{end}}{{end}}
</details>
{{- end}}
{{end}}
