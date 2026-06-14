package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	"strconv"
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
	win, err := releaseAssetContractFor("v0.1.0-canary.20260610090000", "windows", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if win.ArchiveName != "slidex_0.1.0-canary.20260610090000_windows_arm64.zip" {
		t.Fatalf("windows archive name = %q", win.ArchiveName)
	}
}

func TestChannelFromPackageVersionOnlyAcceptsStableAndCanary(t *testing.T) {
	huge := strings.Repeat("9", 128)
	tests := []struct {
		version string
		want    string
	}{
		{"0.1.0", updateChannelProduction},
		{"v0.1.0", updateChannelProduction},
		{"0.1.0-canary.20260610090000", updateChannelCanary},
		{"0.1.0-e9c033e", updateChannelLocalDevelopment},
		{"0.1.0-beta.1", updateChannelLocalDevelopment},
		{"dev-local", updateChannelLocalDevelopment},
		{huge + ".0.0", updateChannelLocalDevelopment},
		{huge + ".0.0-canary.20260610090000", updateChannelLocalDevelopment},
	}
	for _, tc := range tests {
		if got := channelFromPackageVersion(tc.version); got != tc.want {
			t.Fatalf("channelFromPackageVersion(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}

func TestUpdateStatusDetectsImmutableChannelsAndLocalDevelopment(t *testing.T) {
	temp := t.TempDir()
	productionRoot := filepath.Join(temp, "prod-root")
	productionMeta := installMetadataPath(productionRoot)
	writeInstallMetadataForTest(t, productionMeta, releaseInstallMetadataForTest(t, toolVersion))
	status, err := currentUpdateStatus(productionRoot, productionMeta)
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelProduction || !status.UpdatesEnabled {
		t.Fatalf("production status = %#v", status)
	}

	canaryRoot := filepath.Join(temp, "canary-root")
	canaryMeta := installMetadataPath(canaryRoot)
	writeInstallMetadataForTest(t, canaryMeta, releaseInstallMetadataForTest(t, toolVersion+"-canary.20260610010000"))
	status, err = currentUpdateStatus(canaryRoot, canaryMeta)
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelCanary || !status.UpdatesEnabled {
		t.Fatalf("canary status = %#v", status)
	}
	if status.CurrentVersion != toolVersion+"-canary.20260610010000" {
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

func TestUpdateStatusDisablesWhenMetadataRootIsStale(t *testing.T) {
	temp := t.TempDir()
	installRoot := filepath.Join(temp, "active")
	staleRoot := filepath.Join(temp, "old")
	metadataPath := installMetadataPath(installRoot)
	metadata := releaseInstallMetadataForTest(t, toolVersion)
	metadata.InstallRoot = filepath.ToSlash(staleRoot)
	writeInstallMetadataForTest(t, metadataPath, metadata)

	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.InstallRoot != filepath.ToSlash(installRoot) {
		t.Fatalf("status should use resolved install root, got %#v", status)
	}
	if status.InstalledMetadata == nil || status.InstalledMetadata.InstallRoot != filepath.ToSlash(staleRoot) {
		t.Fatalf("installed metadata should remain visible for drift evidence: %#v", status.InstalledMetadata)
	}
	if status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled || status.Status != "disabled" {
		t.Fatalf("stale metadata root should disable updates: %#v", status)
	}
	if !strings.Contains(status.Reason, "update is disabled fail-closed") || !strings.Contains(status.Reason, "different install root") {
		t.Fatalf("stale metadata root reason missing: %#v", status)
	}
}

func TestUpdateStatusDisablesReleaseMetadataOutsideInstallRoot(t *testing.T) {
	temp := t.TempDir()
	installRoot := filepath.Join(temp, "slidex")
	metadataPath := filepath.Join(temp, "external-install.json")
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))

	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled || status.Status != "disabled" {
		t.Fatalf("out-of-tree release metadata should disable updates: %#v", status)
	}
	if !strings.Contains(status.Reason, "metadata path must be") || !strings.Contains(status.Reason, "update is disabled fail-closed") {
		t.Fatalf("out-of-tree metadata reason missing: %#v", status)
	}
}

func TestUpdateStatusDisablesInconsistentReleaseMetadata(t *testing.T) {
	temp := t.TempDir()
	metadataPath := filepath.Join(temp, "install.json")
	metadata := releaseInstallMetadataForTest(t, toolVersion+"-canary.20260610010000")
	metadata.Channel = updateChannelProduction
	writeInstallMetadataForTest(t, metadataPath, metadata)

	status, err := currentUpdateStatus(filepath.Join(temp, "slidex"), metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled || status.Status != "disabled" {
		t.Fatalf("inconsistent release metadata should disable updates: %#v", status)
	}
	if !strings.Contains(status.Reason, "update is disabled fail-closed") || !strings.Contains(status.Reason, "channel must match") {
		t.Fatalf("inconsistent release metadata reason missing: %#v", status)
	}
}

func TestUpdateCheckInconsistentReleaseMetadataDoesNotFetchReleaseAPI(t *testing.T) {
	temp := t.TempDir()
	metadataPath := filepath.Join(temp, "install.json")
	metadata := releaseInstallMetadataForTest(t, toolVersion)
	metadata.Tag = "v9.9.9"
	writeInstallMetadataForTest(t, metadataPath, metadata)
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateCheck([]string{"--install-root", filepath.Join(temp, "slidex"), "--metadata", metadataPath, "--api-url", server.URL, "--json"})
	})
	if runErr != nil {
		t.Fatalf("inconsistent release metadata check failed: %v\n%s", runErr, output)
	}
	if called {
		t.Fatal("inconsistent release metadata should not fetch release metadata")
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("invalid inconsistent metadata check JSON: %v\n%s", err, output)
	}
	if status.Status != "disabled" || status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled {
		t.Fatalf("inconsistent release metadata check should be disabled: %#v", status)
	}
	if !strings.Contains(status.Reason, "tag must resolve") {
		t.Fatalf("inconsistent release metadata reason missing: %#v", status)
	}
}

func TestUpdateStatusDisablesIncompleteReleaseMetadata(t *testing.T) {
	temp := t.TempDir()
	tests := []struct {
		name        string
		mutate      func(*installMetadata)
		wantReason  string
		wantCurrent string
	}{
		{
			name:        "missing schema version",
			mutate:      func(metadata *installMetadata) { metadata.SchemaVersion = "" },
			wantReason:  "schemaVersion must be",
			wantCurrent: toolVersion,
		},
		{
			name:        "missing tool name",
			mutate:      func(metadata *installMetadata) { metadata.ToolName = "" },
			wantReason:  "toolName must be slidex",
			wantCurrent: toolVersion,
		},
		{
			name:       "missing version",
			mutate:     func(metadata *installMetadata) { metadata.Version = "" },
			wantReason: "version is required",
		},
		{
			name:        "missing tag",
			mutate:      func(metadata *installMetadata) { metadata.Tag = "" },
			wantReason:  "tag is required",
			wantCurrent: toolVersion,
		},
		{
			name:        "missing commit",
			mutate:      func(metadata *installMetadata) { metadata.Commit = "" },
			wantReason:  "commit is required",
			wantCurrent: toolVersion,
		},
		{
			name:        "invalid commit",
			mutate:      func(metadata *installMetadata) { metadata.Commit = "not-a-sha" },
			wantReason:  "commit must be a 7-40 character lowercase git SHA",
			wantCurrent: toolVersion,
		},
		{
			name:        "missing build time",
			mutate:      func(metadata *installMetadata) { metadata.BuildTime = "" },
			wantReason:  "buildTime is required",
			wantCurrent: toolVersion,
		},
		{
			name:        "invalid build time",
			mutate:      func(metadata *installMetadata) { metadata.BuildTime = "soon" },
			wantReason:  "buildTime must be RFC3339",
			wantCurrent: toolVersion,
		},
		{
			name:        "missing release asset",
			mutate:      func(metadata *installMetadata) { metadata.ReleaseAssetName = "" },
			wantReason:  "releaseAssetName is required",
			wantCurrent: toolVersion,
		},
		{
			name:        "missing install mode",
			mutate:      func(metadata *installMetadata) { metadata.InstallMode = "" },
			wantReason:  "installMode must be release-package",
			wantCurrent: toolVersion,
		},
		{
			name:        "missing os",
			mutate:      func(metadata *installMetadata) { metadata.OS = "" },
			wantReason:  "os must be " + runtime.GOOS,
			wantCurrent: toolVersion,
		},
		{
			name:        "missing arch",
			mutate:      func(metadata *installMetadata) { metadata.Arch = "" },
			wantReason:  "arch must be " + runtime.GOARCH,
			wantCurrent: toolVersion,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metadata := releaseInstallMetadataForTest(t, toolVersion)
			tc.mutate(&metadata)
			metadataPath := filepath.Join(temp, strings.ReplaceAll(tc.name, " ", "-")+".json")
			writeInstallMetadataForTest(t, metadataPath, metadata)

			status, err := currentUpdateStatus(filepath.Join(temp, "slidex-"+tc.name), metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			if status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled || status.Status != "disabled" {
				t.Fatalf("incomplete release metadata should disable updates: %#v", status)
			}
			if !strings.Contains(status.Reason, "update is disabled fail-closed") || !strings.Contains(status.Reason, tc.wantReason) {
				t.Fatalf("incomplete release metadata reason missing %q: %#v", tc.wantReason, status)
			}
			if tc.wantCurrent != "" && status.CurrentVersion != tc.wantCurrent {
				t.Fatalf("current version = %q, want %q", status.CurrentVersion, tc.wantCurrent)
			}
		})
	}
}

func TestUpdateStatusDisablesSchemaInvalidReleaseMetadata(t *testing.T) {
	temp := t.TempDir()
	metadataPath := filepath.Join(temp, "install.json")
	metadata := releaseInstallMetadataForTest(t, toolVersion)
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpectedField"] = true
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := currentUpdateStatus(filepath.Join(temp, "slidex"), metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled || status.Status != "disabled" {
		t.Fatalf("schema-invalid release metadata should disable updates: %#v", status)
	}
	if !strings.Contains(status.Reason, "schema validation failed") {
		t.Fatalf("schema-invalid metadata reason missing: %#v", status)
	}
}

func TestUpdateCheckIncompleteReleaseMetadataDoesNotFetchReleaseAPI(t *testing.T) {
	temp := t.TempDir()
	metadataPath := filepath.Join(temp, "install.json")
	metadata := releaseInstallMetadataForTest(t, toolVersion)
	metadata.Commit = ""
	writeInstallMetadataForTest(t, metadataPath, metadata)
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateCheck([]string{"--install-root", filepath.Join(temp, "slidex"), "--metadata", metadataPath, "--api-url", server.URL, "--json"})
	})
	if runErr != nil {
		t.Fatalf("incomplete release metadata check failed: %v\n%s", runErr, output)
	}
	if called {
		t.Fatal("incomplete release metadata should not fetch release metadata")
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("invalid incomplete metadata check JSON: %v\n%s", err, output)
	}
	if status.Status != "disabled" || status.Channel != updateChannelLocalDevelopment || status.UpdatesEnabled {
		t.Fatalf("incomplete release metadata check should be disabled: %#v", status)
	}
	if !strings.Contains(status.Reason, "commit is required") {
		t.Fatalf("incomplete release metadata reason missing: %#v", status)
	}
}

func TestUpdateDiscoveryHonorsProductionAndCanaryChannels(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-canary.20260610010000","draft":false,"prerelease":true,"assets":[
	    {"name":"slidex_0.2.0-canary.20260610010000_linux_amd64.tar.gz","browser_download_url":"https://example.invalid/canary.tgz","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	    {"name":"slidex_0.2.0-canary.20260610010000_checksums.txt","browser_download_url":"https://example.invalid/canary.txt"}
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
	if canary.TagName != "v0.2.0-canary.20260610010000" {
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
	if archive.Name != "slidex_0.2.0-canary.20260610010000_linux_amd64.tar.gz" || checksum.Name != "slidex_0.2.0-canary.20260610010000_checksums.txt" {
		t.Fatalf("required assets = %q / %q", archive.Name, checksum.Name)
	}
}

func TestFetchUpdateReleasesHonorsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := fetchUpdateReleases(ctx, server.URL)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected release fetch context deadline, got %v", err)
	}
}

func TestDefaultUpdateAPIURLRequestsLargePage(t *testing.T) {
	t.Setenv(updateAPIURLEnv, "")
	got := defaultUpdateAPIURL()
	if !strings.Contains(got, "per_page=100") {
		t.Fatalf("default update API URL should request a larger page, got %s", got)
	}
}

func TestFetchUpdateReleasesFollowsPagination(t *testing.T) {
	var requests []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		if r.URL.Query().Get("page") == "2" {
			_, _ = io.WriteString(w, `[
			  {"tag_name":"v0.3.0","draft":false,"prerelease":false,"published_at":"2026-03-01T00:00:00Z","assets":[]}
			]`)
			return
		}
		w.Header().Set("Link", "<"+server.URL+"/releases?per_page=100&page=2>; rel=\"next\"")
		_, _ = io.WriteString(w, `[
		  {"tag_name":"v0.4.0-canary.20260610010000","draft":false,"prerelease":true,"published_at":"2026-04-01T00:00:00Z","assets":[]}
		]`)
	}))
	defer server.Close()

	releases, err := fetchUpdateReleases(context.Background(), server.URL+"/releases?per_page=100")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected two paginated requests, got %d: %#v", len(requests), requests)
	}
	release, err := selectUpdateReleaseForCurrent(updateChannelProduction, "0.2.0", releases)
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v0.3.0" {
		t.Fatalf("production release = %s, want v0.3.0", release.TagName)
	}
}

