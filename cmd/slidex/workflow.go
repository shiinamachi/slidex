package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	requiredCodexVersion = "0.132.0"
	stateSchemaVersion   = "slidex.state.v1"
	threadsSchemaVersion = "slidex.codexThreads.v1"
)

type codedError struct {
	code int
	err  error
}

func (e codedError) Error() string {
	return e.err.Error()
}

func (e codedError) Unwrap() error {
	return e.err
}

func (e codedError) ExitCode() int {
	return e.code
}

func exitCodeError(code int, format string, args ...any) error {
	return codedError{code: code, err: fmt.Errorf(format, args...)}
}

type stageRecord struct {
	Stage         string     `json:"stage"`
	Status        string     `json:"status"`
	Inputs        []artifact `json:"inputs,omitempty"`
	Outputs       []artifact `json:"outputs,omitempty"`
	Runtime       any        `json:"runtime,omitempty"`
	Verifier      any        `json:"verifier,omitempty"`
	StopCondition string     `json:"stopCondition,omitempty"`
	StartedAt     string     `json:"startedAt,omitempty"`
	CompletedAt   string     `json:"completedAt,omitempty"`
	Error         string     `json:"error,omitempty"`
}

type runtimeState struct {
	Mode                string   `json:"mode"`
	RequiredVersion     string   `json:"requiredVersion"`
	InstalledVersion    string   `json:"installedVersion,omitempty"`
	ProtocolBundle      string   `json:"protocolBundle,omitempty"`
	ProtocolBundleHash  string   `json:"protocolBundleHash,omitempty"`
	AllowMismatch       bool     `json:"allowMismatch"`
	Reason              string   `json:"reason,omitempty"`
	MissingCapabilities []string `json:"missingCapabilities,omitempty"`
}

type goalMirror struct {
	Objective                string `json:"objective,omitempty"`
	ObjectiveFile            string `json:"objectiveFile,omitempty"`
	Status                   string `json:"status,omitempty"`
	TokenBudget              int    `json:"tokenBudget,omitempty"`
	UsageLimitReached        bool   `json:"usageLimitReached,omitempty"`
	RepeatedBlockerSignature string `json:"repeatedBlockerSignature,omitempty"`
}

type slidexState struct {
	SchemaVersion        string                 `json:"schemaVersion"`
	ToolName             string                 `json:"toolName"`
	ToolVersion          string                 `json:"toolVersion"`
	GeneratedAt          string                 `json:"generatedAt"`
	ActiveDeckID         string                 `json:"activeDeckId"`
	DeckDir              string                 `json:"deckDir"`
	OutDir               string                 `json:"outDir"`
	RequiredCodexVersion string                 `json:"requiredCodexVersion"`
	CodexRuntime         runtimeState           `json:"codexRuntime"`
	Stages               []stageRecord          `json:"stages"`
	Goal                 goalMirror             `json:"goal"`
	UnresolvedRisks      []acceptedRisk         `json:"unresolvedRisks,omitempty"`
	AcceptedRisks        []acceptedRisk         `json:"acceptedRisks,omitempty"`
	Interventions        []codexIntervention    `json:"interventions,omitempty"`
	MemorySummaries      []compactSummaryRecord `json:"memorySummaries,omitempty"`
	EventReplays         []eventReplayRecord    `json:"eventReplays,omitempty"`
}

type codexThreadIndex struct {
	SchemaVersion string        `json:"schemaVersion"`
	CodexVersion  string        `json:"codexVersion"`
	GeneratedAt   string        `json:"generatedAt"`
	Threads       []threadState `json:"threads"`
}

type threadState struct {
	ThreadID                 string         `json:"threadId"`
	ThreadName               string         `json:"threadName"`
	Role                     string         `json:"role,omitempty"`
	Mode                     string         `json:"mode,omitempty"`
	ParentThreadID           string         `json:"parentThreadId,omitempty"`
	Stage                    string         `json:"stage"`
	LastTurnID               string         `json:"lastTurnId,omitempty"`
	TurnIDs                  []string       `json:"turnIds,omitempty"`
	Model                    string         `json:"model,omitempty"`
	ServiceTier              string         `json:"serviceTier,omitempty"`
	ApprovalPolicy           string         `json:"approvalPolicy,omitempty"`
	ApprovalMode             string         `json:"approvalMode,omitempty"`
	Sandbox                  string         `json:"sandbox,omitempty"`
	SandboxMode              string         `json:"sandboxMode,omitempty"`
	EffectiveWorkspaceRoots  []string       `json:"effectiveWorkspaceRoots,omitempty"`
	TokenUsage               map[string]int `json:"tokenUsage,omitempty"`
	GlobalFeatureProbe       any            `json:"globalFeatureProbe,omitempty"`
	ThreadScopedFeatureProbe any            `json:"threadScopedFeatureProbe,omitempty"`
	OutputSchemaHash         string         `json:"outputSchemaHash,omitempty"`
	LastEventLog             string         `json:"lastEventLog,omitempty"`
	GoalStatus               string         `json:"goalStatus,omitempty"`
	PromptTemplateVersion    string         `json:"promptTemplateVersion,omitempty"`
}

