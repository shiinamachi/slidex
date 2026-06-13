//go:build !windows && !darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

func managedAppServerProcessMatchesMetadata(pid int, metadata map[string]any) bool {
	if pid <= 0 {
		return false
	}
	expectedExe := resolvedExecutablePath(metadataString(metadata["processExe"]))
	expectedArgs := metadataStringSlice(metadata["processArgs"])
	if expectedExe == "" || len(expectedArgs) == 0 {
		return false
	}
	processExe, processArgs, ok := procProcessIdentity(pid)
	if !ok {
		return false
	}
	return sameFilesystemPath(expectedExe, processExe) && stringSlicesEqual(processArgs, expectedArgs)
}

func observedManagedAppServerProcessIdentity(pid int, _ *exec.Cmd) (string, []string, bool) {
	return procProcessIdentity(pid)
}

func procProcessIdentity(pid int) (string, []string, bool) {
	if pid <= 0 {
		return "", nil, false
	}
	processExe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", nil, false
	}
	if resolved, err := filepath.EvalSymlinks(processExe); err == nil {
		processExe = resolved
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(raw) == 0 {
		return "", nil, false
	}
	return processExe, splitProcCmdline(raw), true
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
