{{sevIcon .Finding.Effective}} **{{.Finding.Title}}** ({{.Finding.Effective}}, {{.Finding.Confidence}}{{if .Finding.Verified}}, verified{{end}})

{{.Finding.Body}}
{{- with .Finding.Fix}}

**Fix:** {{.}}
{{- end}}
{{- with .Finding.Suggestion}}

```suggestion
{{.}}
```
{{- end}}
