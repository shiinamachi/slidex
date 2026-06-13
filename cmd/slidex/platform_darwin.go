//go:build darwin

package main

func workbenchProcessMatchesManifest(manifest workbenchManifest) bool {
	// macOS does not expose a portable, exact executable+argv check through the
	// APIs used by this package. Avoid substring-based ps matching before
	// signaling a PID; token-authenticated HTTP shutdown remains the Darwin stop
	// path unless an exact matcher is added.
	return false
}
