// Package web embeds the console. Until the Svelte console lands (phase 3 of the port), it is
// the prototype's page, copied unchanged from lacarta/assets/ui.
package web

import "embed"

//go:embed legacy/index.html legacy/vendor/*.js
var Legacy embed.FS
