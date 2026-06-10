//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsCreateNewProcessGroup = 0x00000200
	windowsSynchronize           = 0x00100000
	windowsWaitTimeout           = 258
	windowsMoveFileReplace       = 0x00000001
	windowsMoveFileWriteThrough  = 0x00000008
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func configureWorkbenchCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsCreateNewProcessGroup}
}

func configureManagedAppServerCommand(cmd *exec.Cmd) {
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
	return managedAppServerLoopbackListen()
}

func isReparsePoint(path string) bool {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(pathPtr)
	return err == nil && attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func requirePlatformPrivateFile(path, flagName string) error {
	reason, err := windowsPrivateFileForbiddenReason(path)
	if err != nil {
		return exitCodeError(4, "%s Windows ACL could not be inspected: %v", flagName, err)
	}
	if reason != "" {
		return exitCodeError(4, "%s must be private on Windows; %s", flagName, reason)
	}
	return nil
}

func windowsPrivateFileForbiddenReason(path string) (string, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return "", err
	}
	if sd == nil {
		return "missing security descriptor", nil
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) {
			return "missing DACL", nil
		}
		return "", err
	}
	if dacl == nil {
		return "null DACL", nil
	}
	allowed, err := windowsPrivateFileAllowedSIDs()
	if err != nil {
		return "", err
	}
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			return "", err
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if !windowsAccessMaskGrantsPrivateFileAccess(ace.Mask) {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if windowsSIDAllowed(sid, allowed) {
			continue
		}
		if value := sid.String(); value != "" {
			return fmt.Sprintf("ACL grants file access to %s", value), nil
		}
		return "ACL grants file access to unknown SID", nil
	}
	return "", nil
}

func windowsPrivateFileAllowedSIDs() ([]*windows.SID, error) {
	userSID, err := windowsCurrentUserSID()
	if err != nil {
		return nil, err
	}
	sids := []*windows.SID{userSID}
	for _, sidType := range []windows.WELL_KNOWN_SID_TYPE{
		windows.WinLocalSystemSid,
		windows.WinBuiltinAdministratorsSid,
		windows.WinCreatorOwnerSid,
	} {
		sid, err := windows.CreateWellKnownSid(sidType)
		if err != nil {
			return nil, err
		}
		sids = append(sids, sid)
	}
	return sids, nil
}

func windowsCurrentUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid.Copy()
}

func windowsSIDAllowed(sid *windows.SID, allowed []*windows.SID) bool {
	for _, candidate := range allowed {
		if sid.Equals(candidate) {
			return true
		}
	}
	return false
}

func windowsAccessMaskGrantsPrivateFileAccess(mask windows.ACCESS_MASK) bool {
	const privateFileAccess = windows.ACCESS_MASK(
		windows.GENERIC_READ |
			windows.GENERIC_WRITE |
			windows.GENERIC_ALL |
			windows.MAXIMUM_ALLOWED |
			windows.FILE_GENERIC_READ |
			windows.FILE_GENERIC_WRITE |
			windows.FILE_READ_DATA |
			windows.FILE_WRITE_DATA |
			windows.FILE_APPEND_DATA |
			windows.FILE_READ_EA |
			windows.FILE_WRITE_EA |
			windows.WRITE_DAC |
			windows.WRITE_OWNER)
	return mask&privateFileAccess != 0
}

func replaceFile(src, dst string) error {
	srcPtr, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstPtr, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	ok, _, callErr := moveFileExW.Call(
		uintptr(unsafe.Pointer(srcPtr)),
		uintptr(unsafe.Pointer(dstPtr)),
		uintptr(windowsMoveFileReplace|windowsMoveFileWriteThrough),
	)
	if ok == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("MoveFileExW failed")
	}
	return nil
}
