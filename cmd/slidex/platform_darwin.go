//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func workbenchProcessMatchesManifest(manifest workbenchManifest) bool {
	if manifest.PID <= 0 {
		return false
	}
	if !workbenchManifestHasTrustedControl(manifest) {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(manifest.PID), "-o", "command=").Output()
	if err != nil {
		return false
	}
	return darwinWorkbenchCommandMatchesManifest(string(out), manifest)
}

func darwinWorkbenchCommandMatchesManifest(command string, manifest workbenchManifest) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	currentExe, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(currentExe); err == nil {
		currentExe = resolved
	}
	if !strings.Contains(command, currentExe) && !strings.Contains(command, filepath.Base(currentExe)) {
		return false
	}
	for _, want := range []string{
		"workbench",
		"serve",
		manifest.SessionID,
		strconv.Itoa(manifest.Port),
		filepath.FromSlash(manifest.DeckDir),
	} {
		if strings.TrimSpace(want) == "" || !strings.Contains(command, want) {
			return false
		}
	}
	return true
}
