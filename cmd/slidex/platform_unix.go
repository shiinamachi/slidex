//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

func configureWorkbenchCommand(cmd *exec.Cmd) {
	configureProcessGroupCommand(cmd)
}

func configureManagedAppServerCommand(cmd *exec.Cmd) {
	configureProcessGroupCommand(cmd)
}

func configureProcessGroupCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalWorkbenchProcess(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

func killWorkbenchProcess(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func signalManagedProcess(pid int) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err == nil {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}
}

func killManagedProcess(pid int) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
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
	socketPath := filepath.Join(appServerRuntimeBaseDir(), "slidex", "codex-app-server.sock")
	if unixSocketPathFits(runtime.GOOS, socketPath) {
		return "unix://" + socketPath, nil
	}
	return managedAppServerLoopbackListen()
}

func isReparsePoint(string) bool {
	return false
}

func requirePlatformPrivateFile(string, string) error {
	return nil
}

func applyPlatformFileMode(string, os.FileMode) error {
	return nil
}

func applyPlatformDirMode(string, os.FileMode) error {
	return nil
}

func secureFileLinkCount(_ string, info os.FileInfo) (uint64, bool, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false, nil
	}
	return uint64(stat.Nlink), true, nil
}

func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}

func executableProductVersion(string) string {
	return ""
}
