package novnc

import "testing"

// TestEmbeddedFiles ensures the noVNC client files the browser needs at
// startup are present in the embedded FS (vnc.html plus the JSON files it
// fetches, and a couple of core JS modules).
func TestEmbeddedFiles(t *testing.T) {
	for _, p := range []string{
		"vnc.html",
		"defaults.json",
		"mandatory.json",
		"package.json",
		"app/ui.js",
		"app/webutil.js",
		"core/rfb.js",
		"vendor/pako/LICENSE",
	} {
		if _, err := FS.ReadFile(p); err != nil {
			t.Errorf("FS.ReadFile(%q): %v", p, err)
		}
	}
}

