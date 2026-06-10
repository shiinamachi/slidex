package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestVerifyReleaseAttestationCanBeExplicitlyBypassed(t *testing.T) {
	result, err := verifyReleaseAttestation("/tmp/slidex_0.2.0_linux_amd64.tar.gz", "v0.2.0", attestationPolicyAllowUnverified)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "skipped" || result.Policy != attestationPolicyAllowUnverified {
		t.Fatalf("unexpected attestation result: %#v", result)
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

func allowUnverifiedAttestationForTest() attestationVerification {
	return attestationVerification{Policy: attestationPolicyAllowUnverified, Status: "skipped"}
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
		".slidex",
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
		".slidex/install.json": `{
		  "schemaVersion":"slidex.install.v1",
		  "toolName":"slidex",
		  "version":"` + version + `",
		  "channel":"production",
		  "tag":"v` + version + `",
		  "installMode":"release-package"
		}`,
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
	writeCandidateBinaryForTest(t, filepath.Join(root, binary), version)
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
