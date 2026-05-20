package main

import (
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
	if _, err := writeDeliverySummary(deck); err != nil {
		t.Fatal(err)
	}
	pkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	if pkg["status"] != "pass" {
		t.Fatalf("package should pass, got %#v", pkg)
	}
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
