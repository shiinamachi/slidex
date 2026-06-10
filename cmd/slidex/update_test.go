package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	if status.CurrentVersion != toolVersion+"-abcdef0" {
		t.Fatalf("canary current version should come from package metadata, got %q", status.CurrentVersion)
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

func TestUpdateDiscoveryDoesNotSelectOlderProductionRelease(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.1.0","draft":false,"prerelease":false,"assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selectUpdateReleaseForCurrent(updateChannelProduction, "0.2.0", releases); err == nil || !strings.Contains(err.Error(), "no matching production release") {
		t.Fatalf("expected older production release to be rejected, got %v", err)
	}
}

func TestUpdateDiscoverySelectsNewestProductionReleaseWithoutAPISorting(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.1.0","draft":false,"prerelease":false,"published_at":"2026-01-01T00:00:00Z","assets":[]},
	  {"tag_name":"v0.3.0","draft":false,"prerelease":false,"published_at":"2026-03-01T00:00:00Z","assets":[]},
	  {"tag_name":"v0.2.0","draft":false,"prerelease":false,"published_at":"2026-02-01T00:00:00Z","assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	release, err := selectUpdateReleaseForCurrent(updateChannelProduction, "0.1.0", releases)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.3.0" {
		t.Fatalf("production selected %s", release.Version)
	}
}

func TestUpdateDiscoveryOrdersSameBaseCanaryOnlyWhenCurrentReleaseIsKnown(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-aaaaaaa","draft":false,"prerelease":true,"published_at":"2026-02-01T00:00:00Z","assets":[]},
	  {"tag_name":"v0.2.0-bbbbbbb","draft":false,"prerelease":true,"published_at":"2026-02-02T00:00:00Z","assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	next, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.2.0-aaaaaaa", releases)
	if err != nil {
		t.Fatal(err)
	}
	if next.Version != "0.2.0-bbbbbbb" {
		t.Fatalf("next canary = %s", next.Version)
	}
	if _, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.2.0-ccccccc", releases); err == nil || !strings.Contains(err.Error(), "refusing to infer same-base canary ordering") {
		t.Fatalf("expected unknown same-base canary ordering to fail closed, got %v", err)
	}
}

func TestUpdateDiscoverySelectsNewestCanaryReleaseWithoutAPISorting(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-1111111","draft":false,"prerelease":true,"published_at":"2026-02-01T00:00:00Z","assets":[]},
	  {"tag_name":"v0.2.0-2222222","draft":false,"prerelease":true,"published_at":"2026-02-02T00:00:00Z","assets":[]},
	  {"tag_name":"v0.1.0-fffffff","draft":false,"prerelease":true,"published_at":"2026-01-01T00:00:00Z","assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	release, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.1.0-fffffff", releases)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.2.0-2222222" {
		t.Fatalf("canary selected %s", release.Version)
	}
}

func TestUpdateDiscoveryFailsClosedWhenSameBaseCanaryOrderingIsMissing(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-aaaaaaa","draft":false,"prerelease":true,"assets":[]},
	  {"tag_name":"v0.2.0-bbbbbbb","draft":false,"prerelease":true,"assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.2.0-aaaaaaa", releases); err == nil || !strings.Contains(err.Error(), "release metadata does not determine ordering") {
		t.Fatalf("expected missing same-base canary metadata to fail closed, got %v", err)
	}
}

