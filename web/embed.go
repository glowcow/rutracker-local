// Package web embeds the built Vite SPA so the Go binary serves both the API
// and the frontend from one container.
//
// Build flow: `npm run build` writes web/dist/*, then `go build` bakes that
// tree in via the embed directive below. On a fresh clone only `.gitkeep`
// exists — `all:dist` includes that dotfile so the embed still compiles, and
// the SPA handler 404s at runtime.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var raw embed.FS

// Dist returns the dist/ subtree as a normal fs.FS so callers don't need to
// know about the "dist/" prefix.
func Dist() (fs.FS, error) {
	return fs.Sub(raw, "dist")
}
