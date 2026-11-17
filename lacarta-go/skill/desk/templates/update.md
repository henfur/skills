## {{.Title}} · !{{.MR.IID}} (re-review)

**{{.VerdictLine}}**

Since `{{short .MR.LastReviewedSHA}}`: {{len .Fixed}} fixed, {{len .New}} new, {{len .Open}} still open.
{{- if .Fixed}} Fixed: {{join .Fixed ", "}}.{{end}}

{{.Record.Summary}}
