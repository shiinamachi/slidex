package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	workbenchManifestName        = "workbench_manifest.json"
	workbenchDraftName           = "workbench_draft.json"
	workbenchBrowserEvidenceName = "workbench_browser_evidence.json"
	workbenchBrowserVerifyName   = "workbench_browser_evidence_verification.json"
	workbenchSaveSmokeName       = "workbench_save_smoke.json"
	workbenchSaveSmokeScreenshot = "workbench_save_smoke.png"
	workbenchBrowserScreenshot   = "workbench_browser_screenshot"
	workbenchGenerationLogName   = "workbench_generation.log"
	workbenchAssetManifestName   = "workbench_assets/slidex-workbench-build.json"
	workbenchControlName         = ".workbench_control.json"
	workbenchLockName            = "workbench.lock"
	workbenchScreenshotMaxBytes  = 20 * 1024 * 1024
	workbenchScreenshotMaxPixels = maxRenderedPNGPixels
	workbenchBrowserOpenEnv      = "SLIDEX_BROWSER_OPEN"
	workbenchTokenEnv            = "SLIDEX_WORKBENCH_TOKEN"
	workbenchShutdownTokenEnv    = "SLIDEX_WORKBENCH_SHUTDOWN_TOKEN"
	workbenchReadinessTokenEnv   = "SLIDEX_WORKBENCH_READINESS_TOKEN"
	workbenchReadinessHeader     = "X-Slidex-Workbench-Readiness-Token"
	workbenchJSONBodyMaxBytes    = 64 * 1024
	workbenchLogMaxBytes         = 1024 * 1024
	workbenchLogTruncationMarker = "\n[slidex] workbench log output exceeded maximum allowed size; further output truncated\n"
)

var (
	errWorkbenchJSONTrailing  = errors.New("request body must contain exactly one JSON value")
	workbenchLockWaitTimeout  = 10 * time.Second
	workbenchLockRetryDelay   = 50 * time.Millisecond
	workbenchLockStaleAfter   = 5 * time.Second
	workbenchLockSchemaMarker = "workbench-lock-v1"
	signalWorkbenchProcessFn  = signalWorkbenchProcess
	killWorkbenchProcessFn    = killWorkbenchProcess
)

type workbenchAssetManifest struct {
	SchemaVersion string   `json:"schemaVersion"`
	SourcePackage string   `json:"sourcePackage"`
	SourceVersion string   `json:"sourceVersion"`
	Framework     string   `json:"framework"`
	CSR           bool     `json:"csr"`
	EntryHTML     string   `json:"entryHtml"`
	ModulePreload []string `json:"modulePreloads"`
	InlineScripts []string `json:"inlineScripts"`
	Scripts       []string `json:"scripts"`
	Styles        []string `json:"styles"`
	SourceSHA256  string   `json:"sourceSha256"`
}

var deckIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type workbenchBrowserOpenMode string

const (
	workbenchBrowserOpenStructured workbenchBrowserOpenMode = "structured"
	workbenchBrowserOpenAgent      workbenchBrowserOpenMode = "agent"
	workbenchBrowserOpenManual     workbenchBrowserOpenMode = "manual"
)

type deckBootstrapResult struct {
	Workspace string `json:"workspace"`
	DeckID    string `json:"deckId"`
	DeckDir   string `json:"deckDir"`
	Status    string `json:"status"`
}

type workbenchManifest struct {
	SchemaVersion       string            `json:"schemaVersion"`
	ToolName            string            `json:"toolName"`
	ToolVersion         string            `json:"toolVersion"`
	Status              string            `json:"status"`
	DeckID              string            `json:"deckId"`
	DeckDir             string            `json:"deckDir"`
	OutDir              string            `json:"outDir"`
	Workspace           string            `json:"workspace"`
	Host                string            `json:"host"`
	Port                int               `json:"port"`
	URL                 string            `json:"url"`
	SessionID           string            `json:"sessionId"`
	TokenSHA256         string            `json:"tokenSha256"`
	TokenRedacted       bool              `json:"tokenRedacted"`
	PID                 int               `json:"pid"`
	ServerBind          string            `json:"serverBind"`
	HealthPath          string            `json:"healthPath"`
	ReadinessPath       string            `json:"readinessPath"`
	CreatedAt           string            `json:"createdAt"`
	UpdatedAt           string            `json:"updatedAt"`
	BriefPath           string            `json:"briefPath,omitempty"`
	InputSavedAt        string            `json:"inputSavedAt,omitempty"`
	WizardCompletedAt   string            `json:"wizardCompletedAt,omitempty"`
	DraftSavedAt        string            `json:"draftSavedAt,omitempty"`
	DraftPath           string            `json:"draftPath,omitempty"`
	SavedFieldLengths   map[string]int    `json:"savedFieldLengths,omitempty"`
	GenerationStatus    string            `json:"generationStatus,omitempty"`
	GenerationStartedAt string            `json:"generationStartedAt,omitempty"`
	GenerationEndedAt   string            `json:"generationEndedAt,omitempty"`
	GenerationPID       int               `json:"generationPid,omitempty"`
	GenerationExitCode  int               `json:"generationExitCode,omitempty"`
	GenerationCommand   []string          `json:"generationCommand,omitempty"`
	GenerationLogPath   string            `json:"generationLogPath,omitempty"`
	BrowserOpenStrategy string            `json:"browserOpenStrategy"`
	Notes               []string          `json:"notes,omitempty"`
	Paths               map[string]string `json:"paths,omitempty"`
}

type workbenchDraft struct {
	SchemaVersion string             `json:"schemaVersion"`
	ToolName      string             `json:"toolName"`
	ToolVersion   string             `json:"toolVersion"`
	DeckID        string             `json:"deckId"`
	Status        string             `json:"status"`
	UpdatedAt     string             `json:"updatedAt"`
	Input         workbenchSaveInput `json:"input"`
}

type workbenchControl struct {
	SchemaVersion string `json:"schemaVersion"`
	ToolName      string `json:"toolName"`
	ToolVersion   string `json:"toolVersion"`
	SessionID     string `json:"sessionId"`
	DeckDir       string `json:"deckDir"`
	PID           int    `json:"pid"`
	Port          int    `json:"port"`
	URL           string `json:"url"`
	ShutdownKey   string `json:"shutdownKey"`
	ReadinessKey  string `json:"readinessKey"`
	CreatedAt     string `json:"createdAt"`
}

type workbenchBrowserEvidence struct {
	SchemaVersion       string              `json:"schemaVersion"`
	ToolName            string              `json:"toolName"`
	ToolVersion         string              `json:"toolVersion"`
	CodexVersion        string              `json:"codexVersion"`
	PluginName          string              `json:"pluginName"`
	PluginVersion       string              `json:"pluginVersion"`
	DeckID              string              `json:"deckId"`
	DeckDir             string              `json:"deckDir"`
	Status              string              `json:"status"`
	RecordedAt          string              `json:"recordedAt"`
	Inspector           string              `json:"inspector"`
	Surface             string              `json:"surface"`
	Invocation          string              `json:"invocation"`
	ThreadID            string              `json:"threadId,omitempty"`
	URL                 string              `json:"url"`
	SessionID           string              `json:"sessionId"`
	ServerBind          string              `json:"serverBind"`
	WorkbenchVisible    bool                `json:"workbenchVisible"`
	SavedInputVerified  bool                `json:"savedInputVerified"`
	TokenRedacted       bool                `json:"tokenRedacted"`
	BrowserOpenStrategy string              `json:"browserOpenStrategy"`
	Notes               string              `json:"notes,omitempty"`
	ManifestPath        string              `json:"manifestPath"`
	BriefPath           string              `json:"briefPath"`
	DraftPath           string              `json:"draftPath"`
	EvidencePath        string              `json:"evidencePath"`
	BrowserScreenshot   *artifact           `json:"browserScreenshot,omitempty"`
	VerifiedFiles       map[string]artifact `json:"verifiedFiles"`
}

type workbenchBrowserEvidenceVerification struct {
	SchemaVersion     string              `json:"schemaVersion"`
	ToolName          string              `json:"toolName"`
	ToolVersion       string              `json:"toolVersion"`
	Status            string              `json:"status"`
	CheckedAt         string              `json:"checkedAt"`
	RequireScreenshot bool                `json:"requireScreenshot"`
	DeckID            string              `json:"deckId"`
	DeckDir           string              `json:"deckDir"`
	EvidencePath      string              `json:"evidencePath"`
	VerificationPath  string              `json:"verificationPath"`
	ManifestPath      string              `json:"manifestPath"`
	Findings          []string            `json:"findings"`
	VerifiedFiles     map[string]artifact `json:"verifiedFiles"`
}

type workbenchSaveSmokeResult struct {
	SchemaVersion                   string              `json:"schemaVersion"`
	ToolName                        string              `json:"toolName"`
	ToolVersion                     string              `json:"toolVersion"`
	Status                          string              `json:"status"`
	GeneratedAt                     string              `json:"generatedAt"`
	Workspace                       string              `json:"workspace"`
	DeckID                          string              `json:"deckId"`
	DeckDir                         string              `json:"deckDir"`
	WorkbenchURL                    string              `json:"workbenchUrl"`
	SessionID                       string              `json:"sessionId"`
	ServerBind                      string              `json:"serverBind"`
	StartStatus                     string              `json:"startStatus"`
	DraftStatus                     string              `json:"draftStatus"`
	SaveStatus                      string              `json:"saveStatus"`
	StopStatus                      string              `json:"stopStatus"`
	StartedNew                      bool                `json:"startedNew"`
	ReusedExisting                  bool                `json:"reusedExisting"`
	TokenRedacted                   bool                `json:"tokenRedacted"`
	HTMLBootstrapTokenFound         bool                `json:"htmlBootstrapTokenFound"`
	RawTokenAbsentFromArtifacts     bool                `json:"rawTokenAbsentFromArtifacts"`
	BriefPath                       string              `json:"briefPath"`
	DraftPath                       string              `json:"draftPath"`
	ManifestPath                    string              `json:"manifestPath"`
	LogPath                         string              `json:"logPath"`
	EvidencePath                    string              `json:"evidencePath"`
	BrowserOpenStrategy             string              `json:"browserOpenStrategy"`
	IsActualCodexAppBrowserEvidence bool                `json:"isActualCodexAppBrowserEvidence"`
	BrowserRendered                 bool                `json:"browserRendered,omitempty"`
	ChromeVersion                   string              `json:"chromeVersion,omitempty"`
	ChromeSandbox                   string              `json:"chromeSandbox,omitempty"`
	ChromeNoSandboxReason           string              `json:"chromeNoSandboxReason,omitempty"`
	WorkbenchScreenshot             *artifact           `json:"workbenchScreenshot,omitempty"`
	Input                           workbenchSaveInput  `json:"input"`
	VerifiedFiles                   map[string]artifact `json:"verifiedFiles"`
	Checks                          map[string]any      `json:"checks"`
	Findings                        []string            `json:"findings,omitempty"`
}

type workbenchSaveSmokeOptions struct {
	CaptureScreenshot bool
	ChromePath        string
	ChromeNoSandbox   bool
}

type workbenchBrowserEvidenceInput struct {
	Inspector          string
	Surface            string
	Invocation         string
	ThreadID           string
	URL                string
	WorkbenchVisible   bool
	SavedInputVerified bool
	Notes              string
	ScreenshotPath     string
}

type workbenchSaveInput struct {
	InitialRequest     string `json:"initialRequest"`
	Title              string `json:"title"`
	Audience           string `json:"audience"`
	DecisionGoal       string `json:"decisionGoal"`
	SourceNotes        string `json:"sourceNotes"`
	KeyMessages        string `json:"keyMessages"`
	RequiredClaims     string `json:"requiredClaims"`
	Constraints        string `json:"constraints"`
	OutputExpectations string `json:"outputExpectations"`
}

type workbenchAutoUpdateResult struct {
	Status                   string             `json:"status"`
	UpdatesEnabled           bool               `json:"updatesEnabled"`
	Channel                  string             `json:"channel,omitempty"`
	InstallMode              string             `json:"installMode,omitempty"`
	InstallRoot              string             `json:"installRoot,omitempty"`
	CurrentVersion           string             `json:"currentVersion,omitempty"`
	TargetVersion            string             `json:"targetVersion,omitempty"`
	TargetTag                string             `json:"targetTag,omitempty"`
	RestartRequired          bool               `json:"restartRequired"`
	PendingActivation        bool               `json:"pendingActivation"`
	PendingActivationCommand string             `json:"pendingActivationCommand,omitempty"`
	PluginVerificationStatus string             `json:"pluginVerificationStatus,omitempty"`
	NextVerificationCommand  string             `json:"nextVerificationCommand,omitempty"`
	BlocksWorkbench          bool               `json:"blocksWorkbench"`
	ContinueToWorkbench      bool               `json:"continueToWorkbench"`
	Instruction              string             `json:"instruction,omitempty"`
	Error                    string             `json:"error,omitempty"`
	ApplyResult              *updateApplyResult `json:"applyResult,omitempty"`
}

func runWorkbench(args []string) error {
	if len(args) == 0 {
		return exitCodeError(2, "usage: slidex workbench start|serve|status|stop|save-smoke|evidence|verify-evidence")
	}
	switch args[0] {
	case "start":
		return runWorkbenchStart(args[1:])
	case "serve":
		return runWorkbenchServe(args[1:])
	case "status":
		return runWorkbenchStatus(args[1:])
	case "stop":
		return runWorkbenchStop(args[1:])
	case "save-smoke":
		return runWorkbenchSaveSmoke(args[1:])
	case "evidence":
		return runWorkbenchEvidence(args[1:])
	case "verify-evidence":
		return runWorkbenchVerifyEvidence(args[1:])
	default:
		return exitCodeError(2, "unknown workbench command: %s", args[0])
	}
}

