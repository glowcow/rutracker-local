// Package web embeds the built UI so the binary serves it.
package web

import (
	"embed"
	"io/fs"
)

// web/dist/.gitkeep keeps the embed compiling before the first npm run build.
//
//go:embed all:dist
var raw embed.FS

// Dist returns dist/, or false when the UI was not built into this binary.
func Dist() (fs.FS, bool) {
	dist, err := fs.Sub(raw, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return nil, false
	}
	return dist, true
}
