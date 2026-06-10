package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	updateChannelProduction       = "production"
	updateChannelCanary           = "canary"
	updateChannelLocalDevelopment = "local-development"

	installModeReleasePackage = "release-package"
	installModeSourceCheckout = "source-checkout"
	installModeGoInstall      = "go-install"
	installModeUnknown        = "unknown"

	installMetadataSchemaVersion = "slidex.install.v1"
	updateStateSchemaVersion     = "slidex.updateState.v1"
	updateGitHubReleasesAPI      = "https://api.github.com/repos/shiinamachi/slidex/releases"

	updateInstallRootEnv     = "SLIDEX_INSTALL_ROOT"
	updateInstallMetadataEnv = "SLIDEX_INSTALL_METADATA"
)

var (
	stablePackageVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	canaryPackageVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-[0-9a-f]{7,40}$`)
)

type installMetadata struct {
	SchemaVersion    string `json:"schemaVersion"`
	ToolName         string `json:"toolName"`
	Version          string `json:"version"`
	Channel          string `json:"channel"`
	Tag              string `json:"tag"`
	Commit           string `json:"commit"`
	BuildTime        string `json:"buildTime"`
	InstallRoot      string `json:"installRoot"`
	ReleaseAssetName string `json:"releaseAssetName"`
	InstalledAt      string `json:"installedAt"`
	InstallMode      string `json:"installMode"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
}

type updateState struct {
	SchemaVersion       string `json:"schemaVersion"`
	ToolName            string `json:"toolName"`
	CurrentVersion      string `json:"currentVersion"`
	TargetVersion       string `json:"targetVersion,omitempty"`
	TargetTag           string `json:"targetTag,omitempty"`
	Channel             string `json:"channel"`
	RestartRequired     bool   `json:"restartRequired"`
	RestartReason       string `json:"restartReason,omitempty"`
	PluginUpdatedAt     string `json:"pluginUpdatedAt,omitempty"`
	VerificationStatus  string `json:"verificationStatus"`
	VerificationCommand string `json:"verificationCommand"`
	UpdatedAt           string `json:"updatedAt"`
}

type updateStatus struct {
	ToolName                  string           `json:"toolName"`
	CurrentVersion            string           `json:"currentVersion"`
	Channel                   string           `json:"channel"`
	InstallMode               string           `json:"installMode"`
	InstallRoot               string           `json:"installRoot"`
	MetadataPath              string           `json:"metadataPath"`
	UpdatesEnabled            bool             `json:"updatesEnabled"`
	Status                    string           `json:"status"`
	Reason                    string           `json:"reason,omitempty"`
	Guidance                  string           `json:"guidance,omitempty"`
	TargetVersion             string           `json:"targetVersion,omitempty"`
	TargetTag                 string           `json:"targetTag,omitempty"`
	ReleaseAssetName          string           `json:"releaseAssetName,omitempty"`
	ChecksumAssetName         string           `json:"checksumAssetName,omitempty"`
	RestartRequired           bool             `json:"restartRequired"`
	PluginVerificationStatus  string           `json:"pluginVerificationStatus"`
	NextVerificationCommand   string           `json:"nextVerificationCommand"`
	DiscoveredRelease         *updateRelease   `json:"discoveredRelease,omitempty"`
	CandidateValidation       []qaFinding      `json:"candidateValidation,omitempty"`
	InstalledMetadata         *installMetadata `json:"installedMetadata,omitempty"`
	PersistedRestartStatePath string           `json:"persistedRestartStatePath,omitempty"`
}

type statusBanner struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	Command  string `json:"command,omitempty"`
}

type updateRelease struct {
	TagName    string                 `json:"tagName"`
	Version    string                 `json:"version"`
	Prerelease bool                   `json:"prerelease"`
	Draft      bool                   `json:"draft"`
	Assets     []updateAsset          `json:"assets"`
	Raw        map[string]any         `json:"-"`
	AssetByKey map[string]updateAsset `json:"-"`
}

type updateAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browserDownloadUrl"`
	Digest             string `json:"digest,omitempty"`
	Size               int64  `json:"size,omitempty"`
}

type releaseAssetContract struct {
	Version      string `json:"version"`
	Tag          string `json:"tag"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	ArchiveName  string `json:"archiveName"`
	ChecksumName string `json:"checksumName"`
	ArchiveExt   string `json:"archiveExt"`
}

func runUpdate(args []string) error {
	if len(args) == 0 {
		return exitCodeError(2, "usage: slidex update status|check|verify")
	}
	switch args[0] {
	case "status":
		return runUpdateStatus(args[1:])
	case "check":
		return runUpdateCheck(args[1:])
	case "verify":
		return runUpdateVerify(args[1:])
	default:
		return exitCodeError(2, "unknown update command: %s", args[0])
	}
}

func runUpdateStatus(args []string) error {
	fs := flag.NewFlagSet("update status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "write JSON status")
	metadataPath := fs.String("metadata", "", "install metadata path")
	installRoot := fs.String("install-root", "", "install root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return exitCodeError(2, "usage: slidex update status [--json] [--metadata FILE] [--install-root DIR]")
	}
	status, err := currentUpdateStatus(*installRoot, *metadataPath)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(status)
	}
	printUpdateStatus(status)
	return nil
}

func runUpdateCheck(args []string) error {
	fs := flag.NewFlagSet("update check", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "write JSON status")
	metadataPath := fs.String("metadata", "", "install metadata path")
	installRoot := fs.String("install-root", "", "install root")
	apiURL := fs.String("api-url", updateGitHubReleasesAPI, "GitHub releases API URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return exitCodeError(2, "usage: slidex update check [--json] [--metadata FILE] [--install-root DIR] [--api-url URL]")
	}
	status, err := currentUpdateStatus(*installRoot, *metadataPath)
	if err != nil {
		return err
	}
	if status.UpdatesEnabled {
		releases, err := fetchUpdateReleases(context.Background(), *apiURL)
		if err != nil {
			return err
		}
		release, err := selectUpdateRelease(status.Channel, releases)
		if err != nil {
			return err
		}
		contract, err := releaseAssetContractFor(release.TagName, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return err
		}
		asset, checksum, err := release.requiredAssets(contract)
		if err != nil {
			return err
		}
		status.Status = "available"
		if release.Version == status.CurrentVersion {
			status.Status = "current"
		}
		status.TargetVersion = release.Version
		status.TargetTag = release.TagName
		status.ReleaseAssetName = asset.Name
		status.ChecksumAssetName = checksum.Name
		status.DiscoveredRelease = &release
	}
	if *jsonOut {
		return printJSON(status)
	}
	printUpdateStatus(status)
	return nil
}

func runUpdateVerify(args []string) error {
	fs := flag.NewFlagSet("update verify", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "write JSON status")
	metadataPath := fs.String("metadata", "", "install metadata path")
	installRoot := fs.String("install-root", "", "install root")
	candidate := fs.String("candidate", "", "extracted candidate bundle root")
	targetVersion := fs.String("target-version", "", "expected candidate version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return exitCodeError(2, "usage: slidex update verify [--json] [--metadata FILE] [--install-root DIR] [--candidate DIR --target-version VERSION]")
	}
	status, err := currentUpdateStatus(*installRoot, *metadataPath)
	if err != nil {
		return err
	}
	if *candidate != "" {
		if *targetVersion == "" {
			return exitCodeError(2, "--target-version is required with --candidate")
		}
		status.CandidateValidation = validateCandidateBundle(*candidate, *targetVersion)
		if hasFailures(status.CandidateValidation) {
			status.Status = "candidate-invalid"
		} else {
			status.Status = "candidate-valid"
		}
	}
	if *jsonOut {
		return printJSON(status)
	}
	printUpdateStatus(status)
	if hasFailures(status.CandidateValidation) {
		return exitCodeError(4, "candidate bundle validation failed")
	}
	return nil
}

func currentUpdateStatus(installRootArg, metadataPathArg string) (updateStatus, error) {
	installRoot := installRootArg
	if installRoot == "" {
		installRoot = os.Getenv(updateInstallRootEnv)
	}
	if installRoot == "" {
		installRoot = defaultInstallRoot()
	}
	metadataPath := metadataPathArg
	if metadataPath == "" {
		metadataPath = os.Getenv(updateInstallMetadataEnv)
	}
	if metadataPath == "" {
		metadataPath = installMetadataPath(installRoot)
	}

	metadata, metadataErr := readInstallMetadata(metadataPath)
	channel, mode, reason := inferUpdateChannel(installRoot, metadata, metadataErr)
	state, statePath, _ := readUpdateState(installRoot)
	status := updateStatus{
		ToolName:                  toolName,
		CurrentVersion:            toolVersion,
		Channel:                   channel,
		InstallMode:               mode,
		InstallRoot:               filepath.ToSlash(installRoot),
		MetadataPath:              filepath.ToSlash(metadataPath),
		UpdatesEnabled:            channel == updateChannelProduction || channel == updateChannelCanary,
		Status:                    "ready",
		Reason:                    reason,
		PluginVerificationStatus:  "not_verified",
		NextVerificationCommand:   "slidex update verify --json",
		PersistedRestartStatePath: filepath.ToSlash(statePath),
	}
	if metadata != nil {
		status.InstalledMetadata = metadata
		if metadata.InstallRoot != "" {
			status.InstallRoot = filepath.ToSlash(metadata.InstallRoot)
		}
	}
	if !status.UpdatesEnabled {
		status.Status = "disabled"
		status.Guidance = "Automatic release updates are disabled for local-development installs. Install a production or canary release package to enable updates."
	}
	if state != nil {
		status.RestartRequired = state.RestartRequired
		if state.VerificationStatus != "" {
			status.PluginVerificationStatus = state.VerificationStatus
		}
		if state.VerificationCommand != "" {
			status.NextVerificationCommand = state.VerificationCommand
		}
	}
	return status, nil
}

func updateStatusSnapshot() map[string]any {
	status, err := currentUpdateStatus("", "")
	if err != nil {
		return map[string]any{
			"toolName":       toolName,
			"currentVersion": toolVersion,
			"status":         "unknown",
			"error":          err.Error(),
			"banners": []statusBanner{{
				ID:       "update_status_error",
				Severity: "warn",
				Title:    "Update status unavailable",
				Message:  err.Error(),
			}},
		}
	}
	return map[string]any{
		"toolName":                 status.ToolName,
		"currentVersion":           status.CurrentVersion,
		"channel":                  status.Channel,
		"installMode":              status.InstallMode,
		"installRoot":              status.InstallRoot,
		"updatesEnabled":           status.UpdatesEnabled,
		"status":                   status.Status,
		"reason":                   status.Reason,
		"restartRequired":          status.RestartRequired,
		"pluginVerificationStatus": status.PluginVerificationStatus,
		"nextVerificationCommand":  status.NextVerificationCommand,
		"banners":                  updateStatusBanners(status),
	}
}

func updateStatusBanners(status updateStatus) []statusBanner {
	var banners []statusBanner
	if status.Channel == updateChannelCanary {
		banners = append(banners, statusBanner{
			ID:       "canary_channel",
			Severity: "info",
			Title:    "Canary channel",
			Message:  "This install follows canary prerelease bundles only.",
		})
	}
	if !status.UpdatesEnabled {
		banners = append(banners, statusBanner{
			ID:       "updates_disabled",
			Severity: "warn",
			Title:    "Automatic updates disabled",
			Message:  firstNonEmpty(status.Guidance, status.Reason),
		})
	}
	if status.RestartRequired {
		banners = append(banners, statusBanner{
			ID:       "codex_restart_required",
			Severity: "warn",
			Title:    "Codex restart required",
			Message:  "Restart Codex and start a new thread before treating updated slidex plugin skills as active.",
			Command:  status.NextVerificationCommand,
		})
	} else if status.PluginVerificationStatus == "verified" {
		banners = append(banners, statusBanner{
			ID:       "codex_plugin_verified",
			Severity: "ok",
			Title:    "Codex plugin verified",
			Message:  "The visible slidex plugin state matches this install.",
		})
	}
	return banners
}

func markPluginRestartRequired(installRoot, targetVersion, targetTag string) error {
	if installRoot == "" {
		installRoot = defaultInstallRoot()
	}
	return writeUpdateState(installRoot, updateState{
		CurrentVersion:      toolVersion,
		TargetVersion:       targetVersion,
		TargetTag:           targetTag,
		Channel:             channelFromPackageVersion(targetVersion),
		RestartRequired:     true,
		RestartReason:       "bundled Codex plugin content may have changed during slidex bundle update",
		PluginUpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		VerificationStatus:  "restart_required",
		VerificationCommand: "slidex codex app-server plugin-smoke --json",
	})
}

func markPluginVerified(installRoot, pluginVersion, skillPath string) error {
	if installRoot == "" {
		installRoot = defaultInstallRoot()
	}
	state, _, _ := readUpdateState(installRoot)
	if state == nil {
		state = &updateState{}
	}
	state.CurrentVersion = toolVersion
	if state.TargetVersion == "" {
		state.TargetVersion = pluginVersionBase(pluginVersion)
	}
	state.RestartRequired = false
	state.RestartReason = ""
	state.VerificationStatus = "verified"
	state.VerificationCommand = "slidex update verify --json"
	state.PluginUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeUpdateState(installRoot, *state)
}

func inferUpdateChannel(installRoot string, metadata *installMetadata, metadataErr error) (channel, mode, reason string) {
	if metadata != nil {
		switch metadata.Channel {
		case updateChannelProduction, updateChannelCanary:
			return metadata.Channel, firstNonEmpty(metadata.InstallMode, installModeReleasePackage), "channel recorded by original release package metadata"
		case updateChannelLocalDevelopment:
			return updateChannelLocalDevelopment, firstNonEmpty(metadata.InstallMode, installModeSourceCheckout), "local-development recorded by install metadata"
		}
		return updateChannelLocalDevelopment, firstNonEmpty(metadata.InstallMode, installModeUnknown), "install metadata does not contain a supported immutable update channel"
	}
	if looksLikeSourceCheckout(installRoot) {
		return updateChannelLocalDevelopment, installModeSourceCheckout, "install metadata is absent and install root is a source checkout"
	}
	if metadataErr != nil && !errors.Is(metadataErr, os.ErrNotExist) {
		return updateChannelLocalDevelopment, installModeUnknown, "install metadata could not be read; update is disabled fail-closed: " + metadataErr.Error()
	}
	return updateChannelLocalDevelopment, installModeGoInstall, "install metadata is absent; treating this as a go install or development binary"
}

func defaultInstallRoot() string {
	exe, err := os.Executable()
	if err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		return filepath.Dir(exe)
	}
	return mustAbs(".")
}

func installMetadataPath(installRoot string) string {
	return filepath.Join(installRoot, ".slidex", "install.json")
}

func updateStatePath(installRoot string) string {
	return filepath.Join(installRoot, ".slidex", "update_state.json")
}

func readInstallMetadata(path string) (*installMetadata, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var metadata installMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	if metadata.SchemaVersion != "" && metadata.SchemaVersion != installMetadataSchemaVersion {
		return nil, fmt.Errorf("%s: unsupported schemaVersion %q", filepath.ToSlash(path), metadata.SchemaVersion)
	}
	if metadata.ToolName != "" && metadata.ToolName != toolName {
		return nil, fmt.Errorf("%s: toolName must be %s", filepath.ToSlash(path), toolName)
	}
	return &metadata, nil
}

func readUpdateState(installRoot string) (*updateState, string, error) {
	path := updateStatePath(installRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	var state updateState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, path, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	return &state, path, nil
}

func writeUpdateState(installRoot string, state updateState) error {
	path := updateStatePath(installRoot)
	state.SchemaVersion = updateStateSchemaVersion
	state.ToolName = toolName
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if state.VerificationCommand == "" {
		state.VerificationCommand = "slidex update verify --json"
	}
	if state.VerificationStatus == "" {
		state.VerificationStatus = "restart_required"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeSourceJSONFile(path, state)
}

func looksLikeSourceCheckout(root string) bool {
	if root == "" {
		return false
	}
	for _, marker := range []string{"go.mod", ".git", "cmd/slidex/main.go"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(marker))); err != nil {
			return false
		}
	}
	return true
}

func releasePackageVersionFromTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", errors.New("release tag is required")
	}
	version := strings.TrimPrefix(tag, "v")
	if !isReleaseBaseVersion(version) {
		return "", fmt.Errorf("release tag %q does not map to a valid slidex package version", tag)
	}
	return version, nil
}

func releaseAssetContractFor(tag, goos, goarch string) (releaseAssetContract, error) {
	version, err := releasePackageVersionFromTag(tag)
	if err != nil {
		return releaseAssetContract{}, err
	}
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	if goos == "" || goarch == "" {
		return releaseAssetContract{}, errors.New("os and arch are required")
	}
	return releaseAssetContract{
		Version:      version,
		Tag:          tag,
		OS:           goos,
		Arch:         goarch,
		ArchiveName:  fmt.Sprintf("slidex_%s_%s_%s.%s", version, goos, goarch, ext),
		ChecksumName: fmt.Sprintf("slidex_%s_checksums.txt", version),
		ArchiveExt:   ext,
	}, nil
}

func channelFromPackageVersion(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	switch {
	case canaryPackageVersionPattern.MatchString(version):
		return updateChannelCanary
	case stablePackageVersionPattern.MatchString(version):
		return updateChannelProduction
	default:
		return updateChannelLocalDevelopment
	}
}

func fetchUpdateReleases(ctx context.Context, apiURL string) ([]updateRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "slidex-update/"+toolVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub Releases API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseUpdateReleases(raw)
}

func parseUpdateReleases(raw []byte) ([]updateRelease, error) {
	var releaseValues []map[string]any
	if err := json.Unmarshal(raw, &releaseValues); err != nil {
		return nil, err
	}
	releases := make([]updateRelease, 0, len(releaseValues))
	for _, value := range releaseValues {
		tag := metadataString(value["tag_name"])
		version, err := releasePackageVersionFromTag(tag)
		if err != nil {
			continue
		}
		release := updateRelease{
			TagName:    tag,
			Version:    version,
			Prerelease: metadataBool(value["prerelease"]),
			Draft:      metadataBool(value["draft"]),
			Raw:        value,
			AssetByKey: map[string]updateAsset{},
		}
		rawAssets, _ := value["assets"].([]any)
		for _, rawAsset := range rawAssets {
			assetValue, _ := rawAsset.(map[string]any)
			if assetValue == nil {
				continue
			}
			asset := updateAsset{
				Name:               metadataString(assetValue["name"]),
				BrowserDownloadURL: metadataString(assetValue["browser_download_url"]),
				Digest:             metadataString(assetValue["digest"]),
				Size:               metadataInt64(assetValue["size"]),
			}
			if asset.Name == "" {
				continue
			}
			release.Assets = append(release.Assets, asset)
			release.AssetByKey[asset.Name] = asset
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func selectUpdateRelease(channel string, releases []updateRelease) (updateRelease, error) {
	for _, release := range releases {
		if release.Draft {
			continue
		}
		switch channel {
		case updateChannelProduction:
			if !release.Prerelease && channelFromPackageVersion(release.Version) == updateChannelProduction {
				return release, nil
			}
		case updateChannelCanary:
			if release.Prerelease || channelFromPackageVersion(release.Version) == updateChannelCanary {
				if channelFromPackageVersion(release.Version) == updateChannelCanary {
					return release, nil
				}
			}
		default:
			return updateRelease{}, fmt.Errorf("updates are disabled for channel %q", channel)
		}
	}
	return updateRelease{}, fmt.Errorf("no matching %s release found", channel)
}

func (release updateRelease) requiredAssets(contract releaseAssetContract) (archive updateAsset, checksum updateAsset, err error) {
	archive, ok := release.AssetByKey[contract.ArchiveName]
	if !ok {
		return updateAsset{}, updateAsset{}, fmt.Errorf("release %s is missing archive asset %s", release.TagName, contract.ArchiveName)
	}
	checksum, ok = release.AssetByKey[contract.ChecksumName]
	if !ok {
		return updateAsset{}, updateAsset{}, fmt.Errorf("release %s is missing checksum asset %s", release.TagName, contract.ChecksumName)
	}
	return archive, checksum, nil
}

func verifyReleaseAssetSHA256(assetName string, payload []byte, checksumText string, githubDigest string) (string, error) {
	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	var expected []string
	if digest := normalizeGitHubSHA256Digest(githubDigest); digest != "" {
		expected = append(expected, digest)
	}
	if checksumText != "" {
		checksum, err := checksumForAsset(checksumText, assetName)
		if err != nil {
			return actual, err
		}
		expected = append(expected, checksum)
	}
	if len(expected) == 0 {
		return actual, fmt.Errorf("missing SHA-256 digest evidence for %s", assetName)
	}
	for _, want := range expected {
		if !strings.EqualFold(want, actual) {
			return actual, fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s", assetName, want, actual)
		}
	}
	return actual, nil
}

func normalizeGitHubSHA256Digest(digest string) string {
	digest = strings.TrimSpace(strings.ToLower(digest))
	digest = strings.TrimPrefix(digest, "sha256:")
	if len(digest) != 64 {
		return ""
	}
	for _, r := range digest {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return ""
		}
	}
	return digest
}

func checksumForAsset(text, assetName string) (string, error) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "(") && strings.Contains(line, ")=") {
			if checksum, ok := parseBSDChecksumLine(line, assetName); ok {
				return checksum, nil
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(name) == assetName && normalizeGitHubSHA256Digest(fields[0]) != "" {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksum evidence for %s is missing", assetName)
}

func parseBSDChecksumLine(line, assetName string) (string, bool) {
	open := strings.Index(line, "(")
	close := strings.Index(line, ")=")
	if open < 0 || close <= open {
		return "", false
	}
	name := line[open+1 : close]
	if filepath.Base(name) != assetName {
		return "", false
	}
	checksum := strings.TrimSpace(line[close+2:])
	if normalizeGitHubSHA256Digest(checksum) == "" {
		return "", false
	}
	return strings.ToLower(checksum), true
}

func validateCandidateBundle(root, expectedVersion string) []qaFinding {
	root = filepath.Clean(root)
	var findings []qaFinding
	required := []string{
		"VERSION",
		"decks/_template",
		"schemas",
		"plugins/slidex/.codex-plugin/plugin.json",
		"plugins/slidex/.codex-plugin/version-lock.json",
		".agents/plugins/marketplace.json",
		"internal/codex/protocol",
	}
	for _, rel := range required {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err != nil {
			findings = append(findings, fail("update.candidate_runtime", "missing candidate runtime path: "+err.Error(), filepath.ToSlash(path)))
		}
	}
	version := strings.TrimSpace(readFileOrEmpty(filepath.Join(root, "VERSION")))
	if version != expectedVersion {
		findings = append(findings, fail("update.candidate_version", fmt.Sprintf("candidate VERSION must be %s, got %s", expectedVersion, firstNonEmpty(version, "missing")), filepath.ToSlash(filepath.Join(root, "VERSION"))))
	}
	manifestPath := filepath.Join(root, "plugins", "slidex", ".codex-plugin", "plugin.json")
	if manifest, err := readCandidateJSON(manifestPath); err != nil {
		findings = append(findings, fail("update.candidate_plugin_manifest", err.Error(), filepath.ToSlash(manifestPath)))
	} else {
		if got := metadataString(manifest["name"]); got != toolName {
			findings = append(findings, fail("update.candidate_plugin_manifest", "plugin manifest name must be "+toolName, filepath.ToSlash(manifestPath)))
		}
		if got := pluginVersionBase(metadataString(manifest["version"])); got != expectedVersion {
			findings = append(findings, fail("update.candidate_plugin_manifest", "plugin manifest version base must be "+expectedVersion+", got "+got, filepath.ToSlash(manifestPath)))
		}
	}
	lockPath := filepath.Join(root, "plugins", "slidex", ".codex-plugin", "version-lock.json")
	if lock, err := readCandidateJSON(lockPath); err != nil {
		findings = append(findings, fail("update.candidate_version_lock", err.Error(), filepath.ToSlash(lockPath)))
	} else {
		for _, key := range []string{"pluginVersion", "slidexCliVersion"} {
			if got := metadataString(lock[key]); got != expectedVersion {
				findings = append(findings, fail("update.candidate_version_lock", key+" must be "+expectedVersion+", got "+got, filepath.ToSlash(lockPath)))
			}
		}
		if got := metadataString(lock["requiredCodexCliVersion"]); got == "" {
			findings = append(findings, fail("update.candidate_version_lock", "requiredCodexCliVersion is required", filepath.ToSlash(lockPath)))
		}
	}
	marketplacePath := filepath.Join(root, ".agents", "plugins", "marketplace.json")
	if marketplace, err := readCandidateJSON(marketplacePath); err != nil {
		findings = append(findings, fail("update.candidate_marketplace", err.Error(), filepath.ToSlash(marketplacePath)))
	} else if !candidateMarketplacePointsToBundledPlugin(marketplace) {
		findings = append(findings, fail("update.candidate_marketplace", "candidate marketplace must point slidex to ./plugins/slidex", filepath.ToSlash(marketplacePath)))
	}
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	if _, err := os.Stat(filepath.Join(root, binary)); err != nil {
		findings = append(findings, fail("update.candidate_binary", "missing candidate CLI binary: "+err.Error(), filepath.ToSlash(filepath.Join(root, binary))))
	}
	return findings
}

func readCandidateJSON(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func candidateMarketplacePointsToBundledPlugin(manifest map[string]any) bool {
	plugins, _ := manifest["plugins"].([]any)
	for _, raw := range plugins {
		plugin, _ := raw.(map[string]any)
		if metadataString(plugin["name"]) != toolName {
			continue
		}
		source, _ := plugin["source"].(map[string]any)
		return metadataString(source["source"]) == "local" && metadataString(source["path"]) == "./plugins/slidex"
	}
	return false
}

func metadataBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func metadataInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

func printUpdateStatus(status updateStatus) {
	fmt.Printf("%s update %s\n", status.ToolName, status.Status)
	fmt.Printf("channel: %s\n", status.Channel)
	fmt.Printf("current version: %s\n", status.CurrentVersion)
	if status.TargetVersion != "" {
		fmt.Printf("target version: %s (%s)\n", status.TargetVersion, status.TargetTag)
	}
	if status.ReleaseAssetName != "" {
		fmt.Printf("release asset: %s\n", status.ReleaseAssetName)
	}
	if status.Reason != "" {
		fmt.Printf("reason: %s\n", status.Reason)
	}
	if status.Guidance != "" {
		fmt.Printf("guidance: %s\n", status.Guidance)
	}
	if status.RestartRequired {
		fmt.Println("restart required: restart Codex and start a new thread before treating updated plugin skills as active")
	}
	fmt.Printf("next verification: %s\n", status.NextVerificationCommand)
}

func sortedReleaseAssetNames(release updateRelease) []string {
	names := make([]string, 0, len(release.Assets))
	for _, asset := range release.Assets {
		names = append(names, asset.Name)
	}
	sort.Strings(names)
	return names
}