func runWorkbenchStart(args []string) error {
	fs := flag.NewFlagSet("workbench start", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace root containing decks/")
	deckID := fs.String("deck-id", "", "deck id to create or open")
	deck := fs.String("deck", "", "existing deck workspace directory")
	fromTemplate := fs.String("from-template", defaultDeckTemplatePath, "template deck directory")
	initialRequest := fs.String("initial-request", "", "original user request to seed into the Workbench draft")
	title := fs.String("title", "", "deck title to seed into the Workbench draft")
	audience := fs.String("audience", "", "audience to seed into the Workbench draft")
	decisionGoal := fs.String("decision-goal", "", "decision goal to seed into the Workbench draft")
	sourceNotes := fs.String("source-notes", "", "source-material notes to seed into the Workbench draft")
	keyMessages := fs.String("key-messages", "", "key messages to seed into the Workbench draft")
	requiredClaims := fs.String("required-claims", "", "claims that need evidence review to seed into the Workbench draft")
	constraints := fs.String("constraints", "", "constraints and exclusions to seed into the Workbench draft")
	outputExpectations := fs.String("output-expectations", "", "output expectations to seed into the Workbench draft")
	defaultBrowserMode := workbenchBrowserOpenModeByEnv()
	browserOpen := fs.Bool("browser-open", defaultBrowserMode == workbenchBrowserOpenStructured, "emit legacy browserOpen navigation intent; set false to suppress automatic browser opening")
	browserOpenMode := fs.String("browser-open-mode", string(defaultBrowserMode), "browser opening mode: structured, agent, or manual")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mode := browserOpenModeFromFlags(fs, *browserOpen, *browserOpenMode, defaultBrowserMode)
	autoUpdate := runWorkbenchAutoUpdatePreflight(context.Background())
	if autoUpdate.BlocksWorkbench {
		return printJSON(map[string]any{
			"toolName":         toolName,
			"status":           autoUpdate.Status,
			"autoUpdate":       autoUpdate,
			"openInstruction":  autoUpdate.Instruction,
			"workbenchStarted": false,
		})
	}
	result, manifest, startedNew, err := startWorkbenchWithInput(*workspace, *deckID, *deck, *fromTemplate, workbenchSaveInput{
		InitialRequest:     *initialRequest,
		Title:              *title,
		Audience:           *audience,
		DecisionGoal:       *decisionGoal,
		SourceNotes:        *sourceNotes,
		KeyMessages:        *keyMessages,
		RequiredClaims:     *requiredClaims,
		Constraints:        *constraints,
		OutputExpectations: *outputExpectations,
	})
	if err != nil {
		return err
	}
	response := map[string]any{
		"toolName":                toolName,
		"status":                  manifest.Status,
		"deck":                    result,
		"workbench":               publicWorkbenchStatusWithBrowserOpenMode(manifest, mode),
		"openInstruction":         workbenchOpenInstruction(manifest, mode),
		"browserOpenStrategy":     manifest.BrowserOpenStrategy,
		"autoUpdate":              autoUpdate,
		"proprietaryCanvasAPI":    "not_used",
		"tokenHandling":           "write token is redacted from CLI output and manifest",
		"startedNew":              startedNew,
		"reusedExisting":          !startedNew,
		"workbenchManifestPath":   filepath.ToSlash(filepath.Join(manifest.OutDir, workbenchManifestName)),
		"supportedURLMechanism":   "Codex in-app browser can open local URLs by Browser plugin navigation, URL click, or manual navigation.",
		"unsupportedURLMechanism": "No Codex 0.138.0 App Server client request method was found for plugin-owned automatic browser opening.",
	}
	addWorkbenchBrowserOpenFields(response, manifest, mode)
	return printJSON(response)
}

func addWorkbenchBrowserOpenFields(response map[string]any, manifest workbenchManifest, mode workbenchBrowserOpenMode) {
	response["browserOpenMode"] = string(mode)
	switch mode {
	case workbenchBrowserOpenStructured:
		response["browserOpen"] = workbenchBrowserOpenIntent(manifest)
	case workbenchBrowserOpenAgent:
		response["browserOpenSuppressed"] = true
		response["workbenchURL"] = manifest.URL
		response["agentBrowserInstruction"] = workbenchAgentBrowserInstruction(manifest)
	default:
		response["browserOpenSuppressed"] = true
		response["workbenchURL"] = manifest.URL
	}
}

func runWorkbenchServe(args []string) error {
	fs := flag.NewFlagSet("workbench serve", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	workspace := fs.String("workspace", ".", "workspace root")
	sessionID := fs.String("session", "", "session id")
	token := fs.String("token", "", "write token")
	tokenEnv := fs.String("token-env", "", "environment variable containing the write token")
	shutdownToken := fs.String("shutdown-token", "", "shutdown token")
	shutdownTokenEnv := fs.String("shutdown-token-env", "", "environment variable containing the shutdown token")
	readinessToken := fs.String("readiness-token", "", "readiness token")
	readinessTokenEnv := fs.String("readiness-token-env", "", "environment variable containing the readiness token")
	port := fs.Int("port", 0, "loopback port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token != "" {
		return exitCodeError(2, "--token is not supported; use --token-env to keep the workbench token out of process arguments")
	}
	if *shutdownToken != "" {
		return exitCodeError(2, "--shutdown-token is not supported; use --shutdown-token-env to keep the shutdown token out of process arguments")
	}
	if *readinessToken != "" {
		return exitCodeError(2, "--readiness-token is not supported; use --readiness-token-env to keep the readiness token out of process arguments")
	}
	if *token == "" && *tokenEnv != "" {
		*token = os.Getenv(*tokenEnv)
		_ = os.Unsetenv(*tokenEnv)
	}
	if *shutdownToken == "" && *shutdownTokenEnv != "" {
		*shutdownToken = os.Getenv(*shutdownTokenEnv)
		_ = os.Unsetenv(*shutdownTokenEnv)
	}
	if *readinessToken == "" && *readinessTokenEnv != "" {
		*readinessToken = os.Getenv(*readinessTokenEnv)
		_ = os.Unsetenv(*readinessTokenEnv)
	}
	if *deck == "" || *sessionID == "" || *token == "" || *shutdownToken == "" || *readinessToken == "" || *port <= 0 {
		return exitCodeError(2, "usage: slidex workbench serve --deck DIR --session ID --token-env ENV --shutdown-token-env ENV --readiness-token-env ENV --port PORT")
	}
	deckAbs, err := resolveDeckDir(*workspace, "", *deck, false, defaultDeckTemplatePath)
	if err != nil {
		return err
	}
	return serveWorkbench(deckAbs, *workspace, *sessionID, *token, *shutdownToken, *readinessToken, *port)
}

func runWorkbenchStatus(args []string) error {
	fs := flag.NewFlagSet("workbench status", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace root containing decks/")
	deckID := fs.String("deck-id", "", "deck id")
	deck := fs.String("deck", "", "deck workspace directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	manifest, err := workbenchStatus(*workspace, *deckID, *deck)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "workbench": publicWorkbenchStatus(manifest), "status": manifest.Status})
}

func runWorkbenchStop(args []string) error {
	fs := flag.NewFlagSet("workbench stop", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace root containing decks/")
	deckID := fs.String("deck-id", "", "deck id")
	deck := fs.String("deck", "", "deck workspace directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	manifest, err := stopWorkbench(*workspace, *deckID, *deck)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "workbench": publicWorkbenchStatus(manifest), "status": manifest.Status})
}

func runWorkbenchSaveSmoke(args []string) error {
	fs := flag.NewFlagSet("workbench save-smoke", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace root containing decks/")
	deckID := fs.String("deck-id", "workbench-save-smoke", "deck id to create or open")
	deck := fs.String("deck", "", "existing deck workspace directory")
	fromTemplate := fs.String("from-template", defaultDeckTemplatePath, "template deck directory")
	title := fs.String("title", "Workbench save smoke", "deck title to submit")
	audience := fs.String("audience", "Codex App verification reviewer", "audience to submit")
	decisionGoal := fs.String("decision-goal", "Verify the slidex workbench can persist initial deck creation input.", "decision goal to submit")
	sourceNotes := fs.String("source-notes", "Generated by slidex workbench save-smoke. This is not Codex App GUI/browser evidence.", "source-material notes to submit")
	outputExpectations := fs.String("output-expectations", "Deck-local brief.md, out/workbench_draft.json, and out/workbench_manifest.json are current after HTTP save.", "output expectations to submit")
	screenshot := fs.Bool("screenshot", false, "capture a headless Chrome screenshot of the workbench URL as pre-GUI render evidence")
	chrome := fs.String("chrome", "", "Chrome/Chromium binary for --screenshot")
	chromeNoSandbox := fs.Bool("chrome-no-sandbox", false, "run Chrome with --no-sandbox for --screenshot and record the risk")
	jsonOut := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = jsonOut
	result, err := smokeSaveWorkbench(*workspace, *deckID, *deck, *fromTemplate, workbenchSaveInput{
		Title:              *title,
		Audience:           *audience,
		DecisionGoal:       *decisionGoal,
		SourceNotes:        *sourceNotes,
		OutputExpectations: *outputExpectations,
	}, workbenchSaveSmokeOptions{CaptureScreenshot: *screenshot, ChromePath: *chrome, ChromeNoSandbox: *chromeNoSandbox})
	if err != nil {
		return err
	}
	payload := map[string]any{"toolName": toolName, "status": result.Status, "smoke": result, "evidencePath": result.EvidencePath}
	if result.Status != "pass" {
		_ = printJSON(payload)
		return exitCodeError(4, "workbench save smoke did not pass")
	}
	return printJSON(payload)
}

func runWorkbenchEvidence(args []string) error {
	fs := flag.NewFlagSet("workbench evidence", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace root containing decks/")
	deckID := fs.String("deck-id", "", "deck id")
	deck := fs.String("deck", "", "existing deck workspace directory")
	inspector := fs.String("inspector", "", "person or role that inspected the Codex App browser surface")
	surface := fs.String("surface", "codex_app_in_app_browser", "browser surface: codex_app_in_app_browser or codex_browser_plugin")
	invocation := fs.String("invocation", "", "plugin invocation used, for example @slidex create a deck")
	threadID := fs.String("thread-id", "", "Codex App thread id if visible")
	observedURL := fs.String("url", "", "workbench URL observed in the Codex App browser")
	workbenchVisible := fs.Bool("workbench-visible", false, "confirm the workbench UI was visible in the browser surface")
	savedInputVerified := fs.Bool("saved-input-verified", false, "confirm saved deck creation input was verified")
	notes := fs.String("notes", "", "short inspection notes")
	screenshot := fs.String("screenshot", "", "optional screenshot image captured from the Codex App browser surface")
	if err := fs.Parse(args); err != nil {
		return err
	}
	evidence, err := recordWorkbenchBrowserEvidence(*workspace, *deckID, *deck, workbenchBrowserEvidenceInput{
		Inspector:          *inspector,
		Surface:            *surface,
		Invocation:         *invocation,
		ThreadID:           *threadID,
		URL:                *observedURL,
		WorkbenchVisible:   *workbenchVisible,
		SavedInputVerified: *savedInputVerified,
		Notes:              *notes,
		ScreenshotPath:     *screenshot,
	})
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "status": evidence.Status, "evidence": evidence, "evidencePath": evidence.EvidencePath})
}

func runWorkbenchVerifyEvidence(args []string) error {
	fs := flag.NewFlagSet("workbench verify-evidence", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace root containing decks/")
	deckID := fs.String("deck-id", "", "deck id")
	deck := fs.String("deck", "", "existing deck workspace directory")
	requireScreenshot := fs.Bool("require-screenshot", false, "fail unless browser evidence includes a copied browser screenshot artifact")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result, err := verifyWorkbenchBrowserEvidence(*workspace, *deckID, *deck, *requireScreenshot)
	if err != nil {
		return err
	}
	payload := map[string]any{"toolName": toolName, "status": result.Status, "verification": result}
	if result.Status != "pass" {
		_ = printJSON(payload)
		return exitCodeError(4, "workbench browser evidence verification failed")
	}
	return printJSON(payload)
}

func callMCPDeckBootstrap(args map[string]any) (any, error) {
	deckID, _ := args["deckId"].(string)
	if deckID == "" {
		deckID, _ = args["deck_id"].(string)
	}
	if deckID != "" {
		args["deckId"] = deckID
	}
	return callMCPWorkbenchStart(args)
}

func callMCPDeckInspect(args map[string]any) (any, error) {
	workspace, _ := args["workspace"].(string)
	deckID, _ := args["deckId"].(string)
	deck, _ := args["deck"].(string)
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, defaultDeckTemplatePath)
	if err != nil {
		return nil, err
	}
	return inspectDeck(deckAbs)
}

func callMCPWorkbenchStart(args map[string]any) (any, error) {
	workspace, _ := args["workspace"].(string)
	deckID, _ := args["deckId"].(string)
	deck, _ := args["deck"].(string)
	mode := workbenchBrowserOpenModeArg(args, workbenchBrowserOpenModeByEnv())
	autoUpdate := runWorkbenchAutoUpdatePreflight(context.Background())
	if autoUpdate.BlocksWorkbench {
		return map[string]any{
			"toolName":         toolName,
			"status":           autoUpdate.Status,
			"autoUpdate":       autoUpdate,
			"openInstruction":  autoUpdate.Instruction,
			"workbenchStarted": false,
		}, nil
	}
	result, manifest, startedNew, err := startWorkbenchWithInput(workspace, deckID, deck, defaultDeckTemplatePath, workbenchSaveInput{
		InitialRequest:     stringArg(args, "initialRequest", "initial_request"),
		Title:              stringArg(args, "title"),
		Audience:           stringArg(args, "audience"),
		DecisionGoal:       stringArg(args, "decisionGoal", "decision_goal"),
		SourceNotes:        stringArg(args, "sourceNotes", "source_notes"),
		KeyMessages:        stringArg(args, "keyMessages", "key_messages"),
		RequiredClaims:     stringArg(args, "requiredClaims", "required_claims"),
		Constraints:        stringArg(args, "constraints"),
		OutputExpectations: stringArg(args, "outputExpectations", "output_expectations"),
	})
	if err != nil {
		return nil, err
	}
	response := map[string]any{
		"deck":                 result,
		"workbench":            publicWorkbenchStatusWithBrowserOpenMode(manifest, mode),
		"startedNew":           startedNew,
		"reusedExisting":       !startedNew,
		"openInstruction":      workbenchOpenInstruction(manifest, mode),
		"autoUpdate":           autoUpdate,
		"proprietaryCanvasAPI": "not_used",
	}
	addWorkbenchBrowserOpenFields(response, manifest, mode)
	return response, nil
}

func runWorkbenchAutoUpdatePreflight(ctx context.Context) workbenchAutoUpdateResult {
	result := workbenchAutoUpdateResult{
		Status:              "skipped",
		ContinueToWorkbench: true,
	}
	if !automaticUpdatesAllowed() {
		result.Status = "disabled_by_environment"
		result.Instruction = "Automatic slidex release updates are disabled by " + updateAutoEnv + ". Continue to the local Workbench with the currently installed version."
		return result
	}
	installRoot, unlock, err := lockResolvedUpdateInstallRoot("")
	if err != nil {
		result.Status = "lock_error"
		result.Error = err.Error()
		result.Instruction = "slidex could not acquire the update lock. Continue to the local Workbench with the currently installed version."
		return result
	}
	defer unlock()
	status, err := currentUpdateStatus(installRoot, "")
	if err != nil {
		result.Status = "status_error"
		result.Error = err.Error()
		result.Instruction = "slidex could not read update status. Continue to the local Workbench with the currently installed version."
		return result
	}
	result = workbenchAutoUpdateFromStatus(status, "disabled")
	if !status.UpdatesEnabled {
		result.Status = "disabled"
		result.ContinueToWorkbench = true
		result.Instruction = firstNonEmpty(status.Guidance, "Automatic release updates are disabled for this install. Continue to the local Workbench with the currently installed version.")
		return result
	}
	if status.PendingActivation {
		result.Status = "pending_activation"
		result.BlocksWorkbench = true
		result.ContinueToWorkbench = false
		result.Instruction = workbenchAutoUpdateInstruction(result)
		return result
	}
	if status.RestartRequired {
		result.Status = "restart_required"
		result.BlocksWorkbench = true
		result.ContinueToWorkbench = false
		result.Instruction = workbenchAutoUpdateInstruction(result)
		return result
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	releases, err := fetchUpdateReleases(ctx, defaultUpdateAPIURL())
	if err != nil {
		result.Status = "check_failed"
		result.Error = err.Error()
		result.Instruction = "Automatic update check failed. Continue to the local Workbench with the currently installed version."
		return result
	}
	release, err := selectUpdateReleaseForStatus(status, releases)
	if err != nil {
		result.Status = "check_failed"
		result.Error = err.Error()
		result.Instruction = "Automatic update check failed. Continue to the local Workbench with the currently installed version."
		return result
	}
	result.TargetVersion = release.Version
	result.TargetTag = release.TagName
	if release.Version == status.CurrentVersion {
		result.Status = "current"
		result.Instruction = "slidex is current for the " + status.Channel + " channel. Continue to the local Workbench."
		return result
	}

	candidateRoot, targetVersion, targetTag, err := downloadAndStageSelectedRelease(ctx, status, release)
	if err != nil {
		result.Status = "apply_failed"
		result.Error = err.Error()
		result.Instruction = "Automatic update download or staging failed. Continue to the local Workbench with the currently installed version."
		return result
	}
	result.TargetVersion = targetVersion
	result.TargetTag = targetTag
	applyResult, err := applyCandidateBundle(status, candidateRoot, targetVersion, targetTag)
	result.ApplyResult = &applyResult
	if err != nil {
		result.Status = "apply_failed"
		result.Error = err.Error()
		result.Instruction = "Automatic update apply failed. Continue to the local Workbench with the currently installed version."
		return result
	}
	if hasFailures(applyResult.CandidateValidation) {
		result.Status = "apply_failed"
		result.Error = "candidate bundle validation failed"
		result.Instruction = "Automatic update validation failed. Continue to the local Workbench with the currently installed version."
		return result
	}

	updatedStatus, statusErr := currentUpdateStatus(installRoot, "")
	if statusErr == nil {
		result = workbenchAutoUpdateFromStatus(updatedStatus, result.Status)
		result.ApplyResult = &applyResult
	} else {
		result.RestartRequired = applyResult.RestartRequired
		result.PluginVerificationStatus = applyResult.PluginVerificationStatus
		result.NextVerificationCommand = applyResult.NextVerificationCommand
	}
	switch applyResult.Status {
	case "applied":
		result.Status = "applied_restart_required"
		result.BlocksWorkbench = true
		result.ContinueToWorkbench = false
	case "pending-restart":
		result.Status = "pending_activation"
		result.BlocksWorkbench = true
		result.ContinueToWorkbench = false
		result.PendingActivation = true
	default:
		result.Status = "apply_failed"
		result.Error = "unexpected update apply status: " + applyResult.Status
		result.BlocksWorkbench = false
		result.ContinueToWorkbench = true
	}
	result.Instruction = workbenchAutoUpdateInstruction(result)
	return result
}

func workbenchAutoUpdateFromStatus(status updateStatus, resultStatus string) workbenchAutoUpdateResult {
	return workbenchAutoUpdateResult{
		Status:                   resultStatus,
		UpdatesEnabled:           status.UpdatesEnabled,
		Channel:                  status.Channel,
		InstallMode:              status.InstallMode,
		InstallRoot:              status.InstallRoot,
		CurrentVersion:           status.CurrentVersion,
		TargetVersion:            status.TargetVersion,
		TargetTag:                status.TargetTag,
		RestartRequired:          status.RestartRequired,
		PendingActivation:        status.PendingActivation,
		PendingActivationCommand: status.PendingActivationCommand,
		PluginVerificationStatus: status.PluginVerificationStatus,
		NextVerificationCommand:  status.NextVerificationCommand,
		ContinueToWorkbench:      true,
	}
}

func workbenchAutoUpdateInstruction(result workbenchAutoUpdateResult) string {
	if result.PendingActivation {
		return "A slidex update was staged automatically and must be activated before the local Workbench opens. Restart Codex so the old slidex MCP process exits, run the pending activation command if shown, then start a new Codex thread and invoke slidex again."
	}
	if result.RestartRequired || result.BlocksWorkbench {
		return "A slidex update was applied automatically. Restart Codex and start a new thread before creating this deck so the updated slidex plugin skills are active."
	}
	return result.Instruction
}

func stringArg(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func workbenchBrowserOpenModeByEnv() workbenchBrowserOpenMode {
	raw := strings.TrimSpace(os.Getenv(workbenchBrowserOpenEnv))
	if raw == "" {
		return workbenchBrowserOpenStructured
	}
	if mode, ok := parseWorkbenchBrowserOpenMode(raw); ok {
		return mode
	}
	return workbenchBrowserOpenStructured
}

func browserOpenModeFromFlags(fs *flag.FlagSet, browserOpen bool, rawMode string, fallback workbenchBrowserOpenMode) workbenchBrowserOpenMode {
	explicitBrowserOpen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "browser-open" {
			explicitBrowserOpen = true
		}
	})
	if explicitBrowserOpen {
		if browserOpen {
			return workbenchBrowserOpenStructured
		}
		return workbenchBrowserOpenManual
	}
	if mode, ok := parseWorkbenchBrowserOpenMode(rawMode); ok {
		return mode
	}
	return fallback
}

func workbenchBrowserOpenModeArg(args map[string]any, fallback workbenchBrowserOpenMode) workbenchBrowserOpenMode {
	if value, ok := args["browserOpenMode"]; ok {
		if mode, ok := parseWorkbenchBrowserOpenMode(fmt.Sprint(value)); ok {
			return mode
		}
	}
	if value, ok := args["browser_open_mode"]; ok {
		if mode, ok := parseWorkbenchBrowserOpenMode(fmt.Sprint(value)); ok {
			return mode
		}
	}
	for _, key := range []string{"browserOpen", "browser_open"} {
		value, ok := args[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			if typed {
				return workbenchBrowserOpenStructured
			}
			return workbenchBrowserOpenManual
		case string:
			if mode, ok := parseWorkbenchBrowserOpenMode(typed); ok {
				return mode
			}
		}
	}
	return fallback
}

func parseWorkbenchBrowserOpenMode(raw string) (workbenchBrowserOpenMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "t", "yes", "y", "on", "structured", "intent", "browseropen", "browser_open":
		return workbenchBrowserOpenStructured, true
	case "agent", "@browser", "browser", "browser_plugin", "browser-plugin", "browser_plugin_instruction":
		return workbenchBrowserOpenAgent, true
	case "0", "false", "f", "no", "n", "off", "manual", "none", "suppress", "suppressed":
		return workbenchBrowserOpenManual, true
	default:
		return workbenchBrowserOpenStructured, false
	}
}

func callMCPWorkbenchStatus(args map[string]any) (any, error) {
	workspace, _ := args["workspace"].(string)
	deckID, _ := args["deckId"].(string)
	deck, _ := args["deck"].(string)
	manifest, err := workbenchStatus(workspace, deckID, deck)
	if err != nil {
		return nil, err
	}
	return publicWorkbenchStatus(manifest), nil
}

func callMCPWorkbenchStop(args map[string]any) (any, error) {
	workspace, _ := args["workspace"].(string)
	deckID, _ := args["deckId"].(string)
	deck, _ := args["deck"].(string)
	manifest, err := stopWorkbench(workspace, deckID, deck)
	if err != nil {
		return nil, err
	}
	return publicWorkbenchStatus(manifest), nil
}

func smokeSaveWorkbench(workspace, deckID, deck, fromTemplate string, input workbenchSaveInput, opts workbenchSaveSmokeOptions) (result workbenchSaveSmokeResult, err error) {
	input = normalizeWorkbenchInput(input)
	if input.Title == "" || input.Audience == "" || input.DecisionGoal == "" {
		return result, errors.New("save smoke requires title, audience, and decision goal")
	}
	bootstrap, manifest, startedNew, err := startWorkbench(workspace, deckID, deck, fromTemplate)
	if err != nil {
		return result, err
	}
	deckAbs := filepath.FromSlash(bootstrap.DeckDir)
	if !filepath.IsAbs(deckAbs) {
		deckAbs = filepath.Join(workspaceRoot(workspace), deckAbs)
	}
	workspaceAbs := workspaceRoot(workspace)
	if manifest.Workspace != "" {
		workspaceAbs = filepath.FromSlash(manifest.Workspace)
	}
	briefPath := filepath.Join(deckAbs, "brief.md")
	draftPath := filepath.Join(deckAbs, "out", workbenchDraftName)
	manifestPath := filepath.Join(deckAbs, "out", workbenchManifestName)
	logPath := filepath.Join(deckAbs, "out", "workbench_server.log")
	evidencePath := filepath.Join(deckAbs, "out", workbenchSaveSmokeName)
	screenshotPath := filepath.Join(deckAbs, "out", workbenchSaveSmokeScreenshot)
	result = workbenchSaveSmokeResult{
		SchemaVersion:                   "slidex.workbenchSaveSmoke.v1",
		ToolName:                        toolName,
		ToolVersion:                     toolVersion,
		Status:                          "fail",
		GeneratedAt:                     time.Now().UTC().Format(time.RFC3339),
		Workspace:                       filepath.ToSlash(workspaceAbs),
		DeckID:                          manifest.DeckID,
		DeckDir:                         filepath.ToSlash(deckAbs),
		WorkbenchURL:                    manifest.URL,
		SessionID:                       manifest.SessionID,
		ServerBind:                      manifest.ServerBind,
		StartStatus:                     manifest.Status,
		StartedNew:                      startedNew,
		ReusedExisting:                  !startedNew,
		TokenRedacted:                   manifest.TokenRedacted,
		BriefPath:                       filepath.ToSlash(briefPath),
		DraftPath:                       filepath.ToSlash(draftPath),
		ManifestPath:                    filepath.ToSlash(manifestPath),
		LogPath:                         filepath.ToSlash(logPath),
		EvidencePath:                    filepath.ToSlash(evidencePath),
		BrowserOpenStrategy:             manifest.BrowserOpenStrategy,
		IsActualCodexAppBrowserEvidence: false,
		Input:                           input,
		VerifiedFiles:                   map[string]artifact{},
		Checks:                          map[string]any{"actualCodexAppBrowserEvidence": false},
	}
	defer func() {
		if !result.StartedNew {
			result.StopStatus = "reused_not_stopped"
			result.Checks["workbenchStop"] = map[string]any{
				"status": "reused_not_stopped",
				"reason": "save-smoke reused an existing workbench and did not stop it",
			}
		} else {
			stopped, stopErr := stopWorkbench(workspaceAbs, result.DeckID, "")
			if stopErr != nil {
				result.Findings = append(result.Findings, "workbench stop failed: "+stopErr.Error())
				if err == nil {
					err = stopErr
				}
			} else {
				result.StopStatus = stopped.Status
				result.Checks["workbenchStop"] = publicWorkbenchStatus(stopped)
			}
		}
		if result.Status == "" || result.Status == "fail" {
			result.Status = workbenchSaveSmokeStatus(result)
		}
		if writeErr := secureWriteJSON(evidencePath, result); writeErr != nil && err == nil {
			err = writeErr
		}
	}()

	htmlRaw, err := fetchWorkbenchHTML(manifest.URL)
	if err != nil {
		result.Findings = append(result.Findings, "workbench HTML fetch failed: "+err.Error())
		return result, err
	}
	boot, token, err := extractWorkbenchBoot(string(htmlRaw))
	result.Checks["htmlBytes"] = len(htmlRaw)
	result.Checks["htmlSha256"] = sha256Bytes(htmlRaw)
	if err != nil {
		result.Findings = append(result.Findings, "workbench HTML boot extraction failed: "+err.Error())
		return result, err
	}
	result.HTMLBootstrapTokenFound = token != ""
	result.Checks["htmlBoot"] = map[string]any{
		"deckId":    boot["deckId"],
		"sessionId": boot["sessionId"],
		"apiBase":   boot["apiBase"],
		"tokenHash": sha256Bytes([]byte(token)),
	}
	if !result.HTMLBootstrapTokenFound {
		return result, errors.New("workbench HTML did not include bootstrap token")
	}
	apiBase, _ := boot["apiBase"].(string)
	if strings.TrimSpace(apiBase) == "" {
		return result, errors.New("workbench HTML did not include apiBase")
	}
	draftResp, err := postWorkbenchJSON(manifest.URL, apiBase+"/draft", token, input)
	if err != nil {
		result.Findings = append(result.Findings, "draft POST failed: "+err.Error())
		return result, err
	}
	result.DraftStatus, _ = draftResp["status"].(string)
	result.Checks["draftResponse"] = summarizeWorkbenchSmokeResponse(draftResp)
	saveResp, err := postWorkbenchJSON(manifest.URL, apiBase+"/save", token, input)
	if err != nil {
		result.Findings = append(result.Findings, "save POST failed: "+err.Error())
		return result, err
	}
	result.SaveStatus, _ = saveResp["status"].(string)
	result.Checks["saveResponse"] = summarizeWorkbenchSmokeResponse(saveResp)

	updated, ok := readWorkbenchManifest(deckAbs)
	if !ok {
		return result, fmt.Errorf("workbench manifest missing after save: %s", filepath.ToSlash(manifestPath))
	}
	result.TokenRedacted = updated.TokenRedacted
	result.ServerBind = updated.ServerBind
	result.WorkbenchURL = updated.URL
	result.SessionID = updated.SessionID
	result.Checks["manifestAfterSave"] = publicWorkbenchStatus(updated)
	result.VerifiedFiles = map[string]artifact{
		"brief":    artifactFromPath(briefPath),
		"draft":    artifactFromPath(draftPath),
		"manifest": artifactFromPath(manifestPath),
	}
	if opts.CaptureScreenshot {
		if err := captureWorkbenchSaveSmokeScreenshot(&result, screenshotPath, opts); err != nil {
			result.Findings = append(result.Findings, "workbench screenshot capture failed: "+err.Error())
			return result, err
		}
	}
	tokenCheckPaths := []string{briefPath, draftPath, manifestPath}
	if pathExists(workbenchControlPath(deckAbs)) {
		tokenCheckPaths = append(tokenCheckPaths, workbenchControlPath(deckAbs))
	}
	if pathExists(logPath) {
		tokenCheckPaths = append(tokenCheckPaths, logPath)
	}
	result.RawTokenAbsentFromArtifacts = rawTokenAbsentFromFiles(token, tokenCheckPaths)
	result.Findings = workbenchSaveSmokeFindings(result, updated)
	result.Status = workbenchSaveSmokeStatus(result)
	return result, nil
}

func captureWorkbenchSaveSmokeScreenshot(result *workbenchSaveSmokeResult, screenshotPath string, opts workbenchSaveSmokeOptions) error {
	chromePath, err := resolveChrome(opts.ChromePath)
	if err != nil {
		return err
	}
	result.ChromeVersion = chromeVersion(chromePath)
	result.ChromeSandbox = "enabled"
	if opts.ChromeNoSandbox {
		result.ChromeSandbox = "disabled"
		result.ChromeNoSandboxReason = "explicit --chrome-no-sandbox flag"
	}
	if err := captureURLScreenshot(chromePath, result.WorkbenchURL, screenshotPath, 1440, 900, opts.ChromeNoSandbox); err != nil {
		return err
	}
	dom, err := captureURLDOM(chromePath, result.WorkbenchURL, opts.ChromeNoSandbox)
	if err != nil {
		return err
	}
	domText := string(dom)
	domProbe := map[string]any{
		"bytes":                    len(dom),
		"solidWorkbenchVisible":    strings.Contains(domText, "slidex Solid Workbench"),
		"solidClientControls":      strings.Contains(domText, "Back") && strings.Contains(domText, "Next"),
		"saveBriefVisible":         strings.Contains(domText, "Save brief"),
		"clientExceptionDisplayed": strings.Contains(domText, "Error | Uncaught Client Exception"),
		"fallbackNoticeDisplayed":  strings.Contains(domText, "fallback form still exposes"),
	}
	result.Checks["workbenchDOMProbe"] = domProbe
	if domProbe["clientExceptionDisplayed"] == true {
		return errors.New("workbench DOM rendered a SolidStart client exception")
	}
	if domProbe["fallbackNoticeDisplayed"] == true {
		return errors.New("workbench DOM still shows the no-JavaScript fallback form after client load")
	}
	if domProbe["solidWorkbenchVisible"] != true || domProbe["solidClientControls"] != true || domProbe["saveBriefVisible"] != true {
		return errors.New("workbench DOM did not render the expected Workbench UI")
	}
	dim, blank, err := validatePNG(screenshotPath, 1440, 900)
	if err != nil {
		return err
	}
	screenshotArtifact := artifactFromPath(screenshotPath)
	result.WorkbenchScreenshot = &screenshotArtifact
	result.BrowserRendered = !blank
	result.Checks["workbenchScreenshot"] = map[string]any{
		"path":       filepath.ToSlash(screenshotPath),
		"sha256":     screenshotArtifact.SHA256,
		"size":       screenshotArtifact.Size,
		"dimensions": dim,
		"blank":      blank,
	}
	return nil
}

func captureURLDOM(chromePath, targetURL string, chromeNoSandbox bool) ([]byte, error) {
	args, cleanup, err := chromeHeadlessBaseArgs(chromeNoSandbox)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	args = append(args,
		"--virtual-time-budget=5000",
		"--dump-dom",
		targetURL,
	)
	out, err := runChromeCommand(chromeCommandTimeout, chromePath, args...)
	if err != nil {
		return nil, fmt.Errorf("chrome DOM dump failed: %w\n%s", err, string(out))
	}
	return out, nil
}

func fetchWorkbenchHTML(rawURL string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", rawURL, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256*1024))
}

func extractWorkbenchBoot(htmlText string) (map[string]any, string, error) {
	re := regexp.MustCompile(`(?s)const boot = (\{.*?\});`)
	matches := re.FindStringSubmatch(htmlText)
	if len(matches) != 2 {
		return nil, "", errors.New("bootstrap JSON not found")
	}
	var boot map[string]any
	if err := json.Unmarshal([]byte(matches[1]), &boot); err != nil {
		return nil, "", err
	}
	token, _ := boot["token"].(string)
	return boot, token, nil
}

func postWorkbenchJSON(baseURL, apiPath, token string, input workbenchSaveInput) (map[string]any, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, absoluteWorkbenchAPIURL(baseURL, apiPath), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", parsed.Scheme+"://"+parsed.Host)
	req.Header.Set("X-Slidex-Workbench-Token", token)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %s returned %s: %s", req.URL.String(), resp.Status, strings.TrimSpace(string(raw)))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func absoluteWorkbenchAPIURL(baseURL, apiPath string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return apiPath
	}
	parsed.Path = apiPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func summarizeWorkbenchSmokeResponse(resp map[string]any) map[string]any {
	out := map[string]any{}
	if status, _ := resp["status"].(string); status != "" {
		out["status"] = status
	}
	if manifest, _ := resp["manifest"].(map[string]any); manifest != nil {
		out["manifest"] = manifest
	}
	if draft, _ := resp["draft"].(map[string]any); draft != nil {
		out["draftStatus"], _ = draft["status"].(string)
		out["draftUpdatedAt"], _ = draft["updatedAt"].(string)
	}
	return out
}

func rawTokenAbsentFromFiles(token string, paths []string) bool {
	if token == "" {
		return false
	}
	for _, path := range paths {
		raw, err := readRegularFileWithMaxBytes(path, maxDeckTextArtifactBytes)
		if err != nil || bytes.Contains(raw, []byte(token)) {
			return false
		}
	}
	return true
}

func workbenchSaveSmokeFindings(result workbenchSaveSmokeResult, manifest workbenchManifest) []string {
	var findings []string
	if result.StartStatus != "running" {
		findings = append(findings, "workbench did not start in running state")
	}
	if result.DraftStatus != "draft_saved" {
		findings = append(findings, "draft endpoint did not report draft_saved")
	}
	if result.SaveStatus != "saved" {
		findings = append(findings, "save endpoint did not report saved")
	}
	if manifest.InputSavedAt == "" {
		findings = append(findings, "manifest does not record inputSavedAt")
	}
	if manifest.BriefPath == "" || manifest.DraftPath == "" {
		findings = append(findings, "manifest does not record saved brief/draft paths")
	}
	if result.ServerBind != "127.0.0.1" {
		findings = append(findings, "workbench did not bind to 127.0.0.1")
	}
	if !result.TokenRedacted {
		findings = append(findings, "manifest does not redact token")
	}
	if !result.HTMLBootstrapTokenFound {
		findings = append(findings, "HTML bootstrap token was not found")
	}
	if !result.RawTokenAbsentFromArtifacts {
		findings = append(findings, "raw token appeared in persisted artifacts/logs or artifact read failed")
	}
	if result.WorkbenchScreenshot != nil {
		if result.WorkbenchScreenshot.SHA256 == "" || result.WorkbenchScreenshot.Size <= 0 {
			findings = append(findings, "workbench screenshot is missing or empty")
		}
		if !result.BrowserRendered {
			findings = append(findings, "workbench screenshot appears blank")
		}
	}
	for name, artifact := range result.VerifiedFiles {
		if artifact.SHA256 == "" || artifact.Size <= 0 {
			findings = append(findings, "verified file is missing or empty: "+name)
		}
	}
	return findings
}

func workbenchSaveSmokeStatus(result workbenchSaveSmokeResult) string {
	stopOK := (result.StartedNew && result.StopStatus == "stopped") ||
		(result.ReusedExisting && result.StopStatus == "reused_not_stopped")
	screenshotOK := result.WorkbenchScreenshot == nil || result.BrowserRendered
	if result.StartStatus == "running" &&
		result.DraftStatus == "draft_saved" &&
		result.SaveStatus == "saved" &&
		stopOK &&
		result.ServerBind == "127.0.0.1" &&
		result.TokenRedacted &&
		result.HTMLBootstrapTokenFound &&
		result.RawTokenAbsentFromArtifacts &&
		!result.IsActualCodexAppBrowserEvidence &&
		screenshotOK &&
		len(result.Findings) == 0 &&
		len(result.VerifiedFiles) == 3 {
		return "pass"
	}
	return "fail"
}

func startWorkbench(workspace, deckID, deck, fromTemplate string) (deckBootstrapResult, workbenchManifest, bool, error) {
	return startWorkbenchWithInput(workspace, deckID, deck, fromTemplate, workbenchSaveInput{})
}

func startWorkbenchWithInput(workspace, deckID, deck, fromTemplate string, seed workbenchSaveInput) (deckBootstrapResult, workbenchManifest, bool, error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, true, fromTemplate)
	if err != nil {
		return deckBootstrapResult{}, workbenchManifest{}, false, err
	}
	outDir := filepath.Join(deckAbs, "out")
	if err := ensureSecureDir(outDir); err != nil {
		return deckBootstrapResult{}, workbenchManifest{}, false, err
	}
	unlock, err := acquireWorkbenchLock(outDir)
	if err != nil {
		return deckBootstrapResult{}, workbenchManifest{}, false, err
	}
	defer unlock()
	result := deckBootstrapResult{
		Workspace: filepath.ToSlash(workspaceRoot(workspace)),
		DeckID:    filepath.Base(deckAbs),
		DeckDir:   filepath.ToSlash(deckAbs),
		Status:    "ready",
	}
	if existing, ok := readWorkbenchManifest(deckAbs); ok {
		existing = canonicalWorkbenchManifestPaths(deckAbs, existing)
		if isTrustedWorkbenchReady(existing) {
			existing.Status = "running"
			if seeded, err := seedWorkbenchDraft(deckAbs, existing, seed); err != nil {
				return result, existing, false, err
			} else {
				return result, seeded, false, nil
			}
		}
		removeWorkbenchControl(deckAbs)
		existing.Status = "stale"
		existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeWorkbenchManifest(deckAbs, existing)
	}
	sessionID, err := randomURLToken(18)
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	token, err := randomURLToken(32)
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	shutdownToken, err := randomURLToken(32)
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	readinessToken, err := randomURLToken(32)
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	exe, err := os.Executable()
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	logPath := filepath.Join(outDir, "workbench_server.log")
	logFile, err := prepareWorkbenchServerOutput(logPath)
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	defer logFile.Close()
	var manifest workbenchManifest
	err = retryWorkbenchPortAttempts(5, chooseLoopbackPort, func(port int) (bool, error) {
		cmd := exec.Command(exe, "workbench", "serve", "--workspace", workspaceRoot(workspace), "--deck", deckAbs, "--session", sessionID, "--token-env", workbenchTokenEnv, "--shutdown-token-env", workbenchShutdownTokenEnv, "--readiness-token-env", workbenchReadinessTokenEnv, "--port", strconv.Itoa(port))
		cmd.Env = append(os.Environ(), workbenchTokenEnv+"="+token, workbenchShutdownTokenEnv+"="+shutdownToken, workbenchReadinessTokenEnv+"="+readinessToken)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		configureWorkbenchCommand(cmd)
		if err := cmd.Start(); err != nil {
			return true, err
		}
		manifest = newWorkbenchManifest(deckAbs, workspaceRoot(workspace), sessionID, token, port, cmd.Process.Pid, "starting")
		if err := writeWorkbenchManifest(deckAbs, manifest); err != nil {
			stopWorkbenchProcess(manifest)
			return false, err
		}
		if err := writeWorkbenchControl(deckAbs, newWorkbenchControl(manifest, shutdownToken, readinessToken)); err != nil {
			stopWorkbenchProcess(manifest)
			removeWorkbenchControl(deckAbs)
			return false, err
		}
		if err := waitForWorkbenchReady(manifest, 3*time.Second); err != nil {
			stopWorkbenchProcess(manifest)
			removeWorkbenchControl(deckAbs)
			manifest.Status = "stale"
			manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = writeWorkbenchManifest(deckAbs, manifest)
			return true, err
		}
		manifest.Status = "running"
		manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeWorkbenchManifest(deckAbs, manifest); err != nil {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		var exhausted workbenchPortRetryExhaustedError
		if errors.As(err, &exhausted) {
			return result, workbenchManifest{}, false, err
		}
		if manifest.PID > 0 {
			return result, manifest, true, err
		}
		return result, workbenchManifest{}, false, err
	}
	if seeded, err := seedWorkbenchDraft(deckAbs, manifest, seed); err != nil {
		return result, manifest, true, err
	} else {
		manifest = seeded
	}
	return result, manifest, true, nil
}

func seedWorkbenchDraft(deckAbs string, manifest workbenchManifest, input workbenchSaveInput) (workbenchManifest, error) {
	input = normalizeWorkbenchInput(input)
	if !hasAnyWorkbenchInput(input) {
		return manifest, nil
	}
	draft, err := writeWorkbenchDraft(deckAbs, input, "draft")
	if err != nil {
		return manifest, err
	}
	updated, err := updateWorkbenchManifest(deckAbs, manifest, func(current *workbenchManifest) {
		current.Status = "draft"
		current.DraftSavedAt = draft.UpdatedAt
		current.DraftPath = filepath.ToSlash(filepath.Join(deckAbs, "out", workbenchDraftName))
		current.UpdatedAt = draft.UpdatedAt
	})
	if err != nil {
		return manifest, err
	}
	return updated, nil
}

type workbenchPortRetryExhaustedError struct {
	err error
}

func (e workbenchPortRetryExhaustedError) Error() string {
	if e.err == nil {
		return "workbench did not become ready after port retries"
	}
	return fmt.Sprintf("workbench did not become ready after port retries: %v", e.err)
}

func (e workbenchPortRetryExhaustedError) Unwrap() error {
	return e.err
}

func retryWorkbenchPortAttempts(maxAttempts int, choosePort func() (int, error), run func(port int) (bool, error)) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		port, err := choosePort()
		if err != nil {
			return err
		}
		retry, err := run(port)
		if err == nil {
			return nil
		}
		if !retry {
			return err
		}
		lastErr = err
	}
	return workbenchPortRetryExhaustedError{err: lastErr}
}

func serveWorkbench(deckAbs, workspace, sessionID, token, shutdownToken, readinessToken string, port int) error {
	manifest := newWorkbenchManifest(deckAbs, workspaceRoot(workspace), sessionID, token, port, os.Getpid(), "running")
	if err := writeWorkbenchManifest(deckAbs, manifest); err != nil {
		return err
	}
	mux := http.NewServeMux()
	server := &workbenchHTTPServer{deckAbs: deckAbs, sessionID: sessionID, token: token, shutdownToken: shutdownToken, readinessToken: readinessToken, manifest: manifest}
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/readyz", server.handleReady)
	mux.HandleFunc("/workbench/"+sessionID, server.handleWorkbench)
	mux.HandleFunc("/workbench/"+sessionID+"/assets/", server.handleAsset)
	mux.HandleFunc("/workbench/"+sessionID+"/api/session", server.handleSession)
	mux.HandleFunc("/workbench/"+sessionID+"/api/draft", server.handleDraft)
	mux.HandleFunc("/workbench/"+sessionID+"/api/save", server.handleSave)
	mux.HandleFunc("/workbench/"+sessionID+"/api/complete", server.handleComplete)
	mux.HandleFunc("/workbench/"+sessionID+"/api/shutdown", server.handleShutdown)
	httpServer := &http.Server{
		Addr:              workbenchListenAddr(port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	server.shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}
	err := httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		manifest.Status = "stale"
		manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeWorkbenchManifest(deckAbs, manifest)
	}
	return err
}

type workbenchHTTPServer struct {
	deckAbs        string
	sessionID      string
	token          string
	shutdownToken  string
	readinessToken string
	shutdown       func()
	manifestMu     sync.RWMutex
	manifest       workbenchManifest
}

func (s *workbenchHTTPServer) currentManifest() workbenchManifest {
	s.manifestMu.RLock()
	defer s.manifestMu.RUnlock()
	return s.manifest
}

func (s *workbenchHTTPServer) setManifest(manifest workbenchManifest) {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()
	s.manifest = manifest
}

func (s *workbenchHTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	_ = writeJSONResponse(w, map[string]any{"status": "ok"})
}

func (s *workbenchHTTPServer) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	payload := map[string]any{"status": "ready"}
	if validWorkbenchToken(r.Header.Get(workbenchReadinessHeader), s.readinessToken) {
		manifest := s.currentManifest()
		payload["sessionId"] = s.sessionID
		payload["deckDir"] = manifest.DeckDir
		payload["pid"] = os.Getpid()
	}
	_ = writeJSONResponse(w, payload)
}

func (s *workbenchHTTPServer) handleWorkbench(w http.ResponseWriter, r *http.Request) {
	if !workbenchSessionPathMatches(r.URL.Path, s.sessionID, "") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, s.workbenchHTML())
}

func (s *workbenchHTTPServer) handleAsset(w http.ResponseWriter, r *http.Request) {
	prefix := "/workbench/" + s.sessionID + "/assets/"
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, prefix)
	if !safeWorkbenchAssetPath(name) {
		http.NotFound(w, r)
		return
	}
	raw, err := embeddedWorkbenchAssets.ReadFile("workbench_assets/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", workbenchAssetContentType(name))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

func safeWorkbenchAssetPath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	clean := path.Clean(name)
	if clean == "." || clean != name || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func workbenchAssetContentType(name string) string {
	if strings.HasSuffix(name, ".js") {
		return "application/javascript; charset=utf-8"
	}
	if strings.HasSuffix(name, ".css") {
		return "text/css; charset=utf-8"
	}
	if strings.HasSuffix(name, ".json") {
		return "application/json; charset=utf-8"
	}
	if strings.HasSuffix(name, ".html") {
		return "text/html; charset=utf-8"
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func decodeWorkbenchJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, workbenchJSONBodyMaxBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return errWorkbenchJSONTrailing
}

func writeWorkbenchJSONDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "invalid JSON", http.StatusBadRequest)
}

func readWorkbenchAssetManifest() (workbenchAssetManifest, error) {
	raw, err := embeddedWorkbenchAssets.ReadFile(workbenchAssetManifestName)
	if err != nil {
		return workbenchAssetManifest{}, err
	}
	var manifest workbenchAssetManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return workbenchAssetManifest{}, err
	}
	if manifest.SchemaVersion != "slidex.workbench.assets.v1" {
		return workbenchAssetManifest{}, fmt.Errorf("unsupported workbench asset manifest schema %q", manifest.SchemaVersion)
	}
	if manifest.SourcePackage != "@shiinamachi/slidex-workbench" || manifest.Framework != "solidstart" || !manifest.CSR {
		return workbenchAssetManifest{}, fmt.Errorf("workbench assets must come from the SolidStart CSR package")
	}
	if len(manifest.Scripts) == 0 {
		return workbenchAssetManifest{}, fmt.Errorf("workbench asset manifest has no module scripts")
	}
	for _, asset := range append(append(append([]string{}, manifest.ModulePreload...), manifest.Scripts...), manifest.Styles...) {
		if !safeWorkbenchAssetPath(asset) {
			return workbenchAssetManifest{}, fmt.Errorf("unsafe workbench asset path %q", asset)
		}
	}
	for _, script := range manifest.InlineScripts {
		if !strings.Contains(script, "window.manifest") || strings.Contains(strings.ToLower(script), "</script") {
			return workbenchAssetManifest{}, fmt.Errorf("unsafe Workbench inline manifest script")
		}
	}
	return manifest, nil
}

