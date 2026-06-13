//go:build windows

package main

import (
	"os"
	"os/exec"
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
	activatorCwdSentinel := filepath.Join(parent, "activator-cwd.txt")
	writeCandidateBinaryForTestWithCwdSentinel(t, filepath.Join(candidate, "slidex.exe"), "0.2.0", activatorCwdSentinel)
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
	activatorRoot := filepath.Dir(filepath.FromSlash(pending.ActivatorPath))
	if pathWithin(installRoot, filepath.FromSlash(pending.ActivatorPath)) || pathWithin(filepath.FromSlash(pending.StagedRoot), filepath.FromSlash(pending.ActivatorPath)) {
		t.Fatalf("activator should be outside active and staged roots, got %s", pending.ActivatorPath)
	}
	for _, schemaFile := range []string{installMetadataSchemaFile, updateStateSchemaFile, pendingUpdateSchemaFile} {
		if _, err := os.Stat(filepath.Join(activatorRoot, "schemas", schemaFile)); err != nil {
			t.Fatalf("pending activator should stage schema %s: %v", schemaFile, err)
		}
	}

	status, err = currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !status.PendingActivation || status.PendingActivationCommand == "" || !strings.Contains(status.PendingActivationCommand, "slidex.exe") {
		t.Fatalf("pending activation status missing command: %#v", status)
	}
	if strings.Contains(status.PendingActivationCommand, "-EncodedCommand") || strings.HasPrefix(strings.ToLower(status.PendingActivationCommand), "powershell.exe ") {
		t.Fatalf("pending activation command should run in the caller PowerShell session: %s", status.PendingActivationCommand)
	}
	location := "Set-Location -LiteralPath " + powershellSingleQuote(filepath.ToSlash(activatorRoot)) + "; & "
	if !strings.Contains(status.PendingActivationCommand, location) {
		t.Fatalf("pending activation command should move the caller session to activator root before invocation:\n%s", status.PendingActivationCommand)
	}
	restore := "} finally { Set-Location -LiteralPath " + powershellSingleQuote(filepath.ToSlash(installRoot)) + "; $ErrorActionPreference = $slidexPreviousErrorActionPreference }"
	if !strings.Contains(status.PendingActivationCommand, restore) {
		t.Fatalf("pending activation command should restore caller session to install root after invocation:\n%s", status.PendingActivationCommand)
	}
	if !strings.HasPrefix(status.PendingActivationCommand, "& { ") || !strings.Contains(status.PendingActivationCommand, "$slidexPreviousErrorActionPreference") {
		t.Fatalf("pending activation command should isolate helper variables in a scriptblock:\n%s", status.PendingActivationCommand)
	}
	if !strings.Contains(status.PendingActivationCommand, "$slidexActivationExitCode") || strings.Contains(status.PendingActivationCommand, "exit $LASTEXITCODE") {
		t.Fatalf("pending activation command should preserve failures without exiting the caller shell:\n%s", status.PendingActivationCommand)
	}
	if !strings.Contains(status.PendingActivationCommand, "& "+powershellSingleQuote(filepath.ToSlash(filepath.FromSlash(pending.ActivatorPath)))) {
		t.Fatalf("pending activation command should invoke activator with PowerShell call operator:\n%s", status.PendingActivationCommand)
	}
	if !strings.Contains(status.PendingActivationCommand, powershellSingleQuote(filepath.ToSlash(installRoot))) {
		t.Fatalf("pending activation command should quote install root with spaces:\n%s", status.PendingActivationCommand)
	}
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skipf("powershell.exe unavailable: %v", err)
	}
	callerCwdSentinel := filepath.Join(parent, "caller-cwd.txt")
	runScript := status.PendingActivationCommand + "; [System.IO.File]::WriteAllText(" + powershellSingleQuote(filepath.ToSlash(callerCwdSentinel)) + ", (Get-Location).ProviderPath)"
	run := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", runScript)
	run.Dir = filepath.Join(installRoot, ".slidex")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("pending activation command should launch activator from install-root child cwd: %v\n%s", err, out)
	}
	gotCwd := strings.TrimSpace(readFileOrEmpty(activatorCwdSentinel))
	if !sameFilesystemPath(gotCwd, activatorRoot) {
		t.Fatalf("activator cwd = %s, want %s", gotCwd, activatorRoot)
	}
	gotCallerCwd := strings.TrimSpace(readFileOrEmpty(callerCwdSentinel))
	if !sameFilesystemPath(gotCallerCwd, installRoot) {
		t.Fatalf("caller PowerShell cwd after activation command = %s, want %s", gotCallerCwd, installRoot)
	}
}

