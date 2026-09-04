// Package xterm embeds the xterm.js 5.5.0 terminal and the FitAddon 0.10.0
// (UMD builds), served under /xterm for the agent built-in SSH console on
// the machine detail page. The scripts are loaded on demand by
// web/static/js/app.js.
package xterm

import "embed"

// FS contains the xterm.js assets (xterm.js, xterm.css, addon-fit.js).
//
//go:embed xterm.js xterm.css addon-fit.js
var FS embed.FS