func TestUpdateDiscoveryRequiresCanaryPrereleaseFlag(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-abcdef0","draft":false,"prerelease":false,"assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.1.0-aaaaaaa", releases); err == nil || !strings.Contains(err.Error(), "no matching canary release") {
		t.Fatalf("expected non-prerelease canary tag to be rejected, got %v", err)
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
	canaryRoot := t.TempDir()
	writeCandidateBundleForTest(t, canaryRoot, "0.2.0-abcdef0")
	findings = validateCandidateBundle(canaryRoot, "0.2.0-abcdef0")
	if hasFailures(findings) {
		t.Fatalf("canary candidate should validate with base runtime versions: %#v", findings)
	}
	if err := os.WriteFile(filepath.Join(root, "plugins", "slidex", ".codex-plugin", "version-lock.json"), []byte(`{"pluginVersion":"0.1.0","slidexCliVersion":"0.1.0","requiredCodexCliVersion":"0.138.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	findings = validateCandidateBundle(root, "0.2.0")
	if !hasFailures(findings) {
		t.Fatalf("candidate drift should fail: %#v", findings)
	}
}

func TestValidateCandidateBundleChecksBinaryVersion(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0")
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	writeCandidateBinaryForTest(t, filepath.Join(root, binary), "0.1.0")
	findings := validateCandidateBundle(root, "0.2.0")
	if !findingCheckPresent(findings, "update.candidate_binary_version") {
		t.Fatalf("candidate binary version drift should fail: %#v", findings)
	}
}

func TestValidateCandidateBundleChecksDoctorStatus(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0")
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	writeCandidateBinaryForTestWithDoctorStatus(t, filepath.Join(root, binary), "0.2.0", "fail")
	findings := validateCandidateBundle(root, "0.2.0")
	if !findingCheckPresent(findings, "update.candidate_doctor") {
		t.Fatalf("candidate doctor failure should fail: %#v", findings)
	}
}

func TestValidateCandidateBundleChecksInstallMetadataFields(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0")
	metadataPath := filepath.Join(root, ".slidex", "install.json")
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata installMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata.Commit = ""
	updated, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	findings := validateCandidateBundle(root, "0.2.0")
	if !findingCheckPresent(findings, "update.candidate_install_metadata") {
		t.Fatalf("candidate install metadata omission should fail: %#v", findings)
	}
}

func findingCheckPresent(findings []qaFinding, check string) bool {
	for _, finding := range findings {
		if finding.Check == check {
			return true
		}
	}
	return false
}

func TestApplyCandidateBundleFailsForInvalidCandidate(t *testing.T) {
	installRoot := t.TempDir()
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	result, err := applyCandidateBundle(status, t.TempDir(), "0.2.0", "v0.2.0", allowUnverifiedAttestationForTest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "candidate-invalid" || !hasFailures(result.CandidateValidation) {
		t.Fatalf("invalid candidate result = %#v", result)
	}
}

func TestApplyCandidateBundleReplacesInstallRootAndMarksRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses pending update handoff because the running executable can be locked")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	result, err := applyCandidateBundle(status, candidate, "0.2.0", "v0.2.0", allowUnverifiedAttestationForTest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.RestartRequired {
		t.Fatalf("apply result = %#v", result)
	}
	if result.PluginVerificationStatus != "restart_required" {
		t.Fatalf("apply plugin status = %q", result.PluginVerificationStatus)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != "0.2.0" {
		t.Fatalf("activated VERSION = %q", got)
	}
	if _, err := os.Stat(result.BackupRoot); err != nil {
		t.Fatalf("backup root missing: %v", err)
	}
	status, err = currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !status.RestartRequired || status.PluginVerificationStatus != "restart_required" {
		t.Fatalf("post-apply update status = %#v", status)
	}
	if status.InstalledMetadata == nil || status.InstalledMetadata.InstallRoot != filepath.ToSlash(installRoot) {
		t.Fatalf("install metadata not updated: %#v", status.InstalledMetadata)
	}
	metadata := status.InstalledMetadata
	if metadata.Version != "0.2.0" || metadata.Channel != updateChannelProduction || metadata.Tag != "v0.2.0" || metadata.InstallMode != installModeReleasePackage {
		t.Fatalf("install metadata version/channel fields not updated: %#v", metadata)
	}
	expectedAsset, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ReleaseAssetName != expectedAsset.ArchiveName || metadata.Commit != "0123456789abcdef" || metadata.BuildTime != "2026-06-10T00:00:00Z" {
		t.Fatalf("install metadata package identity not preserved: %#v", metadata)
	}
	if metadata.OS != runtime.GOOS || metadata.Arch != runtime.GOARCH {
		t.Fatalf("install metadata platform = %s/%s", metadata.OS, metadata.Arch)
	}
	if _, err := time.Parse(time.RFC3339, metadata.InstalledAt); err != nil {
		t.Fatalf("install metadata installedAt must be RFC3339, got %q: %v", metadata.InstalledAt, err)
	}
}

func TestRunUpdateApplyDownloadsReleaseAssets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses pending update handoff because the running executable can be locked")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	contract, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(parent, contract.ArchiveName)
	writeTarGzFromDirForTest(t, archivePath, candidate, strings.TrimSuffix(contract.ArchiveName, ".tar.gz"))
	archivePayload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archivePayload)
	digest := hex.EncodeToString(sum[:])
	checksumText := digest + "  " + contract.ArchiveName + "\n"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[{"tag_name":"v0.2.0","draft":false,"prerelease":false,"assets":[{"name":%q,"browser_download_url":%q,"digest":%q},{"name":%q,"browser_download_url":%q}]}]`,
				contract.ArchiveName,
				server.URL+"/assets/"+contract.ArchiveName,
				"sha256:"+digest,
				contract.ChecksumName,
				server.URL+"/assets/"+contract.ChecksumName,
			)
		case "/assets/" + contract.ArchiveName:
			_, _ = w.Write(archivePayload)
		case "/assets/" + contract.ChecksumName:
			_, _ = w.Write([]byte(checksumText))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--api-url", server.URL + "/releases", "--attestation-policy", attestationPolicyAllowUnverified, "--yes", "--json"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != "0.2.0" {
		t.Fatalf("downloaded update VERSION = %q", got)
	}
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !status.RestartRequired || status.TargetVersion != "0.2.0" {
		t.Fatalf("downloaded apply status = %#v", status)
	}
}

