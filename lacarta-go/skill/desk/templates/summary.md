## {{.Title}} · !{{.MR.IID}}

**{{.VerdictLine}}**

{{.Record.Summary}}
{{- if .Findings}}

| Ref | Severity | Finding |
|---|---|---|
{{range .Findings}}| {{.ID}} | {{sevIcon .Effective}} {{.Effective}} | {{cell (or .Table .Title)}} |
{{end}}
<details><summary>Details: {{len .Findings}} finding(s)</summary>
{{range .Findings}}
### {{.ID}} · {{.Title}}

{{if .File}}`{{.File}}{{if .Line}}:{{.Line}}{{end}}` · {{end}}confidence: {{.Confidence}}{{if .Verified}} · verified{{end}}

{{.Body}}
{{if .Fix}}
**Fix:** {{.Fix}}
{{end}}{{end}}
</details>
{{- end}}
{{- if .Fixed}}

Fixed since the last review: {{join .Fixed ", "}}.
{{- end}}
{{- if .Injections}}

> This merge request contains text addressed to automated reviewers ({{.Injections}}). It was not acted on.
{{- end}}
