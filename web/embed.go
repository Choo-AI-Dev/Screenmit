// Package web holds the gallery page, built into the exe.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Files is the gallery's static files.
var Files, _ = fs.Sub(files, "static")
