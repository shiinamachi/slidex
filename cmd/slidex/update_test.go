package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseAssetContractStripsTagVFromAssetNames(t *testing.T) {
	contract, err := releaseAssetContractFor("v0.1.0", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if contract.Version != "0.1.0" {
		t.Fatalf("version = %q", contract.Version)
	}
	if contract.ArchiveName != "slidex_0.1.0_linux_amd64.tar.gz" {
		t.Fatalf("archive name = %q", contract.ArchiveName)
	}
	if contract.ChecksumName != "slidex_0.1.0_checksums.txt" {
		t.Fatalf("checksum name = %q", contract.ChecksumName)
	}
	win, err := releaseAssetContractFor("v0.1.0-e9c033e", "windows", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if win.ArchiveName != "slidex_0.1.0-e9c033e_windows_arm64.zip" {
		t.Fatalf("windows archive name = %q", win.ArchiveName)
	}
}

func TestChannelFromPackageVersionOnlyAcceptsStableAndCanary(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"0.1.0", updateChannelProduction},
		{"v0.1.0", updateChannelProduction},
		{"0.1.0-e9c033e", updateChannelCanary},
		{"0.1.0-beta.1", updateChannelLocalDevelopment},
		{"dev-local", updateChannelLocalDevelopment},
	}
	for _, tc := range tests {
		if got := channelFromPackageVersion(tc.version); got != tc.want {
			t.Fatalf("channelFromPackageVersion(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}

func TestUpdateStatusDetectsImmutableChannelsAndLocalDevelopment(t *testing.T) {
	temp := t.TempDir()
	productionMeta := filepath.Join(temp, "production.json")
	writeInstallMetadataForTest(t, productionMeta, installMetadata{
		SchemaVersion:    installMetadataSchemaVersion,
		ToolName:         toolName,
		Version:          toolVersion,
		Channel:          updateChannelProduction,
		InstallMode:      installModeReleasePackage,
		ReleaseAssetName: "slidex_0.1.0_linux_amd64.tar.gz",
	})
	status, err := currentUpdateStatus(filepath.Join(temp, "prod-root"), productionMeta)
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelProduction || !status.UpdatesEnabled {
		t.Fatalf("production status = %#v", status)
	}

	canaryMeta := filepath.Join(temp, "canary.json")
	writeInstallMetadataForTest(t, canaryMeta, installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion + "-abcdef0",
		Channel:       updateChannelCanary,
		InstallMode:   installModeReleasePackage,
	})
	status, err = currentUpdateStatus(filepath.Join(temp, "canary-root"), canaryMeta)
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelCanary || !status.UpdatesEnabled {
		t.Fatalf("canary status = %#v", status)
	}

	sourceRoot := filepath.Join(temp, "checkout")
	for _, dir := range []string{".git", filepath.Join("cmd", "slidex")} {
		if err := os.MkdirAll(filepath.Join(sourceRoot, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"go.mod", filepath.Join("cmd", "slidex", "main.go")} {
		if err := os.WriteFile(filepath.Join(sourceRoot, file), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	status, err = currentUpdateStatus(sourceRoot, filepath.Join(sourceRoot, ".slidex", "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled || status.Status != "disabled" {
		t.Fatalf("local development status = %#v", status)
	}
}

func TestUpdateDiscoveryHonorsProductionAndCanaryChannels(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-abcdef0","draft":false,"prerelease":true,"assets":[
	    {"name":"slidex_0.2.0-abcdef0_linux_amd64.tar.gz","browser_download_url":"https://example.invalid/canary.tgz","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	    {"name":"slidex_0.2.0-abcdef0_checksums.txt","browser_download_url":"https://example.invalid/canary.txt"}
	  ]},
	  {"tag_name":"v0.1.0","draft":false,"prerelease":false,"assets":[
	    {"name":"slidex_0.1.0_linux_amd64.tar.gz","browser_download_url":"https://example.invalid/stable.tgz","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	    {"name":"slidex_0.1.0_checksums.txt","browser_download_url":"https://example.invalid/stable.txt"}
	  ]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	stable, err := selectUpdateRelease(updateChannelProduction, releases)
	if err != nil {
		t.Fatal(err)
	}
	if stable.TagName != "v0.1.0" {
		t.Fatalf("production selected %s", stable.TagName)
	}
	canary, err := selectUpdateRelease(updateChannelCanary, releases)
	if err != nil {
		t.Fatal(err)
	}
	if canary.TagName != "v0.2.0-abcdef0" {
		t.Fatalf("canary selected %s", canary.TagName)
	}
	contract, err := releaseAssetContractFor(canary.TagName, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	archive, checksum, err := canary.requiredAssets(contract)
	if err != nil {
		t.Fatal(err)
	}
	if archive.Name != "slidex_0.2.0-abcdef0_linux_amd64.tar.gz" || checksum.Name != "slidex_0.2.0-abcdef0_checksums.txt" {
		t.Fatalf("required assets = %q / %q", archive.Name, checksum.Name)
	}
}

func TestVerifyReleaseAssetSHA256FailsClosed(t *testing.T) {
	payload := []byte("release archive")
	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	if _, err := verifyReleaseAssetSHA256("slidex_0.1.0_linux_amd64.tar.gz", payload, "", ""); err == nil {
		t.Fatal("missing digest evidence should fail")
	}
	if _, err := verifyReleaseAssetSHA256("slidex_0.1.0_linux_amd64.tar.gz", payload, actual+"  other.tar.gz\n", ""); err == nil {
		t.Fatal("missing asset checksum line should fail")
	}
	if _, err := verifyReleaseAssetSHA256("slidex_0.1.0_linux_amd64.tar.gz", payload, strings.Repeat("0", 64)+"  slidex_0.1.0_linux_amd64.tar.gz\n", ""); err == nil {
		t.Fatal("checksum mismatch should fail")
	}
	if got, err := verifyReleaseAssetSHA256("slidex_0.1.0_linux_amd64.tar.gz", payload, actual+"  slidex_0.1.0_linux_amd64.tar.gz\n", "sha256:"+actual); err != nil || got != actual {
		t.Fatalf("verified digest = %q, err = %v", got, err)
	}
}

func TestValidateCandidateBundleChecksBundledRuntimeContracts(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0")
	findings := validateCandidateBundle(root, "0.2.0")
	if hasFailures(findings) {
		t.Fatalf("candidate should validate: %#v", findings)
	}
	if err := os.WriteFile(filepath.Join(root, "plugins", "slidex", ".codex-plugin", "version-lock.json"), []byte(`{"pluginVersion":"0.1.0","slidexCliVersion":"0.1.0","requiredCodexCliVersion":"0.138.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	findings = validateCandidateBundle(root, "0.2.0")
	if !hasFailures(findings) {
		t.Fatalf("candidate drift should fail: %#v", findings)
	}
}

func TestReleasePackageArchiveIncludesInstallMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell release smoke uses the Unix package path")
	}
	root := repoRootForTest(t)
	dist := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "package-release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"SLIDEX_RELEASE_VERSION=v"+toolVersion,
		"SLIDEX_BUILD_CHANNEL=production",
		"SLIDEX_TARGETS=linux/amd64",
		"SLIDEX_DIST_DIR="+dist,
		"SLIDEX_BUILD_TIME=2026-06-10T00:00:00Z",
		"SLIDEX_COMMIT_SHA=0123456789abcdef",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package release failed: %v\n%s", err, out)
	}
	archive := filepath.Join(dist, "slidex_"+toolVersion+"_linux_amd64.tar.gz")
	metadata := readInstallMetadataFromTarGzForTest(t, archive, "slidex_"+toolVersion+"_linux_amd64/.slidex/install.json")
	if metadata.Channel != updateChannelProduction {
		t.Fatalf("metadata channel = %q", metadata.Channel)
	}
	if metadata.Tag != "v"+toolVersion {
		t.Fatalf("metadata tag = %q", metadata.Tag)
	}
	if metadata.ReleaseAssetName != "slidex_"+toolVersion+"_linux_amd64.tar.gz" {
		t.Fatalf("metadata asset = %q", metadata.ReleaseAssetName)
	}
	checksum := filepath.Join(dist, "slidex_"+toolVersion+"_checksums.txt")
	if _, err := os.Stat(checksum); err != nil {
		t.Fatalf("checksum name should use asset version without v: %v", err)
	}
}

func writeInstallMetadataForTest(t *testing.T, path string, metadata installMetadata) {
	t.Helper()
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCandidateBundleForTest(t *testing.T, root, version string) {
	t.Helper()
	dirs := []string{
		"decks/_template",
		"schemas",
		"plugins/slidex/.codex-plugin",
		".agents/plugins",
		"internal/codex/protocol",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	files := map[string]string{
		"VERSION": version,
		binary:    "",
		"plugins/slidex/.codex-plugin/plugin.json": `{
		  "name":"slidex",
		  "version":"` + version + `+codex.test",
		  "author":{"name":"shiinamachi"},
		  "license":"MIT",
		  "skills":"./skills/",
		  "mcpServers":"./.mcp.json"
		}`,
		"plugins/slidex/.codex-plugin/version-lock.json": `{
		  "pluginVersion":"` + version + `",
		  "slidexCliVersion":"` + version + `",
		  "requiredCodexCliVersion":"0.138.0"
		}`,
		".agents/plugins/marketplace.json": `{
		  "plugins":[{"name":"slidex","source":{"source":"local","path":"./plugins/slidex"}}]
		}`,
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func readInstallMetadataFromTarGzForTest(t *testing.T, path, wantName string) installMetadata {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err != nil {
			t.Fatalf("metadata %s not found in %s", wantName, path)
		}
		if header.Name != wantName {
			continue
		}
		var metadata installMetadata
		if err := json.NewDecoder(tr).Decode(&metadata); err != nil {
			t.Fatal(err)
		}
		return metadata
	}
}
