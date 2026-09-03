// Package novnc embeds the noVNC v1.7.0 static files served under /novnc.
//
// The copy of vnc.html carries a small dashboard-integration patch (see
// the "Dashboard integration" script in the file): RFB connect/disconnect
// events are relayed to the parent frame so the machine detail page can
// show the connection state of the embedded client.
package novnc

import "embed"

// FS contains the noVNC application: vnc.html, the app/, core/ and vendor/
// directories, plus defaults.json, mandatory.json and package.json, which
// the client fetches at startup.
//
//go:embed all:vnc.html all:app all:core all:vendor defaults.json mandatory.json package.json LICENSE.txt
var FS embed.FS
