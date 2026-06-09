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
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	workbenchManifestName        = "workbench_manifest.json"
	workbenchDraftName           = "workbench_draft.json"
	workbenchBrowserEvidenceName = "workbench_browser_evidence.json"
	workbenchSaveSmokeName       = "workbench_save_smoke.json"
	workbenchBrowserScreenshot   = "workbench_browser_screenshot"
	workbenchLockName            = "workbench.lock"
	workbenchScreenshotMaxBytes  = 20 * 1024 * 1024
)

var deckIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

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
	DraftSavedAt        string            `json:"draftSavedAt,omitempty"`
	DraftPath           string            `json:"draftPath,omitempty"`
	SavedFieldLengths   map[string]int    `json:"savedFieldLengths,omitempty"`
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
	EvidencePath                    string              `json:"evidencePath"`
	BrowserOpenStrategy             string              `json:"browserOpenStrategy"`
	IsActualCodexAppBrowserEvidence bool                `json:"isActualCodexAppBrowserEvidence"`
	Input                           workbenchSaveInput  `json:"input"`
	VerifiedFiles                   map[string]artifact `json:"verifiedFiles"`
	Checks                          map[string]any      `json:"checks"`
	Findings                        []string            `json:"findings,omitempty"`
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
	Title              string `json:"title"`
	Audience           string `json:"audience"`
	DecisionGoal       string `json:"decisionGoal"`
	SourceNotes        string `json:"sourceNotes"`
	OutputExpectations string `json:"outputExpectations"`
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
	fromTemplate := fs.String("from-template", "decks/_template", "template deck directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result, manifest, startedNew, err := startWorkbench(*workspace, *deckID, *deck, *fromTemplate)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{
		"toolName":                toolName,
		"status":                  manifest.Status,
		"deck":                    result,
		"workbench":               publicWorkbenchStatus(manifest),
		"openInstruction":         "Open the returned workbench.url in the Codex App in-app browser or ask @Browser to navigate there.",
		"browserOpenStrategy":     manifest.BrowserOpenStrategy,
		"proprietaryCanvasAPI":    "not_used",
		"tokenHandling":           "write token is redacted from CLI output and manifest",
		"startedNew":              startedNew,
		"reusedExisting":          !startedNew,
		"workbenchManifestPath":   filepath.ToSlash(filepath.Join(manifest.OutDir, workbenchManifestName)),
		"supportedURLMechanism":   "Codex in-app browser can open local URLs by URL click, manual navigation, or Browser plugin use.",
		"unsupportedURLMechanism": "No Codex 0.138.0 App Server client request method was found for plugin-owned automatic browser opening.",
	})
}

func runWorkbenchServe(args []string) error {
	fs := flag.NewFlagSet("workbench serve", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	workspace := fs.String("workspace", ".", "workspace root")
	sessionID := fs.String("session", "", "session id")
	token := fs.String("token", "", "write token")
	tokenEnv := fs.String("token-env", "", "environment variable containing the write token")
	port := fs.Int("port", 0, "loopback port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token != "" {
		return exitCodeError(2, "--token is not supported; use --token-env to keep the workbench token out of process arguments")
	}
	if *token == "" && *tokenEnv != "" {
		*token = os.Getenv(*tokenEnv)
	}
	if *deck == "" || *sessionID == "" || *token == "" || *port <= 0 {
		return exitCodeError(2, "usage: slidex workbench serve --deck DIR --session ID --token-env ENV --port PORT")
	}
	deckAbs, err := resolveDeckDir(*workspace, "", *deck, false, "decks/_template")
	if err != nil {
		return err
	}
	return serveWorkbench(deckAbs, *workspace, *sessionID, *token, *port)
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
	fromTemplate := fs.String("from-template", "decks/_template", "template deck directory")
	title := fs.String("title", "Workbench save smoke", "deck title to submit")
	audience := fs.String("audience", "Codex App verification reviewer", "audience to submit")
	decisionGoal := fs.String("decision-goal", "Verify the slidex workbench can persist initial deck creation input.", "decision goal to submit")
	sourceNotes := fs.String("source-notes", "Generated by slidex workbench save-smoke. This is not Codex App GUI/browser evidence.", "source-material notes to submit")
	outputExpectations := fs.String("output-expectations", "Deck-local brief.md, out/workbench_draft.json, and out/workbench_manifest.json are current after HTTP save.", "output expectations to submit")
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
	})
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
	workspace, _ := args["workspace"].(string)
	deckID, _ := args["deckId"].(string)
	if deckID == "" {
		deckID, _ = args["deck_id"].(string)
	}
	result, err := bootstrapDeckWorkspace(workspace, deckID, "decks/_template", true)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func callMCPDeckInspect(args map[string]any) (any, error) {
	workspace, _ := args["workspace"].(string)
	deckID, _ := args["deckId"].(string)
	deck, _ := args["deck"].(string)
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, "decks/_template")
	if err != nil {
		return nil, err
	}
	return inspectDeck(deckAbs)
}

