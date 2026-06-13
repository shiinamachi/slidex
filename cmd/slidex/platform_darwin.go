//go:build darwin

package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"
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
	processExe, processArgs, ok := darwinProcessIdentity(manifest.PID)
	if !ok {
		return false
	}
	if !sameFilesystemPath(currentExe, processExe) {
		return false
	}
	return workbenchServeArgsMatch(processArgs, manifest)
}

func managedAppServerProcessMatchesMetadata(pid int, metadata map[string]any) bool {
	if pid <= 0 {
		return false
	}
	processExe, processArgs, ok := darwinProcessIdentity(pid)
	if !ok {
		return false
	}
	return managedAppServerProcessIdentityMatchesMetadata(processExe, processArgs, metadata)
}

func observedManagedAppServerProcessIdentity(pid int, cmd *exec.Cmd) (string, []string, bool) {
	return darwinProcessIdentity(pid)
}

func darwinProcessIdentity(pid int) (string, []string, bool) {
	if pid <= 0 {
		return "", nil, false
	}
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", nil, false
	}
	processExe, processArgs, ok := parseDarwinProcArgs2(raw)
	if !ok {
		return "", nil, false
	}
	if resolved, err := filepath.EvalSymlinks(processExe); err == nil {
		processExe = resolved
	}
	return processExe, processArgs, true
}

func parseDarwinProcArgs2(raw []byte) (string, []string, bool) {
	if len(raw) < 4 {
		return "", nil, false
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	if argc <= 0 || argc > 4096 {
		return "", nil, false
	}
	payload := raw[4:]
	exeEnd := bytes.IndexByte(payload, 0)
	if exeEnd <= 0 {
		return "", nil, false
	}
	processExe := string(payload[:exeEnd])
	payload = payload[exeEnd+1:]
	payload = bytes.TrimLeft(payload, "\x00")
	args := make([]string, 0, argc)
	for len(args) < argc {
		argEnd := bytes.IndexByte(payload, 0)
		if argEnd < 0 {
			return "", nil, false
		}
		args = append(args, string(payload[:argEnd]))
		payload = payload[argEnd+1:]
	}
	if processExe == "" || len(args) != argc {
		return "", nil, false
	}
	return processExe, args, true
}
