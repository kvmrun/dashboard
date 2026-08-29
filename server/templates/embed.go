// Package templates embeds the dashboard's HTML templates so the binary
// is self-contained.
package templates

import "embed"

// FS contains all the template files of this package.
//
//go:embed *.html
var FS embed.FS