func TestFetchUpdateReleaseForStatusSearchesPastBusyCanaryPages(t *testing.T) {
	var requests []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		page, _ := strconv.Atoi(firstNonEmpty(r.URL.Query().Get("page"), "1"))
		if page >= 7 {
			_, _ = io.WriteString(w, `[
			  {"tag_name":"v0.3.0","draft":false,"prerelease":false,"published_at":"2026-03-01T00:00:00Z","assets":[]}
			]`)
			return
		}
		w.Header().Set("Link", "<"+server.URL+"/releases?per_page=100&page="+strconv.Itoa(page+1)+">; rel=\"next\"")
		_, _ = io.WriteString(w, `[
		  {"tag_name":"v0.4.0-canary.20260610010000","draft":false,"prerelease":true,"published_at":"2026-04-01T00:00:00Z","assets":[]}
		]`)
	}))
	defer server.Close()

	release, err := fetchUpdateReleaseForStatus(context.Background(), server.URL+"/releases?per_page=100", updateStatus{
		Channel:        updateChannelProduction,
		CurrentVersion: "0.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v0.3.0" {
		t.Fatalf("production release = %s, want v0.3.0", release.TagName)
	}
	if len(requests) != 7 {
		t.Fatalf("expected selection-aware pagination through page 7, got %d requests: %#v", len(requests), requests)
	}
}

func TestDownloadUpdateAssetHonorsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := downloadUpdateAsset(ctx, updateAsset{Name: "slidex.zip", BrowserDownloadURL: server.URL}, 1024)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected asset download context deadline, got %v", err)
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

func TestCompareReleaseBaseVersionsRejectsHugeComponents(t *testing.T) {
	huge := strings.Repeat("9", 128) + ".0.0"
	if _, ok := compareReleaseBaseVersions(huge, "0.1.0"); ok {
		t.Fatal("huge release version component should not compare successfully")
	}
	if releaseIsNotOlder(huge, "0.1.0") {
		t.Fatal("overflowing release version should not be considered an update candidate")
	}
}

func TestReleaseVersionTrustBoundariesRejectHugeComponents(t *testing.T) {
	huge := strings.Repeat("9", 128) + ".0.0"
	hugeCanary := huge + "-canary.20260610090000"
	if _, err := canonicalUpdateTargetVersion(huge); err == nil {
		t.Fatal("canonical target version should reject huge release components")
	}
	if _, err := canonicalUpdateTargetVersion(hugeCanary); err == nil {
		t.Fatal("canonical canary target version should reject huge release components")
	}
	if _, err := releasePackageVersionFromTag("v" + huge); err == nil {
		t.Fatal("release tag parser should reject huge release components")
	}
	if _, err := releasePackageVersionFromTag("v" + hugeCanary); err == nil {
		t.Fatal("canary release tag parser should reject huge release components")
	}
	if _, err := canonicalUpdateTargetVersion("vv0.1.0"); err == nil {
		t.Fatal("canonical target version should reject double-v input")
	}
	if _, err := releasePackageVersionFromTag("vv0.1.0"); err == nil {
		t.Fatal("release tag parser should reject double-v input")
	}
}

func TestUpdateDiscoveryIgnoresOverflowingReleaseVersions(t *testing.T) {
	huge := strings.Repeat("9", 128)
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v` + huge + `.0.0","draft":false,"prerelease":false,"published_at":"2026-04-01T00:00:00Z","assets":[]},
	  {"tag_name":"v0.3.0","draft":false,"prerelease":false,"published_at":"2026-03-01T00:00:00Z","assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 {
		t.Fatalf("overflowing release should be ignored during parse, got %d releases", len(releases))
	}
	release, err := selectUpdateReleaseForCurrent(updateChannelProduction, "0.1.0", releases)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.3.0" {
		t.Fatalf("production selected %s", release.Version)
	}
}

func TestInstalledReleaseMetadataIssueRejectsOverflowingVersion(t *testing.T) {
	huge := strings.Repeat("9", 128) + ".0.0"
	metadata := releaseInstallMetadataForTest(t, toolVersion)
	metadata.Version = huge
	metadata.Channel = updateChannelProduction
	metadata.Tag = "v" + huge
	metadata.ReleaseAssetName = fmt.Sprintf("slidex_%s_%s_%s.tar.gz", huge, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		metadata.ReleaseAssetName = fmt.Sprintf("slidex_%s_%s_%s.zip", huge, runtime.GOOS, runtime.GOARCH)
	}
	if issue := installedReleaseMetadataIssue(&metadata); issue == "" || !strings.Contains(issue, "version") {
		t.Fatalf("expected overflowing install metadata version issue, got %q", issue)
	}
}

func TestInstallMetadataSchemaRejectsOverflowingVersion(t *testing.T) {
	huge := strings.Repeat("9", 128) + ".0.0"
	metadata := releaseInstallMetadataForTest(t, toolVersion)
	metadata.Version = huge
	metadata.Tag = "v" + huge
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRawJSONAgainstBundledSchema(raw, installMetadataSchemaFile); err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("install metadata schema should reject overflowing version, got %v", err)
	}
}

func TestUpdateDiscoveryOrdersSameBaseCanaryByTimestamp(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-canary.20260610010000","draft":false,"prerelease":true,"published_at":"2026-02-01T00:00:00Z","assets":[]},
	  {"tag_name":"v0.2.0-canary.20260610020000","draft":false,"prerelease":true,"published_at":"2026-02-02T00:00:00Z","assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	next, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.2.0-canary.20260610010000", releases)
	if err != nil {
		t.Fatal(err)
	}
	if next.Version != "0.2.0-canary.20260610020000" {
		t.Fatalf("next canary = %s", next.Version)
	}
	next, err = selectUpdateReleaseForCurrent(updateChannelCanary, "0.2.0-canary.20260610015000", releases)
	if err != nil {
		t.Fatal(err)
	}
	if next.Version != "0.2.0-canary.20260610020000" {
		t.Fatalf("inferred next canary = %s", next.Version)
	}
}

func TestUpdateDiscoverySelectsNewestCanaryReleaseWithoutAPISorting(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-canary.20260610010000","draft":false,"prerelease":true,"published_at":"2026-02-01T00:00:00Z","assets":[]},
	  {"tag_name":"v0.2.0-canary.20260610020000","draft":false,"prerelease":true,"published_at":"2026-02-02T00:00:00Z","assets":[]},
	  {"tag_name":"v0.1.0-canary.20260609010000","draft":false,"prerelease":true,"published_at":"2026-01-01T00:00:00Z","assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	release, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.1.0-canary.20260609010000", releases)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.2.0-canary.20260610020000" {
		t.Fatalf("canary selected %s", release.Version)
	}
}

func TestUpdateDiscoveryOrdersTimestampCanaryWithoutReleaseMetadata(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-canary.20260610010000","draft":false,"prerelease":true,"assets":[]},
	  {"tag_name":"v0.2.0-canary.20260610020000","draft":false,"prerelease":true,"assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	release, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.2.0-canary.20260610010000", releases)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.2.0-canary.20260610020000" {
		t.Fatalf("canary selected %s", release.Version)
	}
}

