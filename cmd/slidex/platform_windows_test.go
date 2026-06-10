//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
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

func TestAppServerClientCommandUsesProcessGroupWindows(t *testing.T) {
	cmd := appServerClientExecCommand("codex", "app-server", "--listen", "stdio://")
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windowsCreateNewProcessGroup == 0 {
		t.Fatalf("stdio app-server command should start in a new Windows process group: %#v", cmd.SysProcAttr)
	}
}

func TestPluginDoctorHelperWindowsInvokesSlidexDoctor(t *testing.T) {
	root := repoRootForTest(t)
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "args.log")
	fakeSlidex := filepath.Join(binDir, "slidex.cmd")
	if err := os.WriteFile(fakeSlidex, []byte("@echo off\r\n"+
		"type nul > \"%SLIDEX_HELPER_LOG%\"\r\n"+
		":loop\r\n"+
		"if \"%~1\"==\"\" exit /b 0\r\n"+
		"echo %~1>>\"%SLIDEX_HELPER_LOG%\"\r\n"+
		"shift\r\n"+
		"goto loop\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SLIDEX_HELPER_LOG", logPath)
	cmd := exec.Command("cmd", "/c", filepath.Join(root, "plugins", "slidex", "scripts", "slidex-doctor.cmd"), "--probe")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	want := []string{"doctor", "--codex", "--render", "--json", "--probe"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("helper args = %#v, want %#v", got, want)
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

func TestWindowsProcessTreeOrderKillsChildrenBeforeRoot(t *testing.T) {
	got := windowsProcessTreeOrder(10, []windowsProcessEntry{
		{pid: 10, parent: 1},
		{pid: 11, parent: 10},
		{pid: 12, parent: 10},
		{pid: 13, parent: 11},
		{pid: 20, parent: 1},
	})
	want := []int{13, 11, 12, 10}
	if len(got) != len(want) {
		t.Fatalf("tree order length = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tree order = %v, want %v", got, want)
		}
	}
}

func TestRequirePlatformPrivateFileAllowsCurrentUserWindowsACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	userSID, err := windowsCurrentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	setWindowsTestDACL(t, path, []windows.EXPLICIT_ACCESS{
		windowsTestAccessEntry(userSID, windows.GENERIC_ALL, windows.TRUSTEE_IS_USER),
	})
	if err := requirePlatformPrivateFile(path, "--ws-token-file"); err != nil {
		t.Fatalf("expected current-user-only Windows ACL to pass: %v", err)
	}
}

func TestRequirePlatformPrivateFileRejectsBroadWindowsACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	userSID, err := windowsCurrentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	worldSID, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatal(err)
	}
	setWindowsTestDACL(t, path, []windows.EXPLICIT_ACCESS{
		windowsTestAccessEntry(userSID, windows.GENERIC_ALL, windows.TRUSTEE_IS_USER),
		windowsTestAccessEntry(worldSID, windows.GENERIC_READ, windows.TRUSTEE_IS_WELL_KNOWN_GROUP),
	})
	err = requirePlatformPrivateFile(path, "--ws-token-file")
	if err == nil || !strings.Contains(err.Error(), "ACL grants file access") {
		t.Fatalf("expected broad Windows ACL to fail, got %v", err)
	}
}

func TestSecureWriteFileAppliesPrivateWindowsACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out", "token.json")
	if err := secureWriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reason, err := windowsPrivateFileForbiddenReason(path)
	if err != nil {
		t.Fatal(err)
	}
	if reason != "" {
		t.Fatalf("secureWriteFile left non-private Windows ACL: %s", reason)
	}
}

func TestOpenSecureAppendFileAppliesPrivateWindowsACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out", "run_log.jsonl")
	f, err := openSecureAppendFile(path, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{}\n")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	reason, err := windowsPrivateFileForbiddenReason(path)
	if err != nil {
		t.Fatal(err)
	}
	if reason != "" {
		t.Fatalf("openSecureAppendFile left non-private Windows ACL: %s", reason)
	}
}

func makeTestPrivateFile(t *testing.T, path string) {
	t.Helper()
	userSID, err := windowsCurrentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	setWindowsTestDACL(t, path, []windows.EXPLICIT_ACCESS{
		windowsTestAccessEntry(userSID, windows.GENERIC_ALL, windows.TRUSTEE_IS_USER),
	})
}

func setWindowsTestDACL(t *testing.T, path string, entries []windows.EXPLICIT_ACCESS) {
	t.Helper()
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
}

func windowsTestAccessEntry(sid *windows.SID, permissions windows.ACCESS_MASK, trusteeType windows.TRUSTEE_TYPE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: permissions,
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