func workbenchAssetTags(assetBase string) string {
	manifest, err := readWorkbenchAssetManifest()
	if err != nil {
		return "<!-- workbench asset manifest unavailable: " + html.EscapeString(err.Error()) + " -->"
	}
	var b strings.Builder
	for _, preload := range manifest.ModulePreload {
		b.WriteString(`<link rel="modulepreload" href="`)
		b.WriteString(assetBase)
		b.WriteString(html.EscapeString(preload))
		b.WriteString(`">`)
		b.WriteByte('\n')
	}
	for _, style := range manifest.Styles {
		b.WriteString(`<link rel="stylesheet" href="`)
		b.WriteString(assetBase)
		b.WriteString(html.EscapeString(style))
		b.WriteString(`">`)
		b.WriteByte('\n')
	}
	for _, inlineScript := range manifest.InlineScripts {
		inlineScript, err = workbenchSessionManifestScript(inlineScript, assetBase)
		if err != nil {
			b.WriteString(`<!-- workbench inline manifest unavailable: `)
			b.WriteString(html.EscapeString(err.Error()))
			b.WriteString(` -->`)
			b.WriteByte('\n')
			continue
		}
		b.WriteString(`<script>`)
		b.WriteString(inlineScript)
		b.WriteString(`</script>`)
		b.WriteByte('\n')
	}
	for _, script := range manifest.Scripts {
		b.WriteString(`<script type="module" src="`)
		b.WriteString(assetBase)
		b.WriteString(html.EscapeString(script))
		b.WriteString(`"></script>`)
		b.WriteByte('\n')
	}
	return b.String()
}

