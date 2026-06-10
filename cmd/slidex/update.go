package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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
	"os/exec"
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
	pendingUpdateSchemaVersion   = "slidex.pendingUpdate.v1"
	updateGitHubReleasesAPI      = "https://api.github.com/repos/shiinamachi/slidex/releases"
	updateGitHubRepo             = "shiinamachi/slidex"

	attestationPolicyRequire         = "require"
	attestationPolicyAllowUnverified = "allow-unverified"

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

type updateApplyResult struct {
	ToolName                string                  `json:"toolName"`
	CurrentVersion          string                  `json:"currentVersion"`
	TargetVersion           string                  `json:"targetVersion"`
	TargetTag               string                  `json:"targetTag,omitempty"`
	Channel                 string                  `json:"channel"`
	InstallRoot             string                  `json:"installRoot"`
	Status                  string                  `json:"status"`
	StagedRoot              string                  `json:"stagedRoot,omitempty"`
	BackupRoot              string                  `json:"backupRoot,omitempty"`
	PendingUpdatePath       string                  `json:"pendingUpdatePath,omitempty"`
	RestartRequired         bool                    `json:"restartRequired"`
	NextVerificationCommand string                  `json:"nextVerificationCommand"`
	Attestation             attestationVerification `json:"attestation"`
	CandidateValidation     []qaFinding             `json:"candidateValidation,omitempty"`
}

