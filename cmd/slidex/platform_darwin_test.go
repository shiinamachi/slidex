//go:build darwin

package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDarwinProcArgs2(t *testing.T) {
	raw := newDarwinProcArgs2ForTest("/usr/local/bin/codex", []string{"codex", "app-server", "--listen", "unix:///tmp/slidex.sock"})
	exe, args, ok := parseDarwinProcArgs2(raw)
	if !ok {
		t.Fatal("expected procargs2 parse to succeed")
	}
	if exe != "/usr/local/bin/codex" {
		t.Fatalf("exe = %q", exe)
	}
	if !stringSlicesEqual(args, []string{"codex", "app-server", "--listen", "unix:///tmp/slidex.sock"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestDarwinManagedAppServerIdentityRequiresExactArgs(t *testing.T) {
	exe := "/usr/local/bin/codex"
	args := []string{"codex", "app-server", "--listen", "unix:///tmp/slidex.sock"}
	metadata := map[string]any{"processExe": exe, "processArgs": args}
	if !managedAppServerProcessIdentityMatchesMetadata(exe, args, metadata) {
		t.Fatal("expected exact process identity to match metadata")
	}
	if managedAppServerProcessIdentityMatchesMetadata(exe, []string{"codex", "app-server", "--listen", "unix:///tmp/other.sock"}, metadata) {
		t.Fatal("different argv must not match metadata")
	}
}

func TestWorkbenchProcessMatchesManifestDarwinRejectsMismatchedArgs(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "ready-token", 54321, os.Getpid(), "running")
	control := newWorkbenchControl(manifest, "shutdown-token", "ready-token")
	if err := writeWorkbenchControl(deck, control); err != nil {
		t.Fatal(err)
	}
	if workbenchProcessMatchesManifest(manifest) {
		t.Fatal("darwin matcher must require exact executable and argv before approving PID signaling")
	}
}

func newDarwinProcArgs2ForTest(exe string, args []string) []byte {
	var buf bytes.Buffer
	var argc [4]byte
	binary.LittleEndian.PutUint32(argc[:], uint32(len(args)))
	buf.Write(argc[:])
	buf.WriteString(exe)
	buf.WriteByte(0)
	buf.Write([]byte{0, 0})
	for _, arg := range args {
		buf.WriteString(arg)
		buf.WriteByte(0)
	}
	return buf.Bytes()
}
