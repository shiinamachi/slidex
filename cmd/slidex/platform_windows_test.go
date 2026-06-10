//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedAppServerDefaultListenWindows(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\Me\AppData\Local`)
	listen, err := normalizeManagedListenURL("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(listen, "ws://127.0.0.1:") || !strings.HasSuffix(listen, "/app") {
		t.Fatalf("windows default listen = %q", listen)
	}
	if risk := transportRiskForListen(listen); risk != "" {
		t.Fatalf("windows loopback default should not record risk: %q", risk)
	}
	if path := appServerMetadataPath(); !strings.Contains(path, `AppData\Local`) || !strings.Contains(path, `slidex`) {
		t.Fatalf("windows metadata path should use LOCALAPPDATA: %q", path)
	}
}

func TestManagedAppServerCommandUsesProcessGroupWindows(t *testing.T) {
	cmd := exec.Command("codex", "app-server")
	configureManagedAppServerCommand(cmd)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windowsCreateNewProcessGroup == 0 {
		t.Fatalf("managed app-server command should start in a new Windows process group: %#v", cmd.SysProcAttr)
	}
}

func TestRejectSymlinkEscapeRejectsWindowsJunction(t *testing.T) {
	workspace := t.TempDir()
	decksRoot := filepath.Join(workspace, "decks")
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(decksRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	junction := filepath.Join(decksRoot, "junction")
	out, err := exec.Command("cmd", "/c", "mklink", "/J", junction, outside).CombinedOutput()
	if err != nil {
		t.Skipf("windows junction creation unavailable: %v\n%s", err, out)
	}

	if err := rejectSymlinkEscape(decksRoot, filepath.Join(junction, "deck"), true); err == nil || !strings.Contains(strings.ToLower(err.Error()), "reparse") {
		t.Fatalf("expected junction/reparse rejection, got %v", err)
	}
	if err := rejectSecureWriteTarget(junction); err == nil || !strings.Contains(strings.ToLower(err.Error()), "reparse") {
		t.Fatalf("expected secure write target reparse rejection, got %v", err)
	}
	if err := rejectSymlinkAncestors(filepath.Join(junction, "out", "final_deck.html")); err == nil || !strings.Contains(strings.ToLower(err.Error()), "reparse") {
		t.Fatalf("expected secure write ancestor reparse rejection, got %v", err)
	}
}
