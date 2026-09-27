// Package webui serves the gd control panel: a single-page dashboard over the
// existing CLI internals (config, daemon, serve, doctor). Localhost-only by
// design; an optional access token can be required via GD_UI_TOKEN.
package webui

import (
	"embed"
)

// assets carries the logo image into the binary.
//
//go:embed assets/logo.png
var assets embed.FS
