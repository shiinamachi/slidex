package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image/color"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
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

func TestValidateDeckIDRejectsWindowsReservedNames(t *testing.T) {
	for _, deckID := range []string{"CON", "con", "NUL", "COM1", "LPT9", "AUX.deck", "PRN.txt", "demo."} {
		if err := validateDeckID(deckID); err == nil {
			t.Fatalf("expected %q to be rejected as non-portable", deckID)
		}
	}
	for _, deckID := range []string{"customer-retention", "com10", "lpt10", "auxiliary", "demo.v1"} {
		if err := validateDeckID(deckID); err != nil {
			t.Fatalf("expected %q to be portable, got %v", deckID, err)
		}
	}
}

func TestSafeDeckDirRejectsCaseInsensitiveCollisions(t *testing.T) {
	workspace := t.TempDir()
	decksRoot := filepath.Join(workspace, "decks")
	if err := os.MkdirAll(filepath.Join(decksRoot, "Demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := safeDeckDir(workspace, "demo"); err == nil {
		t.Fatal("expected deck id differing only by case to be rejected")
	}
	if _, err := safeDeckDir(workspace, "Demo"); err != nil {
		t.Fatalf("expected exact deck id case to pass: %v", err)
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

func TestBootstrapDeckUsesEmbeddedTemplateWhenDefaultTemplateMissing(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(t.TempDir())
	result, err := bootstrapDeckWorkspace(workspace, "embedded-template", defaultDeckTemplatePath, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "created" {
		t.Fatalf("status = %q, want created", result.Status)
	}
	brief := readFileOrEmpty(filepath.Join(workspace, "decks", "embedded-template", "brief.md"))
	if !strings.Contains(brief, "# Business Document Brief") {
		t.Fatalf("embedded template brief was not copied: %q", brief)
	}
	if _, err := os.Stat(filepath.Join(workspace, "decks", "_template")); !os.IsNotExist(err) {
		t.Fatalf("test workspace should not need a filesystem template, stat err=%v", err)
	}
}

func TestEmbeddedDefaultTemplateMatchesRepoTemplate(t *testing.T) {
	root := repoRootForTest(t)
	repoTemplate := filepath.Join(root, defaultDeckTemplatePath)
	repoFiles := map[string]string{}
	if err := filepath.WalkDir(repoTemplate, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repoTemplate, filePath)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		repoFiles[filepath.ToSlash(rel)] = string(raw)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	embeddedFiles := map[string]string{}
	if err := fs.WalkDir(embeddedTemplateAssets, embeddedTemplateRoot, func(assetPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filepath.FromSlash(embeddedTemplateRoot), filepath.FromSlash(assetPath))
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		raw, err := embeddedTemplateAssets.ReadFile(assetPath)
		if err != nil {
			return err
		}
		embeddedFiles[rel] = string(raw)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(embeddedFiles) != len(repoFiles) {
		t.Fatalf("embedded template file count = %d, want %d; embedded=%v repo=%v", len(embeddedFiles), len(repoFiles), embeddedFiles, repoFiles)
	}
	for rel, want := range repoFiles {
		if got, ok := embeddedFiles[rel]; !ok || got != want {
			t.Fatalf("embedded template %s drifted from repo template", rel)
		}
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

func TestWorkbenchShutdownRequiresDedicatedTokenAndSameOrigin(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeToken := "write-token"
	shutdownToken := "shutdown-token"
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", writeToken, 43210, 123, "running")
	var called atomic.Bool
	server := &workbenchHTTPServer{
		deckAbs:       deck,
		sessionID:     "session-1",
		token:         writeToken,
		shutdownToken: shutdownToken,
		manifest:      manifest,
		shutdown:      func() { called.Store(true) },
	}

	badOrigin := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/shutdown", nil)
	badOrigin.Header.Set("Origin", "http://evil.example")
	badOrigin.Header.Set("X-Slidex-Workbench-Shutdown-Token", shutdownToken)
	badOriginRecorder := httptest.NewRecorder()
	server.handleShutdown(badOriginRecorder, badOrigin)
	if badOriginRecorder.Code != http.StatusForbidden {
		t.Fatalf("bad origin shutdown status = %d, want %d", badOriginRecorder.Code, http.StatusForbidden)
	}

	badToken := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/shutdown", nil)
	badToken.Header.Set("Origin", "http://127.0.0.1:43210")
	badToken.Header.Set("X-Slidex-Workbench-Token", writeToken)
	badTokenRecorder := httptest.NewRecorder()
	server.handleShutdown(badTokenRecorder, badToken)
	if badTokenRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad shutdown token status = %d, want %d", badTokenRecorder.Code, http.StatusUnauthorized)
	}

	good := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/shutdown", nil)
	good.Header.Set("Origin", "http://127.0.0.1:43210")
	good.Header.Set("X-Slidex-Workbench-Shutdown-Token", shutdownToken)
	goodRecorder := httptest.NewRecorder()
	server.handleShutdown(goodRecorder, good)
	if goodRecorder.Code != http.StatusOK {
		t.Fatalf("good shutdown status = %d body=%s", goodRecorder.Code, goodRecorder.Body.String())
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for !called.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !called.Load() {
		t.Fatal("shutdown callback was not called")
	}
	manifestRaw := readFileOrEmpty(filepath.Join(deck, "out", workbenchManifestName))
	if strings.Contains(manifestRaw, writeToken) || strings.Contains(manifestRaw, shutdownToken) {
		t.Fatalf("shutdown manifest leaked raw token: %s", manifestRaw)
	}
	var stopped workbenchManifest
	if err := json.Unmarshal([]byte(manifestRaw), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "stopping" {
		t.Fatalf("manifest status = %q, want stopping", stopped.Status)
	}
}

func TestWorkbenchCompleteRequiresWizardDetail(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := "complete-token"
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", token, 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: token, manifest: manifest}
	payload := []byte(`{"title":"Demo","audience":"Board","decisionGoal":"Approve pilot"}`)

	req := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/complete", bytes.NewReader(payload))
	req.Header.Set("Origin", "http://127.0.0.1:43210")
	req.Header.Set("X-Slidex-Workbench-Token", token)
	rec := httptest.NewRecorder()
	server.handleComplete(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("incomplete wizard status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "sourceNotes") || !strings.Contains(rec.Body.String(), "keyMessages") {
		t.Fatalf("completion error should list missing wizard fields: %s", rec.Body.String())
	}
}

func TestWorkbenchCompleteStartsGeneration(t *testing.T) {
	oldCommand := newWorkbenchGenerationCommand
	newWorkbenchGenerationCommand = func(deckAbs string) (*exec.Cmd, []string, error) {
		cmd := exec.Command(os.Args[0], "-test.run=TestWorkbenchGenerationHelperProcess")
		cmd.Env = append(os.Environ(), "SLIDEX_TEST_WORKBENCH_GENERATION=1")
		return cmd, []string{"slidex-test-generation", deckAbs}, nil
	}
	defer func() { newWorkbenchGenerationCommand = oldCommand }()

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := "complete-token"
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", token, 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: token, manifest: manifest}
	payload := []byte(`{
		"initialRequest":"Create an investor update deck for the Q3 pilot decision.",
		"title":"Q3 pilot decision deck",
		"audience":"Executive investment committee",
		"decisionGoal":"Approve whether to fund the Q3 pilot.",
		"sourceNotes":"Use only the confirmed customer interviews and budget notes supplied by the user.",
		"keyMessages":"Pilot scope, budget ask, implementation risk, and decision timeline.",
		"requiredClaims":"Do not claim ROI, certifications, or customer counts unless sourced.",
		"constraints":"Avoid unsupported security claims and keep confidential names out.",
		"outputExpectations":"HTML and PDF deck suitable for executive review."
	}`)

	req := httptest.NewRequest(http.MethodPost, "/workbench/session-1/api/complete", bytes.NewReader(payload))
	req.Header.Set("Origin", "http://127.0.0.1:43210")
	req.Header.Set("X-Slidex-Workbench-Token", token)
	rec := httptest.NewRecorder()
	server.handleComplete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["status"] != "generation_started" {
		t.Fatalf("complete response status = %#v", response["status"])
	}
	recorded, ok := readWorkbenchManifest(deck)
	if !ok {
		t.Fatal("manifest missing after complete")
	}
	if recorded.WizardCompletedAt == "" || recorded.GenerationStatus != "running" || recorded.GenerationPID <= 0 {
		t.Fatalf("manifest should record wizard completion and running generation: %#v", recorded)
	}
	if recorded.GenerationLogPath != filepath.ToSlash(filepath.Join(deck, "out", workbenchGenerationLogName)) {
		t.Fatalf("unexpected generation log path: %#v", recorded.GenerationLogPath)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		recorded, _ = readWorkbenchManifest(deck)
		if recorded.GenerationStatus == "completed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if recorded.GenerationStatus != "completed" || recorded.GenerationExitCode != 0 {
		t.Fatalf("generation should complete through helper process: %#v", recorded)
	}
	brief := readFileOrEmpty(filepath.Join(deck, "brief.md"))
	for _, want := range []string{"Original Plugin Request", "Key Messages", "Wizard Completion"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief missing %q:\n%s", want, brief)
		}
	}
}

func TestWorkbenchGenerationHelperProcess(t *testing.T) {
	if os.Getenv("SLIDEX_TEST_WORKBENCH_GENERATION") != "1" {
		return
	}
	fmt.Println("generation helper complete")
	os.Exit(0)
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

func TestWorkbenchControlFileStoresShutdownKeySeparately(t *testing.T) {
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeToken := "write-token"
	shutdownKey := "shutdown-key"
	manifest := newWorkbenchManifest(deck, workspace, "session-1", writeToken, 43210, 123, "running")
	if err := writeWorkbenchControl(deck, newWorkbenchControl(manifest, shutdownKey)); err != nil {
		t.Fatal(err)
	}
	raw := readFileOrEmpty(workbenchControlPath(deck))
	if strings.Contains(raw, writeToken) {
		t.Fatalf("workbench control file leaked write token: %s", raw)
	}
	if !strings.Contains(raw, shutdownKey) {
		t.Fatalf("workbench control file omitted shutdown key: %s", raw)
	}
	info, err := os.Stat(workbenchControlPath(deck))
	if err != nil {
		t.Fatal(err)
	}
	if !privateFileModeAllowed(runtime.GOOS, info.Mode().Perm()) {
		t.Fatalf("workbench control file should satisfy platform private-file policy: %s", info.Mode().Perm())
	}
	control, ok := readWorkbenchControl(deck)
	if !ok {
		t.Fatal("expected workbench control file to be readable")
	}
	if !workbenchControlMatchesManifest(control, manifest) {
		t.Fatalf("control file did not match manifest: control=%#v manifest=%#v", control, manifest)
	}
	removeWorkbenchControl(deck)
	if _, err := os.Stat(workbenchControlPath(deck)); !os.IsNotExist(err) {
		t.Fatalf("control file should be removed, stat err=%v", err)
	}
}

func TestStopWorkbenchProcessUsesHTTPShutdownBeforeSignalFallback(t *testing.T) {
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
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "write-token", port, 999999, "running")
	var stopped atomic.Bool
	var shutdownCalls atomic.Int32
	handler := &workbenchHTTPServer{
		deckAbs:       deck,
		sessionID:     "session-1",
		token:         "write-token",
		shutdownToken: "shutdown-key",
		manifest:      manifest,
		shutdown: func() {
			shutdownCalls.Add(1)
			stopped.Store(true)
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if stopped.Load() {
			http.Error(w, "stopped", http.StatusServiceUnavailable)
			return
		}
		_ = writeJSONResponse(w, map[string]any{
			"status":    "ready",
			"sessionId": manifest.SessionID,
			"deckDir":   manifest.DeckDir,
			"pid":       manifest.PID,
		})
	})
	mux.HandleFunc("/workbench/session-1/api/shutdown", handler.handleShutdown)
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()
	if err := writeWorkbenchControl(deck, newWorkbenchControl(manifest, "shutdown-key")); err != nil {
		t.Fatal(err)
	}

	stopWorkbenchProcess(manifest)
	if shutdownCalls.Load() != 1 {
		t.Fatalf("shutdown calls = %d, want 1", shutdownCalls.Load())
	}
	if !stopped.Load() {
		t.Fatal("workbench should be stopped through HTTP shutdown")
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

func TestWorkbenchHTMLBootsLocalReactWizardAssets(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	html := server.workbenchHTML()
	for _, want := range []string{
		"slidex React Wizard",
		"const boot = ",
		"/workbench/session-1/assets/react-18.3.1.production.min.js",
		"/workbench/session-1/assets/react-dom-18.3.1.production.min.js",
		"/workbench/session-1/assets/workbench-app.js",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("workbench HTML missing %q:\n%s", want, html)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/workbench/session-1/assets/workbench-app.js", nil)
	rec := httptest.NewRecorder()
	server.handleAsset(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ReactDOM.createRoot") {
		t.Fatalf("workbench app asset did not serve React app: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSeedWorkbenchDraftPersistsInitialRequest(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	updated, err := seedWorkbenchDraft(deck, manifest, workbenchSaveInput{
		InitialRequest: "Create a partner proposal deck for a June review.",
		Title:          "Partner proposal",
		Audience:       "Partnership committee",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "draft" || updated.DraftSavedAt == "" {
		t.Fatalf("seeded manifest should record draft status: %#v", updated)
	}
	draft, ok := readWorkbenchDraft(deck)
	if !ok {
		t.Fatal("seeded draft missing")
	}
	if draft.Input.InitialRequest != "Create a partner proposal deck for a June review." {
		t.Fatalf("seeded draft lost initial request: %#v", draft.Input)
	}
}

func TestWorkbenchHTMLRendersRestartRequiredBannerWithoutBlockingForm(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := markPluginRestartRequired(installRoot, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	html := server.workbenchHTML()
	for _, want := range []string{
		`data-banner-id="codex_restart_required"`,
		"slidex codex app-server plugin-smoke --json",
		`<form id="deck-form">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("workbench HTML missing %q:\n%s", want, html)
		}
	}
	if strings.Index(html, `data-banner-id="codex_restart_required"`) > strings.Index(html, `<form id="deck-form">`) {
		t.Fatal("restart banner should render before the deck form without replacing it")
	}
}

func TestWorkbenchHTMLRendersUpdatesDisabledBanner(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := filepath.Join(installRoot, ".slidex", "missing-install.json")
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	html := server.workbenchHTML()
	for _, want := range []string{
		`data-banner-id="updates_disabled"`,
		"Automatic updates disabled",
		"local-development",
		`<form id="deck-form">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("workbench HTML missing %q:\n%s", want, html)
		}
	}
}

func TestWorkbenchHTMLRendersPluginDriftBanner(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := markPluginDrift(installRoot, toolVersion, filepath.Join(t.TempDir(), "plugins", "slidex", "skills", "slidex-start", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	html := server.workbenchHTML()
	if !strings.Contains(html, `data-banner-id="codex_plugin_drift"`) {
		t.Fatalf("workbench HTML missing drift banner:\n%s", html)
	}
	if !strings.Contains(html, `data-banner-id="codex_restart_required"`) {
		t.Fatalf("workbench HTML should keep restart banner with drift:\n%s", html)
	}
}

func TestWorkbenchHTMLRendersPluginVerifiedBanner(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	pluginPath := filepath.Join(installRoot, "plugins", "slidex")
	skillPath := filepath.Join(pluginPath, "skills", "slidex-start", "SKILL.md")
	if err := markPluginVerified(installRoot, toolVersion+"+codex.test", pluginPath, skillPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	html := server.workbenchHTML()
	if !strings.Contains(html, `data-banner-id="codex_plugin_verified"`) {
		t.Fatalf("workbench HTML missing verified banner:\n%s", html)
	}
	if strings.Contains(html, `data-banner-id="codex_restart_required"`) {
		t.Fatalf("verified workbench HTML should not keep restart banner:\n%s", html)
	}
	status := publicWorkbenchStatus(manifest)
	update, ok := status["update"].(map[string]any)
	if !ok || update["verifiedPluginPath"] != filepath.ToSlash(pluginPath) || update["verifiedStartSkillPath"] != filepath.ToSlash(skillPath) {
		t.Fatalf("public workbench status missing verified plugin evidence: %#v", status["update"])
	}
	banners, ok := status["statusBanners"].([]statusBanner)
	if !ok || !hasStatusBannerForTest(banners, "codex_plugin_verified") {
		t.Fatalf("public workbench status missing verified banner: %#v", status["statusBanners"])
	}
}

func TestWorkbenchHTMLRendersPendingActivationBanner(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(t.TempDir(), "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	html := server.workbenchHTML()
	for _, want := range []string{
		`data-banner-id="pending_update_activation"`,
		"activate-pending",
		`<form id="deck-form">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("workbench HTML missing %q:\n%s", want, html)
		}
	}
	if strings.Index(html, `data-banner-id="pending_update_activation"`) > strings.Index(html, `<form id="deck-form">`) {
		t.Fatal("pending activation banner should render before the deck form without replacing it")
	}
}

func TestWorkbenchHTMLStatusBannersDoNotOverlapDeckForm(t *testing.T) {
	chromePath, err := resolveChrome("")
	if err != nil {
		if renderSmokeRequired() {
			t.Fatalf("Chrome/Chromium is required for workbench layout smoke: %v", err)
		}
		t.Skipf("Chrome/Chromium is not available: %v", err)
	}

	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion+"-canary.20260610010000"))
	if err := markPluginRestartRequired(installRoot, "0.2.0-canary.20260610020000", "v0.2.0-canary.20260610020000"); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(t.TempDir(), "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0-canary.20260610020000")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0-canary.20260610020000", "v0.2.0-canary.20260610020000"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	server := &workbenchHTTPServer{deckAbs: deck, sessionID: "session-1", token: "token", manifest: manifest}
	htmlText := server.workbenchHTML()
	for _, want := range []string{
		`data-banner-id="canary_channel"`,
		`data-banner-id="codex_restart_required"`,
		`data-banner-id="pending_update_activation"`,
		`<form id="deck-form">`,
	} {
		if !strings.Contains(htmlText, want) {
			t.Fatalf("workbench HTML missing %q:\n%s", want, htmlText)
		}
	}

	for _, viewport := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "desktop", width: 1280, height: 900},
		{name: "mobile", width: 390, height: 900},
	} {
		t.Run(viewport.name, func(t *testing.T) {
			report, err := probeWorkbenchLayoutWithChrome(t, chromePath, htmlText, viewport.width, viewport.height)
			if err != nil {
				if isChromeSandboxEnvironmentFailure(err) {
					if renderSmokeRequired() {
						t.Fatalf("Chrome workbench layout smoke is required but Chrome could not render in this sandbox: %v", err)
					}
					t.Skipf("Chrome cannot render in this sandbox: %v", err)
				}
				t.Fatal(err)
			}
			if report.Form.Width <= 0 || report.Form.Height <= 0 {
				t.Fatalf("deck form did not render with measurable dimensions: %#v", report.Form)
			}
			if len(report.Banners) < 3 {
				t.Fatalf("expected at least three status banners, got %#v", report.Banners)
			}
			if len(report.Overlaps) > 0 {
				t.Fatalf("status banners overlap deck form at %dx%d: %v", viewport.width, viewport.height, report.Overlaps)
			}
			var maxBannerBottom float64
			for _, banner := range report.Banners {
				if banner.Rect.Width <= 0 || banner.Rect.Height <= 0 {
					t.Fatalf("status banner %q did not render with measurable dimensions: %#v", banner.ID, banner.Rect)
				}
				if banner.Rect.Bottom > maxBannerBottom {
					maxBannerBottom = banner.Rect.Bottom
				}
			}
			if report.Form.Top < maxBannerBottom {
				t.Fatalf("deck form starts before status banners end at %dx%d: form top %.2f, banner bottom %.2f", viewport.width, viewport.height, report.Form.Top, maxBannerBottom)
			}
		})
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

func TestPublicWorkbenchStatusIncludesBrowserOpenIntent(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	status := publicWorkbenchStatus(manifest)
	browserOpen, ok := status["browserOpen"].(map[string]any)
	if !ok {
		t.Fatalf("public status missing browserOpen intent: %#v", status)
	}
	if browserOpen["target"] != "codex_app_in_app_browser" {
		t.Fatalf("unexpected browserOpen target: %#v", browserOpen)
	}
	if browserOpen["preferredAction"] != "browser_plugin_navigation" || browserOpen["toolHint"] != "@Browser" {
		t.Fatalf("browserOpen should prefer Browser plugin navigation: %#v", browserOpen)
	}
	if browserOpen["url"] != manifest.URL || browserOpen["serverBind"] != "127.0.0.1" {
		t.Fatalf("browserOpen should carry loopback workbench URL: %#v", browserOpen)
	}
	if browserOpen["directClientRequestAPI"] != "not_available_in_codex_app_server_0.138.0" {
		t.Fatalf("browserOpen should not claim direct App Server browser-open support: %#v", browserOpen)
	}
	if !strings.Contains(fmt.Sprint(status["browserOpenStrategy"]), "Browser plugin") {
		t.Fatalf("browserOpenStrategy should mention Browser plugin: %#v", status["browserOpenStrategy"])
	}
}

func TestPublicWorkbenchStatusIncludesUpdateBanners(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion+"-canary.20260610010000"))
	if err := markPluginRestartRequired(installRoot, "0.2.0-canary.20260610020000", "v0.2.0-canary.20260610020000"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	status := publicWorkbenchStatus(manifest)
	update, ok := status["update"].(map[string]any)
	if !ok {
		t.Fatalf("missing update snapshot: %#v", status)
	}
	if update["channel"] != updateChannelCanary || update["restartRequired"] != true {
		t.Fatalf("unexpected update snapshot: %#v", update)
	}
	banners, ok := status["statusBanners"].([]statusBanner)
	if !ok {
		t.Fatalf("missing status banners: %#v", status["statusBanners"])
	}
	if !hasStatusBannerForTest(banners, "canary_channel") || !hasStatusBannerForTest(banners, "codex_restart_required") {
		t.Fatalf("missing update banners: %#v", banners)
	}
}

func TestPublicWorkbenchStatusIncludesPendingActivationBanner(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	candidate := filepath.Join(t.TempDir(), "candidate")
	writeCandidateBundleForTest(t, candidate, "0.2.0")
	if _, _, err := stagePendingUpdateHandoff(installRoot, candidate, "0.2.0", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	deck := filepath.Join(t.TempDir(), "decks", "demo")
	manifest := newWorkbenchManifest(deck, filepath.Dir(filepath.Dir(deck)), "session-1", "token", 43210, 123, "running")
	status := publicWorkbenchStatus(manifest)
	update, ok := status["update"].(map[string]any)
	if !ok {
		t.Fatalf("missing update snapshot: %#v", status)
	}
	if update["pendingActivation"] != true || update["pendingActivationCommand"] == "" {
		t.Fatalf("pending activation missing from update snapshot: %#v", update)
	}
	banners, ok := status["statusBanners"].([]statusBanner)
	if !ok {
		t.Fatalf("missing status banners: %#v", status["statusBanners"])
	}
	if !hasStatusBannerForTest(banners, "pending_update_activation") {
		t.Fatalf("missing pending activation banner: %#v", banners)
	}
}

func TestWorkbenchAutoUpdatePreflightLocalDevelopmentContinuesWithoutFetch(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := filepath.Join(installRoot, ".slidex", "missing-install.json")
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not fetch release metadata for local-development", http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv(updateAPIURLEnv, server.URL)

	result := runWorkbenchAutoUpdatePreflight(context.Background())
	if result.Status != "disabled" || !result.ContinueToWorkbench || result.BlocksWorkbench {
		t.Fatalf("local-development should continue to workbench without blocking: %#v", result)
	}
	if called {
		t.Fatal("local-development update preflight should not fetch release metadata")
	}
}

func TestMCPWorkbenchStartAutoAppliesReleaseUpdateBeforeWizard(t *testing.T) {
	parent := t.TempDir()
	installRoot := filepath.Join(parent, "slidex")
	if err := os.MkdirAll(filepath.Join(installRoot, ".slidex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "VERSION"), []byte(toolVersion), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInstallMetadataForTest(t, installMetadataPath(installRoot), releaseInstallMetadataForTest(t, toolVersion))
	targetVersion := "0.2.0"
	if channelFromPackageVersion(toolVersion) == updateChannelCanary {
		targetVersion = "0.2.0-canary.20260611120000"
	}
	candidate := filepath.Join(parent, "candidate")
	writeCandidateBundleForTest(t, candidate, targetVersion)
	server := updateReleaseServerForCandidateForTest(t, candidate, targetVersion)
	defer server.Close()
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, installMetadataPath(installRoot))
	t.Setenv(updateAPIURLEnv, server.URL+"/releases")

	workspace := filepath.Join(parent, "workspace")
	result, err := callMCPWorkbenchStart(map[string]any{
		"workspace": workspace,
		"deckId":    "auto-update",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("workbench.start result = %#v, want object", result)
	}
	if payload["workbenchStarted"] != false {
		t.Fatalf("workbench should not start after applying an update: %#v", payload)
	}
	autoUpdate, ok := payload["autoUpdate"].(workbenchAutoUpdateResult)
	if !ok {
		t.Fatalf("autoUpdate missing from workbench.start result: %#v", payload)
	}
	if !autoUpdate.BlocksWorkbench || autoUpdate.ContinueToWorkbench {
		t.Fatalf("applied update should block the current workbench startup: %#v", autoUpdate)
	}
	if autoUpdate.TargetVersion != targetVersion {
		t.Fatalf("auto update target = %#v, want %s", autoUpdate, targetVersion)
	}
	if runtime.GOOS == "windows" {
		if autoUpdate.Status != "pending_activation" || !autoUpdate.PendingActivation {
			t.Fatalf("windows auto update should require pending activation: %#v", autoUpdate)
		}
	} else {
		if autoUpdate.Status != "applied_restart_required" || !autoUpdate.RestartRequired {
			t.Fatalf("auto update should require restart after apply: %#v", autoUpdate)
		}
		if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(installRoot, "VERSION"))); got != targetVersion {
			t.Fatalf("install root VERSION after auto update = %q, want %s", got, targetVersion)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "decks", "auto-update", "out", workbenchManifestName)); !os.IsNotExist(err) {
		t.Fatalf("workbench manifest should not be created while update blocks startup, stat err=%v", err)
	}
}

func hasStatusBannerForTest(banners []statusBanner, id string) bool {
	for _, banner := range banners {
		if banner.ID == id {
			return true
		}
	}
	return false
}

type workbenchLayoutReport struct {
	Form     workbenchLayoutRect     `json:"form"`
	Banners  []workbenchBannerLayout `json:"banners"`
	Overlaps []string                `json:"overlaps"`
}

type workbenchBannerLayout struct {
	ID   string              `json:"id"`
	Rect workbenchLayoutRect `json:"rect"`
}

type workbenchLayoutRect struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func probeWorkbenchLayoutWithChrome(t *testing.T, chromePath, htmlText string, width, height int) (workbenchLayoutReport, error) {
	t.Helper()
	const markerStart = `<script id="slidex-workbench-layout" type="application/json">`
	const markerEnd = `</script>`
	probeScript := `<script>
(() => {
  const rectFor = (element) => {
    const rect = element.getBoundingClientRect();
    return {left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom, width: rect.width, height: rect.height};
  };
  const form = document.getElementById("deck-form");
  const formRect = rectFor(form);
  const banners = Array.from(document.querySelectorAll("[data-banner-id]")).map((element) => ({
    id: element.getAttribute("data-banner-id"),
    rect: rectFor(element)
  }));
  const overlaps = banners.filter((banner) => !(
    banner.rect.right <= formRect.left ||
    banner.rect.left >= formRect.right ||
    banner.rect.bottom <= formRect.top ||
    banner.rect.top >= formRect.bottom
  )).map((banner) => banner.id);
  document.body.insertAdjacentHTML("beforeend", '` + markerStart + `' + JSON.stringify({form: formRect, banners, overlaps}) + '<\/script>');
})();
</script>`
	if strings.Contains(htmlText, markerStart) {
		return workbenchLayoutReport{}, fmt.Errorf("workbench HTML already contains layout probe marker")
	}
	probedHTML := strings.Replace(htmlText, "</body>", probeScript+"</body>", 1)
	if probedHTML == htmlText {
		return workbenchLayoutReport{}, fmt.Errorf("workbench HTML missing closing body tag")
	}
	htmlPath := filepath.Join(t.TempDir(), "workbench.html")
	if err := os.WriteFile(htmlPath, []byte(probedHTML), 0o600); err != nil {
		return workbenchLayoutReport{}, err
	}

	args, cleanup, err := chromeHeadlessBaseArgs(false)
	if err != nil {
		return workbenchLayoutReport{}, err
	}
	defer cleanup()
	args = append(args,
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--virtual-time-budget=3000",
		"--dump-dom",
		fileURLFromPath(htmlPath),
	)
	out, err := runChromeCommand(chromeCommandTimeout, chromePath, args...)
	output := string(out)
	start := strings.LastIndex(output, markerStart)
	if err != nil && !(isChromeCommandTimeout(err) && start >= 0) {
		return workbenchLayoutReport{}, fmt.Errorf("chrome workbench layout probe failed: %w\n%s", err, output)
	}
	if start < 0 {
		return workbenchLayoutReport{}, fmt.Errorf("layout probe report missing from dumped DOM:\n%s", output)
	}
	start += len(markerStart)
	end := strings.Index(output[start:], markerEnd)
	if end < 0 {
		return workbenchLayoutReport{}, fmt.Errorf("layout probe report is missing closing marker:\n%s", output)
	}
	var report workbenchLayoutReport
	if err := json.Unmarshal([]byte(html.UnescapeString(output[start:start+end])), &report); err != nil {
		return workbenchLayoutReport{}, err
	}
	return report, nil
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

func TestSecureTruncateRejectsSymlinkTarget(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideLog := filepath.Join(t.TempDir(), "codex-app-server.stdout.log")
	if err := os.WriteFile(outsideLog, []byte("outside log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideLog, filepath.Join(outDir, "codex-app-server.stdout.log")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	f, err := openSecureTruncateFile(filepath.Join(outDir, "codex-app-server.stdout.log"), 0o600)
	if err == nil {
		_ = f.Close()
		t.Fatal("expected symlink truncate open to fail")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
	if got := readFileOrEmpty(outsideLog); got != "outside log\n" {
		t.Fatalf("outside log was modified: %q", got)
	}
}

func TestVerifySecureOpenFileRejectsSymlinkAfterOpen(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.log")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	f, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := verifySecureOpenFile(link, f); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected post-open symlink rejection, got %v", err)
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
	writeSolidPNGForTest(t, screenshot, color.RGBA{R: 12, G: 34, B: 56, A: 255})

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

func TestWorkbenchBrowserEvidenceRejectsInvalidBrowserScreenshot(t *testing.T) {
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
	manifest.BriefPath = filepath.ToSlash(filepath.Join(deck, "brief.md"))
	manifest.DraftPath = filepath.ToSlash(filepath.Join(deck, "out", workbenchDraftName))
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	screenshot := filepath.Join(workspace, "codex-browser.png")
	if err := os.WriteFile(screenshot, []byte("fake png screenshot"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := recordWorkbenchBrowserEvidence(workspace, "demo", "", workbenchBrowserEvidenceInput{
		Inspector:          "QA",
		Surface:            "codex_app_in_app_browser",
		Invocation:         "@slidex create a deck called demo",
		URL:                manifest.URL,
		WorkbenchVisible:   true,
		SavedInputVerified: true,
		ScreenshotPath:     screenshot,
	})
	if err == nil || !strings.Contains(err.Error(), "decodable PNG or JPEG") {
		t.Fatalf("invalid screenshot should fail, got %v", err)
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
	normalizeWorkbenchEvidenceRuntimeForTest(t, deck)

	result, err := verifyWorkbenchBrowserEvidence(workspace, "demo", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" {
		t.Fatalf("fresh evidence should pass: %#v", result.Findings)
	}
	verificationPath := filepath.Join(deck, "out", workbenchBrowserVerifyName)
	raw := readFileOrEmpty(verificationPath)
	if !strings.Contains(raw, "slidex.workbenchBrowserEvidenceVerification.v1") {
		t.Fatalf("verification result was not written: %s", raw)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if err := validatePayloadAgainstSchema(payload, filepath.Join(repoRootForTest(t), "schemas", "workbench_browser_evidence_verification.schema.json")); err != nil {
		t.Fatal(err)
	}
	status, err := workbenchStatus(workspace, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if public := publicWorkbenchStatus(status); public["browserEvidenceVerification"] != filepath.ToSlash(verificationPath) {
		t.Fatalf("status omitted browser evidence verification path: %#v", public)
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
	raw = readFileOrEmpty(verificationPath)
	if !strings.Contains(raw, `"status": "fail"`) || !strings.Contains(raw, "verifiedFiles.brief is stale") {
		t.Fatalf("stale verification result was not written: %s", raw)
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
	writeSolidPNGForTest(t, screenshot, color.RGBA{R: 12, G: 34, B: 56, A: 255})
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
	normalizeWorkbenchEvidenceRuntimeForTest(t, deck)
	copied := filepath.Join(deck, "out", "workbench_browser_screenshot.png")
	writeSolidPNGForTest(t, copied, color.RGBA{R: 90, G: 20, B: 10, A: 255})

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
	normalizeWorkbenchEvidenceRuntimeForTest(t, deck)

	result, err := verifyWorkbenchBrowserEvidence(workspace, "demo", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "fail" || !strings.Contains(strings.Join(result.Findings, "\n"), "must include a browser screenshot") {
		t.Fatalf("require-screenshot should fail without screenshot artifact: %#v", result.Findings)
	}
}

func normalizeWorkbenchEvidenceRuntimeForTest(t *testing.T, deck string) {
	t.Helper()
	path := filepath.Join(deck, "out", workbenchBrowserEvidenceName)
	var evidence workbenchBrowserEvidence
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(evidence.CodexVersion) == "" || evidence.CodexVersion == "unavailable" {
		evidence.CodexVersion = requiredCodexVersion
	}
	if strings.TrimSpace(evidence.PluginVersion) == "" || evidence.PluginVersion == "unavailable" {
		evidence.PluginVersion = firstNonEmpty(localPluginVersion(), toolVersion)
	}
	if err := secureWriteJSON(path, evidence); err != nil {
		t.Fatal(err)
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
	workspace := t.TempDir()
	deck := filepath.Join(workspace, "decks", "mcp-envelope")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o755); err != nil {
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
	manifest := newWorkbenchManifest(deck, workspace, "session-1", "token", port, os.Getpid(), "running")
	if err := writeWorkbenchManifest(deck, manifest); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		_ = writeJSONResponse(w, map[string]any{
			"status":    "ready",
			"sessionId": manifest.SessionID,
			"deckDir":   manifest.DeckDir,
			"pid":       manifest.PID,
		})
	})
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()

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
	structured, ok := payload["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call result missing structuredContent: %#v", payload)
	}
	if _, ok := structured["browserOpen"].(map[string]any); !ok {
		t.Fatalf("deck.bootstrap should return React wizard browserOpen intent: %#v", structured)
	}
	content, ok := payload["content"].([]map[string]any)
	if !ok || len(content) != 1 || content[0]["type"] != "text" || !strings.Contains(fmt.Sprint(content[0]["text"]), "Open in Codex App Browser now:") {
		t.Fatalf("tools/call content is not a text envelope: %#v", payload["content"])
	}
}

func TestMCPRenderToolSchemaExposesChromeOptions(t *testing.T) {
	result, err := handleMCPRequest(map[string]any{"method": "tools/list"})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result = %#v, want object", result)
	}
	tools, ok := payload["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools/list missing tools: %#v", payload)
	}
	for _, tool := range tools {
		if tool["name"] != "render" {
			continue
		}
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Fatalf("render tool inputSchema = %#v", tool["inputSchema"])
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("render tool properties = %#v", schema["properties"])
		}
		chrome, ok := props["chrome"].(map[string]any)
		if !ok || chrome["type"] != "string" {
			t.Fatalf("render tool chrome schema = %#v", props["chrome"])
		}
		chromeNoSandbox, ok := props["chromeNoSandbox"].(map[string]any)
		if !ok || chromeNoSandbox["type"] != "boolean" {
			t.Fatalf("render tool chromeNoSandbox schema = %#v", props["chromeNoSandbox"])
		}
		return
	}
	t.Fatal("render MCP tool missing from tools/list")
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

	err = serveWorkbench(deck, workspace, "session-1", "token", "shutdown-token", port)
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
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
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
		<-secondDone
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
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second lock did not release")
	}

	outsideLock := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.WriteFile(outsideLock, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(outDir, workbenchLockName)); err != nil && !os.IsNotExist(err) {
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