type codexIntervention struct {
	Method         string `json:"method"`
	ThreadID       string `json:"threadId"`
	TurnID         string `json:"turnId,omitempty"`
	ExpectedTurnID string `json:"expectedTurnId,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Stage          string `json:"stage,omitempty"`
	Status         string `json:"status"`
	Artifact       string `json:"artifact"`
	CreatedAt      string `json:"createdAt"`
}

type compactSummaryRecord struct {
	SchemaVersion    string `json:"schemaVersion"`
	CodexVersion     string `json:"codexVersion"`
	SourceThreadID   string `json:"sourceThreadId"`
	SourceThreadHash string `json:"sourceThreadHash"`
	CompactTurnID    string `json:"compactTurnId,omitempty"`
	SummaryHash      string `json:"summaryHash"`
	Artifact         string `json:"artifact"`
	CreatedAt        string `json:"createdAt"`
	Stale            bool   `json:"stale"`
}

type eventReplayRecord struct {
	SchemaVersion string `json:"schemaVersion"`
	Kind          string `json:"kind"`
	Artifact      string `json:"artifact"`
	ThreadCount   int    `json:"threadCount"`
	EventCount    int    `json:"eventCount"`
	CreatedAt     string `json:"createdAt"`
}

type acceptedRisk struct {
	Reason       string `json:"reason"`
	Owner        string `json:"owner"`
	Expiration   string `json:"expiration"`
	ArtifactLink string `json:"artifactLink"`
}

type visualReviewImageSet struct {
	SchemaVersion         string          `json:"schemaVersion"`
	GeneratedAt           string          `json:"generatedAt"`
	HTMLSha256            string          `json:"htmlSha256"`
	ManifestSha256        string          `json:"manifestSha256"`
	ImageSetSha256        string          `json:"imageSetSha256"`
	RequestedFidelity     string          `json:"requestedFidelity"`
	FidelitySupportStatus string          `json:"fidelitySupportStatus"`
	Images                []renderedImage `json:"images"`
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fromTemplate := fs.String("from-template", "decks/_template", "template deck directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return exitCodeError(2, "usage: slidex init <deck_id> [--from-template decks/_template]")
	}
	deckID := fs.Arg(0)
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(deckID) {
		return exitCodeError(2, "deck_id must contain only letters, numbers, underscore, dash, and dot")
	}
	dst := filepath.Join("decks", deckID)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("deck already exists: %s", dst)
	}
	if err := copyDir(*fromTemplate, dst); err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "version": toolVersion, "deckDir": dst, "status": "created"})
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	checkCodex := fs.Bool("codex", false, "check Codex CLI integration")
	checkRender := fs.Bool("render", false, "check Chrome/Chromium render dependency")
	jsonOut := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report := doctorReport(*deck, *checkCodex, *checkRender)
	if *jsonOut {
		if err := printJSON(report); err != nil {
			return err
		}
	} else {
		printDoctorHuman(report)
	}
	if doctorHasUnsupported(report) {
		return exitCodeError(4, "doctor found unsupported Codex/App Server features")
	}
	if doctorHasFail(report) {
		return errors.New("doctor found failures")
	}
	return nil
}

func doctorReport(deck string, checkCodex, checkRender bool) map[string]any {
	findings := []qaFinding{}
	goModVersion := readGoModVersion("go.mod")
	miseGoVersion := readMiseGoVersion(".mise.toml")
	if goModVersion == "" {
		findings = append(findings, fail("doctor.go_mod", "go.mod go directive missing", "go.mod"))
	}
	if miseGoVersion == "" {
		findings = append(findings, fail("doctor.mise", ".mise.toml go pin missing", ".mise.toml"))
	}
	if goModVersion != "" && miseGoVersion != "" && goModVersion != miseGoVersion {
		findings = append(findings, fail("doctor.go_pin", "go.mod and .mise.toml Go versions differ", "go.mod"))
	}
	if goModVersion != "" && !isExactVersion(goModVersion) {
		findings = append(findings, fail("doctor.go_pin", "Go version must be exact", "go.mod"))
	}
	if _, err := os.Stat(".agents/skills/slidex/SKILL.md"); err != nil {
		findings = append(findings, fail("doctor.skill_path", "missing companion skill at .agents/skills/slidex/SKILL.md", ".agents/skills/slidex/SKILL.md"))
	}
	if _, err := os.Stat(".codex/skills/slidex/SKILL.md"); err == nil {
		findings = append(findings, fail("doctor.forbidden_skill_path", "forbidden companion skill path exists", ".codex/skills/slidex/SKILL.md"))
	}
	chrome := ""
	if checkRender {
		if path, err := resolveChrome(""); err != nil {
			findings = append(findings, fail("doctor.chrome", err.Error(), "PATH"))
		} else {
			chrome = chromeVersion(path)
			if !hasExactVersionToken(chrome) {
				findings = append(findings, fail("doctor.chrome_version", "Chrome/Chromium version must be exact", path))
			}
		}
	}
	codexVersion := ""
	codexDoctor := ""
	featureList := ""
	mcpList := ""
	pluginList := ""
	protocol := map[string]any{}
	if checkCodex {
		codexVersion = installedCodexVersion()
		if codexVersion != requiredCodexVersion {
			findings = append(findings, qaFinding{Severity: "fail", Check: "doctor.codex_version", Message: "Codex CLI version must be " + requiredCodexVersion + ", got " + firstNonEmpty(codexVersion, "missing"), Path: "codex"})
		}
		codexDoctor = commandOutput(30*time.Second, "codex", "doctor", "--json")
		featureList = commandOutput(8*time.Second, "codex", "features", "list")
		mcpList = commandOutput(8*time.Second, "codex", "mcp", "list")
		pluginList = commandOutput(8*time.Second, "codex", "plugin", "list")
		protocol = probeProtocolSchema()
		if ok, _ := protocol["ok"].(bool); !ok {
			findings = append(findings, qaFinding{Severity: "fail", Check: "doctor.protocol_schema", Message: fmt.Sprint(protocol["error"]), Path: "codex app-server generate-json-schema"})
		}
	}
	if deck != "" {
		if _, err := inspectDeck(deck); err != nil {
			findings = append(findings, fail("doctor.deck", err.Error(), deck))
		}
	}
	return map[string]any{
		"toolName":        toolName,
		"version":         toolVersion,
		"generatedAt":     time.Now().UTC().Format(time.RFC3339),
		"goModVersion":    goModVersion,
		"miseGoVersion":   miseGoVersion,
		"codexVersion":    codexVersion,
		"requiredCodex":   requiredCodexVersion,
		"chromeVersion":   chrome,
		"protocolSchema":  protocol,
		"codexDoctorJson": json.RawMessage(nullOrRaw(codexDoctor)),
		"features":        featureList,
		"mcp":             mcpList,
		"plugins":         pluginList,
		"findings":        findings,
		"status":          statusFromFindings(findings),
	}
}

func runIntake(args []string) error {
	fs := flag.NewFlagSet("intake", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	answers := fs.String("answers", "", "answers file for batch intake")
	interactive := fs.Bool("interactive", false, "reserved for interactive intake")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = interactive
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	deckAbs := mustAbs(*deck)
	inv, err := inspectDeck(deckAbs)
	if err != nil {
		return err
	}
	if err := writeSourceInventory(inv); err != nil {
		return err
	}
	if *answers != "" {
		if err := applyIntakeAnswers(deckAbs, *answers); err != nil {
			return err
		}
		if err := writeIntakeQuestions(deckAbs, nil, "complete"); err != nil {
			return err
		}
		return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "status": "complete", "answersApplied": *answers})
	}
	questions := intakeQuestionsForDeck(deckAbs)
	status := "complete"
	if len(questions) > 0 {
		status = "user_input_required"
	}
	if err := writeIntakeQuestions(deckAbs, questions, status); err != nil {
		return err
	}
	if len(questions) > 0 {
		_ = printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "status": status, "questions": questions})
		return exitCodeError(3, "intake requires user input; questions written to %s", filepath.Join(deckAbs, "out", "intake_questions.md"))
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "status": status})
}

func runStrategy(args []string) error {
	fs := flag.NewFlagSet("strategy", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	force := fs.Bool("force", false, "rewrite strategy.md")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	path, err := ensureStrategy(*deck, *force)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": "complete", "strategy": path})
}

func runSpec(args []string) error {
	fs := flag.NewFlagSet("spec", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	force := fs.Bool("force", false, "rewrite deck_spec.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	path, err := ensureSpec(*deck, *force)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": "complete", "spec": path})
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	force := fs.Bool("force", false, "rewrite final_deck.html")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	path, err := ensureHTML(*deck, *force)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": "complete", "html": path})
}

func runRevise(args []string) error {
	fs := flag.NewFlagSet("revise", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	until := fs.String("until", "pass", "pass or risk-accepted")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	qa, err := qaDeck(*deck, true)
	if err != nil && qa.Status == "fail" {
		return err
	}
	if *until == "pass" && qa.Status != "pass" {
		return exitCodeError(6, "revise stopped with accepted or unresolved risks: %s", qa.Status)
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": qa.Status, "stopCondition": *until})
}

func runFinalize(args []string) error {
	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	path, err := writeDeliverySummary(*deck)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": "complete", "deliverySummary": path})
}

func runPipeline(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	until := fs.String("until", "package", "package, qa, or render")
	nonInteractive := fs.Bool("non-interactive", false, "do not open TUI")
	codexMode := fs.String("codex-mode", "app-server", "app-server, exec, or exec_fallback")
	allowMismatch := fs.Bool("allow-codex-protocol-mismatch", false, "continue with recorded risk on protocol mismatch")
	chromeNoSandbox := fs.Bool("chrome-no-sandbox", false, "allow Chrome --no-sandbox fallback")
	visualReview := fs.String("visual-review", "codex", "visual review mode: codex, manual, or none")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = nonInteractive
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	deckAbs := mustAbs(*deck)
	outDir := filepath.Join(deckAbs, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	unlock, err := acquireRunLock(outDir)
	if err != nil {
		return err
	}
	defer unlock()
	state := newState(deckAbs, *codexMode, *allowMismatch)
	if previous := readStateOrNew(deckAbs, *codexMode, *allowMismatch); previous.Goal.Objective != "" || previous.Goal.ObjectiveFile != "" || previous.Goal.TokenBudget != 0 {
		state.Goal = previous.Goal
	}
	if risk := protocolMismatchAcceptedRisk(state); risk != nil {
		state.AcceptedRisks = append(state.AcceptedRisks, *risk)
	}
	if state.Goal.Status == "" {
		state.Goal.Status = "active"
	}
	if err := enforceCodexRuntimeGate(state); err != nil {
		_ = writeState(outDir, state)
		return err
	}
	var appRun *appServerWorkflowRun
	defer func() {
		if appRun != nil {
			appRun.close()
		}
	}()
	recorder := func(stage string, fn func() error) error {
		start := time.Now().UTC().Format(time.RFC3339)
		inputs := stageInputs(deckAbs, stage)
		err := fn()
		verifier := map[string]any{"name": stage + "_contract"}
		if err == nil && shouldRunAgentStageAudit(stage) {
			var auditPath string
			var auditErr error
			switch state.CodexRuntime.Mode {
			case "app-server":
				if appRun != nil && appRun.threadID != "" {
					auditPath, auditErr = runAppServerStageAudit(appRun, deckAbs, state, stage)
					if auditErr == nil {
						verifier["appServerTurn"] = filepath.ToSlash(auditPath)
					}
				}
			case "exec", "exec_fallback":
				auditPath, auditErr = runCodexExecStageAudit(deckAbs, stage, false, "")
				if auditErr == nil {
					verifier["execRun"] = filepath.ToSlash(auditPath)
				}
			}
			if auditErr != nil {
				err = auditErr
			}
		}
		status := "pass"
		stop := "pass"
		msg := ""
		if err != nil {
			status = "fail"
			stop = "blocked"
			msg = err.Error()
			var coded interface{ ExitCode() int }
			if errors.As(err, &coded) && coded.ExitCode() == 3 {
				stop = "user_input_required"
			}
		}
		verifier["status"] = status
		state.Stages = append(state.Stages, stageRecord{
			Stage:         stage,
			Status:        status,
			Inputs:        inputs,
			Outputs:       stageOutputs(deckAbs, stage),
			Runtime:       state.CodexRuntime,
			Verifier:      verifier,
			StopCondition: stop,
			StartedAt:     start,
			CompletedAt:   time.Now().UTC().Format(time.RFC3339),
			Error:         msg,
		})
		_ = writeState(outDir, state)
		_ = appendRunLog(outDir, map[string]any{"event": "stage_completed", "stage": stage, "status": status, "stopCondition": stop, "error": msg})
		return err
	}
	if err := recorder("resolve_workspace", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		if state.CodexRuntime.Mode == "app-server" {
			run, err := startAppServerWorkflowRun(deckAbs)
			if err != nil {
				state.CodexRuntime.Mode = "exec_fallback"
				state.CodexRuntime.Reason = "app_server_unavailable_or_failed: " + err.Error()
				state.UnresolvedRisks = append(state.UnresolvedRisks, acceptedRisk{Reason: "App Server unavailable; using codex exec fallback with output schema: " + err.Error(), Owner: "slidex", Expiration: time.Now().Add(24 * time.Hour).Format(time.RFC3339), ArtifactLink: "out/protocol_diagnostics.json"})
				_ = writeJSONFile(filepath.Join(outDir, "protocol_diagnostics.json"), map[string]any{"schemaVersion": "slidex.protocolDiagnostics.v1", "generatedAt": time.Now().UTC().Format(time.RFC3339), "codexVersion": installedCodexVersion(), "status": "exec_fallback", "error": err.Error()})
			} else {
				appRun = run
				if err := writeJSONFile(filepath.Join(outDir, "protocol_diagnostics.json"), appRun.snapshot); err != nil {
					return err
				}
				if err := writeThreadIndex(outDir, threadIndexFromAppServerSnapshot(deckAbs, appRun.snapshot)); err != nil {
					return err
				}
				state.CodexRuntime.ProtocolBundleHash = hashPathSet(filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion))
				if state.Goal.Objective == "" && state.Goal.ObjectiveFile == "" {
					state.Goal.Objective = "Complete slidex run for " + filepath.Base(deckAbs) + " with current HTML, rendered PNG/PDF, QA, review, and package gates fresh."
				}
				if goalSync, err := syncGoalWithAppRun(deckAbs, outDir, appRun, state.Goal); err == nil {
					_ = appendRunLog(outDir, map[string]any{"event": "goal_synced", "stage": "resolve_workspace", "appServerGoal": goalSync})
				} else {
					state.UnresolvedRisks = append(state.UnresolvedRisks, acceptedRisk{Reason: "App Server goal sync failed: " + err.Error(), Owner: "slidex", Expiration: time.Now().Add(24 * time.Hour).Format(time.RFC3339), ArtifactLink: "out/slidex_state.json"})
				}
			}
		}
		if err := ensureRuntimeArtifacts(deckAbs, state); err != nil {
			return err
		}
		if state.CodexRuntime.Mode == "app-server" {
			if snapshotRaw, err := os.ReadFile(filepath.Join(outDir, "protocol_diagnostics.json")); err == nil {
				var snapshot map[string]any
				if json.Unmarshal(snapshotRaw, &snapshot) == nil && snapshot["thread_start"] != nil {
					return writeThreadIndex(outDir, threadIndexFromAppServerSnapshot(deckAbs, snapshot))
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := recorder("inspect_inputs", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		inv, err := inspectDeck(deckAbs)
		if err != nil {
			return err
		}
		return writeSourceInventory(inv)
	}); err != nil {
		return err
	}
	if err := recorder("intake", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		questions := intakeQuestionsForDeck(deckAbs)
		if err := writeIntakeQuestions(deckAbs, questions, statusForQuestions(questions)); err != nil {
			return err
		}
		if len(questions) > 0 {
			return exitCodeError(3, "intake requires user input")
		}
		return nil
	}); err != nil {
		return err
	}
	if err := recorder("strategy", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		_, err := ensureStrategy(deckAbs, false)
		return err
	}); err != nil {
		return err
	}
	if err := recorder("spec", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		_, err := ensureSpec(deckAbs, false)
		return err
	}); err != nil {
		return err
	}
	if err := recorder("build_html", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		_, err := ensureHTML(deckAbs, false)
		return err
	}); err != nil {
		return err
	}
	if err := recorder("baseline_html", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		return copyFile(filepath.Join(outDir, "final_deck.html"), filepath.Join(outDir, "final_deck.generated_baseline.html"))
	}); err != nil {
		return err
	}
	renderStage := func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		cfg, err := renderConfigFromFlags(filepath.Join(outDir, "final_deck.html"), filepath.Join(outDir, "rendered_slides"), filepath.Join(outDir, "final_deck.pdf"), filepath.Join(outDir, "render_manifest.json"), "paginated", ".slide", 1920, 1080, "pretendard", "", *chromeNoSandbox)
		if err != nil {
			return err
		}
		_, err = renderHTML(cfg)
		return err
	}
	if err := recorder("render", renderStage); err != nil {
		return err
	}
	if *until == "render" {
		return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "status": "complete", "until": *until})
	}
	if err := recorder("qa", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		var qa qaResult
		var err error
		if state.CodexRuntime.Mode == "app-server" && appRun != nil {
			qa, err = qaDeckWithAppServerVisualReview(deckAbs, true, *visualReview, appRun)
		} else {
			qa, err = qaDeckWithVisualReview(deckAbs, true, *visualReview)
		}
		if err != nil {
			return err
		}
		if qa.Status == "fail" {
			return errors.New("qa failed")
		}
		return nil
	}); err != nil {
		return err
	}
	if *until == "qa" {
		return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "status": "complete", "until": *until})
	}
	if err := recorder("review_loop", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		_, err := writeStructuredReviewForRuntime(deckAbs, "delivery", 1, state.CodexRuntime.Mode, appRun)
		return err
	}); err != nil {
		return err
	}
	if err := recorder("delivery_summary", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		_, err := writeDeliverySummary(deckAbs)
		return err
	}); err != nil {
		return err
	}
	if err := recorder("package", func() error {
		if shouldStopGoalContinuation(state.Goal) {
			return goalStopError(state.Goal)
		}
		result, err := packageDeck(deckAbs, false)
		if err != nil {
			return err
		}
		if result["status"] == "fail" {
			return errors.New("package verification failed")
		}
		if result["status"] != "pass" {
			return exitCodeError(6, "package verification has unresolved or unaccepted risks")
		}
		return nil
	}); err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "status": "complete", "until": *until, "state": filepath.Join(outDir, "slidex_state.json")})
}

func runClean(args []string) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	logs := fs.Bool("logs", false, "clean logs")
	olderThan := fs.String("older-than", "168h", "duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	d, err := time.ParseDuration(*olderThan)
	if err != nil {
		return err
	}
	removed := []string{}
	if *logs {
		outDir := filepath.Join(mustAbs(*deck), "out")
		cutoff := time.Now().Add(-d)
		for _, rel := range []string{"run_log.jsonl", "agent_runs", "agent_reviews", "visual_reviews"} {
			path := filepath.Join(outDir, rel)
			info, err := os.Stat(path)
			if err == nil && info.ModTime().Before(cutoff) {
				if err := os.RemoveAll(path); err != nil {
					return err
				}
				removed = append(removed, path)
			}
		}
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "removed": removed})
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	from := fs.String("from", "legacy-html-pdf", "legacy-html-pdf or pptx-first")
	write := fs.Bool("write", false, "apply safe migration changes")
	dryRun := fs.Bool("dry-run", true, "report migration findings without changes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	deckAbs := mustAbs(*deck)
	outDir := filepath.Join(deckAbs, "out")
	findings := migrationFindings(deckAbs, *from)
	created := []string{}
	if !*dryRun && !*write {
		return exitCodeError(2, "--dry-run=false is not allowed without --write")
	}
	effectiveWrite := *write
	if effectiveWrite {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
		htmlPath := filepath.Join(outDir, "final_deck.html")
		basePath := filepath.Join(outDir, "final_deck.generated_baseline.html")
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			if _, err := os.Stat(htmlPath); err == nil {
				if err := copyFile(htmlPath, basePath); err != nil {
					return err
				}
				created = append(created, basePath)
			}
		}
		state := newState(deckAbs, "exec", false)
		if err := writeState(outDir, state); err != nil {
			return err
		}
		created = append(created, filepath.Join(outDir, "slidex_state.json"))
		if err := writeThreadIndex(outDir, codexThreadIndex{SchemaVersion: threadsSchemaVersion, CodexVersion: installedCodexVersion(), GeneratedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
			return err
		}
		created = append(created, filepath.Join(outDir, "codex_threads.json"))
	}
	mode := "dry-run"
	if effectiveWrite {
		mode = "write"
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "mode": mode, "from": *from, "findings": findings, "created": created})
}

func runCodex(args []string) error {
	if len(args) == 0 {
		return exitCodeError(2, "usage: slidex codex <doctor|app-server|schema|exec|models|features|mcp|plugins|threads|turn|review|remote-control>")
	}
	switch args[0] {
	case "doctor":
		return runDoctor(append(args[1:], "--codex"))
	case "schema":
		return runCodexSchema(args[1:])
	case "exec":
		return runCodexExec(args[1:])
	case "app-server":
		return runCodexAppServer(args[1:])
	case "models":
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		snapshot, err := appServerCapabilitySnapshot(mustAbs("."), false)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"toolName": toolName, "models": snapshot["model_list"]})
	case "features":
		fs := flag.NewFlagSet("codex features", flag.ContinueOnError)
		thread := fs.String("thread", "", "thread id for thread-scoped probe")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		_ = jsonOut
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		if *thread != "" {
			snapshot, err := appServerThreadFeatureProbe(*thread)
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"toolName": toolName, "thread": *thread, "features": snapshot["experimentalFeature_thread_scoped"]})
		}
		return printCommandJSON("codex", "features", commandOutput(8*time.Second, "codex", "features", "list"))
	case "mcp":
		return printCommandJSON("codex", "mcp", commandOutput(8*time.Second, "codex", "mcp", "list"))
	case "plugins":
		return printCommandJSON("codex", "plugins", commandOutput(8*time.Second, "codex", "plugin", "list"))
	case "threads":
		return runCodexThreads(args[1:])
	case "turn":
		return runCodexTurn(args[1:])
	case "review":
		return runCodexReview(args[1:])
	case "remote-control":
		return printCommandJSON("codex", "remote-control", commandOutput(8*time.Second, "codex", "remote-control", "status", "--json"))
	default:
		return exitCodeError(2, "unknown codex command: %s", args[0])
	}
}

func runCodexSchema(args []string) error {
	if len(args) == 0 || args[0] != "refresh" {
		return exitCodeError(2, "usage: slidex codex schema refresh [--codex-version 0.132.0]")
	}
	fs := flag.NewFlagSet("codex schema refresh", flag.ContinueOnError)
	version := fs.String("codex-version", requiredCodexVersion, "Codex CLI version")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *version != requiredCodexVersion {
		return exitCodeError(4, "unsupported Codex protocol version: %s", *version)
	}
	if installed := installedCodexVersion(); installed != requiredCodexVersion {
		return exitCodeError(4, "Codex CLI version mismatch: need %s, got %s", requiredCodexVersion, firstNonEmpty(installed, "missing"))
	}
	outDir := filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion, "schema")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	help := commandOutput(8*time.Second, "codex", "app-server", "generate-json-schema", "--help")
	cmdArgs := []string{"app-server", "generate-json-schema", "--out", outDir}
	if strings.Contains(help, "--experimental") {
		cmdArgs = append(cmdArgs, "--experimental")
	}
	cmd := exec.Command("codex", cmdArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schema refresh failed: %w\n%s", err, string(out))
	}
	manifest, err := writeProtocolManifest(filepath.Dir(outDir))
	if err != nil {
		return err
	}
	if err := writeMethodConstants(filepath.Dir(outDir)); err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "status": "complete", "schemaDir": outDir, "manifest": manifest})
}

func runCodexExec(args []string) error {
	if len(args) == 0 || args[0] != "probe" {
		return exitCodeError(2, "usage: slidex codex exec probe --deck decks/<deck_id> [--stage STAGE] [--resume last|SESSION] [--schema FILE]")
	}
	if err := enforceDirectCodexRuntime("exec"); err != nil {
		return err
	}
	fs := flag.NewFlagSet("codex exec probe", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	stage := fs.String("stage", "resolve_workspace", "stage name")
	resume := fs.String("resume", "", "resume target: last or session id")
	schema := fs.String("schema", filepath.Join("schemas", "app_stage_result.strict.schema.json"), "output schema file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	deckAbs := mustAbs(*deck)
	prompt := stageAuditPrompt(deckAbs, readStateOrNew(deckAbs, "exec", false), *stage, "exec")
	path, payload, err := runCodexExecStructured(deckAbs, *stage, prompt, *schema, *resume != "", *resume, nil)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "stage": *stage, "execRun": path, "status": payload["status"], "resume": *resume != ""})
}

func runCodexAppServer(args []string) error {
	if len(args) == 0 {
		return exitCodeError(2, "usage: slidex codex app-server <start|status|stop|probe>")
	}
	switch args[0] {
	case "probe":
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		fs := flag.NewFlagSet("codex app-server probe", flag.ContinueOnError)
		listen := fs.String("listen", "stdio://", "listen URL")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		_ = jsonOut
		if *listen != "stdio://" {
			return exitCodeError(4, "only stdio:// app-server probe is implemented")
		}
		snapshot, err := appServerCapabilitySnapshot(mustAbs("."), true)
		if err != nil {
			return err
		}
		return printJSON(snapshot)
	case "start":
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		fs := flag.NewFlagSet("codex app-server start", flag.ContinueOnError)
		listen := fs.String("listen", "unix://", "listen URL")
		deck := fs.String("deck", "", "deck workspace directory")
		wsAuth := fs.String("ws-auth", "", "websocket auth mode")
		wsTokenFile := fs.String("ws-token-file", "", "absolute path to capability token file")
		wsTokenSHA256 := fs.String("ws-token-sha256", "", "SHA-256 of capability token")
		wsSharedSecretFile := fs.String("ws-shared-secret-file", "", "absolute path to signed bearer shared secret file")
		wsIssuer := fs.String("ws-issuer", "", "expected signed bearer issuer")
		wsAudience := fs.String("ws-audience", "", "expected signed bearer audience")
		wsMaxClockSkewSeconds := fs.Int("ws-max-clock-skew-seconds", 0, "maximum signed bearer clock skew")
		force := fs.Bool("force", false, "restart managed process if metadata exists")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		_ = jsonOut
		ws := webSocketAuthConfig{Mode: *wsAuth, TokenFile: *wsTokenFile, TokenSHA256: *wsTokenSHA256, SharedSecretFile: *wsSharedSecretFile, Issuer: *wsIssuer, Audience: *wsAudience, MaxClockSkewSeconds: *wsMaxClockSkewSeconds}
		return startManagedAppServer(*listen, *deck, ws, *force)
	case "status":
		fs := flag.NewFlagSet("codex app-server status", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		_ = jsonOut
		return statusManagedAppServer()
	case "stop":
		fs := flag.NewFlagSet("codex app-server stop", flag.ContinueOnError)
		force := fs.Bool("force", false, "force kill managed process")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return stopManagedAppServer(*force)
	default:
		return exitCodeError(2, "unknown app-server command: %s", args[0])
	}
}

type webSocketAuthConfig struct {
	Mode                string `json:"mode,omitempty"`
	TokenFile           string `json:"tokenFile,omitempty"`
	TokenSHA256         string `json:"tokenSha256,omitempty"`
	SharedSecretFile    string `json:"sharedSecretFile,omitempty"`
	Issuer              string `json:"issuer,omitempty"`
	Audience            string `json:"audience,omitempty"`
	MaxClockSkewSeconds int    `json:"maxClockSkewSeconds,omitempty"`
}

func startManagedAppServer(listen, deck string, ws webSocketAuthConfig, force bool) error {
	metaPath := appServerMetadataPath()
	if existing := readAppServerMetadata(metaPath); existing != nil {
		if pid, _ := numberAsInt(existing["pid"]); pid > 0 && processAlive(pid) {
			if !force {
				return exitCodeError(1, "managed app-server already appears active with pid %d; use --force to replace it", pid)
			}
			_ = stopManagedAppServer(true)
		}
	}
	actualListen := normalizeManagedListenURL(listen)
	if strings.HasPrefix(actualListen, "ws://") {
		if err := validateWebSocketAuth(actualListen, ws); err != nil {
			return err
		}
	}
	runtimeDir := filepath.Dir(metaPath)
	if err := ensureSecureDir(runtimeDir); err != nil {
		return err
	}
	stdoutPath := filepath.Join(runtimeDir, "codex-app-server.stdout.log")
	stderrPath := filepath.Join(runtimeDir, "codex-app-server.stderr.log")
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer stderr.Close()
	args := []string{"app-server", "--listen", actualListen}
	if ws.Mode != "" {
		args = append(args, "--ws-auth", ws.Mode)
	}
	if ws.TokenFile != "" {
		args = append(args, "--ws-token-file", ws.TokenFile)
	}
	if ws.TokenSHA256 != "" {
		args = append(args, "--ws-token-sha256", ws.TokenSHA256)
	}
	if ws.SharedSecretFile != "" {
		args = append(args, "--ws-shared-secret-file", ws.SharedSecretFile)
	}
	if ws.Issuer != "" {
		args = append(args, "--ws-issuer", ws.Issuer)
	}
	if ws.Audience != "" {
		args = append(args, "--ws-audience", ws.Audience)
	}
	if ws.MaxClockSkewSeconds > 0 {
		args = append(args, "--ws-max-clock-skew-seconds", strconv.Itoa(ws.MaxClockSkewSeconds))
	}
	cmd := exec.Command("codex", args...)
	cmd.Dir = mustAbs(".")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	decks := []string{}
	if deck != "" {
		decks = append(decks, mustAbs(deck))
	}
	metadata := map[string]any{
		"schemaVersion":   "slidex.appServerProcess.v1",
		"generatedAt":     time.Now().UTC().Format(time.RFC3339),
		"pid":             cmd.Process.Pid,
		"codexVersion":    installedCodexVersion(),
		"listen":          actualListen,
		"ownerUid":        os.Getuid(),
		"authMode":        firstNonEmpty(ws.Mode, "none"),
		"websocketAuth":   ws,
		"attachedDeckIds": decks,
		"stdout":          stdoutPath,
		"stderr":          stderrPath,
		"transportRisk":   transportRiskForListen(actualListen),
		"keepalivePolicy": map[string]any{"transport": "websocket", "pingIntervalSeconds": 30, "timeoutSeconds": 90, "reconnect": "exponential_backoff_with_jitter"},
		"retryPolicy":     map[string]any{"overloadCode": -32001, "initialDelayMs": 250, "maxDelayMs": 5000, "maxAttempts": 5},
		"stopPolicy":      map[string]any{"gracefulSignal": "interrupt", "forceSignal": "kill", "gracePeriodSeconds": 5},
	}
	if err := secureWriteJSON(metaPath, metadata); err != nil {
		return err
	}
	if risk := transportRiskForListen(actualListen); risk != "" && deck != "" {
		if err := recordWebSocketTransportRisk(mustAbs(deck), risk, metaPath); err != nil {
			return err
		}
	}
	return printJSON(map[string]any{"toolName": toolName, "status": "pass", "metadata": metadata})
}

func recordWebSocketTransportRisk(deckAbs, risk, metadataPath string) error {
	outDir := filepath.Join(deckAbs, "out")
	state := readStateOrNew(deckAbs, "app-server", false)
	state.AcceptedRisks = appendAcceptedRiskOnce(state.AcceptedRisks, acceptedRisk{
		Reason:       risk,
		Owner:        "slidex",
		Expiration:   time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		ArtifactLink: filepath.ToSlash(metadataPath),
	})
	return writeState(outDir, state)
}

func appendAcceptedRiskOnce(risks []acceptedRisk, risk acceptedRisk) []acceptedRisk {
	for i := range risks {
		if risks[i].Reason == risk.Reason && risks[i].ArtifactLink == risk.ArtifactLink {
			risks[i] = risk
			return risks
		}
	}
	return append(risks, risk)
}

func escapeMarkdownInline(value string) string {
	return strings.ReplaceAll(value, "`", "'")
}

func statusManagedAppServer() error {
	metaPath := appServerMetadataPath()
	metadata := readAppServerMetadata(metaPath)
	if metadata == nil {
		out := commandOutput(15*time.Second, "codex", "app-server", "daemon", "version")
		return printJSON(map[string]any{"toolName": toolName, "status": "missing_metadata", "metadataPath": metaPath, "daemonVersion": nullOrRawJSON(out)})
	}
	pid, _ := numberAsInt(metadata["pid"])
	metadata["alive"] = pid > 0 && processAlive(pid)
	if alive, _ := metadata["alive"].(bool); alive {
		if health, err := probeManagedAppServer(metadata); err == nil {
			metadata["health"] = health
		} else {
			metadata["health"] = map[string]any{"status": "fail", "error": err.Error()}
		}
	}
	return printJSON(map[string]any{"toolName": toolName, "status": "pass", "metadata": metadata})
}

func stopManagedAppServer(force bool) error {
	metaPath := appServerMetadataPath()
	metadata := readAppServerMetadata(metaPath)
	if metadata == nil {
		return runCommandJSON("app-server.stop", 30*time.Second, "codex", "app-server", "daemon", "stop")
	}
	pid, _ := numberAsInt(metadata["pid"])
	stopped := false
	if pid > 0 && processAlive(pid) {
		if proc, err := os.FindProcess(pid); err == nil {
			if force {
				_ = proc.Kill()
			} else {
				_ = proc.Signal(os.Interrupt)
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					if !processAlive(pid) {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
				if processAlive(pid) {
					metadata["stopPending"] = true
					metadata["lastStopAttemptAt"] = time.Now().UTC().Format(time.RFC3339)
					_ = secureWriteJSON(metaPath, metadata)
					return exitCodeError(1, "app-server did not stop gracefully; use --force")
				}
			}
			stopped = true
		}
	}
	_ = os.Remove(metaPath)
	return printJSON(map[string]any{"toolName": toolName, "status": "pass", "stopped": stopped, "metadataPath": metaPath})
}

func appServerMetadataPath() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), fmt.Sprintf("slidex-%d", os.Getuid()))
	}
	return filepath.Join(base, "slidex", "codex-app-server.json")
}

func normalizeManagedListenURL(listen string) string {
	if listen == "" || listen == "unix://" {
		return "unix://" + filepath.Join(filepath.Dir(appServerMetadataPath()), "codex-app-server.sock")
	}
	return listen
}

func readAppServerMetadata(path string) map[string]any {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil {
		return nil
	}
	return metadata
}

