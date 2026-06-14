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
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
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
	updateGitHubReleasesPerPage  = 100

	installMetadataSchemaFile   = "slidex_install_metadata.schema.json"
	updateStateSchemaFile       = "slidex_update_state.schema.json"
	pendingUpdateSchemaFile     = "slidex_pending_update.schema.json"
	updateStatusSchemaFile      = "slidex_update_status.schema.json"
	updateApplyResultSchemaFile = "slidex_update_apply_result.schema.json"

	updateInstallRootEnv     = "SLIDEX_INSTALL_ROOT"
	updateInstallMetadataEnv = "SLIDEX_INSTALL_METADATA"
	updateAPIURLEnv          = "SLIDEX_UPDATE_API_URL"
	updateAutoEnv            = "SLIDEX_AUTO_UPDATE"
	updateInstallLockSchema  = "slidex.updateLock.v1"

	maxUpdateArchiveCompressedBytes = int64(512 << 20)
	maxUpdateChecksumBytes          = int64(4 << 20)
	maxUpdateArchiveEntries         = 20000
	maxUpdateArchiveFileBytes       = int64(256 << 20)
	maxUpdateArchiveExpandedBytes   = int64(1024 << 20)
	maxUpdateZipCentralDirBytes     = int64(64 << 20)
	maxUpdateMetadataBytes          = int64(1 << 20)
	maxUpdateReleasePages           = 20
	maxUpdateCandidateJSONBytes     = int64(4 << 20)
	maxUpdateVersionBytes           = int64(64 << 10)
	maxReleasePackageVersionLength  = 96
	updateReleaseFetchTimeout       = 30 * time.Second
	updateAssetDownloadTimeout      = 2 * time.Minute
	updateHTTPClientTimeout         = 2 * time.Minute

	zipEndOfCentralDirectorySignature       = uint32(0x06054b50)
	zipCentralDirectoryHeaderSignature      = uint32(0x02014b50)
	zip64EndOfCentralDirectorySignature     = uint32(0x06064b50)
	zip64EndOfCentralDirectoryLocSignature  = uint32(0x07064b50)
	zipCentralDirectoryHeaderMinSize        = 46
	zipEndOfCentralDirectoryMinSize         = 22
	zipMaxCommentSize                       = 65535
	zip64EndOfCentralDirectoryLocatorSize   = 20
	zip64EndOfCentralDirectoryMinRecordSize = 56
)