func TestUpdateDiscoveryRequiresCanaryPrereleaseFlag(t *testing.T) {
	releases, err := parseUpdateReleases([]byte(`[
	  {"tag_name":"v0.2.0-canary.20260610010000","draft":false,"prerelease":false,"assets":[]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selectUpdateReleaseForCurrent(updateChannelCanary, "0.1.0-canary.20260609010000", releases); err == nil || !strings.Contains(err.Error(), "no matching canary release") {
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
	if got, err := verifyReleaseAssetSHA256("slidex_0.1.0_linux_amd64.tar.gz", payload, actual+"  slidex_0.1.0_linux_amd64.tar.gz\n", ""); err != nil || got != actual {
		t.Fatalf("checksum-only verification should pass for local archives, got %q err=%v", got, err)
	}
	if got, err := verifyReleaseAssetSHA256("slidex_0.1.0_linux_amd64.tar.gz", payload, actual+"  slidex_0.1.0_linux_amd64.tar.gz\n", "sha256:"+actual); err != nil || got != actual {
		t.Fatalf("verified digest = %q, err = %v", got, err)
	}
}

func TestStageDownloadedReleaseArchiveRequiresGitHubDigest(t *testing.T) {
	payload := []byte("release archive")
	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	archive := updateAsset{Name: "slidex_0.1.0_linux_amd64.tar.gz"}
	checksum := updateAsset{Name: "slidex_0.1.0_checksums.txt"}
	checksumPayload := []byte(actual + "  " + archive.Name + "\n")

	_, _, err := stageDownloadedReleaseArchive(t.TempDir(), "0.1.0", archive, payload, checksum, checksumPayload)
	if err == nil || !strings.Contains(err.Error(), "GitHub release asset digest is required") {
		t.Fatalf("downloaded archive without GitHub digest err = %v", err)
	}

	archive.Digest = "sha256:" + actual
	stageParent, archivePath, err := stageDownloadedReleaseArchive(t.TempDir(), "0.1.0", archive, payload, checksum, checksumPayload)
	if err != nil {
		t.Fatalf("downloaded archive with GitHub digest failed: %v", err)
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("staged archive missing: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(stageParent), ".slidex/downloads/0.1.0-") {
		t.Fatalf("unexpected stage parent: %s", stageParent)
	}
}

func TestStageDownloadedReleaseArchiveRejectsSymlinkDownloadRoot(t *testing.T) {
	payload := []byte("release archive")
	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	archive := updateAsset{Name: "slidex_0.1.0_linux_amd64.tar.gz", Digest: "sha256:" + actual}
	checksum := updateAsset{Name: "slidex_0.1.0_checksums.txt"}
	checksumPayload := []byte(actual + "  " + archive.Name + "\n")
	installRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(installRoot, ".slidex")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, _, err := stageDownloadedReleaseArchive(installRoot, "0.1.0", archive, payload, checksum, checksumPayload)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlink download root rejection, got %v", err)
	}
}

func TestWriteStreamFileReplacesHardlinkedTarget(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hardlinkOrSkip(t, outside, target)

	if err := writeStreamFile(target, strings.NewReader("target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFileOrEmpty(outside); got != "outside\n" {
		t.Fatalf("outside hardlinked file was modified: %q", got)
	}
	if got := readFileOrEmpty(target); got != "target\n" {
		t.Fatalf("stream target = %q, want new contents", got)
	}
}

func TestExtractZipArchiveRejectsSymlinkDestinationDirectory(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("candidate/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("payload\n")); err != nil {
		t.Fatal(err)
	}
	closeErr := zw.Close()
	fileErr := f.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if fileErr != nil {
		t.Fatal(fileErr)
	}
	extractRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(extractRoot, "candidate")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	err = extractZipArchive(archivePath, extractRoot)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlink destination rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive extraction wrote through symlink, stat err=%v", err)
	}
}

func TestExtractZipArchiveRejectsExtractionBudgetBeforeWriting(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, entry := range []string{"candidate/one.txt", "candidate/two.txt"} {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("1234")); err != nil {
			t.Fatal(err)
		}
	}
	closeErr := zw.Close()
	fileErr := f.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if fileErr != nil {
		t.Fatal(fileErr)
	}

	extractRoot := t.TempDir()
	err = extractZipArchiveWithBudget(archivePath, extractRoot, &updateArchiveExtractionBudget{
		maxEntries:  10,
		maxFileSize: 10,
		maxTotal:    5,
	})
	if err == nil || !strings.Contains(err.Error(), "maximum expanded size") {
		t.Fatalf("expected expanded-size budget rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(extractRoot, "candidate", "one.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("zip preflight should reject before writing files, stat err=%v", err)
	}

	entryLimitRoot := t.TempDir()
	err = extractZipArchiveWithBudget(archivePath, entryLimitRoot, &updateArchiveExtractionBudget{
		maxEntries:  1,
		maxFileSize: 10,
		maxTotal:    100,
	})
	if err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("expected entry-count budget rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(entryLimitRoot, "candidate", "one.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("zip entry-count preflight should reject before writing files, stat err=%v", err)
	}
}

func TestExtractZipArchiveRejectsEOCDEntryBudgetBeforeOpen(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	eocd := make([]byte, zipEndOfCentralDirectoryMinSize)
	binary.LittleEndian.PutUint32(eocd[0:4], zipEndOfCentralDirectorySignature)
	binary.LittleEndian.PutUint16(eocd[8:10], 2)
	binary.LittleEndian.PutUint16(eocd[10:12], 2)
	if err := os.WriteFile(archivePath, eocd, 0o644); err != nil {
		t.Fatal(err)
	}

	err := extractZipArchiveWithBudget(archivePath, t.TempDir(), &updateArchiveExtractionBudget{
		maxEntries:  1,
		maxFileSize: 10,
		maxTotal:    10,
	})
	if err == nil || !strings.Contains(err.Error(), "too many entries before opening ZIP") {
		t.Fatalf("expected EOCD entry budget rejection before zip.OpenReader, got %v", err)
	}
}

func TestExtractZipArchiveRejectsCentralDirectoryBudgetBeforeOpen(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	centralDirectory := make([]byte, 11)
	eocd := make([]byte, zipEndOfCentralDirectoryMinSize)
	binary.LittleEndian.PutUint32(eocd[0:4], zipEndOfCentralDirectorySignature)
	binary.LittleEndian.PutUint16(eocd[8:10], 1)
	binary.LittleEndian.PutUint16(eocd[10:12], 1)
	binary.LittleEndian.PutUint32(eocd[12:16], uint32(len(centralDirectory)))
	if err := os.WriteFile(archivePath, append(centralDirectory, eocd...), 0o644); err != nil {
		t.Fatal(err)
	}

	err := extractZipArchiveWithBudget(archivePath, t.TempDir(), &updateArchiveExtractionBudget{
		maxEntries:          10,
		maxFileSize:         10,
		maxTotal:            10,
		maxCentralDirectory: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "central directory exceeds maximum size before opening ZIP") {
		t.Fatalf("expected central directory budget rejection before zip.OpenReader, got %v", err)
	}
}

func TestExtractZipArchiveRejectsUnderstatedCentralDirectorySizeBeforeOpen(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	writeCentralDirectoryOnlyZip(t, archivePath, 2, 1, 0)

	err := extractZipArchiveWithBudget(archivePath, t.TempDir(), &updateArchiveExtractionBudget{
		maxEntries:          10,
		maxFileSize:         10,
		maxTotal:            10,
		maxCentralDirectory: 1 << 20,
	})
	if err == nil || !strings.Contains(err.Error(), "central directory size does not match metadata before opening ZIP") {
		t.Fatalf("expected central directory size mismatch before zip.OpenReader, got %v", err)
	}
}

func TestExtractZipArchiveRejectsUnderstatedCentralDirectoryEntryCountBeforeOpen(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	actualDirectorySize := writeCentralDirectoryOnlyZip(t, archivePath, 2, 1, 0)
	rewriteZipEOCDCentralDirectorySize(t, archivePath, actualDirectorySize)

	err := extractZipArchiveWithBudget(archivePath, t.TempDir(), &updateArchiveExtractionBudget{
		maxEntries:          10,
		maxFileSize:         10,
		maxTotal:            10,
		maxCentralDirectory: 1 << 20,
	})
	if err == nil || !strings.Contains(err.Error(), "central directory entry count does not match metadata before opening ZIP") {
		t.Fatalf("expected central directory entry-count mismatch before zip.OpenReader, got %v", err)
	}
}

func TestValidateArchiveRelativePathRejectsPortableHazards(t *testing.T) {
	for _, name := range []string{
		"candidate/file.txt",
		"candidate/",
		"candidate/.agents/hook.sh",
		"candidate/.slidex/state.json",
	} {
		if err := validateArchiveRelativePath(name); err != nil {
			t.Fatalf("expected archive path %q to be accepted: %v", name, err)
		}
	}
	for _, name := range []string{
		"",
		"/candidate/file.txt",
		"C:/candidate/file.txt",
		`candidate\file.txt`,
		"candidate//file.txt",
		"candidate/./file.txt",
		"candidate/../file.txt",
		"candidate/NUL",
		"candidate/CON.txt",
		"candidate/slidex.exe:stream",
		"candidate/dir. /file.txt",
		"candidate/trailing-space /file.txt",
		"candidate/trailing-dot./file.txt",
	} {
		if err := validateArchiveRelativePath(name); err == nil {
			t.Fatalf("expected archive path %q to be rejected", name)
		}
	}
}

func TestExtractZipArchiveRejectsUnsafeArchiveEntryPath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("candidate/NUL")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	closeErr := zw.Close()
	fileErr := f.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if fileErr != nil {
		t.Fatal(fileErr)
	}

	err = extractZipArchive(archivePath, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "unsafe archive entry path") {
		t.Fatalf("expected unsafe ZIP entry path rejection, got %v", err)
	}
}

func TestExtractTarGzArchiveRejectsUnsafeArchiveEntryPath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	header := &tar.Header{Name: "candidate/slidex.exe:stream", Mode: 0o644, Size: 7}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	closeErr := tw.Close()
	gzErr := gz.Close()
	fileErr := f.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if gzErr != nil {
		t.Fatal(gzErr)
	}
	if fileErr != nil {
		t.Fatal(fileErr)
	}

	err = extractTarGzArchive(archivePath, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "unsafe archive entry path") {
		t.Fatalf("expected unsafe tar entry path rejection, got %v", err)
	}
}

func writeCentralDirectoryOnlyZip(t *testing.T, archivePath string, entries, declaredEntries int, declaredDirectorySize uint32) int {
	t.Helper()
	var archive []byte
	for i := 0; i < entries; i++ {
		name := []byte(fmt.Sprintf("f%05d", i))
		header := make([]byte, zipCentralDirectoryHeaderMinSize)
		binary.LittleEndian.PutUint32(header[0:4], zipCentralDirectoryHeaderSignature)
		binary.LittleEndian.PutUint16(header[28:30], uint16(len(name)))
		archive = append(archive, header...)
		archive = append(archive, name...)
	}
	actualDirectorySize := len(archive)
	eocd := make([]byte, zipEndOfCentralDirectoryMinSize)
	binary.LittleEndian.PutUint32(eocd[0:4], zipEndOfCentralDirectorySignature)
	binary.LittleEndian.PutUint16(eocd[8:10], uint16(declaredEntries))
	binary.LittleEndian.PutUint16(eocd[10:12], uint16(declaredEntries))
	binary.LittleEndian.PutUint32(eocd[12:16], declaredDirectorySize)
	if err := os.WriteFile(archivePath, append(archive, eocd...), 0o644); err != nil {
		t.Fatal(err)
	}
	return actualDirectorySize
}

func rewriteZipEOCDCentralDirectorySize(t *testing.T, archivePath string, declaredDirectorySize int) {
	t.Helper()
	f, err := os.OpenFile(archivePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Seek(-zipEndOfCentralDirectoryMinSize+12, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(declaredDirectorySize))
	if _, err := f.Write(buf[:]); err != nil {
		t.Fatal(err)
	}
}

func TestExtractTarGzArchiveRejectsExtractionBudget(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "candidate.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	header := &tar.Header{Name: "candidate/file.txt", Mode: 0o644, Size: 6}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("123456")); err != nil {
		t.Fatal(err)
	}
	closeErr := tw.Close()
	gzErr := gz.Close()
	fileErr := f.Close()
	for _, err := range []error{closeErr, gzErr, fileErr} {
		if err != nil {
			t.Fatal(err)
		}
	}

	extractRoot := t.TempDir()
	err = extractTarGzArchiveWithBudget(archivePath, extractRoot, &updateArchiveExtractionBudget{
		maxEntries:  10,
		maxFileSize: 5,
		maxTotal:    10,
	})
	if err == nil || !strings.Contains(err.Error(), "maximum uncompressed size") {
		t.Fatalf("expected file-size budget rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(extractRoot, "candidate", "file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tar budget should reject before writing file, stat err=%v", err)
	}
}

func TestReadRegularFileWithMaxBytesRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.zip")
	if err := os.WriteFile(path, []byte("123456"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readRegularFileWithMaxBytes(path, 5)
	if err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected read size cap rejection, got %v", err)
	}
}

func TestExtractArchiveCandidateCleansStageOnFailure(t *testing.T) {
	installRoot := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("escape\n")); err != nil {
		t.Fatal(err)
	}
	closeErr := zw.Close()
	fileErr := f.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if fileErr != nil {
		t.Fatal(fileErr)
	}

	_, err = extractArchiveCandidate(archivePath, "0.2.0", installRoot)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive entry path") {
		t.Fatalf("expected extraction failure, got %v", err)
	}
	stagedRoot := filepath.Join(installRoot, ".slidex", "staged")
	entries, readErr := os.ReadDir(stagedRoot)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed extraction should remove staged candidate dirs: %#v", entries)
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
	writeCandidateBundleForTest(t, canaryRoot, "0.2.0-canary.20260610010000")
	findings = validateCandidateBundle(canaryRoot, "0.2.0-canary.20260610010000")
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

	codexRoot := t.TempDir()
	writeCandidateBundleForTest(t, codexRoot, "0.2.0")
	if err := os.WriteFile(filepath.Join(codexRoot, "plugins", "slidex", ".codex-plugin", "version-lock.json"), []byte(`{"pluginVersion":"0.2.0","slidexCliVersion":"0.2.0","requiredCodexCliVersion":"0.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	findings = validateCandidateBundle(codexRoot, "0.2.0")
	if !findingCheckPresent(findings, "update.candidate_version_lock") {
		t.Fatalf("candidate required Codex version drift should fail: %#v", findings)
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

func TestRunUpdateVerifyCandidateDoesNotExecuteBinaryByDefault(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := t.TempDir()
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	sentinel := filepath.Join(t.TempDir(), "executed")
	writeCandidateBinaryForTestWithSideEffect(t, filepath.Join(candidate, binary), "0.2.0", "pass", sentinel)

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath, "--candidate", candidate, "--target-version", "0.2.0", "--json"})
	})
	if runErr != nil {
		t.Fatalf("static candidate verify should pass: %v\n%s", runErr, output)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("candidate binary should not execute by default, sentinel stat err=%v", err)
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("invalid candidate verify JSON: %v\n%s", err, output)
	}
	if status.Status != "candidate-valid" {
		t.Fatalf("static candidate verify status = %#v", status)
	}

	output = captureStdoutForTest(t, func() {
		runErr = runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath, "--candidate", candidate, "--target-version", "0.2.0", "--execute-candidate-checks", "--json"})
	})
	if runErr != nil {
		t.Fatalf("dynamic candidate verify should pass: %v\n%s", runErr, output)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("candidate binary should execute with explicit dynamic checks: %v", err)
	}
}

func TestRunUpdateVerifyCandidateRejectsChannelSwitch(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := t.TempDir()
	writeCandidateBundleForTest(t, candidate, "0.2.0-canary.20260610010000")

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath, "--candidate", candidate, "--target-version", "0.2.0-canary.20260610010000", "--json"})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "candidate bundle validation failed") {
		t.Fatalf("candidate verify should reject channel switch, err=%v\n%s", runErr, output)
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("invalid candidate verify JSON: %v\n%s", err, output)
	}
	if status.Status != "candidate-invalid" || !findingCheckPresent(status.CandidateValidation, "update.candidate_channel") {
		t.Fatalf("candidate verify should report channel switch as invalid: %#v", status)
	}
}

func TestRunUpdateVerifyCandidateRejectsUnsafeTree(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := t.TempDir()
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	large := filepath.Join(candidate, "large.bin")
	if err := os.WriteFile(large, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(large, maxUpdateArchiveFileBytes+1); err != nil {
		t.Fatal(err)
	}

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath, "--candidate", candidate, "--target-version", "0.2.0", "--json"})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "candidate bundle validation failed") {
		t.Fatalf("candidate verify should reject unsafe tree, err=%v\n%s", runErr, output)
	}
	var status updateStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("invalid candidate verify JSON: %v\n%s", err, output)
	}
	if status.Status != "candidate-invalid" || !findingCheckPresent(status.CandidateValidation, "update.candidate_tree") {
		t.Fatalf("candidate verify should report unsafe tree as invalid: %#v", status)
	}
}

