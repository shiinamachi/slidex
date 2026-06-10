//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedAppServerDefaultListenUnix(t *testing.T) {
	runtimeDir, err := os.MkdirTemp("/tmp", "slidex-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	listen, err := normalizeManagedListenURL("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(listen, "unix://") || !strings.HasSuffix(listen, "/slidex/codex-app-server.sock") {
		t.Fatalf("unix default listen = %q", listen)
	}
}

func TestManagedAppServerDefaultListenFallsBackWhenUnixSocketPathIsTooLong(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), strings.Repeat("a", 140)))
	listen, err := normalizeManagedListenURL("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(listen, "ws://127.0.0.1:") || !strings.HasSuffix(listen, "/app") {
		t.Fatalf("long unix socket default should fall back to loopback websocket, got %q", listen)
	}
}

func TestManagedAppServerCommandUsesProcessGroupUnix(t *testing.T) {
	cmd := exec.Command("codex", "app-server")
	configureManagedAppServerCommand(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("managed app-server command should start in its own process group: %#v", cmd.SysProcAttr)
	}
}

func TestAppServerClientCommandUsesProcessGroupUnix(t *testing.T) {
	cmd := appServerClientExecCommand("codex", "app-server", "--listen", "stdio://")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("stdio app-server command should start in its own process group: %#v", cmd.SysProcAttr)
	}
}

func TestPluginDoctorHelperUnixInvokesSlidexDoctor(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash unavailable: %v", err)
	}
	root := repoRootForTest(t)
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "args.log")
	fakeSlidex := filepath.Join(binDir, "slidex")
	if err := os.WriteFile(fakeSlidex, []byte(`#!/usr/bin/env sh
: > "$SLIDEX_HELPER_LOG"
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$SLIDEX_HELPER_LOG"
done
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SLIDEX_HELPER_LOG", logPath)
	deckPath := filepath.Join(t.TempDir(), "deck with spaces")
	cmd := exec.Command("bash", filepath.Join(root, "plugins", "slidex", "scripts", "slidex-doctor.sh"), "--deck", deckPath, "--probe")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(raw), "\r\n", "\n")), "\n")
	want := []string{"doctor", "--codex", "--render", "--json", "--deck", deckPath, "--probe"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("helper args = %#v, want %#v", got, want)
	}
}

func makeTestPrivateFile(t *testing.T, path string) {
	t.Helper()
}