func TestRunUpdateApplyRequiresAttestationBeforeActivation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses pending update handoff because the running executable can be locked")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	contract, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(parent, contract.ArchiveName)
	writeTarGzFromDirForTest(t, archivePath, candidate, strings.TrimSuffix(contract.ArchiveName, ".tar.gz"))
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	checksumPath := filepath.Join(parent, contract.ChecksumName)
	if err := os.WriteFile(checksumPath, []byte(hex.EncodeToString(sum[:])+"  "+contract.ArchiveName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	err = runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--archive", archivePath, "--checksums", checksumPath, "--target-version", "0.2.0", "--target-tag", "v0.2.0", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "attestation") {
		t.Fatalf("expected attestation failure, got %v", err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("install root should not activate without attestation, got VERSION %q", got)
	}
}

func TestVerifyReleaseAttestationRequiresGitHubCLIByDefault(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result, err := verifyReleaseAttestation("/tmp/slidex_0.2.0_linux_amd64.tar.gz", "v0.2.0", attestationPolicyRequire)
	if err == nil {
		t.Fatal("required attestation verification should fail without gh")
	}
	if result.Policy != attestationPolicyRequire || result.Status != "fail" || !strings.Contains(result.Error, "GitHub CLI") {
		t.Fatalf("unexpected attestation result: %#v err=%v", result, err)
	}
}

func TestVerifyReleaseAttestationRunsRequiredGitHubChecks(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh.log")
	writeFakeGitHubCLIForTest(t, filepath.Join(binDir, executableNameForTest("gh")))
	t.Setenv("PATH", binDir)
	t.Setenv("GH_LOG", logPath)

	archivePath := filepath.Join(t.TempDir(), "slidex_0.2.0_linux_amd64.tar.gz")
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := verifyReleaseAttestation(archivePath, "v0.2.0", attestationPolicyRequire)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "verified" || result.Policy != attestationPolicyRequire {
		t.Fatalf("unexpected attestation result: %#v", result)
	}
	log := readFileOrEmpty(logPath)
	wants := []string{
		"release verify v0.2.0 --repo shiinamachi/slidex",
		"release verify-asset v0.2.0 " + archivePath + " --repo shiinamachi/slidex",
		"attestation verify " + archivePath + " --repo shiinamachi/slidex --cert-oidc-issuer https://token.actions.githubusercontent.com --cert-identity-regex",
	}
	last := -1
	for _, want := range wants {
		index := strings.Index(log, want)
		if index < 0 {
			t.Fatalf("fake gh log missing %q:\n%s", want, log)
		}
		if index <= last {
			t.Fatalf("fake gh commands out of order:\n%s", log)
		}
		last = index
	}
}

func TestVerifyReleaseAttestationCanBeExplicitlyBypassed(t *testing.T) {
	result, err := verifyReleaseAttestation("/tmp/slidex_0.2.0_linux_amd64.tar.gz", "v0.2.0", attestationPolicyAllowUnverified)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "skipped" || result.Policy != attestationPolicyAllowUnverified {
		t.Fatalf("unexpected attestation result: %#v", result)
	}
}

func TestPrintUpdateStatusIncludesPluginStatus(t *testing.T) {
	out := captureStdoutForTest(t, func() {
		printUpdateStatus(updateStatus{
			ToolName:                 toolName,
			CurrentVersion:           toolVersion,
			Channel:                  updateChannelProduction,
			Status:                   "verification-failed",
			PluginVerificationStatus: "drift",
			NextVerificationCommand:  "slidex update verify --json",
		})
	})
	if !strings.Contains(out, "plugin status: drift") {
		t.Fatalf("status output missing plugin status:\n%s", out)
	}
}

func TestUpdateApplyRejectsLocalDevelopmentStatus(t *testing.T) {
	sourceRoot := t.TempDir()
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
	candidate := filepath.Join(t.TempDir(), "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	err := runUpdateApply([]string{"--install-root", sourceRoot, "--metadata", filepath.Join(sourceRoot, ".slidex", "missing.json"), "--candidate", candidate, "--target-version", "0.2.0", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "updates are disabled") {
		t.Fatalf("local-development apply err = %v", err)
	}
}

func TestStagePendingUpdateHandoffMarksRestartRequired(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	stagedRoot, pendingPath, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stagedRoot, "VERSION")); err != nil {
		t.Fatalf("staged candidate missing: %v", err)
	}
	if pathWithin(installRoot, stagedRoot) {
		t.Fatalf("pending staged root should be outside install root, got %s", stagedRoot)
	}
	if _, err := os.Stat(pendingPath); err != nil {
		t.Fatalf("pending update missing: %v", err)
	}
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !status.PendingActivation || status.PendingActivationCommand == "" {
		t.Fatalf("pending activation not exposed in update status: %#v", status)
	}
	if status.PendingUpdate == nil || status.PendingUpdate.ActivatorPath == "" {
		t.Fatalf("pending activator not recorded: %#v", status.PendingUpdate)
	}
	if _, err := os.Stat(filepath.FromSlash(status.PendingUpdate.ActivatorPath)); err != nil {
		t.Fatalf("pending activator missing: %v", err)
	}
	if !strings.Contains(status.PendingActivationCommand, filepath.ToSlash(status.PendingUpdate.ActivatorPath)) {
		t.Fatalf("pending activation command should use activator path: %s", status.PendingActivationCommand)
	}
	if !status.RestartRequired || status.PluginVerificationStatus != "restart_required" || status.TargetVersion != "0.2.0" {
		t.Fatalf("pending handoff update status = %#v", status)
	}
	if !findingCheckPresent(updateVerificationFindings(status), "update.pending_activation") {
		t.Fatalf("pending activation finding missing")
	}
}