func callMCPWorkbenchStart(args map[string]any) (any, error) {
	workspace, _ := args["workspace"].(string)
	deckID, _ := args["deckId"].(string)
	deck, _ := args["deck"].(string)
	result, manifest, startedNew, err := startWorkbench(workspace, deckID, deck, "decks/_template")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"deck":                 result,
		"workbench":            publicWorkbenchStatus(manifest),
		"startedNew":           startedNew,
		"reusedExisting":       !startedNew,
		"openInstruction":      "Open workbench.url in the Codex App in-app browser, or ask @Browser to navigate to it.",
		"proprietaryCanvasAPI": "not_used",
	}, nil
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

func smokeSaveWorkbench(workspace, deckID, deck, fromTemplate string, input workbenchSaveInput) (result workbenchSaveSmokeResult, err error) {
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
	evidencePath := filepath.Join(deckAbs, "out", workbenchSaveSmokeName)
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
	result.RawTokenAbsentFromArtifacts = rawTokenAbsentFromFiles(token, []string{briefPath, draftPath, manifestPath})
	result.Findings = workbenchSaveSmokeFindings(result, updated)
	result.Status = workbenchSaveSmokeStatus(result)
	return result, nil
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
		raw, err := os.ReadFile(path)
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
		findings = append(findings, "raw token appeared in persisted artifacts or artifact read failed")
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
	if result.StartStatus == "running" &&
		result.DraftStatus == "draft_saved" &&
		result.SaveStatus == "saved" &&
		stopOK &&
		result.ServerBind == "127.0.0.1" &&
		result.TokenRedacted &&
		result.HTMLBootstrapTokenFound &&
		result.RawTokenAbsentFromArtifacts &&
		!result.IsActualCodexAppBrowserEvidence &&
		len(result.Findings) == 0 &&
		len(result.VerifiedFiles) == 3 {
		return "pass"
	}
	return "fail"
}

func startWorkbench(workspace, deckID, deck, fromTemplate string) (deckBootstrapResult, workbenchManifest, bool, error) {
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
		if isWorkbenchReady(existing) {
			existing.Status = "running"
			return result, existing, false, nil
		}
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
	exe, err := os.Executable()
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	logPath := filepath.Join(outDir, "workbench_server.log")
	logFile, err := openSecureAppendFile(logPath, 0o600)
	if err != nil {
		return result, workbenchManifest{}, false, err
	}
	defer logFile.Close()
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		port, err := chooseLoopbackPort()
		if err != nil {
			return result, workbenchManifest{}, false, err
		}
		cmd := exec.Command(exe, "workbench", "serve", "--workspace", workspaceRoot(workspace), "--deck", deckAbs, "--session", sessionID, "--token-env", "SLIDEX_WORKBENCH_TOKEN", "--port", strconv.Itoa(port))
		cmd.Env = append(os.Environ(), "SLIDEX_WORKBENCH_TOKEN="+token)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}
		manifest := newWorkbenchManifest(deckAbs, workspaceRoot(workspace), sessionID, token, port, cmd.Process.Pid, "starting")
		if err := writeWorkbenchManifest(deckAbs, manifest); err != nil {
			stopWorkbenchProcess(manifest)
			return result, manifest, true, err
		}
		if err := waitForWorkbenchReady(manifest, 3*time.Second); err != nil {
			lastErr = err
			stopWorkbenchProcess(manifest)
			manifest.Status = "stale"
			manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = writeWorkbenchManifest(deckAbs, manifest)
			continue
		}
		manifest.Status = "running"
		manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeWorkbenchManifest(deckAbs, manifest); err != nil {
			return result, manifest, true, err
		}
		return result, manifest, true, nil
	}
	return result, workbenchManifest{}, false, fmt.Errorf("workbench did not become ready after port retries: %w", lastErr)
}