func TestValidateCandidateBundleStaticRejectsSymlinkedBinary(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0")
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	binaryPath := filepath.Join(root, binary)
	outside := filepath.Join(t.TempDir(), binary)
	writeCandidateBinaryForTest(t, outside, "0.2.0")
	if err := os.Remove(binaryPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, binaryPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	findings := validateCandidateBundleStatic(root, "0.2.0")
	if !findingCheckPresent(findings, "update.candidate_binary") {
		t.Fatalf("symlinked candidate binary should fail static validation: %#v", findings)
	}
}

func TestValidateCandidateBundleStaticRequiresOwnerExecutableUnixBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executability is not represented by Unix mode bits")
	}
	for _, tc := range []struct {
		name        string
		mode        os.FileMode
		wantFinding bool
	}{
		{name: "no execute bits", mode: 0o644, wantFinding: true},
		{name: "other execute only", mode: 0o001, wantFinding: true},
		{name: "group execute only", mode: 0o010, wantFinding: true},
		{name: "owner executable", mode: 0o700},
		{name: "world executable", mode: 0o755},
		{name: "world writable owner executable", mode: 0o777, wantFinding: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeCandidateBundleForTest(t, root, "0.2.0")
			binaryPath := filepath.Join(root, "slidex")
			if err := os.Chmod(binaryPath, tc.mode); err != nil {
				t.Fatal(err)
			}

			findings := validateCandidateBundleStatic(root, "0.2.0")
			gotFinding := findingCheckPresent(findings, "update.candidate_binary")
			if gotFinding != tc.wantFinding {
				t.Fatalf("candidate binary mode %04o finding = %v, want %v; findings=%#v", tc.mode, gotFinding, tc.wantFinding, findings)
			}
		})
	}
}

func TestValidateCandidateBundleStaticRejectsGroupWorldWritableUnixPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executability is not represented by Unix mode bits")
	}
	for _, tc := range []struct {
		name string
		rel  string
		mode os.FileMode
	}{
		{name: "runtime directory", rel: filepath.Join("plugins", "slidex"), mode: 0o777},
		{name: "runtime file", rel: "README.md", mode: 0o666},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeCandidateBundleForTest(t, root, "0.2.0")
			if err := os.Chmod(filepath.Join(root, tc.rel), tc.mode); err != nil {
				t.Fatal(err)
			}

			findings := validateCandidateBundleStatic(root, "0.2.0")
			if !findingCheckPresent(findings, "update.candidate_tree") {
				t.Fatalf("group/world-writable candidate path should fail static validation: %#v", findings)
			}
		})
	}
}

func TestValidateCandidateBundleStaticRejectsMissingRuntimeLeafFiles(t *testing.T) {
	for _, rel := range []string{
		".agents/skills/slidex/SKILL.md",
		"internal/codex/protocol/codex-cli-0.138.0/schema/ClientRequest.json",
		"plugins/slidex/.mcp.json",
		"plugins/slidex/skills/slidex/SKILL.md",
		"schemas/deck_spec.schema.json",
		"schemas/slidex_pending_update.schema.json",
		"schemas/slidex_update_state.schema.json",
	} {
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			root := t.TempDir()
			writeCandidateBundleForTest(t, root, "0.2.0")
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}

			findings := validateCandidateBundleStatic(root, "0.2.0")
			if !findingCheckPresent(findings, "update.candidate_runtime") {
				t.Fatalf("missing runtime leaf %s should fail static validation: %#v", rel, findings)
			}
		})
	}
}

func TestValidateCandidateBundleStaticRejectsMissingSchemaFiles(t *testing.T) {
	schemaFiles, err := requiredCandidateSchemaFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range schemaFiles {
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			root := t.TempDir()
			writeCandidateBundleForTest(t, root, "0.2.0")
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}

			findings := validateCandidateBundleStatic(root, "0.2.0")
			if !findingCheckPresent(findings, "update.candidate_schema") {
				t.Fatalf("missing candidate schema %s should fail static validation: %#v", rel, findings)
			}
		})
	}
}

