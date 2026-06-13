package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAppServerStderrBufferIsBounded(t *testing.T) {
	buf := newSynchronizedLimitedBuffer(16)
	if _, err := buf.Write([]byte(strings.Repeat("x", 64))); err != nil {
		t.Fatal(err)
	}
	text := buf.String("app-server stderr")
	if !strings.Contains(text, "output exceeded maximum allowed size") {
		t.Fatalf("expected truncation marker, got %q", text)
	}
	if len(text) > 128 {
		t.Fatalf("bounded stderr text is unexpectedly large: %d", len(text))
	}
}

func TestAppServerScanStdoutRejectsOversizedLine(t *testing.T) {
	client := newAppServerClientState()
	client.scanStdoutWithMaxLineBytes(strings.NewReader(strings.Repeat("x", 128)+"\n"), 32)
	if err := client.protocolError(); err == nil {
		t.Fatal("expected protocol error for oversized stdout line")
	}
}

func TestAppServerNotificationCollectorRejectsTooManyNotifications(t *testing.T) {
	collector := newAppServerNotificationCollector(1, 1024)
	if err := collector.append(map[string]any{"method": "first"}); err != nil {
		t.Fatal(err)
	}
	err := collector.append(map[string]any{"method": "second"})
	if err == nil || !strings.Contains(err.Error(), "notification count exceeded") {
		t.Fatalf("expected notification count cap error, got %v", err)
	}
}

func TestAppServerNotificationCollectorRejectsAggregateBytes(t *testing.T) {
	collector := newAppServerNotificationCollector(10, 32)
	err := collector.append(map[string]any{"method": "event", "params": strings.Repeat("x", 64)})
	if err == nil || !strings.Contains(err.Error(), "notification bytes exceeded") {
		t.Fatalf("expected notification byte cap error, got %v", err)
	}
}

func TestAppServerRequestDoesNotAcceptCollidingServerRequestID(t *testing.T) {
	stdin := &testWriteCloser{}
	client := &appServerClient{stdin: stdin, lines: make(chan map[string]any, 2)}
	go func() {
		client.lines <- map[string]any{"id": 1, "method": "approval/request", "params": map[string]any{"prompt": "confirm"}}
		client.lines <- map[string]any{"id": 1, "result": map[string]any{"ok": true}}
	}()

	resp, events, err := client.request("model/list", map[string]any{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result, _ := resp["result"].(map[string]any); result["ok"] != true {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	got := stdin.String()
	if !strings.Contains(got, `"id":1`) || !strings.Contains(got, `"code":-32601`) {
		t.Fatalf("unsupported server request response missing: %q", got)
	}
}

func TestAppServerWaitForTurnCompletionRespondsToServerRequest(t *testing.T) {
	stdin := &testWriteCloser{}
	client := &appServerClient{stdin: stdin, lines: make(chan map[string]any, 2)}
	go func() {
		client.lines <- map[string]any{"id": 99, "method": "userInput/request", "params": map[string]any{"prompt": "answer"}}
		client.lines <- map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "turn-1", "status": "completed"}}}
	}()

	events, completion, err := client.waitForTurnCompletion("thread-1", "turn-1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if got := turnIDFromCompletion(completion); got != "turn-1" {
		t.Fatalf("completion turn id = %q", got)
	}
	got := stdin.String()
	if !strings.Contains(got, `"id":99`) || !strings.Contains(got, `"code":-32601`) {
		t.Fatalf("unsupported server request response missing: %q", got)
	}
}

func TestWriteAppServerMetadataPreservesWebSocketAuthPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")
	tokenFile := filepath.Join(dir, "token.txt")
	sharedSecretFile := filepath.Join(dir, "secret.txt")
	metadata := map[string]any{
		"schemaVersion": "slidex.appServerProcess.v1",
		"pid":           os.Getpid(),
		"websocketAuth": webSocketAuthConfig{
			Mode:             "capability-token",
			TokenFile:        tokenFile,
			TokenSHA256:      strings.Repeat("a", 64),
			SharedSecretFile: sharedSecretFile,
		},
	}

	if err := writeAppServerMetadata(path, metadata); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "[REDACTED]") {
		t.Fatalf("managed metadata should preserve operational paths, got %s", raw)
	}
	got := webSocketAuthConfigFromMetadata(readAppServerMetadata(path))
	if got.TokenFile != tokenFile || got.SharedSecretFile != sharedSecretFile || got.TokenSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("websocket auth did not round-trip: %#v", got)
	}
}

