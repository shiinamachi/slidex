//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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
	configureProcessGroupCommand(cmd)
}

func configureManagedAppServerCommand(cmd *exec.Cmd) {
	configureProcessGroupCommand(cmd)
}

func configureProcessGroupCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsCreateNewProcessGroup}
}

func signalWorkbenchProcess(pid int) {
	killProcess(pid)
}

func killWorkbenchProcess(pid int) {
	killProcess(pid)
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
	processExe, ok := windowsProcessImagePath(manifest.PID)
	if !ok {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(processExe); err == nil {
		processExe = resolved
	}
	if !sameFilesystemPath(currentExe, processExe) {
		return false
	}
	commandLine, ok := windowsProcessCommandLine(manifest.PID)
	if !ok {
		return false
	}
	return workbenchServeArgsMatch(splitWindowsCommandLine(commandLine), manifest)
}

func windowsProcessImagePath(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buffer[:size]), true
}

func windowsProcessCommandLine(pid int) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()
	script := fmt.Sprintf(`$p = Get-CimInstance Win32_Process -Filter "ProcessId = %d"; if ($null -ne $p) { [Console]::Out.Write($p.CommandLine) }`, pid)
	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil || ctx.Err() != nil {
		return "", false
	}
	commandLine := strings.TrimSpace(string(out))
	return commandLine, commandLine != ""
}

func splitWindowsCommandLine(commandLine string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}
	for _, r := range commandLine {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case ' ', '\t', '\r', '\n':
			if inQuotes {
				current.WriteRune(r)
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return args
}

func signalManagedProcess(pid int) {
	killProcess(pid)
}

func killManagedProcess(pid int) {
	killProcess(pid)
}

func killProcess(pid int) {
	for _, treePID := range windowsProcessTreePIDs(pid) {
		killSingleProcess(treePID)
	}
}

func killSingleProcess(pid int) {
	if pid <= 0 {
		return
	}
	if pid == os.Getpid() {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Kill()
}

type windowsProcessEntry struct {
	pid    uint32
	parent uint32
}

func windowsProcessTreePIDs(root int) []int {
	if root <= 0 {
		return nil
	}
	entries, err := windowsProcessEntries()
	if err != nil {
		return []int{root}
	}
	order := windowsProcessTreeOrder(uint32(root), entries)
	if len(order) == 0 {
		return []int{root}
	}
	return order
}

func windowsProcessEntries() ([]windowsProcessEntry, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	var procEntry windows.ProcessEntry32
	procEntry.Size = uint32(unsafe.Sizeof(procEntry))
	if err := windows.Process32First(snapshot, &procEntry); err != nil {
		return nil, err
	}
	var entries []windowsProcessEntry
	for {
		entries = append(entries, windowsProcessEntry{pid: procEntry.ProcessID, parent: procEntry.ParentProcessID})
		err = windows.Process32Next(snapshot, &procEntry)
		if err == nil {
			continue
		}
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			break
		}
		return nil, err
	}
	return entries, nil
}

func windowsProcessTreeOrder(root uint32, entries []windowsProcessEntry) []int {
	childrenByParent := make(map[uint32][]uint32)
	for _, entry := range entries {
		if entry.pid == 0 || entry.pid == entry.parent {
			continue
		}
		childrenByParent[entry.parent] = append(childrenByParent[entry.parent], entry.pid)
	}
	visited := map[uint32]bool{}
	var order []int
	var visit func(uint32)
	visit = func(pid uint32) {
		if visited[pid] {
			return
		}
		visited[pid] = true
		for _, child := range childrenByParent[pid] {
			visit(child)
		}
		order = append(order, int(pid))
	}
	visit(root)
	return order
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

func applyPlatformFileMode(path string, mode os.FileMode) error {
	if mode&0o077 != 0 {
		return nil
	}
	return windowsApplyPrivateDACL(path, false)
}

func executableProductVersion(path string) string {
	var zero windows.Handle
	infoSize, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || infoSize == 0 {
		return ""
	}
	versionInfo := make([]byte, infoSize)
	if err := windows.GetFileVersionInfo(path, 0, infoSize, unsafe.Pointer(&versionInfo[0])); err != nil {
		return ""
	}
	var fixedInfo *windows.VS_FIXEDFILEINFO
	fixedInfoLen := uint32(unsafe.Sizeof(*fixedInfo))
	if err := windows.VerQueryValue(unsafe.Pointer(&versionInfo[0]), `\`, unsafe.Pointer(&fixedInfo), &fixedInfoLen); err != nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		(fixedInfo.FileVersionMS>>16)&0xffff,
		fixedInfo.FileVersionMS&0xffff,
		(fixedInfo.FileVersionLS>>16)&0xffff,
		fixedInfo.FileVersionLS&0xffff,
	)
}

func applyPlatformDirMode(path string, mode os.FileMode) error {
	if mode&0o077 != 0 {
		return nil
	}
	return windowsApplyPrivateDACL(path, true)
}

func windowsApplyPrivateDACL(path string, inherit bool) error {
	acl, err := windowsPrivateACL(inherit)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

func windowsPrivateACL(inherit bool) (*windows.ACL, error) {
	userSID, err := windowsCurrentUserSID()
	if err != nil {
		return nil, err
	}
	sids := []*windows.SID{userSID}
	for _, sidType := range []windows.WELL_KNOWN_SID_TYPE{
		windows.WinLocalSystemSid,
		windows.WinBuiltinAdministratorsSid,
	} {
		sid, err := windows.CreateWellKnownSid(sidType)
		if err != nil {
			return nil, err
		}
		sids = append(sids, sid)
	}
	inheritance := uint32(windows.NO_INHERITANCE)
	if inherit {
		inheritance = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(sids))
	for i, sid := range sids {
		trusteeType := windows.TRUSTEE_TYPE(windows.TRUSTEE_IS_WELL_KNOWN_GROUP)
		if i == 0 {
			trusteeType = windows.TRUSTEE_IS_USER
		}
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       inheritance,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  trusteeType,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	return windows.ACLFromEntries(entries, nil)
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

func secureFileLinkCount(path string, info os.FileInfo) (uint64, bool, error) {
	if info == nil || !info.Mode().IsRegular() {
		return 0, false, nil
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return 0, false, err
	}
	defer windows.CloseHandle(handle)
	var data windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &data); err != nil {
		return 0, false, err
	}
	return uint64(data.NumberOfLinks), true, nil
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