func TestWindowsPendingActivationCommandFailureDoesNotExitCaller(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skipf("powershell.exe unavailable: %v", err)
	}
	parent := filepath.Join(t.TempDir(), "parent with spaces")
	installRoot := filepath.Join(parent, "slidex install")
	activatorRoot := filepath.Join(parent, "pending activator")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(activatorRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	activatorCwdSentinel := filepath.Join(parent, "failing-activator-cwd.txt")
	activator := filepath.Join(activatorRoot, "slidex.cmd")
	activatorScript := "@echo off\r\ncd > " + windowsBatchQuote(activatorCwdSentinel) + "\r\nexit /b 7\r\n"
	if err := os.WriteFile(activator, []byte(activatorScript), 0o755); err != nil {
		t.Fatal(err)
	}
	pendingCommand := windowsPendingActivationPowerShellCommand(filepath.ToSlash(activatorRoot), filepath.ToSlash(installRoot), filepath.ToSlash(activator), "update", "activate-pending")

	errorSentinel := filepath.Join(parent, "activation-error.txt")
	callerCwdSentinel := filepath.Join(parent, "caller-cwd-after-failure.txt")
	preferenceSentinel := filepath.Join(parent, "error-action-preference.txt")
	runScript := "$ErrorActionPreference = 'Continue'; try { " + pendingCommand + " } catch { [System.IO.File]::WriteAllText(" + powershellSingleQuote(filepath.ToSlash(errorSentinel)) + ", $_.Exception.Message) }; [System.IO.File]::WriteAllText(" + powershellSingleQuote(filepath.ToSlash(callerCwdSentinel)) + ", (Get-Location).ProviderPath); [System.IO.File]::WriteAllText(" + powershellSingleQuote(filepath.ToSlash(preferenceSentinel)) + ", $ErrorActionPreference)"
	run := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", runScript)
	run.Dir = filepath.Join(installRoot, ".slidex")
	if err := os.MkdirAll(run.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("pending activation failure should be catchable without exiting caller shell: %v\n%s", err, out)
	}
	gotError := strings.TrimSpace(readFileOrEmpty(errorSentinel))
	if !strings.Contains(gotError, "exit code 7") {
		t.Fatalf("activation error = %q, want exit code 7", gotError)
	}
	gotActivatorCwd := strings.TrimSpace(readFileOrEmpty(activatorCwdSentinel))
	if !sameFilesystemPath(gotActivatorCwd, activatorRoot) {
		t.Fatalf("failing activator cwd = %s, want %s", gotActivatorCwd, activatorRoot)
	}
	gotCallerCwd := strings.TrimSpace(readFileOrEmpty(callerCwdSentinel))
	if !sameFilesystemPath(gotCallerCwd, installRoot) {
		t.Fatalf("caller PowerShell cwd after failing activation command = %s, want %s", gotCallerCwd, installRoot)
	}
	if gotPreference := strings.TrimSpace(readFileOrEmpty(preferenceSentinel)); gotPreference != "Continue" {
		t.Fatalf("caller ErrorActionPreference after failing activation command = %q, want Continue", gotPreference)
	}
}

func windowsBatchQuote(path string) string {
	return `"` + strings.ReplaceAll(path, `"`, `""`) + `"`
}
