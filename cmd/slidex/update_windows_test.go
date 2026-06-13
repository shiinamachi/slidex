//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyCandidateBundleStagesPendingHandoffWindows(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "parent with spaces")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(parent, "slidex install")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))

	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}

	result, err := applyCandidateBundle(status, candidate, "0.2.0", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending-restart" || !result.RestartRequired || result.PluginVerificationStatus != "restart_required" {
		t.Fatalf("windows apply result = %#v", result)
	}
	if result.StagedRoot == "" || result.PendingUpdatePath == "" {
		t.Fatalf("windows apply should report staged root and pending manifest: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("windows handoff should not replace active root immediately, VERSION = %q", got)
	}

	pending, pendingPath, err := readPendingUpdate(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(pendingPath) != filepath.Clean(filepath.FromSlash(result.PendingUpdatePath)) {
		t.Fatalf("pending path = %s, result path = %s", pendingPath, result.PendingUpdatePath)
	}
	if pathWithin(installRoot, filepath.FromSlash(pending.StagedRoot)) {
		t.Fatalf("staged root should be outside active install root: %s", pending.StagedRoot)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(filepath.FromSlash(pending.StagedRoot), "VERSION"))); got != "0.2.0" {
		t.Fatalf("staged root VERSION = %q", got)
	}
	if !strings.HasSuffix(filepath.FromSlash(pending.ActivatorPath), "slidex.exe") {
		t.Fatalf("windows activator should be an exe, got %s", pending.ActivatorPath)
	}
	if pathWithin(installRoot, filepath.FromSlash(pending.ActivatorPath)) || pathWithin(filepath.FromSlash(pending.StagedRoot), filepath.FromSlash(pending.ActivatorPath)) {
		t.Fatalf("activator should be outside active and staged roots, got %s", pending.ActivatorPath)
	}

	status, err = currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !status.PendingActivation || status.PendingActivationCommand == "" || !strings.Contains(status.PendingActivationCommand, "slidex.exe") {
		t.Fatalf("pending activation status missing command: %#v", status)
	}
	if !strings.HasPrefix(status.PendingActivationCommand, "powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand ") {
		t.Fatalf("pending activation command should use encoded PowerShell: %s", status.PendingActivationCommand)
	}
	script := decodeWindowsPowerShellCommandForTest(t, status.PendingActivationCommand)
	if !strings.Contains(script, "& "+powershellSingleQuote(filepath.ToSlash(filepath.FromSlash(pending.ActivatorPath)))) {
		t.Fatalf("pending activation command should invoke activator with PowerShell call operator:\n%s", script)
	}
	if !strings.Contains(script, powershellSingleQuote(filepath.ToSlash(installRoot))) {
		t.Fatalf("pending activation command should quote install root with spaces:\n%s", script)
	}
}
