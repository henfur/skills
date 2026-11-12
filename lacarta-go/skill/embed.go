// Package skill embeds what lacarta installs for agents: for now the spec templates that
// `init` copies into a new project. The whole skill moves here in phase 4 of the port.
package skill

import "embed"

//go:embed templates/*.md
var Templates embed.FS