func TestValidateCandidateBundleStaticRejectsCorruptSchemaFiles(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "empty", content: ""},
		{name: "invalid json", content: "{"},
		{name: "missing schema declaration", content: "{}"},
		{name: "uncompilable schema", content: `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":123}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeCandidateBundleForTest(t, root, "0.2.0")
			path := filepath.Join(root, "schemas", "stage_result.schema.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}

			findings := validateCandidateBundleStatic(root, "0.2.0")
			if !findingCheckPresent(findings, "update.candidate_schema") {
				t.Fatalf("corrupt candidate schema should fail static validation: %#v", findings)
			}
		})
	}
}

func TestValidateCandidateBundleStaticUsesCandidateLocalMetadataSchema(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0")
	path := filepath.Join(root, "schemas", installMetadataSchemaFile)
	if err := os.WriteFile(path, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","not":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := validateCandidateBundleStatic(root, "0.2.0")
	if !findingCheckPresent(findings, "update.candidate_install_metadata") {
		t.Fatalf("candidate metadata must be validated against candidate-local schema: %#v", findings)
	}
}

func TestValidateCandidateBundleStaticRejectsPluginContractDrift(t *testing.T) {
	for _, tc := range []struct {
		name      string
		wantCheck string
		mutate    func(t *testing.T, root string)
	}{
		{
			name:      "missing mcp config",
			wantCheck: "update.candidate_plugin_mcp",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "plugins", "slidex", ".mcp.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "wrong manifest skills path",
			wantCheck: "update.candidate_plugin_manifest",
			mutate: func(t *testing.T, root string) {
				mutateCandidateJSONForTest(t, filepath.Join(root, "plugins", "slidex", ".codex-plugin", "plugin.json"), func(value map[string]any) {
					value["skills"] = "./wrong-skills/"
				})
			},
		},
		{
			name:      "wrong manifest mcp path",
			wantCheck: "update.candidate_plugin_manifest",
			mutate: func(t *testing.T, root string) {
				mutateCandidateJSONForTest(t, filepath.Join(root, "plugins", "slidex", ".codex-plugin", "plugin.json"), func(value map[string]any) {
					value["mcpServers"] = "./wrong.json"
				})
			},
		},
		{
			name:      "wrong mcp command",
			wantCheck: "update.candidate_plugin_mcp",
			mutate: func(t *testing.T, root string) {
				mutateCandidateJSONForTest(t, filepath.Join(root, "plugins", "slidex", ".mcp.json"), func(value map[string]any) {
					servers := value["mcpServers"].(map[string]any)
					server := servers["slidex"].(map[string]any)
					server["command"] = "other-slidex"
				})
			},
		},
		{
			name:      "extra mcp server",
			wantCheck: "update.candidate_plugin_mcp",
			mutate: func(t *testing.T, root string) {
				mutateCandidateJSONForTest(t, filepath.Join(root, "plugins", "slidex", ".mcp.json"), func(value map[string]any) {
					servers := value["mcpServers"].(map[string]any)
					servers["extra"] = map[string]any{
						"command": "other-command",
						"args":    []any{"--stdio"},
					}
				})
			},
		},
		{
			name:      "unexpected mcp env",
			wantCheck: "update.candidate_plugin_mcp",
			mutate: func(t *testing.T, root string) {
				mutateCandidateJSONForTest(t, filepath.Join(root, "plugins", "slidex", ".mcp.json"), func(value map[string]any) {
					servers := value["mcpServers"].(map[string]any)
					server := servers["slidex"].(map[string]any)
					env := server["env"].(map[string]any)
					env["SSH_AUTH_SOCK"] = "/tmp/agent.sock"
				})
			},
		},
		{
			name:      "misdirected marketplace",
			wantCheck: "update.candidate_marketplace",
			mutate: func(t *testing.T, root string) {
				mutateCandidateJSONForTest(t, filepath.Join(root, ".agents", "plugins", "marketplace.json"), func(value map[string]any) {
					plugins := value["plugins"].([]any)
					entry := plugins[0].(map[string]any)
					source := entry["source"].(map[string]any)
					source["path"] = "./plugins/other"
				})
			},
		},
		{
			name:      "duplicate marketplace entry",
			wantCheck: "update.candidate_marketplace",
			mutate: func(t *testing.T, root string) {
				mutateCandidateJSONForTest(t, filepath.Join(root, ".agents", "plugins", "marketplace.json"), func(value map[string]any) {
					value["plugins"] = append(value["plugins"].([]any), map[string]any{
						"name":   "slidex",
						"source": map[string]any{"source": "local", "path": "./plugins/slidex"},
					})
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeCandidateBundleForTest(t, root, "0.2.0")
			tc.mutate(t, root)

			findings := validateCandidateBundleStatic(root, "0.2.0")
			if !findingCheckPresent(findings, tc.wantCheck) {
				t.Fatalf("candidate plugin contract drift should fail %s: %#v", tc.wantCheck, findings)
			}
		})
	}
}

func TestValidatePluginMCPConfigRejectsExtraServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	if err := writeSourceJSONFile(path, map[string]any{
		"mcpServers": map[string]any{
			"slidex": map[string]any{
				"command": toolName,
				"args":    []any{"mcp-server", "--stdio"},
				"env":     map[string]any{"SLIDEX_BROWSER_OPEN": "agent"},
			},
			"extra": map[string]any{
				"command": "other-command",
				"args":    []any{"--stdio"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	findings := validatePluginMCPConfig(path)
	if !findingCheckPresent(findings, "doctor.plugin_mcp") {
		t.Fatalf("extra MCP server should fail doctor plugin MCP validation: %#v", findings)
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

func TestValidateCandidateBundleRejectsInstallMetadataAdditionalProperties(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0")
	metadataPath := filepath.Join(root, ".slidex", "install.json")
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpectedField"] = true
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	findings := validateCandidateBundle(root, "0.2.0")
	if !findingCheckPresent(findings, "update.candidate_install_metadata") {
		t.Fatalf("candidate install metadata additional property should fail: %#v", findings)
	}
}

func TestValidateCandidateBundleRequiresMetadataChannelToMatchTargetVersion(t *testing.T) {
	root := t.TempDir()
	writeCandidateBundleForTest(t, root, "0.2.0-canary.20260610010000")
	metadataPath := filepath.Join(root, ".slidex", "install.json")
	metadata, err := readInstallMetadata(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata.Channel = updateChannelProduction
	writeInstallMetadataForTest(t, metadataPath, *metadata)

	findings := validateCandidateBundle(root, "0.2.0-canary.20260610010000")
	if !findingCheckPresent(findings, "update.candidate_install_metadata") {
		t.Fatalf("candidate metadata channel mismatch should fail: %#v", findings)
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

func candidateExtractedUnderForTest(t *testing.T, root string) bool {
	t.Helper()
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return false
	}
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if entry.Name() == "VERSION" {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestApplyCandidateBundleFailsForInvalidCandidate(t *testing.T) {
	installRoot := t.TempDir()
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	result, err := applyCandidateBundle(status, t.TempDir(), "0.2.0", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "candidate-invalid" || !hasFailures(result.CandidateValidation) {
		t.Fatalf("invalid candidate result = %#v", result)
	}
}

func TestApplyCandidateBundleRejectsMutableStateCollisionBeforeActivation(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if err := os.Mkdir(filepath.Join(candidate, ".slidex", "update_state.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	result, err := applyCandidateBundle(status, candidate, "0.2.0", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_mutable_state") {
		t.Fatalf("mutable state collision should be candidate-invalid: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("mutable state collision should not activate install root, VERSION = %q", got)
	}
	if _, err := os.Stat(updateStatePath(installRoot)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mutable state collision should not write update state in install root: %v", err)
	}
}

func TestValidateLocalCandidateTreeRejectsTotalBudget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "second.txt"), []byte("67890"), 0o644); err != nil {
		t.Fatal(err)
	}
	budget := &updateArchiveExtractionBudget{
		maxEntries:  10,
		maxFileSize: 10,
		maxTotal:    8,
	}

	err := validateLocalCandidateTreeWithBudget(root, budget)
	if err == nil || !strings.Contains(err.Error(), "candidate tree exceeds maximum expanded size") {
		t.Fatalf("expected candidate tree total budget error, got %v", err)
	}
}

func TestCopyCandidateToSiblingStageRejectsOversizedLocalCandidate(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	candidate := filepath.Join(parent, "candidate")
	if err := os.MkdirAll(candidate, 0o755); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(candidate, "large.bin")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(large, maxUpdateArchiveFileBytes+1); err != nil {
		t.Fatal(err)
	}

	stagedRoot, err := copyCandidateToSiblingStage(installRoot, candidate, "1.2.3", "staged")
	if err == nil || !strings.Contains(err.Error(), "candidate tree file exceeds maximum size") {
		t.Fatalf("expected candidate tree file budget error, got %v", err)
	}
	if stagedRoot != "" {
		if _, statErr := os.Stat(stagedRoot); !os.IsNotExist(statErr) {
			t.Fatalf("oversized candidate should not leave staged root, stat err=%v", statErr)
		}
	}
}

func TestApplyCandidateBundleRejectsTargetVersionChannelSwitch(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0-canary.20260610010000")

	result, err := applyCandidateBundle(status, candidate, "0.2.0-canary.20260610010000", "v0.2.0-canary.20260610010000")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_channel") {
		t.Fatalf("production install should reject canary target: %#v", result)
	}

	canaryRoot := filepath.Join(parent, "canary")
	if err := os.MkdirAll(filepath.Join(canaryRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(canaryRoot), releaseInstallMetadataForTest(t, toolVersion+"-canary.20260610000000"))
	canaryStatus, err := currentUpdateStatus(canaryRoot, installMetadataPath(canaryRoot))
	if err != nil {
		t.Fatal(err)
	}
	stableCandidate := filepath.Join(parent, "stable-candidate")
	writeCandidateBundleForTest(t, stableCandidate, "0.2.0")
	result, err = applyCandidateBundle(canaryStatus, stableCandidate, "0.2.0", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_channel") {
		t.Fatalf("canary install should reject production target: %#v", result)
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
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	result, err := applyCandidateBundle(status, candidate, "v0.2.0", "v0.2.0")
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

func TestApplyCandidateBundleRejectsMismatchedTargetTag(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}

	result, err := applyCandidateBundle(status, candidate, "0.2.0", "v9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.target_identity") {
		t.Fatalf("mismatched target tag should be candidate-invalid: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("mismatched target tag should not activate install root, VERSION = %q", got)
	}
}

func TestRunUpdateApplyRejectsMismatchedTargetTagBeforeActivation(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")

	err := runUpdateApply([]string{
		"--install-root", installRoot,
		"--metadata", installMetadataPath(installRoot),
		"--candidate", candidate,
		"--target-version", "0.2.0",
		"--target-tag", "v9.9.9",
		"--yes",
	})
	if err == nil || !strings.Contains(err.Error(), "resolves to 9.9.9") {
		t.Fatalf("expected mismatched target tag failure, got %v", err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("mismatched target tag should not activate install root, VERSION = %q", got)
	}
}

func TestRunUpdateApplyRejectsFutureInvalidCandidateMetadataBeforeActivation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*installMetadata)
	}{
		{name: "unknown commit", mutate: func(metadata *installMetadata) { metadata.Commit = "unknown" }},
		{name: "non RFC3339 build time", mutate: func(metadata *installMetadata) { metadata.BuildTime = "soon" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			installRoot := filepath.Join(parent, "slidex")
			if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
				t.Fatal(err)
			}
			metadataPath := installMetadataPath(installRoot)
			writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
			candidate := filepath.Join(parent, "candidate")
			writeCandidateBundleForTest(t, candidate, "0.2.0")
			candidateMetadataPath := filepath.Join(candidate, ".slidex", "install.json")
			candidateMetadata, err := readInstallMetadata(candidateMetadataPath)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(candidateMetadata)
			writeInstallMetadataForTest(t, candidateMetadataPath, *candidateMetadata)

			var runErr error
			output := captureStdoutForTest(t, func() {
				runErr = runUpdateApply([]string{
					"--install-root", installRoot,
					"--metadata", metadataPath,
					"--candidate", candidate,
					"--target-version", "0.2.0",
					"--target-tag", "v0.2.0",
					"--yes",
					"--json",
				})
			})
			if runErr == nil || !strings.Contains(runErr.Error(), "candidate bundle validation failed") {
				t.Fatalf("future-invalid candidate metadata should fail update apply, err=%v\n%s", runErr, output)
			}
			var result updateApplyResult
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatalf("invalid update apply JSON: %v\n%s", err, output)
			}
			if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_install_metadata") {
				t.Fatalf("future-invalid candidate metadata should be reported as candidate-invalid: %#v", result)
			}
			if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
				t.Fatalf("future-invalid candidate metadata should not replace active install root, VERSION = %q", got)
			}
		})
	}
}

func TestRunUpdateApplyDoesNotExecuteCandidateBinary(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	sentinel := filepath.Join(t.TempDir(), "executed")
	writeCandidateBinaryForTestWithSideEffect(t, filepath.Join(candidate, binary), "0.2.0", "pass", sentinel)

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", installRoot,
			"--metadata", metadataPath,
			"--candidate", candidate,
			"--target-version", "0.2.0",
			"--target-tag", "v0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr != nil {
		t.Fatalf("update apply should pass static candidate checks: %v\n%s", runErr, output)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("candidate binary should not execute during apply, sentinel stat err=%v", err)
	}
}

func TestRunUpdateApplyArchiveRejectsCandidateDoctorFailureBeforeActivation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows update apply stages pending activation instead of directly replacing install root")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	writeCandidateBinaryForTestWithDoctorStatus(t, filepath.Join(candidate, binary), "0.2.0", "fail")
	archivePath, checksumPath := writeCandidateReleaseArchiveForTest(t, parent, candidate, "0.2.0")

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", installRoot,
			"--metadata", metadataPath,
			"--archive", archivePath,
			"--checksums", checksumPath,
			"--target-version", "0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "candidate bundle validation failed") {
		t.Fatalf("archive doctor failure should fail update apply, err=%v\n%s", runErr, output)
	}
	var result updateApplyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid update apply JSON: %v\n%s", err, output)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_doctor") {
		t.Fatalf("archive doctor failure should be reported before activation: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("archive doctor failure should not replace active install root, VERSION = %q", got)
	}
}

func TestRunUpdateApplyRevalidatesStagedCandidateBeforeActivation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows update apply stages pending activation instead of directly replacing install root")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")

	oldHook := beforeUpdateStagedCandidateValidation
	beforeUpdateStagedCandidateValidation = func(stagedRoot string) error {
		return os.Chmod(filepath.Join(stagedRoot, "slidex"), 0o644)
	}
	t.Cleanup(func() {
		beforeUpdateStagedCandidateValidation = oldHook
	})

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", installRoot,
			"--metadata", metadataPath,
			"--candidate", candidate,
			"--target-version", "0.2.0",
			"--target-tag", "v0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "candidate bundle validation failed") {
		t.Fatalf("staged candidate validation should fail update apply, err=%v\n%s", runErr, output)
	}
	var result updateApplyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid update apply JSON: %v\n%s", err, output)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_binary") {
		t.Fatalf("staged candidate binary mutation should be reported as candidate-invalid: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("invalid staged candidate should not replace active install root, VERSION = %q", got)
	}
}

func TestRunUpdateApplyArchiveRejectsStagedBinaryVersionDriftBeforeActivation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows update apply stages pending activation instead of directly replacing install root")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	archivePath, checksumPath := writeCandidateReleaseArchiveForTest(t, parent, candidate, "0.2.0")

	oldHook := beforeUpdateStagedCandidateValidation
	beforeUpdateStagedCandidateValidation = func(stagedRoot string) error {
		writeCandidateBinaryForTest(t, filepath.Join(stagedRoot, "slidex"), "0.1.0")
		return nil
	}
	t.Cleanup(func() {
		beforeUpdateStagedCandidateValidation = oldHook
	})

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", installRoot,
			"--metadata", metadataPath,
			"--archive", archivePath,
			"--checksums", checksumPath,
			"--target-version", "0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "candidate bundle validation failed") {
		t.Fatalf("staged archive binary drift should fail update apply, err=%v\n%s", runErr, output)
	}
	var result updateApplyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid update apply JSON: %v\n%s", err, output)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_binary_version") {
		t.Fatalf("staged archive binary drift should be reported before activation: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("staged archive binary drift should not replace active install root, VERSION = %q", got)
	}
}

func TestRunUpdateApplyRejectsWritableStagedCandidateBeforeActivation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows update apply stages pending activation instead of directly replacing install root")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")

	oldHook := beforeUpdateStagedCandidateValidation
	beforeUpdateStagedCandidateValidation = func(stagedRoot string) error {
		return os.Chmod(filepath.Join(stagedRoot, "plugins", "slidex"), 0o777)
	}
	t.Cleanup(func() {
		beforeUpdateStagedCandidateValidation = oldHook
	})

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", installRoot,
			"--metadata", metadataPath,
			"--candidate", candidate,
			"--target-version", "0.2.0",
			"--target-tag", "v0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "candidate bundle validation failed") {
		t.Fatalf("writable staged candidate validation should fail update apply, err=%v\n%s", runErr, output)
	}
	var result updateApplyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid update apply JSON: %v\n%s", err, output)
	}
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_tree") {
		t.Fatalf("writable staged candidate should be reported as candidate-invalid: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("writable staged candidate should not replace active install root, VERSION = %q", got)
	}
}

func TestActivateStagedInstallRootRollsBackWhenActivationFails(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}

	missingStagedRoot := filepath.Join(parent, "missing-candidate")
	backupRoot, err := activateStagedInstallRoot(installRoot, missingStagedRoot, "0.2.0")
	if err == nil {
		t.Fatal("expected activation failure for missing staged root")
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("rollback did not restore install root VERSION, got %q", got)
	}
	if _, err := os.Stat(installRoot); err != nil {
		t.Fatalf("rollback did not restore install root: %v", err)
	}
	if _, err := os.Stat(backupRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup root should be moved back after rollback, stat err = %v", err)
	}
}

func TestUpdateInstallLockSerializesAccess(t *testing.T) {
	oldWait := updateInstallLockWaitTimeout
	oldRetry := updateInstallLockRetryDelay
	updateInstallLockWaitTimeout = time.Second
	updateInstallLockRetryDelay = 5 * time.Millisecond
	t.Cleanup(func() {
		updateInstallLockWaitTimeout = oldWait
		updateInstallLockRetryDelay = oldRetry
	})

	installRoot := filepath.Join(t.TempDir(), "slidex")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireUpdateInstallLock(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan error, 1)
	releaseSecond := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		unlockSecond, err := acquireUpdateInstallLock(installRoot)
		if err != nil {
			acquired <- err
			return
		}
		acquired <- nil
		<-releaseSecond
		unlockSecond()
	}()
	select {
	case err := <-acquired:
		unlock()
		close(releaseSecond)
		<-secondDone
		t.Fatalf("second update lock acquired before first release: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	unlock()
	select {
	case err := <-acquired:
		if err != nil {
			close(releaseSecond)
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(releaseSecond)
		t.Fatal("second update lock did not acquire after first release")
	}
	close(releaseSecond)
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second update lock did not release")
	}
}

func TestUpdateStaleInstallLockReclaimDoesNotDeleteReplacement(t *testing.T) {
	installRoot := filepath.Join(t.TempDir(), "slidex")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath, err := updateInstallLockPath(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("schema=old pid=0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	_, stale := staleUpdateInstallLockSnapshot(lockPath)
	if !stale {
		t.Fatal("expected stale update install lock")
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := canonicalUpdateInstallRoot(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	replacement := fmt.Sprintf("schema=%s pid=%d nonce=%s installRoot=%s acquired=%s\n", updateInstallLockSchema, os.Getpid(), newLockNonce(), filepath.ToSlash(canonicalRoot), time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(lockPath, []byte(replacement), 0o600); err != nil {
		t.Fatal(err)
	}

	if reclaimStaleLockFile(lockPath, maxUpdateMetadataBytes, staleUpdateInstallLockSnapshot) {
		t.Fatal("stale reclaim removed a replacement update lock")
	}
	got, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != replacement {
		t.Fatalf("replacement lock changed:\n%s", string(got))
	}
}

func TestUpdateInstallLockPathIgnoresTempAndOwnerEnvAliases(t *testing.T) {
	installRoot := filepath.Join(t.TempDir(), "slidex")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := updateInstallLockPath(installRoot)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "tmpdir"))
	t.Setenv("TEMP", filepath.Join(t.TempDir(), "temp"))
	t.Setenv("TMP", filepath.Join(t.TempDir(), "tmp"))
	t.Setenv("USERNAME", "forged-user")
	t.Setenv("USER", "forged-user")
	second, err := updateInstallLockPath(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("lock path changed across temp/owner env aliases:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestUpdateInstallLockPathCanonicalizesSymlinkedRoot(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real", "slidex")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasRoot := filepath.Join(parent, "alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	realLock, err := updateInstallLockPath(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	aliasLock, err := updateInstallLockPath(aliasRoot)
	if err != nil {
		t.Fatal(err)
	}
	if aliasLock != realLock {
		t.Fatalf("lock path did not canonicalize symlink alias:\nreal:  %s\nalias: %s", realLock, aliasLock)
	}
}

func TestCurrentUpdateStatusCanonicalizesSymlinkedInstallRoot(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real", "slidex")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := releaseInstallMetadataForTest(t, toolVersion)
	meta.InstallRoot = filepath.ToSlash(realRoot)
	writeInstallMetadataForTest(t, installMetadataPath(realRoot), meta)

	aliasRoot := filepath.Join(parent, "alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	status, err := currentUpdateStatus(aliasRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if status.InstallRoot != filepath.ToSlash(realRoot) {
		t.Fatalf("status used non-canonical install root: got %s want %s", status.InstallRoot, filepath.ToSlash(realRoot))
	}
	if status.MetadataPath != filepath.ToSlash(installMetadataPath(realRoot)) {
		t.Fatalf("status used non-canonical metadata path: got %s want %s", status.MetadataPath, filepath.ToSlash(installMetadataPath(realRoot)))
	}
}

func TestRunUpdateApplyCanonicalizesSymlinkedInstallRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses pending update handoff because the running executable can be locked")
	}
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real", "slidex")
	if err := os.MkdirAll(filepath.Join(realRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := releaseInstallMetadataForTest(t, toolVersion)
	meta.InstallRoot = filepath.ToSlash(realRoot)
	writeInstallMetadataForTest(t, installMetadataPath(realRoot), meta)

	aliasRoot := filepath.Join(parent, "alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")

	if err := runUpdateApply([]string{"--install-root", aliasRoot, "--candidate", candidate, "--target-version", "0.2.0", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(realRoot, "VERSION"))); got != "0.2.0" {
		t.Fatalf("apply used non-canonical install root, VERSION = %q", got)
	}
	status, err := currentUpdateStatus(aliasRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if status.InstallRoot != filepath.ToSlash(realRoot) {
		t.Fatalf("post-apply status used non-canonical install root: got %s want %s", status.InstallRoot, filepath.ToSlash(realRoot))
	}
	if status.InstalledMetadata == nil || status.InstalledMetadata.InstallRoot != filepath.ToSlash(realRoot) {
		t.Fatalf("post-apply metadata used non-canonical install root: %#v", status.InstalledMetadata)
	}
}

func TestUpdateInternalStageDirUsesRandomUniqueDirs(t *testing.T) {
	installRoot := filepath.Join(t.TempDir(), "slidex")
	first, err := updateInternalStageDir(installRoot, "downloads", "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	second, err := updateInternalStageDir(installRoot, "downloads", "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("staging dirs should be unique, both were %s", first)
	}
	root := filepath.Join(installRoot, ".slidex", "downloads")
	for _, dir := range []string{first, second} {
		if !pathWithin(root, dir) {
			t.Fatalf("stage dir escaped root: %s", dir)
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("stage dir missing: %s stat=%v", dir, err)
		}
	}
}

func TestActivateStagedInstallRootUsesUniqueBackupRoots(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte("0.1.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagedOne := filepath.Join(parent, "staged-one")
	if err := os.MkdirAll(stagedOne, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagedOne, "VERSION"), []byte("0.2.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	backupOne, err := activateStagedInstallRoot(installRoot, stagedOne, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	stagedTwo := filepath.Join(parent, "staged-two")
	if err := os.MkdirAll(stagedTwo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagedTwo, "VERSION"), []byte("0.3.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	backupTwo, err := activateStagedInstallRoot(installRoot, stagedTwo, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if backupOne == backupTwo {
		t.Fatalf("backup roots should be unique, both were %s", backupOne)
	}
	for _, backup := range []string{backupOne, backupTwo} {
		if info, err := os.Stat(backup); err != nil || !info.IsDir() {
			t.Fatalf("backup root missing: %s stat=%v", backup, err)
		}
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
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
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

	if err := runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--api-url", server.URL + "/releases", "--yes", "--json"}); err != nil {
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

func TestRunUpdateApplyRequiresMatchingChecksumBeforeActivation(t *testing.T) {
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
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	contract, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(parent, contract.ArchiveName)
	writeTarGzFromDirForTest(t, archivePath, candidate, strings.TrimSuffix(contract.ArchiveName, ".tar.gz"))
	checksumPath := filepath.Join(parent, contract.ChecksumName)
	if err := os.WriteFile(checksumPath, []byte(strings.Repeat("0", 64)+"  "+contract.ArchiveName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--archive", archivePath, "--checksums", checksumPath, "--target-version", "0.2.0", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected SHA-256 mismatch failure, got %v", err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("install root should not activate with checksum mismatch, got VERSION %q", got)
	}
	if candidateExtractedUnderForTest(t, filepath.Join(installRoot, ".slidex", "staged")) {
		t.Fatal("archive should not be extracted before checksum verification passes")
	}
}

func TestStageVerifiedLocalReleaseArchiveExtractsVerifiedCopy(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	contract, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(parent, contract.ArchiveName)
	topName := strings.TrimSuffix(strings.TrimSuffix(contract.ArchiveName, ".tar.gz"), ".zip")
	if strings.HasSuffix(contract.ArchiveName, ".zip") {
		writeZipFromDirForTest(t, archivePath, candidate, topName)
	} else {
		writeTarGzFromDirForTest(t, archivePath, candidate, topName)
	}
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	checksumPath := filepath.Join(parent, contract.ChecksumName)
	if err := os.WriteFile(checksumPath, []byte(hex.EncodeToString(sum[:])+"  "+contract.ArchiveName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stageParent, stagedArchive, err := stageVerifiedLocalReleaseArchive(installRoot, "0.2.0", archivePath, checksumPath)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(parent, "replacement")
	writeCandidateBundleForTest(t, replacement, "9.9.9")
	if strings.HasSuffix(contract.ArchiveName, ".zip") {
		writeZipFromDirForTest(t, archivePath, replacement, topName)
	} else {
		writeTarGzFromDirForTest(t, archivePath, replacement, topName)
	}

	extracted, err := extractDownloadedReleaseArchive(stageParent, stagedArchive)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(extracted, "VERSION"))); got != "0.2.0" {
		t.Fatalf("extracted VERSION = %q, want verified archive contents", got)
	}
}

func TestRunUpdateApplyRejectsUnsafeArchiveTargetVersionBeforeExtraction(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
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

	err = runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--archive", archivePath, "--checksums", checksumPath, "--target-version", "../../../escaped-stage", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "target version") {
		t.Fatalf("expected unsafe target version failure, got %v", err)
	}
	if candidateExtractedUnderForTest(t, filepath.Join(installRoot, ".slidex", "staged")) {
		t.Fatal("archive should not be extracted for an unsafe target version")
	}
	escaped, err := filepath.Glob(filepath.Join(parent, "escaped-stage*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(escaped) > 0 {
		t.Fatalf("unsafe target version created escaped staging paths: %v", escaped)
	}
}

func TestRunUpdateApplyRejectsNonReleaseArchiveTargetVersionBeforeExtraction(t *testing.T) {
	parent := t.TempDir()
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

	targets := []string{"dev-local", "0.1.0-beta.1", "0.1.0-e9c033e"}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			safeName := strings.NewReplacer(".", "_", "-", "_").Replace(target)
			installRoot := filepath.Join(parent, "slidex-"+safeName)
			if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
				t.Fatal(err)
			}
			writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))

			err := runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--archive", archivePath, "--checksums", checksumPath, "--target-version", target, "--yes"})
			if err == nil || !strings.Contains(err.Error(), "stable or canary package version") {
				t.Fatalf("expected non-release target version failure, got %v", err)
			}
			if candidateExtractedUnderForTest(t, filepath.Join(installRoot, ".slidex", "staged")) {
				t.Fatalf("archive should not be extracted for non-release target version %q", target)
			}
		})
	}
}

func TestRunUpdateApplyArchiveChecksumFailureJSONReportsFailureContract(t *testing.T) {
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
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	contract, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(parent, contract.ArchiveName)
	writeTarGzFromDirForTest(t, archivePath, candidate, strings.TrimSuffix(contract.ArchiveName, ".tar.gz"))
	checksumPath := filepath.Join(parent, contract.ChecksumName)
	if err := os.WriteFile(checksumPath, []byte(strings.Repeat("f", 64)+"  "+contract.ArchiveName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", installRoot,
			"--metadata", installMetadataPath(installRoot),
			"--archive", archivePath,
			"--checksums", checksumPath,
			"--target-version", "0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected SHA-256 mismatch failure, got %v\n%s", runErr, output)
	}
	var result updateApplyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid checksum failure JSON: %v\n%s", err, output)
	}
	if err := validatePayloadAgainstBundledSchema(result, updateApplyResultSchemaFile); err != nil {
		t.Fatalf("checksum failure JSON should match schema: %v\n%s", err, output)
	}
	if result.Status != "failed" || result.Channel != updateChannelProduction || result.TargetVersion != "0.2.0" {
		t.Fatalf("checksum failure fields missing: %#v", result)
	}
	if result.PluginVerificationStatus != "not_verified" || result.NextVerificationCommand != "slidex update verify --json" || result.RestartRequired {
		t.Fatalf("checksum failure plugin/restart fields missing: %#v", result)
	}
	if !strings.Contains(result.Error, "SHA-256 mismatch") {
		t.Fatalf("checksum failure error missing: %#v", result)
	}
}

func TestRunUpdateApplyArchiveAppliesWithoutTargetTag(t *testing.T) {
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
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
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

	err = runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--archive", archivePath, "--checksums", checksumPath, "--target-version", "0.2.0", "--yes"})
	if err != nil {
		t.Fatalf("archive apply without target tag failed: %v", err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != "0.2.0" {
		t.Fatalf("archive apply without target tag VERSION = %q", got)
	}
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if status.TargetTag != "v0.2.0" {
		t.Fatalf("archive apply should infer target tag from metadata, got %q", status.TargetTag)
	}
}

func TestRunUpdateApplyDownloadRequiresMatchingChecksumBeforeExtraction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses pending update handoff because the running executable can be locked")
	}
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
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
	checksumText := strings.Repeat("1", 64) + "  " + contract.ArchiveName + "\n"
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

	err = runUpdateApply([]string{"--install-root", installRoot, "--metadata", installMetadataPath(installRoot), "--api-url", server.URL + "/releases", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected downloaded release checksum failure, got %v", err)
	}
	if candidateExtractedUnderForTest(t, filepath.Join(installRoot, ".slidex", "downloads")) {
		t.Fatal("downloaded archive should not be extracted before checksum verification passes")
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

func TestUpdateApplyRejectsReleaseMetadataOutsideInstallRoot(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(parent, "external-install.json")
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")

	err := runUpdateApply([]string{"--install-root", installRoot, "--metadata", metadataPath, "--candidate", candidate, "--target-version", "0.2.0", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "updates are disabled") {
		t.Fatalf("out-of-tree metadata apply err = %v", err)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("out-of-tree metadata should not activate candidate, VERSION = %q", got)
	}
}

func TestRunUpdateApplyLocalDevelopmentJSONReportsFailureContract(t *testing.T) {
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

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", sourceRoot,
			"--metadata", filepath.Join(sourceRoot, ".slidex", "missing.json"),
			"--candidate", candidate,
			"--target-version", "0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "updates are disabled") {
		t.Fatalf("local-development apply JSON err = %v\n%s", runErr, output)
	}
	var result updateApplyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid local-development apply failure JSON: %v\n%s", err, output)
	}
	if err := validatePayloadAgainstBundledSchema(result, updateApplyResultSchemaFile); err != nil {
		t.Fatalf("local-development apply failure JSON should match schema: %v\n%s", err, output)
	}
	if result.Status != "failed" || result.Channel != updateChannelLocalDevelopment || result.TargetVersion != "0.2.0" {
		t.Fatalf("local-development failure fields missing: %#v", result)
	}
	if result.PluginVerificationStatus != "not_verified" || result.NextVerificationCommand != "slidex update verify --json" {
		t.Fatalf("local-development plugin fields missing: %#v", result)
	}
	if !strings.Contains(result.Error, "updates are disabled") {
		t.Fatalf("local-development failure error missing: %#v", result)
	}
}

func TestRunUpdateApplyCandidateReportsJSON(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")

	var runErr error
	output := captureStdoutForTest(t, func() {
		runErr = runUpdateApply([]string{
			"--install-root", installRoot,
			"--metadata", installMetadataPath(installRoot),
			"--candidate", candidate,
			"--target-version", "v0.2.0",
			"--yes",
			"--json",
		})
	})
	if runErr != nil {
		t.Fatalf("candidate apply failed: %v\n%s", runErr, output)
	}
	var result updateApplyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid candidate apply JSON: %v\n%s", err, output)
	}
	if err := validatePayloadAgainstBundledSchema(result, updateApplyResultSchemaFile); err != nil {
		t.Fatalf("candidate apply JSON should match schema: %v\n%s", err, output)
	}
	if result.Status != "applied" && result.Status != "pending-restart" {
		t.Fatalf("candidate apply result status = %#v", result)
	}
	if result.TargetVersion != "0.2.0" || result.TargetTag != "v0.2.0" {
		t.Fatalf("candidate apply target identity should be canonical: %#v", result)
	}
}

func TestStagePendingUpdateHandoffMarksRestartRequired(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	stagedRoot, pendingPath, err := stagePendingUpdateHandoff(installRoot, candidate, "v0.2.0", "v0.2.0")
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
	if status.PendingUpdate.StagedRootManifestSHA256 == "" {
		t.Fatalf("pending staged root manifest digest not recorded: %#v", status.PendingUpdate)
	}
	expectedDigest, err := candidateTreeManifestDigest(stagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingUpdate.StagedRootManifestSHA256 != expectedDigest {
		t.Fatalf("pending staged root digest = %s, want %s", status.PendingUpdate.StagedRootManifestSHA256, expectedDigest)
	}
	if _, err := os.Stat(filepath.FromSlash(status.PendingUpdate.ActivatorPath)); err != nil {
		t.Fatalf("pending activator missing: %v", err)
	}
	if !strings.Contains(status.PendingActivationCommand, filepath.ToSlash(status.PendingUpdate.ActivatorPath)) {
		t.Fatalf("pending activation command should use activator path: %s", status.PendingActivationCommand)
	}
	if !status.RestartRequired || status.PluginVerificationStatus != "restart_required" || status.TargetVersion != "0.2.0" || status.TargetTag != "v0.2.0" {
		t.Fatalf("pending handoff update status = %#v", status)
	}
	if !findingCheckPresent(updateVerificationFindings(status), "update.pending_activation") {
		t.Fatalf("pending activation finding missing")
	}
}

func TestStagePendingActivatorUsesUniqueSiblingRoots(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")

	first, err := stagePendingActivator(installRoot, candidate, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	second, err := stagePendingActivator(installRoot, candidate, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first) == filepath.Dir(second) {
		t.Fatalf("activator roots should be unique, both were %s", filepath.Dir(first))
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(first)), ".slidex.activator-0.2.0-") {
		t.Fatalf("unexpected activator root %s", first)
	}
	if err := validatePendingActivatorPath(installRoot, first, "0.2.0"); err != nil {
		t.Fatalf("staged activator should validate: %v", err)
	}
	for _, schemaFile := range []string{installMetadataSchemaFile, updateStateSchemaFile, pendingUpdateSchemaFile} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(first), "schemas", schemaFile)); err != nil {
			t.Fatalf("staged activator should include schema %s: %v", schemaFile, err)
		}
	}
}

func TestPendingUpdateIgnoresSerializedActivationCommand(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	pending, _, err := readPendingUpdate(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	pending.ActivationCommand = "evil-command --should-not-be-used"
	if err := writeSourceJSONFile(pendingUpdatePath(installRoot), pending); err != nil {
		t.Fatal(err)
	}

	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "pending-activation" {
		t.Fatalf("status = %s, want pending-activation: %#v", status.Status, status)
	}
	if strings.Contains(status.PendingActivationCommand, "evil-command") {
		t.Fatalf("status trusted serialized activation command: %s", status.PendingActivationCommand)
	}
	if !strings.Contains(status.PendingActivationCommand, filepath.ToSlash(filepath.FromSlash(pending.ActivatorPath))) {
		t.Fatalf("status command should be derived from activator path, got %s", status.PendingActivationCommand)
	}
}

func TestShellQuoteCommandQuotesDollarPaths(t *testing.T) {
	command := shellQuoteCommand([]string{
		"/tmp/slidex-$USER/bin/slidex",
		"update",
		"activate-pending",
		"--install-root",
		"/tmp/slidex-$1/O'Hare",
		"--yes",
		"--json",
	})
	want := "'/tmp/slidex-$USER/bin/slidex' update activate-pending --install-root '/tmp/slidex-$1/O'\\''Hare' --yes --json"
	if command != want {
		t.Fatalf("quoted command = %s, want %s", command, want)
	}
}

func TestValidatePendingUpdateRejectsExternalActivatorPath(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	pending, _, err := readPendingUpdate(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside-"+pendingActivatorBinaryName())
	if err := os.WriteFile(outside, []byte("not the activator"), 0o755); err != nil {
		t.Fatal(err)
	}
	pending.ActivatorPath = filepath.ToSlash(outside)
	if err := validatePendingUpdate(installRoot, pending); err == nil || !strings.Contains(err.Error(), "pending activator") {
		t.Fatalf("expected external activator rejection, got %v", err)
	}
}

func TestReadPendingUpdateRejectsAdditionalProperties(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	path := pendingUpdatePath(installRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpectedField"] = true
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err = readPendingUpdate(installRoot)
	if err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("pending update with additional field should fail schema validation, got %v", err)
	}

	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "pending-invalid" || !status.RestartRequired || status.PluginVerificationStatus != "restart_required" {
		t.Fatalf("schema-invalid pending update should be visible in status: %#v", status)
	}
	if status.PendingUpdatePath == "" || !strings.Contains(status.Reason, "pending update state is invalid") {
		t.Fatalf("schema-invalid pending update evidence missing: %#v", status)
	}
	if err := validatePayloadAgainstBundledSchema(status, updateStatusSchemaFile); err != nil {
		t.Fatalf("pending-invalid status should match schema: %v", err)
	}
	if !hasStatusBannerForTest(updateStatusBanners(status), "pending_update_invalid") {
		t.Fatalf("pending-invalid banner missing: %#v", updateStatusBanners(status))
	}
}

func TestReadPendingUpdateRejectsOverflowingTargetVersion(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	path := pendingUpdatePath(installRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("9", 128) + ".0.0"
	payload["targetVersion"] = huge
	payload["targetTag"] = "v" + huge
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readPendingUpdate(installRoot); err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("pending update with overflowing targetVersion should fail schema validation, got %v", err)
	}
}

func TestReadPendingUpdateRejectsSymlink(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "pending_update.json")
	if err := os.WriteFile(outside, []byte(`{"schemaVersion":"slidex.pendingUpdate.v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, pendingUpdatePath(installRoot)); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	_, _, err := readPendingUpdate(installRoot)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestReadCandidateJSONRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(path, []byte(`{"padding":"`+strings.Repeat("x", int(maxUpdateCandidateJSONBytes)+1)+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readCandidateJSON(path); err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected oversized candidate JSON rejection, got %v", err)
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
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
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

func TestActivatePendingUpdateRejectsTamperedStagedBundle(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	stagedRoot, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagedRoot, "VERSION"), []byte("0.2.1"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "pending-invalid" {
		t.Fatalf("tampered pending status should be invalid: %#v", status)
	}

	result, err := activatePendingUpdate(status)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending-invalid" || !findingCheckPresent(result.CandidateValidation, "update.pending_handoff") {
		t.Fatalf("tampered staged bundle should be rejected before activation: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("tampered pending activation should not replace active VERSION, got %q", got)
	}
}

func TestActivatePendingUpdateRejectsForgedStagedRootOutsideInstallParent(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}

	forgedRoot := filepath.Join(t.TempDir(), ".slidex.pending-0.2.0-forged")
	writeCandidateBundleForTest(t, forgedRoot, "0.2.0")
	forgedDigest, err := candidateTreeManifestDigest(forgedRoot)
	if err != nil {
		t.Fatal(err)
	}
	path := pendingUpdatePath(installRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["stagedRoot"] = filepath.ToSlash(forgedRoot)
	payload["stagedRootManifestSha256"] = forgedDigest
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := currentUpdateStatus(installRoot, installMetadataPath(installRoot))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "pending-invalid" || !strings.Contains(status.Reason, "install parent") {
		t.Fatalf("forged staged root should be pending-invalid: %#v", status)
	}
	result, err := activatePendingUpdate(status)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending-invalid" || !findingCheckPresent(result.CandidateValidation, "update.pending_handoff") {
		t.Fatalf("forged staged root should be rejected before activation: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("forged pending activation should not replace active VERSION, got %q", got)
	}
}

func TestActivatePendingUpdateDoesNotExecuteStagedCandidateBinary(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	sentinel := filepath.Join(t.TempDir(), "executed")
	writeCandidateBinaryForTestWithSideEffect(t, filepath.Join(candidate, binary), "0.2.0", "pass", sentinel)
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
	if result.Status != "applied" {
		t.Fatalf("activate pending result = %#v", result)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("staged candidate binary should not execute during pending activation, sentinel stat err=%v", err)
	}
}

func TestActivatePendingUpdateRejectsTargetVersionChannelSwitch(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion+"-canary.20260610000000"))
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
	if result.Status != "candidate-invalid" || !findingCheckPresent(result.CandidateValidation, "update.candidate_channel") {
		t.Fatalf("pending activation should reject channel switch: %#v", result)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != toolVersion {
		t.Fatalf("pending channel switch should not activate, VERSION = %q", got)
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
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := markPluginRestartRequired(installRoot, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	err := runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath})
	if err == nil || !strings.Contains(err.Error(), "update verification failed") {
		t.Fatalf("restart-required verify err = %v", err)
	}
	pluginPath := filepath.Join(installRoot, "plugins", "slidex")
	skillPath := filepath.Join(pluginPath, "skills", "slidex-start", "SKILL.md")
	if err := markPluginVerified(installRoot, toolVersion+"+codex.test", pluginPath, skillPath); err != nil {
		t.Fatal(err)
	}
	if err := runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath}); err != nil {
		t.Fatalf("verified update should pass: %v", err)
	}
}

func TestUpdateVerifyFailsOnPluginDrift(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
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
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
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

func TestUpdateStatusRejectsForgedVerifiedUpdateState(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := os.MkdirAll(filepath.Dir(updateStatePath(installRoot)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updateStatePath(installRoot), []byte(`{"verificationStatus":"verified","targetVersion":"0.2.0","updatedAt":"2026-06-10T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if !status.RestartRequired || status.PluginVerificationStatus != "restart_required" {
		t.Fatalf("forged update state should fail closed: %#v", status)
	}
	if !strings.Contains(status.Reason, "update state is invalid") {
		t.Fatalf("invalid update state reason missing: %#v", status)
	}
	err = runUpdateVerify([]string{"--install-root", installRoot, "--metadata", metadataPath})
	if err == nil || !strings.Contains(err.Error(), "update verification failed") {
		t.Fatalf("forged verified state should not pass update verify: %v", err)
	}
}

func TestUpdateStatusRejectsUpdateStateAdditionalProperties(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := os.MkdirAll(filepath.Dir(updateStatePath(installRoot)), 0o755); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"schemaVersion":       updateStateSchemaVersion,
		"toolName":            toolName,
		"currentVersion":      toolVersion,
		"targetVersion":       "0.2.0",
		"targetTag":           "v0.2.0",
		"channel":             updateChannelProduction,
		"restartRequired":     true,
		"verificationStatus":  "restart_required",
		"verificationCommand": "slidex codex app-server plugin-smoke --json",
		"updatedAt":           "2026-06-10T00:00:00Z",
		"unexpectedField":     true,
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updateStatePath(installRoot), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if !status.RestartRequired || status.PluginVerificationStatus != "restart_required" {
		t.Fatalf("schema-invalid update state should fail closed: %#v", status)
	}
	if !strings.Contains(status.Reason, "update state is invalid") || !strings.Contains(status.Reason, "validation") {
		t.Fatalf("schema-invalid update state reason missing: %#v", status)
	}
}

func TestUpdateStatusRejectsForgedVerifiedPluginEvidence(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := os.MkdirAll(filepath.Dir(updateStatePath(installRoot)), 0o755); err != nil {
		t.Fatal(err)
	}
	state := updateState{
		CurrentVersion:         toolVersion,
		TargetVersion:          "0.2.0",
		TargetTag:              "v0.2.0",
		Channel:                updateChannelProduction,
		RestartRequired:        false,
		VerificationStatus:     "verified",
		VerificationCommand:    "slidex update verify --json",
		VerifiedPluginVersion:  toolVersion + "+codex.test",
		VerifiedPluginPath:     filepath.ToSlash(filepath.Join(t.TempDir(), "plugins", "slidex")),
		VerifiedStartSkillPath: filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")),
		PluginUpdatedAt:        "2026-06-10T00:00:00Z",
		UpdatedAt:              "2026-06-10T00:00:00Z",
	}
	if err := writeUpdateState(installRoot, state); err != nil {
		t.Fatal(err)
	}

	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if !status.RestartRequired || status.PluginVerificationStatus != "restart_required" {
		t.Fatalf("forged verified plugin evidence should fail closed: %#v", status)
	}
	if !strings.Contains(status.Reason, "verifiedPluginPath must be under") {
		t.Fatalf("invalid verified plugin path reason missing: %#v", status)
	}
}

func TestUpdateCheckHumanAndJSONReportAvailableRelease(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	contract, err := releaseAssetContractFor("v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[
		  {"tag_name":"v0.2.0-canary.20260610010000","draft":false,"prerelease":true,"published_at":"2026-02-02T00:00:00Z","assets":[]},
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
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(t.TempDir(), "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	_, _, err := stagePendingUpdateHandoff(installRoot, candidate, "v0.2.0", "v0.2.0")
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
	if err := validatePayloadAgainstBundledSchema(status, updateStatusSchemaFile); err != nil {
		t.Fatalf("pending activation status JSON should match schema: %v\n%s", err, jsonOutput)
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

func TestUpdateJSONSchemasValidateBookkeepingPayloads(t *testing.T) {
	if findings := doctorUpdateSchemaFindings(); len(findings) > 0 {
		t.Fatalf("doctor update schema samples should pass: %#v", findings)
	}

	metadata := releaseInstallMetadataForTest(t, toolVersion)
	metadata.InstallRoot = filepath.ToSlash(t.TempDir())
	metadata.InstalledAt = "2026-06-10T00:00:00Z"
	if err := validatePayloadAgainstBundledSchema(metadata, installMetadataSchemaFile); err != nil {
		t.Fatalf("install metadata schema should accept release metadata: %v", err)
	}

	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var invalid map[string]any
	if err := json.Unmarshal(raw, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid["unexpectedField"] = true
	if err := validatePayloadAgainstBundledSchema(invalid, installMetadataSchemaFile); err == nil {
		t.Fatal("install metadata schema should reject additional fields")
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
	if err := validatePayloadAgainstBundledSchema(metadata, installMetadataSchemaFile); err != nil {
		t.Fatalf("packaged install metadata should match schema: %v", err)
	}
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

	canaryVersion := toolVersion + "-canary.20260610010000"
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
	if err := validatePayloadAgainstBundledSchema(canaryMetadata, installMetadataSchemaFile); err != nil {
		t.Fatalf("packaged canary install metadata should match schema: %v", err)
	}
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

func TestPackageReleaseRejectsUnsafeDistDirsBeforeDeleting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell release smoke uses the Unix package path")
	}
	root := repoRootForTest(t)
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeRM := filepath.Join(fakeBin, "rm")
	if err := os.WriteFile(fakeRM, []byte("#!/usr/bin/env bash\necho fake rm invoked \"$@\" >&2\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	nonEmptyDist := filepath.Join(t.TempDir(), "non-empty-dist")
	if err := os.MkdirAll(nonEmptyDist, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(nonEmptyDist, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkTarget := filepath.Join(t.TempDir(), "symlink-target")
	if err := os.MkdirAll(symlinkTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkSentinel := filepath.Join(symlinkTarget, "sentinel.txt")
	if err := os.WriteFile(symlinkSentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkDist := filepath.Join(t.TempDir(), "dist-link")
	if err := os.Symlink(symlinkTarget, symlinkDist); err != nil {
		t.Skipf("symlink setup unavailable: %v", err)
	}

	for _, tc := range []struct {
		name string
		dist string
		want string
	}{
		{name: "repo-root", dist: root, want: "refusing dangerous release dist dir"},
		{name: "repo-root-alias", dist: filepath.Join(root, "dist", ".."), want: "refusing dangerous release dist dir"},
		{name: "unmarked-non-empty", dist: nonEmptyDist, want: "refusing to clean unmarked non-empty release dist dir"},
		{name: "symlinked-dist", dist: symlinkDist, want: "refusing symlinked release dist dir"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", filepath.Join(root, "scripts", "package-release.sh"))
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"SLIDEX_RELEASE_VERSION=v"+toolVersion,
				"SLIDEX_BUILD_CHANNEL=production",
				"SLIDEX_TARGETS=linux/amd64",
				"SLIDEX_DIST_DIR="+tc.dist,
				"SLIDEX_BUILD_TIME=2026-06-10T00:00:00Z",
				"SLIDEX_COMMIT_SHA=0123456789abcdef",
			)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("package release should reject %s\n%s", tc.dist, out)
			}
			if !strings.Contains(string(out), tc.want) {
				t.Fatalf("release rejection = %q, want %q", out, tc.want)
			}
			if strings.Contains(string(out), "fake rm invoked") {
				t.Fatalf("release script reached rm before rejecting unsafe dist dir:\n%s", out)
			}
		})
	}
	if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != "keep" {
		t.Fatalf("unmarked dist sentinel should remain, raw=%q err=%v", raw, err)
	}
	if raw, err := os.ReadFile(symlinkSentinel); err != nil || string(raw) != "keep" {
		t.Fatalf("symlink target sentinel should remain, raw=%q err=%v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(symlinkTarget, ".slidex-dist")); !os.IsNotExist(err) {
		t.Fatalf("symlink target must not receive marker, err=%v", err)
	}
}

func TestCandidateBinaryVersionRejectsOversizedOutput(t *testing.T) {
	temp := t.TempDir()
	binary := filepath.Join(temp, "slidex")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	writeCandidateBinaryForTestWithLargeOutput(t, binary, "version")

	_, err := candidateBinaryVersionWithMaxOutput(binary, 128)
	if err == nil || !strings.Contains(err.Error(), "output exceeded") {
		t.Fatalf("expected output cap error, got %v", err)
	}
}

func TestCandidateDoctorStatusRejectsOversizedOutput(t *testing.T) {
	temp := t.TempDir()
	binary := filepath.Join(temp, "slidex")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	writeCandidateBinaryForTestWithLargeOutput(t, binary, "doctor")

	_, err := candidateDoctorStatusWithMaxOutput(temp, binary, 128)
	if err == nil || !strings.Contains(err.Error(), "output exceeded") {
		t.Fatalf("expected output cap error, got %v", err)
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

func releaseInstallMetadataForTest(t *testing.T, version string) installMetadata {
	t.Helper()
	channel := channelFromPackageVersion(version)
	if channel != updateChannelProduction && channel != updateChannelCanary {
		t.Fatalf("test release metadata version must be production or canary, got %q", version)
	}
	contract, err := releaseAssetContractFor("v"+version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	return installMetadata{
		SchemaVersion:    installMetadataSchemaVersion,
		ToolName:         toolName,
		Version:          version,
		Channel:          channel,
		Tag:              "v" + version,
		Commit:           "0123456789abcdef",
		BuildTime:        "2026-06-10T00:00:00Z",
		ReleaseAssetName: contract.ArchiveName,
		InstallMode:      installModeReleasePackage,
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
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
		".mise.toml":                     "go = \"1.26.3\"\n",
		"CODEX_INSTALL_PROMPT.md":        "Install slidex.\n",
		"INSTALL.md":                     "Install slidex.\n",
		"LICENSE":                        "MIT\n",
		"README.ko.md":                   "# slidex\n",
		"README.md":                      "# slidex\n",
		"VERSIONING.md":                  "# slidex Version Management\n",
		"VERSION":                        baseVersion,
		".agents/skills/slidex/SKILL.md": "# slidex skill\n",
		".agents/skills/slidex/references/commands.md":                        "# commands reference\n",
		"internal/codex/protocol/codex-cli-0.138.0/method_constants.go":       "package protocol\n",
		"internal/codex/protocol/codex-cli-0.138.0/generated_types.go":        "package protocol\n",
		"internal/codex/protocol/codex-cli-0.138.0/protocol_manifest.json":    "{}\n",
		"internal/codex/protocol/codex-cli-0.138.0/schema/ClientRequest.json": "{}\n",
		"internal/codex/protocol/codex-cli-0.138.0/schema/ServerRequest.json": "{}\n",
		"plugins/slidex/README.md":                                            "# slidex plugin\n",
		"plugins/slidex/hooks/manifest.json":                                  "{}\n",
		"plugins/slidex/skills/slidex/SKILL.md":                               "# slidex\n",
		"plugins/slidex/skills/slidex-finalize/SKILL.md":                      "# slidex finalize\n",
		"plugins/slidex/skills/slidex-run/SKILL.md":                           "# slidex run\n",
		"plugins/slidex/skills/slidex-start/SKILL.md":                         "# slidex start\n",
		"plugins/slidex/.mcp.json": `{
		  "mcpServers":{
		    "slidex":{
		      "command":"slidex",
		      "args":["mcp-server","--stdio"],
		      "env":{"SLIDEX_BROWSER_OPEN":"agent"}
		    }
		  }
		}`,
		".slidex/install.json": `{
		  "schemaVersion":"slidex.install.v1",
		  "toolName":"slidex",
		  "version":"` + version + `",
		  "channel":"` + channel + `",
		  "tag":"v` + version + `",
		  "commit":"0123456789abcdef",
		  "buildTime":"2026-06-10T00:00:00Z",
		  "installRoot":"",
		  "releaseAssetName":"` + contract.ArchiveName + `",
		  "installedAt":"",
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
	copySchemaFixturesForTest(t, root)
	writeCandidateBinaryForTest(t, filepath.Join(root, binary), baseVersion)
}

func copySchemaFixturesForTest(t *testing.T, root string) {
	t.Helper()
	sourceRoot := repoRelativePath("schemas")
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	destinationRoot := filepath.Join(root, "schemas")
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sourceRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destinationRoot, entry.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mutateCandidateJSONForTest(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	mutate(value)
	updated, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCandidateBinaryForTest(t *testing.T, path, version string) {
	t.Helper()
	writeCandidateBinaryForTestWithDoctorStatus(t, path, version, "pass")
}

func writeCandidateBinaryForTestWithDoctorStatus(t *testing.T, path, version, doctorStatus string) {
	t.Helper()
	writeCandidateBinaryForTestWithSideEffect(t, path, version, doctorStatus, "")
}

func writeCandidateBinaryForTestWithLargeOutput(t *testing.T, path, mode string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	code := `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == ` + fmt.Sprintf("%q", mode) + ` {
		for i := 0; i < 2048; i++ {
			fmt.Print("x")
		}
		return
	}
	fmt.Println("slidex 1.2.3")
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

func writeCandidateBinaryForTestWithSideEffect(t *testing.T, path, version, doctorStatus, sentinel string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	sideEffect := ""
	if sentinel != "" {
		sideEffect = `if err := os.WriteFile(` + fmt.Sprintf("%q", sentinel) + `, []byte("executed\n"), 0o600); err != nil {
		panic(err)
	}
	`
	}
	code := `package main

import (
	"fmt"
	"os"
)

func main() {
	` + sideEffect + `
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

func writeCandidateBinaryForTestWithCwdSentinel(t *testing.T, path, version, sentinel string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	code := `package main

import (
	"fmt"
	"os"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(` + fmt.Sprintf("%q", sentinel) + `, []byte(wd), 0o600); err != nil {
		panic(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("slidex ` + version + `")
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

func writeZipFromDirForTest(t *testing.T, archivePath, root, topName string) {
	t.Helper()
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(topName, rel))
		if d.IsDir() {
			if rel == "." {
				header.Name = filepath.ToSlash(topName)
			}
			header.Name = strings.TrimRight(header.Name, "/") + "/"
			_, err := zw.CreateHeader(header)
			return err
		}
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		raw, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, raw)
		closeErr := raw.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeErr := zw.Close()
	fileErr := f.Close()
	for _, err := range []error{walkErr, closeErr, fileErr} {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWriteZipFromDirForTestPreservesEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "schemas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "decks", "_template"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	writeZipFromDirForTest(t, archivePath, root, "slidex_0.1.0_windows_amd64")

	extractRoot := t.TempDir()
	if err := extractZipArchive(archivePath, extractRoot); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"schemas", filepath.Join("decks", "_template")} {
		path := filepath.Join(extractRoot, "slidex_0.1.0_windows_amd64", rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("extracted empty directory %s missing: %v", filepath.ToSlash(rel), err)
		}
		if !info.IsDir() {
			t.Fatalf("extracted empty directory %s is not a directory", filepath.ToSlash(rel))
		}
	}
}

func updateReleaseServerForCandidateForTest(t *testing.T, candidate, version string) *httptest.Server {
	t.Helper()
	contract, err := releaseAssetContractFor("v"+version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), contract.ArchiveName)
	topName := strings.TrimSuffix(strings.TrimSuffix(contract.ArchiveName, ".tar.gz"), ".zip")
	if strings.HasSuffix(contract.ArchiveName, ".zip") {
		writeZipFromDirForTest(t, archivePath, candidate, topName)
	} else {
		writeTarGzFromDirForTest(t, archivePath, candidate, topName)
	}
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
			_, _ = fmt.Fprintf(w, `[{"tag_name":%q,"draft":false,"prerelease":%t,"assets":[{"name":%q,"browser_download_url":%q,"digest":%q},{"name":%q,"browser_download_url":%q}]}]`,
				"v"+version,
				channelFromPackageVersion(version) == updateChannelCanary,
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
	return server
}

func writeCandidateReleaseArchiveForTest(t *testing.T, parent, candidate, version string) (archivePath, checksumPath string) {
	t.Helper()
	contract, err := releaseAssetContractFor("v"+version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archivePath = filepath.Join(parent, contract.ArchiveName)
	topName := strings.TrimSuffix(strings.TrimSuffix(contract.ArchiveName, ".tar.gz"), ".zip")
	if strings.HasSuffix(contract.ArchiveName, ".zip") {
		writeZipFromDirForTest(t, archivePath, candidate, topName)
	} else {
		writeTarGzFromDirForTest(t, archivePath, candidate, topName)
	}
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	checksumPath = filepath.Join(parent, contract.ChecksumName)
	if err := os.WriteFile(checksumPath, []byte(hex.EncodeToString(sum[:])+"  "+contract.ArchiveName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return archivePath, checksumPath
}
