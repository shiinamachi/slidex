//go:build !windows && !darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

func workbenchProcessMatchesManifest(manifest workbenchManifest) bool {
	if manifest.PID <= 0 {
		return false
	}
	currentExe, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(currentExe); err == nil {
		currentExe = resolved
	}
	processExe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", manifest.PID))
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(processExe); err == nil {
		processExe = resolved
	}
	if !sameFilesystemPath(currentExe, processExe) {
		return false
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", manifest.PID))
	if err != nil || len(raw) == 0 {
		return false
	}
	return workbenchServeArgsMatch(splitProcCmdline(raw), manifest)
}

func splitProcCmdline(raw []byte) []string {
	raw = bytes.TrimRight(raw, "\x00")
	if len(raw) == 0 {
		return nil
	}
	parts := bytes.Split(raw, []byte{0})
	args := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			args = append(args, string(part))
		}
	}
	return args
}