func TestActivatePendingUpdateAppliesStagedBundle(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	result, err := activatePendingUpdate(status)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.RestartRequired || result.PluginVerificationStatus != "restart_required" {
		t.Fatalf("activate pending result = %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != "0.2.0" {
		t.Fatalf("activated pending VERSION = %q", got)
	}
	if _, err := os.Stat(result.BackupRoot); err != nil {
		t.Fatalf("backup root missing: %v", err)
	}
	if _, err := os.Stat(pendingUpdatePath(installRoot)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending update manifest should not remain in activated root: %v", err)
	}
	status, err = currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingActivation || !status.RestartRequired || status.PluginVerificationStatus != "restart_required" {
		t.Fatalf("post activation status = %#v", status)
	}
	metadata := status.InstalledMetadata
	if metadata == nil {
		t.Fatal("post activation install metadata missing")
	}
	if metadata.Version != "0.2.0" || metadata.Channel != updateChannelProduction || metadata.Tag != "v0.2.0" || metadata.InstallMode != installModeReleasePackage {
		t.Fatalf("pending activation metadata version/channel fields not updated: %#v", metadata)
	}
	expectedAsset, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ReleaseAssetName != expectedAsset.ArchiveName || metadata.Commit != "0123456789abcdef" || metadata.BuildTime != "2026-06-10T00:00:00Z" {
		t.Fatalf("pending activation metadata package identity not preserved: %#v", metadata)
	}
	if metadata.OS != runtime.GOOS || metadata.Arch != runtime.GOARCH {
		t.Fatalf("pending activation metadata platform = %s/%s", metadata.OS, metadata.Arch)
	}
	if _, err := time.Parse(time.RFC3339, metadata.InstalledAt); err != nil {
		t.Fatalf("pending activation metadata installedAt must be RFC3339, got %q: %v", metadata.InstalledAt, err)
	}
}

func TestRunUpdateActivatePendingRequiresYes(t *testing.T) {
	err := runUpdateActivatePending([]string{"--install-root", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("activate pending without --yes err = %v", err)
	}
}

func TestUpdateVerifyFailsUntilPluginVerified(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	if err := markPluginRestartRequired(installRoot, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	err := runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath})
	if err == nil || !strings.Contains(err.Error(), "update verification failed") {
		t.Fatalf("restart-required verify err = %v", err)
	}
	if err := markPluginVerified(installRoot, toolVersion+"+codex.test", filepath.Join(installRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath}); err != nil {
		t.Fatalf("verified update should pass: %v", err)
	}
}

func TestUpdateVerifyFailsOnPluginDrift(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	if err := markPluginDrift(installRoot, toolVersion+"+codex.test", filepath.Join(t.TempDir(), "plugins", "slidex", "skills", "slidex-start", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	findings := updateVerificationFindings(status)
	if !findingCheckPresent(findings, "update.plugin_drift") {
		t.Fatalf("drift finding missing: %#v", findings)
	}
	err = runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath})
	if err == nil || !strings.Contains(err.Error(), "update verification failed") {
		t.Fatalf("drift verify err = %v", err)
	}
}

func TestUpdateVerifyJSONReportsRestartRequiredContract(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	if err := markPluginRestartRequired(installRoot, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath, "--json"})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "update verification failed") {
		t.Fatalf("restart-required verify err = %v", runErr)
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("invalid update verify JSON: %v\n%s", err, output)
	}
	if status.Channel != updateChannelProduction || status.CurrentVersion != toolVersion || status.TargetVersion != "0.2.0" || status.TargetTag != "v0.2.0" {
		t.Fatalf("version/channel fields missing from verify JSON: %#v", status)
	}
	if status.Status != "verification-failed" || !status.RestartRequired || status.PluginVerificationStatus != "restart_required" {
		t.Fatalf("restart/plugin fields missing from verify JSON: %#v", status)
	}
	if status.NextVerificationCommand != "slidex codex app-server plugin-smoke --json" {
		t.Fatalf("next verification command = %q", status.NextVerificationCommand)
	}
	for _, check := range []string{"update.restart_required", "update.plugin_not_verified"} {
		if !findingCheckPresent(status.VerificationFindings, check) {
			t.Fatalf("verify JSON missing finding %q: %#v", check, status.VerificationFindings)
		}
	}
}