var (
	stablePackageVersionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	canaryPackageVersionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-canary\.[0-9]{14}$`)
	gitCommitPattern             = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	updateHTTPClient             = &http.Client{Timeout: updateHTTPClientTimeout}
	updateInstallLockWaitTimeout = 30 * time.Second
	updateInstallLockRetryDelay  = 100 * time.Millisecond
	updateInstallLockStaleAfter  = 30 * time.Minute
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
	SchemaVersion            string `json:"schemaVersion"`
	ToolName                 string `json:"toolName"`
	TargetVersion            string `json:"targetVersion"`
	TargetTag                string `json:"targetTag,omitempty"`
	InstallRoot              string `json:"installRoot"`
	StagedRoot               string `json:"stagedRoot"`
	StagedRootManifestSHA256 string `json:"stagedRootManifestSha256"`
	ActivatorPath            string `json:"activatorPath,omitempty"`
	ActivatorSHA256          string `json:"activatorSha256"`
	ActivationCommand        string `json:"activationCommand,omitempty"`
	Reason                   string `json:"reason"`
	CreatedAt                string `json:"createdAt"`
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

func resolveUpdateInstallRoot(installRootArg string) string {
	installRoot := strings.TrimSpace(installRootArg)
	if installRoot == "" {
		installRoot = strings.TrimSpace(os.Getenv(updateInstallRootEnv))
	}
	if installRoot == "" {
		installRoot = defaultInstallRoot()
	}
	return filepath.Clean(installRoot)
}

func canonicalUpdateInstallRoot(installRoot string) (string, error) {
	root := filepath.Clean(strings.TrimSpace(installRoot))
	if root == "" || root == "." {
		root = defaultInstallRoot()
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	} else if os.IsNotExist(err) {
		parent := filepath.Dir(root)
		if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
			root = filepath.Join(resolvedParent, filepath.Base(root))
		} else {
			return "", parentErr
		}
	} else {
		return "", err
	}
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root, nil
}

func resolveUpdateMetadataPath(installRoot, metadataPathArg string) string {
	metadataPath := strings.TrimSpace(metadataPathArg)
	if metadataPath == "" {
		metadataPath = strings.TrimSpace(os.Getenv(updateInstallMetadataEnv))
	}
	if metadataPath == "" {
		metadataPath = installMetadataPath(installRoot)
	}
	return metadataPath
}

func canonicalUpdateMetadataPath(path string) (string, error) {
	path = filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	if path == "" || path == "." {
		return "", errors.New("install metadata path is required")
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if os.IsNotExist(err) {
		parent := filepath.Dir(path)
		suffix := filepath.Base(path)
		for {
			if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
				path = filepath.Join(resolvedParent, suffix)
				break
			} else if !os.IsNotExist(parentErr) {
				return "", parentErr
			}
			nextParent := filepath.Dir(parent)
			if nextParent == parent {
				return "", err
			}
			suffix = filepath.Join(filepath.Base(parent), suffix)
			parent = nextParent
		}
	} else {
		return "", err
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path, nil
}

func acquireUpdateInstallLock(installRoot string) (func(), error) {
	_, unlock, err := acquireCanonicalUpdateInstallLock(installRoot)
	return unlock, err
}

func lockResolvedUpdateInstallRoot(installRoot string) (string, func(), error) {
	resolvedRoot := resolveUpdateInstallRoot(installRoot)
	canonicalRoot, unlock, err := acquireCanonicalUpdateInstallLock(resolvedRoot)
	if err != nil {
		return "", nil, err
	}
	return canonicalRoot, unlock, nil
}

func acquireCanonicalUpdateInstallLock(installRoot string) (string, func(), error) {
	canonicalRoot, err := canonicalUpdateInstallRoot(installRoot)
	if err != nil {
		return "", nil, err
	}
	lockPath, err := updateInstallLockPathForCanonicalRoot(canonicalRoot)
	if err != nil {
		return "", nil, err
	}
	deadline := time.Now().Add(updateInstallLockWaitTimeout)
	for {
		if err := rejectSecureWriteTarget(lockPath); err != nil {
			return "", nil, err
		}
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if err := applyPlatformFileMode(lockPath, 0o600); err != nil {
				_ = f.Close()
				_ = os.Remove(lockPath)
				return "", nil, err
			}
			_, _ = fmt.Fprintf(f, "schema=%s pid=%d nonce=%s installRoot=%s acquired=%s\n", updateInstallLockSchema, os.Getpid(), newLockNonce(), filepath.ToSlash(canonicalRoot), time.Now().UTC().Format(time.RFC3339))
			return canonicalRoot, func() {
				releaseLockFile(lockPath, f)
			}, nil
		}
		if !os.IsExist(err) {
			return "", nil, err
		}
		if reclaimStaleLockFile(lockPath, maxUpdateMetadataBytes, staleUpdateInstallLockSnapshot) {
			continue
		}
		now := time.Now()
		if updateInstallLockWaitTimeout <= 0 || !deadline.After(now) {
			return "", nil, fmt.Errorf("update install lock %s is still held after %s", filepath.ToSlash(lockPath), updateInstallLockWaitTimeout)
		}
		sleepFor := updateInstallLockRetryDelay
		if sleepFor <= 0 {
			sleepFor = 100 * time.Millisecond
		}
		if remaining := deadline.Sub(now); remaining < sleepFor {
			sleepFor = remaining
		}
		time.Sleep(sleepFor)
	}
}

func updateInstallLockPath(installRoot string) (string, error) {
	canonicalRoot, err := canonicalUpdateInstallRoot(installRoot)
	if err != nil {
		return "", err
	}
	return updateInstallLockPathForCanonicalRoot(canonicalRoot)
}

func updateInstallLockPathForCanonicalRoot(canonicalRoot string) (string, error) {
	lockDir := filepath.Join(filepath.Dir(canonicalRoot), ".slidex-update-locks")
	if err := ensureSecureDir(lockDir); err != nil {
		return "", err
	}
	rootSum := sha256.Sum256([]byte(filepath.ToSlash(canonicalRoot)))
	return filepath.Join(lockDir, hex.EncodeToString(rootSum[:])+".lock"), nil
}

func staleUpdateInstallLock(lockPath string) bool {
	_, stale := staleUpdateInstallLockSnapshot(lockPath)
	return stale
}

func staleUpdateInstallLockSnapshot(lockPath string) (lockFileSnapshot, bool) {
	snapshot, ok := readLockFileSnapshot(lockPath, maxUpdateMetadataBytes)
	if !ok {
		return lockFileSnapshot{}, false
	}
	staleAfter := updateInstallLockStaleAfter
	if staleAfter <= 0 {
		staleAfter = 30 * time.Minute
	}
	if !snapshot.rawOK {
		return snapshot, time.Since(snapshot.info.ModTime()) > staleAfter
	}
	fields := map[string]string{}
	for _, field := range strings.Fields(string(snapshot.raw)) {
		name, value, ok := strings.Cut(field, "=")
		if ok {
			fields[name] = value
		}
	}
	if fields["schema"] == updateInstallLockSchema {
		pid, err := strconv.Atoi(fields["pid"])
		if err != nil || pid <= 0 {
			return snapshot, time.Since(snapshot.info.ModTime()) > staleAfter
		}
		return snapshot, !processAlive(pid)
	}
	return snapshot, time.Since(snapshot.info.ModTime()) > staleAfter
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
	lockedRoot, unlock, err := lockResolvedUpdateInstallRoot(*installRoot)
	if err != nil {
		return err
	}
	*installRoot = lockedRoot
	defer unlock()
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
	lockedRoot, unlock, err := lockResolvedUpdateInstallRoot(*installRoot)
	if err != nil {
		return err
	}
	*installRoot = lockedRoot
	defer unlock()
	status, err := currentUpdateStatus(*installRoot, *metadataPath)
	if err != nil {
		return err
	}
	if status.UpdatesEnabled {
		release, err := fetchUpdateReleaseForStatus(context.Background(), *apiURL, status)
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
	lockedRoot, unlock, err := lockResolvedUpdateInstallRoot(*installRoot)
	if err != nil {
		return err
	}
	*installRoot = lockedRoot
	defer unlock()
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
		canonicalVersion, canonicalTag, err := canonicalUpdateTargetInputs(*targetVersion, *targetTag)
		if err != nil {
			return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
		}
		*targetVersion = canonicalVersion
		*targetTag = canonicalTag
		stageParent, stagedArchive, err := stageVerifiedLocalReleaseArchive(status.InstallRoot, *targetVersion, *archive, *checksums)
		if err != nil {
			return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
		}
		extracted, err := extractDownloadedReleaseArchive(stageParent, stagedArchive)
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
	canonicalVersion, canonicalTag, err := canonicalUpdateTargetInputs(*targetVersion, *targetTag)
	if err != nil {
		return maybePrintUpdateApplyFailure(*jsonOut, status, *targetVersion, *targetTag, err)
	}
	*targetVersion = canonicalVersion
	*targetTag = canonicalTag
	applyOptions := updateApplyOptions{
		ExecuteCandidateChecks: *candidate == "",
	}
	result, err := applyCandidateBundleWithOptions(status, candidateRoot, *targetVersion, *targetTag, applyOptions)
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
	executeCandidateChecks := fs.Bool("execute-candidate-checks", false, "execute candidate CLI dynamic validation checks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return exitCodeError(2, "usage: slidex update verify [--json] [--metadata FILE] [--install-root DIR] [--candidate DIR --target-version VERSION [--execute-candidate-checks]]")
	}
	lockedRoot, unlock, err := lockResolvedUpdateInstallRoot(*installRoot)
	if err != nil {
		return err
	}
	*installRoot = lockedRoot
	defer unlock()
	status, err := currentUpdateStatus(*installRoot, *metadataPath)
	if err != nil {
		return err
	}
	if *candidate != "" {
		if *targetVersion == "" {
			return exitCodeError(2, "--target-version is required with --candidate")
		}
		status.CandidateValidation = validateCandidateBundleForStatus(*candidate, *targetVersion, status.Channel)
		if *executeCandidateChecks && !hasFailures(status.CandidateValidation) {
			status.CandidateValidation = append(status.CandidateValidation, validateCandidateBundleDynamicChecks(*candidate, *targetVersion)...)
		}
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
	lockedRoot, unlock, err := lockResolvedUpdateInstallRoot(*installRoot)
	if err != nil {
		return err
	}
	*installRoot = lockedRoot
	defer unlock()
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
	installRoot := resolveUpdateInstallRoot(installRootArg)
	canonicalRoot, err := canonicalUpdateInstallRoot(installRoot)
	if err != nil {
		return updateStatus{}, err
	}
	installRoot = canonicalRoot
	metadataPath := resolveUpdateMetadataPath(installRoot, metadataPathArg)

	metadata, metadataErr := readInstallMetadata(metadataPath)
	channel, mode, reason := inferUpdateChannel(installRoot, metadata, metadataErr)
	if metadata != nil && (channel == updateChannelProduction || channel == updateChannelCanary) {
		if issue := installedReleaseMetadataIssue(metadata); issue != "" {
			channel = updateChannelLocalDevelopment
			mode = firstNonEmpty(metadata.InstallMode, installModeUnknown)
			reason = "install metadata is inconsistent; update is disabled fail-closed: " + issue
		} else if issue := installedReleaseMetadataBindingIssue(installRoot, metadataPath, metadata); issue != "" {
			channel = updateChannelLocalDevelopment
			mode = firstNonEmpty(metadata.InstallMode, installModeUnknown)
			reason = "install metadata is not bound to the resolved install root; update is disabled fail-closed: " + issue
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
			status.Reason = appendReason(status.Reason, "install metadata records a different install root "+filepath.ToSlash(metadata.InstallRoot)+"; resolved install root is "+filepath.ToSlash(installRoot))
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
		if err := validatePendingUpdate(installRoot, pending); err != nil {
			status.Status = "pending-invalid"
			status.PendingUpdatePath = filepath.ToSlash(pendingPath)
			status.RestartRequired = true
			status.PluginVerificationStatus = "restart_required"
			status.NextVerificationCommand = "slidex update activate-pending --yes --json"
			status.Reason = appendReason(status.Reason, "pending update state is invalid and must be repaired before activation: "+err.Error())
		} else {
			activationCommand := pendingActivationCommand(filepath.FromSlash(pending.ActivatorPath), status.InstallRoot)
			safePending := *pending
			safePending.ActivationCommand = activationCommand
			status.PendingActivation = true
			status.PendingUpdate = &safePending
			status.PendingUpdatePath = filepath.ToSlash(pendingPath)
			status.Status = "pending-activation"
			status.TargetVersion = pending.TargetVersion
			status.TargetTag = pending.TargetTag
			status.RestartRequired = true
			status.PluginVerificationStatus = "restart_required"
			status.PendingActivationCommand = activationCommand
		}
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
	return updateAPIURLWithPerPage(updateGitHubReleasesAPI, updateGitHubReleasesPerPage)
}

func updateAPIURLWithPerPage(rawURL string, perPage int) string {
	if perPage <= 0 {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	if query.Get("per_page") == "" {
		query.Set("per_page", strconv.Itoa(perPage))
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
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
		SchemaVersion:            pendingUpdateSchemaVersion,
		ToolName:                 toolName,
		TargetVersion:            toolVersion,
		TargetTag:                "v" + toolVersion,
		InstallRoot:              installRoot,
		StagedRoot:               filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(installRoot)), ".slidex.staged-"+toolVersion)),
		StagedRootManifestSHA256: strings.Repeat("a", 64),
		ActivatorPath:            filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(installRoot)), ".slidex.activator-"+toolVersion, activatorBinary)),
		ActivatorSHA256:          strings.Repeat("b", 64),
		ActivationCommand:        "slidex update activate-pending --yes --json",
		Reason:                   "doctor schema contract sample",
		CreatedAt:                now,
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
	canonicalVersion, canonicalTag, err := canonicalUpdateTargetInputs(targetVersion, targetTag)
	if err != nil {
		return err
	}
	return writeUpdateState(installRoot, updateState{
		CurrentVersion:      toolVersion,
		TargetVersion:       canonicalVersion,
		TargetTag:           canonicalTag,
		Channel:             channelFromPackageVersion(canonicalVersion),
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
	payload, err := readRegularFileWithMaxBytes(archivePath, maxUpdateArchiveCompressedBytes)
	if err != nil {
		return err
	}
	checksumText, err := readRegularFileWithMaxBytes(checksumsPath, maxUpdateChecksumBytes)
	if err != nil {
		return err
	}
	if _, err := verifyReleaseAssetSHA256(filepath.Base(archivePath), payload, string(checksumText), ""); err != nil {
		return err
	}
	return nil
}

func stageVerifiedLocalReleaseArchive(installRoot, targetVersion, archivePath, checksumsPath string) (stageParent, stagedArchivePath string, err error) {
	payload, err := readRegularFileWithMaxBytes(archivePath, maxUpdateArchiveCompressedBytes)
	if err != nil {
		return "", "", err
	}
	checksumText, err := readRegularFileWithMaxBytes(checksumsPath, maxUpdateChecksumBytes)
	if err != nil {
		return "", "", err
	}
	archiveName := filepath.Base(archivePath)
	if _, err := verifyReleaseAssetSHA256(archiveName, payload, string(checksumText), ""); err != nil {
		return "", "", err
	}
	stageParent, err = updateInternalStageDir(installRoot, "downloads", targetVersion)
	if err != nil {
		return "", "", err
	}
	if err := ensureSecureDir(stageParent); err != nil {
		return "", "", err
	}
	stagedArchivePath = filepath.Join(stageParent, archiveName)
	if err := secureWriteFile(stagedArchivePath, payload, 0o644); err != nil {
		_ = os.RemoveAll(stageParent)
		return "", "", err
	}
	return stageParent, stagedArchivePath, nil
}

func extractArchiveCandidate(archivePath, targetVersion, installRoot string) (string, error) {
	stageParent, err := updateInternalStageDir(installRoot, "staged", targetVersion)
	if err != nil {
		return "", err
	}
	if err := ensureSecureDir(stageParent); err != nil {
		return "", err
	}
	candidateRoot, err := extractReleaseArchive(archivePath, stageParent)
	if err != nil {
		_ = os.RemoveAll(stageParent)
		return "", err
	}
	return candidateRoot, nil
}

func readRegularFileWithMaxBytes(path string, maxBytes int64) ([]byte, error) {
	f, info, err := openRegularFileForRead(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if maxBytes > 0 && info.Size() > maxBytes {
		return nil, fmt.Errorf("file exceeds maximum allowed size: %s is %d bytes > %d", filepath.ToSlash(path), info.Size(), maxBytes)
	}
	return readAllWithMaxBytes(f, maxBytes, filepath.ToSlash(path))
}

func readAllWithMaxBytes(r io.Reader, maxBytes int64, label string) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(r)
	}
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("payload exceeds maximum allowed size: %s is greater than %d bytes", label, maxBytes)
	}
	return raw, nil
}

func updateInternalStageDir(installRoot, kind, targetVersion string) (string, error) {
	versionSegment, err := safeUpdateTargetVersionSegment(targetVersion)
	if err != nil {
		return "", err
	}
	root := filepath.Join(installRoot, ".slidex", kind)
	if err := ensureSecureDir(root); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(root, versionSegment+"-")
	if err != nil {
		return "", err
	}
	if !pathWithin(root, stage) {
		_ = os.RemoveAll(stage)
		return "", fmt.Errorf("update staging path escapes %s: %s", filepath.ToSlash(root), filepath.ToSlash(stage))
	}
	return stage, nil
}

func safeUpdateTargetVersionSegment(targetVersion string) (string, error) {
	version, err := canonicalUpdateTargetVersion(targetVersion)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(version) ||
		filepath.VolumeName(version) != "" ||
		strings.ContainsAny(version, `/\`) ||
		version == "." ||
		version == ".." ||
		filepath.Clean(version) != version ||
		filepath.Base(version) != version {
		return "", fmt.Errorf("target version must be a safe path segment, got %q", targetVersion)
	}
	return version, nil
}

func canonicalUpdateTargetVersion(targetVersion string) (string, error) {
	version := strings.TrimPrefix(strings.TrimSpace(targetVersion), "v")
	if version == "" {
		return "", errors.New("target version is required")
	}
	if !isReleasePackageVersion(version) {
		return "", fmt.Errorf("target version must be a stable or canary package version, got %q", targetVersion)
	}
	return version, nil
}

func canonicalUpdateTargetInputs(targetVersion, targetTag string) (string, string, error) {
	version, err := canonicalUpdateTargetVersion(targetVersion)
	if err != nil {
		return "", "", err
	}
	tag := strings.TrimSpace(targetTag)
	if tag == "" {
		return version, "", nil
	}
	tagVersion, err := releasePackageVersionFromTag(tag)
	if err != nil {
		return "", "", fmt.Errorf("target tag %q is not a valid release tag: %w", targetTag, err)
	}
	if tagVersion != version {
		return "", "", fmt.Errorf("target tag %q resolves to %s, want %s", targetTag, tagVersion, version)
	}
	return version, "v" + tagVersion, nil
}

func downloadAndStageReleaseCandidate(ctx context.Context, status updateStatus, apiURL string) (candidateRoot, targetVersion, targetTag string, err error) {
	release, err := fetchUpdateReleaseForStatus(ctx, apiURL, status)
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
	archivePayload, err := downloadUpdateAsset(ctx, archive, maxUpdateArchiveCompressedBytes)
	if err != nil {
		return "", "", "", err
	}
	checksumPayload, err := downloadUpdateAsset(ctx, checksum, maxUpdateChecksumBytes)
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

func downloadUpdateAsset(ctx context.Context, asset updateAsset, maxBytes int64) ([]byte, error) {
	if asset.BrowserDownloadURL == "" {
		return nil, fmt.Errorf("release asset %s is missing browser_download_url", asset.Name)
	}
	ctx, cancel := contextWithDefaultTimeout(ctx, updateAssetDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "slidex-update/"+toolVersion)
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download %s returned %s: %s", asset.Name, resp.Status, strings.TrimSpace(string(body)))
	}
	return readAllWithMaxBytes(resp.Body, maxBytes, asset.Name)
}

func stageDownloadedReleaseArchive(installRoot, targetVersion string, archive updateAsset, archivePayload []byte, checksum updateAsset, checksumPayload []byte) (stageParent, archivePath string, err error) {
	if normalizeGitHubSHA256Digest(archive.Digest) == "" {
		return "", "", fmt.Errorf("GitHub release asset digest is required for downloaded archive %s", archive.Name)
	}
	if _, err := verifyReleaseAssetSHA256(archive.Name, archivePayload, string(checksumPayload), archive.Digest); err != nil {
		return "", "", err
	}
	stageParent, err = updateInternalStageDir(installRoot, "downloads", targetVersion)
	if err != nil {
		return "", "", err
	}
	if err := ensureSecureDir(stageParent); err != nil {
		return "", "", err
	}
	archivePath = filepath.Join(stageParent, archive.Name)
	checksumPath := filepath.Join(stageParent, checksum.Name)
	if err := secureWriteFile(archivePath, archivePayload, 0o644); err != nil {
		return "", "", err
	}
	if err := secureWriteFile(checksumPath, checksumPayload, 0o644); err != nil {
		return "", "", err
	}
	return stageParent, archivePath, nil
}

func extractDownloadedReleaseArchive(stageParent, archivePath string) (candidateRoot string, err error) {
	extractRoot := filepath.Join(stageParent, "extract")
	if err := ensureSecureDir(extractRoot); err != nil {
		return "", err
	}
	candidateRoot, err = extractReleaseArchive(archivePath, extractRoot)
	if err != nil {
		_ = os.RemoveAll(stageParent)
		return "", err
	}
	return candidateRoot, nil
}

type updateArchiveExtractionBudget struct {
	maxEntries          int
	maxFileSize         int64
	maxTotal            int64
	maxCentralDirectory int64
	entries             int
	total               int64
}

func defaultUpdateArchiveExtractionBudget() *updateArchiveExtractionBudget {
	return &updateArchiveExtractionBudget{
		maxEntries:          maxUpdateArchiveEntries,
		maxFileSize:         maxUpdateArchiveFileBytes,
		maxTotal:            maxUpdateArchiveExpandedBytes,
		maxCentralDirectory: maxUpdateZipCentralDirBytes,
	}
}

func (b *updateArchiveExtractionBudget) addEntry(name string) error {
	b.entries++
	if b.maxEntries > 0 && b.entries > b.maxEntries {
		return fmt.Errorf("archive contains too many entries: %d > %d at %s", b.entries, b.maxEntries, name)
	}
	return nil
}

func (b *updateArchiveExtractionBudget) reserveFile(name string, size int64) error {
	if size < 0 {
		return fmt.Errorf("archive entry has negative size: %s", name)
	}
	if b.maxFileSize > 0 && size > b.maxFileSize {
		return fmt.Errorf("archive entry exceeds maximum uncompressed size: %s is %d bytes > %d", name, size, b.maxFileSize)
	}
	if b.maxTotal > 0 && b.total > b.maxTotal-size {
		return fmt.Errorf("archive exceeds maximum expanded size at %s: %d bytes > %d", name, b.total+size, b.maxTotal)
	}
	b.total += size
	return nil
}

func (b *updateArchiveExtractionBudget) addCandidateTreeEntry(name string) error {
	b.entries++
	if b.maxEntries > 0 && b.entries > b.maxEntries {
		return fmt.Errorf("candidate tree contains too many entries: %d > %d at %s", b.entries, b.maxEntries, name)
	}
	return nil
}

func (b *updateArchiveExtractionBudget) reserveCandidateTreeFile(name string, size int64) error {
	if size < 0 {
		return fmt.Errorf("candidate tree file has negative size: %s", name)
	}
	if b.maxFileSize > 0 && size > b.maxFileSize {
		return fmt.Errorf("candidate tree file exceeds maximum size: %s is %d bytes > %d", name, size, b.maxFileSize)
	}
	if b.maxTotal > 0 && b.total > b.maxTotal-size {
		return fmt.Errorf("candidate tree exceeds maximum expanded size at %s: %d bytes > %d", name, b.total+size, b.maxTotal)
	}
	b.total += size
	return nil
}

func validateLocalCandidateTree(root string) error {
	return validateLocalCandidateTreeWithBudget(root, defaultUpdateArchiveExtractionBudget())
}

func validateLocalCandidateTreeWithBudget(root string, budget *updateArchiveExtractionBudget) error {
	return walkLocalCandidateTreeWithBudget(root, budget, nil)
}

func validateLocalCandidateTreeMode(name string, info os.FileInfo) error {
	if !modeBitsAreSecurityRelevant() {
		return nil
	}
	mode := info.Mode().Perm()
	if mode&0o022 != 0 {
		return fmt.Errorf("candidate tree path must not be group/world writable: %s mode %04o", name, mode)
	}
	return nil
}

func copyLocalCandidateTreeWithBudget(src, dst string, budget *updateArchiveExtractionBudget) error {
	cleanDst := filepath.Clean(dst)
	return walkLocalCandidateTreeWithBudget(src, budget, func(path, rel string, info os.FileInfo) error {
		target := cleanDst
		if rel != "." {
			target = filepath.Join(cleanDst, rel)
		}
		if !pathWithin(cleanDst, target) {
			return fmt.Errorf("candidate tree copy target escapes staging root: %s", filepath.ToSlash(target))
		}
		if info.IsDir() {
			mode := info.Mode().Perm()
			if mode == 0 {
				mode = 0o755
			}
			return ensureSecureDirMode(target, mode)
		}
		return copyFileWithMaxBytes(path, target, info.Size())
	})
}

func candidateTreeManifestDigest(root string) (string, error) {
	h := sha256.New()
	err := walkLocalCandidateTreeWithBudget(root, defaultUpdateArchiveExtractionBudget(), func(path, rel string, info os.FileInfo) error {
		name := filepath.ToSlash(rel)
		mode := info.Mode().Perm()
		if info.IsDir() {
			_, _ = fmt.Fprintf(h, "dir\t%s\t%04o\n", name, mode)
			return nil
		}
		fileHash, err := sha256FileWithMaxBytes(path, maxUpdateArchiveFileBytes)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "file\t%s\t%04o\t%d\t%s\n", name, mode, info.Size(), fileHash)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func walkLocalCandidateTreeWithBudget(root string, budget *updateArchiveExtractionBudget, visit func(path, rel string, info os.FileInfo) error) error {
	if budget == nil {
		budget = defaultUpdateArchiveExtractionBudget()
	}
	cleanRoot := filepath.Clean(root)
	return filepath.WalkDir(cleanRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if isSymlinkOrReparsePoint(path, info) {
			return fmt.Errorf("candidate tree must not contain symlinks or reparse points: %s", filepath.ToSlash(path))
		}
		rel, err := filepath.Rel(cleanRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			if !info.IsDir() {
				return fmt.Errorf("candidate root must be a directory: %s", filepath.ToSlash(cleanRoot))
			}
			if err := validateLocalCandidateTreeMode(".", info); err != nil {
				return err
			}
			if visit != nil {
				return visit(path, rel, info)
			}
			return nil
		}
		name := filepath.ToSlash(rel)
		if err := validateArchiveRelativePath(name); err != nil {
			return fmt.Errorf("unsafe candidate tree path: %w", err)
		}
		if err := budget.addCandidateTreeEntry(name); err != nil {
			return err
		}
		if err := validateLocalCandidateTreeMode(name, info); err != nil {
			return err
		}
		if info.IsDir() {
			if visit != nil {
				return visit(path, rel, info)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("candidate tree contains unsupported file type: %s", name)
		}
		if err := budget.reserveCandidateTreeFile(name, info.Size()); err != nil {
			return err
		}
		if visit != nil {
			return visit(path, rel, info)
		}
		return nil
	})
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
	return extractTarGzArchiveWithBudget(archivePath, dest, defaultUpdateArchiveExtractionBudget())
}

func extractTarGzArchiveWithBudget(archivePath, dest string, budget *updateArchiveExtractionBudget) error {
	if budget == nil {
		budget = defaultUpdateArchiveExtractionBudget()
	}
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
		if err := validateArchiveRelativePath(header.Name); err != nil {
			return err
		}
		if err := budget.addEntry(header.Name); err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean(header.Name))
		if !pathWithin(dest, target) {
			return fmt.Errorf("archive entry escapes extraction root: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := ensureSecureDirMode(target, archiveDirMode(os.FileMode(header.Mode))); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := budget.reserveFile(header.Name, header.Size); err != nil {
				return err
			}
			if err := ensureSecureDirMode(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeStreamFileLimited(target, tr, os.FileMode(header.Mode)&0o777, header.Size); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
	}
	return nil
}

func extractZipArchive(archivePath, dest string) error {
	return extractZipArchiveWithBudget(archivePath, dest, defaultUpdateArchiveExtractionBudget())
}

func extractZipArchiveWithBudget(archivePath, dest string, budget *updateArchiveExtractionBudget) error {
	if budget == nil {
		budget = defaultUpdateArchiveExtractionBudget()
	}
	if err := validateZipArchiveDirectoryBudget(archivePath, budget); err != nil {
		return err
	}
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, file := range zr.File {
		if err := validateArchiveRelativePath(file.Name); err != nil {
			return err
		}
		if err := budget.addEntry(file.Name); err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean(file.Name))
		if !pathWithin(dest, target) {
			return fmt.Errorf("archive entry escapes extraction root: %s", file.Name)
		}
		mode := file.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported symlink in archive: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := ensureSecureDirMode(target, archiveDirMode(mode)); err != nil {
				return err
			}
			continue
		}
		if file.UncompressedSize64 > uint64(1<<63-1) {
			return fmt.Errorf("archive entry size is too large to validate: %s", file.Name)
		}
		uncompressedSize := int64(file.UncompressedSize64)
		if err := budget.reserveFile(file.Name, uncompressedSize); err != nil {
			return err
		}
	}
	for _, file := range zr.File {
		target := filepath.Join(dest, filepath.Clean(file.Name))
		mode := file.FileInfo().Mode()
		if file.FileInfo().IsDir() {
			if err := ensureSecureDirMode(target, archiveDirMode(mode)); err != nil {
				return err
			}
			continue
		}
		uncompressedSize := int64(file.UncompressedSize64)
		rc, err := file.Open()
		if err != nil {
			return err
		}
		if err := ensureSecureDirMode(filepath.Dir(target), 0o755); err != nil {
			_ = rc.Close()
			return err
		}
		err = writeStreamFileLimited(target, rc, mode&0o777, uncompressedSize)
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

func validateArchiveRelativePath(name string) error {
	if name == "" {
		return fmt.Errorf("unsafe archive entry path: empty path")
	}
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("unsafe archive entry path %q: NUL byte is not allowed", name)
	}
	if strings.Contains(name, `\`) {
		return fmt.Errorf("unsafe archive entry path %q: backslash separators are not portable", name)
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || isWindowsDrivePath(name) {
		return fmt.Errorf("unsafe archive entry path %q: absolute paths are not allowed", name)
	}
	checkName := strings.TrimSuffix(name, "/")
	if checkName == "" {
		return fmt.Errorf("unsafe archive entry path %q: empty, dot, and dot-dot path segments are not allowed", name)
	}
	parts := strings.Split(checkName, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("unsafe archive entry path %q: empty, dot, and dot-dot path segments are not allowed", name)
		}
		if strings.Contains(part, ":") {
			return fmt.Errorf("unsafe archive entry path %q: colon is not allowed in path segments", name)
		}
		if strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") {
			return fmt.Errorf("unsafe archive entry path %q: trailing spaces and dots are not portable", name)
		}
		stem := strings.TrimRight(part, " .")
		if isWindowsReservedFilenameStem(stem) {
			return fmt.Errorf("unsafe archive entry path %q: Windows reserved device names are not allowed", name)
		}
	}
	return nil
}

type zipArchiveDirectoryMetadata struct {
	entries                uint64
	centralDirectorySize   uint64
	centralDirectoryOffset uint64
	centralDirectoryEnd    uint64
}

func validateZipArchiveDirectoryBudget(archivePath string, budget *updateArchiveExtractionBudget) error {
	metadata, err := readZipArchiveDirectoryMetadata(archivePath)
	if err != nil {
		return err
	}
	if budget.maxEntries > 0 && metadata.entries > uint64(budget.maxEntries) {
		return fmt.Errorf("archive contains too many entries before opening ZIP: %d > %d", metadata.entries, budget.maxEntries)
	}
	if budget.maxCentralDirectory > 0 && metadata.centralDirectorySize > uint64(budget.maxCentralDirectory) {
		return fmt.Errorf("ZIP central directory exceeds maximum size before opening ZIP: %d bytes > %d", metadata.centralDirectorySize, budget.maxCentralDirectory)
	}
	if err := validateZipArchiveDirectoryContents(archivePath, metadata, budget); err != nil {
		return err
	}
	return nil
}

func readZipArchiveDirectoryMetadata(archivePath string) (zipArchiveDirectoryMetadata, error) {
	var metadata zipArchiveDirectoryMetadata
	f, info, err := openRegularFileForRead(archivePath)
	if err != nil {
		return metadata, err
	}
	defer f.Close()
	archiveSize := info.Size()
	if archiveSize < zipEndOfCentralDirectoryMinSize {
		return metadata, fmt.Errorf("ZIP archive is too small to contain an end of central directory record: %s", filepath.ToSlash(archivePath))
	}
	tailSize := int64(zipEndOfCentralDirectoryMinSize + zipMaxCommentSize)
	if archiveSize < tailSize {
		tailSize = archiveSize
	}
	tail := make([]byte, int(tailSize))
	tailOffset := archiveSize - tailSize
	if _, err := f.ReadAt(tail, tailOffset); err != nil {
		return metadata, err
	}
	eocdIndex := -1
	for i := len(tail) - zipEndOfCentralDirectoryMinSize; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:i+4]) != zipEndOfCentralDirectorySignature {
			continue
		}
		commentLen := int(binary.LittleEndian.Uint16(tail[i+20 : i+22]))
		if i+zipEndOfCentralDirectoryMinSize+commentLen == len(tail) {
			eocdIndex = i
			break
		}
	}
	if eocdIndex < 0 {
		return metadata, fmt.Errorf("ZIP archive is missing an end of central directory record: %s", filepath.ToSlash(archivePath))
	}
	eocd := tail[eocdIndex:]
	eocdOffset := tailOffset + int64(eocdIndex)
	diskNumber := binary.LittleEndian.Uint16(eocd[4:6])
	centralDirectoryDisk := binary.LittleEndian.Uint16(eocd[6:8])
	entriesOnDisk := binary.LittleEndian.Uint16(eocd[8:10])
	totalEntries := binary.LittleEndian.Uint16(eocd[10:12])
	centralDirectorySize32 := binary.LittleEndian.Uint32(eocd[12:16])
	centralDirectoryOffset32 := binary.LittleEndian.Uint32(eocd[16:20])
	if diskNumber != 0 || centralDirectoryDisk != 0 {
		return metadata, fmt.Errorf("multi-disk ZIP archives are not supported: %s", filepath.ToSlash(archivePath))
	}
	requiresZip64 := entriesOnDisk == 0xffff || totalEntries == 0xffff || centralDirectorySize32 == 0xffffffff || centralDirectoryOffset32 == 0xffffffff
	if requiresZip64 {
		metadata, err = readZip64ArchiveDirectoryMetadata(f, archiveSize, eocdOffset)
		if err != nil {
			return metadata, err
		}
	} else {
		if entriesOnDisk != totalEntries {
			return metadata, fmt.Errorf("multi-disk ZIP archives are not supported: %s", filepath.ToSlash(archivePath))
		}
		metadata = zipArchiveDirectoryMetadata{
			entries:                uint64(totalEntries),
			centralDirectorySize:   uint64(centralDirectorySize32),
			centralDirectoryOffset: uint64(centralDirectoryOffset32),
			centralDirectoryEnd:    uint64(eocdOffset),
		}
	}
	if err := validateZipArchiveDirectoryBounds(metadata, eocdOffset, archivePath); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func readZip64ArchiveDirectoryMetadata(f *os.File, archiveSize, eocdOffset int64) (zipArchiveDirectoryMetadata, error) {
	var metadata zipArchiveDirectoryMetadata
	if eocdOffset < zip64EndOfCentralDirectoryLocatorSize {
		return metadata, fmt.Errorf("ZIP64 archive is missing a locator before the end of central directory")
	}
	var locator [zip64EndOfCentralDirectoryLocatorSize]byte
	if _, err := f.ReadAt(locator[:], eocdOffset-zip64EndOfCentralDirectoryLocatorSize); err != nil {
		return metadata, err
	}
	if binary.LittleEndian.Uint32(locator[0:4]) != zip64EndOfCentralDirectoryLocSignature {
		return metadata, fmt.Errorf("ZIP64 archive is missing a locator before the end of central directory")
	}
	diskWithDirectory := binary.LittleEndian.Uint32(locator[4:8])
	zip64DirectoryOffset := binary.LittleEndian.Uint64(locator[8:16])
	totalDisks := binary.LittleEndian.Uint32(locator[16:20])
	if diskWithDirectory != 0 || totalDisks > 1 {
		return metadata, fmt.Errorf("multi-disk ZIP64 archives are not supported")
	}
	if zip64DirectoryOffset > uint64(archiveSize) || uint64(archiveSize)-zip64DirectoryOffset < zip64EndOfCentralDirectoryMinRecordSize {
		return metadata, fmt.Errorf("ZIP64 end of central directory record exceeds archive bounds")
	}
	var record [zip64EndOfCentralDirectoryMinRecordSize]byte
	if _, err := f.ReadAt(record[:], int64(zip64DirectoryOffset)); err != nil {
		return metadata, err
	}
	if binary.LittleEndian.Uint32(record[0:4]) != zip64EndOfCentralDirectorySignature {
		return metadata, fmt.Errorf("ZIP64 archive is missing an end of central directory record")
	}
	if recordSize := binary.LittleEndian.Uint64(record[4:12]); recordSize < 44 {
		return metadata, fmt.Errorf("ZIP64 end of central directory record is too small: %d bytes", recordSize)
	}
	diskNumber := binary.LittleEndian.Uint32(record[16:20])
	centralDirectoryDisk := binary.LittleEndian.Uint32(record[20:24])
	if diskNumber != 0 || centralDirectoryDisk != 0 {
		return metadata, fmt.Errorf("multi-disk ZIP64 archives are not supported")
	}
	entriesOnDisk := binary.LittleEndian.Uint64(record[24:32])
	totalEntries := binary.LittleEndian.Uint64(record[32:40])
	if entriesOnDisk != totalEntries {
		return metadata, fmt.Errorf("multi-disk ZIP64 archives are not supported")
	}
	return zipArchiveDirectoryMetadata{
		entries:                totalEntries,
		centralDirectorySize:   binary.LittleEndian.Uint64(record[40:48]),
		centralDirectoryOffset: binary.LittleEndian.Uint64(record[48:56]),
		centralDirectoryEnd:    zip64DirectoryOffset,
	}, nil
}

func validateZipArchiveDirectoryBounds(metadata zipArchiveDirectoryMetadata, eocdOffset int64, archivePath string) error {
	if metadata.centralDirectoryEnd > uint64(eocdOffset) || metadata.centralDirectoryOffset > metadata.centralDirectoryEnd {
		return fmt.Errorf("ZIP central directory exceeds archive bounds: %s", filepath.ToSlash(archivePath))
	}
	if metadata.centralDirectorySize > metadata.centralDirectoryEnd-metadata.centralDirectoryOffset {
		return fmt.Errorf("ZIP central directory exceeds archive bounds: %s", filepath.ToSlash(archivePath))
	}
	return nil
}

func validateZipArchiveDirectoryContents(archivePath string, metadata zipArchiveDirectoryMetadata, budget *updateArchiveExtractionBudget) error {
	actualSize := metadata.centralDirectoryEnd - metadata.centralDirectoryOffset
	if metadata.centralDirectorySize != actualSize {
		return fmt.Errorf("ZIP central directory size does not match metadata before opening ZIP: %d bytes != %d", actualSize, metadata.centralDirectorySize)
	}
	if budget.maxCentralDirectory > 0 && actualSize > uint64(budget.maxCentralDirectory) {
		return fmt.Errorf("ZIP central directory exceeds maximum size before opening ZIP: %d bytes > %d", actualSize, budget.maxCentralDirectory)
	}
	f, _, err := openRegularFileForRead(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var count uint64
	offset := metadata.centralDirectoryOffset
	for offset < metadata.centralDirectoryEnd {
		remaining := metadata.centralDirectoryEnd - offset
		if remaining < zipCentralDirectoryHeaderMinSize {
			return fmt.Errorf("ZIP central directory entry exceeds archive bounds before opening ZIP")
		}
		var header [zipCentralDirectoryHeaderMinSize]byte
		if _, err := f.ReadAt(header[:], int64(offset)); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header[0:4]) != zipCentralDirectoryHeaderSignature {
			return fmt.Errorf("ZIP central directory contains an invalid entry before opening ZIP")
		}
		nameLen := uint64(binary.LittleEndian.Uint16(header[28:30]))
		extraLen := uint64(binary.LittleEndian.Uint16(header[30:32]))
		commentLen := uint64(binary.LittleEndian.Uint16(header[32:34]))
		entrySize := uint64(zipCentralDirectoryHeaderMinSize) + nameLen + extraLen + commentLen
		if entrySize > remaining {
			return fmt.Errorf("ZIP central directory entry exceeds archive bounds before opening ZIP")
		}
		count++
		if budget.maxEntries > 0 && count > uint64(budget.maxEntries) {
			return fmt.Errorf("archive contains too many entries before opening ZIP: %d > %d", count, budget.maxEntries)
		}
		offset += entrySize
	}
	if count != metadata.entries {
		return fmt.Errorf("ZIP central directory entry count does not match metadata before opening ZIP: %d != %d", count, metadata.entries)
	}
	return nil
}

func writeStreamFile(path string, r io.Reader, mode os.FileMode) error {
	return writeStreamFileLimited(path, r, mode, -1)
}

func writeStreamFileLimited(path string, r io.Reader, mode os.FileMode, maxBytes int64) error {
	if mode == 0 {
		mode = 0o644
	}
	dir := filepath.Dir(path)
	if err := ensureSecureDirMode(dir, 0o755); err != nil {
		return err
	}
	if err := rejectSecureWriteTarget(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := applyPlatformFileMode(tmpPath, mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := copyStreamWithMaxBytes(tmp, r, maxBytes, filepath.ToSlash(path)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(dir); err != nil {
		return err
	}
	if err := rejectSecureWriteTarget(path); err != nil {
		return err
	}
	if err := replaceFile(tmpPath, path); err != nil {
		return err
	}
	if err := applyPlatformFileMode(path, mode); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func copyStreamWithMaxBytes(w io.Writer, r io.Reader, maxBytes int64, label string) error {
	if maxBytes < 0 {
		_, err := io.Copy(w, r)
		return err
	}
	n, err := io.Copy(w, io.LimitReader(r, maxBytes+1))
	if err != nil {
		return err
	}
	if n > maxBytes {
		return fmt.Errorf("archive entry exceeds declared extraction budget: %s is greater than %d bytes", label, maxBytes)
	}
	return nil
}

func archiveDirMode(mode os.FileMode) os.FileMode {
	mode &= 0o777
	if mode == 0 {
		return 0o755
	}
	return mode
}

func shellQuoteCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		if strings.IndexFunc(arg, func(r rune) bool {
			return !(r == '/' || r == '.' || r == '-' || r == '_' || r == ':' || r == '=' || r == '+' || r == ',' || r == '@' || r == '^' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
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
	workDir := ""
	if strings.TrimSpace(activatorPath) != "" {
		command = filepath.ToSlash(activatorPath)
		workDir = filepath.ToSlash(filepath.Dir(filepath.FromSlash(activatorPath)))
	}
	args := []string{"update", "activate-pending", "--install-root", filepath.ToSlash(installRoot), "--yes", "--json"}
	if runtime.GOOS == "windows" {
		if workDir != "" {
			return windowsPendingActivationPowerShellCommand(workDir, filepath.ToSlash(installRoot), command, args...)
		}
		return windowsPowerShellInlineCommandInDir("", command, args...)
	}
	return shellQuoteCommand(append([]string{command}, args...))
}

func windowsPendingActivationPowerShellCommand(activatorRoot, installRoot, name string, args ...string) string {
	var script strings.Builder
	script.WriteString("& { $slidexPreviousErrorActionPreference = $ErrorActionPreference; $ErrorActionPreference='Stop'; $slidexActivationExitCode = 0; try { ")
	writeWindowsPowerShellSetLocation(&script, activatorRoot)
	script.WriteString("; ")
	writeWindowsPowerShellInvocation(&script, name, args...)
	script.WriteString("; $slidexActivationExitCode = if ($null -eq $LASTEXITCODE) { 0 } else { $LASTEXITCODE }")
	script.WriteString(" } finally { ")
	writeWindowsPowerShellSetLocation(&script, installRoot)
	script.WriteString("; $ErrorActionPreference = $slidexPreviousErrorActionPreference }; if ($slidexActivationExitCode -ne 0) { throw ('slidex pending activation failed with exit code ' + $slidexActivationExitCode) } }")
	return script.String()
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

func candidateCLIBinaryName() string {
	if runtime.GOOS == "windows" {
		return "slidex.exe"
	}
	return "slidex"
}

type updateApplyOptions struct {
	// Explicit --candidate trees remain static-only so update apply does not execute arbitrary local bundles.
	ExecuteCandidateChecks bool
}

func applyCandidateBundle(status updateStatus, candidateRoot, targetVersion, targetTag string) (updateApplyResult, error) {
	return applyCandidateBundleWithOptions(status, candidateRoot, targetVersion, targetTag, updateApplyOptions{})
}

func applyCandidateBundleWithOptions(status updateStatus, candidateRoot, targetVersion, targetTag string, options updateApplyOptions) (updateApplyResult, error) {
	canonicalVersion, canonicalTag, targetErr := canonicalUpdateTargetInputs(targetVersion, targetTag)
	if targetErr == nil {
		targetVersion = canonicalVersion
		targetTag = canonicalTag
	}
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
	if targetErr != nil {
		result.CandidateValidation = append(result.CandidateValidation, fail("update.target_identity", targetErr.Error(), filepath.ToSlash(filepath.Join(candidateRoot, ".slidex", "install.json"))))
		return result, nil
	}
	result.CandidateValidation = validateCandidateBundleForApply(candidateRoot, targetVersion, status.Channel, options.ExecuteCandidateChecks)
	if hasFailures(result.CandidateValidation) {
		return result, nil
	}
	if runtime.GOOS == "windows" {
		stagedRoot, pendingPath, stagedValidation, err := stagePendingUpdateHandoffWithOptions(status.InstallRoot, candidateRoot, targetVersion, targetTag, status.Channel, options)
		result.StagedRoot = filepath.ToSlash(stagedRoot)
		result.PendingUpdatePath = filepath.ToSlash(pendingPath)
		if err != nil {
			return result, err
		}
		result.CandidateValidation = append(result.CandidateValidation, stagedValidation...)
		if hasFailures(result.CandidateValidation) {
			return result, nil
		}
		result.Status = "pending-restart"
		result.RestartRequired = true
		result.PluginVerificationStatus = "restart_required"
		return result, nil
	}
	stagedRoot, backupRoot, stagedValidation, err := replaceInstallRootWithCandidate(status.InstallRoot, candidateRoot, targetVersion, status.Channel, options)
	result.StagedRoot = filepath.ToSlash(stagedRoot)
	result.BackupRoot = filepath.ToSlash(backupRoot)
	if err != nil {
		result.Status = "rollback"
		return result, err
	}
	result.CandidateValidation = append(result.CandidateValidation, stagedValidation...)
	if hasFailures(result.CandidateValidation) {
		return result, nil
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
	stagedRoot, pendingPath, findings, err := stagePendingUpdateHandoffWithOptions(installRoot, candidateRoot, targetVersion, targetTag, channelFromPackageVersion(strings.TrimPrefix(strings.TrimSpace(targetVersion), "v")), updateApplyOptions{})
	if err != nil {
		return stagedRoot, pendingPath, err
	}
	if hasFailures(findings) {
		return stagedRoot, pendingPath, errors.New("candidate bundle validation failed")
	}
	return stagedRoot, pendingPath, nil
}

func stagePendingUpdateHandoffWithOptions(installRoot, candidateRoot, targetVersion, targetTag, statusChannel string, options updateApplyOptions) (stagedRoot, pendingPath string, findings []qaFinding, err error) {
	stagedRoot, err = stageCandidateForWindowsHandoff(installRoot, candidateRoot, targetVersion)
	if err != nil {
		return "", "", nil, err
	}
	activatorPath := ""
	pendingPathForCleanup := ""
	committed := false
	defer func() {
		if committed {
			return
		}
		if stagedRoot != "" {
			_ = os.RemoveAll(stagedRoot)
		}
		if activatorPath != "" {
			_ = os.RemoveAll(filepath.Dir(activatorPath))
		}
		if pendingPathForCleanup != "" {
			_ = os.Remove(pendingPathForCleanup)
		}
	}()
	if beforeUpdateStagedCandidateValidation != nil {
		if err := beforeUpdateStagedCandidateValidation(stagedRoot); err != nil {
			return stagedRoot, "", nil, err
		}
	}
	findings = validateCandidateBundleForApply(stagedRoot, targetVersion, statusChannel, options.ExecuteCandidateChecks)
	if hasFailures(findings) {
		return stagedRoot, "", findings, nil
	}
	activatorPath, err = stagePendingActivator(installRoot, stagedRoot, targetVersion)
	if err != nil {
		return stagedRoot, "", findings, err
	}
	pendingPath, err = writePendingUpdate(installRoot, stagedRoot, activatorPath, targetVersion, targetTag)
	if err != nil {
		return stagedRoot, "", findings, err
	}
	pendingPathForCleanup = pendingPath
	pending, _, err := readPendingUpdate(installRoot)
	if err != nil {
		return stagedRoot, "", findings, err
	}
	if err := validatePendingUpdate(installRoot, pending); err != nil {
		return stagedRoot, "", findings, err
	}
	if err := validatePendingUpdateAgainstActivatorSchema(pendingPath, activatorPath); err != nil {
		return stagedRoot, "", findings, err
	}
	if err := markPluginRestartRequired(installRoot, targetVersion, targetTag); err != nil {
		return stagedRoot, "", findings, err
	}
	committed = true
	return stagedRoot, pendingPath, findings, nil
}

func validatePendingUpdateAgainstActivatorSchema(pendingPath, activatorPath string) error {
	raw, err := readRegularFileWithMaxBytes(pendingPath, maxUpdateMetadataBytes)
	if err != nil {
		return err
	}
	schemaPath := filepath.Join(filepath.Dir(activatorPath), "schemas", pendingUpdateSchemaFile)
	if err := validateRawJSONAgainstSchema(raw, schemaPath); err != nil {
		return fmt.Errorf("pending update is incompatible with staged activator schema: %w", err)
	}
	return nil
}

func stageCandidateForWindowsHandoff(installRoot, candidateRoot, targetVersion string) (string, error) {
	return copyCandidateToSiblingStage(installRoot, candidateRoot, targetVersion, "pending")
}

func stagePendingActivator(installRoot, candidateRoot, targetVersion string) (string, error) {
	versionSegment, err := safeUpdateTargetVersionSegment(targetVersion)
	if err != nil {
		return "", err
	}
	binary := candidateCLIBinaryName()
	source := filepath.Join(candidateRoot, binary)
	if _, err := os.Stat(source); err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.Clean(installRoot))
	base := filepath.Base(filepath.Clean(installRoot))
	activatorRoot, err := os.MkdirTemp(parent, "."+base+".activator-"+versionSegment+"-")
	if err != nil {
		return "", err
	}
	if !sameFilesystemPath(filepath.Dir(activatorRoot), parent) {
		_ = os.RemoveAll(activatorRoot)
		return "", fmt.Errorf("pending activator path escapes install parent: %s", filepath.ToSlash(activatorRoot))
	}
	if err := rejectSymlinkAncestors(activatorRoot); err != nil {
		_ = os.RemoveAll(activatorRoot)
		return "", err
	}
	if err := ensureSecureDirMode(activatorRoot, 0o700); err != nil {
		_ = os.RemoveAll(activatorRoot)
		return "", err
	}
	destination := filepath.Join(activatorRoot, binary)
	if err := copyFileWithMaxBytes(source, destination, maxUpdateArchiveFileBytes); err != nil {
		_ = os.RemoveAll(activatorRoot)
		return "", err
	}
	if err := stagePendingActivatorSupportFiles(candidateRoot, activatorRoot); err != nil {
		_ = os.RemoveAll(activatorRoot)
		return "", err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destination, 0o755); err != nil {
			return "", err
		}
	}
	if err := validatePendingActivatorPath(installRoot, destination, targetVersion); err != nil {
		_ = os.RemoveAll(activatorRoot)
		return "", err
	}
	return destination, nil
}

func stagePendingActivatorSupportFiles(candidateRoot, activatorRoot string) error {
	source := filepath.Join(candidateRoot, "schemas")
	destination := filepath.Join(activatorRoot, "schemas")
	if err := copyLocalCandidateTreeWithBudget(source, destination, defaultUpdateArchiveExtractionBudget()); err != nil {
		return fmt.Errorf("stage pending activator schemas: %w", err)
	}
	for _, schemaFile := range []string{installMetadataSchemaFile, updateStateSchemaFile, pendingUpdateSchemaFile} {
		path := filepath.Join(destination, schemaFile)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("pending activator schema is unavailable: %s: %w", filepath.ToSlash(path), err)
		}
		if isSymlinkOrReparsePoint(path, info) {
			return fmt.Errorf("pending activator schema must not be a symlink or reparse point: %s", filepath.ToSlash(path))
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("pending activator schema must be a regular file: %s", filepath.ToSlash(path))
		}
	}
	return nil
}

func pendingUpdatePath(installRoot string) string {
	return filepath.Join(installRoot, ".slidex", "pending_update.json")
}

func readPendingUpdate(installRoot string) (*pendingUpdate, string, error) {
	path := pendingUpdatePath(installRoot)
	raw, err := readRegularFileWithMaxBytes(path, maxUpdateMetadataBytes)
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
	canonicalVersion, canonicalTag, err := canonicalUpdateTargetInputs(targetVersion, targetTag)
	if err != nil {
		return "", err
	}
	stagedDigest, err := candidateTreeManifestDigest(stagedRoot)
	if err != nil {
		return "", err
	}
	activatorDigest, err := sha256FileWithMaxBytes(activatorPath, maxUpdateArchiveFileBytes)
	if err != nil {
		return "", err
	}
	path := pendingUpdatePath(installRoot)
	pending := pendingUpdate{
		SchemaVersion:            pendingUpdateSchemaVersion,
		ToolName:                 toolName,
		TargetVersion:            canonicalVersion,
		TargetTag:                canonicalTag,
		InstallRoot:              filepath.ToSlash(installRoot),
		StagedRoot:               filepath.ToSlash(stagedRoot),
		StagedRootManifestSHA256: stagedDigest,
		ActivatorPath:            filepath.ToSlash(activatorPath),
		ActivatorSHA256:          activatorDigest,
		ActivationCommand:        pendingActivationCommand(activatorPath, installRoot),
		Reason:                   "Windows may lock the running slidex executable; activate this staged bundle on next run.",
		CreatedAt:                time.Now().UTC().Format(time.RFC3339),
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
	canonicalVersion, canonicalTag, err := canonicalUpdateTargetInputs(pending.TargetVersion, pending.TargetTag)
	if err != nil {
		result.Status = "pending-invalid"
		result.CandidateValidation = append(result.CandidateValidation, fail("update.pending_handoff", err.Error(), filepath.ToSlash(pendingUpdatePath(status.InstallRoot))))
		return result, nil
	}
	pending.TargetVersion = canonicalVersion
	pending.TargetTag = canonicalTag
	result.TargetVersion = canonicalVersion
	result.TargetTag = canonicalTag
	stagedRoot := filepath.FromSlash(pending.StagedRoot)
	result.CandidateValidation = validateCandidateBundleForStatus(stagedRoot, pending.TargetVersion, status.Channel)
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
	if strings.TrimSpace(pending.StagedRootManifestSHA256) == "" {
		return errors.New("pending update stagedRootManifestSha256 is required")
	}
	if !isSHA256Hex(pending.StagedRootManifestSHA256) {
		return errors.New("pending update stagedRootManifestSha256 must be a SHA-256 hex digest")
	}
	if strings.TrimSpace(pending.ActivatorPath) == "" {
		return errors.New("pending update activatorPath is required")
	}
	if strings.TrimSpace(pending.ActivatorSHA256) == "" {
		return errors.New("pending update activatorSha256 is required")
	}
	if !isSHA256Hex(pending.ActivatorSHA256) {
		return errors.New("pending update activatorSha256 must be a SHA-256 hex digest")
	}
	activatorPath := filepath.FromSlash(pending.ActivatorPath)
	if err := validatePendingActivatorPath(installRoot, activatorPath, pending.TargetVersion); err != nil {
		return err
	}
	actualActivatorDigest, err := sha256FileWithMaxBytes(activatorPath, maxUpdateArchiveFileBytes)
	if err != nil {
		return fmt.Errorf("pending activator digest validation failed: %w", err)
	}
	if !strings.EqualFold(actualActivatorDigest, pending.ActivatorSHA256) {
		return fmt.Errorf("pending activator digest mismatch: got %s, want %s", actualActivatorDigest, pending.ActivatorSHA256)
	}
	if pending.InstallRoot != "" && !sameFilesystemPath(filepath.FromSlash(pending.InstallRoot), installRoot) {
		return fmt.Errorf("pending update installRoot must be %s, got %s", filepath.ToSlash(installRoot), pending.InstallRoot)
	}
	stagedRoot := filepath.Clean(filepath.FromSlash(pending.StagedRoot))
	if err := validatePendingStagedRootPath(installRoot, stagedRoot, pending.TargetVersion); err != nil {
		return err
	}
	info, err := os.Lstat(stagedRoot)
	if err != nil {
		return fmt.Errorf("pending staged root is unavailable: %w", err)
	}
	if isSymlinkOrReparsePoint(stagedRoot, info) {
		return fmt.Errorf("pending staged root must not be a symlink or reparse point: %s", filepath.ToSlash(stagedRoot))
	}
	if !info.IsDir() {
		return fmt.Errorf("pending staged root must be a directory: %s", filepath.ToSlash(stagedRoot))
	}
	actualDigest, err := candidateTreeManifestDigest(stagedRoot)
	if err != nil {
		return fmt.Errorf("pending staged root validation failed: %w", err)
	}
	if !strings.EqualFold(actualDigest, pending.StagedRootManifestSHA256) {
		return fmt.Errorf("pending staged root manifest digest mismatch: got %s, want %s", actualDigest, pending.StagedRootManifestSHA256)
	}
	stagedBinaryDigest, err := sha256FileWithMaxBytes(filepath.Join(stagedRoot, candidateCLIBinaryName()), maxUpdateArchiveFileBytes)
	if err != nil {
		return fmt.Errorf("pending staged candidate binary digest validation failed: %w", err)
	}
	if !strings.EqualFold(stagedBinaryDigest, pending.ActivatorSHA256) {
		return fmt.Errorf("pending activator digest must match staged candidate binary digest: got %s, want %s", pending.ActivatorSHA256, stagedBinaryDigest)
	}
	return nil
}

func validatePendingActivatorPath(installRoot, activatorPath, targetVersion string) error {
	versionSegment, err := safeUpdateTargetVersionSegment(targetVersion)
	if err != nil {
		return err
	}
	installRoot = filepath.Clean(installRoot)
	activatorPath = filepath.Clean(activatorPath)
	parent := filepath.Dir(installRoot)
	activatorRoot := filepath.Dir(activatorPath)
	if !sameFilesystemPath(filepath.Dir(activatorRoot), parent) {
		return fmt.Errorf("pending activator must be under install parent %s, got %s", filepath.ToSlash(parent), filepath.ToSlash(activatorPath))
	}
	base := filepath.Base(installRoot)
	prefix := "." + base + ".activator-" + versionSegment + "-"
	if !strings.HasPrefix(filepath.Base(activatorRoot), prefix) {
		return fmt.Errorf("pending activator must use prefix %q, got %s", prefix, filepath.ToSlash(activatorPath))
	}
	if filepath.Base(activatorPath) != pendingActivatorBinaryName() {
		return fmt.Errorf("pending activator must be %s, got %s", pendingActivatorBinaryName(), filepath.ToSlash(activatorPath))
	}
	if err := rejectSymlinkAncestors(activatorRoot); err != nil {
		return err
	}
	info, err := os.Lstat(activatorPath)
	if err != nil {
		return fmt.Errorf("pending activator is unavailable: %w", err)
	}
	if isSymlinkOrReparsePoint(activatorPath, info) {
		return fmt.Errorf("pending activator must not be a symlink or reparse point: %s", filepath.ToSlash(activatorPath))
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("pending activator must be a regular file: %s", filepath.ToSlash(activatorPath))
	}
	return nil
}

func pendingActivatorBinaryName() string {
	if runtime.GOOS == "windows" {
		return "slidex.exe"
	}
	return "slidex"
}

func validatePendingStagedRootPath(installRoot, stagedRoot, targetVersion string) error {
	versionSegment, err := safeUpdateTargetVersionSegment(targetVersion)
	if err != nil {
		return err
	}
	installRoot = filepath.Clean(installRoot)
	stagedRoot = filepath.Clean(stagedRoot)
	parent := filepath.Dir(installRoot)
	if !sameFilesystemPath(filepath.Dir(stagedRoot), parent) {
		return fmt.Errorf("pending staged root must be under install parent %s, got %s", filepath.ToSlash(parent), filepath.ToSlash(stagedRoot))
	}
	base := filepath.Base(installRoot)
	prefix := "." + base + ".pending-" + versionSegment + "-"
	if !strings.HasPrefix(filepath.Base(stagedRoot), prefix) {
		return fmt.Errorf("pending staged root must use prefix %q, got %s", prefix, filepath.ToSlash(stagedRoot))
	}
	return nil
}

func isSHA256Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

var beforeUpdateStagedCandidateValidation func(stagedRoot string) error

func replaceInstallRootWithCandidate(installRoot, candidateRoot, targetVersion, statusChannel string, options updateApplyOptions) (stagedRoot, backupRoot string, findings []qaFinding, err error) {
	stagedRoot, err = copyCandidateToSiblingStage(installRoot, candidateRoot, targetVersion, "staged")
	if err != nil {
		return stagedRoot, backupRoot, findings, err
	}
	if beforeUpdateStagedCandidateValidation != nil {
		if err := beforeUpdateStagedCandidateValidation(stagedRoot); err != nil {
			_ = os.RemoveAll(stagedRoot)
			return stagedRoot, backupRoot, findings, err
		}
	}
	findings = validateCandidateBundleForApply(stagedRoot, targetVersion, statusChannel, options.ExecuteCandidateChecks)
	if hasFailures(findings) {
		_ = os.RemoveAll(stagedRoot)
		return stagedRoot, backupRoot, findings, nil
	}
	backupRoot, err = activateStagedInstallRoot(installRoot, stagedRoot, targetVersion)
	return stagedRoot, backupRoot, findings, err
}

func copyCandidateToSiblingStage(installRoot, candidateRoot, targetVersion, kind string) (string, error) {
	versionSegment, err := safeUpdateTargetVersionSegment(targetVersion)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.Clean(installRoot))
	base := filepath.Base(filepath.Clean(installRoot))
	if err := validateLocalCandidateTree(candidateRoot); err != nil {
		return "", err
	}
	stagedRoot, err := os.MkdirTemp(parent, "."+base+"."+kind+"-"+versionSegment+"-")
	if err != nil {
		return "", err
	}
	if !sameFilesystemPath(filepath.Dir(stagedRoot), parent) {
		_ = os.RemoveAll(stagedRoot)
		return "", fmt.Errorf("candidate staging path escapes install parent: %s", filepath.ToSlash(stagedRoot))
	}
	if err := copyLocalCandidateTreeWithBudget(candidateRoot, stagedRoot, defaultUpdateArchiveExtractionBudget()); err != nil {
		_ = os.RemoveAll(stagedRoot)
		return stagedRoot, err
	}
	return stagedRoot, nil
}

func activateStagedInstallRoot(installRoot, stagedRoot, targetVersion string) (backupRoot string, err error) {
	versionSegment, err := safeUpdateTargetVersionSegment(targetVersion)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.Clean(installRoot))
	base := filepath.Base(filepath.Clean(installRoot))
	backupRoot, err = reserveUniqueSiblingPath(parent, "."+base+".backup-"+versionSegment+"-")
	if err != nil {
		return backupRoot, err
	}
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

func reserveUniqueSiblingPath(parent, pattern string) (string, error) {
	path, err := os.MkdirTemp(parent, pattern)
	if err != nil {
		return "", err
	}
	if !sameFilesystemPath(filepath.Dir(path), parent) {
		_ = os.RemoveAll(path)
		return "", fmt.Errorf("reserved sibling path escapes install parent: %s", filepath.ToSlash(path))
	}
	if err := os.Remove(path); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	return path, nil
}

func updateInstallMetadataAfterActivation(installRoot, targetVersion, targetTag, channel string) error {
	canonicalVersion, canonicalTag, err := canonicalUpdateTargetInputs(targetVersion, targetTag)
	if err != nil {
		return err
	}
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
	metadata.Version = canonicalVersion
	metadata.Channel = channel
	if canonicalTag != "" {
		metadata.Tag = canonicalTag
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

func installedReleaseMetadataBindingIssue(installRoot, metadataPath string, metadata *installMetadata) string {
	expectedPath, err := canonicalUpdateMetadataPath(installMetadataPath(installRoot))
	if err != nil {
		return "canonical install metadata path could not be resolved: " + err.Error()
	}
	actualPath, err := canonicalUpdateMetadataPath(metadataPath)
	if err != nil {
		return "install metadata path could not be resolved: " + err.Error()
	}
	if !sameFilesystemPath(actualPath, expectedPath) {
		return "install metadata path must be " + filepath.ToSlash(expectedPath) + ", got " + filepath.ToSlash(actualPath)
	}
	if metadata != nil && strings.TrimSpace(metadata.InstallRoot) != "" {
		recordedRoot, err := canonicalUpdateInstallRoot(filepath.FromSlash(metadata.InstallRoot))
		if err != nil {
			return "install metadata installRoot could not be resolved: " + err.Error()
		}
		if !sameFilesystemPath(recordedRoot, installRoot) {
			return "install metadata records a different install root " + filepath.ToSlash(recordedRoot) + "; resolved install root is " + filepath.ToSlash(installRoot)
		}
	}
	return ""
}

func validateInstallMetadataSchema(metadata *installMetadata) error {
	if metadata == nil {
		return errors.New("install metadata is missing")
	}
	schemaPath, err := bundledSchemaPathStrict(installMetadataSchemaFile)
	if err != nil {
		return err
	}
	return validateInstallMetadataAgainstSchemaPath(metadata, schemaPath)
}

func validateInstallMetadataAgainstSchemaPath(metadata *installMetadata, schemaPath string) error {
	if metadata == nil {
		return errors.New("install metadata is missing")
	}
	if len(metadata.Raw) > 0 {
		return validateRawJSONAgainstSchema(metadata.Raw, schemaPath)
	}
	return validatePayloadAgainstSchema(*metadata, schemaPath)
}

var resolveExecutableInstallRoot = executableInstallRoot

func executableInstallRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe), nil
}

func defaultInstallRoot() string {
	root, err := executableInstallRoot()
	if err == nil && root != "" {
		return root
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
	path, err := bundledSchemaPathStrict(schemaName)
	if err == nil {
		return path
	}
	return missingBundledSchemaPath(schemaName)
}

func bundledSchemaPathStrict(schemaName string) (string, error) {
	rel := filepath.Join("schemas", filepath.FromSlash(schemaName))
	var firstErr error
	if installRoot, err := resolveExecutableInstallRoot(); err == nil && installRoot != "" {
		installCandidate := filepath.Join(installRoot, rel)
		if _, err := os.Stat(installCandidate); err == nil {
			return installCandidate, nil
		} else if firstErr == nil {
			firstErr = err
		}
	} else if err != nil {
		firstErr = err
	}
	if candidate := resolveSourceRelativePath(rel); candidate != "" {
		return candidate, nil
	}
	return "", trustedSchemaResolutionError(schemaName, firstErr)
}

func missingBundledSchemaPath(schemaName string) string {
	rel := filepath.Join("schemas", filepath.FromSlash(schemaName))
	root := string(filepath.Separator)
	if volume := filepath.VolumeName(mustAbs(".")); volume != "" {
		root = volume + string(filepath.Separator)
	}
	return filepath.Join(root, "__slidex_missing_trusted_builtin_schema__", rel)
}

func validatePayloadAgainstBundledSchema(payload any, schemaName string) error {
	schemaPath, err := bundledSchemaPathStrict(schemaName)
	if err != nil {
		return err
	}
	return validatePayloadAgainstSchema(payload, schemaPath)
}

func validateRawJSONAgainstBundledSchema(raw []byte, schemaName string) error {
	schemaPath, err := bundledSchemaPathStrict(schemaName)
	if err != nil {
		return err
	}
	return validateRawJSONAgainstSchema(raw, schemaPath)
}

func readInstallMetadata(path string) (*installMetadata, error) {
	raw, err := readRegularFileWithMaxBytes(path, maxUpdateMetadataBytes)
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
	raw, err := readRegularFileWithMaxBytes(path, maxUpdateMetadataBytes)
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
	if !isReleasePackageVersion(version) {
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
	if !isReleasePackageVersion(version) {
		return updateChannelLocalDevelopment
	}
	switch {
	case canaryPackageVersionPattern.MatchString(version):
		return updateChannelCanary
	case stablePackageVersionPattern.MatchString(version):
		return updateChannelProduction
	default:
		return updateChannelLocalDevelopment
	}
}

func isReleasePackageVersion(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > maxReleasePackageVersionLength {
		return false
	}
	if strings.HasPrefix(version, "v") {
		return false
	}
	if !stablePackageVersionPattern.MatchString(version) && !canaryPackageVersionPattern.MatchString(version) {
		return false
	}
	_, ok := parseReleaseBaseVersionParts(version)
	return ok
}

func releaseBaseVersion(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if canaryPackageVersionPattern.MatchString(version) {
		return strings.SplitN(version, "-", 2)[0]
	}
	return version
}

func fetchUpdateReleases(ctx context.Context, apiURL string) ([]updateRelease, error) {
	return fetchUpdateReleasesUntil(ctx, apiURL, nil)
}

func fetchUpdateReleaseForStatus(ctx context.Context, apiURL string, status updateStatus) (updateRelease, error) {
	releases, err := fetchUpdateReleasesUntil(ctx, apiURL, func(releases []updateRelease) bool {
		_, err := selectUpdateReleaseForStatus(status, releases)
		return err == nil
	})
	if err != nil {
		return updateRelease{}, err
	}
	return selectUpdateReleaseForStatus(status, releases)
}

func fetchUpdateReleasesUntil(ctx context.Context, apiURL string, stop func([]updateRelease) bool) ([]updateRelease, error) {
	ctx, cancel := contextWithDefaultTimeout(ctx, updateReleaseFetchTimeout)
	defer cancel()
	var releases []updateRelease
	nextURL := strings.TrimSpace(apiURL)
	if nextURL == "" {
		return nil, errors.New("update API URL is required")
	}
	for page := 0; nextURL != ""; page++ {
		if page >= maxUpdateReleasePages {
			return nil, fmt.Errorf("GitHub Releases API pagination exceeded %d pages", maxUpdateReleasePages)
		}
		pageReleases, linkHeader, err := fetchUpdateReleasePage(ctx, nextURL)
		if err != nil {
			return nil, err
		}
		releases = append(releases, pageReleases...)
		if stop != nil && stop(releases) {
			return releases, nil
		}
		nextURL = nextUpdateReleasePageURL(linkHeader)
	}
	return releases, nil
}

func fetchUpdateReleasePage(ctx context.Context, apiURL string) ([]updateRelease, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "slidex-update/"+toolVersion)
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("GitHub Releases API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, "", err
	}
	releases, err := parseUpdateReleases(raw)
	if err != nil {
		return nil, "", err
	}
	return releases, resp.Header.Get("Link"), nil
}

func nextUpdateReleasePageURL(linkHeader string) string {
	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end <= start+1 {
			continue
		}
		return strings.TrimSpace(part[start+1 : end])
	}
	return ""
}

func contextWithDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
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
			TagName:     "v" + version,
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
	if channel == updateChannelCanary && isReleasePackageVersion(currentVersion) && canaryPackageVersionPattern.MatchString(currentVersion) {
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

func parseReleaseBaseVersionParts(version string) ([3]uint64, bool) {
	var parts [3]uint64
	version = releaseBaseVersion(version)
	if !stablePackageVersionPattern.MatchString(version) {
		return parts, false
	}
	for i, part := range strings.Split(version, ".") {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return [3]uint64{}, false
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
	findings := validateCandidateBundleStatic(root, expectedVersion)
	if hasFailures(findings) {
		return findings
	}
	return append(findings, validateCandidateBundleDynamicChecks(root, expectedVersion)...)
}

func validateCandidateBundleForStatus(root, expectedVersion, statusChannel string) []qaFinding {
	metadataPath := filepath.Join(root, ".slidex", "install.json")
	findings := validateCandidateBundleStatic(root, expectedVersion)
	findings = append(findings, validateCandidateChannelForStatus(statusChannel, expectedVersion, metadataPath)...)
	if metadata, err := readInstallMetadata(metadataPath); err == nil && metadata.Channel != statusChannel {
		findings = append(findings, fail("update.candidate_channel", "candidate channel must remain "+statusChannel+", got "+metadata.Channel, filepath.ToSlash(metadataPath)))
	}
	return findings
}

func validateCandidateBundleForApply(root, expectedVersion, statusChannel string, executeCandidateChecks bool) []qaFinding {
	findings := validateCandidateBundleForStatus(root, expectedVersion, statusChannel)
	if hasFailures(findings) || !executeCandidateChecks {
		return findings
	}
	return append(findings, validateCandidateBundleDynamicChecks(root, expectedVersion)...)
}

func validateCandidateBundleStatic(root, expectedVersion string) []qaFinding {
	root = filepath.Clean(root)
	expectedVersion = strings.TrimPrefix(strings.TrimSpace(expectedVersion), "v")
	expectedBaseVersion := releaseBaseVersion(expectedVersion)
	expectedChannel := channelFromPackageVersion(expectedVersion)
	var findings []qaFinding
	if err := validateLocalCandidateTree(root); err != nil {
		findings = append(findings, fail("update.candidate_tree", "candidate tree validation failed: "+err.Error(), filepath.ToSlash(root)))
	}
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
	requiredFiles, requiredFilesErr := requiredCandidateRuntimeFiles()
	if requiredFilesErr != nil {
		findings = append(findings, fail("update.candidate_runtime", "trusted candidate runtime file set could not be resolved: "+requiredFilesErr.Error(), filepath.ToSlash(filepath.Join(root, "schemas"))))
	}
	for _, rel := range requiredFiles {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			findings = append(findings, fail("update.candidate_runtime", "missing candidate runtime file: "+err.Error(), filepath.ToSlash(path)))
			continue
		}
		if isSymlinkOrReparsePoint(path, info) {
			findings = append(findings, fail("update.candidate_runtime", "candidate runtime file must not be a symlink or reparse point", filepath.ToSlash(path)))
		} else if !info.Mode().IsRegular() {
			findings = append(findings, fail("update.candidate_runtime", "candidate runtime file must be a regular file", filepath.ToSlash(path)))
		}
	}
	findings = append(findings, validateCandidateSchemaFiles(root)...)
	findings = append(findings, validateCandidateBundleMutableStatePaths(root)...)
	versionPath := filepath.Join(root, "VERSION")
	versionRaw, versionErr := readRegularFileWithMaxBytes(versionPath, maxUpdateVersionBytes)
	if versionErr != nil {
		findings = append(findings, fail("update.candidate_version", "candidate VERSION could not be read safely: "+versionErr.Error(), filepath.ToSlash(versionPath)))
	} else if version := strings.TrimSpace(string(versionRaw)); version != expectedBaseVersion {
		findings = append(findings, fail("update.candidate_version", fmt.Sprintf("candidate VERSION must be %s, got %s", expectedBaseVersion, firstNonEmpty(version, "missing")), filepath.ToSlash(versionPath)))
	}
	metadataPath := filepath.Join(root, ".slidex", "install.json")
	if metadata, err := readInstallMetadata(metadataPath); err != nil {
		findings = append(findings, fail("update.candidate_install_metadata", err.Error(), filepath.ToSlash(metadataPath)))
	} else {
		if err := validateInstallMetadataSchema(metadata); err != nil {
			findings = append(findings, fail("update.candidate_install_metadata", err.Error(), filepath.ToSlash(metadataPath)))
		}
		candidateSchemaPath := filepath.Join(root, "schemas", installMetadataSchemaFile)
		if err := validateInstallMetadataAgainstSchemaPath(metadata, candidateSchemaPath); err != nil {
			findings = append(findings, fail("update.candidate_install_metadata", "candidate-local schema validation failed: "+err.Error(), filepath.ToSlash(metadataPath)))
		}
		if issue := installedReleaseMetadataIssue(metadata); issue != "" {
			findings = append(findings, fail("update.candidate_install_metadata", issue, filepath.ToSlash(metadataPath)))
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
		findings = append(findings, validateSlidexPluginManifestContract(manifest, expectedBaseVersion, "update.candidate_plugin_manifest", filepath.ToSlash(manifestPath))...)
	}
	mcpPath := filepath.Join(root, "plugins", "slidex", ".mcp.json")
	if mcpConfig, err := readCandidateJSON(mcpPath); err != nil {
		findings = append(findings, fail("update.candidate_plugin_mcp", err.Error(), filepath.ToSlash(mcpPath)))
	} else {
		findings = append(findings, validateSlidexPluginMCPContract(mcpConfig, "update.candidate_plugin_mcp", filepath.ToSlash(mcpPath))...)
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
	} else {
		findings = append(findings, validateCandidateMarketplaceContract(marketplace, filepath.ToSlash(marketplacePath))...)
	}
	binary := "slidex"
	if runtime.GOOS == "windows" {
		binary = "slidex.exe"
	}
	binaryPath := filepath.Join(root, binary)
	if info, err := os.Lstat(binaryPath); err != nil {
		findings = append(findings, fail("update.candidate_binary", "missing candidate CLI binary: "+err.Error(), filepath.ToSlash(binaryPath)))
	} else if isSymlinkOrReparsePoint(binaryPath, info) {
		findings = append(findings, fail("update.candidate_binary", "candidate CLI binary must not be a symlink or reparse point", filepath.ToSlash(binaryPath)))
	} else if !info.Mode().IsRegular() {
		findings = append(findings, fail("update.candidate_binary", "candidate CLI binary must be a regular file", filepath.ToSlash(binaryPath)))
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		findings = append(findings, fail("update.candidate_binary", "candidate CLI binary must set owner execute permission", filepath.ToSlash(binaryPath)))
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		findings = append(findings, fail("update.candidate_binary", fmt.Sprintf("candidate CLI binary must not be group/world writable: mode %04o", info.Mode().Perm()), filepath.ToSlash(binaryPath)))
	}
	return findings
}

func requiredCandidateRuntimeFiles() ([]string, error) {
	files := []string{
		".agents/skills/slidex/SKILL.md",
		".agents/skills/slidex/references/commands.md",
		"internal/codex/protocol/codex-cli-0.138.0/protocol_manifest.json",
		"internal/codex/protocol/codex-cli-0.138.0/schema/ClientRequest.json",
		"internal/codex/protocol/codex-cli-0.138.0/schema/ServerRequest.json",
		"plugins/slidex/.mcp.json",
		"plugins/slidex/README.md",
		"plugins/slidex/hooks/manifest.json",
		"plugins/slidex/skills/slidex/SKILL.md",
		"plugins/slidex/skills/slidex-finalize/SKILL.md",
		"plugins/slidex/skills/slidex-run/SKILL.md",
		"plugins/slidex/skills/slidex-start/SKILL.md",
	}
	schemaFiles, err := requiredCandidateSchemaFiles()
	if err != nil {
		return files, err
	}
	files = append(files, schemaFiles...)
	sort.Strings(files)
	return files, nil
}

func requiredCandidateSchemaFiles() ([]string, error) {
	dir, err := trustedBundledSchemaDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}
		files = append(files, filepath.ToSlash(filepath.Join("schemas", entry.Name())))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no built-in schema files found in %s", filepath.ToSlash(dir))
	}
	sort.Strings(files)
	return files, nil
}

func trustedBundledSchemaDir() (string, error) {
	path, err := bundledSchemaPathStrict(installMetadataSchemaFile)
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

func validateCandidateSchemaFiles(root string) []qaFinding {
	required, err := requiredCandidateSchemaFiles()
	if err != nil {
		return []qaFinding{fail("update.candidate_schema", "trusted candidate schema set could not be resolved: "+err.Error(), filepath.ToSlash(filepath.Join(root, "schemas")))}
	}
	var findings []qaFinding
	for _, rel := range required {
		path := filepath.Join(root, filepath.FromSlash(rel))
		raw, err := readRegularFileWithMaxBytes(path, maxProjectSchemaBytes)
		if err != nil {
			findings = append(findings, fail("update.candidate_schema", "candidate schema could not be read safely: "+err.Error(), filepath.ToSlash(path)))
			continue
		}
		if strings.TrimSpace(string(raw)) == "" {
			findings = append(findings, fail("update.candidate_schema", "candidate schema must not be empty", filepath.ToSlash(path)))
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			findings = append(findings, fail("update.candidate_schema", "candidate schema is not valid JSON: "+err.Error(), filepath.ToSlash(path)))
			continue
		}
		if metadataString(schema["$schema"]) == "" {
			findings = append(findings, fail("update.candidate_schema", "candidate schema must declare $schema", filepath.ToSlash(path)))
			continue
		}
		if _, schemaFindings := compileFullJSONSchema(schema, filepath.ToSlash(path)); hasFailures(schemaFindings) {
			for _, finding := range schemaFindings {
				findings = append(findings, fail("update.candidate_schema", finding.Message, filepath.ToSlash(path)))
			}
		}
	}
	return findings
}

func validateCandidateBundleMutableStatePaths(root string) []qaFinding {
	var findings []qaFinding
	for _, rel := range []string{
		".slidex/update_state.json",
		".slidex/pending_update.json",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Lstat(path); err == nil {
			findings = append(findings, fail("update.candidate_mutable_state", "candidate must not include updater mutable state path: "+rel, filepath.ToSlash(path)))
		} else if !errors.Is(err, os.ErrNotExist) {
			findings = append(findings, fail("update.candidate_mutable_state", "candidate mutable state path could not be inspected: "+err.Error(), filepath.ToSlash(path)))
		}
	}
	return findings
}

func validateCandidateBundleDynamicChecks(root, expectedVersion string) []qaFinding {
	root = filepath.Clean(root)
	expectedVersion = strings.TrimPrefix(strings.TrimSpace(expectedVersion), "v")
	expectedBaseVersion := releaseBaseVersion(expectedVersion)
	binary := candidateCLIBinaryName()
	binaryPath := filepath.Join(root, binary)
	info, err := os.Lstat(binaryPath)
	if err != nil {
		return []qaFinding{fail("update.candidate_binary", "missing candidate CLI binary: "+err.Error(), filepath.ToSlash(binaryPath))}
	}
	if isSymlinkOrReparsePoint(binaryPath, info) {
		return []qaFinding{fail("update.candidate_binary", "candidate CLI binary must not be a symlink or reparse point", filepath.ToSlash(binaryPath))}
	}
	if !info.Mode().IsRegular() {
		return []qaFinding{fail("update.candidate_binary", "candidate CLI binary must be a regular file", filepath.ToSlash(binaryPath))}
	}
	if version, err := candidateBinaryVersion(binaryPath); err != nil {
		return []qaFinding{fail("update.candidate_binary_version", "candidate CLI version command failed: "+err.Error(), filepath.ToSlash(binaryPath))}
	} else if version != expectedBaseVersion {
		return []qaFinding{fail("update.candidate_binary_version", "candidate CLI version must be "+expectedBaseVersion+", got "+version, filepath.ToSlash(binaryPath))}
	}
	if doctorStatus, err := candidateDoctorStatus(root, binaryPath); err != nil {
		return []qaFinding{fail("update.candidate_doctor", "candidate doctor failed: "+err.Error(), filepath.ToSlash(binaryPath))}
	} else if doctorStatus != "pass" {
		return []qaFinding{fail("update.candidate_doctor", "candidate doctor status must be pass, got "+doctorStatus, filepath.ToSlash(binaryPath))}
	}
	return nil
}

func candidateBinaryVersion(path string) (string, error) {
	return candidateBinaryVersionWithMaxOutput(path, maxExternalCommandOutputBytes)
}

func candidateBinaryVersionWithMaxOutput(path string, maxOutputBytes int64) (string, error) {
	out, err := runBufferedCommandWithInputEnvAndMaxOutput(8*time.Second, maxOutputBytes, "", nil, candidateDynamicCheckEnv(), path, "version")
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
	return candidateDoctorStatusWithMaxOutput(root, binaryPath, maxExternalCommandOutputBytes)
}

func candidateDoctorStatusWithMaxOutput(root, binaryPath string, maxOutputBytes int64) (string, error) {
	out, err := runBufferedCommandWithInputEnvAndMaxOutput(30*time.Second, maxOutputBytes, root, nil, candidateDynamicCheckEnv(), binaryPath, "doctor", "--json")
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		return "", err
	}
	return metadataString(report["status"]), nil
}

func candidateDynamicCheckEnv() []string {
	return sanitizedSensitiveSubprocessEnv(os.Environ())
}

func readCandidateJSON(path string) (map[string]any, error) {
	raw, err := readRegularFileWithMaxBytes(path, maxUpdateCandidateJSONBytes)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func validateCandidateMarketplaceContract(manifest map[string]any, path string) []qaFinding {
	plugins, _ := manifest["plugins"].([]any)
	var findings []qaFinding
	found := 0
	for _, raw := range plugins {
		plugin, _ := raw.(map[string]any)
		if metadataString(plugin["name"]) != toolName {
			continue
		}
		found++
		source, _ := plugin["source"].(map[string]any)
		if metadataString(source["source"]) != "local" {
			findings = append(findings, fail("update.candidate_marketplace", "candidate marketplace slidex source must be local", path))
		}
		if metadataString(source["path"]) != "./plugins/slidex" {
			findings = append(findings, fail("update.candidate_marketplace", "candidate marketplace must point slidex to ./plugins/slidex", path))
		}
	}
	if found == 0 {
		findings = append(findings, fail("update.candidate_marketplace", "candidate marketplace must include slidex plugin entry", path))
	} else if found > 1 {
		findings = append(findings, fail("update.candidate_marketplace", "candidate marketplace must not include duplicate slidex plugin entries", path))
	}
	return findings
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
