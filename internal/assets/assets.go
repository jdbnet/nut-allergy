// Package assets holds the embedded web UI and the linux/amd64 agent binary.
package assets

import "embed"

// Agent is the linux/amd64 agent binary copied in by the Makefile.
//
//go:embed nut-allergy-agent
var Agent []byte

// Web is the built Vue app.
//
//go:embed all:dist
var Web embed.FS
