// Package web embeds the console: the Svelte app built into dist/ (source in app/; `bun run
// build` there), and the prototype's page in legacy/, served at /legacy/ until the new console
// has been checked in a browser and the old one is removed.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS

//go:embed legacy/index.html legacy/vendor/*.js
var Legacy embed.FS
