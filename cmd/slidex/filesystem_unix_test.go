//go:build !windows

package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestOpenRegularFileForReadRejectsFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "brief.md")
	mkfifoOrSkip(t, fifo)
	f, _, err := openRegularFileForRead(fifo)
	if err == nil {
		_ = f.Close()
		t.Fatal("expected FIFO read target to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "regular file") {
		t.Fatalf("expected regular file rejection, got %v", err)
	}
}

func TestCopyFileRejectsFIFOSource(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "source.pipe")
	mkfifoOrSkip(t, fifo)
	err := copyFile(fifo, filepath.Join(dir, "copy.txt"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "regular file") {
		t.Fatalf("expected FIFO copy source rejection, got %v", err)
	}
}

func TestOpenSecureAppendFileRejectsFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "run_log.jsonl")
	mkfifoOrSkip(t, fifo)
	f, err := openSecureAppendFile(fifo, 0o600)
	if err == nil {
		_ = f.Close()
		t.Fatal("expected FIFO append target to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "regular file") {
		t.Fatalf("expected regular file rejection, got %v", err)
	}
}

func mkfifoOrSkip(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable on this platform or filesystem: %v", err)
	}
}
