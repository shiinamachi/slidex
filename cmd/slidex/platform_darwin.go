//go:build darwin

package main

func workbenchProcessMatchesManifest(manifest workbenchManifest) bool {
	if manifest.PID <= 0 {
		return false
	}
	return workbenchManifestHasTrustedControl(manifest)
}
