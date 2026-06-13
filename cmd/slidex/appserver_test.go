package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestPrepareManagedAppServerOutputDiscardsChildStreams(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "codex-app-server.stdout.log")
	stderrPath := filepath.Join(dir, "codex-app-server.stderr.log")
	stdout, stderr, err := prepareManagedAppServerOutput(stdoutPath, stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	defer stderr.Close()

	if _, err := io.WriteString(stdout, strings.Repeat("o", 1024)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stderr, strings.Repeat("e", 1024)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{stdoutPath, stderrPath} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "discarded to avoid unbounded log growth") {
			t.Fatalf("managed log placeholder missing in %s: %q", path, raw)
		}
		if len(raw) > 256 {
			t.Fatalf("managed log placeholder should remain small, got %d bytes", len(raw))
		}
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
