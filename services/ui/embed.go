package ui

import "embed"

// StaticFiles contains all embedded UI assets.
//
//go:embed *.html static
var StaticFiles embed.FS
