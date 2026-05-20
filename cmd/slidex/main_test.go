package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestExtractSlidesUsesHTMLParserForNestedSections(t *testing.T) {
	src := `<!doctype html><html lang="ko"><body><main class="deck">
<section class="slide" id="slide_01" data-slide-id="slide_01">
  <h1>겉 제목</h1>
  <section><p>중첩 섹션 본문</p></section>
</section>
</main></body></html>`
	slides := extractSlides(src)
	if len(slides) != 1 {
		t.Fatalf("expected 1 slide, got %d", len(slides))
	}
	if slides[0].ID != "slide_01" {
		t.Fatalf("unexpected slide id: %s", slides[0].ID)
	}
	if !strings.Contains(slides[0].Text, "중첩 섹션 본문") {
		t.Fatalf("nested section text was not preserved: %q", slides[0].Text)
	}
}

func TestDeterministicRenderQAPackageE2E(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if _, err := resolveChrome(""); err != nil {
		t.Skipf("Chrome/Chromium is not available: %v", err)
	}

	deck := filepath.Join(t.TempDir(), "minimal_deck")
	if err := copyDir(filepath.Join(root, "fixtures", "minimal_deck"), deck); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(deck, "out")
	if _, err := ensureStrategy(deck, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureSpec(deck, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureHTML(deck, true); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(outDir, "final_deck.html"), filepath.Join(outDir, "final_deck.generated_baseline.html")); err != nil {
		t.Fatal(err)
	}
	cfg, err := renderConfigFromFlags(
		filepath.Join(outDir, "final_deck.html"),
		filepath.Join(outDir, "rendered_slides"),
		filepath.Join(outDir, "final_deck.pdf"),
		filepath.Join(outDir, "render_manifest.json"),
		"paginated",
		".slide",
		1920,
		1080,
		"pretendard",
		"",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := renderHTML(cfg)
	if err != nil {
		if isChromeSandboxEnvironmentFailure(err) {
			t.Skipf("Chrome cannot render in this sandbox: %v", err)
		}
		t.Fatal(err)
	}
	if manifest.ChromeSandbox != "enabled" {
		t.Fatalf("Chrome sandbox should be enabled by default, got %q", manifest.ChromeSandbox)
	}
	if manifest.SlideEnumerationMethod != "chrome-dom" {
		t.Fatalf("expected chrome-dom enumeration, got %q", manifest.SlideEnumerationMethod)
	}
	qa, err := qaDeck(deck, true)
	if err != nil {
		t.Fatalf("qa failed: %v", err)
	}
	if qa.Status == "fail" {
		t.Fatalf("qa status should not fail: %+v", qa.Findings)
	}
	writeTestVisualReviewPass(t, deck, manifest)
	if err := ensureRuntimeArtifacts(deck, newState(deck, "exec", false)); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDeliverySummary(deck); err != nil {
		t.Fatal(err)
	}
	if _, err := writeStructuredReview(deck, "delivery", 1); err != nil {
		t.Fatal(err)
	}
	pkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	if pkg["status"] != "pass" {
		t.Fatalf("package should pass, got %#v", pkg)
	}
	if status, findings := runVisualReview(deck, manifest, "none"); status != "pass_with_risks" || hasFailures(findings) {
		t.Fatalf("visual review none should be non-blocking at QA stage, status=%s findings=%v", status, findings)
	}
	noVisualPkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	if noVisualPkg["status"] != "fail" {
		t.Fatalf("package must fail when visual review is disabled, got %#v", noVisualPkg)
	}
	writeTestVisualReviewPass(t, deck, manifest)
	if err := os.WriteFile(filepath.Join(outDir, "final_deck.html"), []byte(readFileOrEmpty(filepath.Join(outDir, "final_deck.html"))+"\n<!-- stale edit -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	if stale["status"] != "fail" {
		t.Fatalf("stale package should fail, got %#v", stale)
	}
	findings, ok := stale["findings"].([]qaFinding)
	if !ok || !packageHasStaleFinding(findings) {
		t.Fatalf("stale package should produce stale finding, got %#v", stale["findings"])
	}
	err = runPackage([]string{"--deck", deck})
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) || coded.ExitCode() != 5 {
		t.Fatalf("runPackage stale exit code = %v, %v; want 5", coded, err)
	}
}

func TestMigrateDryRunNeverWritesWithoutWrite(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	if err := copyDir(filepath.Join(repoRootForTest(t), "fixtures", "minimal_deck"), deck); err != nil {
		t.Fatal(err)
	}
	err := runMigrate([]string{"--deck", deck, "--dry-run=false"})
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) || coded.ExitCode() != 2 {
		t.Fatalf("expected --dry-run=false without --write to fail with exit 2, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(deck, "out", "slidex_state.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run=false without --write must not create state, stat err=%v", err)
	}
}

func TestStrictAppServerSchemasAcceptLocalPayloads(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	stagePayload := stageResultBaseline(filepath.Join(root, "fixtures", "minimal_deck"), "resolve_workspace")
	if err := validatePayloadAgainstSchema(stagePayload, filepath.Join("schemas", "app_stage_result.strict.schema.json")); err != nil {
		t.Fatal(err)
	}
	reviewPayload := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         "delivery",
		"round":         1,
		"mode":          "structured_turn",
		"status":        "pass",
		"imageEvidence": []map[string]any{},
		"findings":      []map[string]any{},
	}
	if err := validatePayloadAgainstSchema(reviewPayload, filepath.Join("schemas", "app_review_findings.strict.schema.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCodexExecResumeArgsKeepOutputSchema(t *testing.T) {
	args := codexExecArgs("schemas/app_stage_result.strict.schema.json", "last.json", true, "session-123", nil)
	got := strings.Join(args, " ")
	for _, want := range []string{"exec resume", "--json", "--output-schema schemas/app_stage_result.strict.schema.json", "--output-last-message last.json", "session-123", "-"} {
		if !strings.Contains(got, want) {
			t.Fatalf("resume args %q missing %q", got, want)
		}
	}
	lastArgs := strings.Join(codexExecArgs("schema.json", "last.json", true, "last", nil), " ")
	if !strings.Contains(lastArgs, "--last") {
		t.Fatalf("resume --last args missing --last: %q", lastArgs)
	}
}

func TestAppServerTurnCompletionUsesObservedTurnID(t *testing.T) {
	client := &appServerClient{lines: make(chan map[string]any, 4)}
	go func() {
		client.lines <- map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "other-turn", "status": "completed"}}}
		client.lines <- map[string]any{"method": "turn/started", "params": map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "actual-turn", "status": "inProgress"}}}
		client.lines <- map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "actual-turn", "status": "completed"}}}
	}()
	events, completion, err := client.waitForTurnCompletion("thread-1", "response-turn", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	if got := turnIDFromCompletion(completion); got != "actual-turn" {
		t.Fatalf("completion turn id = %q", got)
	}
}

