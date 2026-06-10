//go:build windows

package main

import (
	"strings"
	"testing"
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
