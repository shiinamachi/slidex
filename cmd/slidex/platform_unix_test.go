//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedAppServerDefaultListenUnix(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
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