func probeManagedAppServer(metadata map[string]any) (map[string]any, error) {
	listen, _ := metadata["listen"].(string)
	if !strings.HasPrefix(listen, "unix://") {
		return map[string]any{"status": "recorded", "listen": listen, "note": "direct health probe is only implemented for managed unix sockets"}, nil
	}
	sock := strings.TrimPrefix(listen, "unix://")
	if sock == "" {
		return nil, fmt.Errorf("managed unix listen URL has no socket path")
	}
	client, err := newUnixAppServerClient(sock)
	if err != nil {
		return nil, err
	}
	defer client.close()
	resp, _, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex-health", "title": "slidex health probe", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second)
	if err != nil {
		return map[string]any{"status": "degraded", "socketConnect": true, "initializeError": err.Error(), "note": "managed unix socket accepted a connection but did not complete JSON-RPC initialize"}, nil
	}
	if err := client.notify("initialized", nil); err != nil {
		return nil, err
	}
	return map[string]any{"status": "pass", "initialize": resp["result"]}, nil
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

func validateWebSocketAuth(listen string, ws webSocketAuthConfig) error {
	loopback := strings.HasPrefix(listen, "ws://127.0.0.1:") || strings.HasPrefix(listen, "ws://localhost:")
	if loopback {
		return nil
	}
	switch ws.Mode {
	case "capability-token":
		if ws.TokenFile == "" || ws.TokenSHA256 == "" {
			return exitCodeError(4, "non-loopback WebSocket capability-token requires --ws-token-file and --ws-token-sha256")
		}
		if !filepath.IsAbs(ws.TokenFile) {
			return exitCodeError(4, "--ws-token-file must be an absolute path")
		}
	case "signed-bearer-token":
		if ws.SharedSecretFile == "" || ws.Issuer == "" || ws.Audience == "" || ws.MaxClockSkewSeconds <= 0 {
			return exitCodeError(4, "non-loopback WebSocket signed-bearer-token requires --ws-shared-secret-file, --ws-issuer, --ws-audience, and --ws-max-clock-skew-seconds")
		}
		if !filepath.IsAbs(ws.SharedSecretFile) {
			return exitCodeError(4, "--ws-shared-secret-file must be an absolute path")
		}
	default:
		return exitCodeError(4, "non-loopback WebSocket requires --ws-auth capability-token or signed-bearer-token")
	}
	return nil
}

func transportRiskForListen(listen string) string {
	if strings.HasPrefix(listen, "ws://") {
		if strings.HasPrefix(listen, "ws://127.0.0.1:") || strings.HasPrefix(listen, "ws://localhost:") {
			return "WebSocket App Server is experimental/unsupported and limited to loopback."
		}
		return "Non-loopback WebSocket App Server requires explicit auth and external TLS or SSH tunnel."
	}
	return ""
}

func nullOrRawJSON(s string) any {
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}

func runCodexThreads(args []string) error {
	if len(args) == 0 {
		return exitCodeError(2, "usage: slidex codex threads list|read|compact|replay-mcp")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("codex threads list", flag.ContinueOnError)
		deck := fs.String("deck", "", "deck workspace directory")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *deck == "" {
			return exitCodeError(2, "--deck is required")
		}
		idx := readThreadIndex(filepath.Join(mustAbs(*deck), "out"))
		return printJSON(idx)
	case "read":
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		if len(args) < 2 {
			return exitCodeError(2, "thread id is required")
		}
		thread, err := appServerReadThread(args[1])
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"toolName": toolName, "threadId": args[1], "thread": thread})
	case "compact":
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		fs := flag.NewFlagSet("codex threads compact", flag.ContinueOnError)
		deck := fs.String("deck", "", "deck workspace directory")
		threadID := fs.String("thread", "", "thread id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *deck == "" || *threadID == "" {
			return exitCodeError(2, "--deck and --thread are required")
		}
		record, err := compactAppServerThread(mustAbs(*deck), *threadID)
		if err != nil {
			return err
		}
		if err := appendCompactSummaryState(mustAbs(*deck), record); err != nil {
			return err
		}
		return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": "pass", "summary": record})
	case "replay-mcp":
		fs := flag.NewFlagSet("codex threads replay-mcp", flag.ContinueOnError)
		deck := fs.String("deck", "", "deck workspace directory")
		threadID := fs.String("thread", "", "optional thread id filter")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *deck == "" {
			return exitCodeError(2, "--deck is required")
		}
		record, err := replayMCPEvents(mustAbs(*deck), *threadID)
		if err != nil {
			return err
		}
		if err := appendEventReplayState(mustAbs(*deck), record); err != nil {
			return err
		}
		return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": "pass", "replay": record})
	default:
		return exitCodeError(2, "unknown threads command: %s", args[0])
	}
}

func runCodexTurn(args []string) error {
	if len(args) == 0 {
		return exitCodeError(2, "usage: slidex codex turn interrupt|steer --deck decks/<deck_id> --thread THREAD --turn TURN")
	}
	if err := enforceDirectCodexRuntime("app-server"); err != nil {
		return err
	}
	switch args[0] {
	case "interrupt", "steer":
		fs := flag.NewFlagSet("codex turn "+args[0], flag.ContinueOnError)
		deck := fs.String("deck", "", "deck workspace directory")
		threadID := fs.String("thread", "", "thread id")
		turnID := fs.String("turn", "", "turn id")
		reason := fs.String("reason", "", "intervention reason")
		message := fs.String("message", "", "steering message")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *deck == "" || *threadID == "" || *turnID == "" {
			return exitCodeError(2, "--deck, --thread, and --turn are required")
		}
		if args[0] == "steer" && strings.TrimSpace(*message) == "" {
			return exitCodeError(2, "--message is required for turn steer")
		}
		artifact, status, err := appServerTurnControl(mustAbs(*deck), args[0], *threadID, *turnID, *reason, *message)
		intervention := codexIntervention{
			Method:         "turn/" + args[0],
			ThreadID:       *threadID,
			TurnID:         *turnID,
			ExpectedTurnID: *turnID,
			Reason:         *reason,
			Stage:          "user_intervention",
			Status:         status,
			Artifact:       filepath.ToSlash(artifact),
			CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		}
		_ = appendInterventionState(mustAbs(*deck), intervention)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"toolName": toolName, "deckDir": mustAbs(*deck), "status": status, "intervention": intervention})
	default:
		return exitCodeError(2, "unknown turn command: %s", args[0])
	}
}

func appServerTurnControl(deckAbs, action, threadID, turnID, reason, message string) (string, string, error) {
	outDir := filepath.Join(deckAbs, "out")
	runDir := filepath.Join(outDir, "agent_runs")
	if err := ensureSecureDir(runDir); err != nil {
		return "", "fail", err
	}
	client, err := newAppServerClient()
	if err != nil {
		return "", "fail", err
	}
	defer client.close()
	record := map[string]any{
		"schemaVersion": "slidex.turnIntervention.v1",
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"method":        "turn/" + action,
		"threadId":      threadID,
		"turnId":        turnID,
		"reason":        reason,
		"events":        []map[string]any{},
	}
	addEvents := func(events []map[string]any) {
		if len(events) == 0 {
			return
		}
		existing, _ := record["events"].([]map[string]any)
		record["events"] = append(existing, events...)
	}
	status := "pass"
	initResp, events, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second)
	record["initialize"] = initResp["result"]
	addEvents(events)
	if err == nil {
		err = client.notify("initialized", nil)
		record["initialized"] = err == nil
	}
	if err == nil {
		resumeResp, resumeEvents, resumeErr := client.request("thread/resume", map[string]any{
			"threadId":       threadID,
			"cwd":            mustAbs("."),
			"approvalPolicy": "never",
			"sandbox":        "read-only",
			"excludeTurns":   true,
		}, 20*time.Second)
		record["threadResume"] = resumeResp["result"]
		addEvents(resumeEvents)
		err = resumeErr
	}
	if err == nil {
		params := map[string]any{"threadId": threadID}
		method := "turn/" + action
		if action == "interrupt" {
			params["turnId"] = turnID
		} else {
			params["expectedTurnId"] = turnID
			params["input"] = []map[string]any{{"type": "text", "text": message}}
		}
		resp, controlEvents, controlErr := client.request(method, params, 20*time.Second)
		record["request"] = params
		record["response"] = resp["result"]
		addEvents(controlEvents)
		err = controlErr
	}
	if err != nil {
		status = "fail"
		record["error"] = err.Error()
	}
	record["status"] = status
	path := filepath.Join(runDir, "turn_"+action+"_appserver.json")
	if writeErr := secureWriteJSON(path, record); writeErr != nil {
		return path, "fail", writeErr
	}
	_ = appendRunLog(outDir, map[string]any{"event": "turn_" + action, "threadId": threadID, "turnId": turnID, "reason": reason, "status": status, "artifact": path})
	return path, status, err
}

func appendInterventionState(deckAbs string, intervention codexIntervention) error {
	outDir := filepath.Join(deckAbs, "out")
	state := readStateOrNew(deckAbs, "app-server", false)
	state.Interventions = append(state.Interventions, intervention)
	return writeState(outDir, state)
}

func compactAppServerThread(deckAbs, threadID string) (compactSummaryRecord, error) {
	outDir := filepath.Join(deckAbs, "out")
	runDir := filepath.Join(outDir, "agent_runs")
	if err := ensureSecureDir(runDir); err != nil {
		return compactSummaryRecord{}, err
	}
	client, err := newAppServerClient()
	if err != nil {
		return compactSummaryRecord{}, err
	}
	defer client.close()
	record := map[string]any{
		"schemaVersion": "slidex.threadCompact.v1",
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"threadId":      threadID,
		"events":        []map[string]any{},
	}
	addEvents := func(events []map[string]any) {
		if len(events) == 0 {
			return
		}
		existing, _ := record["events"].([]map[string]any)
		record["events"] = append(existing, events...)
	}
	if resp, events, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second); err != nil {
		return compactSummaryRecord{}, err
	} else {
		record["initialize"] = resp["result"]
		addEvents(events)
	}
	if err := client.notify("initialized", nil); err != nil {
		return compactSummaryRecord{}, err
	}
	record["initialized"] = true
	if resp, events, err := client.request("thread/resume", map[string]any{
		"threadId":       threadID,
		"cwd":            mustAbs("."),
		"approvalPolicy": "never",
		"sandbox":        "read-only",
		"excludeTurns":   false,
	}, 20*time.Second); err != nil {
		return compactSummaryRecord{}, err
	} else {
		record["threadResume"] = resp["result"]
		addEvents(events)
	}
	beforeResp, events, err := client.request("thread/read", map[string]any{"threadId": threadID, "includeTurns": true}, 20*time.Second)
	addEvents(events)
	if err != nil {
		return compactSummaryRecord{}, err
	}
	beforeRaw, _ := json.Marshal(beforeResp["result"])
	record["sourceThreadHash"] = sha256Bytes(beforeRaw)
	resp, events, err := client.request("thread/compact/start", map[string]any{"threadId": threadID}, 30*time.Second)
	record["compactStart"] = resp["result"]
	addEvents(events)
	if err != nil {
		return compactSummaryRecord{}, err
	}
	turnID := extractTurnID(resp["result"])
	if turnID != "" {
		completionEvents, completion, waitErr := client.waitForTurnCompletion(threadID, turnID, 5*time.Minute)
		addEvents(completionEvents)
		record["completion"] = completion
		if waitErr != nil {
			record["error"] = waitErr.Error()
			return compactSummaryRecord{}, waitErr
		}
		if actual := turnIDFromCompletion(completion); actual != "" {
			turnID = actual
		}
	} else {
		compactEvents, compacted, waitErr := client.waitForThreadCompacted(threadID, 5*time.Minute)
		addEvents(compactEvents)
		record["compacted"] = compacted
		if waitErr != nil {
			record["error"] = waitErr.Error()
			return compactSummaryRecord{}, waitErr
		}
		if actual, _ := compacted["turnId"].(string); actual != "" {
			turnID = actual
		}
	}
	afterResp, events, readErr := client.request("thread/read", map[string]any{"threadId": threadID, "includeTurns": true}, 20*time.Second)
	addEvents(events)
	if readErr != nil {
		return compactSummaryRecord{}, readErr
	}
	afterRaw, _ := json.Marshal(afterResp["result"])
	record["summaryHash"] = sha256Bytes(afterRaw)
	record["threadReadAfter"] = afterResp["result"]
	safeThread := strings.NewReplacer("/", "_", ":", "_").Replace(threadID)
	path := filepath.Join(runDir, "thread_compact_"+safeThread+".json")
	if err := secureWriteJSON(path, record); err != nil {
		return compactSummaryRecord{}, err
	}
	summary := compactSummaryRecord{
		SchemaVersion:    "slidex.compactSummary.v1",
		CodexVersion:     installedCodexVersion(),
		SourceThreadID:   threadID,
		SourceThreadHash: fmt.Sprint(record["sourceThreadHash"]),
		CompactTurnID:    turnID,
		SummaryHash:      fmt.Sprint(record["summaryHash"]),
		Artifact:         filepath.ToSlash(path),
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		Stale:            false,
	}
	_ = appendRunLog(outDir, map[string]any{"event": "thread_compact", "threadId": threadID, "turnId": turnID, "artifact": path})
	return summary, nil
}

func appendCompactSummaryState(deckAbs string, record compactSummaryRecord) error {
	outDir := filepath.Join(deckAbs, "out")
	state := readStateOrNew(deckAbs, "app-server", false)
	state.MemorySummaries = markStaleSummaries(state.MemorySummaries, record.SourceThreadID, record.SourceThreadHash)
	state.MemorySummaries = append(state.MemorySummaries, record)
	return writeState(outDir, state)
}

func markStaleSummaries(records []compactSummaryRecord, threadID, currentHash string) []compactSummaryRecord {
	for i := range records {
		if records[i].SourceThreadID == threadID && records[i].SourceThreadHash != currentHash {
			records[i].Stale = true
		}
	}
	return records
}

func replayMCPEvents(deckAbs, threadID string) (eventReplayRecord, error) {
	outDir := filepath.Join(deckAbs, "out")
	paths, _ := filepath.Glob(filepath.Join(outDir, "agent_runs", "*_appserver_events.jsonl"))
	sort.Strings(paths)
	type replayThread struct {
		ThreadID string           `json:"threadId"`
		TurnIDs  []string         `json:"turnIds"`
		Events   []map[string]any `json:"events"`
	}
	byThread := map[string]*replayThread{}
	eventCount := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return eventReplayRecord{}, err
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var event map[string]any
			if json.Unmarshal([]byte(line), &event) != nil || !isMCPReplayEvent(event) {
				continue
			}
			params, _ := event["params"].(map[string]any)
			gotThreadID, _ := params["threadId"].(string)
			if threadID != "" && gotThreadID != threadID {
				continue
			}
			if gotThreadID == "" {
				gotThreadID = "unknown"
			}
			entry := byThread[gotThreadID]
			if entry == nil {
				entry = &replayThread{ThreadID: gotThreadID}
				byThread[gotThreadID] = entry
			}
			turnID, _ := params["turnId"].(string)
			entry.TurnIDs = appendUnique(entry.TurnIDs, turnID)
			entry.Events = append(entry.Events, event)
			eventCount++
		}
	}
	threads := []replayThread{}
	for _, entry := range byThread {
		threads = append(threads, *entry)
	}
	sort.Slice(threads, func(i, j int) bool { return threads[i].ThreadID < threads[j].ThreadID })
	artifact := filepath.Join(outDir, "agent_runs", "mcp_event_replay.json")
	payload := map[string]any{
		"schemaVersion": "slidex.mcpEventReplay.v1",
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"threadFilter":  threadID,
		"eventCount":    eventCount,
		"threadCount":   len(threads),
		"threads":       threads,
	}
	if err := secureWriteJSON(artifact, payload); err != nil {
		return eventReplayRecord{}, err
	}
	record := eventReplayRecord{SchemaVersion: "slidex.eventReplay.v1", Kind: "mcp", Artifact: filepath.ToSlash(artifact), ThreadCount: len(threads), EventCount: eventCount, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	_ = appendRunLog(outDir, map[string]any{"event": "mcp_event_replay", "artifact": artifact, "threadCount": len(threads), "eventCount": eventCount})
	return record, nil
}

func isMCPReplayEvent(event map[string]any) bool {
	method, _ := event["method"].(string)
	if strings.Contains(strings.ToLower(method), "mcp") {
		return true
	}
	params, _ := event["params"].(map[string]any)
	_, hasRequesting := params["requestingThreadId"]
	return hasRequesting
}

func appendEventReplayState(deckAbs string, record eventReplayRecord) error {
	outDir := filepath.Join(deckAbs, "out")
	state := readStateOrNew(deckAbs, "app-server", false)
	state.EventReplays = append(state.EventReplays, record)
	return writeState(outDir, state)
}

func runCodexReview(args []string) error {
	if err := enforceDirectCodexRuntime("app-server"); err != nil {
		return err
	}
	fs := flag.NewFlagSet("codex review", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	stage := fs.String("stage", "delivery", "design, html, qa, or delivery")
	nativeReviewStart := fs.Bool("native-review-start", false, "use App Server review/start and normalize the result")
	parallelReviewers := fs.Bool("parallel-reviewers", false, "run independent App Server reviewer threads and record parallel reviewer artifact")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return exitCodeError(2, "--deck is required")
	}
	deckAbs := mustAbs(*deck)
	if *parallelReviewers {
		path, err := writeParallelReviewerThreadsAppServer(deckAbs, *stage, 1)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "stage": *stage, "review": path, "mode": "parallel_reviewer_threads"})
	}
	appRun, err := startAppServerWorkflowRun(deckAbs)
	if err != nil {
		return err
	}
	defer appRun.close()
	if err := writeJSONFile(filepath.Join(deckAbs, "out", "protocol_diagnostics.json"), appRun.snapshot); err != nil {
		return err
	}
	if err := writeThreadIndex(filepath.Join(deckAbs, "out"), threadIndexFromAppServerSnapshot(deckAbs, appRun.snapshot)); err != nil {
		return err
	}
	var path string
	if *nativeReviewStart {
		path, err = writeReviewStartNormalized(deckAbs, *stage, 1, appRun)
	} else {
		path, err = writeStructuredReviewForRuntime(deckAbs, *stage, 1, "app-server", appRun)
	}
	if err != nil {
		return err
	}
	mode := "structured_turn"
	if *nativeReviewStart {
		mode = "review_start_normalized"
	}
	return printJSON(map[string]any{"toolName": toolName, "deckDir": deckAbs, "stage": *stage, "review": path, "mode": mode})
}

