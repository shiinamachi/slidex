package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	badToken := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(payload))
	badToken.Header.Set("Origin", "http://127.0.0.1:43210")
	badToken.Header.Set("X-Slidex-Workbench-Token", "wrong")
	badTokenRecorder := httptest.NewRecorder()
	server.handleSave(badTokenRecorder, badToken)
	if badTokenRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d, want %d", badTokenRecorder.Code, http.StatusUnauthorized)
	}

	badOrigin := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(payload))
	badOrigin.Header.Set("Origin", "http://evil.example")
	badOrigin.Header.Set("X-Slidex-Workbench-Token", token)
	badOriginRecorder := httptest.NewRecorder()
	server.handleSave(badOriginRecorder, badOrigin)
	if badOriginRecorder.Code != http.StatusForbidden {
		t.Fatalf("bad origin status = %d, want %d", badOriginRecorder.Code, http.StatusForbidden)
	}

	good := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(payload))
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
