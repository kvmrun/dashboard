// Package static embeds the dashboard's frontend assets (CSS, JS).
package static

import "embed"

// FS contains the frontend assets.
//
//go:embed css js
var FS embed.FS
