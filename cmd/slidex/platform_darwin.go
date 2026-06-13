//go:build darwin

package main

import "os/exec"

func workbenchProcessMatchesManifest(manifest workbenchManifest) bool {
	// macOS does not expose a portable, exact executable+argv check through the
	// APIs used by this package. Avoid substring-based ps matching before
	// signaling a PID; token-authenticated HTTP shutdown remains the Darwin stop
	// path unless an exact matcher is added.
	return false
}

func managedAppServerProcessMatchesMetadata(pid int, metadata map[string]any) bool {
	// Keep the same fail-closed stance as Workbench process verification until a
	// portable exact executable+argv matcher is added for macOS.
	return false
}

func observedManagedAppServerProcessIdentity(pid int, cmd *exec.Cmd) (string, []string, bool) {
	return "", nil, false
}