func TestUpdateCheckHumanAndJSONReportAvailableRelease(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	contract, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[
		  {"tag_name":"v0.2.0-abcdef0","draft":false,"prerelease":true,"published_at":"2026-02-02T00:00:00Z","assets":[]},
		  {"tag_name":"v0.2.0","draft":false,"prerelease":false,"published_at":"2026-02-01T00:00:00Z","assets":[
		    {"name":%q,"browser_download_url":"https://example.invalid/archive","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		    {"name":%q,"browser_download_url":"https://example.invalid/checksums"}
		  ]}
		]`, contract.ArchiveName, contract.ChecksumName)
	}))
	defer server.Close()

	var runErr error
	jsonOutput := captureStdoutForTest(t, func() {
		runErr = runUpdateCheck([]string{"--install-root", installRoot, "--metadata", metadataPath, "--api-url", server.URL, "--json"})
	})
	if runErr != nil {
		t.Fatalf("update check JSON failed: %v", runErr)
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(jsonOutput), &status); err != nil {
		t.Fatalf("invalid update check JSON: %v\n%s", err, jsonOutput)
	}
	if status.Status != "available" || status.Channel != updateChannelProduction || status.TargetVersion != "0.2.0" || status.TargetTag != "v0.2.0" {
		t.Fatalf("available release fields missing from update check JSON: %#v", status)
	}
	if status.ReleaseAssetName != contract.ArchiveName || status.ChecksumAssetName != contract.ChecksumName {
		t.Fatalf("asset fields missing from update check JSON: %#v", status)
	}
	if status.PluginVerificationStatus != "not_verified" || status.RestartRequired || status.NextVerificationCommand != "slidex update verify --json" {
		t.Fatalf("plugin/restart fields missing from update check JSON: %#v", status)
	}

	humanOutput := captureStdoutForTest(t, func() {
		runErr = runUpdateCheck([]string{"--install-root", installRoot, "--metadata", metadataPath, "--api-url", server.URL})
	})
	if runErr != nil {
		t.Fatalf("update check human failed: %v", runErr)
	}
	for _, want := range []string{
		"slidex update available",
		"channel: production",
		"current version: " + toolVersion,
		"target version: 0.2.0 (v0.2.0)",
		"release asset: " + contract.ArchiveName,
		"plugin status: not_verified",
		"next verification: slidex update verify --json",
	} {
		if !strings.Contains(humanOutput, want) {
			t.Fatalf("human update check missing %q:\n%s", want, humanOutput)
		}
	}
}

func TestUpdateCheckLocalDevelopmentDoesNotFetchReleaseAPI(t *testing.T) {
	installRoot := t.TempDir()
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateCheck([]string{"--install-root", installRoot, "--metadata", filepath.Join(installRoot, ".slidex", "missing.json"), "--api-url", server.URL, "--json"})
	})
	if runErr != nil {
		t.Fatalf("local-development update check failed: %v", runErr)
	}
	if called {
		t.Fatal("local-development update check should not fetch release metadata")
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("invalid local-development update check JSON: %v\n%s", err, output)
	}
	if status.Status != "disabled" || status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled {
		t.Fatalf("local-development status missing from update check JSON: %#v", status)
	}
	if !strings.Contains(status.Guidance, "disabled") {
		t.Fatalf("local-development guidance missing from update check JSON: %#v", status)
	}
}

func TestUpdateStatusHumanAndJSONReportPendingActivation(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, installMetadata{
		SchemaVersion: installMetadataSchemaVersion,
		ToolName:      toolName,
		Version:       toolVersion,
		Channel:       updateChannelProduction,
		InstallMode:   installModeReleasePackage,
	})
	candidate := filepath.Join(t.TempDir(), "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	_, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}

	var runErr error
	jsonOutput := captureStdoutForTest(t, func() {
		runErr = runUpdateStatus([]string{"--install-root", installRoot, "--metadata", metadataPath, "--json"})
	})
	if runErr != nil {
		t.Fatalf("update status JSON failed: %v", runErr)
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(jsonOutput), &status); err != nil {
		t.Fatalf("invalid update status JSON: %v\n%s", err, jsonOutput)
	}
	if status.Status != "pending-activation" || !status.PendingActivation || status.PendingActivationCommand == "" {
		t.Fatalf("pending activation missing from status JSON: %#v", status)
	}
	if !status.RestartRequired || status.PluginVerificationStatus != "restart_required" || status.NextVerificationCommand != "slidex codex app-server plugin-smoke --json" {
		t.Fatalf("restart verification missing from status JSON: %#v", status)
	}

	humanOutput := captureStdoutForTest(t, func() {
		runErr = runUpdateStatus([]string{"--install-root", installRoot, "--metadata", metadataPath})
	})
	if runErr != nil {
		t.Fatalf("update status human failed: %v", runErr)
	}
	for _, want := range []string{
		"slidex update pending-activation",
		"channel: production",
		"current version: " + toolVersion,
		"target version: 0.2.0 (v0.2.0)",
		"plugin status: restart_required",
		"restart required: restart Codex and start a new thread",
		"pending activation: " + status.PendingActivationCommand,
		"next verification: slidex codex app-server plugin-smoke --json",
	} {
		if !strings.Contains(humanOutput, want) {
			t.Fatalf("human update status missing %q:\n%s", want, humanOutput)
		}
	}
}

func allowUnverifiedAttestationForTest() attestationVerification {
	return attestationVerification{Policy: attestationPolicyAllowUnverified, Status: "skipped"}
}

func executableNameForTest(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func writeFakeGitHubCLIForTest(t *testing.T, path string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	code := `package main

import (
	"os"
	"strings"
)

func main() {
	logPath := os.Getenv("GH_LOG")
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			panic(err)
		}
		_, _ = f.WriteString(strings.Join(os.Args[1:], " ") + "\n")
		_ = f.Close()
	}
}
`
	if err := os.WriteFile(source, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", path, source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fake gh build failed: %v\n%s", err, out)
	}
}

func captureStdoutForTest(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = oldStdout
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
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
	if metadata.Version != toolVersion {
		t.Fatalf("metadata version = %q", metadata.Version)
	}
	if metadata.Commit != "0123456789abcdef" {
		t.Fatalf("metadata commit = %q", metadata.Commit)
	}
	if metadata.BuildTime != "2026-06-10T00:00:00Z" {
		t.Fatalf("metadata buildTime = %q", metadata.BuildTime)
	}
	if metadata.InstallMode != installModeReleasePackage {
		t.Fatalf("metadata installMode = %q", metadata.InstallMode)
	}
	if metadata.OS != "linux" || metadata.Arch != "amd64" {
		t.Fatalf("metadata os/arch = %s/%s", metadata.OS, metadata.Arch)
	}
	checksum := filepath.Join(dist, "slidex_"+toolVersion+"_checksums.txt")
	if _, err := os.Stat(checksum); err != nil {
		t.Fatalf("checksum name should use asset version without v: %v", err)
	}

	canaryVersion := toolVersion + "-abcdef0"
	canaryDist := t.TempDir()
	cmd = exec.Command("bash", filepath.Join(root, "scripts", "package-release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"SLIDEX_RELEASE_VERSION="+canaryVersion,
		"SLIDEX_BUILD_CHANNEL=canary",
		"SLIDEX_TARGETS=linux/amd64",
		"SLIDEX_DIST_DIR="+canaryDist,
		"SLIDEX_BUILD_TIME=2026-06-10T01:00:00Z",
		"SLIDEX_COMMIT_SHA=abcdef0123456789",
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("canary package release failed: %v\n%s", err, out)
	}
	canaryArchive := filepath.Join(canaryDist, "slidex_"+canaryVersion+"_linux_amd64.tar.gz")
	canaryMetadata := readInstallMetadataFromTarGzForTest(t, canaryArchive, "slidex_"+canaryVersion+"_linux_amd64/.slidex/install.json")
	if canaryMetadata.Channel != updateChannelCanary {
		t.Fatalf("canary metadata channel = %q", canaryMetadata.Channel)
	}
	if canaryMetadata.Tag != "v"+canaryVersion {
		t.Fatalf("canary metadata tag = %q", canaryMetadata.Tag)
	}
	if canaryMetadata.Version != canaryVersion {
		t.Fatalf("canary metadata version = %q", canaryMetadata.Version)
	}
	if canaryMetadata.ReleaseAssetName != "slidex_"+canaryVersion+"_linux_amd64.tar.gz" {
		t.Fatalf("canary metadata asset = %q", canaryMetadata.ReleaseAssetName)
	}
	if canaryMetadata.Commit != "abcdef0123456789" || canaryMetadata.BuildTime != "2026-06-10T01:00:00Z" {
		t.Fatalf("canary metadata build identity = %q / %q", canaryMetadata.Commit, canaryMetadata.BuildTime)
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
	baseVersion := releaseBaseVersion(version)
	channel := channelFromPackageVersion(version)
	if channel == updateChannelLocalDevelopment {
		channel = updateChannelProduction
	}
	contract, err := releaseAssetContractFor("v"+version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	dirs := []string{
		".agents/skills/slidex",
		".slidex",
		"decks/_template",
		"examples",
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
		".mise.toml":              "go = \"1.26.3\"\n",
		"CODEX_INSTALL_PROMPT.md": "Install slidex.\n",
		"INSTALL.md":              "Install slidex.\n",
		"LICENSE":                 "MIT\n",
		"README.ko.md":            "# slidex\n",
		"README.md":               "# slidex\n",
		"VERSIONING.md":           "# slidex Version Management\n",
		"VERSION":                 baseVersion,
		".slidex/install.json": `{
		  "schemaVersion":"slidex.install.v1",
		  "toolName":"slidex",
		  "version":"` + version + `",
		  "channel":"` + channel + `",
		  "tag":"v` + version + `",
		  "commit":"0123456789abcdef",
		  "buildTime":"2026-06-10T00:00:00Z",
		  "releaseAssetName":"` + contract.ArchiveName + `",
		  "os":"` + runtime.GOOS + `",
		  "arch":"` + runtime.GOARCH + `",
		  "installMode":"release-package"
		}`,
		"commands.md":                    "# commands\n",
		"decks/README.md":                "# decks\n",
		"examples/sample_deck_spec.json": "{}\n",
		"go.mod":                         "module slidex\n\ngo 1.26.3\n",
		"go.sum":                         "",
		"slidex.toml":                    "[app_server_api]\ndefault = \"deny\"\n",
		"plugins/slidex/.codex-plugin/plugin.json": `{
		  "name":"slidex",
		  "version":"` + baseVersion + `+codex.test",
		  "author":{"name":"shiinamachi"},
		  "license":"MIT",
		  "skills":"./skills/",
		  "mcpServers":"./.mcp.json"
		}`,
		"plugins/slidex/.codex-plugin/version-lock.json": `{
		  "pluginVersion":"` + baseVersion + `",
		  "slidexCliVersion":"` + baseVersion + `",
		  "requiredCodexCliVersion":"0.138.0"
		}`,
		".agents/plugins/marketplace.json": `{
		  "plugins":[{"name":"slidex","source":{"source":"local","path":"./plugins/slidex"}}]
		}`,
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeCandidateBinaryForTest(t, filepath.Join(root, binary), baseVersion)
}

func writeCandidateBinaryForTest(t *testing.T, path, version string) {
	t.Helper()
	writeCandidateBinaryForTestWithDoctorStatus(t, path, version, "pass")
}

func writeCandidateBinaryForTestWithDoctorStatus(t *testing.T, path, version, doctorStatus string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	code := `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("slidex ` + version + `")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		fmt.Println(` + "`" + `{"status":"` + doctorStatus + `"}` + "`" + `)
		return
	}
	fmt.Println("slidex ` + version + `")
}
`
	if err := os.WriteFile(source, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", path, source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("candidate test binary build failed: %v\n%s", err, out)
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

func writeTarGzFromDirForTest(t *testing.T, archivePath, root, topName string) {
	t.Helper()
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := topName
		if rel != "." {
			name = filepath.ToSlash(filepath.Join(topName, rel))
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = tw.Write(raw)
		return err
	})
	closeErr := tw.Close()
	gzErr := gz.Close()
	fileErr := f.Close()
	for _, err := range []error{walkErr, closeErr, gzErr, fileErr} {
		if err != nil {
			t.Fatal(err)
		}
	}
}