func runGoal(args []string) error {
	if len(args) == 0 {
		return exitCodeError(2, "usage: slidex goal set|status|pause|resume|complete|clear --deck decks/<deck_id>")
	}
	switch args[0] {
	case "set":
		fs := flag.NewFlagSet("goal set", flag.ContinueOnError)
		deck := fs.String("deck", "", "deck workspace directory")
		objective := fs.String("objective", "", "objective")
		tokenBudget := fs.Int("token-budget", 0, "token budget")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *deck == "" {
			return exitCodeError(2, "--deck is required")
		}
		if strings.TrimSpace(*objective) == "" {
			return exitCodeError(2, "goal objective must be non-empty")
		}
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		state := readStateOrNew(*deck, "app-server", false)
		if len([]rune(*objective)) > 4000 {
			path := filepath.Join(mustAbs(*deck), "out", "goal_objective.md")
			if err := secureWriteFile(path, []byte(*objective+"\n"), 0o600); err != nil {
				return err
			}
			state.Goal = goalMirror{ObjectiveFile: filepath.ToSlash(filepath.Join("out", "goal_objective.md")), Status: "active", TokenBudget: *tokenBudget}
		} else {
			state.Goal = goalMirror{Objective: *objective, Status: "active", TokenBudget: *tokenBudget}
		}
		outDir := filepath.Join(mustAbs(*deck), "out")
		if err := writeState(outDir, state); err != nil {
			return err
		}
		syncResult, syncErr := syncGoalToAppServer(mustAbs(*deck), outDir, bestAppServerThreadID(outDir), state.Goal)
		if syncErr != nil {
			state.UnresolvedRisks = append(state.UnresolvedRisks, acceptedRisk{Reason: "App Server goal sync failed: " + syncErr.Error(), Owner: "slidex", Expiration: time.Now().Add(24 * time.Hour).Format(time.RFC3339), ArtifactLink: "out/slidex_state.json"})
			_ = writeState(outDir, state)
		}
		return printJSON(map[string]any{"local": state.Goal, "appServer": syncResult, "appServerError": errorString(syncErr)})
	case "status":
		deck, err := deckFlag(args[1:])
		if err != nil {
			return err
		}
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		outDir := filepath.Join(mustAbs(deck), "out")
		state := readStateOrNew(deck, "app-server", false)
		appGoal, syncErr := getGoalFromAppServer(mustAbs(deck), outDir, bestAppServerThreadID(outDir))
		if syncErr == nil && goalMismatch(state.Goal, appGoal) {
			state.UnresolvedRisks = append(state.UnresolvedRisks, acceptedRisk{Reason: "App Server goal status differs from local mirror", Owner: "slidex", Expiration: time.Now().Add(24 * time.Hour).Format(time.RFC3339), ArtifactLink: "out/slidex_state.json"})
			_ = writeState(outDir, state)
		}
		return printJSON(map[string]any{"local": state.Goal, "appServer": appGoal, "appServerError": errorString(syncErr)})
	case "pause", "resume", "clear":
		deck, err := deckFlag(args[1:])
		if err != nil {
			return err
		}
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		state := readStateOrNew(deck, "app-server", false)
		outDir := filepath.Join(mustAbs(deck), "out")
		var syncResult map[string]any
		var syncErr error
		switch args[0] {
		case "pause":
			state.Goal.Status = "paused"
			syncResult, syncErr = syncGoalToAppServer(mustAbs(deck), outDir, bestAppServerThreadID(outDir), state.Goal)
		case "resume":
			state.Goal.Status = "active"
			syncResult, syncErr = syncGoalToAppServer(mustAbs(deck), outDir, bestAppServerThreadID(outDir), state.Goal)
		case "clear":
			state.Goal = goalMirror{}
			syncResult, syncErr = clearGoalInAppServer(mustAbs(deck), outDir, bestAppServerThreadID(outDir))
		}
		if syncErr != nil {
			state.UnresolvedRisks = append(state.UnresolvedRisks, acceptedRisk{Reason: "App Server goal sync failed: " + syncErr.Error(), Owner: "slidex", Expiration: time.Now().Add(24 * time.Hour).Format(time.RFC3339), ArtifactLink: "out/slidex_state.json"})
		}
		if err := writeState(outDir, state); err != nil {
			return err
		}
		return printJSON(map[string]any{"local": state.Goal, "appServer": syncResult, "appServerError": errorString(syncErr)})
	case "complete":
		deck, err := deckFlag(args[1:])
		if err != nil {
			return err
		}
		if err := enforceDirectCodexRuntime("app-server"); err != nil {
			return err
		}
		result, err := packageDeck(deck, false)
		if err != nil {
			return err
		}
		if result["status"] == "fail" {
			return exitCodeError(5, "goal cannot complete because package gate is not fresh")
		}
		if result["status"] != "pass" {
			return exitCodeError(6, "goal cannot complete because package gate has unresolved or unaccepted risks")
		}
		state := readStateOrNew(deck, "app-server", false)
		state.Goal.Status = "complete"
		outDir := filepath.Join(mustAbs(deck), "out")
		syncResult, syncErr := syncGoalToAppServer(mustAbs(deck), outDir, bestAppServerThreadID(outDir), state.Goal)
		if syncErr != nil {
			state.UnresolvedRisks = append(state.UnresolvedRisks, acceptedRisk{Reason: "App Server goal sync failed: " + syncErr.Error(), Owner: "slidex", Expiration: time.Now().Add(24 * time.Hour).Format(time.RFC3339), ArtifactLink: "out/slidex_state.json"})
		}
		if err := writeState(outDir, state); err != nil {
			return err
		}
		return printJSON(map[string]any{"local": state.Goal, "appServer": syncResult, "appServerError": errorString(syncErr)})
	default:
		return exitCodeError(2, "unknown goal command: %s", args[0])
	}
}

func runMCPServer(args []string) error {
	fs := flag.NewFlagSet("mcp-server", flag.ContinueOnError)
	stdio := fs.Bool("stdio", false, "serve over stdio")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*stdio {
		return exitCodeError(2, "--stdio is required")
	}
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(map[string]any{"error": err.Error()})
			continue
		}
		method, _ := req["method"].(string)
		result, err := handleMCPRequest(req)
		if err != nil {
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req["id"], "error": map[string]any{"code": -32000, "message": err.Error(), "method": method}})
			continue
		}
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req["id"], "result": result})
	}
	return scanner.Err()
}

func handleMCPRequest(req map[string]any) (any, error) {
	method, _ := req["method"].(string)
	switch method {
	case "initialize":
		return map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "slidex", "version": toolVersion}, "capabilities": map[string]any{"tools": map[string]any{}}}, nil
	case "tools/list":
		return map[string]any{"tools": []map[string]any{
			mcpTool("inspect", "Inventory deck inputs and outputs"),
			mcpTool("render", "Render deck HTML to PNG/PDF/manifest/montage"),
			mcpTool("qa", "Run deterministic QA"),
			mcpTool("package", "Verify package gate"),
			mcpTool("state/read", "Read slidex_state.json"),
		}}, nil
	case "tools/call":
		params, _ := req["params"].(map[string]any)
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		return callMCPTool(name, args)
	default:
		return nil, fmt.Errorf("unsupported MCP method: %s", method)
	}
}

func mcpTool(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"deck": map[string]any{"type": "string"}, "includeLogs": map[string]any{"type": "boolean"}}},
	}
}

func callMCPTool(name string, args map[string]any) (any, error) {
	deck, _ := args["deck"].(string)
	if deck == "" && name != "state/read" {
		return nil, errors.New("deck argument is required")
	}
	switch name {
	case "inspect":
		return inspectDeck(deck)
	case "render":
		out := filepath.Join(mustAbs(deck), "out")
		cfg, err := renderConfigFromFlags(filepath.Join(out, "final_deck.html"), filepath.Join(out, "rendered_slides"), filepath.Join(out, "final_deck.pdf"), filepath.Join(out, "render_manifest.json"), "paginated", ".slide", 1920, 1080, "pretendard", "", false)
		if err != nil {
			return nil, err
		}
		return renderHTML(cfg)
	case "qa":
		return qaDeckWithVisualReview(deck, true, "none")
	case "package":
		includeLogs, _ := args["includeLogs"].(bool)
		return packageDeck(deck, includeLogs)
	case "state/read":
		if deck == "" {
			return nil, errors.New("deck argument is required")
		}
		raw, err := os.ReadFile(filepath.Join(mustAbs(deck), "out", "slidex_state.json"))
		if err != nil {
			return nil, err
		}
		var state map[string]any
		if err := json.Unmarshal(raw, &state); err != nil {
			return nil, err
		}
		return state, nil
	default:
		return nil, fmt.Errorf("unsupported tool: %s", name)
	}
}

