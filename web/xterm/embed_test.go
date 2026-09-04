package xterm

import "testing"

// TestEmbeddedFiles ensures the xterm.js assets the browser needs for the
// agent built-in SSH console are present in the embedded FS.
func TestEmbeddedFiles(t *testing.T) {
	for _, p := range []string{
		"xterm.js",
		"xterm.css",
		"addon-fit.js",
	} {
		if _, err := FS.ReadFile(p); err != nil {
			t.Errorf("FS.ReadFile(%q): %v", p, err)
		}
	}
}