func TestGoalStatusEnumIsReadFromGeneratedSchema(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if !appServerGoalStatusAllowed("usageLimited") {
		t.Fatal("generated ThreadGoalStatus enum should allow usageLimited")
	}
	if appServerGoalStatusAllowed("usage_limited") {
		t.Fatal("local snake_case status should not be accepted without mapping")
	}
	if got := goalStatusForAppServer("usage_limited"); got != "usageLimited" {
		t.Fatalf("goalStatusForAppServer = %q", got)
	}
}

func TestRuntimeGateBlocksProtocolMismatchByDefault(t *testing.T) {
	state := newState(filepath.Join(t.TempDir(), "deck"), "app-server", false)
	state.CodexRuntime.InstalledVersion = "0.0.0"
	if err := enforceCodexRuntimeGate(state); err == nil {
		t.Fatal("expected version mismatch to fail")
	}
	state.CodexRuntime.AllowMismatch = true
	if err := enforceCodexRuntimeGate(state); err != nil {
		t.Fatalf("allow mismatch should bypass version gate: %v", err)
	}
}

func TestGoalContinuationStopsForUsageLimitAndRepeatedBlocker(t *testing.T) {
	if !shouldStopGoalContinuation(goalMirror{UsageLimitReached: true}) {
		t.Fatal("usage limit should stop continuation")
	}
	var coded interface{ ExitCode() int }
	if err := goalStopError(goalMirror{UsageLimitReached: true}); !errors.As(err, &coded) || coded.ExitCode() != 7 {
		t.Fatalf("usage limit exit = %v, %v; want 7", coded, err)
	}
	if !shouldStopGoalContinuation(goalMirror{RepeatedBlockerSignature: "same-blocker"}) {
		t.Fatal("repeated blocker should stop continuation")
	}
	if err := goalStopError(goalMirror{RepeatedBlockerSignature: "same-blocker"}); !errors.As(err, &coded) || coded.ExitCode() != 8 {
		t.Fatalf("repeated blocker exit = %v, %v; want 8", coded, err)
	}
}