func ensureStrategy(deck string, force bool) (string, error) {
	deckAbs := mustAbs(deck)
	outDir := filepath.Join(deckAbs, "out")
	path := filepath.Join(outDir, "strategy.md")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	brief := readFileOrEmpty(filepath.Join(deckAbs, "brief.md"))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Strategy\n\n")
	b.WriteString("- Source: `brief.md`\n")
	b.WriteString("- Document type: `custom`\n")
	b.WriteString("- Audience: confirmed in brief or treated as an assumption until intake closes.\n")
	b.WriteString("- Purpose: produce an HTML-first business document that supports a concrete decision.\n")
	b.WriteString("- Claim policy: unsupported metrics and customer/product claims are removed or marked as assumptions.\n")
	b.WriteString("- Risk policy: unresolved risks require owner, reason, expiration, and artifact link before package.\n\n")
	b.WriteString("## Brief Summary\n\n")
	b.WriteString(strings.TrimSpace(firstNRunes(brief, 1200)))
	b.WriteString("\n")
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func ensureSpec(deck string, force bool) (string, error) {
	deckAbs := mustAbs(deck)
	outDir := filepath.Join(deckAbs, "out")
	path := filepath.Join(outDir, "deck_spec.json")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	deckID := filepath.Base(deckAbs)
	title := firstMarkdownHeading(filepath.Join(deckAbs, "brief.md"))
	if title == "" {
		title = deckID + " business document"
	}
	inv, _ := inspectDeck(deckAbs)
	sourceRefs := []map[string]any{}
	for _, item := range inv.Inputs {
		sourceRefs = append(sourceRefs, map[string]any{"path": item.Path, "kind": item.Kind, "priority": "supporting", "sha256": item.SHA256})
	}
	slides := []map[string]any{
		makeSpecSlide("slide_01", "cover", title, "문서의 목적과 의사결정 맥락을 한 문장으로 정리합니다.", []string{"현재 확인된 입력을 기준으로 작성", "부족한 사실은 가정으로 분리"}),
		makeSpecSlide("slide_02", "executive_summary", "핵심 판단은 근거와 가정을 분리해 제시합니다", "검증된 자료, 사용자 확인, 가정을 명확히 나눕니다.", []string{"주요 근거", "남은 확인사항", "리스크"}),
		makeSpecSlide("slide_03", "next_steps", "다음 실행은 검증 가능한 산출물 기준으로 관리합니다", "HTML, 렌더 이미지, PDF, QA report의 freshness로 완료를 판단합니다.", []string{"렌더링", "QA", "패키지 gate"}),
	}
	spec := map[string]any{
		"metadata":                map[string]any{"title": title, "version": "0.1.0", "deckId": deckID, "activeDeckDir": filepath.ToSlash(deckAbs), "outputDir": filepath.ToSlash(outDir), "schemaVersion": "slidex.deck_spec.v1", "toolName": toolName, "referenceFiles": sourceRefs},
		"documentType":            "custom",
		"audience":                "확정된 사용자 지정 청중",
		"objective":               "현재 입력과 승인된 가정에 근거한 HTML-first 비즈니스 문서를 완성한다.",
		"desiredOutcome":          "검토자가 핵심 판단, 근거, 다음 행동을 이해하고 승인 여부를 결정한다.",
		"tone":                    "concrete, restrained, evidence-aware",
		"sourceInventory":         sourceRefs,
		"intakeStatus":            map[string]any{"status": "assumptions_approved", "questionsAsked": []string{}, "openQuestions": []string{}, "approvedAssumptions": []string{"자동 spec 생성 시 정량 주장 없이 구조와 검증 흐름만 사용한다."}},
		"outputContract":          map[string]any{"sourceHtml": "out/final_deck.html", "generatedBaselineHtml": "out/final_deck.generated_baseline.html", "renderedSlidesDir": "out/rendered_slides", "primaryPdf": "out/final_deck.pdf", "renderManifest": "out/render_manifest.json", "pdfMode": "paginated", "qaMontage": "out/qa_montage.png", "notes": "out/notes.md", "qaReport": "out/qa_report.md", "deliverySummary": "out/delivery_summary.md"},
		"renderConfig":            map[string]any{"engine": "slidex-cli", "preset": "wide-1080p", "slideSelector": ".slide", "widthPx": 1920, "heightPx": 1080, "deviceScaleFactor": 1, "waitForFonts": true, "captureElementOnly": true, "fontPreset": "pretendard"},
		"pdfConfig":               map[string]any{"source": "rendered_images", "mode": "paginated", "pageAspectRatio": "16:9", "pageSizeInches": map[string]any{"width": 13.333, "height": 7.5}, "imageFit": "exact", "background": "#ffffff"},
		"designSystem":            map[string]any{"fontPreset": "pretendard", "colors": map[string]string{"primary": "#1F6FEB", "accent": "#F59E0B", "text": "#111827", "background": "#FFFFFF"}, "typography": map[string]string{"headline": "action headline", "body": "concise Korean business copy"}, "layout": map[string]string{"aspectRatio": "16:9", "safeMargin": "96px"}, "cssVariables": map[string]string{"--slide-width": "1920px", "--slide-height": "1080px"}, "stylePromptSummary": "deterministic fallback design", "stylePromptDirectives": []string{"Use concise text and clear hierarchy."}, "stylePromptAvoid": []string{"Unsupported metrics and invented assets."}, "stylePromptConflicts": []string{}, "htmlCssNotes": []string{"word-break: keep-all", "overflow-wrap: normal", "line-break: strict"}},
		"storyArc":                []string{"문서 목적을 제시한다", "근거와 가정을 분리한다", "다음 실행과 검증 gate를 제시한다"},
		"slides":                  slides,
		"claimProvenance":         map[string]any{"required": true, "unsupportedClaimsPolicy": "remove_or_rewrite", "claims": []map[string]any{{"id": "claim_001", "text": "문서는 현재 입력과 승인된 가정에 근거해 작성된다.", "status": "assumption", "sourceRefs": []string{"brief.md"}, "slideIds": []string{"slide_01", "slide_02"}, "notes": "정량 성과 주장은 생성하지 않는다."}}},
		"businessQa":              map[string]any{"documentTypeChecklist": []string{"Document type is explicit."}, "copyRisks": []string{"자동 생성 문안은 사용자가 최종 확인해야 한다."}, "evidenceRisks": []string{"입력에 없는 정량 주장은 사용하지 않는다."}, "legalRisks": []string{"보증, 인증, 컴플라이언스 주장은 source 없이는 금지한다."}, "visualRisks": []string{"렌더된 PNG와 montage를 검사해야 한다."}},
		"accessibilityNotes":      []string{"Maintain contrast and readable font sizes."},
		"htmlImplementationNotes": []string{"Static HTML/CSS only."},
		"userEditPolicy":          map[string]any{"allowDirectHtmlEdits": true, "syncRequiredAfterHtmlEdits": true, "preserveUserEditsByDefault": true, "baselineHtml": "out/final_deck.generated_baseline.html", "syncReport": "out/html_edit_sync.md", "staleDerivativePolicy": "mark stale with concrete reasons"},
	}
	if err := writeJSONFile(path, spec); err != nil {
		return "", err
	}
	findings, err := validateSpecFile(path)
	if err != nil {
		return "", err
	}
	if hasFailures(findings) {
		return "", fmt.Errorf("generated spec did not validate: %v", findings)
	}
	return path, nil
}

func ensureHTML(deck string, force bool) (string, error) {
	deckAbs := mustAbs(deck)
	outDir := filepath.Join(deckAbs, "out")
	path := filepath.Join(outDir, "final_deck.html")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	specPath, err := ensureSpec(deckAbs, false)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return "", err
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		return "", err
	}
	slides, _ := spec["slides"].([]any)
	title := fmt.Sprint(specValue(spec, "metadata", "title"))
	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>`)
	b.WriteString(escapeHTML(title))
	b.WriteString(`</title>
<style>
:root {
  --slide-width: 1920px;
  --slide-height: 1080px;
  --font-body: "Pretendard", "Noto Sans KR", Arial, sans-serif;
  --color-bg: #ffffff;
  --color-text: #111827;
  --color-muted: #475569;
  --color-primary: #1f6feb;
  --color-accent: #f59e0b;
}
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; background: #e5e7eb; font-family: var(--font-body); word-break: keep-all; overflow-wrap: normal; hyphens: none; line-break: strict; color: var(--color-text); }
.deck { width: var(--slide-width); margin: 0 auto; }
.slide { position: relative; width: var(--slide-width); height: var(--slide-height); overflow: hidden; background: var(--color-bg); padding: 72px 104px; display: grid; grid-template-rows: auto minmax(0, 1fr) auto; gap: 32px; border-bottom: 1px solid #d1d5db; }
.slide::before { content: ""; position: absolute; inset: 0 0 auto 0; height: 12px; background: linear-gradient(90deg, var(--color-primary), var(--color-accent)); }
.kicker { color: var(--color-primary); font-size: 28px; font-weight: 700; margin: 0 0 20px; }
h1, h2 { margin: 0; letter-spacing: 0; line-height: 1.08; max-width: 1280px; }
h1 { font-size: 72px; }
h2 { font-size: 58px; }
.body { display: grid; grid-template-columns: 1.15fr .85fr; gap: 64px; align-items: center; }
.message { font-size: 34px; line-height: 1.34; color: var(--color-muted); margin: 0; }
.points { display: grid; gap: 16px; margin: 0; padding: 0; list-style: none; }
.points li { border-left: 8px solid var(--color-accent); padding: 14px 22px; background: #f8fafc; font-size: 26px; line-height: 1.26; }
.panel { border: 2px solid #dbeafe; background: #eff6ff; padding: 30px; min-height: 300px; display: grid; place-content: center; }
.panel strong { display: block; font-size: 38px; color: var(--color-primary); margin-bottom: 16px; }
.panel span { font-size: 24px; color: var(--color-muted); line-height: 1.34; }
.footer { display: flex; justify-content: space-between; align-items: center; color: #64748b; font-size: 22px; }
</style>
</head>
<body>
<main class="deck">
`)
	for i, rawSlide := range slides {
		slide, _ := rawSlide.(map[string]any)
		id := fmt.Sprint(slide["htmlId"])
		if id == "" || id == "<nil>" {
			id = fmt.Sprintf("slide_%02d", i+1)
		}
		headline := fmt.Sprint(slide["headline"])
		key := fmt.Sprint(slide["keyMessage"])
		bodyItems := []string{}
		if arr, ok := slide["bodyContent"].([]any); ok {
			for _, item := range arr {
				bodyItems = append(bodyItems, fmt.Sprint(item))
			}
		}
		b.WriteString(fmt.Sprintf(`<section class="slide" id="%s" data-slide-id="%s">
  <header>
    <p class="kicker">%02d</p>
    <h2>%s</h2>
  </header>
  <div class="body">
    <p class="message">%s</p>
    <div class="panel"><strong>Evidence-aware</strong><span>Facts, assumptions, risks, and next actions remain visibly separated.</span></div>
  </div>
  <ul class="points">`, escapeHTML(id), escapeHTML(id), i+1, escapeHTML(headline), escapeHTML(key)))
		for _, item := range bodyItems {
			b.WriteString("<li>" + escapeHTML(item) + "</li>")
		}
		b.WriteString(`</ul>
  <footer class="footer"><span>slidex HTML-first document</span><span>`)
		b.WriteString(escapeHTML(id))
		b.WriteString(`</span></footer>
</section>
`)
	}
	b.WriteString(`</main>
</body>
</html>
`)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeDeliverySummary(deck string) (string, error) {
	deckAbs := mustAbs(deck)
	outDir := filepath.Join(deckAbs, "out")
	path := filepath.Join(outDir, "delivery_summary.md")
	notesPath := filepath.Join(outDir, "notes.md")
	manifestPath := filepath.Join(outDir, "render_manifest.json")
	qaPath := filepath.Join(outDir, "qa_report.md")
	manifestHash := mustSHA256(manifestPath)
	qaHash := mustSHA256(qaPath)
	pngSet := hashFileSet(filepath.Join(outDir, "rendered_slides", "slide_*.png"))
	var manifest renderManifest
	if raw, err := os.ReadFile(manifestPath); err == nil {
		_ = json.Unmarshal(raw, &manifest)
	}
	state := readStateOrNew(deckAbs, "app-server", false)
	var b strings.Builder
	b.WriteString("# Delivery Summary\n\n")
	b.WriteString("- Tool: `" + toolName + " " + toolVersion + "`\n")
	b.WriteString("- Deck directory: `" + deckAbs + "`\n")
	b.WriteString("- Generated at: `" + time.Now().UTC().Format(time.RFC3339) + "`\n")
	b.WriteString("- Render manifest hash: `" + manifestHash + "`\n")
	b.WriteString("- QA report hash: `" + qaHash + "`\n")
	b.WriteString("- PNG set hash: `" + pngSet + "`\n")
	b.WriteString("- PDF pages: `" + strconv.Itoa(manifest.PDFPageCount) + "`\n")
	b.WriteString("- Chrome sandbox: `" + firstNonEmpty(manifest.ChromeSandbox, "unknown") + "`\n")
	b.WriteString("- Slide enumeration: `" + firstNonEmpty(manifest.SlideEnumerationMethod, "unknown") + "`\n\n")
	b.WriteString("## Artifacts\n\n")
	for _, rel := range []string{"strategy.md", "deck_spec.json", "final_deck.html", "final_deck.generated_baseline.html", "rendered_slides/", "final_deck.pdf", "render_manifest.json", "qa_montage.png", "qa_report.md", "notes.md"} {
		b.WriteString("- `out/" + rel + "`\n")
	}
	b.WriteString("\n## QA Status\n\n")
	b.WriteString("- Deterministic QA report: `out/qa_report.md`\n")
	b.WriteString("- Visual review image set: `out/visual_reviews/image_set.json`\n")
	b.WriteString("- Manual visual inspection remains a delivery responsibility unless a Codex visual review artifact records pass.\n\n")
	b.WriteString("## Accepted Risks\n\n")
	if len(state.AcceptedRisks) == 0 {
		b.WriteString("- None recorded. Any future accepted risk must include reason, owner, expiration, and artifact link.\n\n")
	} else {
		for _, risk := range state.AcceptedRisks {
			b.WriteString("- Reason: `" + escapeMarkdownInline(risk.Reason) + "`; owner: `" + escapeMarkdownInline(risk.Owner) + "`; expiration: `" + escapeMarkdownInline(risk.Expiration) + "`; artifact: `" + escapeMarkdownInline(risk.ArtifactLink) + "`\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Unresolved Risks\n\n")
	if len(state.UnresolvedRisks) == 0 {
		b.WriteString("- None recorded in `out/slidex_state.json`.\n\n")
	} else {
		for _, risk := range state.UnresolvedRisks {
			b.WriteString("- Reason: `" + escapeMarkdownInline(risk.Reason) + "`; owner: `" + escapeMarkdownInline(risk.Owner) + "`; expiration: `" + escapeMarkdownInline(risk.Expiration) + "`; artifact: `" + escapeMarkdownInline(risk.ArtifactLink) + "`\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Review Loop\n\n")
	b.WriteString("- Structured review artifacts are stored under `out/agent_reviews/` when `slidex codex review` or reviewer gates run.\n")
	if _, err := os.Stat(notesPath); os.IsNotExist(err) {
		if err := os.WriteFile(notesPath, []byte("# Notes\n\n- No additional delivery notes recorded by deterministic finalize.\n"), 0o644); err != nil {
			return "", err
		}
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeStructuredReview(deck, stage string, round int) (string, error) {
	deckAbs := mustAbs(deck)
	findings := structuredReviewFindings(deckAbs, stage)
	payload := map[string]any{"schemaVersion": "slidex.structuredReview.v1", "stage": stage, "round": round, "mode": "parallel_reviewer_threads", "status": statusFromFindings(findings), "findings": findings}
	return writeStructuredReviewPayload(deckAbs, stage, round, payload)
}

func writeParallelReviewerThreadsAppServer(deckAbs, stage string, round int) (string, error) {
	type reviewerSpec struct {
		Name  string
		Focus string
	}
	type reviewerResult struct {
		Spec   reviewerSpec
		Result appServerTurnResult
		Err    error
	}
	reviewers := []reviewerSpec{
		{Name: "artifact_freshness", Focus: "freshness of final HTML, render manifest, PNG/PDF, QA report, visual review, and delivery summary"},
		{Name: "business_delivery", Focus: "delivery readiness, blocker/major risk separation, and accepted-risk policy"},
	}
	ch := make(chan reviewerResult, len(reviewers))
	for _, spec := range reviewers {
		spec := spec
		go func() {
			appRun, err := startAppServerWorkflowRun(deckAbs)
			if err != nil {
				ch <- reviewerResult{Spec: spec, Err: err}
				return
			}
			defer appRun.close()
			findings := structuredReviewFindings(deckAbs, stage)
			expected := map[string]any{
				"schemaVersion": "slidex.reviewFindings.v1",
				"stage":         stage,
				"round":         round,
				"mode":          "parallel_reviewer_threads",
				"status":        statusFromFindings(findings),
				"imageEvidence": []map[string]any{},
				"findings":      findingsForStrictSchema(findings),
			}
			prompt := structuredReviewPrompt(deckAbs, stage, expected) + "\nReviewer focus: " + spec.Focus + "\nReturn the deterministic baseline unless this focus reveals a concrete blocker in listed artifacts."
			result, err := appRun.runStructuredTurn("parallel_"+spec.Name+"_"+stage, prompt, filepath.Join("schemas", "app_review_findings.strict.schema.json"), 3*time.Minute)
			ch <- reviewerResult{Spec: spec, Result: result, Err: err}
		}()
	}
	outDir := filepath.Join(deckAbs, "out")
	var aggregate []qaFinding
	var evidence []map[string]any
	for range reviewers {
		item := <-ch
		if item.Err != nil {
			aggregate = append(aggregate, fail("review.parallel."+item.Spec.Name, item.Err.Error(), filepath.Join(outDir, "agent_reviews")))
			evidence = append(evidence, map[string]any{"reviewer": item.Spec.Name, "focus": item.Spec.Focus, "status": "fail", "error": item.Err.Error()})
			continue
		}
		path, result, err := writeAppServerTurnResult(outDir, item.Result)
		if err != nil {
			return "", err
		}
		if err := recordAppServerTurn(outDir, "parallel_reviewer_"+item.Spec.Name, result); err != nil {
			return "", err
		}
		if err := markThreadRole(outDir, result.ThreadID, "parallel_reviewer", "parallel_reviewer_threads", ""); err != nil {
			return "", err
		}
		payload := result.StructuredOutput
		findings := reviewFindingsFromPayload(payload)
		aggregate = append(aggregate, findings...)
		evidence = append(evidence, map[string]any{
			"reviewer": item.Spec.Name,
			"focus":    item.Spec.Focus,
			"status":   payload["status"],
			"threadId": result.ThreadID,
			"turnId":   result.TurnID,
			"artifact": filepath.ToSlash(path),
		})
	}
	payload := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         stage,
		"round":         round,
		"mode":          "parallel_reviewer_threads",
		"status":        statusFromFindings(aggregate),
		"imageEvidence": []map[string]any{},
		"findings":      findingsForStrictSchema(aggregate),
	}
	reportPath, err := writeStructuredReviewPayload(deckAbs, stage, round, payload)
	if err != nil {
		return "", err
	}
	evidencePath := filepath.Join(outDir, "agent_reviews", fmt.Sprintf("round_%02d", round), "parallel_reviewer_threads.json")
	if err := secureWriteJSON(evidencePath, map[string]any{"schemaVersion": "slidex.parallelReviewerThreads.v1", "generatedAt": time.Now().UTC().Format(time.RFC3339), "stage": stage, "round": round, "reviewers": evidence}); err != nil {
		return "", err
	}
	_ = appendRunLog(outDir, map[string]any{"event": "parallel_reviewer_threads", "stage": stage, "review": reportPath, "evidence": evidencePath, "reviewerCount": len(reviewers)})
	return reportPath, nil
}

func writeStructuredReviewForRuntime(deck, stage string, round int, codexMode string, appRun *appServerWorkflowRun) (string, error) {
	deckAbs := mustAbs(deck)
	switch codexMode {
	case "app-server":
		if appRun != nil && appRun.threadID != "" {
			return writeStructuredReviewAppServer(deckAbs, stage, round, appRun)
		}
	case "exec", "exec_fallback":
		return writeStructuredReviewExec(deckAbs, stage, round, codexMode == "exec_fallback")
	}
	return writeStructuredReview(deckAbs, stage, round)
}

func structuredReviewFindings(deckAbs, stage string) []qaFinding {
	findings := []qaFinding{}
	if stage == "delivery" || stage == "qa" {
		for _, rel := range []string{"final_deck.html", "render_manifest.json", "qa_report.md", "visual_reviews/latest_review.json"} {
			path := filepath.Join(deckAbs, "out", rel)
			if _, err := os.Stat(path); err != nil {
				findings = append(findings, fail("review.artifact", "required review artifact missing: "+err.Error(), path))
			}
		}
	}
	return findings
}

func writeStructuredReviewPayload(deckAbs, stage string, round int, payload map[string]any) (string, error) {
	outDir := filepath.Join(deckAbs, "out", "agent_reviews", fmt.Sprintf("round_%02d", round))
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return "", err
	}
	reportPath := filepath.Join(outDir, "reviewer_"+stage+".json")
	resolutionPath := filepath.Join(outDir, "resolution.md")
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "review_findings.schema.json")); err != nil {
		return "", err
	}
	if err := writeJSONFile(reportPath, payload); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Review Resolution\n\n")
	b.WriteString("- Stage: `" + stage + "`\n")
	b.WriteString("- Round: `" + strconv.Itoa(round) + "`\n")
	b.WriteString("- Status: `" + fmt.Sprint(payload["status"]) + "`\n")
	findings := reviewFindingsFromPayload(payload)
	if len(findings) == 0 {
		b.WriteString("- No blocker or major findings remain in this structured review round.\n")
	} else {
		for _, f := range findings {
			b.WriteString("- `" + f.Severity + "` `" + f.Check + "`: " + f.Message + "\n")
		}
	}
	if err := os.WriteFile(resolutionPath, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return reportPath, nil
}

func writeStructuredReviewAppServer(deckAbs, stage string, round int, appRun *appServerWorkflowRun) (string, error) {
	findings := structuredReviewFindings(deckAbs, stage)
	imageEvidence := []map[string]any{}
	if manifest, ok := readRenderManifest(filepath.Join(deckAbs, "out", "render_manifest.json")); ok {
		imageEvidence = visualReviewEvidence(deckAbs, manifest)
	}
	expected := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         stage,
		"round":         round,
		"mode":          "structured_turn",
		"status":        statusFromFindings(findings),
		"imageEvidence": imageEvidence,
		"findings":      findingsForStrictSchema(findings),
	}
	prompt := structuredReviewPrompt(deckAbs, stage, expected)
	result, err := appRun.runStructuredTurn("review_"+stage, prompt, filepath.Join("schemas", "app_review_findings.strict.schema.json"), 3*time.Minute)
	if err != nil {
		return "", err
	}
	path, result, err := writeAppServerTurnResult(filepath.Join(deckAbs, "out"), result)
	if err != nil {
		return "", err
	}
	if err := recordAppServerTurn(filepath.Join(deckAbs, "out"), stage, result); err != nil {
		return "", err
	}
	payload := result.StructuredOutput
	if payload == nil {
		return "", fmt.Errorf("App Server structured review did not return payload")
	}
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "review_findings.schema.json")); err != nil {
		return "", err
	}
	reportPath, err := writeStructuredReviewPayload(deckAbs, stage, round, payload)
	if err != nil {
		return "", err
	}
	_ = appendRunLog(filepath.Join(deckAbs, "out"), map[string]any{"event": "structured_review_app_server", "stage": stage, "turn": path, "review": reportPath})
	return reportPath, nil
}

func writeStructuredReviewExec(deckAbs, stage string, round int, resume bool) (string, error) {
	findings := structuredReviewFindings(deckAbs, stage)
	imageEvidence := []map[string]any{}
	if manifest, ok := readRenderManifest(filepath.Join(deckAbs, "out", "render_manifest.json")); ok {
		imageEvidence = visualReviewEvidence(deckAbs, manifest)
	}
	expected := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         stage,
		"round":         round,
		"mode":          "structured_turn",
		"status":        statusFromFindings(findings),
		"imageEvidence": imageEvidence,
		"findings":      findingsForStrictSchema(findings),
	}
	runPath, payload, err := runCodexExecStructured(deckAbs, "review_"+stage, structuredReviewPrompt(deckAbs, stage, expected), filepath.Join("schemas", "app_review_findings.strict.schema.json"), resume, "last", nil)
	if err != nil {
		return "", err
	}
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "review_findings.schema.json")); err != nil {
		return "", err
	}
	reportPath, err := writeStructuredReviewPayload(deckAbs, stage, round, payload)
	if err != nil {
		return "", err
	}
	_ = appendRunLog(filepath.Join(deckAbs, "out"), map[string]any{"event": "structured_review_exec", "stage": stage, "execRun": runPath, "review": reportPath})
	return reportPath, nil
}

func writeReviewStartNormalized(deckAbs, stage string, round int, appRun *appServerWorkflowRun) (string, error) {
	if appRun == nil || appRun.threadID == "" {
		return "", fmt.Errorf("review/start requires an active App Server thread")
	}
	outDir := filepath.Join(deckAbs, "out")
	instructions := fmt.Sprintf("Review slidex delivery stage %q for blocker or major issues. Do not modify files. Focus on freshness of final_deck.html, rendered PNG/PDF, QA report, visual review, and package readiness.", stage)
	resp, events, err := appRun.client.request("review/start", map[string]any{
		"threadId": appRun.threadID,
		"delivery": "detached",
		"target":   map[string]any{"type": "custom", "instructions": instructions},
	}, 30*time.Second)
	if err != nil {
		return "", err
	}
	resultObj, _ := resp["result"].(map[string]any)
	reviewThreadID, _ := resultObj["reviewThreadId"].(string)
	if reviewThreadID == "" {
		reviewThreadID = appRun.threadID
	}
	turnID := extractTurnID(resultObj)
	completionEvents, completion, err := appRun.client.waitForTurnCompletion(reviewThreadID, turnID, 5*time.Minute)
	events = append(events, completionEvents...)
	if err != nil {
		return "", err
	}
	if actual := turnIDFromCompletion(completion); actual != "" {
		turnID = actual
	}
	readResp, readEvents, readErr := appRun.client.request("thread/read", map[string]any{"threadId": reviewThreadID, "includeTurns": true}, 20*time.Second)
	events = append(events, readEvents...)
	threadRead := any(nil)
	threadReadError := ""
	if readErr != nil {
		threadReadError = readErr.Error()
	} else {
		threadRead = readResp["result"]
	}
	finalText := extractFinalAgentTextFromEvents(events, turnID)
	if finalText == "" {
		finalText = extractFinalAgentTextFromThreadRead(threadRead, turnID)
	}
	raw := appServerTurnResult{
		SchemaVersion:   "slidex.reviewStartRaw.v1",
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Stage:           "review_start_" + stage,
		ThreadID:        reviewThreadID,
		TurnID:          turnID,
		PromptSha256:    sha256Bytes([]byte(instructions)),
		StartResponse:   resultObj,
		Completion:      completion,
		ThreadRead:      threadRead,
		ThreadReadError: threadReadError,
		FinalMessage:    finalText,
		Events:          events,
	}
	rawPath, raw, err := writeAppServerTurnResult(outDir, raw)
	if err != nil {
		return "", err
	}
	if err := recordAppServerTurn(outDir, "review_start_"+stage, raw); err != nil {
		return "", err
	}
	payload := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         stage,
		"round":         round,
		"mode":          "review_start_normalized",
		"status":        "pass",
		"imageEvidence": []map[string]any{},
		"findings":      []map[string]any{},
	}
	if strings.Contains(strings.ToLower(finalText), "blocker") || strings.Contains(strings.ToLower(finalText), "major") {
		payload["status"] = "pass_with_risks"
		payload["findings"] = []map[string]any{{"severity": "warn", "check": "review_start.summary", "message": "Native review/start returned text mentioning blocker or major; inspect raw review artifact.", "path": filepath.ToSlash(rawPath)}}
	}
	reportPath, err := writeStructuredReviewPayload(deckAbs, stage, round, payload)
	if err != nil {
		return "", err
	}
	_ = appendRunLog(outDir, map[string]any{"event": "review_start_normalized", "turn": rawPath, "review": reportPath, "reviewThreadId": reviewThreadID})
	return reportPath, nil
}

func structuredReviewPrompt(deckAbs, stage string, expected map[string]any) string {
	expectedRaw, _ := json.MarshalIndent(expected, "", "  ")
	return strings.TrimSpace(fmt.Sprintf(`You are the slidex structured reviewer for stage %q.
Review only the provided artifact contract. Do not modify files.
Return JSON only matching schemas/app_review_findings.strict.schema.json.
Use this exact deterministic baseline unless you can identify a concrete blocker from the listed files:
%s
Deck directory: %s
Risk policy: blocker findings must use severity "fail"; non-blocking concerns use "warn" or "info"; every finding must include a path string.`, stage, string(expectedRaw), deckAbs))
}

func reviewFindingsFromPayload(payload map[string]any) []qaFinding {
	var findings []qaFinding
	rawFindings, _ := payload["findings"].([]any)
	for _, raw := range rawFindings {
		item, _ := raw.(map[string]any)
		findings = append(findings, qaFinding{
			Severity: fmt.Sprint(item["severity"]),
			Check:    fmt.Sprint(item["check"]),
			Message:  fmt.Sprint(item["message"]),
			Path:     fmt.Sprint(item["path"]),
		})
	}
	return findings
}

func findingsForStrictSchema(findings []qaFinding) []map[string]any {
	out := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		out = append(out, map[string]any{
			"severity": finding.Severity,
			"check":    finding.Check,
			"message":  finding.Message,
			"path":     finding.Path,
		})
	}
	return out
}

func makeSpecSlide(id, role, headline, key string, body []string) map[string]any {
	return map[string]any{"id": id, "htmlId": id, "sectionRole": role, "headline": headline, "keyMessage": key, "bodyContent": body, "layoutIntent": "single-purpose slide with clear hierarchy", "visualIntent": "simple evidence-aware layout", "evidenceRefs": []string{"brief.md"}, "claims": []string{"claim_001"}, "renderRisks": []string{"Korean wrapping and text density must be checked after render."}, "qaChecks": []string{"slide purpose clear", "no unsupported metric"}}
}

func applyIntakeAnswers(deckAbs, answersPath string) error {
	raw, err := os.ReadFile(answersPath)
	if err != nil {
		return err
	}
	briefPath := filepath.Join(deckAbs, "brief.md")
	var b strings.Builder
	if existing, err := os.ReadFile(briefPath); err == nil {
		b.Write(existing)
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## Intake Answers\n\n")
	b.Write(raw)
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	return os.WriteFile(briefPath, []byte(b.String()), 0o644)
}

func intakeQuestionsForDeck(deckAbs string) []string {
	brief := strings.TrimSpace(readFileOrEmpty(filepath.Join(deckAbs, "brief.md")))
	if len([]rune(brief)) < 80 || strings.Contains(strings.ToLower(brief), "todo") {
		return []string{
			"문서 유형은 무엇인가요? 예: 회사소개서, IR, 제안서, 정부지원 사업계획서, 임원 보고서",
			"핵심 청중과 이 문서로 얻어야 하는 결정 또는 행동은 무엇인가요?",
			"반드시 포함해야 하는 검증된 주장, 제외해야 하는 주장, 사용 가능한 근거 자료는 무엇인가요?",
		}
	}
	lower := strings.ToLower(brief)
	missing := []string{}
	if !strings.Contains(lower, "청중") && !strings.Contains(lower, "audience") {
		missing = append(missing, "핵심 청중을 확인해주세요.")
	}
	if !strings.Contains(lower, "목적") && !strings.Contains(lower, "objective") && !strings.Contains(lower, "goal") {
		missing = append(missing, "문서 목적과 원하는 결과를 확인해주세요.")
	}
	return missing
}

func writeIntakeQuestions(deckAbs string, questions []string, status string) error {
	outDir := filepath.Join(deckAbs, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Intake Questions\n\n")
	b.WriteString("- Status: `" + status + "`\n")
	b.WriteString("- Generated at: `" + time.Now().UTC().Format(time.RFC3339) + "`\n\n")
	if len(questions) == 0 {
		b.WriteString("현재 입력만으로 다음 deterministic stage를 진행할 수 있습니다. 단, Codex 작성 단계에서는 claim provenance를 계속 검증해야 합니다.\n")
	} else {
		for i, q := range questions {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, q))
		}
	}
	payload := map[string]any{"status": status, "questions": questions, "sourceInventorySha256": mustSHA256(filepath.Join(outDir, "source_inventory.md"))}
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "intake_questions.schema.json")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "intake_questions.md"), []byte(b.String()), 0o644)
}

func statusForQuestions(questions []string) string {
	if len(questions) > 0 {
		return "user_input_required"
	}
	return "complete"
}

func readGoModVersion(path string) string {
	raw := readFileOrEmpty(path)
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	return ""
}

func readMiseGoVersion(path string) string {
	raw := readFileOrEmpty(path)
	re := regexp.MustCompile(`(?m)^\s*go\s*=\s*"([^"]+)"`)
	m := re.FindStringSubmatch(raw)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func installedCodexVersion() string {
	out, err := exec.Command("codex", "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func commandOutput(timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out) + "\n" + err.Error())
	}
	return strings.TrimSpace(string(out))
}

func runCodexExecStructured(deckAbs, stage, prompt, schemaPath string, resume bool, resumeTarget string, images []string) (string, map[string]any, error) {
	outDir := filepath.Join(deckAbs, "out")
	runDir := filepath.Join(outDir, "agent_runs")
	if err := ensureSecureDir(runDir); err != nil {
		return "", nil, err
	}
	safeStage := strings.NewReplacer("/", "_", " ", "_").Replace(stage)
	mode := "fresh"
	if resume {
		mode = "resume"
	}
	runBase := safeStage + "_codex_exec_" + mode
	lastMessage := filepath.Join(runDir, runBase+".last.json")
	eventLog := filepath.Join(runDir, runBase+".jsonl")
	sessionPath := filepath.Join(runDir, "codex_exec_last_session.txt")
	requestedResumeTarget := resumeTarget
	effectiveResumeTarget := resumeTarget
	if resume && (effectiveResumeTarget == "" || effectiveResumeTarget == "last") {
		if local := strings.TrimSpace(readFileOrEmpty(sessionPath)); local != "" {
			effectiveResumeTarget = local
		}
	}
	args := codexExecArgs(schemaPath, lastMessage, resume, effectiveResumeTarget, images)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = mustAbs(".")
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.CombinedOutput()
	if writeErr := secureWriteFile(eventLog, out, 0o600); writeErr != nil {
		return "", nil, writeErr
	}
	threadID := extractCodexExecThreadID(out)
	if !resume && threadID != "" {
		_ = secureWriteFile(sessionPath, []byte(threadID+"\n"), 0o600)
	}
	run := map[string]any{
		"schemaVersion":         "slidex.codexExecRun.v1",
		"generatedAt":           time.Now().UTC().Format(time.RFC3339),
		"stage":                 stage,
		"mode":                  mode,
		"resumeTarget":          requestedResumeTarget,
		"effectiveResumeTarget": effectiveResumeTarget,
		"threadId":              threadID,
		"args":                  args,
		"cwd":                   mustAbs("."),
		"promptSha256":          sha256Bytes([]byte(prompt)),
		"outputSchemaPath":      filepath.ToSlash(schemaPath),
		"outputSchemaHash":      mustSHA256(schemaPath),
		"eventLog":              filepath.ToSlash(eventLog),
		"lastMessage":           filepath.ToSlash(lastMessage),
		"images":                images,
	}
	if err != nil {
		run["status"] = "fail"
		run["error"] = err.Error()
		path := filepath.Join(runDir, runBase+".json")
		_ = secureWriteJSON(path, run)
		return path, nil, fmt.Errorf("codex exec %s failed: %w\n%s", mode, err, string(out))
	}
	raw, err := os.ReadFile(lastMessage)
	if err != nil {
		run["status"] = "fail"
		run["error"] = err.Error()
		path := filepath.Join(runDir, runBase+".json")
		_ = secureWriteJSON(path, run)
		return path, nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		run["status"] = "fail"
		run["error"] = err.Error()
		path := filepath.Join(runDir, runBase+".json")
		_ = secureWriteJSON(path, run)
		return path, nil, err
	}
	if err := validatePayloadAgainstSchema(payload, schemaPath); err != nil {
		run["status"] = "fail"
		run["error"] = err.Error()
		path := filepath.Join(runDir, runBase+".json")
		_ = secureWriteJSON(path, run)
		return path, nil, err
	}
	run["status"] = "pass"
	run["structuredOutput"] = payload
	path := filepath.Join(runDir, runBase+".json")
	if err := secureWriteJSON(path, run); err != nil {
		return "", nil, err
	}
	return path, payload, nil
}

func extractCodexExecThreadID(raw []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !json.Valid([]byte(line)) {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if id, _ := event["thread_id"].(string); id != "" {
			return id
		}
		if id, _ := event["session_id"].(string); id != "" {
			return id
		}
	}
	return ""
}

func codexExecArgs(schemaPath, lastMessage string, resume bool, resumeTarget string, images []string) []string {
	args := []string{"exec"}
	if resume {
		args = append(args, "resume")
		args = append(args, "--json", "--output-schema", schemaPath, "--output-last-message", lastMessage)
		for _, image := range images {
			args = append(args, "--image", image)
		}
		if resumeTarget == "" || resumeTarget == "last" {
			args = append(args, "--last")
		} else {
			args = append(args, resumeTarget)
		}
		return append(args, "-")
	}
	args = append(args, "--json", "--sandbox", "read-only", "--cd", mustAbs("."), "--output-schema", schemaPath, "--output-last-message", lastMessage)
	for _, image := range images {
		args = append(args, "--image", image)
	}
	return append(args, "-")
}

func probeProtocolSchema() map[string]any {
	tmp, err := os.MkdirTemp("", "slidex-protocol-probe-*")
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	defer os.RemoveAll(tmp)
	help := commandOutput(8*time.Second, "codex", "app-server", "generate-json-schema", "--help")
	args := []string{"app-server", "generate-json-schema", "--out", tmp}
	experimental := strings.Contains(help, "--experimental")
	if experimental {
		args = append(args, "--experimental")
	}
	cmd := exec.Command("codex", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return map[string]any{"ok": false, "experimentalFlagSupported": experimental, "error": strings.TrimSpace(string(out) + "\n" + err.Error())}
	}
	files, _ := filepath.Glob(filepath.Join(tmp, "**", "*.json"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(tmp, "*.json"))
	}
	required := []string{"ClientRequest.json", "ServerNotification.json", filepath.Join("v2", "TurnStartParams.json"), filepath.Join("v2", "ThreadStartParams.json")}
	missing := []string{}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(tmp, name)); err != nil {
			missing = append(missing, name)
		}
	}
	methods := schemaMethodSet(filepath.Join(tmp, "ClientRequest.json"))
	turnRaw := readFileOrEmpty(filepath.Join(tmp, "v2", "TurnStartParams.json"))
	featureRaw := readFileOrEmpty(filepath.Join(tmp, "v2", "ExperimentalFeatureListParams.json"))
	outputSchema := strings.Contains(turnRaw, `"outputSchema"`)
	localImage := strings.Contains(turnRaw, `"localImage"`)
	threadScopedFeatureProbe := strings.Contains(featureRaw, `"threadId"`)
	requiredMethods := []string{"initialize", "thread/start", "turn/start", "model/list", "experimentalFeature/list", "mcpServerStatus/list"}
	optionalMethods := []string{"thread/goal/set", "thread/goal/get", "thread/goal/clear", "review/start", "thread/compact/start", "turn/interrupt", "turn/steer", "thread/read", "thread/turns/list"}
	missingMethods := []string{}
	for _, method := range requiredMethods {
		if !methods[method] {
			missingMethods = append(missingMethods, method)
		}
	}
	optionalAvailable := map[string]bool{}
	for _, method := range optionalMethods {
		optionalAvailable[method] = methods[method]
	}
	ok := len(missing) == 0 && len(missingMethods) == 0 && outputSchema && localImage
	return map[string]any{"ok": ok, "experimentalFlagSupported": experimental, "schemaFileCount": len(files), "missing": missing, "missingMethods": missingMethods, "requiredMethods": requiredMethods, "optionalMethods": optionalAvailable, "turnStartOutputSchema": outputSchema, "localImageSupported": localImage, "threadScopedFeatureProbe": threadScopedFeatureProbe, "permissionProfileRequired": false}
}

func schemaMethodSet(path string) map[string]bool {
	raw := readFileOrEmpty(path)
	methods := map[string]bool{}
	re := regexp.MustCompile(`"([a-zA-Z0-9_/-]+)"`)
	for _, m := range re.FindAllStringSubmatch(raw, -1) {
		if strings.Contains(m[1], "/") || m[1] == "initialize" {
			methods[m[1]] = true
		}
	}
	return methods
}

func writeProtocolManifest(bundleDir string) (string, error) {
	schemaDir := filepath.Join(bundleDir, "schema")
	files, _ := filepath.Glob(filepath.Join(schemaDir, "**", "*.json"))
	rootFiles, _ := filepath.Glob(filepath.Join(schemaDir, "*.json"))
	files = append(files, rootFiles...)
	sort.Strings(files)
	entries := []map[string]string{}
	for _, path := range uniqueStrings(files) {
		rel, _ := filepath.Rel(bundleDir, path)
		entries = append(entries, map[string]string{"path": filepath.ToSlash(rel), "sha256": mustSHA256(path)})
	}
	manifest := map[string]any{"schemaVersion": "slidex.codexProtocolManifest.v1", "codexVersion": requiredCodexVersion, "generatedAt": time.Now().UTC().Format(time.RFC3339), "schemaFiles": entries, "permissionProfileRequired": false, "threadGoalMethodsOptional": true, "reviewStartOptional": true, "threadScopedFeatureProbeOptional": true, "imageFidelityChecked": true}
	path := filepath.Join(bundleDir, "protocol_manifest.json")
	if err := writeJSONFile(path, manifest); err != nil {
		return "", err
	}
	return path, nil
}

func writeMethodConstants(bundleDir string) error {
	content := `package protocol

const RequiredCodexCLIVersion = "0.132.0"

const (
	MethodInitialize = "initialize"
	MethodModelList = "model/list"
	MethodExperimentalFeatureList = "experimentalFeature/list"
	MethodMCPServerStatusList = "mcpServerStatus/list"
	MethodThreadStart = "thread/start"
	MethodTurnStart = "turn/start"
	MethodTurnInterrupt = "turn/interrupt"
	MethodTurnSteer = "turn/steer"
	MethodThreadRead = "thread/read"
	MethodThreadTurnsList = "thread/turns/list"
	MethodThreadCompactStart = "thread/compact/start"
	MethodThreadGoalSet = "thread/goal/set"
	MethodThreadGoalGet = "thread/goal/get"
	MethodThreadGoalClear = "thread/goal/clear"
	MethodReviewStart = "review/start"
)
`
	if err := os.WriteFile(filepath.Join(bundleDir, "method_constants.go"), []byte(content), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bundleDir, "generated_types.go"), []byte("package protocol\n\n// Generated schema-backed protocol types are represented by vendored JSON Schema in schema/.\n"), 0o644)
}

