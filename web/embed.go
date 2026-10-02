// Package web embeds the built Vite SPA so the Go binary serves both the
// API and the frontend from one container.
//
// Build flow:
//
//  1. `npm run build` in this directory writes web/dist/* (overwriting the
//     `.gitkeep` placeholder).
//  2. `go build` reads the embed directive below and bakes `dist/` into the
//     resulting binary.
//
// During CI / fresh clones where the frontend hasn't been built, only
// `.gitkeep` exists. `all:dist` includes that dotfile so the embed
// compiles, and the SPA handler returns 404 at runtime — acceptable
// because /api/* still works and the test stage doesn't hit /.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var raw embed.FS

// Dist returns the dist/ subtree as a normal fs.FS so callers don't need
// to know about the "dist/" prefix.
func Dist() (fs.FS, error) {
	return fs.Sub(raw, "dist")
}
