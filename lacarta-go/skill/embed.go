// Package skill embeds what lacarta installs for agents: the spec templates that `init` copies
// into a new project, the review desk's guide, default instructions and comment templates, and the incident-response
// guide and report templates.
// The whole skill moves here in phase 4 of the port.
package skill

import "embed"

//go:embed templates/*.md
var Templates embed.FS

//go:embed desk/guide.md desk/instructions/*.md desk/templates/*.md
var Desk embed.FS

//go:embed incident/guide.md incident/templates/*.md
var Incident embed.FS
