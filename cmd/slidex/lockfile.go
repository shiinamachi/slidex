package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

const lockReclaimGuardSchema = "slidex.lockReclaim.v1"

var lockFileBeforeRemoveHook func(string)

type lockFileSnapshot struct {
	info  os.FileInfo
	raw   []byte
	rawOK bool
}

func newLockNonce() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func readLockFileSnapshot(path string, maxBytes int64) (lockFileSnapshot, bool) {
	info, err := os.Lstat(path)
	if err != nil || isSymlinkOrReparsePoint(path, info) {
		return lockFileSnapshot{}, false
	}
	snapshot := lockFileSnapshot{info: info}
	raw, err := readRegularFileWithMaxBytes(path, maxBytes)
	if err == nil {
		snapshot.raw = raw
		snapshot.rawOK = true
	}
	return snapshot, true
}

func removeLockFileIfUnchanged(path string, snapshot lockFileSnapshot, maxBytes int64) bool {
	current, ok := readLockFileSnapshot(path, maxBytes)
	if !ok || !os.SameFile(snapshot.info, current.info) {
		return false
	}
	if snapshot.rawOK {
		if !current.rawOK || !bytes.Equal(snapshot.raw, current.raw) {
			return false
		}
	} else if current.info.Size() != snapshot.info.Size() || !current.info.ModTime().Equal(snapshot.info.ModTime()) {
		return false
	}
	if lockFileBeforeRemoveHook != nil {
		lockFileBeforeRemoveHook(path)
	}
	return os.Remove(path) == nil
}

func reclaimStaleLockFile(path string, maxBytes int64, staleSnapshot func(string) (lockFileSnapshot, bool)) bool {
	unlockGuard, ok := acquireLockReclaimGuard(path)
	if !ok {
		return false
	}
	defer unlockGuard()
	snapshot, stale := staleSnapshot(path)
	if !stale {
		return false
	}
	return removeLockFileIfUnchanged(path, snapshot, maxBytes)
}

func acquireLockReclaimGuard(lockPath string) (func(), bool) {
	guardPath := lockPath + ".reclaim"
	if err := rejectSecureInPlaceWriteTarget(guardPath); err != nil {
		return nil, false
	}
	f, err := os.OpenFile(guardPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if err := applyPlatformFileMode(guardPath, 0o600); err != nil {
		_ = f.Close()
		return nil, false
	}
	fileInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, false
	}
	pathInfo, err := os.Lstat(guardPath)
	if err != nil || isSymlinkOrReparsePoint(guardPath, pathInfo) || !os.SameFile(fileInfo, pathInfo) {
		_ = f.Close()
		return nil, false
	}
	if !fileInfo.Mode().IsRegular() {
		_ = f.Close()
		return nil, false
	}
	links, ok, err := secureFileLinkCount(guardPath, fileInfo)
	if err != nil || (ok && links > 1) {
		_ = f.Close()
		return nil, false
	}
	if err := tryLockReclaimGuardFile(f); err != nil {
		_ = f.Close()
		return nil, false
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_, _ = fmt.Fprintf(f, "schema=%s pid=%d nonce=%s acquired=%s\n", lockReclaimGuardSchema, os.Getpid(), newLockNonce(), time.Now().UTC().Format(time.RFC3339))
	_ = f.Sync()
	return func() {
		_ = unlockReclaimGuardFile(f)
		_ = f.Close()
	}, true
}

func releaseLockFile(path string, f *os.File) {
	fileInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || isSymlinkOrReparsePoint(path, pathInfo) || !os.SameFile(fileInfo, pathInfo) {
		_ = f.Close()
		return
	}
	_ = f.Close()
	_ = os.Remove(path)
}
