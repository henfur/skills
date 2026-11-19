// Package skill embeds what lacarta installs for agents: the lacarta skill itself (SKILL.md and its
// references, written to ~/.claude/skills/lacarta by `install`), the spec templates that `init`
// copies into a new project, the review desk's guide, default instructions and comment templates,
// and the incident-response guide and report templates.
package skill

import "embed"

//go:embed SKILL.md references/*.md
var Agent embed.FS

//go:embed templates/*.md
var Templates embed.FS

//go:embed desk/guide.md desk/instructions/*.md desk/templates/*.md
var Desk embed.FS

//go:embed incident/guide.md incident/templates/*.md
var Incident embed.FS
