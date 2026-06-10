//go:build !windows

package main

import (
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
