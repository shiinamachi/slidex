//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func configureWorkbenchCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalWorkbenchProcess(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

func killWorkbenchProcess(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func currentOwnerID() any {
	return os.Getuid()
}

func appServerRuntimeBaseDir() string {
	if base := os.Getenv("XDG_RUNTIME_DIR"); base != "" {
		return base
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("slidex-%d", os.Getuid()))
}

func managedAppServerDefaultListen() (string, error) {
	return "unix://" + filepath.Join(appServerRuntimeBaseDir(), "slidex", "codex-app-server.sock"), nil
}