func TestStageAuditNormalizationUsesDeterministicBaseline(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"delivery_summary.md", "notes.md"} {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	payload := map[string]any{"stage": "delivery_summary", "status": "blocked", "summary": "model guessed files are missing", "artifacts": []any{}, "risks": []any{}}
	corrected, correction := normalizeStageAuditOutput(deck, "delivery_summary", payload)
	if correction == nil {
		t.Fatal("expected false blocked audit to be corrected")
	}
	if got, _ := corrected["status"].(string); got != "pass" {
		t.Fatalf("corrected status = %q, want pass", got)
	}
	artifacts, _ := corrected["artifacts"].([]map[string]any)
	if len(artifacts) != 2 {
		t.Fatalf("corrected artifacts = %d, want 2", len(artifacts))
	}
}

func TestProtocolMismatchAllowRecordsAcceptedRisk(t *testing.T) {
	state := newState(filepath.Join(t.TempDir(), "deck"), "app-server", true)
	state.CodexRuntime.InstalledVersion = "0.0.0"
	risk := protocolMismatchAcceptedRisk(state)
	if risk == nil {
		t.Fatal("expected allow-mismatch to produce accepted risk")
	}
	if risk.Owner != "slidex" || risk.ArtifactLink != "out/slidex_state.json" {
		t.Fatalf("unexpected risk: %#v", risk)
	}
	if _, err := time.Parse(time.RFC3339, risk.Expiration); err != nil {
		t.Fatalf("risk expiration is not RFC3339: %v", err)
	}
}

