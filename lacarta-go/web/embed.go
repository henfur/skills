// Package web embeds the console: the Svelte app built into dist/ (source in app/; `bun run
// build` there).
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
