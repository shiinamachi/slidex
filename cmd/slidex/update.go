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

	installMetadataSchemaFile   = "slidex_install_metadata.schema.json"
	updateStateSchemaFile       = "slidex_update_state.schema.json"
	pendingUpdateSchemaFile     = "slidex_pending_update.schema.json"
	updateStatusSchemaFile      = "slidex_update_status.schema.json"
	updateApplyResultSchemaFile = "slidex_update_apply_result.schema.json"

	updateInstallRootEnv     = "SLIDEX_INSTALL_ROOT"
	updateInstallMetadataEnv = "SLIDEX_INSTALL_METADATA"
	updateAPIURLEnv          = "SLIDEX_UPDATE_API_URL"
	updateAutoEnv            = "SLIDEX_AUTO_UPDATE"
)

var (
	stablePackageVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	canaryPackageVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-canary\.[0-9]{14}$`)
	gitCommitPattern            = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
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
	Raw              []byte `json:"-"`
}

type updateState struct {
	SchemaVersion          string `json:"schemaVersion"`
	ToolName               string `json:"toolName"`
	CurrentVersion         string `json:"currentVersion"`
	TargetVersion          string `json:"targetVersion,omitempty"`
	TargetTag              string `json:"targetTag,omitempty"`
	Channel                string `json:"channel"`
	RestartRequired        bool   `json:"restartRequired"`
	RestartReason          string `json:"restartReason,omitempty"`
	PluginUpdatedAt        string `json:"pluginUpdatedAt,omitempty"`
	VerificationStatus     string `json:"verificationStatus"`
	VerificationCommand    string `json:"verificationCommand"`
	VerifiedPluginVersion  string `json:"verifiedPluginVersion,omitempty"`
	VerifiedPluginPath     string `json:"verifiedPluginPath,omitempty"`
	VerifiedStartSkillPath string `json:"verifiedStartSkillPath,omitempty"`
	UpdatedAt              string `json:"updatedAt"`
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
	VerificationFindings      []qaFinding      `json:"verificationFindings,omitempty"`
	DiscoveredRelease         *updateRelease   `json:"discoveredRelease,omitempty"`
	CandidateValidation       []qaFinding      `json:"candidateValidation,omitempty"`
	InstalledMetadata         *installMetadata `json:"installedMetadata,omitempty"`
	PendingActivation         bool             `json:"pendingActivation,omitempty"`
	PendingActivationCommand  string           `json:"pendingActivationCommand,omitempty"`
	PendingUpdatePath         string           `json:"pendingUpdatePath,omitempty"`
	PendingUpdate             *pendingUpdate   `json:"pendingUpdate,omitempty"`
	PersistedRestartStatePath string           `json:"persistedRestartStatePath,omitempty"`
	VerifiedPluginVersion     string           `json:"verifiedPluginVersion,omitempty"`
	VerifiedPluginPath        string           `json:"verifiedPluginPath,omitempty"`
	VerifiedStartSkillPath    string           `json:"verifiedStartSkillPath,omitempty"`
}

type updateApplyResult struct {
	ToolName                 string      `json:"toolName"`
	CurrentVersion           string      `json:"currentVersion"`
	TargetVersion            string      `json:"targetVersion"`
	TargetTag                string      `json:"targetTag,omitempty"`
	Channel                  string      `json:"channel"`
	InstallRoot              string      `json:"installRoot"`
	Status                   string      `json:"status"`
	StagedRoot               string      `json:"stagedRoot,omitempty"`
	BackupRoot               string      `json:"backupRoot,omitempty"`
	PendingUpdatePath        string      `json:"pendingUpdatePath,omitempty"`
	RestartRequired          bool        `json:"restartRequired"`
	PluginVerificationStatus string      `json:"pluginVerificationStatus"`
	NextVerificationCommand  string      `json:"nextVerificationCommand"`
	CandidateValidation      []qaFinding `json:"candidateValidation,omitempty"`
	Error                    string      `json:"error,omitempty"`
}

type pendingUpdate struct {
	SchemaVersion     string `json:"schemaVersion"`
	ToolName          string `json:"toolName"`
	TargetVersion     string `json:"targetVersion"`
	TargetTag         string `json:"targetTag,omitempty"`
	InstallRoot       string `json:"installRoot"`
	StagedRoot        string `json:"stagedRoot"`
	ActivatorPath     string `json:"activatorPath,omitempty"`
	ActivationCommand string `json:"activationCommand,omitempty"`
	Reason            string `json:"reason"`
	CreatedAt         string `json:"createdAt"`
}

type statusBanner struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	Command  string `json:"command,omitempty"`
}

type updateRelease struct {
	TagName     string                 `json:"tagName"`
	Version     string                 `json:"version"`
	Prerelease  bool                   `json:"prerelease"`
	Draft       bool                   `json:"draft"`
	PublishedAt string                 `json:"publishedAt,omitempty"`
	CreatedAt   string                 `json:"createdAt,omitempty"`
	Assets      []updateAsset          `json:"assets"`
	Raw         map[string]any         `json:"-"`
	AssetByKey  map[string]updateAsset `json:"-"`
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
		return exitCodeError(2, "usage: slidex update status|check|apply|verify|activate-pending")
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
	case "activate-pending":
		return runUpdateActivatePending(args[1:])
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
	apiURL := fs.String("api-url", defaultUpdateAPIURL(), "GitHub releases API URL")
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
		release, err := selectUpdateReleaseForStatus(status, releases)
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
	apiURL := fs.String("api-url", defaultUpdateAPIURL(), "GitHub releases API URL")
	yes := fs.Bool("yes", false, "activate the staged update")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return exitCodeError(2, "usage: slidex update apply --yes [--json] [--install-root DIR] [--metadata FILE] [--api-url URL] [--target-version VERSION --candidate DIR | --target-version VERSION --archive FILE --checksums FILE [--target-tag TAG]]")
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
		err := exitCodeError(4, "updates are disabled for channel %s: %s", status.Channel, firstNonEmpty(status.Guidance, status.Reason))
		return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
	}
	candidateRoot := *candidate
	if *candidate == "" && *archive == "" {
		downloadedRoot, downloadedVersion, downloadedTag, err := downloadAndStageReleaseCandidate(context.Background(), status, *apiURL)
		if err != nil {
			if *targetVersion == "" {
				*targetVersion = downloadedVersion
			}
			if *targetTag == "" {
				*targetTag = downloadedTag
			}
			return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
		}
		candidateRoot = downloadedRoot
		if *targetVersion == "" {
			*targetVersion = downloadedVersion
		}
		if *targetTag == "" {
			*targetTag = downloadedTag
		}
	} else if *archive != "" {
		if *checksums == "" {
			err := exitCodeError(2, "--checksums is required with --archive")
			return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
		}
		if *targetVersion == "" {
			err := exitCodeError(2, "--target-version is required with --archive")
			return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
		}
		if err := verifyArchiveCandidateSHA256(*archive, *checksums); err != nil {
			return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
		}
		extracted, err := extractArchiveCandidate(*archive, *targetVersion, status.InstallRoot)
		if err != nil {
			return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
		}
		candidateRoot = extracted
	}
	if *targetVersion == "" {
		err := exitCodeError(2, "--target-version is required with --candidate")
		return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
	}
	if *targetTag == "" && strings.TrimSpace(candidateRoot) != "" {
		if metadata, err := readInstallMetadata(filepath.Join(candidateRoot, ".slidex", "install.json")); err == nil {
			*targetTag = metadata.Tag
		}
	}
	result, err := applyCandidateBundle(status, candidateRoot, *targetVersion, *targetTag)
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
	} else {
		status.VerificationFindings = updateVerificationFindings(status)
		if hasFailures(status.VerificationFindings) {
			status.Status = "verification-failed"
		} else if status.UpdatesEnabled {
			status.Status = "verified"
		}
	}
	if *jsonOut {
		if err := printJSON(status); err != nil {
			return err
		}
	} else {
		printUpdateStatus(status)
	}
	if hasFailures(status.CandidateValidation) {
		return exitCodeError(4, "candidate bundle validation failed")
	}
	if hasFailures(status.VerificationFindings) {
		return exitCodeError(4, "update verification failed")
	}
	return nil
}

func runUpdateActivatePending(args []string) error {
	fs := flag.NewFlagSet("update activate-pending", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "write JSON status")
	metadataPath := fs.String("metadata", "", "install metadata path")
	installRoot := fs.String("install-root", "", "install root")
	yes := fs.Bool("yes", false, "activate the pending staged update")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return exitCodeError(2, "usage: slidex update activate-pending --yes [--json] [--metadata FILE] [--install-root DIR]")
	}
	if !*yes {
		return exitCodeError(2, "slidex update activate-pending requires --yes before replacing the install root")
	}
	status, err := currentUpdateStatus(*installRoot, *metadataPath)
	if err != nil {
		return err
	}
	result, err := activatePendingUpdate(status)
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
		return exitCodeError(4, "pending candidate bundle validation failed")
	}
	return nil
}

func maybePrintUpdateApplyFailure(jsonOut bool, status updateStatus, targetVersion, targetTag string, err error) error {
	if !jsonOut {
		return err
	}
	result := updateApplyFailureResult(status, targetVersion, targetTag, err)
	if printErr := printJSON(result); printErr != nil {
		return printErr
	}
	return err
}

func updateApplyFailureResult(status updateStatus, targetVersion, targetTag string, err error) updateApplyResult {
	return updateApplyResult{
		ToolName:                 toolName,
		CurrentVersion:           status.CurrentVersion,
		TargetVersion:            targetVersion,
		TargetTag:                targetTag,
		Channel:                  status.Channel,
		InstallRoot:              status.InstallRoot,
		Status:                   "failed",
		RestartRequired:          status.RestartRequired,
		PluginVerificationStatus: status.PluginVerificationStatus,
		NextVerificationCommand:  firstNonEmpty(status.NextVerificationCommand, "slidex update verify --json"),
		Error:                    err.Error(),
	}
}

func updateVerificationFindings(status updateStatus) []qaFinding {
	if !status.UpdatesEnabled {
		return nil
	}
	var findings []qaFinding
	if status.PendingActivation {
		findings = append(findings, fail("update.pending_activation", "a staged update must be activated before post-restart plugin verification", status.PendingUpdatePath))
	}
	if status.RestartRequired {
		findings = append(findings, fail("update.restart_required", "Codex restart and post-restart plugin smoke are still required", status.PersistedRestartStatePath))
	}
	switch status.PluginVerificationStatus {
	case "verified":
	case "drift":
		findings = append(findings, fail("update.plugin_drift", "visible Codex plugin or bundled skill path does not match the active install root", status.PersistedRestartStatePath))
	default:
		findings = append(findings, fail("update.plugin_not_verified", "post-restart Codex plugin verification has not passed", status.PersistedRestartStatePath))
	}
	return findings
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
	if metadata != nil && (channel == updateChannelProduction || channel == updateChannelCanary) {
		if issue := installedReleaseMetadataIssue(metadata); issue != "" {
			channel = updateChannelLocalDevelopment
			mode = firstNonEmpty(metadata.InstallMode, installModeUnknown)
			reason = "install metadata is inconsistent; update is disabled fail-closed: " + issue
		}
	}
	state, statePath, stateErr := readUpdateState(installRoot)
	pending, pendingPath, pendingErr := readPendingUpdate(installRoot)
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
		if metadata.Version != "" {
			status.CurrentVersion = metadata.Version
		}
		if metadata.InstallRoot != "" && !sameFilesystemPath(metadata.InstallRoot, installRoot) {
			status.Reason = appendReason(status.Reason, "install metadata records a different install root "+filepath.ToSlash(metadata.InstallRoot)+"; using resolved install root "+filepath.ToSlash(installRoot))
		}
	}
	if !status.UpdatesEnabled {
		status.Status = "disabled"
		status.Guidance = "Automatic release updates are disabled for local-development installs. Install a production or canary release package to enable updates."
	}
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		status.RestartRequired = true
		status.PluginVerificationStatus = "restart_required"
		status.NextVerificationCommand = "slidex codex app-server plugin-smoke --json"
		status.Reason = appendReason(status.Reason, "update state is invalid and plugin verification must be repeated: "+stateErr.Error())
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
		status.VerifiedPluginVersion = state.VerifiedPluginVersion
		status.VerifiedPluginPath = state.VerifiedPluginPath
		status.VerifiedStartSkillPath = state.VerifiedStartSkillPath
	}
	if pendingErr != nil && !errors.Is(pendingErr, os.ErrNotExist) {
		status.Status = "pending-invalid"
		status.PendingUpdatePath = filepath.ToSlash(pendingPath)
		status.RestartRequired = true
		status.PluginVerificationStatus = "restart_required"
		status.NextVerificationCommand = "slidex update activate-pending --yes --json"
		status.Reason = appendReason(status.Reason, "pending update state is invalid and must be repaired before activation: "+pendingErr.Error())
	}
	if pending != nil {
		status.PendingActivation = true
		status.PendingUpdate = pending
		status.PendingUpdatePath = filepath.ToSlash(pendingPath)
		status.Status = "pending-activation"
		status.TargetVersion = pending.TargetVersion
		status.TargetTag = pending.TargetTag
		status.RestartRequired = true
		status.PluginVerificationStatus = "restart_required"
		status.PendingActivationCommand = firstNonEmpty(pending.ActivationCommand, pendingActivationCommand(filepath.FromSlash(pending.ActivatorPath), status.InstallRoot))
	}
	return status, nil
}

func appendReason(base, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	switch {
	case base == "":
		return extra
	case extra == "":
		return base
	default:
		return base + "; " + extra
	}
}

func sameFilesystemPath(left, right string) bool {
	left = filepath.Clean(filepath.FromSlash(strings.TrimSpace(left)))
	right = filepath.Clean(filepath.FromSlash(strings.TrimSpace(right)))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
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
		"pendingActivation":        status.PendingActivation,
		"pendingActivationCommand": status.PendingActivationCommand,
		"verifiedPluginVersion":    status.VerifiedPluginVersion,
		"verifiedPluginPath":       status.VerifiedPluginPath,
		"verifiedStartSkillPath":   status.VerifiedStartSkillPath,
		"banners":                  updateStatusBanners(status),
	}
}

func defaultUpdateAPIURL() string {
	if value := strings.TrimSpace(os.Getenv(updateAPIURLEnv)); value != "" {
		return value
	}
	return updateGitHubReleasesAPI
}

func automaticUpdatesAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(updateAutoEnv))) {
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return true
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
	if status.Status == "pending-invalid" {
		banners = append(banners, statusBanner{
			ID:       "pending_update_invalid",
			Severity: "warn",
			Title:    "Pending update invalid",
			Message:  "A staged slidex update could not be read and must be repaired before activation.",
			Command:  status.NextVerificationCommand,
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
	if status.PendingActivation {
		banners = append(banners, statusBanner{
			ID:       "pending_update_activation",
			Severity: "warn",
			Title:    "Pending update activation",
			Message:  "A slidex update is staged and still needs install-root activation.",
			Command:  status.PendingActivationCommand,
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

func doctorUpdateSchemaFindings() []qaFinding {
	now := time.Now().UTC().Format(time.RFC3339)
	installRoot := filepath.ToSlash(filepath.Join(mustAbs("."), "doctor-update-install"))
	pluginPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(installRoot), "plugins", "slidex"))
	skillPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(pluginPath), "skills", "slidex-start", "SKILL.md"))
	activatorBinary := "slidex"
	if runtime.GOOS == "windows" {
		activatorBinary = "slidex.exe"
	}
	contract, err := releaseAssetContractFor("v"+toolVersion, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return []qaFinding{fail("doctor.update_schema", err.Error(), "schemas")}
	}
	metadata := installMetadata{
		SchemaVersion:    installMetadataSchemaVersion,
		ToolName:         toolName,
		Version:          toolVersion,
		Channel:          updateChannelProduction,
		Tag:              "v" + toolVersion,
		Commit:           strings.Repeat("a", 40),
		BuildTime:        now,
		InstallRoot:      installRoot,
		ReleaseAssetName: contract.ArchiveName,
		InstalledAt:      now,
		InstallMode:      installModeReleasePackage,
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
	}
	pending := pendingUpdate{
		SchemaVersion:     pendingUpdateSchemaVersion,
		ToolName:          toolName,
		TargetVersion:     toolVersion,
		TargetTag:         "v" + toolVersion,
		InstallRoot:       installRoot,
		StagedRoot:        filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(installRoot)), ".slidex.staged-"+toolVersion)),
		ActivatorPath:     filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(installRoot)), ".slidex.activator-"+toolVersion, activatorBinary)),
		ActivationCommand: "slidex update activate-pending --yes --json",
		Reason:            "doctor schema contract sample",
		CreatedAt:         now,
	}
	state := updateState{
		SchemaVersion:          updateStateSchemaVersion,
		ToolName:               toolName,
		CurrentVersion:         toolVersion,
		TargetVersion:          toolVersion,
		TargetTag:              "v" + toolVersion,
		Channel:                updateChannelProduction,
		RestartRequired:        false,
		VerificationStatus:     "verified",
		VerificationCommand:    "slidex update verify --json",
		VerifiedPluginVersion:  toolVersion,
		VerifiedPluginPath:     pluginPath,
		VerifiedStartSkillPath: skillPath,
		UpdatedAt:              now,
	}
	status := updateStatus{
		ToolName:                 toolName,
		CurrentVersion:           toolVersion,
		Channel:                  updateChannelProduction,
		InstallMode:              installModeReleasePackage,
		InstallRoot:              installRoot,
		MetadataPath:             filepath.ToSlash(filepath.Join(filepath.FromSlash(installRoot), ".slidex", "install.json")),
		UpdatesEnabled:           true,
		Status:                   "verified",
		TargetVersion:            toolVersion,
		TargetTag:                "v" + toolVersion,
		RestartRequired:          false,
		PluginVerificationStatus: "verified",
		NextVerificationCommand:  "slidex update verify --json",
		InstalledMetadata:        &metadata,
		PendingActivation:        false,
		VerifiedPluginVersion:    toolVersion,
		VerifiedPluginPath:       pluginPath,
		VerifiedStartSkillPath:   skillPath,
	}
	result := updateApplyResult{
		ToolName:                 toolName,
		CurrentVersion:           toolVersion,
		TargetVersion:            toolVersion,
		TargetTag:                "v" + toolVersion,
		Channel:                  updateChannelProduction,
		InstallRoot:              installRoot,
		Status:                   "applied",
		StagedRoot:               filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(installRoot)), ".slidex.staged-"+toolVersion)),
		BackupRoot:               filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(installRoot)), ".slidex.backup-"+toolVersion)),
		RestartRequired:          true,
		PluginVerificationStatus: "restart_required",
		NextVerificationCommand:  "slidex codex app-server plugin-smoke --json",
	}
	samples := []struct {
		schema  string
		payload any
	}{
		{installMetadataSchemaFile, metadata},
		{updateStateSchemaFile, state},
		{pendingUpdateSchemaFile, pending},
		{updateStatusSchemaFile, status},
		{updateApplyResultSchemaFile, result},
	}
	var findings []qaFinding
	for _, sample := range samples {
		path := bundledSchemaPath(sample.schema)
		if err := validatePayloadAgainstSchema(sample.payload, path); err != nil {
			findings = append(findings, fail("doctor.update_schema", err.Error(), path))
		}
	}
	return findings
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

func markPluginVerified(installRoot, pluginVersion, pluginPath, skillPath string) error {
	if installRoot == "" {
		installRoot = defaultInstallRoot()
	}
	currentVersion, currentTag := updateStatePackageIdentity(installRoot)
	state, _, _ := readUpdateState(installRoot)
	if state == nil {
		state = &updateState{}
	}
	state.CurrentVersion = firstNonEmpty(currentVersion, toolVersion)
	if state.TargetVersion == "" {
		state.TargetVersion = state.CurrentVersion
	}
	if state.TargetTag == "" {
		state.TargetTag = currentTag
	}
	state.RestartRequired = false
	state.RestartReason = ""
	state.VerificationStatus = "verified"
	state.VerificationCommand = "slidex update verify --json"
	state.VerifiedPluginVersion = strings.TrimSpace(pluginVersion)
	state.VerifiedPluginPath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(pluginPath)))
	state.VerifiedStartSkillPath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(skillPath)))
	state.PluginUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeUpdateState(installRoot, *state)
}

func markPluginDrift(installRoot, pluginVersion, skillPath string) error {
	if installRoot == "" {
		installRoot = defaultInstallRoot()
	}
	currentVersion, currentTag := updateStatePackageIdentity(installRoot)
	state, _, _ := readUpdateState(installRoot)
	if state == nil {
		state = &updateState{}
	}
	state.CurrentVersion = firstNonEmpty(currentVersion, toolVersion)
	if state.TargetVersion == "" {
		state.TargetVersion = state.CurrentVersion
	}
	if state.TargetTag == "" {
		state.TargetTag = currentTag
	}
	state.RestartRequired = true
	state.RestartReason = "Codex plugin verification found a visible plugin or skill path that does not match this install root"
	state.VerificationStatus = "drift"
	state.VerificationCommand = "slidex codex app-server plugin-smoke --json"
	state.PluginUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeUpdateState(installRoot, *state)
}

func updateStatePackageIdentity(installRoot string) (version, tag string) {
	metadata, err := readInstallMetadata(installMetadataPath(installRoot))
	if err != nil || metadata == nil {
		return "", ""
	}
	return strings.TrimSpace(metadata.Version), strings.TrimSpace(metadata.Tag)
}

func verifyArchiveCandidateSHA256(archivePath, checksumsPath string) error {
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	checksumText, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}
	if _, err := verifyReleaseAssetSHA256(filepath.Base(archivePath), payload, string(checksumText), ""); err != nil {
		return err
	}
	return nil
}

func extractArchiveCandidate(archivePath, targetVersion, installRoot string) (string, error) {
	stageParent := filepath.Join(installRoot, ".slidex", "staged", targetVersion+"-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
		return "", err
	}
	return extractReleaseArchive(archivePath, stageParent)
}

func downloadAndStageReleaseCandidate(ctx context.Context, status updateStatus, apiURL string) (candidateRoot, targetVersion, targetTag string, err error) {
	releases, err := fetchUpdateReleases(ctx, apiURL)
	if err != nil {
		return "", "", "", err
	}
	release, err := selectUpdateReleaseForStatus(status, releases)
	if err != nil {
		return "", "", "", err
	}
	return downloadAndStageSelectedRelease(ctx, status, release)
}

func downloadAndStageSelectedRelease(ctx context.Context, status updateStatus, release updateRelease) (candidateRoot, targetVersion, targetTag string, err error) {
	contract, err := releaseAssetContractFor(release.TagName, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", "", "", err
	}
	archive, checksum, err := release.requiredAssets(contract)
	if err != nil {
		return "", "", "", err
	}
	archivePayload, err := downloadUpdateAsset(ctx, archive)
	if err != nil {
		return "", "", "", err
	}
	checksumPayload, err := downloadUpdateAsset(ctx, checksum)
	if err != nil {
		return "", "", "", err
	}
	stageParent, archivePath, err := stageDownloadedReleaseArchive(status.InstallRoot, contract.Version, archive, archivePayload, checksum, checksumPayload)
	if err != nil {
		return "", "", "", err
	}
	candidateRoot, err = extractDownloadedReleaseArchive(stageParent, archivePath)
	if err != nil {
		return "", "", "", err
	}
	return candidateRoot, contract.Version, release.TagName, nil
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

func stageDownloadedReleaseArchive(installRoot, targetVersion string, archive updateAsset, archivePayload []byte, checksum updateAsset, checksumPayload []byte) (stageParent, archivePath string, err error) {
	if normalizeGitHubSHA256Digest(archive.Digest) == "" {
		return "", "", fmt.Errorf("GitHub release asset digest is required for downloaded archive %s", archive.Name)
	}
	if _, err := verifyReleaseAssetSHA256(archive.Name, archivePayload, string(checksumPayload), archive.Digest); err != nil {
		return "", "", err
	}
	stageParent = filepath.Join(installRoot, ".slidex", "downloads", targetVersion+"-"+time.Now().UTC().Format("20060102T150405Z"))
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
	return stageParent, archivePath, nil
}

func extractDownloadedReleaseArchive(stageParent, archivePath string) (candidateRoot string, err error) {
	extractRoot := filepath.Join(stageParent, "extract")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		return "", err
	}
	return extractReleaseArchive(archivePath, extractRoot)
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

func pendingActivationCommand(activatorPath, installRoot string) string {
	command := "slidex"
	if strings.TrimSpace(activatorPath) != "" {
		command = filepath.ToSlash(activatorPath)
	}
	return shellQuoteCommand([]string{command, "update", "activate-pending", "--install-root", filepath.ToSlash(installRoot), "--yes", "--json"})
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

func applyCandidateBundle(status updateStatus, candidateRoot, targetVersion, targetTag string) (updateApplyResult, error) {
	result := updateApplyResult{
		ToolName:                 toolName,
		CurrentVersion:           status.CurrentVersion,
		TargetVersion:            targetVersion,
		TargetTag:                targetTag,
		Channel:                  status.Channel,
		InstallRoot:              status.InstallRoot,
		Status:                   "candidate-invalid",
		PluginVerificationStatus: status.PluginVerificationStatus,
		NextVerificationCommand:  "slidex codex app-server plugin-smoke --json",
	}
	result.CandidateValidation = validateCandidateBundle(candidateRoot, targetVersion)
	result.CandidateValidation = append(result.CandidateValidation, validateCandidateChannelForStatus(status.Channel, targetVersion, filepath.Join(candidateRoot, ".slidex", "install.json"))...)
	if metadata, err := readInstallMetadata(filepath.Join(candidateRoot, ".slidex", "install.json")); err == nil && metadata.Channel != status.Channel {
		result.CandidateValidation = append(result.CandidateValidation, fail("update.candidate_channel", "candidate channel must remain "+status.Channel+", got "+metadata.Channel, filepath.ToSlash(filepath.Join(candidateRoot, ".slidex", "install.json"))))
	}
	if hasFailures(result.CandidateValidation) {
		return result, nil
	}
	if runtime.GOOS == "windows" {
		stagedRoot, pendingPath, err := stagePendingUpdateHandoff(status.InstallRoot, candidateRoot, targetVersion, targetTag)
		if err != nil {
			return result, err
		}
		result.Status = "pending-restart"
		result.StagedRoot = filepath.ToSlash(stagedRoot)
		result.PendingUpdatePath = filepath.ToSlash(pendingPath)
		result.RestartRequired = true
		result.PluginVerificationStatus = "restart_required"
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
	result.PluginVerificationStatus = "restart_required"
	return result, nil
}

func validateCandidateChannelForStatus(statusChannel, targetVersion, evidencePath string) []qaFinding {
	targetChannel := channelFromPackageVersion(targetVersion)
	if targetChannel != updateChannelProduction && targetChannel != updateChannelCanary {
		return []qaFinding{fail("update.candidate_channel", "candidate target version must resolve to production or canary, got "+targetVersion, filepath.ToSlash(evidencePath))}
	}
	if targetChannel != statusChannel {
		return []qaFinding{fail("update.candidate_channel", "candidate target version channel must remain "+statusChannel+", got "+targetChannel+" from "+targetVersion, filepath.ToSlash(evidencePath))}
	}
	return nil
}

func stagePendingUpdateHandoff(installRoot, candidateRoot, targetVersion, targetTag string) (stagedRoot, pendingPath string, err error) {
	stagedRoot, err = stageCandidateForWindowsHandoff(installRoot, candidateRoot, targetVersion)
	if err != nil {
		return "", "", err
	}
	activatorPath, err := stagePendingActivator(installRoot, candidateRoot, targetVersion)
	if err != nil {
		return stagedRoot, "", err
	}
	pendingPath, err = writePendingUpdate(installRoot, stagedRoot, activatorPath, targetVersion, targetTag)
	if err != nil {
		return stagedRoot, "", err
	}
	if err := markPluginRestartRequired(installRoot, targetVersion, targetTag); err != nil {
		return stagedRoot, pendingPath, err
	}
	return stagedRoot, pendingPath, nil
}

func stageCandidateForWindowsHandoff(installRoot, candidateRoot, targetVersion string) (string, error) {
	return copyCandidateToSiblingStage(installRoot, candidateRoot, targetVersion, "pending")
}

func stagePendingActivator(installRoot, candidateRoot, targetVersion string) (string, error) {
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	source := filepath.Join(candidateRoot, binary)
	if _, err := os.Stat(source); err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.Clean(installRoot))
	base := filepath.Base(filepath.Clean(installRoot))
	stamp := targetVersion + "-" + time.Now().UTC().Format("20060102T150405Z")
	activatorRoot := filepath.Join(parent, "."+base+".activator-"+stamp)
	_ = os.RemoveAll(activatorRoot)
	if err := os.MkdirAll(activatorRoot, 0o755); err != nil {
		return "", err
	}
	destination := filepath.Join(activatorRoot, binary)
	if err := copyFile(source, destination); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destination, 0o755); err != nil {
			return "", err
		}
	}
	return destination, nil
}

func pendingUpdatePath(installRoot string) string {
	return filepath.Join(installRoot, ".slidex", "pending_update.json")
}

func readPendingUpdate(installRoot string) (*pendingUpdate, string, error) {
	path := pendingUpdatePath(installRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	var pending pendingUpdate
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, path, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	if err := validateRawJSONAgainstBundledSchema(raw, pendingUpdateSchemaFile); err != nil {
		return nil, path, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	if pending.SchemaVersion != "" && pending.SchemaVersion != pendingUpdateSchemaVersion {
		return nil, path, fmt.Errorf("%s: unsupported schemaVersion %q", filepath.ToSlash(path), pending.SchemaVersion)
	}
	if pending.ToolName != "" && pending.ToolName != toolName {
		return nil, path, fmt.Errorf("%s: toolName must be %s", filepath.ToSlash(path), toolName)
	}
	if err := validatePayloadAgainstBundledSchema(pending, pendingUpdateSchemaFile); err != nil {
		return nil, path, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	return &pending, path, nil
}

func writePendingUpdate(installRoot, stagedRoot, activatorPath, targetVersion, targetTag string) (string, error) {
	path := pendingUpdatePath(installRoot)
	pending := pendingUpdate{
		SchemaVersion:     pendingUpdateSchemaVersion,
		ToolName:          toolName,
		TargetVersion:     targetVersion,
		TargetTag:         targetTag,
		InstallRoot:       filepath.ToSlash(installRoot),
		StagedRoot:        filepath.ToSlash(stagedRoot),
		ActivatorPath:     filepath.ToSlash(activatorPath),
		ActivationCommand: pendingActivationCommand(activatorPath, installRoot),
		Reason:            "Windows may lock the running slidex executable; activate this staged bundle on next run.",
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := validatePayloadAgainstBundledSchema(pending, pendingUpdateSchemaFile); err != nil {
		return "", err
	}
	return path, writeSourceJSONFile(path, pending)
}

func activatePendingUpdate(status updateStatus) (updateApplyResult, error) {
	result := updateApplyResult{
		ToolName:                 toolName,
		CurrentVersion:           status.CurrentVersion,
		Channel:                  status.Channel,
		InstallRoot:              status.InstallRoot,
		Status:                   "pending-not-found",
		PluginVerificationStatus: status.PluginVerificationStatus,
		NextVerificationCommand:  "slidex codex app-server plugin-smoke --json",
	}
	pending := status.PendingUpdate
	if pending == nil {
		var err error
		pending, _, err = readPendingUpdate(status.InstallRoot)
		if err != nil {
			return result, fmt.Errorf("pending update handoff not found: %w", err)
		}
	}
	result.TargetVersion = pending.TargetVersion
	result.TargetTag = pending.TargetTag
	result.StagedRoot = filepath.ToSlash(pending.StagedRoot)
	if err := validatePendingUpdate(status.InstallRoot, pending); err != nil {
		result.Status = "pending-invalid"
		result.CandidateValidation = append(result.CandidateValidation, fail("update.pending_handoff", err.Error(), filepath.ToSlash(pendingUpdatePath(status.InstallRoot))))
		return result, nil
	}
	result.CandidateValidation = validateCandidateBundle(filepath.FromSlash(pending.StagedRoot), pending.TargetVersion)
	result.CandidateValidation = append(result.CandidateValidation, validateCandidateChannelForStatus(status.Channel, pending.TargetVersion, filepath.Join(filepath.FromSlash(pending.StagedRoot), ".slidex", "install.json"))...)
	if metadata, err := readInstallMetadata(filepath.Join(filepath.FromSlash(pending.StagedRoot), ".slidex", "install.json")); err == nil && metadata.Channel != status.Channel {
		result.CandidateValidation = append(result.CandidateValidation, fail("update.candidate_channel", "candidate channel must remain "+status.Channel+", got "+metadata.Channel, filepath.ToSlash(filepath.Join(filepath.FromSlash(pending.StagedRoot), ".slidex", "install.json"))))
	}
	if hasFailures(result.CandidateValidation) {
		result.Status = "candidate-invalid"
		return result, nil
	}
	backupRoot, err := activateStagedInstallRoot(status.InstallRoot, filepath.FromSlash(pending.StagedRoot), pending.TargetVersion)
	result.BackupRoot = filepath.ToSlash(backupRoot)
	if err != nil {
		result.Status = "rollback"
		return result, err
	}
	if err := updateInstallMetadataAfterActivation(status.InstallRoot, pending.TargetVersion, pending.TargetTag, status.Channel); err != nil {
		return result, err
	}
	if err := markPluginRestartRequired(status.InstallRoot, pending.TargetVersion, pending.TargetTag); err != nil {
		return result, err
	}
	result.Status = "applied"
	result.RestartRequired = true
	result.PluginVerificationStatus = "restart_required"
	return result, nil
}

func validatePendingUpdate(installRoot string, pending *pendingUpdate) error {
	if pending == nil {
		return errors.New("pending update is missing")
	}
	if pending.TargetVersion == "" {
		return errors.New("pending update targetVersion is required")
	}
	if strings.TrimSpace(pending.StagedRoot) == "" {
		return errors.New("pending update stagedRoot is required")
	}
	if strings.TrimSpace(pending.ActivatorPath) != "" {
		if _, err := os.Stat(filepath.FromSlash(pending.ActivatorPath)); err != nil {
			return fmt.Errorf("pending activator is unavailable: %w", err)
		}
	}
	if pending.InstallRoot != "" && filepath.Clean(filepath.FromSlash(pending.InstallRoot)) != filepath.Clean(installRoot) {
		return fmt.Errorf("pending update installRoot must be %s, got %s", filepath.ToSlash(installRoot), pending.InstallRoot)
	}
	if _, err := os.Stat(filepath.FromSlash(pending.StagedRoot)); err != nil {
		return fmt.Errorf("pending staged root is unavailable: %w", err)
	}
	return nil
}

func replaceInstallRootWithCandidate(installRoot, candidateRoot, targetVersion string) (stagedRoot, backupRoot string, err error) {
	stagedRoot, err = copyCandidateToSiblingStage(installRoot, candidateRoot, targetVersion, "staged")
	if err != nil {
		return stagedRoot, backupRoot, err
	}
	backupRoot, err = activateStagedInstallRoot(installRoot, stagedRoot, targetVersion)
	return stagedRoot, backupRoot, err
}

func copyCandidateToSiblingStage(installRoot, candidateRoot, targetVersion, kind string) (string, error) {
	parent := filepath.Dir(filepath.Clean(installRoot))
	base := filepath.Base(filepath.Clean(installRoot))
	stamp := targetVersion + "-" + time.Now().UTC().Format("20060102T150405Z")
	stagedRoot := filepath.Join(parent, "."+base+"."+kind+"-"+stamp)
	_ = os.RemoveAll(stagedRoot)
	if err := copyDir(candidateRoot, stagedRoot); err != nil {
		return stagedRoot, err
	}
	return stagedRoot, nil
}

func activateStagedInstallRoot(installRoot, stagedRoot, targetVersion string) (backupRoot string, err error) {
	parent := filepath.Dir(filepath.Clean(installRoot))
	base := filepath.Base(filepath.Clean(installRoot))
	stamp := targetVersion + "-" + time.Now().UTC().Format("20060102T150405Z")
	backupRoot = filepath.Join(parent, "."+base+".backup-"+stamp)
	if err := os.Rename(installRoot, backupRoot); err != nil {
		return backupRoot, err
	}
	if err := os.Rename(stagedRoot, installRoot); err != nil {
		rollbackErr := os.Rename(backupRoot, installRoot)
		if rollbackErr != nil {
			return backupRoot, fmt.Errorf("activation failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return backupRoot, err
	}
	return backupRoot, nil
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
	if err := validatePayloadAgainstBundledSchema(*metadata, installMetadataSchemaFile); err != nil {
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
	if result.PluginVerificationStatus != "" {
		fmt.Printf("plugin status: %s\n", result.PluginVerificationStatus)
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

func installedReleaseMetadataIssue(metadata *installMetadata) string {
	if metadata == nil {
		return ""
	}
	if metadata.SchemaVersion != installMetadataSchemaVersion {
		return "install metadata schemaVersion must be " + installMetadataSchemaVersion + ", got " + metadata.SchemaVersion
	}
	if metadata.ToolName != toolName {
		return "install metadata toolName must be " + toolName + ", got " + metadata.ToolName
	}
	if metadata.Version == "" {
		return "install metadata version is required"
	}
	versionChannel := channelFromPackageVersion(metadata.Version)
	if versionChannel != updateChannelProduction && versionChannel != updateChannelCanary {
		return "install metadata version must resolve to production or canary, got " + metadata.Version
	}
	if metadata.Channel != versionChannel {
		return "install metadata channel must match package version channel " + versionChannel + ", got " + metadata.Channel
	}
	if metadata.Tag == "" {
		return "install metadata tag is required"
	}
	tagVersion, err := releasePackageVersionFromTag(metadata.Tag)
	if err != nil || tagVersion != metadata.Version {
		return "install metadata tag must resolve to " + metadata.Version + ", got " + metadata.Tag
	}
	if metadata.Commit == "" {
		return "install metadata commit is required"
	}
	if !gitCommitPattern.MatchString(metadata.Commit) {
		return "install metadata commit must be a 7-40 character lowercase git SHA, got " + metadata.Commit
	}
	if metadata.BuildTime == "" {
		return "install metadata buildTime is required"
	}
	if _, err := time.Parse(time.RFC3339, metadata.BuildTime); err != nil {
		return "install metadata buildTime must be RFC3339: " + err.Error()
	}
	if metadata.ReleaseAssetName == "" {
		return "install metadata releaseAssetName is required"
	}
	contract, err := releaseAssetContractFor("v"+metadata.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err.Error()
	}
	if metadata.ReleaseAssetName != contract.ArchiveName {
		return "install metadata releaseAssetName must be " + contract.ArchiveName + ", got " + metadata.ReleaseAssetName
	}
	if metadata.InstallMode != installModeReleasePackage {
		return "install metadata installMode must be " + installModeReleasePackage + ", got " + metadata.InstallMode
	}
	if metadata.OS != runtime.GOOS {
		return "install metadata os must be " + runtime.GOOS + ", got " + metadata.OS
	}
	if metadata.Arch != runtime.GOARCH {
		return "install metadata arch must be " + runtime.GOARCH + ", got " + metadata.Arch
	}
	if err := validateInstallMetadataSchema(metadata); err != nil {
		return "install metadata schema validation failed: " + err.Error()
	}
	return ""
}

func validateInstallMetadataSchema(metadata *installMetadata) error {
	if metadata == nil {
		return errors.New("install metadata is missing")
	}
	if len(metadata.Raw) > 0 {
		return validateRawJSONAgainstBundledSchema(metadata.Raw, installMetadataSchemaFile)
	}
	return validatePayloadAgainstBundledSchema(*metadata, installMetadataSchemaFile)
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

func bundledSchemaPath(schemaName string) string {
	rel := filepath.Join("schemas", filepath.FromSlash(schemaName))
	if installRoot := defaultInstallRoot(); installRoot != "" {
		candidate := filepath.Join(installRoot, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return repoRelativePath(rel)
}

func validatePayloadAgainstBundledSchema(payload any, schemaName string) error {
	return validatePayloadAgainstSchema(payload, bundledSchemaPath(schemaName))
}

func validateRawJSONAgainstBundledSchema(raw []byte, schemaName string) error {
	return validateRawJSONAgainstSchema(raw, bundledSchemaPath(schemaName))
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
	metadata.Raw = append([]byte(nil), raw...)
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
	if err := validateRawJSONAgainstBundledSchema(raw, updateStateSchemaFile); err != nil {
		return nil, path, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	if state.SchemaVersion != updateStateSchemaVersion {
		return nil, path, fmt.Errorf("%s: schemaVersion must be %s, got %q", filepath.ToSlash(path), updateStateSchemaVersion, state.SchemaVersion)
	}
	if state.ToolName != toolName {
		return nil, path, fmt.Errorf("%s: toolName must be %s, got %q", filepath.ToSlash(path), toolName, state.ToolName)
	}
	switch state.VerificationStatus {
	case "restart_required", "verified", "drift":
	default:
		return nil, path, fmt.Errorf("%s: unsupported verificationStatus %q", filepath.ToSlash(path), state.VerificationStatus)
	}
	if state.TargetVersion == "" {
		return nil, path, fmt.Errorf("%s: targetVersion is required", filepath.ToSlash(path))
	}
	if state.VerificationStatus == "verified" {
		if err := validateVerifiedUpdateState(installRoot, path, state); err != nil {
			return nil, path, err
		}
	}
	if state.UpdatedAt == "" {
		return nil, path, fmt.Errorf("%s: updatedAt is required", filepath.ToSlash(path))
	}
	if _, err := time.Parse(time.RFC3339, state.UpdatedAt); err != nil {
		return nil, path, fmt.Errorf("%s: updatedAt must be RFC3339: %w", filepath.ToSlash(path), err)
	}
	return &state, path, nil
}

func validateVerifiedUpdateState(installRoot, path string, state updateState) error {
	path = filepath.ToSlash(path)
	if pluginVersionBase(state.VerifiedPluginVersion) != toolVersion {
		return fmt.Errorf("%s: verifiedPluginVersion must match current slidex version %s, got %q", path, toolVersion, state.VerifiedPluginVersion)
	}
	pluginRoot := filepath.Join(filepath.Clean(installRoot), "plugins", "slidex")
	pluginPath := filepath.Clean(filepath.FromSlash(state.VerifiedPluginPath))
	if strings.TrimSpace(state.VerifiedPluginPath) == "" {
		return fmt.Errorf("%s: verifiedPluginPath is required", path)
	}
	if !filepath.IsAbs(pluginPath) {
		return fmt.Errorf("%s: verifiedPluginPath must be absolute, got %q", path, state.VerifiedPluginPath)
	}
	if !pathWithin(pluginRoot, pluginPath) {
		return fmt.Errorf("%s: verifiedPluginPath must be under %s, got %s", path, filepath.ToSlash(pluginRoot), filepath.ToSlash(pluginPath))
	}
	skillPath := filepath.Clean(filepath.FromSlash(state.VerifiedStartSkillPath))
	if strings.TrimSpace(state.VerifiedStartSkillPath) == "" {
		return fmt.Errorf("%s: verifiedStartSkillPath is required", path)
	}
	if !filepath.IsAbs(skillPath) {
		return fmt.Errorf("%s: verifiedStartSkillPath must be absolute, got %q", path, state.VerifiedStartSkillPath)
	}
	if !strings.HasSuffix(filepath.ToSlash(skillPath), "skills/slidex-start/SKILL.md") {
		return fmt.Errorf("%s: verifiedStartSkillPath must end with skills/slidex-start/SKILL.md, got %s", path, filepath.ToSlash(skillPath))
	}
	if status := postRestartSkillPathStatus(pluginRoot, skillPath, state.VerifiedPluginVersion); status != "verified" {
		return fmt.Errorf("%s: verifiedStartSkillPath must be under %s or a matching Codex plugin cache, got %s", path, filepath.ToSlash(pluginRoot), filepath.ToSlash(skillPath))
	}
	return nil
}

func writeUpdateState(installRoot string, state updateState) error {
	path := updateStatePath(installRoot)
	state.SchemaVersion = updateStateSchemaVersion
	state.ToolName = toolName
	if state.CurrentVersion == "" {
		state.CurrentVersion = toolVersion
	}
	if state.Channel == "" {
		state.Channel = channelFromPackageVersion(firstNonEmpty(state.TargetVersion, state.CurrentVersion))
	}
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
	if err := validatePayloadAgainstBundledSchema(state, updateStateSchemaFile); err != nil {
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

func releaseBaseVersion(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if canaryPackageVersionPattern.MatchString(version) {
		return strings.SplitN(version, "-", 2)[0]
	}
	return version
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
			TagName:     tag,
			Version:     version,
			Prerelease:  metadataBool(value["prerelease"]),
			Draft:       metadataBool(value["draft"]),
			PublishedAt: metadataString(value["published_at"]),
			CreatedAt:   metadataString(value["created_at"]),
			Raw:         value,
			AssetByKey:  map[string]updateAsset{},
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
	return selectUpdateReleaseForCurrent(channel, "", releases)
}

func selectUpdateReleaseForStatus(status updateStatus, releases []updateRelease) (updateRelease, error) {
	return selectUpdateReleaseForCurrent(status.Channel, status.CurrentVersion, releases)
}

func selectUpdateReleaseForCurrent(channel, currentVersion string, releases []updateRelease) (updateRelease, error) {
	currentVersion = strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")
	if channel == updateChannelCanary && canaryPackageVersionPattern.MatchString(currentVersion) {
		return selectCanaryUpdateRelease(currentVersion, releases)
	}
	var candidates []updateRelease
	for _, release := range releases {
		if release.Draft {
			continue
		}
		switch channel {
		case updateChannelProduction:
			if !release.Prerelease && channelFromPackageVersion(release.Version) == updateChannelProduction && releaseIsNotOlder(release.Version, currentVersion) {
				candidates = append(candidates, release)
			}
		case updateChannelCanary:
			if release.Prerelease && channelFromPackageVersion(release.Version) == updateChannelCanary && canaryReleaseIsNotOlder(release.Version, currentVersion) {
				candidates = append(candidates, release)
			}
		default:
			return updateRelease{}, fmt.Errorf("updates are disabled for channel %q", channel)
		}
	}
	if len(candidates) > 0 {
		release, err := bestUpdateRelease(candidates)
		if err != nil {
			return updateRelease{}, err
		}
		return release, nil
	}
	return updateRelease{}, fmt.Errorf("no matching %s release found", channel)
}

func selectCanaryUpdateRelease(currentVersion string, releases []updateRelease) (updateRelease, error) {
	currentBase := releaseBaseVersion(currentVersion)
	var candidates []updateRelease
	for _, release := range releases {
		if release.Draft || !release.Prerelease || channelFromPackageVersion(release.Version) != updateChannelCanary {
			continue
		}
		cmp, ok := compareReleaseBaseVersions(releaseBaseVersion(release.Version), currentBase)
		if !ok {
			continue
		}
		if cmp < 0 {
			continue
		}
		if canaryReleaseIsNotOlder(release.Version, currentVersion) {
			candidates = append(candidates, release)
		}
	}
	if len(candidates) == 0 {
		return updateRelease{}, fmt.Errorf("no matching %s release found", updateChannelCanary)
	}
	release, err := bestUpdateRelease(candidates)
	if err != nil {
		return updateRelease{}, err
	}
	_, ok := compareReleaseBaseVersions(releaseBaseVersion(release.Version), currentBase)
	if !ok {
		return updateRelease{}, fmt.Errorf("no matching %s release found", updateChannelCanary)
	}
	return release, nil
}

func releaseIsNotOlder(candidateVersion, currentVersion string) bool {
	if currentVersion == "" {
		return true
	}
	cmp, ok := compareReleaseBaseVersions(releaseBaseVersion(candidateVersion), releaseBaseVersion(currentVersion))
	return ok && cmp >= 0
}

func canaryReleaseIsNotOlder(candidateVersion, currentVersion string) bool {
	if currentVersion == "" {
		return true
	}
	cmp, ok := compareReleaseBaseVersions(releaseBaseVersion(candidateVersion), releaseBaseVersion(currentVersion))
	if !ok || cmp < 0 {
		return false
	}
	if cmp > 0 || candidateVersion == currentVersion {
		return true
	}
	candidateTimestamp, candidateOK := canaryVersionTimestamp(candidateVersion)
	currentTimestamp, currentOK := canaryVersionTimestamp(currentVersion)
	return candidateOK && currentOK && candidateTimestamp >= currentTimestamp
}

func compareReleaseBaseVersions(left, right string) (int, bool) {
	leftParts, ok := parseReleaseBaseVersionParts(left)
	if !ok {
		return 0, false
	}
	rightParts, ok := parseReleaseBaseVersionParts(right)
	if !ok {
		return 0, false
	}
	for i := range leftParts {
		if leftParts[i] > rightParts[i] {
			return 1, true
		}
		if leftParts[i] < rightParts[i] {
			return -1, true
		}
	}
	return 0, true
}

func bestUpdateRelease(candidates []updateRelease) (updateRelease, error) {
	if len(candidates) == 0 {
		return updateRelease{}, errors.New("no update release candidates")
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		cmp, ok := compareUpdateReleaseRecency(candidate, best)
		if !ok {
			return updateRelease{}, fmt.Errorf("release metadata does not determine ordering between %s and %s", candidate.TagName, best.TagName)
		}
		if cmp > 0 {
			best = candidate
		}
	}
	return best, nil
}

func compareUpdateReleaseRecency(left, right updateRelease) (int, bool) {
	cmp, ok := compareReleaseBaseVersions(releaseBaseVersion(left.Version), releaseBaseVersion(right.Version))
	if !ok || cmp != 0 {
		return cmp, ok
	}
	if left.Version == right.Version {
		return 0, true
	}
	leftCanaryTimestamp, leftCanaryOK := canaryVersionTimestamp(left.Version)
	rightCanaryTimestamp, rightCanaryOK := canaryVersionTimestamp(right.Version)
	if leftCanaryOK && rightCanaryOK {
		return strings.Compare(leftCanaryTimestamp, rightCanaryTimestamp), true
	}
	leftTime, leftOK := releaseMetadataTime(left)
	rightTime, rightOK := releaseMetadataTime(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	if leftTime.After(rightTime) {
		return 1, true
	}
	if leftTime.Before(rightTime) {
		return -1, true
	}
	return strings.Compare(left.Version, right.Version), true
}

func releaseMetadataTime(release updateRelease) (time.Time, bool) {
	for _, raw := range []string{release.PublishedAt, release.CreatedAt} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func canaryVersionTimestamp(version string) (string, bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !canaryPackageVersionPattern.MatchString(version) {
		return "", false
	}
	_, timestamp, ok := strings.Cut(version, "-canary.")
	return timestamp, ok && len(timestamp) == 14
}

func parseReleaseBaseVersionParts(version string) ([3]int, bool) {
	var parts [3]int
	version = releaseBaseVersion(version)
	if !stablePackageVersionPattern.MatchString(version) {
		return parts, false
	}
	for i, part := range strings.Split(version, ".") {
		var value int
		for _, r := range part {
			value = value*10 + int(r-'0')
		}
		parts[i] = value
	}
	return parts, true
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
	expectedVersion = strings.TrimPrefix(strings.TrimSpace(expectedVersion), "v")
	expectedBaseVersion := releaseBaseVersion(expectedVersion)
	expectedChannel := channelFromPackageVersion(expectedVersion)
	var findings []qaFinding
	required := []string{
		".agents/plugins/marketplace.json",
		".agents/skills/slidex",
		".mise.toml",
		"CODEX_INSTALL_PROMPT.md",
		"INSTALL.md",
		"LICENSE",
		"README.ko.md",
		"README.md",
		"VERSIONING.md",
		"VERSION",
		".slidex/install.json",
		"commands.md",
		"decks/README.md",
		"decks/_template",
		"examples/sample_deck_spec.json",
		"go.mod",
		"go.sum",
		"internal/codex/protocol",
		"plugins/slidex",
		"schemas",
		"slidex.toml",
		"plugins/slidex/.codex-plugin/plugin.json",
		"plugins/slidex/.codex-plugin/version-lock.json",
	}
	for _, rel := range required {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err != nil {
			findings = append(findings, fail("update.candidate_runtime", "missing candidate runtime path: "+err.Error(), filepath.ToSlash(path)))
		}
	}
	version := strings.TrimSpace(readFileOrEmpty(filepath.Join(root, "VERSION")))
	if version != expectedBaseVersion {
		findings = append(findings, fail("update.candidate_version", fmt.Sprintf("candidate VERSION must be %s, got %s", expectedBaseVersion, firstNonEmpty(version, "missing")), filepath.ToSlash(filepath.Join(root, "VERSION"))))
	}
	metadataPath := filepath.Join(root, ".slidex", "install.json")
	if metadata, err := readInstallMetadata(metadataPath); err != nil {
		findings = append(findings, fail("update.candidate_install_metadata", err.Error(), filepath.ToSlash(metadataPath)))
	} else {
		if err := validateInstallMetadataSchema(metadata); err != nil {
			findings = append(findings, fail("update.candidate_install_metadata", err.Error(), filepath.ToSlash(metadataPath)))
		}
		if metadata.Version != expectedVersion {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata version must be "+expectedVersion+", got "+metadata.Version, filepath.ToSlash(metadataPath)))
		}
		if metadata.Channel != updateChannelProduction && metadata.Channel != updateChannelCanary {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata channel must be production or canary, got "+metadata.Channel, filepath.ToSlash(metadataPath)))
		} else if metadata.Channel != expectedChannel {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata channel must match target version channel "+expectedChannel+", got "+metadata.Channel, filepath.ToSlash(metadataPath)))
		}
		if metadata.Tag == "" {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata tag is required", filepath.ToSlash(metadataPath)))
		} else if tagVersion, err := releasePackageVersionFromTag(metadata.Tag); err != nil || tagVersion != expectedVersion {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata tag must resolve to "+expectedVersion+", got "+metadata.Tag, filepath.ToSlash(metadataPath)))
		}
		if metadata.Commit == "" {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata commit is required", filepath.ToSlash(metadataPath)))
		}
		if metadata.BuildTime == "" {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata buildTime is required", filepath.ToSlash(metadataPath)))
		}
		if metadata.InstallMode != installModeReleasePackage {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata installMode must be "+installModeReleasePackage+", got "+metadata.InstallMode, filepath.ToSlash(metadataPath)))
		}
		expectedAsset, err := releaseAssetContractFor("v"+expectedVersion, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			findings = append(findings, fail("update.candidate_install_metadata", err.Error(), filepath.ToSlash(metadataPath)))
		} else if metadata.ReleaseAssetName != expectedAsset.ArchiveName {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata releaseAssetName must be "+expectedAsset.ArchiveName+", got "+metadata.ReleaseAssetName, filepath.ToSlash(metadataPath)))
		}
		if metadata.OS != runtime.GOOS {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata os must be "+runtime.GOOS+", got "+metadata.OS, filepath.ToSlash(metadataPath)))
		}
		if metadata.Arch != runtime.GOARCH {
			findings = append(findings, fail("update.candidate_install_metadata", "install metadata arch must be "+runtime.GOARCH+", got "+metadata.Arch, filepath.ToSlash(metadataPath)))
		}
	}
	manifestPath := filepath.Join(root, "plugins", "slidex", ".codex-plugin", "plugin.json")
	if manifest, err := readCandidateJSON(manifestPath); err != nil {
		findings = append(findings, fail("update.candidate_plugin_manifest", err.Error(), filepath.ToSlash(manifestPath)))
	} else {
		if got := metadataString(manifest["name"]); got != toolName {
			findings = append(findings, fail("update.candidate_plugin_manifest", "plugin manifest name must be "+toolName, filepath.ToSlash(manifestPath)))
		}
		if got := pluginVersionBase(metadataString(manifest["version"])); got != expectedBaseVersion {
			findings = append(findings, fail("update.candidate_plugin_manifest", "plugin manifest version base must be "+expectedBaseVersion+", got "+got, filepath.ToSlash(manifestPath)))
		}
	}
	lockPath := filepath.Join(root, "plugins", "slidex", ".codex-plugin", "version-lock.json")
	if lock, err := readCandidateJSON(lockPath); err != nil {
		findings = append(findings, fail("update.candidate_version_lock", err.Error(), filepath.ToSlash(lockPath)))
	} else {
		for _, key := range []string{"pluginVersion", "slidexCliVersion"} {
			if got := metadataString(lock[key]); got != expectedBaseVersion {
				findings = append(findings, fail("update.candidate_version_lock", key+" must be "+expectedBaseVersion+", got "+got, filepath.ToSlash(lockPath)))
			}
		}
		if got := metadataString(lock["requiredCodexCliVersion"]); got != requiredCodexVersion {
			findings = append(findings, fail("update.candidate_version_lock", "requiredCodexCliVersion must be "+requiredCodexVersion+", got "+got, filepath.ToSlash(lockPath)))
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
	} else if version != expectedBaseVersion {
		findings = append(findings, fail("update.candidate_binary_version", "candidate CLI version must be "+expectedBaseVersion+", got "+version, filepath.ToSlash(binaryPath)))
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
	fmt.Printf("plugin status: %s\n", status.PluginVerificationStatus)
	if status.RestartRequired {
		fmt.Println("restart required: restart Codex and start a new thread before treating updated plugin skills as active")
	}
	if status.PendingActivation {
		fmt.Printf("pending activation: %s\n", status.PendingActivationCommand)
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