func TestTokenUsageAndMCPReplayPreserveThreadRouting(t *testing.T) {
	events := []map[string]any{
		{
			"method": "thread/tokenUsage/updated",
			"params": map[string]any{
				"threadId": "other-thread",
				"turnId":   "turn-x",
				"tokenUsage": map[string]any{
					"total": map[string]any{
						"inputTokens":           float64(999),
						"cachedInputTokens":     float64(999),
						"outputTokens":          float64(999),
						"reasoningOutputTokens": float64(999),
						"totalTokens":           float64(999),
					},
				},
			},
		},
		{
			"method": "thread/tokenUsage/updated",
			"params": map[string]any{
				"threadId": "thread-1",
				"turnId":   "turn-1",
				"tokenUsage": map[string]any{
					"total": map[string]any{
						"inputTokens":           float64(10),
						"cachedInputTokens":     float64(2),
						"outputTokens":          float64(3),
						"reasoningOutputTokens": float64(4),
						"totalTokens":           float64(19),
					},
					"modelContextWindow": float64(128000),
				},
			},
		},
	}
	usage := tokenUsageFromEvents(events, "thread-1")
	if usage["totalTokens"] != 19 || usage["modelContextWindow"] != 128000 {
		t.Fatalf("unexpected token usage: %#v", usage)
	}

	deck := filepath.Join(t.TempDir(), "deck")
	runDir := filepath.Join(deck, "out", "agent_runs")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(runDir, "qa_appserver_events.jsonl")
	lines := []string{
		`{"method":"item/mcpToolCall/progress","params":{"threadId":"thread-1","turnId":"turn-1","requestingThreadId":"parent","itemId":"item-1","message":"start"}}`,
		`{"method":"item/completed","params":{"threadId":"thread-2","turnId":"turn-2"}}`,
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	record, err := replayMCPEvents(deck, "")
	if err != nil {
		t.Fatal(err)
	}
	if record.ThreadCount != 1 || record.EventCount != 1 {
		t.Fatalf("unexpected replay record: %#v", record)
	}
	replayText := readFileOrEmpty(filepath.Join(deck, "out", "agent_runs", "mcp_event_replay.json"))
	if !strings.Contains(replayText, `"requestingThreadId": "parent"`) {
		t.Fatalf("replay did not preserve requestingThreadId: %s", replayText)
	}
}

func TestQAReportRecordsCodexRuntimeMode(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	state := newState(deck, "exec", false)
	if err := writeState(outDir, state); err != nil {
		t.Fatal(err)
	}
	mode, _ := qaRuntimeForDeck(deck)
	if mode != "exec" {
		t.Fatalf("qa runtime mode = %q, want exec", mode)
	}
	reportPath := filepath.Join(outDir, "qa_report.md")
	if err := writeQAReport(reportPath, qaResult{ToolName: toolName, Version: toolVersion, DeckDir: deck, Status: "pass", RuntimeMode: mode}); err != nil {
		t.Fatal(err)
	}
	report := readFileOrEmpty(reportPath)
	if !strings.Contains(report, "runtimeMode: exec") || !strings.Contains(report, "Runtime mode: `exec`") {
		t.Fatalf("qa report did not record runtime mode: %s", report)
	}
}

func TestExecAuditCorrectionIsWrittenBackToArtifact(t *testing.T) {
	runPath := filepath.Join(t.TempDir(), "run.json")
	original := map[string]any{"schemaVersion": "slidex.codexExecRun.v1", "structuredOutput": map[string]any{"status": "blocked"}}
	if err := secureWriteJSON(runPath, original); err != nil {
		t.Fatal(err)
	}
	corrected := map[string]any{"stage": "delivery_summary", "status": "pass", "summary": "ok", "artifacts": []any{}, "risks": []any{}}
	correction := map[string]any{"reason": "deterministic baseline complete"}
	if err := recordCodexExecAuditCorrection(runPath, corrected, correction); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if raw, err := os.ReadFile(runPath); err != nil {
		t.Fatal(err)
	} else if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	structured, _ := got["structuredOutput"].(map[string]any)
	if structured["status"] != "pass" || got["auditCorrection"] == nil {
		t.Fatalf("correction was not written back: %#v", got)
	}
}

func TestWebSocketRiskIsRecordedInStateAndDeliverySummary(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	risk := "WebSocket App Server is experimental/unsupported and limited to loopback."
	if err := recordWebSocketTransportRisk(deck, risk, filepath.Join(outDir, "codex-app-server.json")); err != nil {
		t.Fatal(err)
	}
	state := readStateOrNew(deck, "app-server", false)
	if len(state.AcceptedRisks) != 1 || state.AcceptedRisks[0].Reason != risk {
		t.Fatalf("risk was not recorded: %#v", state.AcceptedRisks)
	}
	if err := os.WriteFile(filepath.Join(outDir, "render_manifest.json"), []byte(`{"pdfPageCount":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "qa_report.md"), []byte("# QA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := writeDeliverySummary(deck)
	if err != nil {
		t.Fatal(err)
	}
	summary := readFileOrEmpty(path)
	if !strings.Contains(summary, risk) {
		t.Fatalf("delivery summary did not include risk: %s", summary)
	}
}

func writeTestVisualReviewPass(t *testing.T, deck string, manifest renderManifest) {
	t.Helper()
	payload := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         "visual_qa",
		"round":         1,
		"mode":          "manual",
		"status":        "pass",
		"imageEvidence": visualReviewEvidence(deck, manifest),
		"findings":      []qaFinding{},
	}
	path := filepath.Join(deck, "out", "visual_reviews", "latest_review.json")
	if err := validatePayloadAgainstSchema(payload, filepath.Join("schemas", "review_findings.schema.json")); err != nil {
		t.Fatal(err)
	}
	if err := secureWriteJSON(path, payload); err != nil {
		t.Fatal(err)
	}
}

func TestStateAndRunLogUseSecurePermissionsAndRedaction(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	state := newState(deck, "exec", false)
	if err := ensureRuntimeArtifacts(deck, state); err != nil {
		t.Fatal(err)
	}
	if err := appendRunLog(outDir, map[string]any{"message": "CODEX_API_KEY=secret-token Authorization: Bearer raw-token"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(outDir, "slidex_state.json"), filepath.Join(outDir, "codex_threads.json"), filepath.Join(outDir, "run_log.jsonl")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o, want 0600", path, info.Mode().Perm())
		}
	}
	outInfo, err := os.Stat(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if outInfo.Mode().Perm() != 0o700 {
		t.Fatalf("out dir mode = %o, want 0700", outInfo.Mode().Perm())
	}
	logText := readFileOrEmpty(filepath.Join(outDir, "run_log.jsonl"))
	if strings.Contains(logText, "secret-token") || strings.Contains(logText, "raw-token") {
		t.Fatalf("run log was not redacted: %s", logText)
	}
}

func TestStrictStageAndReviewSchemasValidateRuntimePayloads(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := filepath.Join(t.TempDir(), "deck")
	if err := copyDir(filepath.Join(root, "fixtures", "minimal_deck"), deck); err != nil {
		t.Fatal(err)
	}
	stagePayload := stageResultBaseline(deck, "resolve_workspace")
	if err := validatePayloadAgainstSchema(stagePayload, filepath.Join("schemas", "app_stage_result.strict.schema.json")); err != nil {
		t.Fatal(err)
	}
	reviewPayload := map[string]any{
		"schemaVersion": "slidex.reviewFindings.v1",
		"stage":         "delivery",
		"round":         1,
		"mode":          "structured_turn",
		"status":        "pass",
		"imageEvidence": []map[string]any{},
		"findings":      []map[string]any{},
	}
	if err := validatePayloadAgainstSchema(reviewPayload, filepath.Join("schemas", "app_review_findings.strict.schema.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCodexExecArgsIncludeSchemaForFreshAndResume(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	fresh := codexExecArgs("schemas/app_stage_result.strict.schema.json", "/tmp/last.json", false, "", []string{"/tmp/slide.png"})
	wantFresh := []string{"exec", "--json", "--sandbox", "read-only", "--cd", root, "--output-schema", "schemas/app_stage_result.strict.schema.json", "--output-last-message", "/tmp/last.json", "--image", "/tmp/slide.png", "-"}
	if !reflect.DeepEqual(fresh, wantFresh) {
		t.Fatalf("fresh args = %#v, want %#v", fresh, wantFresh)
	}
	resumeLast := codexExecArgs("schemas/app_stage_result.strict.schema.json", "/tmp/last.json", true, "last", nil)
	wantResumeLast := []string{"exec", "resume", "--json", "--output-schema", "schemas/app_stage_result.strict.schema.json", "--output-last-message", "/tmp/last.json", "--last", "-"}
	if !reflect.DeepEqual(resumeLast, wantResumeLast) {
		t.Fatalf("resume --last args = %#v, want %#v", resumeLast, wantResumeLast)
	}
	resumeSession := codexExecArgs("schemas/app_stage_result.strict.schema.json", "/tmp/last.json", true, "019e-session", nil)
	wantResumeSession := []string{"exec", "resume", "--json", "--output-schema", "schemas/app_stage_result.strict.schema.json", "--output-last-message", "/tmp/last.json", "019e-session", "-"}
	if !reflect.DeepEqual(resumeSession, wantResumeSession) {
		t.Fatalf("resume session args = %#v, want %#v", resumeSession, wantResumeSession)
	}
}

func TestAppServerFinalMessageExtractionAcceptsActualCompletedTurnID(t *testing.T) {
	events := []map[string]any{
		{
			"method": "item/completed",
			"params": map[string]any{
				"turnId": "actual-turn",
				"item": map[string]any{
					"type":  "agentMessage",
					"text":  `{"stage":"resolve_workspace","status":"pass","summary":"ok","artifacts":[],"risks":[]}`,
					"phase": "final_answer",
				},
			},
		},
	}
	text := extractFinalAgentTextFromEvents(events, "actual-turn")
	if !strings.Contains(text, `"resolve_workspace"`) {
		t.Fatalf("final agent text was not extracted: %q", text)
	}
}

func TestAppServerThreadCompactWaitsForMatchingThread(t *testing.T) {
	client := &appServerClient{lines: make(chan map[string]any, 3)}
	go func() {
		client.lines <- map[string]any{"method": "thread/compacted", "params": map[string]any{"threadId": "other", "turnId": "turn-other"}}
		client.lines <- map[string]any{"method": "thread/compacted", "params": map[string]any{"threadId": "thread-1", "turnId": "turn-1"}}
	}()
	events, compacted, err := client.waitForThreadCompacted("thread-1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if got, _ := compacted["turnId"].(string); got != "turn-1" {
		t.Fatalf("compact turn id = %q", got)
	}
}

func isChromeSandboxEnvironmentFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "trace/breakpoint trap") || strings.Contains(msg, "crashpad") || strings.Contains(msg, "Operation not permitted")
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
	}
}