func workbenchSessionManifestScript(inlineScript, assetBase string) (string, error) {
	trimmed := strings.TrimSpace(inlineScript)
	const prefix = "window.manifest = "
	if !strings.HasPrefix(trimmed, prefix) {
		return inlineScript, nil
	}
	raw := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	raw = strings.TrimSuffix(raw, ";")
	var manifest any
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		return "", fmt.Errorf("parse SolidStart manifest: %w", err)
	}
	rewriteSolidStartAssetURLs(manifest, assetBase)
	rewritten, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("encode SolidStart manifest: %w", err)
	}
	return "window.manifest = " + string(rewritten), nil
}

func rewriteSolidStartAssetURLs(value any, assetBase string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if pathValue, ok := child.(string); ok {
				typed[key] = rewriteSolidStartAssetURL(pathValue, assetBase)
				continue
			}
			rewriteSolidStartAssetURLs(child, assetBase)
		}
	case []any:
		for _, child := range typed {
			rewriteSolidStartAssetURLs(child, assetBase)
		}
	}
}

func rewriteSolidStartAssetURL(value, assetBase string) string {
	const buildPrefix = "/_build/"
	if !strings.HasPrefix(value, buildPrefix) {
		return value
	}
	return assetBase + strings.TrimPrefix(value, "/")
}

