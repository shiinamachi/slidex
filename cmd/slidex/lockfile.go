package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

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
	return os.Remove(path) == nil
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
