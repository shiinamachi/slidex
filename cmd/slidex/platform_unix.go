//go:build !windows

package main

import (
	"bytes"
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

func tryLockReclaimGuardFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockReclaimGuardFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
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
