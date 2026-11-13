package auditreport

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// HTML renders the report's markdown as a print-ready page: the same text the auditor approved,
// every character escaped, with the AI-assistance watermark and footer on every printed page.
func HTML(d *Doc, md string) string {
	var b strings.Builder
	title := html.EscapeString("Security audit report: " + d.Project)
	fmt.Fprintf(&b, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>%s</title>
<meta name="lacarta-report" content="%s"><meta name="ai-assisted" content="true">
<meta name="author" content="%s"><meta name="generator" content="lacarta (AI-assisted audit)">
<style>%s</style></head><body>
<div class="watermark" aria-hidden="true">AI-ASSISTED AUDIT</div>
<div class="footer">AI-assisted audit · report %s · verify: see "How to verify this report"</div>
<main>%s</main></body></html>
`, title, html.EscapeString(d.ID), html.EscapeString(d.Auditor), css, html.EscapeString(d.ID), mdToHTML(md))
	return b.String()
}

const css = `
@page { size: A4; margin: 18mm 16mm 20mm; }
* { box-sizing: border-box; }
body { font: 10.5pt/1.5 "Inter", "Helvetica Neue", Arial, sans-serif; color: #1a1a1a; margin: 0; }
main { max-width: 820px; margin: 0 auto; padding: 24px; }
h1 { font-size: 20pt; margin: 0 0 12px; } h2 { font-size: 14pt; margin: 28px 0 8px; border-bottom: 1px solid #ccc; }
h3 { font-size: 12pt; margin: 22px 0 6px; break-after: avoid; }
blockquote { margin: 12px 0; padding: 10px 14px; border-left: 4px solid #6b46c1; background: #f4f0fb; }
table { border-collapse: collapse; width: 100%; margin: 8px 0 14px; font-size: 9.5pt; }
th, td { border: 1px solid #bbb; padding: 4px 6px; text-align: left; vertical-align: top; overflow-wrap: anywhere; }
pre { background: #f5f5f5; padding: 8px 10px; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 9pt; }
code { font-family: "JetBrains Mono", Menlo, monospace; font-size: 9pt; overflow-wrap: anywhere; }
.watermark { position: fixed; top: 45%; left: 0; right: 0; text-align: center; transform: rotate(-30deg);
  font: 700 54pt/1 Arial, sans-serif; color: rgba(107, 70, 193, 0.08); letter-spacing: 6px; z-index: -1; pointer-events: none; }
.footer { position: fixed; bottom: 0; left: 0; right: 0; text-align: center; font-size: 8pt; color: #555;
  padding: 4px; border-top: 1px solid #ddd; background: #fff; }
@media screen { .footer { position: sticky; } }
`

var (
	reCode   = regexp.MustCompile("`([^`]+)`")
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic = regexp.MustCompile(`(^|[\s(])_([^_]+)_([\s).,:;]|$)`)
	reOL     = regexp.MustCompile(`^\d+\. `)
)

// inline escapes the text, then applies code spans, bold and italics.
func inline(s string) string {
	parts := reCode.Split(s, -1)
	codes := reCode.FindAllStringSubmatch(s, -1)
	var b strings.Builder
	for i, p := range parts {
		t := html.EscapeString(p)
		t = reBold.ReplaceAllString(t, "<strong>$1</strong>")
		t = reItalic.ReplaceAllString(t, "$1<em>$2</em>$3")
		b.WriteString(t)
		if i < len(codes) {
			b.WriteString("<code>" + html.EscapeString(codes[i][1]) + "</code>")
		}
	}
	return b.String()
}

func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(strings.TrimSuffix(line, "|"), "|")
	cells := []string{}
	cur := strings.Builder{}
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) && line[i+1] == '|' {
			cur.WriteByte('|')
			i++
			continue
		}
		if line[i] == '|' {
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(line[i])
	}
	return append(cells, strings.TrimSpace(cur.String()))
}

// mdToHTML converts the subset of markdown the report template writes.
func mdToHTML(md string) string {
	lines := strings.Split(md, "\n")
	var b strings.Builder
	i := 0
	if len(lines) > 0 && lines[0] == "---" { // front matter
		for i = 1; i < len(lines) && lines[i] != "---"; i++ {
		}
		i++
	}
	for i < len(lines) {
		l := lines[i]
		switch {
		case strings.TrimSpace(l) == "":
			i++
		case strings.HasPrefix(l, "```"):
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
				code = append(code, lines[i])
				i++
			}
			i++
			b.WriteString("<pre><code>" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>\n")
		case strings.HasPrefix(l, "#"):
			n := len(l) - len(strings.TrimLeft(l, "#"))
			fmt.Fprintf(&b, "<h%d>%s</h%d>\n", n, inline(strings.TrimSpace(l[n:])), n)
			i++
		case strings.HasPrefix(l, "> "):
			var q []string
			for i < len(lines) && strings.HasPrefix(lines[i], "> ") {
				q = append(q, strings.TrimPrefix(lines[i], "> "))
				i++
			}
			b.WriteString("<blockquote>" + inline(strings.Join(q, " ")) + "</blockquote>\n")
		case strings.HasPrefix(l, "|"):
			var rows [][]string
			for i < len(lines) && strings.HasPrefix(lines[i], "|") {
				rows = append(rows, splitRow(lines[i]))
				i++
			}
			b.WriteString("<table>")
			for r, row := range rows {
				if r == 1 && len(row) > 0 && strings.HasPrefix(row[0], "---") {
					continue
				}
				tag := "td"
				if r == 0 {
					tag = "th"
				}
				b.WriteString("<tr>")
				for _, c := range row {
					fmt.Fprintf(&b, "<%s>%s</%s>", tag, inline(c), tag)
				}
				b.WriteString("</tr>")
			}
			b.WriteString("</table>\n")
		case strings.HasPrefix(l, "- ") || reOL.MatchString(l):
			tag := "ul"
			if reOL.MatchString(l) {
				tag = "ol"
			}
			b.WriteString("<" + tag + ">")
			for i < len(lines) && (strings.HasPrefix(lines[i], "- ") || reOL.MatchString(lines[i])) {
				item := strings.TrimPrefix(lines[i], "- ")
				item = reOL.ReplaceAllString(item, "")
				b.WriteString("<li>" + inline(item) + "</li>")
				i++
			}
			b.WriteString("</" + tag + ">\n")
		default:
			var p []string
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !strings.HasPrefix(lines[i], "#") &&
				!strings.HasPrefix(lines[i], "|") && !strings.HasPrefix(lines[i], "```") && !strings.HasPrefix(lines[i], "> ") &&
				!strings.HasPrefix(lines[i], "- ") && !reOL.MatchString(lines[i]) {
				p = append(p, lines[i])
				i++
			}
			b.WriteString("<p>" + inline(strings.Join(p, " ")) + "</p>\n")
		}
	}
	return b.String()
}
