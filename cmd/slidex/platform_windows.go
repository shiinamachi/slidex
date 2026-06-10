//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	windowsCreateNewProcessGroup = 0x00000200
	windowsSynchronize           = 0x00100000
	windowsWaitTimeout           = 258
)

func configureWorkbenchCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsCreateNewProcessGroup}
}

func signalWorkbenchProcess(pid int) {
	killProcess(pid)
}

func killWorkbenchProcess(pid int) {
	killProcess(pid)
}

func signalManagedProcess(pid int) {
	killProcess(pid)
}

func killManagedProcess(pid int) {
	killProcess(pid)
}

func killProcess(pid int) {
	if pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Kill()
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(windowsSynchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	status, err := syscall.WaitForSingleObject(handle, 0)
	return err == nil && status == windowsWaitTimeout
}

func currentOwnerID() any {
	for _, name := range []string{"USERNAME", "USER"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return "unknown"
}

func appServerRuntimeBaseDir() string {
	for _, name := range []string{"LOCALAPPDATA", "APPDATA"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("slidex-%v", currentOwnerID()))
}

func managedAppServerDefaultListen() (string, error) {
	port, err := chooseLoopbackPort()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ws://127.0.0.1:%d/app", port), nil
}