type attestationVerification struct {
	Policy  string `json:"policy"`
	Status  string `json:"status"`
	Command string `json:"command,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

type pendingUpdate struct {
	SchemaVersion string `json:"schemaVersion"`
	ToolName      string `json:"toolName"`
	TargetVersion string `json:"targetVersion"`
	TargetTag     string `json:"targetTag,omitempty"`
	InstallRoot   string `json:"installRoot"`
	StagedRoot    string `json:"stagedRoot"`
	Reason        string `json:"reason"`
	CreatedAt     string `json:"createdAt"`
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
		return exitCodeError(2, "usage: slidex update status|check|apply|verify")
	}
	switch args[0] {
	case "status":
		return runUpdateStatus(args[1:])
	case "check":
		return runUpdateCheck(args[1:])
	case "apply":
		return runUpdateApply(args[1:])
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

func runUpdateApply(args []string) error {
	fs := flag.NewFlagSet("update apply", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "write JSON status")
	metadataPath := fs.String("metadata", "", "install metadata path")
	installRoot := fs.String("install-root", "", "install root")
	candidate := fs.String("candidate", "", "extracted candidate bundle root")
	archive := fs.String("archive", "", "release archive path")
	checksums := fs.String("checksums", "", "release checksums file")
	targetVersion := fs.String("target-version", "", "expected target version")
	targetTag := fs.String("target-tag", "", "target release tag")
	apiURL := fs.String("api-url", updateGitHubReleasesAPI, "GitHub releases API URL")
	attestationPolicy := fs.String("attestation-policy", attestationPolicyRequire, "attestation policy: require or allow-unverified")
	yes := fs.Bool("yes", false, "activate the staged update")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return exitCodeError(2, "usage: slidex update apply --yes [--json] [--install-root DIR] [--metadata FILE] [--api-url URL] [--attestation-policy require|allow-unverified] [--target-version VERSION --candidate DIR | --target-version VERSION --archive FILE --checksums FILE --target-tag TAG]")
	}
	if err := validateAttestationPolicy(*attestationPolicy); err != nil {
		return err
	}
	if !*yes {
		return exitCodeError(2, "slidex update apply requires --yes before replacing the install root")
	}
	if *candidate != "" && *archive != "" {
		return exitCodeError(2, "provide only one of --candidate or --archive")
	}
	status, err := currentUpdateStatus(*installRoot, *metadataPath)
	if err != nil {
		return err
	}
	if !status.UpdatesEnabled {
		return exitCodeError(4, "updates are disabled for channel %s: %s", status.Channel, firstNonEmpty(status.Guidance, status.Reason))
	}
	candidateRoot := *candidate
	attestation := attestationVerification{Policy: *attestationPolicy, Status: "not_applicable"}
	if *candidate == "" && *archive == "" {
		downloadedRoot, downloadedVersion, downloadedTag, downloadedAttestation, err := downloadAndStageReleaseCandidate(context.Background(), status, *apiURL, *attestationPolicy)
		if err != nil {
			return err
		}
		candidateRoot = downloadedRoot
		attestation = downloadedAttestation
		if *targetVersion == "" {
			*targetVersion = downloadedVersion
		}
		if *targetTag == "" {
			*targetTag = downloadedTag
		}
	} else if *archive != "" {
		if *checksums == "" {
			return exitCodeError(2, "--checksums is required with --archive")
		}
		if *targetVersion == "" {
			return exitCodeError(2, "--target-version is required with --archive")
		}
		extracted, err := stageArchiveCandidate(*archive, *checksums, *targetVersion, status.InstallRoot)
		if err != nil {
			return err
		}
		archiveAttestation, err := verifyReleaseAttestation(*archive, *targetTag, *attestationPolicy)
		if err != nil {
			return err
		}
		attestation = archiveAttestation
		candidateRoot = extracted
	}
	if *targetVersion == "" {
		return exitCodeError(2, "--target-version is required with --candidate")
	}
	result, err := applyCandidateBundle(status, candidateRoot, *targetVersion, *targetTag, attestation)
	if *jsonOut {
		if printErr := printJSON(result); printErr != nil && err == nil {
			err = printErr
		}
	} else {
		printUpdateApplyResult(result)
	}
	if err != nil {
		return err
	}
	if hasFailures(result.CandidateValidation) {
		return exitCodeError(4, "candidate bundle validation failed")
	}
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
		status.TargetVersion = state.TargetVersion
		status.TargetTag = state.TargetTag
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
		"targetVersion":            status.TargetVersion,
		"targetTag":                status.TargetTag,
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
	}
	if status.PluginVerificationStatus == "drift" {
		banners = append(banners, statusBanner{
			ID:       "codex_plugin_drift",
			Severity: "warn",
			Title:    "Codex plugin drift",
			Message:  "The visible slidex plugin or bundled skill path does not match this install root.",
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

func markPluginDrift(installRoot, pluginVersion, skillPath string) error {
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
	state.RestartRequired = true
	state.RestartReason = "Codex plugin verification found a visible plugin or skill path that does not match this install root"
	state.VerificationStatus = "drift"
	state.VerificationCommand = "slidex codex app-server plugin-smoke --json"
	state.PluginUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeUpdateState(installRoot, *state)
}

func stageArchiveCandidate(archivePath, checksumsPath, targetVersion, installRoot string) (string, error) {
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		return "", err
	}
	checksumText, err := os.ReadFile(checksumsPath)
	if err != nil {
		return "", err
	}
	if _, err := verifyReleaseAssetSHA256(filepath.Base(archivePath), payload, string(checksumText), ""); err != nil {
		return "", err
	}
	stageParent := filepath.Join(installRoot, ".slidex", "staged", targetVersion+"-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
		return "", err
	}
	return extractReleaseArchive(archivePath, stageParent)
}

func downloadAndStageReleaseCandidate(ctx context.Context, status updateStatus, apiURL, attestationPolicy string) (candidateRoot, targetVersion, targetTag string, attestation attestationVerification, err error) {
	releases, err := fetchUpdateReleases(ctx, apiURL)
	if err != nil {
		return "", "", "", attestationVerification{}, err
	}
	release, err := selectUpdateRelease(status.Channel, releases)
	if err != nil {
		return "", "", "", attestationVerification{}, err
	}
	contract, err := releaseAssetContractFor(release.TagName, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", "", "", attestationVerification{}, err
	}
	archive, checksum, err := release.requiredAssets(contract)
	if err != nil {
		return "", "", "", attestationVerification{}, err
	}
	archivePayload, err := downloadUpdateAsset(ctx, archive)
	if err != nil {
		return "", "", "", attestationVerification{}, err
	}
	checksumPayload, err := downloadUpdateAsset(ctx, checksum)
	if err != nil {
		return "", "", "", attestationVerification{}, err
	}
	candidateRoot, archivePath, err := stageDownloadedArchiveCandidate(status.InstallRoot, contract.Version, archive, archivePayload, checksum, checksumPayload)
	if err != nil {
		return "", "", "", attestationVerification{}, err
	}
	attestation, err = verifyReleaseAttestation(archivePath, release.TagName, attestationPolicy)
	if err != nil {
		return "", "", "", attestation, err
	}
	return candidateRoot, contract.Version, release.TagName, attestation, nil
}

func downloadUpdateAsset(ctx context.Context, asset updateAsset) ([]byte, error) {
	if asset.BrowserDownloadURL == "" {
		return nil, fmt.Errorf("release asset %s is missing browser_download_url", asset.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "slidex-update/"+toolVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download %s returned %s: %s", asset.Name, resp.Status, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

func stageDownloadedArchiveCandidate(installRoot, targetVersion string, archive updateAsset, archivePayload []byte, checksum updateAsset, checksumPayload []byte) (candidateRoot, archivePath string, err error) {
	if _, err := verifyReleaseAssetSHA256(archive.Name, archivePayload, string(checksumPayload), archive.Digest); err != nil {
		return "", "", err
	}
	stageParent := filepath.Join(installRoot, ".slidex", "downloads", targetVersion+"-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
		return "", "", err
	}
	archivePath = filepath.Join(stageParent, archive.Name)
	checksumPath := filepath.Join(stageParent, checksum.Name)
	if err := os.WriteFile(archivePath, archivePayload, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(checksumPath, checksumPayload, 0o644); err != nil {
		return "", "", err
	}
	extractRoot := filepath.Join(stageParent, "extract")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		return "", "", err
	}
	candidateRoot, err = extractReleaseArchive(archivePath, extractRoot)
	return candidateRoot, archivePath, err
}

func extractReleaseArchive(archivePath, dest string) (string, error) {
	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		if err := extractZipArchive(archivePath, dest); err != nil {
			return "", err
		}
	case strings.HasSuffix(archivePath, ".tar.gz"), strings.HasSuffix(archivePath, ".tgz"):
		if err := extractTarGzArchive(archivePath, dest); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported release archive format: %s", filepath.ToSlash(archivePath))
	}
	return singleExtractedRoot(dest), nil
}

func extractTarGzArchive(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean(header.Name))
		if !pathWithin(dest, target) {
			return fmt.Errorf("archive entry escapes extraction root: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeStreamFile(target, tr, os.FileMode(header.Mode)&0o777); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
	}
	return nil
}

func extractZipArchive(archivePath, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, file := range zr.File {
		target := filepath.Join(dest, filepath.Clean(file.Name))
		if !pathWithin(dest, target) {
			return fmt.Errorf("archive entry escapes extraction root: %s", file.Name)
		}
		mode := file.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported symlink in archive: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, mode&0o777); err != nil {
				return err
			}
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = rc.Close()
			return err
		}
		err = writeStreamFile(target, rc, mode&0o777)
		closeErr := rc.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func writeStreamFile(path string, r io.Reader, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func validateAttestationPolicy(policy string) error {
	switch policy {
	case attestationPolicyRequire, attestationPolicyAllowUnverified:
		return nil
	default:
		return exitCodeError(2, "--attestation-policy must be require or allow-unverified, got %q", policy)
	}
}

func verifyReleaseAttestation(archivePath, targetTag, policy string) (attestationVerification, error) {
	result := attestationVerification{Policy: policy}
	switch policy {
	case attestationPolicyAllowUnverified:
		result.Status = "skipped"
		result.Output = "attestation verification explicitly bypassed by --attestation-policy allow-unverified"
		return result, nil
	case attestationPolicyRequire:
		if targetTag == "" {
			result.Status = "fail"
			result.Error = "--target-tag is required when attestation verification is required"
			return result, errors.New(result.Error)
		}
		gh, err := exec.LookPath("gh")
		if err != nil {
			result.Status = "fail"
			result.Error = "GitHub CLI gh is required for release attestation verification"
			return result, errors.New(result.Error)
		}
		commands := [][]string{
			{gh, "release", "verify", targetTag, "--repo", updateGitHubRepo},
			{gh, "release", "verify-asset", targetTag, archivePath, "--repo", updateGitHubRepo},
			{gh, "attestation", "verify", archivePath, "--repo", updateGitHubRepo, "--cert-oidc-issuer", "https://token.actions.githubusercontent.com", "--cert-identity-regex", "^https://github.com/shiinamachi/slidex/.github/workflows/cross-platform.yml@refs/(heads/(main|develop)|tags/v[0-9].*)$"},
		}
		var outputs []string
		var commandStrings []string
		for _, args := range commands {
			commandStrings = append(commandStrings, shellQuoteCommand(args))
			out, err := runVerificationCommand(args, 45*time.Second)
			outputs = append(outputs, strings.TrimSpace(out))
			if err != nil {
				result.Status = "fail"
				result.Command = strings.Join(commandStrings, " && ")
				result.Output = truncateForJSON(strings.Join(outputs, "\n"), 6000)
				result.Error = err.Error()
				return result, fmt.Errorf("release attestation verification failed: %w", err)
			}
		}
		result.Status = "verified"
		result.Command = strings.Join(commandStrings, " && ")
		result.Output = truncateForJSON(strings.Join(outputs, "\n"), 6000)
		return result, nil
	default:
		err := validateAttestationPolicy(policy)
		result.Status = "fail"
		result.Error = err.Error()
		return result, err
	}
}

func runVerificationCommand(args []string, timeout time.Duration) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty verification command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(out), ctx.Err()
	}
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func shellQuoteCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		if strings.IndexFunc(arg, func(r rune) bool {
			return !(r == '/' || r == '.' || r == '-' || r == '_' || r == ':' || r == '=' || r == '+' || r == ',' || r == '@' || r == '^' || r == '$' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
		}) < 0 {
			quoted = append(quoted, arg)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
	}
	return strings.Join(quoted, " ")
}

func truncateForJSON(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func singleExtractedRoot(dest string) string {
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return dest
	}
	return filepath.Join(dest, entries[0].Name())
}

func applyCandidateBundle(status updateStatus, candidateRoot, targetVersion, targetTag string, attestation attestationVerification) (updateApplyResult, error) {
	result := updateApplyResult{
		ToolName:                toolName,
		CurrentVersion:          status.CurrentVersion,
		TargetVersion:           targetVersion,
		TargetTag:               targetTag,
		Channel:                 status.Channel,
		InstallRoot:             status.InstallRoot,
		Status:                  "candidate-invalid",
		NextVerificationCommand: "slidex codex app-server plugin-smoke --json",
		Attestation:             attestation,
	}
	result.CandidateValidation = validateCandidateBundle(candidateRoot, targetVersion)
	if metadata, err := readInstallMetadata(filepath.Join(candidateRoot, ".slidex", "install.json")); err == nil && metadata.Channel != status.Channel {
		result.CandidateValidation = append(result.CandidateValidation, fail("update.candidate_channel", "candidate channel must remain "+status.Channel+", got "+metadata.Channel, filepath.ToSlash(filepath.Join(candidateRoot, ".slidex", "install.json"))))
	}
	if hasFailures(result.CandidateValidation) {
		return result, nil
	}
	if runtime.GOOS == "windows" {
		stagedRoot, err := stageCandidateForWindowsHandoff(status.InstallRoot, candidateRoot, targetVersion)
		if err != nil {
			return result, err
		}
		pendingPath, err := writePendingUpdate(status.InstallRoot, stagedRoot, targetVersion, targetTag)
		if err != nil {
			return result, err
		}
		result.Status = "pending-restart"
		result.StagedRoot = filepath.ToSlash(stagedRoot)
		result.PendingUpdatePath = filepath.ToSlash(pendingPath)
		return result, nil
	}
	stagedRoot, backupRoot, err := replaceInstallRootWithCandidate(status.InstallRoot, candidateRoot, targetVersion)
	result.StagedRoot = filepath.ToSlash(stagedRoot)
	result.BackupRoot = filepath.ToSlash(backupRoot)
	if err != nil {
		result.Status = "rollback"
		return result, err
	}
	if err := updateInstallMetadataAfterActivation(status.InstallRoot, targetVersion, targetTag, status.Channel); err != nil {
		return result, err
	}
	if err := markPluginRestartRequired(status.InstallRoot, targetVersion, targetTag); err != nil {
		return result, err
	}
	result.Status = "applied"
	result.RestartRequired = true
	return result, nil
}

func stageCandidateForWindowsHandoff(installRoot, candidateRoot, targetVersion string) (string, error) {
	stagedRoot := filepath.Join(installRoot, ".slidex", "pending", targetVersion)
	_ = os.RemoveAll(stagedRoot)
	if err := copyDir(candidateRoot, stagedRoot); err != nil {
		return "", err
	}
	return stagedRoot, nil
}

func pendingUpdatePath(installRoot string) string {
	return filepath.Join(installRoot, ".slidex", "pending_update.json")
}

func writePendingUpdate(installRoot, stagedRoot, targetVersion, targetTag string) (string, error) {
	path := pendingUpdatePath(installRoot)
	pending := pendingUpdate{
		SchemaVersion: pendingUpdateSchemaVersion,
		ToolName:      toolName,
		TargetVersion: targetVersion,
		TargetTag:     targetTag,
		InstallRoot:   filepath.ToSlash(installRoot),
		StagedRoot:    filepath.ToSlash(stagedRoot),
		Reason:        "Windows may lock the running slidex executable; activate this staged bundle on next run.",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, writeSourceJSONFile(path, pending)
}

func replaceInstallRootWithCandidate(installRoot, candidateRoot, targetVersion string) (stagedRoot, backupRoot string, err error) {
	parent := filepath.Dir(filepath.Clean(installRoot))
	base := filepath.Base(filepath.Clean(installRoot))
	stamp := targetVersion + "-" + time.Now().UTC().Format("20060102T150405Z")
	stagedRoot = filepath.Join(parent, "."+base+".staged-"+stamp)
	backupRoot = filepath.Join(parent, "."+base+".backup-"+stamp)
	_ = os.RemoveAll(stagedRoot)
	if err := copyDir(candidateRoot, stagedRoot); err != nil {
		return stagedRoot, backupRoot, err
	}
	if err := os.Rename(installRoot, backupRoot); err != nil {
		_ = os.RemoveAll(stagedRoot)
		return stagedRoot, backupRoot, err
	}
	if err := os.Rename(stagedRoot, installRoot); err != nil {
		rollbackErr := os.Rename(backupRoot, installRoot)
		if rollbackErr != nil {
			return stagedRoot, backupRoot, fmt.Errorf("activation failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return stagedRoot, backupRoot, err
	}
	return stagedRoot, backupRoot, nil
}

func updateInstallMetadataAfterActivation(installRoot, targetVersion, targetTag, channel string) error {
	path := installMetadataPath(installRoot)
	metadata, err := readInstallMetadata(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if metadata == nil {
		metadata = &installMetadata{}
	}
	metadata.SchemaVersion = installMetadataSchemaVersion
	metadata.ToolName = toolName
	metadata.Version = targetVersion
	metadata.Channel = channel
	if targetTag != "" {
		metadata.Tag = targetTag
	}
	metadata.InstallRoot = filepath.ToSlash(installRoot)
	metadata.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	metadata.InstallMode = installModeReleasePackage
	if metadata.OS == "" {
		metadata.OS = runtime.GOOS
	}
	if metadata.Arch == "" {
		metadata.Arch = runtime.GOARCH
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeSourceJSONFile(path, metadata)
}

func printUpdateApplyResult(result updateApplyResult) {
	fmt.Printf("%s update apply %s\n", result.ToolName, result.Status)
	fmt.Printf("channel: %s\n", result.Channel)
	fmt.Printf("current version: %s\n", result.CurrentVersion)
	fmt.Printf("target version: %s\n", result.TargetVersion)
	fmt.Printf("install root: %s\n", result.InstallRoot)
	if result.BackupRoot != "" {
		fmt.Printf("backup root: %s\n", result.BackupRoot)
	}
	if result.PendingUpdatePath != "" {
		fmt.Printf("pending update: %s\n", result.PendingUpdatePath)
	}
	if result.Attestation.Policy != "" {
		fmt.Printf("attestation: %s (%s)\n", result.Attestation.Status, result.Attestation.Policy)
	}
	if result.RestartRequired {
		fmt.Println("restart required: restart Codex and start a new thread before treating updated plugin skills as active")
	}
	fmt.Printf("next verification: %s\n", result.NextVerificationCommand)
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
		".slidex/install.json",
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
	metadataPath := filepath.Join(root, ".slidex", "install.json")
	if metadata, err := readInstallMetadata(metadataPath); err != nil {
		findings = append(findings, fail("update.candidate_install_metadata", err.Error(), filepath.ToSlash(metadataPath)))
	} else {
		if metadata.Version != expectedVersion {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata version must be "+expectedVersion+", got "+metadata.Version, filepath.ToSlash(metadataPath)))
		}
		if metadata.Channel != updateChannelProduction && metadata.Channel != updateChannelCanary {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata channel must be production or canary, got "+metadata.Channel, filepath.ToSlash(metadataPath)))
		}
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
	binaryPath := filepath.Join(root, binary)
	if _, err := os.Stat(binaryPath); err != nil {
		findings = append(findings, fail("update.candidate_binary", "missing candidate CLI binary: "+err.Error(), filepath.ToSlash(binaryPath)))
	} else if version, err := candidateBinaryVersion(binaryPath); err != nil {
		findings = append(findings, fail("update.candidate_binary_version", "candidate CLI version command failed: "+err.Error(), filepath.ToSlash(binaryPath)))
	} else if version != expectedVersion {
		findings = append(findings, fail("update.candidate_binary_version", "candidate CLI version must be "+expectedVersion+", got "+version, filepath.ToSlash(binaryPath)))
	} else if doctorStatus, err := candidateDoctorStatus(root, binaryPath); err != nil {
		findings = append(findings, fail("update.candidate_doctor", "candidate doctor failed: "+err.Error(), filepath.ToSlash(binaryPath)))
	} else if doctorStatus != "pass" {
		findings = append(findings, fail("update.candidate_doctor", "candidate doctor status must be pass, got "+doctorStatus, filepath.ToSlash(binaryPath)))
	}
	return findings
}

func candidateBinaryVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return "", errors.New("empty version output")
	}
	return fields[len(fields)-1], nil
}

func candidateDoctorStatus(root, binaryPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "doctor", "--json")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		return "", err
	}
	return metadataString(report["status"]), nil
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
