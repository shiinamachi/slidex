package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