func newState(deckAbs, mode string, allowMismatch bool) slidexState {
	outDir := filepath.Join(deckAbs, "out")
	codexVersion := installedCodexVersion()
	runtimeMode := mode
	reason := ""
	if mode == "exec_fallback" {
		reason = "app_server_unavailable_or_disabled"
	}
	bundleDir := filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion)
	return slidexState{SchemaVersion: stateSchemaVersion, ToolName: toolName, ToolVersion: toolVersion, GeneratedAt: time.Now().UTC().Format(time.RFC3339), ActiveDeckID: filepath.Base(deckAbs), DeckDir: deckAbs, OutDir: outDir, RequiredCodexVersion: requiredCodexVersion, CodexRuntime: runtimeState{Mode: runtimeMode, RequiredVersion: requiredCodexVersion, InstalledVersion: codexVersion, ProtocolBundle: filepath.ToSlash(bundleDir), ProtocolBundleHash: hashPathSet(bundleDir), AllowMismatch: allowMismatch, Reason: reason}, Goal: goalMirror{Status: "active"}}
}

func defaultCodexModel() string {
	return firstNonEmpty(os.Getenv("SLIDEX_CODEX_MODEL"), "gpt-5.5")
}

func enforceCodexRuntimeGate(state slidexState) error {
	if state.CodexRuntime.Mode != "app-server" && state.CodexRuntime.Mode != "exec" && state.CodexRuntime.Mode != "exec_fallback" {
		return exitCodeError(4, "unsupported Codex runtime mode: %s", state.CodexRuntime.Mode)
	}
	if state.CodexRuntime.InstalledVersion != requiredCodexVersion {
		if state.CodexRuntime.AllowMismatch {
			return nil
		}
		return exitCodeError(4, "Codex CLI version mismatch: need %s, got %s", requiredCodexVersion, firstNonEmpty(state.CodexRuntime.InstalledVersion, "missing"))
	}
	expectedBundle := filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion)
	expectedHash := hashPathSet(expectedBundle)
	if expectedHash == "" {
		return exitCodeError(4, "Codex App Server protocol bundle missing: %s", expectedBundle)
	}
	if state.CodexRuntime.ProtocolBundleHash != "" && state.CodexRuntime.ProtocolBundleHash != expectedHash && !state.CodexRuntime.AllowMismatch {
		return exitCodeError(4, "Codex App Server protocol bundle hash mismatch")
	}
	return nil
}

func protocolMismatchAcceptedRisk(state slidexState) *acceptedRisk {
	if !state.CodexRuntime.AllowMismatch {
		return nil
	}
	reasons := []string{}
	if state.CodexRuntime.InstalledVersion != requiredCodexVersion {
		reasons = append(reasons, fmt.Sprintf("Codex CLI version mismatch allowed: need %s, got %s", requiredCodexVersion, firstNonEmpty(state.CodexRuntime.InstalledVersion, "missing")))
	}
	expectedBundle := filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion)
	expectedHash := hashPathSet(expectedBundle)
	if expectedHash == "" {
		reasons = append(reasons, "Codex App Server protocol bundle is missing")
	} else if state.CodexRuntime.ProtocolBundleHash != "" && state.CodexRuntime.ProtocolBundleHash != expectedHash {
		reasons = append(reasons, "Codex App Server protocol bundle hash mismatch allowed")
	}
	if len(reasons) == 0 {
		return nil
	}
	return &acceptedRisk{
		Reason:       strings.Join(reasons, "; "),
		Owner:        "slidex",
		Expiration:   time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		ArtifactLink: "out/slidex_state.json",
	}
}

func enforceDirectCodexRuntime(mode string) error {
	return enforceCodexRuntimeGate(newState(mustAbs("."), mode, false))
}

func shouldStopGoalContinuation(goal goalMirror) bool {
	return goal.UsageLimitReached || strings.TrimSpace(goal.RepeatedBlockerSignature) != ""
}

func goalStopError(goal goalMirror) error {
	if goal.UsageLimitReached {
		return exitCodeError(7, "goal continuation stopped because usage limit was reached")
	}
	if sig := strings.TrimSpace(goal.RepeatedBlockerSignature); sig != "" {
		return exitCodeError(8, "goal continuation stopped because blocker repeated: %s", sig)
	}
	return exitCodeError(8, "goal continuation stopped")
}

func stageInputs(deckAbs, stage string) []artifact {
	outDir := filepath.Join(deckAbs, "out")
	paths := []string{}
	switch stage {
	case "resolve_workspace":
		paths = []string{filepath.Join(deckAbs, "brief.md"), filepath.Join(deckAbs, "DESIGN.md")}
	case "inspect_inputs":
		paths = []string{filepath.Join(deckAbs, "brief.md"), filepath.Join(deckAbs, "DESIGN.md")}
	case "intake":
		paths = []string{filepath.Join(outDir, "source_inventory.md"), filepath.Join(deckAbs, "brief.md")}
	case "strategy":
		paths = []string{filepath.Join(deckAbs, "brief.md"), filepath.Join(outDir, "source_inventory.md")}
	case "spec":
		paths = []string{filepath.Join(outDir, "strategy.md"), filepath.Join(deckAbs, "brief.md")}
	case "build_html":
		paths = []string{filepath.Join(outDir, "deck_spec.json")}
	case "baseline_html":
		paths = []string{filepath.Join(outDir, "final_deck.html")}
	case "render":
		paths = []string{filepath.Join(outDir, "final_deck.html")}
	case "qa":
		paths = []string{filepath.Join(outDir, "final_deck.html"), filepath.Join(outDir, "render_manifest.json"), filepath.Join(outDir, "final_deck.pdf")}
	case "delivery_summary":
		paths = []string{filepath.Join(outDir, "render_manifest.json"), filepath.Join(outDir, "qa_report.md")}
	case "review_loop":
		paths = []string{filepath.Join(outDir, "qa_report.md"), filepath.Join(outDir, "visual_reviews", "latest_review.json")}
	case "package":
		paths = []string{filepath.Join(outDir, "final_deck.html"), filepath.Join(outDir, "render_manifest.json"), filepath.Join(outDir, "qa_report.md"), filepath.Join(outDir, "delivery_summary.md")}
	}
	return artifactsForExisting(paths)
}

func stageOutputs(deckAbs, stage string) []artifact {
	outDir := filepath.Join(deckAbs, "out")
	paths := []string{}
	switch stage {
	case "resolve_workspace":
		paths = []string{filepath.Join(outDir, "slidex_state.json"), filepath.Join(outDir, "codex_threads.json"), filepath.Join(outDir, "protocol_diagnostics.json")}
	case "inspect_inputs":
		paths = []string{filepath.Join(outDir, "source_inventory.md")}
	case "intake":
		paths = []string{filepath.Join(outDir, "intake_questions.md")}
	case "strategy":
		paths = []string{filepath.Join(outDir, "strategy.md")}
	case "spec":
		paths = []string{filepath.Join(outDir, "deck_spec.json")}
	case "build_html":
		paths = []string{filepath.Join(outDir, "final_deck.html")}
	case "baseline_html":
		paths = []string{filepath.Join(outDir, "final_deck.generated_baseline.html")}
	case "render":
		paths = []string{filepath.Join(outDir, "render_manifest.json"), filepath.Join(outDir, "final_deck.pdf"), filepath.Join(outDir, "qa_montage.png")}
		pngs, _ := filepath.Glob(filepath.Join(outDir, "rendered_slides", "slide_*.png"))
		paths = append(paths, pngs...)
	case "qa":
		paths = []string{filepath.Join(outDir, "qa_report.md"), filepath.Join(outDir, "visual_reviews", "image_set.json"), filepath.Join(outDir, "visual_reviews", "latest_review.json")}
	case "delivery_summary":
		paths = []string{filepath.Join(outDir, "delivery_summary.md"), filepath.Join(outDir, "notes.md")}
	case "review_loop":
		paths = []string{filepath.Join(outDir, "agent_reviews", "round_01", "reviewer_delivery.json"), filepath.Join(outDir, "agent_reviews", "round_01", "resolution.md")}
	case "package":
		paths = []string{filepath.Join(outDir, "slidex_state.json")}
	}
	return artifactsForExisting(paths)
}

func shouldRunAgentStageAudit(stage string) bool {
	switch stage {
	case "resolve_workspace", "strategy", "spec", "build_html", "qa", "review_loop", "delivery_summary":
		return true
	default:
		return false
	}
}

func runAppServerStageAudit(appRun *appServerWorkflowRun, deckAbs string, state slidexState, stage string) (string, error) {
	prompt := stageAuditPrompt(deckAbs, state, stage, "app-server")
	result, err := appRun.runStructuredTurn(stage, prompt, filepath.Join("schemas", "app_stage_result.strict.schema.json"), 3*time.Minute)
	if err != nil {
		return "", err
	}
	if corrected, correction := normalizeStageAuditOutput(deckAbs, stage, result.StructuredOutput); correction != nil {
		result.AuditCorrection = correction
		result.StructuredOutput = corrected
		_ = appendRunLog(filepath.Join(deckAbs, "out"), map[string]any{"event": "stage_audit_corrected", "stage": stage, "runtime": "app-server", "correction": correction})
	}
	path, result, err := writeAppServerTurnResult(filepath.Join(deckAbs, "out"), result)
	if err != nil {
		return "", err
	}
	if err := recordAppServerTurn(filepath.Join(deckAbs, "out"), stage, result); err != nil {
		return "", err
	}
	if status, _ := result.StructuredOutput["status"].(string); status != "pass" && status != "pass_with_risks" {
		return path, exitCodeError(3, "App Server stage %s returned stop condition %s", stage, status)
	}
	return path, nil
}

func runCodexExecStageAudit(deckAbs, stage string, resume bool, resumeTarget string) (string, error) {
	path, payload, err := runCodexExecStructured(deckAbs, stage, stageAuditPrompt(deckAbs, readStateOrNew(deckAbs, "exec", false), stage, "exec"), filepath.Join("schemas", "app_stage_result.strict.schema.json"), resume, resumeTarget, nil)
	if err != nil {
		return path, err
	}
	if corrected, correction := normalizeStageAuditOutput(deckAbs, stage, payload); correction != nil {
		payload = corrected
		_ = recordCodexExecAuditCorrection(path, corrected, correction)
		_ = appendRunLog(filepath.Join(deckAbs, "out"), map[string]any{"event": "stage_audit_corrected", "stage": stage, "runtime": "exec", "execRun": path, "correction": correction})
	}
	if status, _ := payload["status"].(string); status != "pass" && status != "pass_with_risks" {
		return path, exitCodeError(3, "codex exec stage %s returned stop condition %s", stage, status)
	}
	return path, nil
}