func (s *workbenchHTTPServer) handleSession(w http.ResponseWriter, r *http.Request) {
	if !workbenchSessionPathMatches(r.URL.Path, s.sessionID, "/api/session") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manifest := s.currentManifest()
	if !sameOriginOrNoOrigin(r, manifest.URL) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if !validWorkbenchToken(r.Header.Get("X-Slidex-Workbench-Token"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = writeJSONResponse(w, publicWorkbenchStatus(manifest))
}

func (s *workbenchHTTPServer) handleDraft(w http.ResponseWriter, r *http.Request) {
	if !workbenchSessionPathMatches(r.URL.Path, s.sessionID, "/api/draft") {
		http.NotFound(w, r)
		return
	}
	if !validWorkbenchToken(r.Header.Get("X-Slidex-Workbench-Token"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	manifestSnapshot := s.currentManifest()
	switch r.Method {
	case http.MethodGet:
		if !sameOriginOrNoOrigin(r, manifestSnapshot.URL) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		if draft, ok := readWorkbenchDraft(s.deckAbs); ok {
			_ = writeJSONResponse(w, map[string]any{"status": "ok", "draft": draft})
			return
		}
		_ = writeJSONResponse(w, map[string]any{"status": "empty"})
	case http.MethodPost:
		if !sameOriginRequired(r, manifestSnapshot.URL) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		defer r.Body.Close()
		var input workbenchSaveInput
		if err := decodeWorkbenchJSON(w, r, &input); err != nil {
			writeWorkbenchJSONDecodeError(w, err)
			return
		}
		input = normalizeWorkbenchInput(input)
		if !hasAnyWorkbenchInput(input) {
			http.Error(w, "draft is empty", http.StatusBadRequest)
			return
		}
		draft, err := writeWorkbenchDraft(s.deckAbs, input, "draft")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		manifest, err := updateWorkbenchManifest(s.deckAbs, manifestSnapshot, func(manifest *workbenchManifest) {
			manifest.Status = "draft"
			manifest.DraftSavedAt = draft.UpdatedAt
			manifest.DraftPath = filepath.ToSlash(filepath.Join(s.deckAbs, "out", workbenchDraftName))
			manifest.UpdatedAt = draft.UpdatedAt
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.setManifest(manifest)
		_ = writeJSONResponse(w, map[string]any{"status": "draft_saved", "draft": draft, "manifest": publicWorkbenchStatus(manifest)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *workbenchHTTPServer) handleSave(w http.ResponseWriter, r *http.Request) {
	if !workbenchSessionPathMatches(r.URL.Path, s.sessionID, "/api/save") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manifestSnapshot := s.currentManifest()
	if !sameOriginRequired(r, manifestSnapshot.URL) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if !validWorkbenchToken(r.Header.Get("X-Slidex-Workbench-Token"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()
	var input workbenchSaveInput
	if err := decodeWorkbenchJSON(w, r, &input); err != nil {
		writeWorkbenchJSONDecodeError(w, err)
		return
	}
	manifest, err := s.saveWorkbenchInput(input, "saved", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = writeJSONResponse(w, map[string]any{"status": "saved", "manifest": publicWorkbenchStatus(manifest)})
}

func (s *workbenchHTTPServer) handleComplete(w http.ResponseWriter, r *http.Request) {
	if !workbenchSessionPathMatches(r.URL.Path, s.sessionID, "/api/complete") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manifestSnapshot := s.currentManifest()
	if !sameOriginRequired(r, manifestSnapshot.URL) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if !validWorkbenchToken(r.Header.Get("X-Slidex-Workbench-Token"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()
	var input workbenchSaveInput
	if err := decodeWorkbenchJSON(w, r, &input); err != nil {
		writeWorkbenchJSONDecodeError(w, err)
		return
	}
	input = normalizeWorkbenchInput(input)
	if err := validateWorkbenchCompletionInput(input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	manifest, err := s.saveWorkbenchInput(input, "saved", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	manifest, reused, err := s.startGeneration(manifest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status := "generation_started"
	if reused {
		status = "generation_reused"
	}
	_ = writeJSONResponse(w, map[string]any{"status": status, "manifest": publicWorkbenchStatus(manifest)})
}

func (s *workbenchHTTPServer) saveWorkbenchInput(input workbenchSaveInput, status, wizardCompletedAt string) (workbenchManifest, error) {
	input = normalizeWorkbenchInput(input)
	if err := writeWorkbenchBrief(s.deckAbs, input); err != nil {
		return workbenchManifest{}, err
	}
	draft, err := writeWorkbenchDraft(s.deckAbs, input, status)
	if err != nil {
		return workbenchManifest{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	manifest, err := updateWorkbenchManifest(s.deckAbs, s.currentManifest(), func(manifest *workbenchManifest) {
		manifest.Status = status
		manifest.InputSavedAt = now
		if wizardCompletedAt != "" {
			manifest.WizardCompletedAt = wizardCompletedAt
		}
		manifest.DraftSavedAt = draft.UpdatedAt
		manifest.DraftPath = filepath.ToSlash(filepath.Join(s.deckAbs, "out", workbenchDraftName))
		manifest.UpdatedAt = now
		manifest.BriefPath = filepath.ToSlash(filepath.Join(s.deckAbs, "brief.md"))
		manifest.SavedFieldLengths = workbenchFieldLengths(input)
	})
	if err != nil {
		return workbenchManifest{}, err
	}
	s.setManifest(manifest)
	return manifest, nil
}

func validateWorkbenchCompletionInput(input workbenchSaveInput) error {
	missing := []string{}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "title", value: input.Title},
		{name: "audience", value: input.Audience},
		{name: "decisionGoal", value: input.DecisionGoal},
		{name: "sourceNotes", value: input.SourceNotes},
		{name: "keyMessages", value: input.KeyMessages},
		{name: "outputExpectations", value: input.OutputExpectations},
	} {
		if len([]rune(strings.TrimSpace(field.value))) < 8 {
			missing = append(missing, field.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("wizard completion requires more detail for: %s", strings.Join(missing, ", "))
	}
	return nil
}

func workbenchFieldLengths(input workbenchSaveInput) map[string]int {
	return map[string]int{
		"initialRequest":     len(input.InitialRequest),
		"title":              len(input.Title),
		"audience":           len(input.Audience),
		"decisionGoal":       len(input.DecisionGoal),
		"sourceNotes":        len(input.SourceNotes),
		"keyMessages":        len(input.KeyMessages),
		"requiredClaims":     len(input.RequiredClaims),
		"constraints":        len(input.Constraints),
		"outputExpectations": len(input.OutputExpectations),
	}
}

var (
	newWorkbenchGenerationCommand = defaultWorkbenchGenerationCommand
	workbenchGenerationTimeout    = 30 * time.Minute
)

func defaultWorkbenchGenerationCommand(ctx context.Context, deckAbs string) (*exec.Cmd, []string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	args := []string{"run", "--deck", deckAbs, "--non-interactive"}
	cmd := exec.CommandContext(ctx, exe, args...)
	return cmd, append([]string{exe}, args...), nil
}

func sanitizedWorkbenchChildEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	sanitized := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(strings.ToUpper(name), "SLIDEX_WORKBENCH_") {
			continue
		}
		sanitized = append(sanitized, entry)
	}
	return sanitized
}

type boundedWorkbenchLogWriter struct {
	mu        sync.Mutex
	file      *os.File
	limit     int64
	written   int64
	truncated bool
}

func openBoundedWorkbenchLog(path string) (*boundedWorkbenchLogWriter, error) {
	file, err := openSecureTruncateFile(path, 0o600)
	if err != nil {
		return nil, err
	}
	return &boundedWorkbenchLogWriter{file: file, limit: workbenchLogMaxBytes}, nil
}

func prepareWorkbenchServerOutput(logPath string) (*os.File, error) {
	placeholder := []byte("slidex workbench server output is discarded to avoid unbounded log growth.\n")
	if err := secureWriteFile(logPath, placeholder, 0o600); err != nil {
		return nil, err
	}
	return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
}

func (w *boundedWorkbenchLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || w.limit <= 0 || w.written >= w.limit {
		return len(p), nil
	}
	remaining := w.limit - w.written
	toWrite := p
	if int64(len(toWrite)) > remaining {
		toWrite = toWrite[:remaining]
	}
	if !w.truncated && int64(len(p)) > remaining {
		marker := []byte(workbenchLogTruncationMarker)
		if remaining > int64(len(marker)) {
			toWrite = append([]byte{}, p[:remaining-int64(len(marker))]...)
			toWrite = append(toWrite, marker...)
		} else {
			toWrite = marker[:remaining]
		}
		w.truncated = true
	}
	if len(toWrite) == 0 {
		return len(p), nil
	}
	n, err := w.file.Write(toWrite)
	w.written += int64(n)
	if err != nil {
		return n, err
	}
	return len(p), nil
}

func (w *boundedWorkbenchLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (s *workbenchHTTPServer) startGeneration(manifest workbenchManifest) (workbenchManifest, bool, error) {
	if manifest.GenerationStatus == "running" && processAlive(manifest.GenerationPID) {
		return manifest, true, nil
	}
	logPath := filepath.Join(s.deckAbs, "out", workbenchGenerationLogName)
	logFile, err := openBoundedWorkbenchLog(logPath)
	if err != nil {
		return manifest, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), workbenchGenerationTimeout)
	cmd, command, err := newWorkbenchGenerationCommand(ctx, s.deckAbs)
	if err != nil {
		cancel()
		_ = logFile.Close()
		return manifest, false, err
	}
	cmd.Env = sanitizedWorkbenchChildEnv(cmd.Env)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	configureManagedAppServerCommand(cmd)
	if err := cmd.Start(); err != nil {
		cancel()
		_ = logFile.Close()
		return manifest, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	manifest, err = updateWorkbenchManifest(s.deckAbs, manifest, func(manifest *workbenchManifest) {
		manifest.Status = "generating"
		manifest.GenerationStatus = "running"
		manifest.GenerationStartedAt = now
		manifest.GenerationEndedAt = ""
		manifest.GenerationPID = cmd.Process.Pid
		manifest.GenerationExitCode = 0
		manifest.GenerationCommand = command
		manifest.GenerationLogPath = filepath.ToSlash(logPath)
		manifest.UpdatedAt = now
	})
	if err != nil {
		cancel()
		signalManagedProcess(cmd.Process.Pid)
		_ = logFile.Close()
		return manifest, false, err
	}
	s.setManifest(manifest)
	go s.waitForGeneration(ctx, cancel, cmd, logFile, manifest)
	return manifest, false, nil
}

func (s *workbenchHTTPServer) waitForGeneration(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, logFile io.Closer, started workbenchManifest) {
	err := cmd.Wait()
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	cancel()
	_ = logFile.Close()
	endedAt := time.Now().UTC().Format(time.RFC3339)
	manifest, writeErr := updateWorkbenchManifest(s.deckAbs, started, func(manifest *workbenchManifest) {
		manifest.GenerationEndedAt = endedAt
		manifest.UpdatedAt = endedAt
		manifest.GenerationExitCode = generationExitCode(cmd)
		switch {
		case timedOut:
			manifest.Status = "generation_failed"
			manifest.GenerationStatus = "timeout"
			manifest.Notes = append(manifest.Notes, fmt.Sprintf("Workbench generation exceeded %s and was terminated.", workbenchGenerationTimeout))
		case err == nil:
			manifest.Status = "generated"
			manifest.GenerationStatus = "completed"
		default:
			manifest.Status = "generation_failed"
			manifest.GenerationStatus = "failed"
		}
	})
	if writeErr == nil {
		s.setManifest(manifest)
	}
}

func generationExitCode(cmd *exec.Cmd) int {
	if cmd == nil || cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}

func (s *workbenchHTTPServer) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !workbenchSessionPathMatches(r.URL.Path, s.sessionID, "/api/shutdown") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manifestSnapshot := s.currentManifest()
	if !sameOriginRequired(r, manifestSnapshot.URL) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if !validWorkbenchToken(r.Header.Get("X-Slidex-Workbench-Shutdown-Token"), s.shutdownToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	manifest, err := updateWorkbenchManifest(s.deckAbs, manifestSnapshot, func(manifest *workbenchManifest) {
		manifest.Status = "stopping"
		manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setManifest(manifest)
	_ = writeJSONResponse(w, map[string]any{"status": "stopping", "manifest": publicWorkbenchStatus(manifest)})
	if s.shutdown != nil {
		go func() {
			time.Sleep(20 * time.Millisecond)
			s.shutdown()
		}()
	}
}

func workbenchSessionPathMatches(path, sessionID, suffix string) bool {
	return path == "/workbench/"+sessionID+suffix
}

func (s *workbenchHTTPServer) workbenchHTML() string {
	manifest := s.currentManifest()
	filePathHTML := workbenchFilePathHTML(manifest)
	bootstrap := map[string]any{
		"deckId":        manifest.DeckID,
		"deckDir":       manifest.DeckDir,
		"sessionId":     s.sessionID,
		"apiBase":       "/workbench/" + s.sessionID + "/api",
		"assetBase":     "/workbench/" + s.sessionID + "/assets/",
		"workbenchBase": "/workbench/" + s.sessionID,
		"token":         s.token,
		"filePathHTML":  filePathHTML,
	}
	raw, _ := json.Marshal(bootstrap)
	title := html.EscapeString(manifest.DeckID)
	assetBase := "/workbench/" + s.sessionID + "/assets/"
	return `<!doctype html>
<html lang="ko">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>slidex workbench - ` + title + `</title>
  <style>
    :root { color-scheme: light; --ink:#182026; --muted:#53606b; --line:#d7dde2; --soft:#f4f7f9; --accent:#0f766e; --accent-strong:#0b5f59; --paper:#ffffff; --warn:#8a5a00; --blue:#2563eb; --amber:#b45309; }
    * { box-sizing: border-box; }
    body { margin:0; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color:var(--ink); background:var(--soft); }
    main { max-width: 1120px; margin: 0 auto; padding: 28px; }
    .wizard-header { display:flex; align-items:flex-start; justify-content:space-between; gap:20px; margin-bottom:18px; }
    .eyebrow { margin:0 0 5px; color:var(--accent); font-size:12px; font-weight:750; text-transform:uppercase; letter-spacing:0; }
    h1 { margin:0; font-size:24px; line-height:1.2; letter-spacing:0; }
    h2 { margin:0; font-size:17px; line-height:1.3; letter-spacing:0; }
    .meta { color:var(--muted); font-size:13px; line-height:1.5; overflow-wrap:anywhere; }
    .status-banners { display:grid; gap:10px; margin:0 0 18px; }
    .status-banner { border:1px solid var(--line); border-left-width:4px; border-radius:6px; background:var(--paper); padding:10px 12px; font-size:13px; line-height:1.45; }
    .status-banner strong { display:block; font-size:13px; margin-bottom:2px; }
    .status-banner code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size:12px; overflow-wrap:anywhere; }
    .status-banner.warn { border-left-color:var(--warn); }
    .status-banner.info { border-left-color:#2563eb; }
    .status-banner.ok { border-left-color:var(--accent); }
    .stepper { display:flex; flex-wrap:wrap; gap:8px; margin:0 0 14px; }
    .step { border:1px solid var(--line); border-radius:6px; padding:8px 10px; min-height:36px; background:var(--paper); color:var(--ink); font-weight:700; cursor:pointer; }
    .step.active { border-color:var(--accent); color:var(--accent-strong); box-shadow: inset 0 0 0 1px var(--accent); }
    form { display:grid; gap:14px; }
    .step-panel { border:1px solid var(--line); border-radius:8px; background:var(--paper); padding:16px; }
    .field-grid { display:grid; grid-template-columns: minmax(0,1fr) minmax(0,1fr); gap:14px; margin-top:14px; }
    .field { display:grid; gap:7px; font-size:13px; font-weight:650; }
    input, textarea { width:100%; min-width:0; border:1px solid var(--line); border-radius:6px; padding:11px 12px; font:inherit; background:var(--paper); color:var(--ink); }
    textarea { min-height:122px; resize:vertical; line-height:1.45; }
    .field-wide { grid-column:1 / -1; }
    .actions { display:flex; align-items:center; flex-wrap:wrap; gap:10px; }
    button { border:0; border-radius:6px; padding:10px 14px; min-height:40px; background:var(--accent); color:white; font-weight:700; cursor:pointer; }
    button:hover { background:var(--accent-strong); }
    button:disabled { opacity:.58; cursor:not-allowed; }
    button.primary { background:var(--blue); }
    button.primary:hover { background:#1d4ed8; }
    .status { font-size:13px; color:var(--muted); }
    .status.warn { color:var(--warn); }
    .review-grid { display:grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap:10px; }
    .review-item { border:1px solid var(--line); border-radius:6px; padding:10px; display:grid; gap:4px; font-size:13px; }
    .review-item span { color:var(--accent-strong); }
    .review-item.missing span { color:var(--amber); }
    .generation, .paths { margin-top:18px; border-top:1px solid var(--line); padding-top:14px; }
    .generation dl,
    .paths dl { margin:0; display:grid; grid-template-columns: minmax(90px, max-content) minmax(0, 1fr); gap:8px 14px; font-size:13px; line-height:1.45; }
    .generation dt,
    .paths dt { color:var(--muted); font-weight:650; }
    .generation dd,
    .paths dd { margin:0; overflow-wrap:anywhere; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
    .paths h2 { margin:0 0 10px; font-size:14px; line-height:1.3; letter-spacing:0; }
    .notice { margin-top:18px; border-top:1px solid var(--line); padding-top:14px; color:var(--muted); font-size:13px; line-height:1.5; }
    @media (max-width: 760px) { main { padding:20px; } .wizard-header { display:block; } .field-grid, .review-grid { grid-template-columns:1fr; } .deck-dir { margin-top:8px; } }
  </style>
</head>
<body>
  <main>
    ` + workbenchStatusBannersHTML() + `
	    <div id="app">
	      <header class="wizard-header">
	        <div>
	          <p class="eyebrow">slidex Solid Workbench</p>
	          <h1>Deck intake workbench</h1>
          <div class="meta">Deck: <strong>` + title + `</strong></div>
        </div>
        <div class="meta deck-dir">` + html.EscapeString(manifest.DeckDir) + `</div>
      </header>
      <nav class="stepper" aria-label="Wizard steps">
        <button class="step active" type="button">1. 요청 정리</button>
        <button class="step" type="button">2. 목표</button>
        <button class="step" type="button">3. 근거</button>
        <button class="step" type="button">4. 메시지</button>
        <button class="step" type="button">5. 검토</button>
      </nav>
      <form id="deck-form">
        <section class="step-panel">
          <h2>요청 정리</h2>
          <div class="field-grid">
            <label class="field field-wide"><span>요청 원문</span><textarea name="initialRequest" spellcheck="true"></textarea></label>
            <label class="field"><span>문서 제목</span><input name="title" autocomplete="off" required></label>
            <label class="field"><span>핵심 청중</span><input name="audience" autocomplete="off" required></label>
            <label class="field field-wide"><span>결정 목표</span><textarea name="decisionGoal" spellcheck="true" required></textarea></label>
            <label class="field field-wide"><span>근거 자료와 확정된 사실</span><textarea name="sourceNotes" spellcheck="true"></textarea></label>
            <label class="field field-wide"><span>핵심 메시지</span><textarea name="keyMessages" spellcheck="true"></textarea></label>
            <label class="field field-wide"><span>검증 필요 주장</span><textarea name="requiredClaims" spellcheck="true"></textarea></label>
            <label class="field field-wide"><span>제외/주의사항</span><textarea name="constraints" spellcheck="true"></textarea></label>
            <label class="field field-wide"><span>출력 기대</span><textarea name="outputExpectations" spellcheck="true"></textarea></label>
          </div>
        </section>
        <div class="actions"><button type="submit">Save brief</button><button class="primary" type="button">Complete & generate</button><output class="status"></output></div>
      </form>
      <section class="paths" aria-label="Deck files">
        <h2>Deck files</h2>
        <dl>` + filePathHTML + `</dl>
      </section>
	      <div class="notice">SolidStart assets are served locally from this workbench session. If scripts are unavailable, this fallback form still exposes the deck intake fields.</div>
	    </div>
	  </main>
	  <script>
	    const boot = ` + string(raw) + `;
	    window.__SLIDEX_WORKBENCH__ = boot;
	  </script>
	  ` + workbenchAssetTags(assetBase) + `
	</body>
	</html>`
}

func workbenchFilePathHTML(manifest workbenchManifest) string {
	paths := manifest.Paths
	if paths == nil {
		paths = map[string]string{}
	}
	items := []struct {
		key   string
		label string
		path  string
	}{
		{key: "brief", label: "Brief", path: filepath.ToSlash(filepath.Join(manifest.DeckDir, "brief.md"))},
		{key: "draft", label: "Draft", path: filepath.ToSlash(filepath.Join(manifest.OutDir, workbenchDraftName))},
		{key: "manifest", label: "Manifest", path: filepath.ToSlash(filepath.Join(manifest.OutDir, workbenchManifestName))},
	}
	var b strings.Builder
	for _, item := range items {
		path := firstNonEmpty(paths[item.key], item.path)
		b.WriteString("<dt>" + html.EscapeString(item.label) + "</dt><dd>" + html.EscapeString(path) + "</dd>")
	}
	return b.String()
}

func workbenchStatusBannersHTML() string {
	snapshot := updateStatusSnapshot()
	banners, _ := snapshot["banners"].([]statusBanner)
	if len(banners) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="status-banners" aria-label="slidex status">`)
	for _, banner := range banners {
		severity := html.EscapeString(firstNonEmpty(banner.Severity, "info"))
		b.WriteString(`<div class="status-banner ` + severity + `" data-banner-id="` + html.EscapeString(banner.ID) + `">`)
		b.WriteString(`<strong>` + html.EscapeString(banner.Title) + `</strong>`)
		b.WriteString(`<span>` + html.EscapeString(banner.Message) + `</span>`)
		if banner.Command != "" {
			b.WriteString(`<div><code>` + html.EscapeString(banner.Command) + `</code></div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

func bootstrapDeckWorkspace(workspace, deckID, fromTemplate string, allowExisting bool) (deckBootstrapResult, error) {
	if workspace == "" {
		workspace = "."
	}
	root := workspaceRoot(workspace)
	if err := validateDeckID(deckID); err != nil {
		return deckBootstrapResult{}, err
	}
	deckAbs, err := safeDeckDir(root, deckID)
	if err != nil {
		return deckBootstrapResult{}, err
	}
	if _, err := os.Stat(deckAbs); err == nil {
		if !allowExisting {
			return deckBootstrapResult{}, fmt.Errorf("deck already exists: %s", filepath.ToSlash(displayDeckPath(root, deckAbs)))
		}
		return deckBootstrapResult{Workspace: filepath.ToSlash(root), DeckID: deckID, DeckDir: filepath.ToSlash(deckAbs), Status: "existing"}, nil
	} else if !os.IsNotExist(err) {
		return deckBootstrapResult{}, err
	}
	templateAbs := resolveTemplateDir(root, fromTemplate)
	if pathExists(templateAbs) {
		if err := copyDeckTemplateDir(templateAbs, deckAbs); err != nil {
			_ = removePartialDeckWorkspace(root, deckAbs)
			return deckBootstrapResult{}, err
		}
	} else if isDefaultTemplateRef(fromTemplate) {
		if err := copyEmbeddedDefaultTemplate(deckAbs); err != nil {
			_ = removePartialDeckWorkspace(root, deckAbs)
			return deckBootstrapResult{}, err
		}
	} else {
		if err := copyDeckTemplateDir(templateAbs, deckAbs); err != nil {
			_ = removePartialDeckWorkspace(root, deckAbs)
			return deckBootstrapResult{}, err
		}
	}
	return deckBootstrapResult{Workspace: filepath.ToSlash(root), DeckID: deckID, DeckDir: filepath.ToSlash(displayDeckPath(root, deckAbs)), Status: "created"}, nil
}

func removePartialDeckWorkspace(root, deckAbs string) error {
	decksRoot := filepath.Join(workspaceRoot(root), "decks")
	cleanDeck := filepath.Clean(deckAbs)
	if filepath.VolumeName(decksRoot) != filepath.VolumeName(cleanDeck) || !pathWithin(decksRoot, cleanDeck) {
		return fmt.Errorf("partial deck cleanup target escapes decks root: %s", filepath.ToSlash(cleanDeck))
	}
	info, err := os.Lstat(cleanDeck)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if isSymlinkOrReparsePoint(cleanDeck, info) {
		return fmt.Errorf("partial deck cleanup target must not be a symlink or reparse point: %s", filepath.ToSlash(cleanDeck))
	}
	if !info.IsDir() {
		return fmt.Errorf("partial deck cleanup target must be a directory: %s", filepath.ToSlash(cleanDeck))
	}
	if err := rejectSymlinkAncestors(filepath.Dir(cleanDeck)); err != nil {
		return err
	}
	return os.RemoveAll(cleanDeck)
}

func resolveTemplateDir(root, fromTemplate string) string {
	template := fromTemplate
	if template == "" {
		template = defaultDeckTemplatePath
	}
	if filepath.IsAbs(template) {
		return filepath.Clean(template)
	}
	rootCandidate := filepath.Join(root, template)
	if pathExists(rootCandidate) {
		return rootCandidate
	}
	if !isDefaultTemplateRef(template) {
		return rootCandidate
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidate := filepath.Join(dir, template)
			if pathExists(candidate) {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	return rootCandidate
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func resolveDeckDir(workspace, deckID, deck string, create bool, fromTemplate string) (string, error) {
	if workspace == "" {
		workspace = "."
	}
	root := workspaceRoot(workspace)
	if deck != "" {
		deckAbs := deck
		if !filepath.IsAbs(deckAbs) {
			deckAbs = filepath.Join(root, deckAbs)
		}
		deckAbs = filepath.Clean(deckAbs)
		decksRoot := filepath.Join(root, "decks")
		if !pathWithin(decksRoot, deckAbs) {
			return "", fmt.Errorf("deck path must stay under %s: %s", filepath.ToSlash(decksRoot), filepath.ToSlash(deckAbs))
		}
		if err := rejectSymlinkEscape(decksRoot, deckAbs, false); err != nil {
			return "", err
		}
		if _, err := os.Stat(deckAbs); err != nil {
			return "", err
		}
		return deckAbs, nil
	}
	if deckID == "" {
		return "", errors.New("deckId or deck is required")
	}
	if create {
		result, err := bootstrapDeckWorkspace(root, deckID, fromTemplate, true)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(result.DeckDir) {
			return filepath.Clean(result.DeckDir), nil
		}
		return filepath.Join(root, result.DeckDir), nil
	}
	if err := validateDeckID(deckID); err != nil {
		return "", err
	}
	return safeDeckDir(root, deckID)
}

func validateDeckID(deckID string) error {
	if !deckIDPattern.MatchString(deckID) {
		return exitCodeError(2, "deck_id must start with a letter or number and contain only letters, numbers, underscore, dash, and dot")
	}
	if deckID == "." || deckID == ".." {
		return exitCodeError(2, "deck_id must not be a dot path segment")
	}
	if strings.HasSuffix(deckID, ".") {
		return exitCodeError(2, "deck_id must not end with a dot because it is not portable on Windows")
	}
	if windowsReservedDeckID(deckID) {
		return exitCodeError(2, "deck_id must not use a Windows reserved device name")
	}
	return nil
}

func windowsReservedDeckID(deckID string) bool {
	name := deckID
	if before, _, ok := strings.Cut(deckID, "."); ok {
		name = before
	}
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func safeDeckDir(root, deckID string) (string, error) {
	decksRoot := filepath.Join(root, "decks")
	if collision, err := caseInsensitiveDeckIDCollision(decksRoot, deckID); err != nil {
		return "", err
	} else if collision != "" {
		return "", exitCodeError(2, "deck_id collides with existing deck on case-insensitive file systems: %s", collision)
	}
	deckAbs := filepath.Clean(filepath.Join(decksRoot, deckID))
	if !pathWithin(decksRoot, deckAbs) {
		return "", fmt.Errorf("deck path escapes decks directory: %s", deckID)
	}
	if err := rejectSymlinkEscape(decksRoot, deckAbs, true); err != nil {
		return "", err
	}
	return deckAbs, nil
}

func caseInsensitiveDeckIDCollision(decksRoot, deckID string) (string, error) {
	entries, err := os.ReadDir(decksRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name != deckID && strings.EqualFold(name, deckID) {
			return name, nil
		}
	}
	return "", nil
}

func workspaceRoot(workspace string) string {
	if workspace == "" {
		workspace = "."
	}
	return filepath.Clean(mustAbs(workspace))
}

func displayDeckPath(root, deckAbs string) string {
	if rel, err := filepath.Rel(root, deckAbs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return deckAbs
}

func pathWithin(root, child string) bool {
	root = filepath.Clean(root)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func rejectSymlinkEscape(root, target string, allowMissingTarget bool) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if !pathWithin(root, target) {
		return fmt.Errorf("path escapes root: %s", filepath.ToSlash(target))
	}
	if err := rejectSymlinkComponent(root); err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) && allowMissingTarget {
				return nil
			}
			return err
		}
		if isSymlinkOrReparsePoint(current, info) {
			return fmt.Errorf("deck path must not contain symlinks or reparse points: %s", filepath.ToSlash(current))
		}
	}
	return nil
}

func rejectSymlinkComponent(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if isSymlinkOrReparsePoint(path, info) {
		return fmt.Errorf("deck path must not contain symlinks or reparse points: %s", filepath.ToSlash(path))
	}
	return nil
}

func isSymlinkOrReparsePoint(path string, info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0 || isReparsePoint(path)
}

func workbenchStatus(workspace, deckID, deck string) (workbenchManifest, error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, defaultDeckTemplatePath)
	if err != nil {
		return workbenchManifest{}, err
	}
	if _, ok := readWorkbenchManifest(deckAbs); !ok {
		return workbenchStatusForDeck(deckAbs), nil
	}
	unlock, err := acquireWorkbenchLock(filepath.Join(deckAbs, "out"))
	if err != nil {
		return workbenchManifest{}, err
	}
	defer unlock()
	manifest := workbenchStatusForDeck(deckAbs)
	if manifest.Status == "stale" || manifest.Status == "running" {
		_ = writeWorkbenchManifest(deckAbs, manifest)
	}
	return manifest, nil
}

func workbenchStatusForDeck(deckAbs string) workbenchManifest {
	manifest, ok := readWorkbenchManifest(deckAbs)
	if !ok {
		return canonicalWorkbenchManifestPaths(deckAbs, workbenchManifest{Status: "not_started"})
	}
	manifest = canonicalWorkbenchManifestPaths(deckAbs, manifest)
	if isTrustedWorkbenchReady(manifest) {
		manifest.Status = "running"
	} else if manifest.Status == "running" || manifest.Status == "starting" {
		manifest.Status = "stale"
		manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return manifest
}

func stopWorkbench(workspace, deckID, deck string) (workbenchManifest, error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, defaultDeckTemplatePath)
	if err != nil {
		return workbenchManifest{}, err
	}
	unlock, err := acquireWorkbenchLock(filepath.Join(deckAbs, "out"))
	if err != nil {
		return workbenchManifest{}, err
	}
	defer unlock()
	manifest := workbenchStatusForDeck(deckAbs)
	if manifest.PID > 0 && manifest.Status == "running" {
		stopWorkbenchProcess(manifest)
	}
	removeWorkbenchControl(deckAbs)
	manifest.Status = "stopped"
	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = writeWorkbenchManifest(deckAbs, manifest)
	return manifest, nil
}

func canonicalWorkbenchManifestPaths(deckAbs string, manifest workbenchManifest) workbenchManifest {
	manifest.DeckID = filepath.Base(deckAbs)
	manifest.DeckDir = filepath.ToSlash(deckAbs)
	manifest.OutDir = filepath.ToSlash(filepath.Join(deckAbs, "out"))
	if manifest.Paths == nil {
		manifest.Paths = map[string]string{}
	}
	manifest.Paths["brief"] = filepath.ToSlash(filepath.Join(deckAbs, "brief.md"))
	manifest.Paths["draft"] = filepath.ToSlash(filepath.Join(deckAbs, "out", workbenchDraftName))
	manifest.Paths["manifest"] = filepath.ToSlash(filepath.Join(deckAbs, "out", workbenchManifestName))
	return manifest
}

func acquireWorkbenchLock(outDir string) (func(), error) {
	if err := ensureSecureDir(outDir); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(outDir, workbenchLockName)
	deadline := time.Now().Add(workbenchLockWaitTimeout)
	for {
		if err := rejectSecureWriteTarget(lockPath); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if err := applyPlatformFileMode(lockPath, 0o600); err != nil {
				_ = f.Close()
				_ = os.Remove(lockPath)
				return nil, err
			}
			_, _ = fmt.Fprintf(f, "schema=%s tool=slidex pid=%d nonce=%s acquired=%s\n", workbenchLockSchemaMarker, os.Getpid(), newLockNonce(), time.Now().UTC().Format(time.RFC3339))
			return func() {
				releaseLockFile(lockPath, f)
			}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if reclaimStaleLockFile(lockPath, maxDeckLogBytes, staleWorkbenchLockSnapshot) {
			continue
		}
		now := time.Now()
		if workbenchLockWaitTimeout <= 0 || !deadline.After(now) {
			return nil, fmt.Errorf("workbench lock %s is still held after %s", lockPath, workbenchLockWaitTimeout)
		}
		sleepFor := workbenchLockRetryDelay
		if sleepFor <= 0 {
			sleepFor = 50 * time.Millisecond
		}
		if remaining := deadline.Sub(now); remaining < sleepFor {
			sleepFor = remaining
		}
		time.Sleep(sleepFor)
	}
}

func staleWorkbenchLock(lockPath string) bool {
	_, stale := staleWorkbenchLockSnapshot(lockPath)
	return stale
}

func staleWorkbenchLockSnapshot(lockPath string) (lockFileSnapshot, bool) {
	snapshot, ok := readLockFileSnapshot(lockPath, maxDeckLogBytes)
	if !ok {
		return lockFileSnapshot{}, false
	}
	staleAfter := workbenchLockStaleAfter
	if staleAfter <= 0 {
		staleAfter = 5 * time.Second
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
	if fields["schema"] == workbenchLockSchemaMarker && fields["tool"] == "slidex" {
		pid, err := strconv.Atoi(fields["pid"])
		if err != nil || pid <= 0 {
			return snapshot, time.Since(snapshot.info.ModTime()) > staleAfter
		}
		return snapshot, !processAlive(pid)
	}
	return snapshot, time.Since(snapshot.info.ModTime()) > staleAfter
}

func stopWorkbenchProcess(manifest workbenchManifest) {
	if manifest.PID <= 0 {
		return
	}
	if !workbenchManifestHasTrustedControl(manifest) {
		return
	}
	if requestWorkbenchShutdown(manifest) {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if !isWorkbenchReady(manifest) {
				return
			}
			time.Sleep(80 * time.Millisecond)
		}
	}
	if !workbenchProcessMatchesManifest(manifest) {
		return
	}
	signalWorkbenchProcessFn(manifest.PID)
	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !isWorkbenchReady(manifest) {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
	killWorkbenchProcessFn(manifest.PID)
}

func requestWorkbenchShutdown(manifest workbenchManifest) bool {
	control, ok := trustedWorkbenchControl(manifest)
	if !ok {
		return false
	}
	origin := fmt.Sprintf("http://127.0.0.1:%d", manifest.Port)
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	shutdownURL := absoluteWorkbenchAPIURL(manifest.URL, "/workbench/"+manifest.SessionID+"/api/shutdown")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, shutdownURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Slidex-Workbench-Shutdown-Token", control.ShutdownKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16*1024))
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

func workbenchServeArgsMatch(args []string, manifest workbenchManifest) bool {
	if manifest.Workspace == "" || manifest.DeckDir == "" || manifest.SessionID == "" || manifest.Port <= 0 {
		return false
	}
	flagArgs, ok := workbenchServeFlagArgs(args)
	if !ok {
		return false
	}
	workspace, ok := flagValueFromArgs(flagArgs, "--workspace")
	if !ok || !sameFilesystemPath(workspace, manifest.Workspace) {
		return false
	}
	deck, ok := flagValueFromArgs(flagArgs, "--deck")
	if !ok || !sameFilesystemPath(deck, manifest.DeckDir) {
		return false
	}
	sessionID, ok := flagValueFromArgs(flagArgs, "--session")
	if !ok || sessionID != manifest.SessionID {
		return false
	}
	port, ok := flagValueFromArgs(flagArgs, "--port")
	if !ok || port != strconv.Itoa(manifest.Port) {
		return false
	}
	if tokenEnv, ok := flagValueFromArgs(flagArgs, "--token-env"); !ok || tokenEnv != workbenchTokenEnv {
		return false
	}
	if shutdownEnv, ok := flagValueFromArgs(flagArgs, "--shutdown-token-env"); !ok || shutdownEnv != workbenchShutdownTokenEnv {
		return false
	}
	if readinessEnv, ok := flagValueFromArgs(flagArgs, "--readiness-token-env"); !ok || readinessEnv != workbenchReadinessTokenEnv {
		return false
	}
	return true
}

func workbenchServeFlagArgs(args []string) ([]string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "workbench" && args[i+1] == "serve" {
			return args[i+2:], true
		}
	}
	return nil, false
}

func flagValueFromArgs(args []string, name string) (string, bool) {
	prefix := name + "="
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == name:
			if i+1 >= len(args) {
				return "", false
			}
			return args[i+1], true
		case strings.HasPrefix(args[i], prefix):
			return strings.TrimPrefix(args[i], prefix), true
		}
	}
	return "", false
}

func newWorkbenchManifest(deckAbs, workspace, sessionID, token string, port, pid int, status string) workbenchManifest {
	now := time.Now().UTC().Format(time.RFC3339)
	urlValue := fmt.Sprintf("http://127.0.0.1:%d/workbench/%s", port, sessionID)
	return workbenchManifest{
		SchemaVersion:       "slidex.workbenchManifest.v1",
		ToolName:            toolName,
		ToolVersion:         toolVersion,
		Status:              status,
		DeckID:              filepath.Base(deckAbs),
		DeckDir:             filepath.ToSlash(deckAbs),
		OutDir:              filepath.ToSlash(filepath.Join(deckAbs, "out")),
		Workspace:           filepath.ToSlash(workspaceRoot(workspace)),
		Host:                "127.0.0.1",
		Port:                port,
		URL:                 urlValue,
		SessionID:           sessionID,
		TokenSHA256:         sha256Hex(token),
		TokenRedacted:       true,
		PID:                 pid,
		ServerBind:          "127.0.0.1",
		HealthPath:          "/healthz",
		ReadinessPath:       "/readyz",
		CreatedAt:           now,
		UpdatedAt:           now,
		BrowserOpenStrategy: "Packaged Codex MCP returns agentBrowserInstruction by default so the agent explicitly uses @Browser; legacy browserOpen intent is optional. No proprietary Canvas mount API is used.",
		Notes: []string{
			"Server binds to 127.0.0.1 only.",
			"Mutating routes require X-Slidex-Workbench-Token and same-origin validation.",
			"Full write token is not written to manifests or CLI output.",
			"Draft state is autosaved under out/workbench_draft.json for reload and crash recovery.",
		},
		Paths: map[string]string{
			"brief":    filepath.ToSlash(filepath.Join(deckAbs, "brief.md")),
			"draft":    filepath.ToSlash(filepath.Join(deckAbs, "out", workbenchDraftName)),
			"manifest": filepath.ToSlash(filepath.Join(deckAbs, "out", workbenchManifestName)),
		},
	}
}

func readWorkbenchManifest(deckAbs string) (workbenchManifest, bool) {
	unlock := lockWorkbenchManifest(deckAbs)
	defer unlock()
	return readWorkbenchManifestUnlocked(deckAbs)
}

func readWorkbenchManifestUnlocked(deckAbs string) (workbenchManifest, bool) {
	path := workbenchManifestPath(deckAbs)
	raw, err := readRegularFileWithMaxBytes(path, maxDeckJSONBytes)
	if err != nil {
		return workbenchManifest{}, false
	}
	var manifest workbenchManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return workbenchManifest{}, false
	}
	return manifest, true
}

func writeWorkbenchManifest(deckAbs string, manifest workbenchManifest) error {
	unlock := lockWorkbenchManifest(deckAbs)
	defer unlock()
	return writeWorkbenchManifestUnlocked(deckAbs, manifest)
}

func updateWorkbenchManifest(deckAbs string, fallback workbenchManifest, mutate func(*workbenchManifest)) (workbenchManifest, error) {
	unlock := lockWorkbenchManifest(deckAbs)
	defer unlock()
	manifest := fallback
	if current, ok := readWorkbenchManifestUnlocked(deckAbs); ok {
		manifest = current
	}
	mutate(&manifest)
	if err := writeWorkbenchManifestUnlocked(deckAbs, manifest); err != nil {
		return workbenchManifest{}, err
	}
	return manifest, nil
}

func writeWorkbenchManifestUnlocked(deckAbs string, manifest workbenchManifest) error {
	return secureWriteJSON(workbenchManifestPath(deckAbs), manifest)
}

func workbenchManifestPath(deckAbs string) string {
	return filepath.Join(deckAbs, "out", workbenchManifestName)
}

var workbenchManifestLocks sync.Map

func lockWorkbenchManifest(deckAbs string) func() {
	key := filepath.Clean(workbenchManifestPath(deckAbs))
	value, _ := workbenchManifestLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func workbenchControlPath(deckAbs string) string {
	return filepath.Join(deckAbs, "out", workbenchControlName)
}

func newWorkbenchControl(manifest workbenchManifest, shutdownKey, readinessKey string) workbenchControl {
	return workbenchControl{
		SchemaVersion: "slidex.workbenchControl.v1",
		ToolName:      toolName,
		ToolVersion:   toolVersion,
		SessionID:     manifest.SessionID,
		DeckDir:       manifest.DeckDir,
		PID:           manifest.PID,
		Port:          manifest.Port,
		URL:           manifest.URL,
		ShutdownKey:   shutdownKey,
		ReadinessKey:  readinessKey,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}

func writeWorkbenchControl(deckAbs string, control workbenchControl) error {
	raw, err := json.MarshalIndent(control, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return secureWriteFile(workbenchControlPath(deckAbs), raw, 0o600)
}

func readWorkbenchControl(deckAbs string) (workbenchControl, bool) {
	path := workbenchControlPath(deckAbs)
	if err := rejectSecureWriteTarget(path); err != nil {
		return workbenchControl{}, false
	}
	if err := requirePrivateFile(path, "workbench control file"); err != nil {
		return workbenchControl{}, false
	}
	raw, err := readRegularFileWithMaxBytes(path, maxDeckJSONBytes)
	if err != nil {
		return workbenchControl{}, false
	}
	var control workbenchControl
	if err := json.Unmarshal(raw, &control); err != nil {
		return workbenchControl{}, false
	}
	return control, true
}

func removeWorkbenchControl(deckAbs string) {
	path := workbenchControlPath(deckAbs)
	if err := rejectSecureWriteTarget(path); err != nil {
		return
	}
	_ = os.Remove(path)
}

func workbenchControlMatchesManifest(control workbenchControl, manifest workbenchManifest) bool {
	return control.SchemaVersion == "slidex.workbenchControl.v1" &&
		control.ShutdownKey != "" &&
		control.ReadinessKey != "" &&
		control.SessionID == manifest.SessionID &&
		control.DeckDir == manifest.DeckDir &&
		control.PID == manifest.PID &&
		control.Port == manifest.Port &&
		control.URL == manifest.URL
}

func isTrustedWorkbenchReady(manifest workbenchManifest) bool {
	return workbenchManifestHasTrustedControl(manifest) && isWorkbenchReady(manifest)
}

func workbenchManifestHasTrustedControl(manifest workbenchManifest) bool {
	_, ok := trustedWorkbenchControl(manifest)
	return ok
}

func trustedWorkbenchControl(manifest workbenchManifest) (workbenchControl, bool) {
	if manifest.Host != "127.0.0.1" || manifest.Port <= 0 || manifest.SessionID == "" || manifest.DeckDir == "" {
		return workbenchControl{}, false
	}
	origin, ok := originFromURL(manifest.URL)
	if !ok || origin != fmt.Sprintf("http://127.0.0.1:%d", manifest.Port) {
		return workbenchControl{}, false
	}
	deckAbs := filepath.Clean(filepath.FromSlash(manifest.DeckDir))
	control, ok := readWorkbenchControl(deckAbs)
	if !ok || !workbenchControlMatchesManifest(control, manifest) {
		return workbenchControl{}, false
	}
	return control, true
}

func readWorkbenchDraft(deckAbs string) (workbenchDraft, bool) {
	raw, err := readRegularFileWithMaxBytes(filepath.Join(deckAbs, "out", workbenchDraftName), maxDeckJSONBytes)
	if err != nil {
		return workbenchDraft{}, false
	}
	var draft workbenchDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		return workbenchDraft{}, false
	}
	return draft, true
}

func writeWorkbenchDraft(deckAbs string, input workbenchSaveInput, status string) (workbenchDraft, error) {
	draft := workbenchDraft{
		SchemaVersion: "slidex.workbenchDraft.v1",
		ToolName:      toolName,
		ToolVersion:   toolVersion,
		DeckID:        filepath.Base(deckAbs),
		Status:        status,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Input:         input,
	}
	if err := secureWriteJSON(filepath.Join(deckAbs, "out", workbenchDraftName), draft); err != nil {
		return draft, err
	}
	return draft, nil
}

func localPluginVersion() string {
	path := ""
	if cwd, err := os.Getwd(); err == nil {
		for current := cwd; ; current = filepath.Dir(current) {
			candidate := filepath.Join(current, "plugins", "slidex", ".codex-plugin", "plugin.json")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	if path == "" {
		path = filepath.Join("plugins", "slidex", ".codex-plugin", "plugin.json")
	}
	raw, err := readRegularFileWithMaxBytes(path, maxUpdateCandidateJSONBytes)
	if err != nil {
		return ""
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(manifest["version"]))
}

func recordWorkbenchBrowserEvidence(workspace, deckID, deck string, input workbenchBrowserEvidenceInput) (workbenchBrowserEvidence, error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, defaultDeckTemplatePath)
	if err != nil {
		return workbenchBrowserEvidence{}, err
	}
	manifest, ok := readWorkbenchManifest(deckAbs)
	if !ok {
		return workbenchBrowserEvidence{}, fmt.Errorf("workbench manifest is required before recording browser evidence: %s", filepath.ToSlash(filepath.Join(deckAbs, "out", workbenchManifestName)))
	}
	if err := validateWorkbenchBrowserEvidenceInput(input, manifest); err != nil {
		return workbenchBrowserEvidence{}, err
	}
	briefPath := filepath.Join(deckAbs, "brief.md")
	draftPath := filepath.Join(deckAbs, "out", workbenchDraftName)
	manifestPath := filepath.Join(deckAbs, "out", workbenchManifestName)
	evidencePath := filepath.Join(deckAbs, "out", workbenchBrowserEvidenceName)
	if strings.TrimSpace(manifest.InputSavedAt) == "" {
		return workbenchBrowserEvidence{}, errors.New("workbench manifest does not show saved input; save the workbench form before recording browser evidence")
	}
	for _, path := range []string{briefPath, draftPath, manifestPath} {
		if info, err := os.Stat(path); err != nil {
			return workbenchBrowserEvidence{}, fmt.Errorf("required saved workbench artifact is missing: %s: %w", filepath.ToSlash(path), err)
		} else if info.IsDir() {
			return workbenchBrowserEvidence{}, fmt.Errorf("required saved workbench artifact is a directory: %s", filepath.ToSlash(path))
		}
	}
	screenshot, err := copyWorkbenchBrowserScreenshot(deckAbs, input.ScreenshotPath)
	if err != nil {
		return workbenchBrowserEvidence{}, err
	}
	evidence := workbenchBrowserEvidence{
		SchemaVersion:       "slidex.workbenchBrowserEvidence.v1",
		ToolName:            toolName,
		ToolVersion:         toolVersion,
		CodexVersion:        firstNonEmpty(installedCodexVersion(), "unavailable"),
		PluginName:          "slidex",
		PluginVersion:       firstNonEmpty(localPluginVersion(), "unavailable"),
		DeckID:              manifest.DeckID,
		DeckDir:             manifest.DeckDir,
		Status:              "verified",
		RecordedAt:          time.Now().UTC().Format(time.RFC3339),
		Inspector:           strings.TrimSpace(input.Inspector),
		Surface:             strings.TrimSpace(input.Surface),
		Invocation:          strings.TrimSpace(input.Invocation),
		ThreadID:            strings.TrimSpace(input.ThreadID),
		URL:                 strings.TrimSpace(input.URL),
		SessionID:           manifest.SessionID,
		ServerBind:          manifest.ServerBind,
		WorkbenchVisible:    input.WorkbenchVisible,
		SavedInputVerified:  input.SavedInputVerified,
		TokenRedacted:       manifest.TokenRedacted,
		BrowserOpenStrategy: manifest.BrowserOpenStrategy,
		Notes:               strings.TrimSpace(input.Notes),
		ManifestPath:        filepath.ToSlash(manifestPath),
		BriefPath:           filepath.ToSlash(briefPath),
		DraftPath:           filepath.ToSlash(draftPath),
		EvidencePath:        filepath.ToSlash(evidencePath),
		BrowserScreenshot:   screenshot,
		VerifiedFiles: map[string]artifact{
			"brief":    artifactFromPath(briefPath),
			"draft":    artifactFromPath(draftPath),
			"manifest": artifactFromPath(manifestPath),
		},
	}
	if err := secureWriteJSON(evidencePath, evidence); err != nil {
		return workbenchBrowserEvidence{}, err
	}
	return evidence, nil
}

func copyWorkbenchBrowserScreenshot(deckAbs, sourcePath string) (*artifact, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return nil, nil
	}
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, err
	}
	if err := rejectSymlinkAncestors(sourceAbs); err != nil {
		return nil, fmt.Errorf("browser screenshot path contains a symlink: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(sourceAbs))
	switch ext {
	case ".png", ".jpg", ".jpeg":
	default:
		return nil, fmt.Errorf("browser screenshot must be .png, .jpg, or .jpeg: %s", filepath.ToSlash(sourceAbs))
	}
	f, info, err := openRegularFileForRead(sourceAbs)
	if err != nil {
		return nil, fmt.Errorf("browser screenshot is missing or unreadable: %s: %w", filepath.ToSlash(sourceAbs), err)
	}
	defer f.Close()
	if info.Size() > workbenchScreenshotMaxBytes {
		return nil, fmt.Errorf("browser screenshot exceeds %d bytes: %s", workbenchScreenshotMaxBytes, filepath.ToSlash(sourceAbs))
	}
	raw, err := io.ReadAll(io.LimitReader(f, workbenchScreenshotMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("browser screenshot is empty: %s", filepath.ToSlash(sourceAbs))
	}
	if len(raw) > workbenchScreenshotMaxBytes {
		return nil, fmt.Errorf("browser screenshot exceeds %d bytes: %s", workbenchScreenshotMaxBytes, filepath.ToSlash(sourceAbs))
	}
	if _, blank, _, err := inspectWorkbenchBrowserScreenshot(raw); err != nil {
		return nil, fmt.Errorf("browser screenshot must be a decodable PNG or JPEG image: %w", err)
	} else if blank {
		return nil, fmt.Errorf("browser screenshot appears blank: %s", filepath.ToSlash(sourceAbs))
	}
	target := filepath.Join(deckAbs, "out", workbenchBrowserScreenshot+ext)
	if err := secureWriteFile(target, raw, 0o600); err != nil {
		return nil, err
	}
	artifact := artifactFromPath(target)
	artifact.Path = filepath.ToSlash(target)
	return &artifact, nil
}

func inspectWorkbenchBrowserScreenshot(raw []byte) (dimension, bool, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return dimension{}, false, "", err
	}
	if format != "png" && format != "jpeg" {
		return dimension{}, false, format, fmt.Errorf("unsupported screenshot image format %q", format)
	}
	dim := dimension{Width: cfg.Width, Height: cfg.Height}
	if err := validateWorkbenchScreenshotDimensions(dim); err != nil {
		return dim, false, format, err
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return dimension{}, false, "", err
	}
	if format != "png" && format != "jpeg" {
		return dimension{}, false, format, fmt.Errorf("unsupported screenshot image format %q", format)
	}
	bounds := img.Bounds()
	dim = dimension{Width: bounds.Dx(), Height: bounds.Dy()}
	if dim.Width <= 0 || dim.Height <= 0 {
		return dim, true, format, errors.New("screenshot image has empty dimensions")
	}
	return dim, isBlank(img), format, nil
}

func validateWorkbenchScreenshotDimensions(dim dimension) error {
	if dim.Width <= 0 || dim.Height <= 0 {
		return errors.New("screenshot image has empty dimensions")
	}
	w := int64(dim.Width)
	h := int64(dim.Height)
	if w > workbenchScreenshotMaxPixels/h {
		return fmt.Errorf("screenshot image exceeds maximum pixel count: %dx%d > %d pixels", dim.Width, dim.Height, workbenchScreenshotMaxPixels)
	}
	if pixels := w * h; pixels > workbenchScreenshotMaxPixels {
		return fmt.Errorf("screenshot image exceeds maximum pixel count: %dx%d > %d pixels", dim.Width, dim.Height, workbenchScreenshotMaxPixels)
	}
	return nil
}

func verifyWorkbenchBrowserEvidence(workspace, deckID, deck string, requireScreenshot bool) (result workbenchBrowserEvidenceVerification, err error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, defaultDeckTemplatePath)
	if err != nil {
		return workbenchBrowserEvidenceVerification{}, err
	}
	manifestPath := filepath.Join(deckAbs, "out", workbenchManifestName)
	evidencePath := filepath.Join(deckAbs, "out", workbenchBrowserEvidenceName)
	verificationPath := filepath.Join(deckAbs, "out", workbenchBrowserVerifyName)
	result = workbenchBrowserEvidenceVerification{
		SchemaVersion:     "slidex.workbenchBrowserEvidenceVerification.v1",
		ToolName:          toolName,
		ToolVersion:       toolVersion,
		Status:            "fail",
		CheckedAt:         time.Now().UTC().Format(time.RFC3339),
		RequireScreenshot: requireScreenshot,
		DeckID:            filepath.Base(deckAbs),
		DeckDir:           filepath.ToSlash(deckAbs),
		EvidencePath:      filepath.ToSlash(evidencePath),
		VerificationPath:  filepath.ToSlash(verificationPath),
		ManifestPath:      filepath.ToSlash(manifestPath),
		Findings:          []string{},
		VerifiedFiles:     map[string]artifact{},
	}
	defer func() {
		if writeErr := secureWriteJSON(verificationPath, result); writeErr != nil && err == nil {
			err = writeErr
		}
	}()
	addFinding := func(format string, args ...any) {
		result.Findings = append(result.Findings, fmt.Sprintf(format, args...))
	}

	if err := rejectSymlinkAncestors(evidencePath); err != nil {
		addFinding("browser evidence path contains a symlink: %v", err)
		return result, nil
	}
	if err := rejectSecureWriteTarget(evidencePath); err != nil {
		addFinding("browser evidence path is not a regular secure target: %v", err)
		return result, nil
	}
	raw, err := readRegularFileWithMaxBytes(evidencePath, maxDeckJSONBytes)
	if err != nil {
		addFinding("browser evidence is missing or unreadable: %s: %v", filepath.ToSlash(evidencePath), err)
		return result, nil
	}
	var evidence workbenchBrowserEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		addFinding("browser evidence is not valid JSON: %v", err)
		return result, nil
	}
	manifest, ok := readWorkbenchManifest(deckAbs)
	if !ok {
		addFinding("workbench manifest is missing or unreadable: %s", filepath.ToSlash(manifestPath))
	}

	if evidence.SchemaVersion != "slidex.workbenchBrowserEvidence.v1" {
		addFinding("browser evidence schemaVersion is %q", evidence.SchemaVersion)
	}
	if evidence.Status != "verified" {
		addFinding("browser evidence status is %q", evidence.Status)
	}
	if strings.TrimSpace(evidence.CodexVersion) == "" || evidence.CodexVersion == "unavailable" {
		addFinding("browser evidence codexVersion is %q", evidence.CodexVersion)
	}
	if evidence.PluginName != "slidex" {
		addFinding("browser evidence pluginName is %q", evidence.PluginName)
	}
	if strings.TrimSpace(evidence.PluginVersion) == "" || evidence.PluginVersion == "unavailable" {
		addFinding("browser evidence pluginVersion is %q", evidence.PluginVersion)
	}
	if !strings.Contains(strings.ToLower(evidence.Invocation), "slidex") {
		addFinding("browser evidence invocation must name slidex, got %q", evidence.Invocation)
	}
	if evidence.DeckID != filepath.Base(deckAbs) {
		addFinding("browser evidence deckId is %q, want %q", evidence.DeckID, filepath.Base(deckAbs))
	}
	if evidence.DeckDir != filepath.ToSlash(deckAbs) {
		addFinding("browser evidence deckDir is %q, want %q", evidence.DeckDir, filepath.ToSlash(deckAbs))
	}
	if evidence.EvidencePath != filepath.ToSlash(evidencePath) {
		addFinding("browser evidence path is %q, want %q", evidence.EvidencePath, filepath.ToSlash(evidencePath))
	}
	if evidence.ManifestPath != filepath.ToSlash(manifestPath) {
		addFinding("browser evidence manifestPath is %q, want %q", evidence.ManifestPath, filepath.ToSlash(manifestPath))
	}
	if evidence.Surface != "codex_app_in_app_browser" && evidence.Surface != "codex_browser_plugin" {
		addFinding("browser evidence surface is %q", evidence.Surface)
	}
	if !evidence.WorkbenchVisible {
		addFinding("browser evidence does not confirm visible workbench")
	}
	if !evidence.SavedInputVerified {
		addFinding("browser evidence does not confirm saved input verification")
	}
	if !evidence.TokenRedacted {
		addFinding("browser evidence does not confirm token redaction")
	}
	if requireScreenshot && evidence.BrowserScreenshot == nil {
		addFinding("browser evidence must include a browser screenshot artifact when --require-screenshot is set")
	}
	if evidence.ServerBind != "127.0.0.1" {
		addFinding("browser evidence serverBind is %q", evidence.ServerBind)
	}
	parsed, err := url.Parse(evidence.URL)
	if err != nil {
		addFinding("browser evidence URL is invalid: %v", err)
	} else if parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		addFinding("browser evidence URL must be an http://127.0.0.1 loopback URL: %s", evidence.URL)
	}

	if ok {
		if strings.TrimSpace(manifest.InputSavedAt) == "" {
			addFinding("workbench manifest does not record saved input")
		}
		if manifest.URL != evidence.URL {
			addFinding("browser evidence URL is %q, current manifest URL is %q", evidence.URL, manifest.URL)
		}
		if manifest.SessionID != evidence.SessionID {
			addFinding("browser evidence sessionId is %q, current manifest sessionId is %q", evidence.SessionID, manifest.SessionID)
		}
		if manifest.ServerBind != "127.0.0.1" || manifest.Host != "127.0.0.1" {
			addFinding("workbench manifest must bind and advertise 127.0.0.1")
		}
		if !manifest.TokenRedacted || manifest.TokenSHA256 == "" {
			addFinding("workbench manifest must redact the write token")
		}
	}

	expectedFiles := map[string]string{
		"brief":    filepath.Join(deckAbs, "brief.md"),
		"draft":    filepath.Join(deckAbs, "out", workbenchDraftName),
		"manifest": manifestPath,
	}
	expectedEvidencePaths := map[string]string{
		"brief":    evidence.BriefPath,
		"draft":    evidence.DraftPath,
		"manifest": evidence.ManifestPath,
	}
	for name, path := range expectedFiles {
		if err := rejectSymlinkAncestors(path); err != nil {
			addFinding("%s path contains a symlink: %v", name, err)
			continue
		}
		if err := rejectSecureWriteTarget(path); err != nil {
			addFinding("%s path is not a regular secure target: %v", name, err)
			continue
		}
		actual := artifactFromPath(path)
		result.VerifiedFiles[name] = actual
		if actual.SHA256 == "" || actual.Size <= 0 {
			addFinding("%s artifact is missing or empty: %s", name, filepath.ToSlash(path))
			continue
		}
		if expectedEvidencePaths[name] != filepath.ToSlash(path) {
			addFinding("browser evidence %s path is %q, want %q", name, expectedEvidencePaths[name], filepath.ToSlash(path))
		}
		recorded, exists := evidence.VerifiedFiles[name]
		if !exists {
			addFinding("browser evidence is missing verifiedFiles.%s", name)
			continue
		}
		if recorded.Path != actual.Path || recorded.SHA256 != actual.SHA256 || recorded.Size != actual.Size {
			addFinding("browser evidence verifiedFiles.%s is stale", name)
		}
	}
	if evidence.BrowserScreenshot != nil {
		screenshotPath := filepath.FromSlash(evidence.BrowserScreenshot.Path)
		outDir := filepath.Join(deckAbs, "out")
		if !filepath.IsAbs(screenshotPath) {
			screenshotPath = filepath.Join(outDir, screenshotPath)
		}
		if !pathWithin(outDir, screenshotPath) {
			addFinding("browser screenshot evidence must stay under deck out/: %s", filepath.ToSlash(screenshotPath))
		} else if err := rejectSymlinkAncestors(screenshotPath); err != nil {
			addFinding("browser screenshot path contains a symlink: %v", err)
		} else if err := rejectSecureWriteTarget(screenshotPath); err != nil {
			addFinding("browser screenshot path is not a regular secure target: %v", err)
		} else {
			actual := artifactFromPath(screenshotPath)
			result.VerifiedFiles["browserScreenshot"] = actual
			if actual.SHA256 == "" || actual.Size <= 0 {
				addFinding("browser screenshot artifact is missing or empty: %s", filepath.ToSlash(screenshotPath))
			} else {
				raw, err := readRegularFileWithMaxBytes(screenshotPath, workbenchScreenshotMaxBytes)
				if err != nil {
					addFinding("browser screenshot is missing or unreadable: %s: %v", filepath.ToSlash(screenshotPath), err)
				} else if _, blank, _, err := inspectWorkbenchBrowserScreenshot(raw); err != nil {
					addFinding("browser screenshot is not a decodable PNG or JPEG image: %v", err)
				} else if blank {
					addFinding("browser screenshot appears blank")
				}
				if evidence.BrowserScreenshot.Path != actual.Path ||
					evidence.BrowserScreenshot.SHA256 != actual.SHA256 ||
					evidence.BrowserScreenshot.Size != actual.Size {
					addFinding("browser screenshot evidence is stale")
				}
			}
		}
	}

	if len(result.Findings) == 0 {
		result.Status = "pass"
	}
	return result, nil
}

func validateWorkbenchBrowserEvidenceInput(input workbenchBrowserEvidenceInput, manifest workbenchManifest) error {
	if strings.TrimSpace(input.Inspector) == "" {
		return errors.New("--inspector is required")
	}
	surface := strings.TrimSpace(input.Surface)
	if surface != "codex_app_in_app_browser" && surface != "codex_browser_plugin" {
		return fmt.Errorf("--surface must be codex_app_in_app_browser or codex_browser_plugin, got %q", surface)
	}
	invocation := strings.TrimSpace(input.Invocation)
	if invocation == "" {
		return errors.New("--invocation is required and must describe the @slidex or slidex-start plugin call")
	}
	if !strings.Contains(strings.ToLower(invocation), "slidex") {
		return fmt.Errorf("--invocation must name slidex, got %q", invocation)
	}
	observedURL := strings.TrimSpace(input.URL)
	if observedURL == "" {
		return errors.New("--url is required")
	}
	if observedURL != manifest.URL {
		return fmt.Errorf("observed URL does not match current workbench manifest URL: got %s want %s", observedURL, manifest.URL)
	}
	parsed, err := url.Parse(observedURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		return fmt.Errorf("workbench browser evidence URL must be an http://127.0.0.1 loopback URL: %s", observedURL)
	}
	if !input.WorkbenchVisible {
		return errors.New("--workbench-visible is required after inspecting the Codex App browser surface")
	}
	if !input.SavedInputVerified {
		return errors.New("--saved-input-verified is required after checking saved deck-local artifacts")
	}
	if manifest.ServerBind != "127.0.0.1" || manifest.Host != "127.0.0.1" {
		return errors.New("workbench manifest must bind and advertise 127.0.0.1 before browser evidence can be recorded")
	}
	if !manifest.TokenRedacted || manifest.TokenSHA256 == "" {
		return errors.New("workbench manifest must redact the write token before browser evidence can be recorded")
	}
	if manifest.SessionID == "" {
		return errors.New("workbench manifest is missing session id")
	}
	return nil
}

func publicWorkbenchStatus(manifest workbenchManifest) map[string]any {
	return publicWorkbenchStatusWithBrowserOpenMode(manifest, workbenchBrowserOpenStructured)
}

func publicWorkbenchStatusWithBrowserOpen(manifest workbenchManifest, browserOpen bool) map[string]any {
	if browserOpen {
		return publicWorkbenchStatusWithBrowserOpenMode(manifest, workbenchBrowserOpenStructured)
	}
	return publicWorkbenchStatusWithBrowserOpenMode(manifest, workbenchBrowserOpenManual)
}

func publicWorkbenchStatusWithBrowserOpenMode(manifest workbenchManifest, mode workbenchBrowserOpenMode) map[string]any {
	update := updateStatusSnapshot()
	status := map[string]any{
		"status":              manifest.Status,
		"deckId":              manifest.DeckID,
		"deckDir":             manifest.DeckDir,
		"outDir":              manifest.OutDir,
		"url":                 manifest.URL,
		"sessionId":           manifest.SessionID,
		"tokenRedacted":       manifest.TokenRedacted && manifest.TokenSHA256 != "",
		"pid":                 manifest.PID,
		"host":                manifest.Host,
		"port":                manifest.Port,
		"serverBind":          manifest.ServerBind,
		"browserOpenStrategy": manifest.BrowserOpenStrategy,
		"manifest":            filepath.ToSlash(filepath.Join(manifest.OutDir, workbenchManifestName)),
		"wizardCompletedAt":   manifest.WizardCompletedAt,
		"generationStatus":    manifest.GenerationStatus,
		"generationStartedAt": manifest.GenerationStartedAt,
		"generationEndedAt":   manifest.GenerationEndedAt,
		"generationPid":       manifest.GenerationPID,
		"generationExitCode":  manifest.GenerationExitCode,
		"generationCommand":   manifest.GenerationCommand,
		"generationLog":       manifest.GenerationLogPath,
		"update":              update,
		"statusBanners":       update["banners"],
	}
	status["browserOpenMode"] = string(mode)
	switch mode {
	case workbenchBrowserOpenStructured:
		status["browserOpen"] = workbenchBrowserOpenIntent(manifest)
	case workbenchBrowserOpenAgent:
		status["browserOpenSuppressed"] = true
		status["manualOpenURL"] = manifest.URL
		status["agentBrowserInstruction"] = workbenchAgentBrowserInstruction(manifest)
	default:
		status["browserOpenSuppressed"] = true
		status["manualOpenURL"] = manifest.URL
	}
	evidencePath := filepath.Join(manifest.OutDir, workbenchBrowserEvidenceName)
	if _, err := os.Stat(evidencePath); err == nil {
		status["browserEvidence"] = filepath.ToSlash(evidencePath)
	}
	verificationPath := filepath.Join(manifest.OutDir, workbenchBrowserVerifyName)
	if _, err := os.Stat(verificationPath); err == nil {
		status["browserEvidenceVerification"] = filepath.ToSlash(verificationPath)
	}
	return status
}

func workbenchOpenInstruction(manifest workbenchManifest, mode workbenchBrowserOpenMode) string {
	if manifest.URL == "" {
		return "Open the slidex workbench in the Codex App in-app browser after workbench.url is available."
	}
	if mode == workbenchBrowserOpenAgent {
		return workbenchAgentBrowserInstruction(manifest)
	}
	if mode == workbenchBrowserOpenManual {
		return "Workbench is running at " + manifest.URL + ". Browser navigation intent is suppressed; open the URL manually only when you need the local Workbench."
	}
	return "Immediately open " + manifest.URL + " in the Codex App in-app browser using the Browser plugin or @Browser when available; if Browser use is unavailable, click the URL or navigate manually."
}

func workbenchAgentBrowserInstruction(manifest workbenchManifest) string {
	if manifest.URL == "" {
		return "Use @Browser to open the slidex workbench URL after workbench.url is available. Do not use Chrome or an external browser for this startup."
	}
	return "Use @Browser to open " + manifest.URL + " in the Codex App in-app browser. Do not use Chrome or an external browser for this startup."
}

func workbenchBrowserOpenIntent(manifest workbenchManifest) map[string]any {
	return map[string]any{
		"target":                    "codex_app_in_app_browser",
		"preferredAction":           "browser_plugin_navigation",
		"toolHint":                  "@Browser",
		"url":                       manifest.URL,
		"fallbackAction":            "url_click_or_manual_navigation",
		"requiresLoopback":          true,
		"serverBind":                manifest.ServerBind,
		"proprietaryCanvasAPI":      "not_used",
		"directClientRequestAPI":    "not_available_in_codex_app_server_0.138.0",
		"actualBrowserEvidenceGate": "slidex workbench evidence followed by slidex workbench verify-evidence",
	}
}

func chooseLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("loopback listener did not return TCP address")
	}
	return addr.Port, nil
}

func workbenchListenAddr(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}

func waitForWorkbenchReady(manifest workbenchManifest, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if isWorkbenchReady(manifest) {
			return nil
		}
		lastErr = errors.New("workbench not ready")
		time.Sleep(80 * time.Millisecond)
	}
	return lastErr
}

func isWorkbenchReady(manifest workbenchManifest) bool {
	if manifest.Host != "127.0.0.1" || manifest.Port <= 0 {
		return false
	}
	control, ok := trustedWorkbenchControl(manifest)
	if !ok || control.ReadinessKey == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/readyz", manifest.Port), nil)
	if err != nil {
		return false
	}
	req.Header.Set(workbenchReadinessHeader, control.ReadinessKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16*1024)).Decode(&payload); err != nil {
		return false
	}
	pid, pidOK := numberAsInt(payload["pid"])
	return fmt.Sprint(payload["sessionId"]) == manifest.SessionID &&
		fmt.Sprint(payload["deckDir"]) == manifest.DeckDir &&
		pidOK &&
		pid == manifest.PID
}

func randomURLToken(bytesLen int) (string, error) {
	raw := make([]byte, bytesLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validWorkbenchToken(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	return hmac.Equal([]byte(got), []byte(want))
}

func sameOriginOrNoOrigin(r *http.Request, expectedURL string) bool {
	return sameOriginCheck(r, expectedURL, true)
}

func sameOriginRequired(r *http.Request, expectedURL string) bool {
	return sameOriginCheck(r, expectedURL, false)
}

func sameOriginCheck(r *http.Request, expectedURL string, allowMissing bool) bool {
	expectedOrigin, ok := originFromURL(expectedURL)
	if !ok {
		return false
	}
	seen := false
	if origin := r.Header.Get("Origin"); origin != "" {
		seen = true
		if origin != expectedOrigin {
			return false
		}
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		seen = true
		parsed, err := url.Parse(referer)
		if err != nil {
			return false
		}
		if parsed.Scheme+"://"+parsed.Host != expectedOrigin {
			return false
		}
	}
	return seen || allowMissing
}

func originFromURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

func normalizeWorkbenchInput(input workbenchSaveInput) workbenchSaveInput {
	input.InitialRequest = strings.TrimSpace(input.InitialRequest)
	input.Title = strings.TrimSpace(input.Title)
	input.Audience = strings.TrimSpace(input.Audience)
	input.DecisionGoal = strings.TrimSpace(input.DecisionGoal)
	input.SourceNotes = strings.TrimSpace(input.SourceNotes)
	input.KeyMessages = strings.TrimSpace(input.KeyMessages)
	input.RequiredClaims = strings.TrimSpace(input.RequiredClaims)
	input.Constraints = strings.TrimSpace(input.Constraints)
	input.OutputExpectations = strings.TrimSpace(input.OutputExpectations)
	return input
}

func hasAnyWorkbenchInput(input workbenchSaveInput) bool {
	return input.InitialRequest != "" || input.Title != "" || input.Audience != "" || input.DecisionGoal != "" || input.SourceNotes != "" || input.KeyMessages != "" || input.RequiredClaims != "" || input.Constraints != "" || input.OutputExpectations != ""
}

func writeWorkbenchBrief(deckAbs string, input workbenchSaveInput) error {
	if input.Title == "" || input.Audience == "" || input.DecisionGoal == "" {
		return errors.New("title, audience, and decisionGoal are required")
	}
	var b strings.Builder
	b.WriteString("# " + input.Title + "\n\n")
	if input.InitialRequest != "" {
		b.WriteString("## Original Plugin Request\n\n" + input.InitialRequest + "\n\n")
	}
	b.WriteString("## Audience\n\n" + input.Audience + "\n\n")
	b.WriteString("## Decision Goal\n\n" + input.DecisionGoal + "\n\n")
	if input.SourceNotes != "" {
		b.WriteString("## Source-Material Notes\n\n" + input.SourceNotes + "\n\n")
	}
	if input.KeyMessages != "" {
		b.WriteString("## Key Messages\n\n" + input.KeyMessages + "\n\n")
	}
	if input.RequiredClaims != "" {
		b.WriteString("## Claims Requiring Evidence Review\n\n" + input.RequiredClaims + "\n\n")
	}
	if input.Constraints != "" {
		b.WriteString("## Constraints And Exclusions\n\n" + input.Constraints + "\n\n")
	}
	if input.OutputExpectations != "" {
		b.WriteString("## Output Expectations\n\n" + input.OutputExpectations + "\n\n")
	}
	b.WriteString("## Wizard Completion\n\n")
	b.WriteString("- The Workbench marked this intake as ready for generation after required title, audience, decision goal, source notes, key messages, and output expectations were provided.\n")
	b.WriteString("- If any user-provided claim lacks evidence, label it as an assumption or remove it during generation.\n\n")
	b.WriteString("## Evidence Policy\n\n")
	b.WriteString("- Material claims must be sourced, user-confirmed, or labeled as assumptions.\n")
	b.WriteString("- Unsupported metrics, outcomes, certifications, security claims, and guarantees must be removed or rewritten.\n")
	b.WriteString("- This brief was initialized from the slidex Codex Plugin workbench.\n")
	return secureWriteFile(filepath.Join(deckAbs, "brief.md"), []byte(b.String()), 0o600)
}

func writeJSONResponse(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
