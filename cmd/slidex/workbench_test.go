package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBootstrapDeckRejectsTraversalAndCreatesUnderDecks(t *testing.T) {
	root := repoRootForTest(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "decks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(filepath.Join(root, "decks", "_template"), filepath.Join(workspace, "decks", "_template")); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrapDeckWorkspace(workspace, "..", "decks/_template", true); err == nil {
		t.Fatal("path traversal deck id should fail")
	}
	if _, err := bootstrapDeckWorkspace(workspace, ".hidden", "decks/_template", true); err == nil {
		t.Fatal("hidden dot-prefix deck id should fail")
	}
	result, err := bootstrapDeckWorkspace(workspace, "customer-retention", "decks/_template", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "created" {
		t.Fatalf("status = %q, want created", result.Status)
	}
	if result.DeckDir != "decks/customer-retention" {
		t.Fatalf("deck output path = %s, want decks/customer-retention", result.DeckDir)
	}
	if _, err := os.Stat(filepath.Join(workspace, "decks", "customer-retention", "brief.md")); err != nil {
		t.Fatalf("template brief was not copied: %v", err)
	}
}

func TestBootstrapDeckUsesRepoTemplateForExternalWorkspace(t *testing.T) {
	workspace := t.TempDir()
	result, err := bootstrapDeckWorkspace(workspace, "external-template", "decks/_template", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "created" {
		t.Fatalf("status = %q, want created", result.Status)
	}
	if _, err := os.Stat(filepath.Join(workspace, "decks", "external-template", "brief.md")); err != nil {
		t.Fatalf("template brief was not copied from repo fallback: %v", err)
	}
}

func TestWorkbenchSaveRequiresTokenAndSameOrigin(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := "secret-token"
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", token, 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: token, manifest: manifest}
	payload := []byte(`{"title":"Demo","audience":"Board","decisionGoal":"Approve pilot","sourceNotes":"Use confirmed notes","outputExpectations":"HTML/PDF"}`)

	badToken := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/save", bytes.NewReader(payload))
	badToken.Header.Set("Origin", "http://127.0.0.1:43210")
	badToken.Header.Set("X-Slidex-Workbench-Token", "wrong")
	badTokenRecorder := httptest.NewRecorder()
	server.handleSave(badTokenRecorder, badToken)
	if badTokenRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d, want %d", badTokenRecorder.Code, http.StatusUnauthorized)
	}

	badOrigin := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/save", bytes.NewReader(payload))
	badOrigin.Header.Set("Origin", "http://evil.example")
	badOrigin.Header.Set("X-Slidex-Workbench-Token", token)
	badOriginRecorder := httptest.NewRecorder()
	server.handleSave(badOriginRecorder, badOrigin)
	if badOriginRecorder.Code != http.StatusForbidden {
		t.Fatalf("bad origin status = %d, want %d", badOriginRecorder.Code, http.StatusForbidden)
	}

	badReferer := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/save", bytes.NewReader(payload))
	badReferer.Header.Set("Origin", "http://127.0.0.1:43210")
	badReferer.Header.Set("Referer", "http://evil.example/form")
	badReferer.Header.Set("X-Slidex-Workbench-Token", token)
	badRefererRecorder := httptest.NewRecorder()
	server.handleSave(badRefererRecorder, badReferer)
	if badRefererRecorder.Code != http.StatusForbidden {
		t.Fatalf("bad referer status = %d, want %d", badRefererRecorder.Code, http.StatusForbidden)
	}

	noOrigin := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/save", bytes.NewReader(payload))
	noOrigin.Header.Set("X-Slidex-Workbench-Token", token)
	noOriginRecorder := httptest.NewRecorder()
	server.handleSave(noOriginRecorder, noOrigin)
	if noOriginRecorder.Code != http.StatusForbidden {
		t.Fatalf("no origin status = %d, want %d", noOriginRecorder.Code, http.StatusForbidden)
	}

	goodReferer := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/save", bytes.NewReader(payload))
	goodReferer.Header.Set("Referer", "http://127.0.0.1:43210/workbench/session-1")
	goodReferer.Header.Set("X-Slidex-Workbench-Token", token)
	goodRefererRecorder := httptest.NewRecorder()
	server.handleSave(goodRefererRecorder, goodReferer)
	if goodRefererRecorder.Code != http.StatusOK {
		t.Fatalf("good referer status = %d body=%s", goodRefererRecorder.Code, goodRefererRecorder.Body.String())
	}

	good := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/save", bytes.NewReader(payload))
	good.Header.Set("Origin", "http://127.0.0.1:43210")
	good.Header.Set("X-Slidex-Workbench-Token", token)
	goodRecorder := httptest.NewRecorder()
	server.handleSave(goodRecorder, good)
	if goodRecorder.Code != http.StatusOK {
		t.Fatalf("good save status = %d body=%s", goodRecorder.Code, goodRecorder.Body.String())
	}
	if brief := readFileOrEmpty(filepath.Join(deck, "brief.md")); !strings.Contains(brief, "Approve pilot") {
		t.Fatalf("brief did not include saved input: %s", brief)
	}
	manifestRaw := readFileOrEmpty(filepath.Join(deck, "out", workbenchManifestName))
	if strings.Contains(manifestRaw, token) {
		t.Fatalf("manifest leaked raw token: %s", manifestRaw)
	}
	var saved workbenchManifest
	if err := json.Unmarshal([]byte(manifestRaw), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.TokenRedacted || saved.TokenSHA256 == "" || saved.InputSavedAt == "" {
		t.Fatalf("manifest did not record redacted token/save status: %#v", saved)
	}
}

func TestWorkbenchDraftRequiresTokenAndPersistsRecovery(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := "draft-token"
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", token, 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: token, manifest: manifest}
	payload := []byte(`{"title":"Recovered draft","audience":"","decisionGoal":"","sourceNotes":"partial","outputExpectations":""}`)

	noToken := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/draft", bytes.NewReader(payload))
	noToken.Header.Set("Origin", "http://127.0.0.1:43210")
	noTokenRecorder := httptest.NewRecorder()
	server.handleDraft(noTokenRecorder, noToken)
	if noTokenRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("draft without token status = %d, want %d", noTokenRecorder.Code, http.StatusUnauthorized)
	}

	noOrigin := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/draft", bytes.NewReader(payload))
	noOrigin.Header.Set("X-Slidex-Workbench-Token", token)
	noOriginRecorder := httptest.NewRecorder()
	server.handleDraft(noOriginRecorder, noOrigin)
	if noOriginRecorder.Code != http.StatusForbidden {
		t.Fatalf("draft without origin status = %d, want %d", noOriginRecorder.Code, http.StatusForbidden)
	}

	good := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/draft", bytes.NewReader(payload))
	good.Header.Set("Origin", "http://127.0.0.1:43210")
	good.Header.Set("X-Slidex-Workbench-Token", token)
	goodRecorder := httptest.NewRecorder()
	server.handleDraft(goodRecorder, good)
	if goodRecorder.Code != http.StatusOK {
		t.Fatalf("draft save status = %d body=%s", goodRecorder.Code, goodRecorder.Body.String())
	}
	draftRaw := readFileOrEmpty(filepath.Join(deck, "out", workbenchDraftName))
	if strings.Contains(draftRaw, token) {
		t.Fatalf("draft leaked raw token: %s", draftRaw)
	}
	if !strings.Contains(draftRaw, "Recovered draft") {
		t.Fatalf("draft did not include saved input: %s", draftRaw)
	}

	get := httptest.NewRequest(http.MethodGet, "/workbench/session-1/api/draft", nil)
	get.Header.Set("Origin", "http://127.0.0.1:43210")
	get.Header.Set("X-Slidex-Workbench-Token", token)
	getRecorder := httptest.NewRecorder()
	server.handleDraft(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("draft reload status = %d body=%s", getRecorder.Code, getRecorder.Body.String())
	}
	if !strings.Contains(getRecorder.Body.String(), "Recovered draft") {
		t.Fatalf("draft reload omitted saved input: %s", getRecorder.Body.String())
	}
}

func TestWorkbenchHandlersRejectMismatchedSessionPath(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := "session-token"
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", token, 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: token, manifest: manifest}

	workbenchReq := httptest.NewRequest(http.MethodGet, "/workbench/wrong-session", nil)
	workbenchRec := httptest.NewRecorder()
	server.handleWorkbench(workbenchRec, workbenchReq)
	if workbenchRec.Code != http.StatusNotFound {
		t.Fatalf("wrong workbench session status = %d, want %d", workbenchRec.Code, http.StatusNotFound)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/workbench/wrong-session/api/session", nil)
	sessionRec := httptest.NewRecorder()
	server.handleSession(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusNotFound {
		t.Fatalf("wrong session API status = %d, want %d", sessionRec.Code, http.StatusNotFound)
	}

	noTokenSession := httptest.NewRequest(http.MethodGet, "/workbench/session-1/api/session", nil)
	noTokenSessionRecorder := httptest.NewRecorder()
	server.handleSession(noTokenSessionRecorder, noTokenSession)
	if noTokenSessionRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("session API without token status = %d, want %d", noTokenSessionRecorder.Code, http.StatusUnauthorized)
	}

	badOriginSession := httptest.NewRequest(http.MethodGet, "/workbench/session-1/api/session", nil)
	badOriginSession.Header.Set("Origin", "http://evil.example")
	badOriginSession.Header.Set("X-Slidex-Workbench-Token", token)
	badOriginSessionRecorder := httptest.NewRecorder()
	server.handleSession(badOriginSessionRecorder, badOriginSession)
	if badOriginSessionRecorder.Code != http.StatusForbidden {
		t.Fatalf("session API bad origin status = %d, want %d", badOriginSessionRecorder.Code, http.StatusForbidden)
	}

	goodSession := httptest.NewRequest(http.MethodGet, "/workbench/session-1/api/session", nil)
	goodSession.Header.Set("X-Slidex-Workbench-Token", token)
	goodSessionRecorder := httptest.NewRecorder()
	server.handleSession(goodSessionRecorder, goodSession)
	if goodSessionRecorder.Code != http.StatusOK {
		t.Fatalf("session API with token status = %d body=%s", goodSessionRecorder.Code, goodSessionRecorder.Body.String())
	}

	payload := []byte(`{"title":"Demo","audience":"Board","decisionGoal":"Approve pilot"}`)
	draftReq := httptest.NewRequest(http.MethodPost, "/workbench/wrong-session/api/draft", bytes.NewReader(payload))
	draftReq.Header.Set("Origin", "http://127.0.0.1:43210")
	draftReq.Header.Set("X-Slidex-Workbench-Token", token)
	draftRec := httptest.NewRecorder()
	server.handleDraft(draftRec, draftReq)
	if draftRec.Code != http.StatusNotFound {
		t.Fatalf("wrong draft session status = %d, want %d", draftRec.Code, http.StatusNotFound)
	}

	saveReq := httptest.NewRequest(http.MethodPost, "/workbench/wrong-session/api/save", bytes.NewReader(payload))
	saveReq.Header.Set("Origin", "http://127.0.0.1:43210")
	saveReq.Header.Set("X-Slidex-Workbench-Token", token)
	saveRec := httptest.NewRecorder()
	server.handleSave(saveRec, saveReq)
	if saveRec.Code != http.StatusNotFound {
		t.Fatalf("wrong save session status = %d, want %d", saveRec.Code, http.StatusNotFound)
	}
}

func TestWorkbenchSaveSmokeHelpers(t *testing.T) {
	html := `<script>const boot = {"deckId":"demo","sessionId":"session-1","apiBase":"/workbench/session-1/api","token":"secret-token"};</script>`
	boot, token, err := extractWorkbenchBoot(html)
	if err != nil {
		t.Fatal(err)
	}
	if token != "secret-token" || boot["apiBase"] != "/workbench/session-1/api" {
		t.Fatalf("unexpected boot parse: boot=%#v token=%q", boot, token)
	}
	apiURL := absoluteWorkbenchAPIURL("http://127.0.0.1:49152/workbench/session-1", "/workbench/session-1/api/save")
	if apiURL != "http://127.0.0.1:49152/workbench/session-1/api/save" {
		t.Fatalf("unexpected api URL: %s", apiURL)
	}
	result := workbenchSaveSmokeResult{
		StartStatus:                     "running",
		DraftStatus:                     "draft_saved",
		SaveStatus:                      "saved",
		StopStatus:                      "stopped",
		StartedNew:                      true,
		ServerBind:                      "127.0.0.1",
		TokenRedacted:                   true,
		HTMLBootstrapTokenFound:         true,
		RawTokenAbsentFromArtifacts:     true,
		IsActualCodexAppBrowserEvidence: false,
		VerifiedFiles: map[string]artifact{
			"brief":    {Path: "brief.md", SHA256: strings.Repeat("a", 64), Size: 1},
			"draft":    {Path: "workbench_draft.json", SHA256: strings.Repeat("b", 64), Size: 1},
			"manifest": {Path: "workbench_manifest.json", SHA256: strings.Repeat("c", 64), Size: 1},
		},
	}
	if status := workbenchSaveSmokeStatus(result); status != "pass" {
		t.Fatalf("status = %q, want pass", status)
	}
	result.WorkbenchScreenshot = &artifact{Path: "workbench_save_smoke.png", SHA256: strings.Repeat("d", 64), Size: 1}
	result.BrowserRendered = true
	if status := workbenchSaveSmokeStatus(result); status != "pass" {
		t.Fatalf("rendered screenshot status = %q, want pass", status)
	}
	result.BrowserRendered = false
	if status := workbenchSaveSmokeStatus(result); status != "fail" {
		t.Fatalf("blank screenshot status = %q, want fail", status)
	}
	result.WorkbenchScreenshot = nil
	result.BrowserRendered = false
	result.StartedNew = false
	result.ReusedExisting = true
	result.StopStatus = "reused_not_stopped"
	if status := workbenchSaveSmokeStatus(result); status != "pass" {
		t.Fatalf("reused status = %q, want pass", status)
	}
	result.IsActualCodexAppBrowserEvidence = true
	if status := workbenchSaveSmokeStatus(result); status != "fail" {
		t.Fatalf("save smoke must not pass as actual browser evidence: %q", status)
	}
}

func TestWorkbenchRawTokenCheckIncludesServerLog(t *testing.T) {
	dir := t.TempDir()
	token := "raw-log-token"
	brief := filepath.Join(dir, "brief.md")
	draft := filepath.Join(dir, workbenchDraftName)
	manifest := filepath.Join(dir, workbenchManifestName)
	logPath := filepath.Join(dir, "workbench_server.log")
	for _, path := range []string{brief, draft, manifest} {
		if err := os.WriteFile(path, []byte("safe\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(logPath, []byte("safe log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !rawTokenAbsentFromFiles(token, []string{brief, draft, manifest, logPath}) {
		t.Fatal("safe server log should pass raw token absence check")
	}
	if err := os.WriteFile(logPath, []byte("leaked "+token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rawTokenAbsentFromFiles(token, []string{brief, draft, manifest, logPath}) {
		t.Fatal("server log containing raw token should fail absence check")
	}
}

func TestWorkbenchHTMLShowsDeckLocalFilePaths(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	html := server.workbenchHTML()
	for _, want := range []string{
		"Deck files",
		filepath.ToSlash(filepath.Join(deck, "brief.md")),
		filepath.ToSlash(filepath.Join(deck, "out", workbenchDraftName)),
		filepath.ToSlash(filepath.Join(deck, "out", workbenchManifestName)),
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("workbench HTML missing %q:\n%s", want, html)
		}
	}
}

func TestWorkbenchSaveSmokeDoesNotStopReusedWorkbench(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portRaw, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatal(err)
	}
	token := "reused-token"
	manifest := newWorkbenchManifest(deck, workspace, "session-1", token, port, os.Getpid(), "running")
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	handler := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: token, manifest: manifest}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", handler.handleReady)
	mux.HandleFunc("/workbench/session-1", handler.handleWorkbench)
	mux.HandleFunc("/workbench/session-1/api/draft", handler.handleDraft)
	mux.HandleFunc("/workbench/session-1/api/save", handler.handleSave)
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()

	result, err := smokeSaveWorkbench(workspace, "demo", "", "decks/_template", workbenchSaveInput{
		Title:              "Reused workbench",
		Audience:           "QA",
		DecisionGoal:       "Keep the user's workbench running",
		SourceNotes:        "Existing server should be reused.",
		OutputExpectations: "save-smoke must not stop reused workbench.",
	}, workbenchSaveSmokeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" || result.StartedNew || !result.ReusedExisting || result.StopStatus != "reused_not_stopped" {
		t.Fatalf("unexpected save-smoke result: %#v", result)
	}
	recorded, ok := readWorkbenchManifest(deck)
	if !ok {
		t.Fatal("manifest missing after save-smoke")
	}
	if recorded.Status == "stopped" {
		t.Fatalf("save-smoke stopped a reused workbench: %#v", recorded)
	}
}

func TestAppServerSkillSmokeSaveInputUsesStartedWorkbenchSession(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portRaw, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatal(err)
	}
	token := "skill-smoke-token"
	manifest := newWorkbenchManifest(deck, workspace, "session-1", token, port, os.Getpid(), "running")
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	handler := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: token, manifest: manifest}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", handler.handleReady)
	mux.HandleFunc("/workbench/session-1", handler.handleWorkbench)
	mux.HandleFunc("/workbench/session-1/api/draft", handler.handleDraft)
	mux.HandleFunc("/workbench/session-1/api/save", handler.handleSave)
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()

	result, err := saveAppServerSkillSmokeInput(deck, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" || result.DraftStatus != "draft_saved" || result.SaveStatus != "saved" {
		t.Fatalf("unexpected skill-smoke save result: %#v", result)
	}
	if !result.RawTokenAbsentFromArtifacts || len(result.VerifiedFiles) != 3 {
		t.Fatalf("skill-smoke save should verify redaction and files: %#v", result)
	}
	recorded, ok := readWorkbenchManifest(deck)
	if !ok {
		t.Fatal("manifest missing after skill-smoke save")
	}
	if recorded.InputSavedAt == "" || recorded.DraftSavedAt == "" {
		t.Fatalf("manifest should record saved input timestamps: %#v", recorded)
	}
}

func TestWorkbenchHealthRejectsOriginAndOmitsCORS(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	server.handleHealth(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin health status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("health handler must not emit permissive CORS: %#v", rec.Header())
	}
	if manifest.ServerBind != "127.0.0.1" || manifest.Host != "127.0.0.1" {
		t.Fatalf("workbench must be loopback-only: %#v", manifest)
	}
}

func TestPublicWorkbenchStatusReportsActualTokenRedaction(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	status := publicWorkbenchStatus(manifest)
	if status["tokenRedacted"] != true {
		t.Fatalf("tokenRedacted = %#v, want true", status["tokenRedacted"])
	}
	manifest.TokenRedacted = false
	status = publicWorkbenchStatus(manifest)
	if status["tokenRedacted"] != false {
		t.Fatalf("tokenRedacted should reflect manifest false, got %#v", status["tokenRedacted"])
	}
	manifest.TokenRedacted = true
	manifest.TokenSHA256 = ""
	status = publicWorkbenchStatus(manifest)
	if status["tokenRedacted"] != false {
		t.Fatalf("tokenRedacted should require token hash, got %#v", status["tokenRedacted"])
	}
}

func TestDoctorWorkbenchFindingsCoverSecurityContract(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if findings := doctorWorkbenchFindings(); len(findings) != 0 {
		t.Fatalf("doctor workbench security contract findings = %#v", findings)
	}
}

func TestResolveDeckDirRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "decks"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-deck")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "decks", "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	if _, err := resolveDeckDir(workspace, "", "decks/escape", false, "decks/_template"); err == nil {
		t.Fatal("expected symlink deck path to be rejected")
	}
}

func TestWorkbenchWritesRejectSymlinkTargets(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideBrief := filepath.Join(outside, "brief.md")
	if err := os.WriteFile(outsideBrief, []byte("outside brief\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideBrief, filepath.Join(deck, "brief.md")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	input := workbenchSaveInput{Title: "Demo", Audience: "Board", DecisionGoal: "Approve pilot"}
	if err := writeWorkbenchBrief(deck, input); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink brief write to fail, got %v", err)
	}
	if got := readFileOrEmpty(outsideBrief); got != "outside brief\n" {
		t.Fatalf("outside brief was modified: %q", got)
	}

	outsideManifest := filepath.Join(outside, "manifest.json")
	if err := os.WriteFile(outsideManifest, []byte("outside manifest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideManifest, filepath.Join(deck, "out", workbenchManifestName)); err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	if err := writeWorkbenchManifest(deck, manifest); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink manifest write to fail, got %v", err)
	}
	if got := readFileOrEmpty(outsideManifest); got != "outside manifest\n" {
		t.Fatalf("outside manifest was modified: %q", got)
	}

	outsideDraft := filepath.Join(outside, "draft.json")
	if err := os.WriteFile(outsideDraft, []byte("outside draft\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDraft, filepath.Join(deck, "out", workbenchDraftName)); err != nil {
		t.Fatal(err)
	}
	if _, err := writeWorkbenchDraft(deck, input, "draft"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink draft write to fail, got %v", err)
	}
	if got := readFileOrEmpty(outsideDraft); got != "outside draft\n" {
		t.Fatalf("outside draft was modified: %q", got)
	}
}

func TestWorkbenchWritesRejectSymlinkParentDirectory(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(deck, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideOut := filepath.Join(t.TempDir(), "outside-out")
	if err := os.MkdirAll(outsideOut, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideOut, filepath.Join(deck, "out")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	if err := writeWorkbenchManifest(deck, manifest); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink parent write to fail, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outsideOut, workbenchManifestName)); !os.IsNotExist(err) {
		t.Fatalf("outside manifest should not be created, stat err=%v", err)
	}
}

func TestWorkbenchLogRejectsSymlinkTarget(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "decks", "demo", "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideLog := filepath.Join(t.TempDir(), "workbench_server.log")
	if err := os.WriteFile(outsideLog, []byte("outside log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideLog, filepath.Join(outDir, "workbench_server.log")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	f, err := openSecureAppendFile(filepath.Join(outDir, "workbench_server.log"), 0o600)
	if err == nil {
		_ = f.Close()
		t.Fatal("expected symlink log open to fail")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
	if got := readFileOrEmpty(outsideLog); got != "outside log\n" {
		t.Fatalf("outside log was modified: %q", got)
	}
}

func TestWorkbenchBrowserEvidenceRecordsVerifiedCodexSurface(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := "evidence-token"
	input := workbenchSaveInput{Title: "Demo", Audience: "Board", DecisionGoal: "Approve pilot"}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", token, 43210, 123, "running")
	if _, err := writeWorkbenchDraft(deck, input, "saved"); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkbenchBrief(deck, input); err != nil {
		t.Fatal(err)
	}
	manifest.InputSavedAt = "2026-06-09T00:00:00Z"
	manifest.BriefPath = filepath.ToSlash(filepath.Join(deck, "brief.md"))
	manifest.DraftPath = filepath.ToSlash(filepath.Join(deck, "out", workbenchDraftName))
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	screenshot := filepath.Join(workspace, "codex-browser.png")
	if err := os.WriteFile(screenshot, []byte("fake png screenshot"), 0o600); err != nil {
		t.Fatal(err)
	}

	evidence, err := recordWorkbenchBrowserEvidence(workspace, "demo", "", workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                manifest.URL,
		WorkbenchVisible:   true,
		SavedInputVerified: true,
		Notes:              "Codex App browser showed the slidex workbench.",
		ScreenshotPath:     screenshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != "verified" || !evidence.WorkbenchVisible || !evidence.SavedInputVerified {
		t.Fatalf("unexpected browser evidence: %#v", evidence)
	}
	if evidence.CodexVersion == "" || evidence.PluginName != "slidex" || evidence.PluginVersion == "" {
		t.Fatalf("evidence omitted codex/plugin runtime fields: %#v", evidence)
	}
	if evidence.Invocation != "@slidex create a deck called demo" {
		t.Fatalf("evidence invocation = %q", evidence.Invocation)
	}
	if evidence.BrowserScreenshot == nil || evidence.BrowserScreenshot.SHA256 == "" {
		t.Fatalf("evidence omitted browser screenshot artifact: %#v", evidence)
	}
	if _, err := os.Stat(filepath.Join(deck, "out", "workbench_browser_screenshot.png")); err != nil {
		t.Fatalf("browser screenshot was not copied under deck out/: %v", err)
	}
	evidencePath := filepath.Join(deck, "out", workbenchBrowserEvidenceName)
	raw := readFileOrEmpty(evidencePath)
	if !strings.Contains(raw, "codex_app_in_app_browser") {
		t.Fatalf("evidence did not record surface: %s", raw)
	}
	if strings.Contains(raw, token) {
		t.Fatalf("browser evidence leaked raw token: %s", raw)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if err := validatePayloadAgainstSchema(payload, filepath.Join(repoRootForTest(t), "schemas", "workbench_browser_evidence.schema.json")); err != nil {
		t.Fatal(err)
	}
	status, err := workbenchStatus(workspace, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	public := publicWorkbenchStatus(status)
	if public["browserEvidence"] != filepath.ToSlash(evidencePath) {
		t.Fatalf("status omitted browser evidence path: %#v", public)
	}
}

func TestWorkbenchVerifyEvidenceDetectsStaleArtifacts(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	input := workbenchSaveInput{Title: "Demo", Audience: "Board", DecisionGoal: "Approve pilot"}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", 43210, 123, "running")
	if _, err := writeWorkbenchDraft(deck, input, "saved"); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkbenchBrief(deck, input); err != nil {
		t.Fatal(err)
	}
	manifest.InputSavedAt = "2026-06-09T00:00:00Z"
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := recordWorkbenchBrowserEvidence(workspace, "demo", "", workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                manifest.URL,
		WorkbenchVisible:   true,
		SavedInputVerified: true,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := verifyWorkbenchBrowserEvidence(workspace, "demo", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" {
		t.Fatalf("fresh evidence should pass: %#v", result.Findings)
	}

	if err := os.WriteFile(filepath.Join(deck, "brief.md"), []byte("changed after evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = verifyWorkbenchBrowserEvidence(workspace, "demo", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "fail" || !strings.Contains(strings.Join(result.Findings, "\n"), "verifiedFiles.brief is stale") {
		t.Fatalf("stale brief should fail verification: %#v", result.Findings)
	}
}

func TestWorkbenchVerifyEvidenceDetectsStaleBrowserScreenshot(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	input := workbenchSaveInput{Title: "Demo", Audience: "Board", DecisionGoal: "Approve pilot"}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", 43210, 123, "running")
	if _, err := writeWorkbenchDraft(deck, input, "saved"); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkbenchBrief(deck, input); err != nil {
		t.Fatal(err)
	}
	manifest.InputSavedAt = "2026-06-09T00:00:00Z"
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	screenshot := filepath.Join(workspace, "codex-browser.png")
	if err := os.WriteFile(screenshot, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := recordWorkbenchBrowserEvidence(workspace, "demo", "", workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                manifest.URL,
		WorkbenchVisible:   true,
		SavedInputVerified: true,
		ScreenshotPath:     screenshot,
	}); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(deck, "out", "workbench_browser_screenshot.png")
	if err := os.WriteFile(copied, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := verifyWorkbenchBrowserEvidence(workspace, "demo", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "fail" || !strings.Contains(strings.Join(result.Findings, "\n"), "browser screenshot evidence is stale") {
		t.Fatalf("stale browser screenshot should fail verification: %#v", result.Findings)
	}
}

func TestWorkbenchVerifyEvidenceCanRequireBrowserScreenshot(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	input := workbenchSaveInput{Title: "Demo", Audience: "Board", DecisionGoal: "Approve pilot"}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", 43210, 123, "running")
	if _, err := writeWorkbenchDraft(deck, input, "saved"); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkbenchBrief(deck, input); err != nil {
		t.Fatal(err)
	}
	manifest.InputSavedAt = "2026-06-09T00:00:00Z"
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := recordWorkbenchBrowserEvidence(workspace, "demo", "", workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                manifest.URL,
		WorkbenchVisible:   true,
		SavedInputVerified: true,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := verifyWorkbenchBrowserEvidence(workspace, "demo", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "fail" || !strings.Contains(strings.Join(result.Findings, "\n"), "must include a browser screenshot") {
		t.Fatalf("require-screenshot should fail without screenshot artifact: %#v", result.Findings)
	}
}

func TestWorkbenchBrowserEvidenceRequiresCurrentSavedArtifacts(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", 43210, 123, "running")
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	_, err := recordWorkbenchBrowserEvidence(workspace, "demo", "", workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                manifest.URL,
		WorkbenchVisible:   true,
		SavedInputVerified: true,
	})
	if err == nil || !strings.Contains(err.Error(), "saved input") {
		t.Fatalf("expected missing saved input error, got %v", err)
	}

	manifest.InputSavedAt = "2026-06-09T00:00:00Z"
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	_, err = recordWorkbenchBrowserEvidence(workspace, "demo", "", workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                "http://127.0.0.1:9999/workbench/wrong",
		WorkbenchVisible:   true,
		SavedInputVerified: true,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected URL mismatch error, got %v", err)
	}
}

func TestMCPToolsCallUsesCodexCompatibleEnvelope(t *testing.T) {
	root := repoRootForTest(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "decks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(filepath.Join(root, "decks", "_template"), filepath.Join(workspace, "decks", "_template")); err != nil {
		t.Fatal(err)
	}
	result, err := handleMCPRequest(map[string]any{
		"method": "tools/call",
		"params": map[string]any{
			"name": "deck.bootstrap",
			"arguments": map[string]any{
				"workspace": workspace,
				"deckId":    "mcp-envelope",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("tools/call result = %#v, want object", result)
	}
	if _, ok := payload["structuredContent"]; !ok {
		t.Fatalf("tools/call result missing structuredContent: %#v", payload)
	}
	content, ok := payload["content"].([]map[string]any)
	if !ok || len(content) != 1 || content[0]["type"] != "text" || !strings.Contains(fmt.Sprint(content[0]["text"]), "mcp-envelope") {
		t.Fatalf("tools/call content is not a text envelope: %#v", payload["content"])
	}
}

func TestWorkbenchReadyValidatesSessionDeckAndPID(t *testing.T) {
	deck := filepath.ToSlash(filepath.Join(t.TempDir(), "decks", "demo"))
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		_ = writeJSONResponse(w, map[string]any{"status": "ready", "sessionId": "session-1", "deckDir": deck, "pid": os.Getpid()})
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()
	_, portRaw, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatal(err)
	}

	manifest := workbenchManifest{Host: "127.0.0.1", Port: port, SessionID: "session-1", DeckDir: deck, PID: os.Getpid()}
	if !isWorkbenchReady(manifest) {
		t.Fatal("expected matching ready response to be accepted")
	}
	manifest.SessionID = "wrong-session"
	if isWorkbenchReady(manifest) {
		t.Fatal("expected mismatched session to be rejected")
	}
}

func TestWorkbenchLoopbackContractRejectsNonCanonicalHosts(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	if manifest.Host != "127.0.0.1" || manifest.ServerBind != "127.0.0.1" || !strings.HasPrefix(manifest.URL, "http://127.0.0.1:") {
		t.Fatalf("manifest must advertise exact 127.0.0.1 loopback contract: %#v", manifest)
	}
	for _, host := range []string{"localhost", "0.0.0.0", "::1"} {
		candidate := manifest
		candidate.Host = host
		if isWorkbenchReady(candidate) {
			t.Fatalf("isWorkbenchReady accepted non-canonical host %q", host)
		}
	}
	badURL := manifest
	badURL.URL = strings.Replace(manifest.URL, "127.0.0.1", "localhost", 1)
	err := validateWorkbenchBrowserEvidenceInput(workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                badURL.URL,
		WorkbenchVisible:   true,
		SavedInputVerified: true,
	}, badURL)
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("localhost browser evidence URL should be rejected, got %v", err)
	}
}

func TestWorkbenchStatusReflectsRunningReadyServer(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		_ = writeJSONResponse(w, map[string]any{
			"status":    "ready",
			"sessionId": "session-1",
			"deckDir":   filepath.ToSlash(deck),
			"pid":       os.Getpid(),
		})
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()
	_, portRaw, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", port, os.Getpid(), "starting")
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}

	status, err := workbenchStatus(workspace, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "running" {
		t.Fatalf("status = %q, want running", status.Status)
	}
}

func TestWorkbenchServeMarksManifestStaleOnPortCollision(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portRaw, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatal(err)
	}
	if got := workbenchListenAddr(port); got != "127.0.0.1:"+portRaw {
		t.Fatalf("listen addr = %q, want 127.0.0.1:%s", got, portRaw)
	}
	if strings.Contains(workbenchListenAddr(port), "0.0.0.0") {
		t.Fatalf("workbench must not bind to all interfaces by default: %s", workbenchListenAddr(port))
	}

	err = serveWorkbench(deck, workspace, "session-1", "token", port)
	if err == nil {
		t.Fatal("expected occupied port to fail")
	}
	manifest, ok := readWorkbenchManifest(deck)
	if !ok {
		t.Fatal("manifest missing after serve collision")
	}
	if manifest.Status != "stale" || manifest.ServerBind != "127.0.0.1" || manifest.Host != "127.0.0.1" {
		t.Fatalf("serve collision did not record stale loopback manifest: %#v", manifest)
	}
}

func TestRetryWorkbenchPortAttemptsRetriesOnlyRetryableFailures(t *testing.T) {
	ports := []int{41001, 41002}
	var chosen []int
	choose := func() (int, error) {
		if len(ports) == 0 {
			t.Fatal("port chooser called too many times")
		}
		port := ports[0]
		ports = ports[1:]
		chosen = append(chosen, port)
		return port, nil
	}
	var ran []int
	err := retryWorkbenchPortAttempts(5, choose, func(port int) (bool, error) {
		ran = append(ran, port)
		if port == 41001 {
			return true, fmt.Errorf("occupied port %d", port)
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(chosen) != "[41001 41002]" || fmt.Sprint(ran) != "[41001 41002]" {
		t.Fatalf("retry did not advance ports: chosen=%v ran=%v", chosen, ran)
	}

	ports = []int{42001, 42002}
	chosen = nil
	err = retryWorkbenchPortAttempts(5, choose, func(port int) (bool, error) {
		return false, fmt.Errorf("non-retryable %d", port)
	})
	if err == nil || !strings.Contains(err.Error(), "non-retryable") {
		t.Fatalf("expected non-retryable error, got %v", err)
	}
	if len(chosen) != 1 || chosen[0] != 42001 {
		t.Fatalf("non-retryable failure should stop after one attempt, chosen=%v", chosen)
	}
}

func TestWorkbenchStatusMarksUnreadyManifestStaleAndStopRecordsStopped(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", 1, 999999, "running")
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}

	status, err := workbenchStatus(workspace, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "stale" {
		t.Fatalf("status = %q, want stale", status.Status)
	}
	recorded, ok := readWorkbenchManifest(deck)
	if !ok {
		t.Fatal("manifest missing after stale status")
	}
	if recorded.Status != "stale" {
		t.Fatalf("recorded status after status = %q, want stale", recorded.Status)
	}
	stopped, err := stopWorkbench(workspace, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "stopped" {
		t.Fatalf("stop status = %q, want stopped", stopped.Status)
	}
	recorded, ok = readWorkbenchManifest(deck)
	if !ok {
		t.Fatal("manifest missing after stop")
	}
	if recorded.Status != "stopped" {
		t.Fatalf("recorded status = %q, want stopped", recorded.Status)
	}
}

func TestWorkbenchStatusAndStopNormalizeCorruptManifestPaths(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(filepath.Join(outside, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", 1, 999999, "running")
	manifest.DeckID = "wrong"
	manifest.DeckDir = filepath.ToSlash(outside)
	manifest.OutDir = filepath.ToSlash(filepath.Join(outside, "out"))
	manifest.Paths = map[string]string{
		"brief":    filepath.ToSlash(filepath.Join(outside, "brief.md")),
		"draft":    filepath.ToSlash(filepath.Join(outside, "out", workbenchDraftName)),
		"manifest": filepath.ToSlash(filepath.Join(outside, "out", workbenchManifestName)),
	}
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}

	status, err := workbenchStatus(workspace, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if status.DeckID != "demo" || status.DeckDir != filepath.ToSlash(deck) || status.OutDir != filepath.ToSlash(filepath.Join(deck, "out")) {
		t.Fatalf("status did not normalize paths: %#v", status)
	}
	for name, path := range status.Paths {
		if !pathWithin(deck, filepath.FromSlash(path)) {
			t.Fatalf("status path %s escaped deck: %s", name, path)
		}
	}

	stopped, err := stopWorkbench(workspace, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.DeckDir != filepath.ToSlash(deck) || stopped.Status != "stopped" {
		t.Fatalf("stop did not normalize deck-local status: %#v", stopped)
	}
	if _, err := os.Stat(filepath.Join(outside, "out", workbenchManifestName)); !os.IsNotExist(err) {
		t.Fatalf("stop wrote outside deck, stat err=%v", err)
	}
	recorded, ok := readWorkbenchManifest(deck)
	if !ok {
		t.Fatal("deck-local manifest missing after stop")
	}
	if recorded.DeckDir != filepath.ToSlash(deck) || recorded.Status != "stopped" {
		t.Fatalf("deck-local manifest was not normalized: %#v", recorded)
	}
}

func TestWorkbenchLockSerializesAccessAndRejectsSymlink(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "decks", "demo", "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireWorkbenchLock(outDir)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan error, 1)
	releaseSecond := make(chan struct{})
	go func() {
		unlockSecond, err := acquireWorkbenchLock(outDir)
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
		t.Fatalf("second lock acquired before first release: %v", err)
	case <-time.After(120 * time.Millisecond):
	}
	unlock()
	select {
	case err := <-acquired:
		if err != nil {
			close(releaseSecond)
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		close(releaseSecond)
		t.Fatal("second lock did not acquire after first release")
	}
	close(releaseSecond)

	outsideLock := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.WriteFile(outsideLock, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(outDir, workbenchLockName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideLock, filepath.Join(outDir, workbenchLockName)); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	if unlock, err := acquireWorkbenchLock(outDir); err == nil {
		unlock()
		t.Fatal("expected symlink workbench lock to be rejected")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}