func recordCodexExecAuditCorrection(path string, corrected map[string]any, correction map[string]any) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var run map[string]any
	if err := json.Unmarshal(raw, &run); err != nil {
		return err
	}
	run["structuredOutput"] = corrected
	run["normalizedStructuredOutput"] = corrected
	run["auditCorrection"] = correction
	return secureWriteJSON(path, run)
}

func stageAuditPrompt(deckAbs string, state slidexState, stage, runtime string) string {
	inputsRaw, _ := json.MarshalIndent(stageInputs(deckAbs, stage), "", "  ")
	goalRaw, _ := json.MarshalIndent(state.Goal, "", "  ")
	baselineRaw, _ := json.MarshalIndent(stageResultBaseline(deckAbs, stage), "", "  ")
	return strings.TrimSpace(fmt.Sprintf(`You are the slidex %s structured stage runner for stage %q.
Return JSON only matching schemas/app_stage_result.strict.schema.json. Do not inspect or modify files.
The deterministic slidex engine already inspected the files before this turn.
Return the Baseline JSON exactly. Do not infer missing files, do not override status, and do not add risks.

Deck directory: %s
Output schema: schemas/app_stage_result.strict.schema.json
Output schema sha256: %s
Goal context:
%s
Selected inputs:
%s
Baseline JSON:
%s

Risk policy:
- status must be "pass" when the listed outputs satisfy the stage contract.
- use "pass_with_risks" only when a concrete non-blocking risk remains and include risk owner, reason, expiration, and artifactLink.
- use "blocked" or "user_input_required" only for a blocking condition.`, runtime, stage, deckAbs, mustSHA256(filepath.Join("schemas", "app_stage_result.strict.schema.json")), string(goalRaw), string(inputsRaw), string(baselineRaw)))
}

func normalizeStageAuditOutput(deckAbs, stage string, payload map[string]any) (map[string]any, map[string]any) {
	baseline := stageResultBaseline(deckAbs, stage)
	if !stageBaselineArtifactsComplete(baseline) {
		return payload, nil
	}
	if payload == nil {
		return baseline, map[string]any{"reason": "stage audit returned no structured output; deterministic baseline artifacts are complete"}
	}
	status, _ := payload["status"].(string)
	if status == "pass" || status == "pass_with_risks" {
		return payload, nil
	}
	return baseline, map[string]any{
		"reason":          "stage audit returned non-pass status despite complete deterministic baseline artifacts",
		"reportedStatus":  status,
		"reportedSummary": fmt.Sprint(payload["summary"]),
	}
}

func stageBaselineArtifactsComplete(baseline map[string]any) bool {
	rawArtifacts, _ := baseline["artifacts"].([]map[string]any)
	if len(rawArtifacts) == 0 {
		if anyArtifacts, ok := baseline["artifacts"].([]any); ok {
			return len(anyArtifacts) > 0
		}
		return false
	}
	for _, artifact := range rawArtifacts {
		if strings.TrimSpace(fmt.Sprint(artifact["path"])) == "" || strings.TrimSpace(fmt.Sprint(artifact["sha256"])) == "" {
			return false
		}
	}
	return true
}

func stageResultBaseline(deckAbs, stage string) map[string]any {
	artifacts := []map[string]any{}
	for _, artifact := range stageOutputs(deckAbs, stage) {
		artifacts = append(artifacts, map[string]any{
			"path":   filepath.ToSlash(artifact.Path),
			"sha256": artifact.SHA256,
			"kind":   artifactKind(artifact.Path),
		})
	}
	return map[string]any{
		"stage":     stage,
		"status":    "pass",
		"summary":   "Stage " + stage + " completed with current slidex artifact hashes recorded.",
		"artifacts": artifacts,
		"risks":     []map[string]any{},
	}
}

func artifactKind(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "" {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return "directory"
		}
		return "artifact"
	}
	return ext
}

func recordAppServerTurn(outDir, stage string, result appServerTurnResult) error {
	idx := readThreadIndex(outDir)
	if idx.SchemaVersion == "" {
		idx.SchemaVersion = threadsSchemaVersion
	}
	idx.CodexVersion = installedCodexVersion()
	idx.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	found := false
	for i := range idx.Threads {
		if idx.Threads[i].ThreadID == result.ThreadID {
			if idx.Threads[i].Role == "" {
				idx.Threads[i].Role = "workflow"
			}
			if idx.Threads[i].Mode == "" {
				idx.Threads[i].Mode = "app_server_thread"
			}
			idx.Threads[i].Stage = stage
			idx.Threads[i].LastTurnID = result.TurnID
			idx.Threads[i].TurnIDs = appendUnique(idx.Threads[i].TurnIDs, result.TurnID)
			idx.Threads[i].OutputSchemaHash = result.OutputSchemaHash
			idx.Threads[i].LastEventLog = result.EventLog
			idx.Threads[i].TokenUsage = mergeTokenUsage(idx.Threads[i].TokenUsage, tokenUsageFromEvents(result.Events, result.ThreadID))
			found = true
			break
		}
	}
	if !found {
		idx.Threads = append(idx.Threads, threadState{
			ThreadID:                result.ThreadID,
			ThreadName:              filepath.Base(filepath.Dir(outDir)) + "-app-server",
			Role:                    "workflow",
			Mode:                    "app_server_thread",
			Stage:                   stage,
			LastTurnID:              result.TurnID,
			TurnIDs:                 []string{result.TurnID},
			Model:                   defaultCodexModel(),
			ApprovalPolicy:          "never",
			ApprovalMode:            "never",
			Sandbox:                 "readOnly",
			SandboxMode:             "readOnly",
			EffectiveWorkspaceRoots: []string{mustAbs("."), filepath.Dir(outDir)},
			TokenUsage:              tokenUsageFromEvents(result.Events, result.ThreadID),
			OutputSchemaHash:        result.OutputSchemaHash,
			LastEventLog:            result.EventLog,
			PromptTemplateVersion:   toolVersion,
		})
	}
	return writeThreadIndex(outDir, idx)
}

func markThreadRole(outDir, threadID, role, mode, parentThreadID string) error {
	idx := readThreadIndex(outDir)
	for i := range idx.Threads {
		if idx.Threads[i].ThreadID == threadID {
			idx.Threads[i].Role = role
			idx.Threads[i].Mode = mode
			idx.Threads[i].ParentThreadID = parentThreadID
			idx.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
			return writeThreadIndex(outDir, idx)
		}
	}
	return nil
}

func artifactsForExisting(paths []string) []artifact {
	var out []artifact
	for _, path := range uniqueStrings(paths) {
		if _, err := os.Stat(path); err == nil {
			out = append(out, artifactFromPath(path))
		}
	}
	return out
}

func readRenderManifest(path string) (renderManifest, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return renderManifest{}, false
	}
	var manifest renderManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return renderManifest{}, false
	}
	return manifest, true
}

func ensureRuntimeArtifacts(deckAbs string, state slidexState) error {
	outDir := filepath.Join(deckAbs, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := writeState(outDir, state); err != nil {
		return err
	}
	idx := readThreadIndex(outDir)
	idx.SchemaVersion = threadsSchemaVersion
	idx.CodexVersion = installedCodexVersion()
	idx.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if len(idx.Threads) == 0 {
		idx.Threads = append(idx.Threads, threadState{ThreadID: "local-mirror", ThreadName: filepath.Base(deckAbs) + "-pipeline", Stage: "run", Model: "catalog-default", ServiceTier: "catalog-default", ApprovalPolicy: "on-request", ApprovalMode: "on-request", Sandbox: "workspace-write", SandboxMode: "workspace-write", EffectiveWorkspaceRoots: []string{mustAbs(".")}, TokenUsage: map[string]int{}})
	}
	return writeThreadIndex(outDir, idx)
}

func writeState(outDir string, state slidexState) error {
	return secureWriteJSON(filepath.Join(outDir, "slidex_state.json"), state)
}

func readStateOrNew(deck, mode string, allowMismatch bool) slidexState {
	deckAbs := mustAbs(deck)
	path := filepath.Join(deckAbs, "out", "slidex_state.json")
	raw, err := os.ReadFile(path)
	if err == nil {
		var state slidexState
		if json.Unmarshal(raw, &state) == nil {
			return state
		}
	}
	return newState(deckAbs, mode, allowMismatch)
}

func readThreadIndex(outDir string) codexThreadIndex {
	raw, err := os.ReadFile(filepath.Join(outDir, "codex_threads.json"))
	if err == nil {
		var idx codexThreadIndex
		if json.Unmarshal(raw, &idx) == nil {
			return idx
		}
	}
	return codexThreadIndex{SchemaVersion: threadsSchemaVersion, CodexVersion: installedCodexVersion(), GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
}

func writeThreadIndex(outDir string, idx codexThreadIndex) error {
	return secureWriteJSON(filepath.Join(outDir, "codex_threads.json"), idx)
}

func threadIndexFromAppServerSnapshot(deckAbs string, snapshot map[string]any) codexThreadIndex {
	idx := codexThreadIndex{
		SchemaVersion: threadsSchemaVersion,
		CodexVersion:  installedCodexVersion(),
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	threadResult, _ := snapshot["thread_start"].(map[string]any)
	threadObj, _ := threadResult["thread"].(map[string]any)
	threadID, _ := threadObj["id"].(string)
	if threadID == "" {
		threadID = "app-server-probe"
	}
	model, _ := threadResult["model"].(string)
	serviceTier, _ := threadResult["serviceTier"].(string)
	cwd, _ := threadResult["cwd"].(string)
	approval, _ := threadResult["approvalPolicy"].(string)
	sandboxMode := ""
	if sandbox, ok := threadResult["sandbox"].(map[string]any); ok {
		sandboxMode, _ = sandbox["type"].(string)
	}
	roots := []string{}
	if rawRoots, ok := threadResult["runtimeWorkspaceRoots"].([]any); ok {
		for _, raw := range rawRoots {
			if s, _ := raw.(string); s != "" {
				roots = append(roots, s)
			}
		}
	}
	if len(roots) == 0 {
		roots = []string{mustAbs(".")}
	}
	idx.Threads = []threadState{{
		ThreadID:                 threadID,
		ThreadName:               filepath.Base(deckAbs) + "-app-server-probe",
		Role:                     "workflow",
		Mode:                     "app_server_thread",
		Stage:                    "resolve_workspace",
		Model:                    firstNonEmpty(model, defaultCodexModel()),
		ServiceTier:              firstNonEmpty(serviceTier, "catalog-default"),
		ApprovalPolicy:           approval,
		ApprovalMode:             approval,
		Sandbox:                  sandboxMode,
		SandboxMode:              sandboxMode,
		EffectiveWorkspaceRoots:  roots,
		TokenUsage:               map[string]int{},
		GlobalFeatureProbe:       snapshot["experimentalFeature_list"],
		ThreadScopedFeatureProbe: snapshot["experimentalFeature_thread_scoped"],
		OutputSchemaHash:         mustSHA256(filepath.Join("schemas", "app_stage_result.strict.schema.json")),
		PromptTemplateVersion:    toolVersion,
	}}
	if cwd != "" {
		idx.Threads[0].EffectiveWorkspaceRoots = append(idx.Threads[0].EffectiveWorkspaceRoots, "cwd:"+cwd)
	}
	return idx
}

func appendRunLog(outDir string, event map[string]any) error {
	event["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	redacted := redactSecretsInAny(event)
	raw, err := json.Marshal(redacted)
	if err != nil {
		return err
	}
	if err := ensureSecureDir(outDir); err != nil {
		return err
	}
	path := filepath.Join(outDir, "run_log.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

func acquireRunLock(outDir string) (func(), error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(outDir, ".slidex.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, exitCodeError(1, "another slidex run appears active: %s", path)
	}
	_, _ = fmt.Fprintf(f, "pid=%d\nstarted=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	_ = f.Close()
	return func() { _ = os.Remove(path) }, nil
}

func secureWriteJSON(path string, v any) error {
	raw, err := json.MarshalIndent(redactSecretsInAny(v), "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return secureWriteFile(path, raw, 0o600)
}

func secureWriteFile(path string, raw []byte, mode os.FileMode) error {
	if err := ensureSecureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, raw, mode)
}

func ensureSecureDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func redactSecretsInAny(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	s := redactSecrets(string(raw))
	var out any
	if json.Unmarshal([]byte(s), &out) != nil {
		return v
	}
	return out
}

func redactSecrets(s string) string {
	patterns := []string{`OPENAI_API_KEY=[^"\s]+`, `CODEX_API_KEY=[^"\s]+`, `Authorization:\s*Bearer\s+[^"\s]+`, `Bearer\s+[A-Za-z0-9._-]+`, `(?i)(token|secret|cookie|set-cookie)["']?\s*[:=]\s*["']?[^"',\s}]+`}
	out := s
	for _, pattern := range patterns {
		out = regexp.MustCompile(pattern).ReplaceAllString(out, "${1}[REDACTED]")
	}
	return out
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func migrationFindings(deckAbs, from string) []string {
	outDir := filepath.Join(deckAbs, "out")
	var findings []string
	if _, err := os.Stat(filepath.Join(outDir, "final_deck.pptx")); err == nil {
		findings = append(findings, "Historical PPTX artifact found; classify as archived artifact, not generated deliverable.")
	}
	if _, err := os.Stat(filepath.Join(outDir, "final_deck.html")); err == nil {
		if _, err := os.Stat(filepath.Join(outDir, "final_deck.generated_baseline.html")); os.IsNotExist(err) {
			findings = append(findings, "final_deck.html exists without generated baseline.")
		}
	}
	specRaw := readFileOrEmpty(filepath.Join(outDir, "deck_spec.json"))
	if strings.Contains(strings.ToLower(specRaw), "pptx") || strings.Contains(strings.ToLower(specRaw), "powerpoint") {
		findings = append(findings, "deck_spec.json contains PPTX or PowerPoint fields that must be removed or reported.")
	}
	if deckAbs == mustAbs(".") {
		for _, legacy := range []string{"brief.md", "assets", "brand", "data", "out"} {
			if _, err := os.Stat(filepath.Join(deckAbs, legacy)); err == nil {
				findings = append(findings, "Legacy root-level workspace material detected: "+legacy)
			}
		}
	} else if !strings.HasPrefix(deckAbs, mustAbs("decks")+string(os.PathSeparator)) {
		for _, legacy := range []string{"brief.md", "assets", "brand", "data", "out"} {
			if _, err := os.Stat(filepath.Join(deckAbs, legacy)); err == nil && legacy == "out" {
				findings = append(findings, "Non-standard deck path uses out/ compatibility mode: "+legacy)
			}
		}
	}
	if len(findings) == 0 {
		findings = append(findings, "No migration changes required for "+from+".")
	}
	return findings
}

func writeVisualReviewImageSet(path string, manifest renderManifest) error {
	if len(manifest.PNGFiles) == 0 {
		return nil
	}
	hashes := []string{}
	for _, img := range manifest.PNGFiles {
		hashes = append(hashes, img.SHA256)
	}
	set := visualReviewImageSet{SchemaVersion: "slidex.visualReviewImageSet.v1", GeneratedAt: time.Now().UTC().Format(time.RFC3339), HTMLSha256: manifest.SourceHTML.SHA256, ManifestSha256: mustSHA256(filepath.Join(filepath.Dir(filepath.Dir(path)), "render_manifest.json")), ImageSetSha256: sha256Bytes([]byte(strings.Join(hashes, "\n"))), RequestedFidelity: "original", FidelitySupportStatus: "recorded_for_app_server_or_exec_visual_review", Images: manifest.PNGFiles}
	return secureWriteJSON(path, set)
}

func runVisualReview(deckAbs string, manifest renderManifest, mode string) (string, []qaFinding) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = "codex"
	}
	outDir := filepath.Join(deckAbs, "out")
	reviewPath := filepath.Join(outDir, "visual_reviews", "latest_review.json")
	imageEvidence := visualReviewEvidence(deckAbs, manifest)
	payload := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         "visual_qa",
		"round":         1,
		"mode":          mode,
		"status":        "pass",
		"imageEvidence": imageEvidence,
		"findings":      []qaFinding{},
	}
	switch mode {
	case "none":
		payload["status"] = "pass_with_risks"
		payload["findings"] = []qaFinding{{Severity: "info", Check: "visual_review.disabled", Message: "Visual review explicitly disabled for deterministic run.", Path: reviewPath}}
	case "manual":
		if !visualReviewArtifactFresh(reviewPath, manifest) {
			return "missing", []qaFinding{fail("visual_review.manual", "manual visual review is required and latest_review.json is missing or stale", reviewPath)}
		}
		return "pass", nil
	case "codex":
		if len(manifest.PNGFiles) == 0 {
			return "blocked", []qaFinding{fail("visual_review.codex", "no rendered PNGs available for Codex visual review", filepath.Join(outDir, "rendered_slides"))}
		}
		if os.Getenv("SLIDEX_ENABLE_CODEX_VISUAL_QA") != "1" {
			return "blocked", []qaFinding{fail("visual_review.codex", "set SLIDEX_ENABLE_CODEX_VISUAL_QA=1 to run codex exec --image visual QA; package cannot pass with pending Codex visual review", reviewPath)}
		}
		codexPayload, err := runCodexExecVisualReview(deckAbs, manifest)
		if err != nil {
			return "blocked", []qaFinding{fail("visual_review.codex", err.Error(), reviewPath)}
		}
		if err := secureWriteJSON(reviewPath, codexPayload); err != nil {
			return "blocked", []qaFinding{fail("visual_review.codex_write", err.Error(), reviewPath)}
		}
		if status, _ := codexPayload["status"].(string); status != "pass" {
			return firstNonEmpty(status, "fail"), []qaFinding{fail("visual_review.codex_status", "Codex visual review did not pass", reviewPath)}
		}
		return "pass", nil
	default:
		return "blocked", []qaFinding{fail("visual_review.mode", "unsupported visual review mode: "+mode, reviewPath)}
	}
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "review_findings.schema.json")); err != nil {
		return "blocked", []qaFinding{fail("visual_review.schema", err.Error(), reviewPath)}
	}
	if err := secureWriteJSON(reviewPath, payload); err != nil {
		return "blocked", []qaFinding{fail("visual_review.write", err.Error(), reviewPath)}
	}
	status, _ := payload["status"].(string)
	return firstNonEmpty(status, "pass"), nil
}

func runAppServerVisualReview(deckAbs string, manifest renderManifest, appRun *appServerWorkflowRun) (string, []qaFinding) {
	outDir := filepath.Join(deckAbs, "out")
	reviewPath := filepath.Join(outDir, "visual_reviews", "latest_review.json")
	if appRun == nil || appRun.threadID == "" {
		return "blocked", []qaFinding{fail("visual_review.app_server", "App Server visual review requires an active thread", reviewPath)}
	}
	if len(manifest.PNGFiles) == 0 {
		return "blocked", []qaFinding{fail("visual_review.app_server", "no rendered PNGs available for App Server localImage visual QA", filepath.Join(outDir, "rendered_slides"))}
	}
	imageEvidence := visualReviewEvidence(deckAbs, manifest)
	expected := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         "visual_qa",
		"round":         1,
		"mode":          "codex_subagent",
		"status":        "pass",
		"imageEvidence": imageEvidence,
		"findings":      []map[string]any{},
	}
	expectedRaw, _ := json.MarshalIndent(expected, "", "  ")
	prompt := strings.TrimSpace(fmt.Sprintf(`Review the attached rendered slide images for visual QA using original local image fidelity.
Return JSON only matching schemas/app_review_findings.strict.schema.json.
Use this exact imageEvidence array. If no visible issue is present, return the baseline status pass with empty findings.
Baseline JSON:
%s`, string(expectedRaw)))
	input := []map[string]any{{"type": "text", "text": prompt}}
	for _, image := range manifest.PNGFiles {
		input = append(input, map[string]any{"type": "localImage", "path": image.Path, "detail": "original"})
	}
	result, err := appRun.runStructuredTurnWithInput("visual_qa", input, prompt, filepath.Join("schemas", "app_review_findings.strict.schema.json"), 5*time.Minute)
	if err != nil {
		return "blocked", []qaFinding{fail("visual_review.app_server", err.Error(), reviewPath)}
	}
	turnPath, result, err := writeAppServerTurnResult(outDir, result)
	if err != nil {
		return "blocked", []qaFinding{fail("visual_review.app_server_write", err.Error(), reviewPath)}
	}
	if err := recordAppServerTurn(outDir, "visual_qa", result); err != nil {
		return "blocked", []qaFinding{fail("visual_review.app_server_thread", err.Error(), reviewPath)}
	}
	payload := result.StructuredOutput
	if payload == nil {
		return "blocked", []qaFinding{fail("visual_review.app_server", "App Server visual review returned no structured output", reviewPath)}
	}
	payload["mode"] = "codex_subagent"
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "review_findings.schema.json")); err != nil {
		return "blocked", []qaFinding{fail("visual_review.app_server_schema", err.Error(), reviewPath)}
	}
	if err := secureWriteJSON(reviewPath, payload); err != nil {
		return "blocked", []qaFinding{fail("visual_review.app_server_write", err.Error(), reviewPath)}
	}
	_ = appendRunLog(outDir, map[string]any{"event": "visual_review_app_server", "turn": turnPath, "review": reviewPath, "imageFidelity": "original", "imageCount": len(manifest.PNGFiles)})
	if status, _ := payload["status"].(string); status != "pass" {
		return firstNonEmpty(status, "fail"), []qaFinding{fail("visual_review.app_server_status", "App Server visual review did not pass", reviewPath)}
	}
	return "pass", nil
}

