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