func serveWorkbench(deckAbs, workspace, sessionID, token string, port int) error {
	manifest := newWorkbenchManifest(deckAbs, workspaceRoot(workspace), sessionID, token, port, os.Getpid(), "running")
	if err := writeWorkbenchManifest(deckAbs, manifest); err != nil {
		return err
	}
	mux := http.NewServeMux()
	server := &workbenchHTTPServer{deckAbs: deckAbs, sessionID: sessionID, token: token, manifest: manifest}
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/readyz", server.handleReady)
	mux.HandleFunc("/workbench/"+sessionID, server.handleWorkbench)
	mux.HandleFunc("/workbench/"+sessionID+"/api/session", server.handleSession)
	mux.HandleFunc("/workbench/"+sessionID+"/api/draft", server.handleDraft)
	mux.HandleFunc("/workbench/"+sessionID+"/api/save", server.handleSave)
	addr := "127.0.0.1:" + strconv.Itoa(port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return httpServer.ListenAndServe()
}

type workbenchHTTPServer struct {
	deckAbs   string
	sessionID string
	token     string
	manifest  workbenchManifest
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
	_ = writeJSONResponse(w, map[string]any{
		"status":    "ready",
		"sessionId": s.sessionID,
		"deckDir":   s.manifest.DeckDir,
		"pid":       os.Getpid(),
	})
}

func (s *workbenchHTTPServer) handleWorkbench(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, s.workbenchHTML())
}

func (s *workbenchHTTPServer) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = writeJSONResponse(w, publicWorkbenchStatus(s.manifest))
}