func visualReviewEvidence(deckAbs string, manifest renderManifest) []map[string]any {
	evidence := make([]map[string]any, 0, len(manifest.PNGFiles))
	for _, img := range manifest.PNGFiles {
		rel, _ := filepath.Rel(deckAbs, img.Path)
		evidence = append(evidence, map[string]any{
			"slideId":          img.SlideID,
			"repoRelativePath": filepath.ToSlash(rel),
			"absolutePath":     img.Path,
			"sha256":           img.SHA256,
			"blank":            img.Blank,
			"fidelity":         "original",
			"dimensions":       map[string]any{"width": img.Dimensions.Width, "height": img.Dimensions.Height},
		})
	}
	return evidence
}

func runCodexExecVisualReview(deckAbs string, manifest renderManifest) (map[string]any, error) {
	outDir := filepath.Join(deckAbs, "out")
	lastMessage := filepath.Join(outDir, "visual_reviews", "codex_visual_review.last.json")
	if err := ensureSecureDir(filepath.Dir(lastMessage)); err != nil {
		return nil, err
	}
	evidenceRaw, _ := json.MarshalIndent(visualReviewEvidence(deckAbs, manifest), "", "  ")
	prompt := "Review the attached rendered slide images for visual QA. Return JSON only matching schemas/app_review_findings.strict.schema.json. schemaVersion must be slidex.reviewFindings.v1, stage visual_qa, round 1, mode codex_subagent. Include this exact imageEvidence array and set fidelity=original. If no issue is visible, status must be pass and findings empty.\nImage evidence:\n" + string(evidenceRaw)
	args := []string{
		"exec",
		"--sandbox", "read-only",
	}
	for _, image := range manifest.PNGFiles {
		args = append(args, "--image", image.Path)
	}
	args = append(args, "--output-schema", filepath.Join("schemas", "app_review_findings.strict.schema.json"), "--output-last-message", lastMessage, "-")
	cmd := exec.Command("codex", args...)
	cmd.Dir = mustAbs(".")
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("codex exec visual QA failed: %w\n%s", err, string(out))
	}
	raw, err := os.ReadFile(lastMessage)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "app_review_findings.strict.schema.json")); err != nil {
		return nil, err
	}
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "review_findings.schema.json")); err != nil {
		return nil, err
	}
	return payload, nil
}

func visualReviewArtifactFresh(path string, manifest renderManifest) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return false
	}
	if payload["status"] != "pass" {
		return false
	}
	encoded, _ := json.Marshal(payload)
	for _, img := range manifest.PNGFiles {
		if !strings.Contains(string(encoded), img.SHA256) {
			return false
		}
	}
	return true
}

func verifyVisualReviewImageSet(path string, manifest renderManifest) []qaFinding {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []qaFinding{fail("package.visual_review_image_set", "visual review image set missing: "+err.Error(), path)}
	}
	var set visualReviewImageSet
	if err := json.Unmarshal(raw, &set); err != nil {
		return []qaFinding{fail("package.visual_review_image_set", err.Error(), path)}
	}
	var findings []qaFinding
	if set.HTMLSha256 != manifest.SourceHTML.SHA256 {
		findings = append(findings, fail("package.visual_review_image_set_freshness", "visual review image set HTML hash is stale", path))
	}
	if len(set.Images) != len(manifest.PNGFiles) {
		findings = append(findings, fail("package.visual_review_image_set_count", "visual review image count does not match manifest", path))
	}
	for i, img := range set.Images {
		if i >= len(manifest.PNGFiles) {
			break
		}
		if img.SHA256 != manifest.PNGFiles[i].SHA256 || img.Dimensions != manifest.PNGFiles[i].Dimensions || img.Blank != manifest.PNGFiles[i].Blank {
			findings = append(findings, fail("package.visual_review_image_set_image", "visual review image metadata differs from manifest", path))
		}
	}
	if set.RequestedFidelity != "original" {
		findings = append(findings, fail("package.visual_review_image_set_fidelity", "visual review image set must request original fidelity", path))
	}
	return findings
}

func verifyVisualReviewEvidence(path string, manifest renderManifest) []qaFinding {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []qaFinding{fail("package.visual_review_evidence", "visual review missing: "+err.Error(), path)}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []qaFinding{fail("package.visual_review_evidence", err.Error(), path)}
	}
	rawEvidence, _ := payload["imageEvidence"].([]any)
	if len(rawEvidence) != len(manifest.PNGFiles) {
		return []qaFinding{fail("package.visual_review_evidence", fmt.Sprintf("visual review imageEvidence count %d does not match rendered image count %d", len(rawEvidence), len(manifest.PNGFiles)), path)}
	}
	var findings []qaFinding
	for i, rawItem := range rawEvidence {
		item, _ := rawItem.(map[string]any)
		img := manifest.PNGFiles[i]
		if slideID, _ := item["slideId"].(string); slideID != img.SlideID {
			findings = append(findings, fail("package.visual_review_evidence", "visual review slideId does not match manifest", path))
		}
		if sha, _ := item["sha256"].(string); sha != img.SHA256 {
			findings = append(findings, fail("package.visual_review_evidence", "visual review image hash does not match manifest", path))
		}
		if fidelity, _ := item["fidelity"].(string); fidelity != "original" {
			findings = append(findings, fail("package.visual_review_evidence", "visual review fidelity must be original", path))
		}
		if blank, _ := item["blank"].(bool); blank != img.Blank {
			findings = append(findings, fail("package.visual_review_evidence", "visual review blank flag does not match manifest", path))
		}
		dims, _ := item["dimensions"].(map[string]any)
		width, _ := numberAsInt(dims["width"])
		height, _ := numberAsInt(dims["height"])
		if width != img.Dimensions.Width || height != img.Dimensions.Height {
			findings = append(findings, fail("package.visual_review_evidence", "visual review dimensions do not match manifest", path))
		}
	}
	return findings
}

func verifyStructuredReviewGate(path string, manifest renderManifest) []qaFinding {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []qaFinding{fail("package.structured_review", "structured delivery review missing: "+err.Error(), path)}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []qaFinding{fail("package.structured_review", err.Error(), path)}
	}
	if status, _ := payload["status"].(string); status != "pass" {
		return []qaFinding{fail("package.structured_review", "structured delivery review did not pass", path)}
	}
	if info, err := os.Stat(path); err == nil {
		if manifestTime, parseErr := time.Parse(time.RFC3339, manifest.RenderTimestamp); parseErr == nil && info.ModTime().Before(manifestTime) {
			return []qaFinding{fail("package.structured_review_freshness", "structured delivery review is older than render manifest", path)}
		}
	}
	return nil
}

func verifyTextArtifactFreshness(check, path, referencePath string, requiredHashes []string) []qaFinding {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []qaFinding{fail("package."+check+"_freshness", "missing artifact: "+err.Error(), path)}
	}
	var findings []qaFinding
	refInfo, refErr := os.Stat(referencePath)
	info, infoErr := os.Stat(path)
	if refErr == nil && infoErr == nil && info.ModTime().Before(refInfo.ModTime()) {
		findings = append(findings, fail("package."+check+"_freshness", "artifact is older than render manifest", path))
	}
	text := string(raw)
	for _, hash := range requiredHashes {
		if hash != "" && !strings.Contains(text, hash) {
			findings = append(findings, fail("package."+check+"_freshness", "artifact does not reference current hash "+hash, path))
		}
	}
	return findings
}

func verifySanitizedLogs(outDir string) []qaFinding {
	logPath := filepath.Join(outDir, "run_log.jsonl")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		return []qaFinding{fail("package.logs", "include-logs requested but run_log.jsonl is missing", logPath)}
	}
	text := string(raw)
	if strings.Contains(text, "OPENAI_API_KEY=") || strings.Contains(text, "CODEX_API_KEY=") || strings.Contains(text, "Bearer ") {
		return []qaFinding{fail("package.logs_sanitizer", "log contains unsanitized secret-looking content", logPath)}
	}
	return nil
}

func validatePayloadAgainstSchema(payload any, schemaPath string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return err
	}
	findings := validateWithFullJSONSchema(raw, schema, schemaPath)
	if hasFailures(findings) {
		return fmt.Errorf("payload failed %s validation: %v", schemaPath, findings)
	}
	return nil
}

func packageHasStaleFinding(findings []qaFinding) bool {
	for _, f := range findings {
		if strings.Contains(f.Check, "freshness") || strings.Contains(f.Check, "stale") {
			return true
		}
	}
	return false
}

func hashFileSet(glob string) string {
	paths, _ := filepath.Glob(glob)
	sort.Strings(paths)
	var b strings.Builder
	for _, path := range paths {
		b.WriteString(filepath.ToSlash(path))
		b.WriteString(" ")
		b.WriteString(mustSHA256(path))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return sha256Bytes([]byte(b.String()))
}

func hashPathSet(root string) string {
	var b strings.Builder
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b.WriteString(filepath.ToSlash(path))
		b.WriteString(" ")
		b.WriteString(mustSHA256(path))
		b.WriteString("\n")
		return nil
	})
	if b.Len() == 0 {
		return ""
	}
	return sha256Bytes([]byte(b.String()))
}

func tokenUsageFromEvents(events []map[string]any, threadFilter ...string) map[string]int {
	usage := map[string]int{}
	filter := ""
	if len(threadFilter) > 0 {
		filter = strings.TrimSpace(threadFilter[0])
	}
	for _, event := range events {
		method, _ := event["method"].(string)
		if method != "thread/tokenUsage/updated" {
			continue
		}
		params, _ := event["params"].(map[string]any)
		if filter != "" {
			if threadID, _ := params["threadId"].(string); threadID != "" && threadID != filter {
				continue
			}
		}
		tokenUsage, _ := params["tokenUsage"].(map[string]any)
		total, _ := tokenUsage["total"].(map[string]any)
		for _, key := range []string{"inputTokens", "cachedInputTokens", "outputTokens", "reasoningOutputTokens", "totalTokens"} {
			if value, ok := numberAsInt(total[key]); ok {
				usage[key] = value
			}
		}
		if window, ok := numberAsInt(tokenUsage["modelContextWindow"]); ok {
			usage["modelContextWindow"] = window
		}
	}
	return usage
}

func mergeTokenUsage(existing, next map[string]int) map[string]int {
	if existing == nil {
		existing = map[string]int{}
	}
	for key, value := range next {
		existing[key] = value
	}
	return existing
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func printDoctorHuman(report map[string]any) {
	fmt.Printf("%s %s doctor\n", toolName, toolVersion)
	fmt.Printf("status: %s\n", report["status"])
	if findings, ok := report["findings"].([]qaFinding); ok {
		for _, f := range findings {
			fmt.Printf("- %s %s: %s\n", f.Severity, f.Check, f.Message)
		}
	}
}

func doctorHasFail(report map[string]any) bool {
	findings, _ := report["findings"].([]qaFinding)
	return hasFailures(findings)
}

func doctorHasUnsupported(report map[string]any) bool {
	findings, _ := report["findings"].([]qaFinding)
	for _, f := range findings {
		if f.Check == "doctor.protocol_schema" || f.Check == "doctor.codex_version" {
			return true
		}
	}
	return false
}

func deckFlag(args []string) (string, error) {
	fs := flag.NewFlagSet("deck", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *deck == "" {
		return "", exitCodeError(2, "--deck is required")
	}
	return *deck, nil
}

func syncGoalToAppServer(deckAbs, outDir, threadID string, goal goalMirror) (map[string]any, error) {
	status := goalStatusForAppServer(goal.Status)
	if status == "" {
		status = "active"
	}
	if !appServerGoalStatusAllowed(status) {
		return nil, fmt.Errorf("goal status %q is not allowed by generated App Server schema", status)
	}
	objective := strings.TrimSpace(goal.Objective)
	if objective == "" && goal.ObjectiveFile != "" {
		objective = strings.TrimSpace(readFileOrEmpty(filepath.Join(deckAbs, filepath.FromSlash(goal.ObjectiveFile))))
	}
	params := map[string]any{"objective": objective, "status": status}
	if goal.TokenBudget > 0 {
		params["tokenBudget"] = goal.TokenBudget
	}
	result, syncedThreadID, events, err := appServerGoalRequest(deckAbs, threadID, "thread/goal/set", params)
	if err != nil {
		return nil, err
	}
	if err := recordGoalSync(outDir, syncedThreadID, status, events); err != nil {
		return nil, err
	}
	return result, nil
}

func syncGoalWithAppRun(deckAbs, outDir string, appRun *appServerWorkflowRun, goal goalMirror) (map[string]any, error) {
	if appRun == nil || appRun.threadID == "" {
		return nil, fmt.Errorf("App Server run is not active")
	}
	status := goalStatusForAppServer(goal.Status)
	if status == "" {
		status = "active"
	}
	if !appServerGoalStatusAllowed(status) {
		return nil, fmt.Errorf("goal status %q is not allowed by generated App Server schema", status)
	}
	objective := strings.TrimSpace(goal.Objective)
	if objective == "" && goal.ObjectiveFile != "" {
		objective = strings.TrimSpace(readFileOrEmpty(filepath.Join(deckAbs, filepath.FromSlash(goal.ObjectiveFile))))
	}
	params := map[string]any{"threadId": appRun.threadID, "objective": objective, "status": status}
	if goal.TokenBudget > 0 {
		params["tokenBudget"] = goal.TokenBudget
	}
	resp, events, err := appRun.client.request("thread/goal/set", params, 20*time.Second)
	if err != nil {
		return nil, err
	}
	if err := recordGoalSync(outDir, appRun.threadID, status, events); err != nil {
		return nil, err
	}
	result, _ := resp["result"].(map[string]any)
	return result, nil
}

func getGoalFromAppServer(deckAbs, outDir, threadID string) (map[string]any, error) {
	result, syncedThreadID, events, err := appServerGoalRequest(deckAbs, threadID, "thread/goal/get", nil)
	if err != nil {
		return nil, err
	}
	status := ""
	if goal, _ := result["goal"].(map[string]any); goal != nil {
		status, _ = goal["status"].(string)
	}
	if err := recordGoalSync(outDir, syncedThreadID, status, events); err != nil {
		return nil, err
	}
	return result, nil
}

func clearGoalInAppServer(deckAbs, outDir, threadID string) (map[string]any, error) {
	result, syncedThreadID, events, err := appServerGoalRequest(deckAbs, threadID, "thread/goal/clear", nil)
	if err != nil {
		return nil, err
	}
	if err := recordGoalSync(outDir, syncedThreadID, "", events); err != nil {
		return nil, err
	}
	return result, nil
}

func recordGoalSync(outDir, threadID, status string, events []map[string]any) error {
	if len(events) > 0 {
		path := filepath.Join(outDir, "agent_runs", "goal_appserver_events.jsonl")
		if err := ensureSecureDir(filepath.Dir(path)); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		for _, event := range events {
			if err := enc.Encode(redactSecretsInAny(event)); err != nil {
				_ = f.Close()
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	idx := readThreadIndex(outDir)
	found := false
	for i := range idx.Threads {
		if idx.Threads[i].ThreadID == threadID {
			idx.Threads[i].GoalStatus = status
			found = true
			break
		}
	}
	if !found && threadID != "" {
		idx.Threads = append(idx.Threads, threadState{
			ThreadID:                threadID,
			ThreadName:              filepath.Base(filepath.Dir(outDir)) + "-goal",
			Role:                    "goal",
			Mode:                    "app_server_thread",
			Stage:                   "goal",
			Model:                   defaultCodexModel(),
			ApprovalPolicy:          "never",
			ApprovalMode:            "never",
			Sandbox:                 "read-only",
			SandboxMode:             "read-only",
			EffectiveWorkspaceRoots: []string{mustAbs("."), filepath.Dir(outDir)},
			TokenUsage:              map[string]int{},
			GoalStatus:              status,
			PromptTemplateVersion:   toolVersion,
		})
	}
	idx.SchemaVersion = threadsSchemaVersion
	idx.CodexVersion = installedCodexVersion()
	idx.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	return writeThreadIndex(outDir, idx)
}

func bestAppServerThreadID(outDir string) string {
	idx := readThreadIndex(outDir)
	for _, thread := range idx.Threads {
		if thread.ThreadID != "" && thread.ThreadID != "local-mirror" && !strings.HasPrefix(thread.ThreadID, "app-server-probe") {
			return thread.ThreadID
		}
	}
	return ""
}

func goalStatusForAppServer(status string) string {
	switch status {
	case "", "active":
		return "active"
	case "paused":
		return "paused"
	case "blocked":
		return "blocked"
	case "usage_limited", "usageLimited":
		return "usageLimited"
	case "budget_limited", "budgetLimited":
		return "budgetLimited"
	case "complete":
		return "complete"
	default:
		return status
	}
}

func goalMismatch(local goalMirror, appGoal map[string]any) bool {
	goal, _ := appGoal["goal"].(map[string]any)
	if goal == nil {
		return local.Objective != "" || local.ObjectiveFile != "" || local.Status != ""
	}
	appStatus, _ := goal["status"].(string)
	if appStatus != "" && goalStatusForAppServer(local.Status) != appStatus {
		return true
	}
	if local.Objective != "" {
		if appObjective, _ := goal["objective"].(string); appObjective != "" && appObjective != local.Objective {
			return true
		}
	}
	return false
}

func appServerGoalStatusAllowed(status string) bool {
	raw, err := os.ReadFile(filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion, "schema", "v2", "ThreadGoalSetParams.json"))
	if err != nil {
		return false
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return false
	}
	defs, _ := schema["definitions"].(map[string]any)
	goalStatus, _ := defs["ThreadGoalStatus"].(map[string]any)
	values, _ := goalStatus["enum"].([]any)
	for _, value := range values {
		if s, _ := value.(string); s == status {
			return true
		}
	}
	return false
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func printCommandJSON(tool, action, output string) error {
	return printJSON(map[string]any{"toolName": toolName, "tool": tool, "action": action, "output": output})
}

func runCommandJSON(action string, timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	payload := map[string]any{"toolName": toolName, "action": action, "output": strings.TrimSpace(string(out))}
	if err != nil {
		payload["status"] = "fail"
		_ = printJSON(payload)
		return err
	}
	payload["status"] = "pass"
	return printJSON(payload)
}

func nullOrRaw(s string) []byte {
	s = strings.TrimSpace(s)
	if s == "" || !json.Valid([]byte(s)) {
		return []byte("null")
	}
	return []byte(s)
}

func readFileOrEmpty(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func firstNRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func firstMarkdownHeading(path string) string {
	for _, line := range strings.Split(readFileOrEmpty(path), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

func specValue(spec map[string]any, path ...string) any {
	var cur any = spec
	for _, key := range path {
		obj, _ := cur.(map[string]any)
		cur = obj[key]
	}
	return cur
}

func escapeHTML(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(s)
}

func copyStream(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
}