func TestPrepareManagedAppServerOutputCapturesBoundedStartupStreams(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "codex-app-server.stdout.log")
	stderrPath := filepath.Join(dir, "codex-app-server.stderr.log")
	stdout, stderr, err := prepareManagedAppServerOutput(stdoutPath, stderrPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := io.WriteString(stdout, "stdout startup evidence\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stderr, "stderr startup evidence\n"); err != nil {
		t.Fatal(err)
	}
	if err := stdout.flush(); err != nil {
		t.Fatal(err)
	}
	if err := stderr.flush(); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		path string
		want string
	}{
		{stdoutPath, "stdout startup evidence"},
		{stderrPath, "stderr startup evidence"},
	} {
		raw, err := os.ReadFile(item.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), item.want) {
			t.Fatalf("managed startup log missing in %s: %q", item.path, raw)
		}
		if len(raw) > 256 {
			t.Fatalf("managed startup log should remain small, got %d bytes", len(raw))
		}
	}
}

func TestStartManagedAppServerCleansMetadataWhenChildExits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable script is POSIX-specific")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeCodex := filepath.Join(binDir, "codex")
	script := `#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  --version)
    echo "codex-cli 0.138.0"
    exit 0
    ;;
  app-server)
    echo "fake app-server bind failed" >&2
    exit 12
    ;;
esac
echo "unexpected fake codex invocation: $*" >&2
exit 13
`
	if err := os.WriteFile(fakeCodex, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runtimeDir := filepath.Join(root, "runtime")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	err := startManagedAppServer("ws://127.0.0.1:1/app", "", webSocketAuthConfig{}, false)
	if err == nil || !strings.Contains(err.Error(), "exited before readiness") {
		t.Fatalf("expected early-exit readiness failure, got %v", err)
	}
	if _, statErr := os.Stat(appServerMetadataPath()); !os.IsNotExist(statErr) {
		t.Fatalf("metadata should not remain after failed start, stat err=%v", statErr)
	}
	raw, readErr := os.ReadFile(filepath.Join(runtimeDir, "slidex", "codex-app-server.stderr.log"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(raw), "fake app-server bind failed") {
		t.Fatalf("startup stderr was not captured: %q", raw)
	}
}

func TestManagedAppServerSignalIdentityRejectsUntrustedMetadata(t *testing.T) {
	if managedAppServerMetadataTrustedForSignal(os.Getpid(), map[string]any{"ownerUid": currentOwnerID()}) {
		t.Fatal("metadata without process identity must not be trusted for signaling")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{
		"ownerUid":    "definitely-not-current-user",
		"processExe":  exe,
		"processArgs": append([]string(nil), os.Args...),
	}
	if managedAppServerMetadataTrustedForSignal(os.Getpid(), metadata) {
		t.Fatal("metadata for a different owner must not be trusted for signaling")
	}
}

func TestReadJSONSchemaObjectRejectsOversizedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(path, []byte(`{"padding":"`+strings.Repeat("x", int(maxProjectSchemaBytes)+1)+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSONSchemaObject(path); err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected oversized schema rejection, got %v", err)
	}
}

func TestEnsureSmokeWorkspaceTemplateRejectsOversizedSource(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	source := filepath.Join(root, "decks", "_template")
	if err := os.MkdirAll(filepath.Join(source, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "brief.md"), []byte("# Brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(source, "assets", "large.bin")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(large, maxDeckTemplateFileBytes+1); err != nil {
		t.Fatal(err)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	err = ensureSmokeWorkspaceTemplate(workspace)
	if err == nil || !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("expected oversized smoke template rejection, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "decks", "_template")); !os.IsNotExist(statErr) {
		t.Fatalf("failed smoke template copy should remove partial template, stat err=%v", statErr)
	}
}