func (s *workbenchHTTPServer) handleDraft(w http.ResponseWriter, r *http.Request) {
	if !validWorkbenchToken(r.Header.Get("X-Slidex-Workbench-Token"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !sameOriginOrNoOrigin(r, s.manifest.URL) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		if draft, ok := readWorkbenchDraft(s.deckAbs); ok {
			_ = writeJSONResponse(w, map[string]any{"status": "ok", "draft": draft})
			return
		}
		_ = writeJSONResponse(w, map[string]any{"status": "empty"})
	case http.MethodPost:
		if !sameOriginRequired(r, s.manifest.URL) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		defer r.Body.Close()
		var input workbenchSaveInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
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
		manifest := s.manifest
		manifest.Status = "draft"
		manifest.DraftSavedAt = draft.UpdatedAt
		manifest.DraftPath = filepath.ToSlash(filepath.Join(s.deckAbs, "out", workbenchDraftName))
		manifest.UpdatedAt = draft.UpdatedAt
		if err := writeWorkbenchManifest(s.deckAbs, manifest); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.manifest = manifest
		_ = writeJSONResponse(w, map[string]any{"status": "draft_saved", "draft": draft, "manifest": publicWorkbenchStatus(manifest)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *workbenchHTTPServer) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequired(r, s.manifest.URL) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if !validWorkbenchToken(r.Header.Get("X-Slidex-Workbench-Token"), s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()
	var input workbenchSaveInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	input = normalizeWorkbenchInput(input)
	if err := writeWorkbenchBrief(s.deckAbs, input); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	draft, err := writeWorkbenchDraft(s.deckAbs, input, "saved")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	manifest := s.manifest
	now := time.Now().UTC().Format(time.RFC3339)
	manifest.Status = "saved"
	manifest.InputSavedAt = now
	manifest.DraftSavedAt = draft.UpdatedAt
	manifest.DraftPath = filepath.ToSlash(filepath.Join(s.deckAbs, "out", workbenchDraftName))
	manifest.UpdatedAt = now
	manifest.BriefPath = filepath.ToSlash(filepath.Join(s.deckAbs, "brief.md"))
	manifest.SavedFieldLengths = map[string]int{
		"title":              len(input.Title),
		"audience":           len(input.Audience),
		"decisionGoal":       len(input.DecisionGoal),
		"sourceNotes":        len(input.SourceNotes),
		"outputExpectations": len(input.OutputExpectations),
	}
	if err := writeWorkbenchManifest(s.deckAbs, manifest); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.manifest = manifest
	_ = writeJSONResponse(w, map[string]any{"status": "saved", "manifest": publicWorkbenchStatus(manifest)})
}

func (s *workbenchHTTPServer) workbenchHTML() string {
	bootstrap := map[string]any{
		"deckId":    s.manifest.DeckID,
		"deckDir":   s.manifest.DeckDir,
		"sessionId": s.sessionID,
		"apiBase":   "/workbench/" + s.sessionID + "/api",
		"token":     s.token,
	}
	raw, _ := json.Marshal(bootstrap)
	title := html.EscapeString(s.manifest.DeckID)
	return `<!doctype html>
<html lang="ko">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>slidex workbench - ` + title + `</title>
  <style>
    :root { color-scheme: light; --ink:#182026; --muted:#53606b; --line:#d7dde2; --soft:#f4f7f9; --accent:#0f766e; --accent-strong:#0b5f59; --paper:#ffffff; --warn:#8a5a00; }
    * { box-sizing: border-box; }
    body { margin:0; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color:var(--ink); background:var(--soft); }
    main { max-width: 1120px; margin: 0 auto; padding: 28px; }
    header { display:flex; align-items:flex-start; justify-content:space-between; gap:20px; margin-bottom:22px; }
    h1 { margin:0; font-size:24px; line-height:1.2; letter-spacing:0; }
    .meta { color:var(--muted); font-size:13px; line-height:1.5; overflow-wrap:anywhere; }
    form { display:grid; grid-template-columns: minmax(0,1fr) minmax(0,1fr); gap:16px; }
    label { display:grid; gap:7px; font-size:13px; font-weight:650; }
    input, textarea { width:100%; min-width:0; border:1px solid var(--line); border-radius:6px; padding:11px 12px; font:inherit; background:var(--paper); color:var(--ink); }
    textarea { min-height:132px; resize:vertical; line-height:1.45; }
    .wide { grid-column:1 / -1; }
    .actions { grid-column:1 / -1; display:flex; align-items:center; gap:12px; margin-top:4px; }
    button { border:0; border-radius:6px; padding:10px 14px; min-height:40px; background:var(--accent); color:white; font-weight:700; cursor:pointer; }
    button:hover { background:var(--accent-strong); }
    button:disabled { opacity:.58; cursor:not-allowed; }
    output { font-size:13px; color:var(--muted); }
    output.warn { color:var(--warn); }
    .paths { margin-top:18px; border-top:1px solid var(--line); padding-top:14px; }
    .paths h2 { margin:0 0 10px; font-size:14px; line-height:1.3; letter-spacing:0; }
    .paths dl { margin:0; display:grid; grid-template-columns: minmax(90px, max-content) minmax(0, 1fr); gap:8px 14px; font-size:13px; line-height:1.45; }
    .paths dt { color:var(--muted); font-weight:650; }
    .paths dd { margin:0; overflow-wrap:anywhere; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
    .notice { margin-top:18px; border-top:1px solid var(--line); padding-top:14px; color:var(--muted); font-size:13px; line-height:1.5; }
    @media (max-width: 760px) { main { padding:20px; } header { display:block; } form { grid-template-columns:1fr; } }
  </style>
</head>
<body>
  <main>
    <header>
      <div>
        <h1>slidex workbench</h1>
        <div class="meta">Deck: <strong>` + title + `</strong></div>
      </div>
      <div class="meta">` + html.EscapeString(s.manifest.DeckDir) + `</div>
    </header>
    <form id="deck-form">
      <label>Deck title<input name="title" autocomplete="off" required></label>
      <label>Audience<input name="audience" autocomplete="off" required></label>
      <label class="wide">Decision goal<input name="decisionGoal" autocomplete="off" required></label>
      <label>Source-material notes<textarea name="sourceNotes" spellcheck="true"></textarea></label>
      <label>Output expectations<textarea name="outputExpectations" spellcheck="true"></textarea></label>
      <div class="actions"><button type="submit">Save initial brief</button><output id="status"></output></div>
    </form>
    <section class="paths" aria-label="Deck files">
      <h2>Deck files</h2>
      <dl>` + workbenchFilePathHTML(s.manifest) + `</dl>
    </section>
    <div class="notice">Later strategy, build, render, QA, and package stages remain separate slidex workflow steps.</div>
  </main>
  <script>
    const boot = ` + string(raw) + `;
    const form = document.getElementById("deck-form");
    const status = document.getElementById("status");
    let draftTimer = null;
    let filling = false;
    function formData() {
      return Object.fromEntries(new FormData(form).entries());
    }
    function setStatus(text, warn = false) {
      status.value = text;
      status.classList.toggle("warn", warn);
    }
    async function loadDraft() {
      const response = await fetch(boot.apiBase + "/draft", {headers: {"X-Slidex-Workbench-Token": boot.token}});
      if (!response.ok) return;
      const payload = await response.json();
      if (!payload.draft || !payload.draft.input) return;
      filling = true;
      for (const [key, value] of Object.entries(payload.draft.input)) {
        const field = form.elements[key];
        if (field && !field.value) field.value = value || "";
      }
      filling = false;
      setStatus("Recovered draft from out/workbench_draft.json");
    }
    async function saveDraft() {
      const data = formData();
      if (!Object.values(data).some((value) => String(value || "").trim() !== "")) return;
      const response = await fetch(boot.apiBase + "/draft", {
        method: "POST",
        headers: {"Content-Type": "application/json", "X-Slidex-Workbench-Token": boot.token},
        body: JSON.stringify(data)
      });
      if (response.ok) setStatus("Draft saved");
    }
    form.addEventListener("input", () => {
      if (filling) return;
      clearTimeout(draftTimer);
      draftTimer = setTimeout(saveDraft, 500);
    });
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const data = formData();
      setStatus("Saving...");
      const response = await fetch(boot.apiBase + "/save", {
        method: "POST",
        headers: {"Content-Type": "application/json", "X-Slidex-Workbench-Token": boot.token},
        body: JSON.stringify(data)
      });
      if (!response.ok) {
        setStatus("Save failed", true);
        return;
      }
      setStatus("Saved to brief.md and out/workbench_manifest.json");
    });
    loadDraft();
  </script>
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
	if err := copyDir(templateAbs, deckAbs); err != nil {
		return deckBootstrapResult{}, err
	}
	return deckBootstrapResult{Workspace: filepath.ToSlash(root), DeckID: deckID, DeckDir: filepath.ToSlash(displayDeckPath(root, deckAbs)), Status: "created"}, nil
}

func resolveTemplateDir(root, fromTemplate string) string {
	template := fromTemplate
	if template == "" {
		template = "decks/_template"
	}
	if filepath.IsAbs(template) {
		return filepath.Clean(template)
	}
	rootCandidate := filepath.Join(root, template)
	if pathExists(rootCandidate) {
		return rootCandidate
	}
	if filepath.Clean(template) != filepath.Clean("decks/_template") {
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
	return nil
}

func safeDeckDir(root, deckID string) (string, error) {
	decksRoot := filepath.Join(root, "decks")
	deckAbs := filepath.Clean(filepath.Join(decksRoot, deckID))
	if !pathWithin(decksRoot, deckAbs) {
		return "", fmt.Errorf("deck path escapes decks directory: %s", deckID)
	}
	if err := rejectSymlinkEscape(decksRoot, deckAbs, true); err != nil {
		return "", err
	}
	return deckAbs, nil
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
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("deck path must not contain symlinks: %s", filepath.ToSlash(current))
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
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("deck path must not contain symlinks: %s", filepath.ToSlash(path))
	}
	return nil
}

func workbenchStatus(workspace, deckID, deck string) (workbenchManifest, error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, "decks/_template")
	if err != nil {
		return workbenchManifest{}, err
	}
	return workbenchStatusForDeck(deckAbs), nil
}

func workbenchStatusForDeck(deckAbs string) workbenchManifest {
	manifest, ok := readWorkbenchManifest(deckAbs)
	if !ok {
		return canonicalWorkbenchManifestPaths(deckAbs, workbenchManifest{Status: "not_started"})
	}
	manifest = canonicalWorkbenchManifestPaths(deckAbs, manifest)
	if isWorkbenchReady(manifest) {
		manifest.Status = "running"
	} else if manifest.Status == "running" || manifest.Status == "starting" {
		manifest.Status = "stale"
	}
	return manifest
}

func stopWorkbench(workspace, deckID, deck string) (workbenchManifest, error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, "decks/_template")
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
	f, err := openSecureAppendFile(filepath.Join(outDir, workbenchLockName), 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	_, _ = fmt.Fprintf(f, "pid=%d acquired=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func stopWorkbenchProcess(manifest workbenchManifest) {
	if manifest.PID <= 0 {
		return
	}
	_ = syscall.Kill(-manifest.PID, syscall.SIGTERM)
	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !isWorkbenchReady(manifest) {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
	_ = syscall.Kill(-manifest.PID, syscall.SIGKILL)
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
		BrowserOpenStrategy: "Codex App in-app browser URL click/manual navigation or Browser plugin navigation; no proprietary Canvas mount API is used.",
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
	raw, err := os.ReadFile(filepath.Join(deckAbs, "out", workbenchManifestName))
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
	return secureWriteJSON(filepath.Join(deckAbs, "out", workbenchManifestName), manifest)
}

func readWorkbenchDraft(deckAbs string) (workbenchDraft, bool) {
	raw, err := os.ReadFile(filepath.Join(deckAbs, "out", workbenchDraftName))
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
	raw, err := os.ReadFile(path)
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
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, "decks/_template")
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
	info, err := os.Stat(sourceAbs)
	if err != nil {
		return nil, fmt.Errorf("browser screenshot is missing or unreadable: %s: %w", filepath.ToSlash(sourceAbs), err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("browser screenshot is a directory: %s", filepath.ToSlash(sourceAbs))
	}
	ext := strings.ToLower(filepath.Ext(sourceAbs))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		return nil, fmt.Errorf("browser screenshot must be .png, .jpg, .jpeg, or .webp: %s", filepath.ToSlash(sourceAbs))
	}
	f, err := os.Open(sourceAbs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
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
	target := filepath.Join(deckAbs, "out", workbenchBrowserScreenshot+ext)
	if err := secureWriteFile(target, raw, 0o600); err != nil {
		return nil, err
	}
	artifact := artifactFromPath(target)
	artifact.Path = filepath.ToSlash(target)
	return &artifact, nil
}

func verifyWorkbenchBrowserEvidence(workspace, deckID, deck string, requireScreenshot bool) (workbenchBrowserEvidenceVerification, error) {
	deckAbs, err := resolveDeckDir(workspace, deckID, deck, false, "decks/_template")
	if err != nil {
		return workbenchBrowserEvidenceVerification{}, err
	}
	manifestPath := filepath.Join(deckAbs, "out", workbenchManifestName)
	evidencePath := filepath.Join(deckAbs, "out", workbenchBrowserEvidenceName)
	result := workbenchBrowserEvidenceVerification{
		SchemaVersion:     "slidex.workbenchBrowserEvidenceVerification.v1",
		ToolName:          toolName,
		ToolVersion:       toolVersion,
		Status:            "fail",
		CheckedAt:         time.Now().UTC().Format(time.RFC3339),
		RequireScreenshot: requireScreenshot,
		DeckID:            filepath.Base(deckAbs),
		DeckDir:           filepath.ToSlash(deckAbs),
		EvidencePath:      filepath.ToSlash(evidencePath),
		ManifestPath:      filepath.ToSlash(manifestPath),
		Findings:          []string{},
		VerifiedFiles:     map[string]artifact{},
	}
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
	raw, err := os.ReadFile(evidencePath)
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
			} else if evidence.BrowserScreenshot.Path != actual.Path ||
				evidence.BrowserScreenshot.SHA256 != actual.SHA256 ||
				evidence.BrowserScreenshot.Size != actual.Size {
				addFinding("browser screenshot evidence is stale")
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
	}
	evidencePath := filepath.Join(manifest.OutDir, workbenchBrowserEvidenceName)
	if _, err := os.Stat(evidencePath); err == nil {
		status["browserEvidence"] = filepath.ToSlash(evidencePath)
	}
	return status
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
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/readyz", manifest.Port), nil)
	if err != nil {
		return false
	}
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
	expected, err := url.Parse(expectedURL)
	if err != nil {
		return false
	}
	expectedOrigin := expected.Scheme + "://" + expected.Host
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

func normalizeWorkbenchInput(input workbenchSaveInput) workbenchSaveInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Audience = strings.TrimSpace(input.Audience)
	input.DecisionGoal = strings.TrimSpace(input.DecisionGoal)
	input.SourceNotes = strings.TrimSpace(input.SourceNotes)
	input.OutputExpectations = strings.TrimSpace(input.OutputExpectations)
	return input
}

func hasAnyWorkbenchInput(input workbenchSaveInput) bool {
	return input.Title != "" || input.Audience != "" || input.DecisionGoal != "" || input.SourceNotes != "" || input.OutputExpectations != ""
}

func writeWorkbenchBrief(deckAbs string, input workbenchSaveInput) error {
	if input.Title == "" || input.Audience == "" || input.DecisionGoal == "" {
		return errors.New("title, audience, and decisionGoal are required")
	}
	var b strings.Builder
	b.WriteString("# " + input.Title + "\n\n")
	b.WriteString("## Audience\n\n" + input.Audience + "\n\n")
	b.WriteString("## Decision Goal\n\n" + input.DecisionGoal + "\n\n")
	if input.SourceNotes != "" {
		b.WriteString("## Source-Material Notes\n\n" + input.SourceNotes + "\n\n")
	}
	if input.OutputExpectations != "" {
		b.WriteString("## Output Expectations\n\n" + input.OutputExpectations + "\n\n")
	}
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
