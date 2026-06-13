//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkbenchProcessMatchesManifestDarwinUsesTrustedControl(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "ready-token", 54321, 12345, "running")
	control := newWorkbenchControl(manifest, "shutdown-token", "ready-token")
	if err := writeWorkbenchControl(deck, control); err != nil {
		t.Fatal(err)
	}
	if !workbenchProcessMatchesManifest(manifest) {
		t.Fatal("darwin fallback should trust matching loopback workbench control")
	}
	manifest.PID = 0
	if workbenchProcessMatchesManifest(manifest) {
		t.Fatal("darwin fallback should reject missing pid")
	}
}
