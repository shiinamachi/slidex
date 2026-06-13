package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	xhtml "golang.org/x/net/html"
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

func TestExtractProbeJSONScriptRequiresMatchingNonce(t *testing.T) {
	src := `<!doctype html><html><body>
<script id="slidex-slide-enumeration" type="application/json">[{"id":"fake"}]</script>
<script id="slidex-slide-enumeration" type="application/json" data-slidex-probe="real-nonce">[{&quot;id&quot;:&quot;real&quot;}]</script>
</body></html>`

	payload, found := extractProbeJSONScript(src, "slidex-slide-enumeration", "real-nonce")
	if !found {
		t.Fatal("expected matching probe report to be found")
	}
	if payload != `[{"id":"real"}]` {
		t.Fatalf("unexpected payload: %s", payload)
	}

	if _, found := extractProbeJSONScript(src, "slidex-slide-enumeration", "missing-nonce"); found {
		t.Fatal("probe report with missing nonce should not be found")
	}
}

func TestBuildSlideWrapperMarksOverflowReportWithNonce(t *testing.T) {
	wrapper := buildSlideWrapper("", `<section class="slide"><p>ok</p></section>`, 1600, 900, "pretendard", "overflow-nonce")
	if !strings.Contains(wrapper, `report.setAttribute('data-slidex-probe', "overflow-nonce");`) {
		t.Fatalf("overflow probe nonce was not embedded in wrapper:\n%s", wrapper)
	}
}

func TestStripExecutableHTMLForProbeRemovesActiveContent(t *testing.T) {
	src := `<!doctype html><html><head>
<script>window.evil = true;</script>
<meta http-equiv="refresh" content="0;url=https://example.com">
<link rel="import" href="https://example.com/import.html">
<link rel="modulepreload" href="evil.js">
<link rel="preload" as="script" href="evil.js">
<style>.slide{color:red}</style>
</head><body onload="evil()">
<section class="slide" onclick="evil()">
  <a href=" javascript:evil()">bad link</a>
  <img src="x" onerror="evil()">
  <iframe srcdoc="<script>parent.evil()</script>"></iframe>
  <object data="x"></object>
  <embed src="x">
  <p>Static copy</p>
</section>
</body></html>`
	got := stripExecutableHTMLForProbe(src)
	lower := strings.ToLower(got)
	for _, forbidden := range []string{"<script", "onload", "onclick", "onerror", "javascript:", "srcdoc", "<iframe", "<object", "<embed", "http-equiv", "rel=\"import\"", "modulepreload", "evil.js"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("probe HTML still contains executable content %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, `.slide{color:red}`) || !strings.Contains(got, "Static copy") {
		t.Fatalf("probe sanitizer removed static content:\n%s", got)
	}
	slides := extractSlides(got)
	if len(slides) != 1 {
		t.Fatalf("sanitized probe HTML should preserve one slide, got %d", len(slides))
	}
	if strings.Contains(strings.ToLower(slides[0].FullHTML), "<script") {
		t.Fatalf("sanitized slide retained script HTML:\n%s", slides[0].FullHTML)
	}
}

func TestRenderHTMLRejectsSymlinkSourceHTML(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.html")
	if err := os.WriteFile(outside, []byte(`<!doctype html><html><body><section class="slide">outside</section></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "final_deck.html")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, err := renderHTML(renderConfig{HTMLPath: link})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlinked source HTML rejection, got %v", err)
	}
	if _, _, err := extractSlidesWithChrome("", link, ".slide", false); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected Chrome enumeration source HTML rejection, got %v", err)
	}
}

func TestParseInitArgsAcceptsFromTemplateAroundDeckID(t *testing.T) {
	for _, args := range [][]string{
		{"demo", "--from-template", "custom_template"},
		{"--from-template", "custom_template", "demo"},
		{"demo", "--from-template=custom_template"},
		{"--from-template=custom_template", "demo"},
	} {
		deckID, fromTemplate, err := parseInitArgs(args)
		if err != nil {
			t.Fatalf("parseInitArgs(%v) failed: %v", args, err)
		}
		if deckID != "demo" || fromTemplate != "custom_template" {
			t.Fatalf("parseInitArgs(%v) = %q %q", args, deckID, fromTemplate)
		}
	}
}

func TestParseInitArgsRejectsAmbiguousInput(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"demo", "extra"},
		{"demo", "--from-template"},
		{"demo", "--unknown"},
	} {
		if _, _, err := parseInitArgs(args); err == nil {
			t.Fatalf("parseInitArgs(%v) should fail", args)
		}
	}
}

func TestRunInitCreatesDeckWorkspaceFromTemplate(t *testing.T) {
	root := repoRootForTest(t)
	workspace := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if err := runInit([]string{"portable-init", "--from-template", filepath.Join(root, "decks", "_template")}); err != nil {
		t.Fatal(err)
	}
	deckDir := filepath.Join(workspace, "decks", "portable-init")
	for _, rel := range []string{
		"brief.md",
		"DESIGN.md",
		filepath.Join("assets", "README.md"),
		filepath.Join("brand", "README.md"),
		filepath.Join("data", "README.md"),
		filepath.Join("source", "README.md"),
	} {
		if _, err := os.Stat(filepath.Join(deckDir, rel)); err != nil {
			t.Fatalf("init did not create %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "brief.md")); !os.IsNotExist(err) {
		t.Fatalf("init should not create root-level legacy brief.md, got err=%v", err)
	}
}

func TestRunPipelineRejectsInvalidUntilBeforeSideEffects(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(deck, 0o755); err != nil {
		t.Fatal(err)
	}
	err := runPipeline([]string{"--deck", deck, "--until", "bogus"})
	if err == nil || !strings.Contains(err.Error(), "--until must be one of") {
		t.Fatalf("expected invalid --until error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(deck, "out")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid --until should fail before creating out directory, stat err=%v", statErr)
	}
}

func TestRunBufferedCommandReportsTimeout(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := runBufferedCommand(50*time.Millisecond, exe, "-test.run=^TestRunBufferedCommandHelperProcess$", "--", "timeout")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got out=%q err=%v", out, err)
	}
	if !strings.Contains(string(out), "ready") {
		t.Fatalf("expected buffered output before timeout, got %q", out)
	}
}

func TestRunBufferedCommandUsesDirAndStdin(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out, err := runBufferedCommandWithInput(time.Second, dir, strings.NewReader("payload"), exe, "-test.run=^TestRunBufferedCommandHelperProcess$", "--", "dir-stdin")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != dir {
		t.Fatalf("command dir output = %q, want %q", strings.TrimSpace(string(out)), dir)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "result.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "payload" {
		t.Fatalf("stdin was not passed through: %q", raw)
	}
}

func TestRunBufferedCommandRejectsOversizedOutput(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := runBufferedCommandWithInputAndMaxOutput(time.Second, 128, "", nil, exe, "-test.run=^TestRunBufferedCommandHelperProcess$", "--", "large-output")
	if err == nil || !strings.Contains(err.Error(), "output exceeded") {
		t.Fatalf("expected output cap error, got out=%d err=%v", len(out), err)
	}
	if len(out) > 128 {
		t.Fatalf("retained output should be capped, got %d bytes", len(out))
	}
}

func TestRunBufferedCommandHelperProcess(t *testing.T) {
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	switch mode {
	case "timeout":
		fmt.Print("ready")
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "dir-stdin":
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v", err)
			os.Exit(2)
		}
		if err := os.WriteFile("result.txt", raw, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write result: %v", err)
			os.Exit(2)
		}
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "get cwd: %v", err)
			os.Exit(2)
		}
		fmt.Print(cwd)
		os.Exit(0)
	case "large-output":
		for i := 0; i < 2048; i++ {
			fmt.Fprint(os.Stdout, "o")
			fmt.Fprint(os.Stderr, "e")
		}
		os.Exit(0)
	default:
		return
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
		if renderSmokeRequired() {
			t.Fatalf("Chrome/Chromium is required for cross-platform render smoke: %v", err)
		}
		t.Skipf("Chrome/Chromium is not available: %v", err)
	}

	deck := filepath.Join(t.TempDir(), "workspace with spaces", "minimal_deck")
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
			if renderSmokeRequired() {
				t.Fatalf("Chrome render smoke is required but Chrome could not render in this sandbox: %v", err)
			}
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
	var storedManifest renderManifest
	manifestRaw, err := os.ReadFile(filepath.Join(outDir, "render_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestRaw, &storedManifest); err != nil {
		t.Fatal(err)
	}
	if storedManifest.SourceHTML.Path != "out/final_deck.html" ||
		storedManifest.PDF.Path != "out/final_deck.pdf" ||
		storedManifest.QAMontage.Path != "out/qa_montage.png" ||
		len(storedManifest.PNGFiles) == 0 ||
		!strings.HasPrefix(storedManifest.PNGFiles[0].Path, "out/rendered_slides/") {
		t.Fatalf("stored render manifest should use deck-relative slash paths: %#v", storedManifest)
	}
	qa, err := qaDeckWithVisualReviewRunner(deck, true, "manual", func(string, renderManifest, string) (string, []qaFinding) {
		return "pass", nil
	})
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
	for _, stage := range structuredReviewStages() {
		if _, err := writeStructuredReview(deck, stage, 1); err != nil {
			t.Fatal(err)
		}
	}
	pkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	if pkg["status"] != "pass" {
		t.Fatalf("package should pass, got %#v", pkg)
	}
	specPath := filepath.Join(outDir, "deck_spec.json")
	manifestPath := filepath.Join(outDir, "render_manifest.json")
	qaReportPath := filepath.Join(outDir, "qa_report.md")
	visualReviewPath := filepath.Join(outDir, "visual_reviews", "latest_review.json")
	originalSpec := readFileOrEmpty(specPath)
	originalManifest := readFileOrEmpty(manifestPath)
	originalQAReport := readFileOrEmpty(qaReportPath)

	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manifestPath, 0o755); err != nil {
		t.Fatal(err)
	}
	badManifestPkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	badManifestFindings, _ := badManifestPkg["findings"].([]qaFinding)
	if badManifestPkg["status"] != "fail" ||
		!hasFindingCheck(badManifestFindings, "package.manifest_read") ||
		!hasFindingCheck(badManifestFindings, "ED-PACKAGE-001") {
		t.Fatalf("package should fail closed on invalid render manifest path, got %#v", badManifestPkg)
	}
	if err := os.RemoveAll(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := secureWriteFile(manifestPath, []byte(originalManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	var externalPathManifest renderManifest
	if err := json.Unmarshal([]byte(originalManifest), &externalPathManifest); err != nil {
		t.Fatal(err)
	}
	outsidePNG := filepath.Join(t.TempDir(), "outside.png")
	writeSolidPNGForTest(t, outsidePNG, color.RGBA{R: 200, G: 50, B: 25, A: 255})
	externalPathManifest.PNGFiles[0].Path = outsidePNG
	externalPathManifest.PNGFiles[0].SHA256 = mustSHA256(outsidePNG)
	if err := secureWriteJSON(manifestPath, externalPathManifest); err != nil {
		t.Fatal(err)
	}
	externalPathPkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	externalPathFindings, _ := externalPathPkg["findings"].([]qaFinding)
	if externalPathPkg["status"] != "fail" || !hasFindingCheck(externalPathFindings, "manifest.artifact_paths") {
		t.Fatalf("package should reject non-canonical render manifest artifact paths, got %#v", externalPathPkg)
	}
	if err := secureWriteFile(manifestPath, []byte(originalManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	var visualPayload map[string]any
	if err := json.Unmarshal([]byte(readFileOrEmpty(visualReviewPath)), &visualPayload); err != nil {
		t.Fatal(err)
	}
	visualEvidence, _ := visualPayload["imageEvidence"].([]any)
	if len(visualEvidence) == 0 {
		t.Fatal("expected visual review image evidence")
	}
	firstVisualEvidence, _ := visualEvidence[0].(map[string]any)
	firstVisualEvidence["absolutePath"] = ""
	if err := secureWriteJSON(visualReviewPath, visualPayload); err != nil {
		t.Fatal(err)
	}
	badVisualEvidencePkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	badVisualEvidenceFindings, _ := badVisualEvidencePkg["findings"].([]qaFinding)
	if badVisualEvidencePkg["status"] != "fail" || !hasFindingCheck(badVisualEvidenceFindings, "visual review absolutePath") {
		t.Fatalf("package should reject mismatched visual review absolutePath, got %#v", badVisualEvidencePkg)
	}
	writeTestVisualReviewPass(t, deck, manifest)

	reviewerPath := filepath.Join(outDir, "agent_reviews", "round_01", "reviewer_delivery.json")
	var reviewerPayload map[string]any
	if err := json.Unmarshal([]byte(readFileOrEmpty(reviewerPath)), &reviewerPayload); err != nil {
		t.Fatal(err)
	}
	rawEvidence, _ := reviewerPayload["imageEvidence"].([]any)
	if len(rawEvidence) == 0 {
		t.Fatal("expected structured review image evidence")
	}
	firstEvidence, _ := rawEvidence[0].(map[string]any)
	firstEvidence["sha256"] = strings.Repeat("0", 64)
	if err := secureWriteJSON(reviewerPath, reviewerPayload); err != nil {
		t.Fatal(err)
	}
	badEvidencePkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	badEvidenceFindings, _ := badEvidencePkg["findings"].([]qaFinding)
	if badEvidencePkg["status"] != "fail" || !hasFindingCheck(badEvidenceFindings, "package.structured_review_evidence") {
		t.Fatalf("package should reject mismatched structured review image evidence, got %#v", badEvidencePkg)
	}
	for _, stage := range structuredReviewStages() {
		if _, err := writeStructuredReview(deck, stage, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := json.Unmarshal([]byte(readFileOrEmpty(reviewerPath)), &reviewerPayload); err != nil {
		t.Fatal(err)
	}
	rawEvidence, _ = reviewerPayload["imageEvidence"].([]any)
	firstEvidence, _ = rawEvidence[0].(map[string]any)
	firstEvidence["absolutePath"] = ""
	if err := secureWriteJSON(reviewerPath, reviewerPayload); err != nil {
		t.Fatal(err)
	}
	emptyAbsPkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	emptyAbsFindings, _ := emptyAbsPkg["findings"].([]qaFinding)
	if emptyAbsPkg["status"] != "fail" || !hasFindingCheck(emptyAbsFindings, "absolutePath") {
		t.Fatalf("package should reject empty structured review absolutePath, got %#v", emptyAbsPkg)
	}
	for _, stage := range structuredReviewStages() {
		if _, err := writeStructuredReview(deck, stage, 1); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(specPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, stage := range structuredReviewStages() {
		if _, err := writeStructuredReview(deck, stage, 1); err != nil {
			t.Fatal(err)
		}
	}
	invalidSpecPkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	invalidSpecFindings, _ := invalidSpecPkg["findings"].([]qaFinding)
	if invalidSpecPkg["status"] != "fail" || !hasFindingCheck(invalidSpecFindings, "package.deck_spec") {
		t.Fatalf("package should reject invalid current deck_spec.json, got %#v", invalidSpecPkg)
	}
	if err := os.WriteFile(specPath, []byte(originalSpec+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hashDriftPkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	hashDriftFindings, _ := hashDriftPkg["findings"].([]qaFinding)
	if hashDriftPkg["status"] != "fail" || !hasFindingCheck(hashDriftFindings, "deckSpecSha256") {
		t.Fatalf("package should reject structured review deckSpecSha256 drift, got %#v", hashDriftPkg)
	}
	if err := os.WriteFile(specPath, []byte(originalSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, stage := range structuredReviewStages() {
		if _, err := writeStructuredReview(deck, stage, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(qaReportPath, []byte(strings.Replace(originalQAReport, "deterministicStatus: pass", "deterministicStatus: fail", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDeliverySummary(deck); err != nil {
		t.Fatal(err)
	}
	for _, stage := range structuredReviewStages() {
		if _, err := writeStructuredReview(deck, stage, 1); err != nil {
			t.Fatal(err)
		}
	}
	qaStatusPkg, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	qaStatusFindings, _ := qaStatusPkg["findings"].([]qaFinding)
	if qaStatusPkg["status"] != "fail" || !hasFindingCheck(qaStatusFindings, "package.qa_report_status") {
		t.Fatalf("package should reject non-pass QA report status, got %#v", qaStatusPkg)
	}
	if err := os.WriteFile(qaReportPath, []byte(originalQAReport), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDeliverySummary(deck); err != nil {
		t.Fatal(err)
	}
	for _, stage := range structuredReviewStages() {
		if _, err := writeStructuredReview(deck, stage, 1); err != nil {
			t.Fatal(err)
		}
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

func renderSmokeRequired() bool {
	return os.Getenv("SLIDEX_REQUIRE_RENDER_SMOKE") == "1"
}

func TestRunVisualReviewRecordWritesFreshManualEvidence(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	pngPath := filepath.Join(outDir, "rendered_slides", "slide_01.png")
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSolidPNGForTest(t, pngPath, color.RGBA{R: 12, G: 34, B: 56, A: 255})
	htmlPath := filepath.Join(outDir, "final_deck.html")
	if err := os.WriteFile(htmlPath, []byte("<!doctype html><section class=\"slide\">OK</section>"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := renderManifest{
		SourceHTML:         artifact{Path: htmlPath, SHA256: mustSHA256(htmlPath)},
		PDF:                artifact{Path: filepath.Join(outDir, "final_deck.pdf")},
		QAMontage:          artifact{Path: filepath.Join(outDir, "qa_montage.png")},
		ExpectedDimensions: dimension{Width: 2, Height: 2},
		PNGFiles: []renderedImage{{
			SlideID:    "slide_01",
			Path:       pngPath,
			SHA256:     mustSHA256(pngPath),
			Dimensions: dimension{Width: 2, Height: 2},
			Blank:      false,
		}},
	}
	if err := secureWriteJSON(filepath.Join(outDir, "render_manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}

	if err := runVisualReviewRecord([]string{"--deck", deck, "--inspector", "QA", "--notes", "montage and PDF inspected"}); err != nil {
		t.Fatal(err)
	}

	reviewPath := filepath.Join(outDir, "visual_reviews", "latest_review.json")
	if !visualReviewArtifactFresh(reviewPath, manifest) {
		t.Fatalf("manual visual review should be fresh: %s", readFileOrEmpty(reviewPath))
	}
	if findings := verifyVisualReviewImageSet(filepath.Join(outDir, "visual_reviews", "image_set.json"), manifest); len(findings) > 0 {
		t.Fatalf("image set should verify, got %#v", findings)
	}
	checkDir := t.TempDir()
	if err := os.Chdir(checkDir); err != nil {
		t.Fatal(err)
	}
	if findings := verifyVisualReviewEvidence(reviewPath, deck, manifest); len(findings) > 0 {
		t.Fatalf("visual review evidence should verify, got %#v", findings)
	}
	rawReview := readFileOrEmpty(reviewPath)
	if !strings.Contains(rawReview, `"repoRelativePath": "out/rendered_slides/slide_01.png"`) {
		t.Fatalf("manual review evidence should use deck-relative image paths: %s", rawReview)
	}
	if !strings.Contains(readFileOrEmpty(reviewPath), "montage and PDF inspected") {
		t.Fatalf("manual notes were not recorded: %s", readFileOrEmpty(reviewPath))
	}
}

func TestRunVisualReviewManualRejectsForgedHashOnlyReview(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	pngPath := filepath.Join(outDir, "rendered_slides", "slide_01.png")
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSolidPNGForTest(t, pngPath, color.RGBA{R: 12, G: 34, B: 56, A: 255})
	manifest := renderManifest{
		PNGFiles: []renderedImage{{
			SlideID:    "slide_01",
			Path:       pngPath,
			SHA256:     mustSHA256(pngPath),
			Dimensions: dimension{Width: 2, Height: 2},
			Blank:      false,
		}},
	}
	reviewPath := filepath.Join(outDir, "visual_reviews", "latest_review.json")
	if err := secureWriteJSON(reviewPath, map[string]any{
		"status": "pass",
		"notes":  "hash-only forged review " + manifest.PNGFiles[0].SHA256,
	}); err != nil {
		t.Fatal(err)
	}

	status, findings := runVisualReview(deck, manifest, "manual")
	if status != "missing" || !hasFindingCheck(findings, "visual_review.manual_schema") {
		t.Fatalf("manual visual review should reject forged hash-only JSON, status=%s findings=%#v", status, findings)
	}
}

func TestRunVisualReviewRecordRejectsNonCanonicalManifestImages(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	pngPath := filepath.Join(outDir, "rendered_slides", "slide_01.png")
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSolidPNGForTest(t, pngPath, color.RGBA{R: 12, G: 34, B: 56, A: 255})
	outsidePNG := filepath.Join(t.TempDir(), "outside.png")
	writeSolidPNGForTest(t, outsidePNG, color.RGBA{R: 90, G: 10, B: 80, A: 255})
	htmlPath := filepath.Join(outDir, "final_deck.html")
	if err := os.WriteFile(htmlPath, []byte("<!doctype html><section class=\"slide\">OK</section>"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := renderManifest{
		SourceHTML:         artifact{Path: htmlPath, SHA256: mustSHA256(htmlPath)},
		PDF:                artifact{Path: filepath.Join(outDir, "final_deck.pdf")},
		QAMontage:          artifact{Path: filepath.Join(outDir, "qa_montage.png")},
		ExpectedDimensions: dimension{Width: 2, Height: 2},
		PNGFiles: []renderedImage{{
			SlideID:    "slide_01",
			Path:       outsidePNG,
			SHA256:     mustSHA256(outsidePNG),
			Dimensions: dimension{Width: 2, Height: 2},
			Blank:      false,
		}},
	}
	if err := secureWriteJSON(filepath.Join(outDir, "render_manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}

	err = runVisualReviewRecord([]string{"--deck", deck, "--inspector", "QA"})
	if err == nil || !strings.Contains(err.Error(), "render manifest artifact paths are not canonical") {
		t.Fatalf("visual review record should reject non-canonical manifest image paths, got %v", err)
	}
}

func TestRunReviewWritesDeterministicStructuredReview(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "strategy.md"), []byte("# Strategy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "deck_spec.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runReview([]string{"--deck", deck, "--stage", "design"}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(outDir, "agent_reviews", "round_01", "reviewer_design.json")
	raw := readFileOrEmpty(path)
	if !strings.Contains(raw, `"status": "pass"`) || !strings.Contains(raw, `"mode": "parallel_reviewer_threads"`) {
		t.Fatalf("unexpected deterministic review payload: %s", raw)
	}
}

func hasFindingCheck(findings []qaFinding, needle string) bool {
	for _, finding := range findings {
		if strings.Contains(finding.Check, needle) || strings.Contains(finding.Message, needle) {
			return true
		}
	}
	return false
}

func TestEditorialSpecFindingsEnforceSlideAndClaimGates(t *testing.T) {
	spec := map[string]any{
		"editorialProfile": map[string]any{
			"decisionRequirement": "decision",
			"requestedDecision":   "승인",
		},
		"editorialDesignPolicy": map[string]any{
			"appendixRelaxationAllowed": false,
			"copyLimits": map[string]any{
				"headlineChars": 80,
				"takeawayChars": 80,
				"maxBullets":    2,
				"bulletChars":   80,
				"cjkLineChars":  6,
			},
		},
		"slides": []any{
			map[string]any{
				"id":             "slide_01",
				"appendix":       false,
				"headline":       "출처 없는 수치 주장을 점검합니다",
				"readerQuestion": "",
				"takeaway":       "",
				"bodyContent":    []any{"첫 번째 근거", "두 번째 근거", "세 번째 근거", "한국어문장이너무길어서줄바꿈위험"},
			},
		},
		"claimProvenance": map[string]any{
			"claims": []any{
				map[string]any{
					"id":        "claim_metric",
					"text":      "전환율은 35% 개선된다.",
					"status":    "sourced",
					"material":  true,
					"claimType": "metric",
				},
				map[string]any{
					"id":       "claim_best",
					"text":     "시장 최고 성과를 보장한다.",
					"status":   "assumption",
					"material": true,
				},
			},
		},
	}
	findings := editorialSpecFindings(spec, "deck_spec.json")
	for _, check := range []string{"ED-STRUCT-003", "ED-COPY-002", "ED-COPY-003", "ED-TYPE-004", "ED-CLAIM-001", "ED-CLAIM-002", "ED-CLAIM-003"} {
		if !hasFindingCheck(findings, check) {
			t.Fatalf("expected %s finding, got %#v", check, findings)
		}
	}
}

func TestEditorialSpecFindingsRelaxAppendixDensity(t *testing.T) {
	denseBody := []any{
		"한국어문장이너무길어서줄바꿈위험",
		"두 번째 과밀 항목",
		"세 번째 과밀 항목",
	}
	spec := map[string]any{
		"editorialDesignPolicy": map[string]any{
			"appendixRelaxationAllowed": true,
			"copyLimits": map[string]any{
				"headlineChars": 80,
				"takeawayChars": 80,
				"maxBullets":    2,
				"bulletChars":   80,
				"cjkLineChars":  6,
			},
		},
		"slides": []any{
			map[string]any{
				"id":          "appendix_01",
				"sectionRole": "appendix",
				"slideType":   "appendix",
				"appendix":    true,
				"headline":    "부록",
				"takeaway":    "상세 근거",
				"bodyContent": denseBody,
			},
			map[string]any{
				"id":          "slide_01",
				"sectionRole": "content",
				"slideType":   "custom",
				"appendix":    false,
				"headline":    "본문",
				"takeaway":    "핵심 근거",
				"bodyContent": denseBody,
			},
		},
	}
	findings := editorialSpecFindings(spec, "deck_spec.json")
	for _, finding := range findings {
		if strings.Contains(finding.Message, "appendix_01") {
			t.Fatalf("appendix slide should relax density findings, got %#v", findings)
		}
	}
	if !hasFindingCheck(findings, "slide_01 has 3 body bullets") || !hasFindingCheck(findings, "slide_01 bullet 1 has a CJK run") {
		t.Fatalf("non-appendix dense slide should still report copy and CJK findings: %#v", findings)
	}
}

func TestEditorialHTMLFindingsEnforceStructureTypeAndA11y(t *testing.T) {
	html := `<!doctype html><html lang="ko"><head><style>
body { font-family: Arial, sans-serif; text-align: justify; }
.slide { padding: 8px; }
h1 { font-size: 96px; }
.body { display: grid; gap: 15px; }
.message { color: #777777; background: #777777; font-size: 12px; }
</style></head><body><main class="deck">
<section class="slide" id="slide_01" data-slide-id="slide_01">
  <h2>첫 번째 제목</h2>
  <h2>두 번째 제목</h2>
  <p class="message">작은 저대비 본문</p>
  <table><tr><td>35%</td></tr></table>
  <div class="chart color-only" data-chart-type="sankey" data-color-only="true">색상으로만 의미를 구분하는 낯선 차트</div>
  <img src="chart.png" data-primary-text="true">
</section>
</main></body></html>`
	slides := extractSlides(html)
	spec := map[string]any{
		"editorialProfile": map[string]any{"locale": "ko-KR"},
		"slides": []any{
			map[string]any{"id": "slide_01", "htmlId": "slide_01", "appendix": false},
		},
	}
	findings := editorialHTMLFindings("final_deck.html", html, spec, slides)
	for _, check := range []string{"ED-GRID-001", "ED-GRID-002", "ED-A11Y-001", "ED-TYPE-001", "ED-TYPE-002", "ED-TYPE-003", "ED-HIER-001", "ED-HIER-002", "ED-STRUCT-003", "ED-DATAVIZ-002", "ED-DATAVIZ-003", "ED-DATAVIZ-004", "ED-DATAVIZ-005", "ED-A11Y-002", "ED-A11Y-003"} {
		if !hasFindingCheck(findings, check) {
			t.Fatalf("expected %s finding, got %#v", check, findings)
		}
	}
}

func TestVerifyHTMLEditSyncFailsWhenBaselineDiffers(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "final_deck.html")
	baselinePath := filepath.Join(dir, "final_deck.generated_baseline.html")
	if err := os.WriteFile(htmlPath, []byte("<html>edited</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath, []byte("<html>baseline</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := verifyHTMLEditSync(htmlPath, baselinePath)
	if !hasFindingCheck(findings, "ED-RENDER-003") {
		t.Fatalf("expected ED-RENDER-003 finding, got %#v", findings)
	}
}

func TestVerifyHTMLEditSyncFailsClosedOnHashErrors(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "final_deck.html")
	if err := os.WriteFile(htmlPath, []byte("<html>edited</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.html")
	if err := os.WriteFile(outside, []byte("<html>outside</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	baselinePath := filepath.Join(dir, "final_deck.generated_baseline.html")
	if err := os.Symlink(outside, baselinePath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	findings := verifyHTMLEditSync(htmlPath, baselinePath)
	if !hasFindingCheck(findings, "ED-RENDER-003") || !hasFindingCheck(findings, "could not hash") {
		t.Fatalf("expected fail-closed HTML sync hash finding, got %#v", findings)
	}
}

func TestEditorialGridClippingEvidenceUsesRuleID(t *testing.T) {
	err := editorialGridClippingError("slide_01", []string{"div.content overflows"})
	if err == nil || !strings.Contains(err.Error(), "ED-GRID-003") {
		t.Fatalf("clipping error should include ED-GRID-003, got %v", err)
	}
	findings := editorialManifestWarningFindings(renderManifest{Warnings: []string{"overflow check could not run for slide_01: chrome failed"}}, "render_manifest.json")
	if !hasFindingCheck(findings, "ED-GRID-003") {
		t.Fatalf("manifest overflow warning should become ED-GRID-003, got %#v", findings)
	}
}

func TestVerifyPDFPNGVisualParityDetectsMismatch(t *testing.T) {
	dir := t.TempDir()
	pngA := filepath.Join(dir, "slide_01.png")
	pngB := filepath.Join(dir, "slide_02.png")
	pdfPath := filepath.Join(dir, "final_deck.pdf")
	writeSolidPNGForTest(t, pngA, color.RGBA{R: 255, A: 255})
	writeSolidPNGForTest(t, pngB, color.RGBA{B: 255, A: 255})
	if err := writePDFFromPNGs(pdfPath, []string{pngA}, 540, 540); err != nil {
		t.Fatal(err)
	}
	matching := []renderedImage{{SlideID: "slide_01", Path: pngA, SHA256: mustSHA256(pngA), Dimensions: dimension{Width: 2, Height: 2}}}
	if findings := verifyPDFPNGVisualParity(pdfPath, matching); hasFailures(findings) {
		t.Fatalf("matching PDF/PNG parity should pass: %#v", findings)
	}
	mismatched := []renderedImage{{SlideID: "slide_01", Path: pngB, SHA256: mustSHA256(pngB), Dimensions: dimension{Width: 2, Height: 2}}}
	if findings := verifyPDFPNGVisualParity(pdfPath, mismatched); !hasFindingCheck(findings, "ED-RENDER-004") {
		t.Fatalf("mismatched PDF/PNG parity should fail ED-RENDER-004: %#v", findings)
	}
}

func TestValidatePNGRejectsSymlinkArtifact(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real.png")
	linkPath := filepath.Join(dir, "slide_01.png")
	writeSolidPNGForTest(t, realPath, color.RGBA{R: 255, A: 255})
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, _, err := validatePNG(linkPath, 2, 2)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink PNG rejection, got %v", err)
	}
}

func TestValidateSpecFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	linkPath := filepath.Join(dir, "deck_spec.json")
	if err := os.WriteFile(outside, []byte(`{"metadata":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, err := validateSpecFile(linkPath)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink spec rejection, got %v", err)
	}
}

func TestReadRegularFileRejectsOversizedDeckText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "final_deck.html")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxDeckTextArtifactBytes + 1); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = readRegularFile(path)
	if err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected oversized deck text rejection, got %v", err)
	}
}

func TestVerifySanitizedLogsRejectsSymlink(t *testing.T) {
	outDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "run_log.jsonl")
	logPath := filepath.Join(outDir, "run_log.jsonl")
	if err := os.WriteFile(outside, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, logPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	findings := verifySanitizedLogs(outDir)
	if !hasFindingCheck(findings, "package.logs") || !hasFindingCheck(findings, "symlink") {
		t.Fatalf("expected symlink log rejection, got %#v", findings)
	}
}

func TestReadRunLogSegmentsRejectsSymlink(t *testing.T) {
	outDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "run_log.jsonl")
	logPath := filepath.Join(outDir, "run_log.jsonl")
	if err := os.WriteFile(outside, []byte(`{"event":"run_started"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, logPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, err := readRunLogSegments(logPath, time.Now())
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink run log rejection, got %v", err)
	}
}

func TestReplayMCPEventsRejectsSymlinkEventLog(t *testing.T) {
	deck := t.TempDir()
	agentRuns := filepath.Join(deck, "out", "agent_runs")
	if err := os.MkdirAll(agentRuns, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "events.jsonl")
	eventPath := filepath.Join(agentRuns, "resolve_workspace_appserver_events.jsonl")
	if err := os.WriteFile(outside, []byte(`{"method":"thread/item/updated","params":{"threadId":"t","turnId":"u"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, eventPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, err := replayMCPEvents(deck, "")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink event log rejection, got %v", err)
	}
}

func TestPackageDeckSkipsSpecParsingWhenRequiredSpecInvalid(t *testing.T) {
	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "deck_spec.json")
	specPath := filepath.Join(outDir, "deck_spec.json")
	if err := os.WriteFile(outside, []byte(`{"metadata":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, specPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	result, err := packageDeck(deck, false)
	if err != nil {
		t.Fatal(err)
	}
	findings, ok := result["findings"].([]qaFinding)
	if !ok {
		t.Fatalf("unexpected package findings payload: %#v", result["findings"])
	}
	if !hasFindingCheck(findings, "ED-PACKAGE-001") || !hasFindingCheck(findings, "symlink") {
		t.Fatalf("expected invalid required spec finding, got %#v", findings)
	}
	if hasFindingCheck(findings, "package.deck_spec") {
		t.Fatalf("package should not parse an invalid required spec after file validation fails: %#v", findings)
	}
}

func TestMCPStateReadRejectsSymlinkStateFile(t *testing.T) {
	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"outside":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(outDir, "slidex_state.json")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, err := callMCPTool("state/read", map[string]any{"deck": deck})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected MCP state/read symlink rejection, got %v", err)
	}
}

func TestMCPStateReadRejectsSymlinkDeck(t *testing.T) {
	realDeck := t.TempDir()
	outDir := filepath.Join(realDeck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "slidex_state.json"), []byte(`{"schemaVersion":"slidex.state.v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	linkDeck := filepath.Join(t.TempDir(), "deck-link")
	if err := os.Symlink(realDeck, linkDeck); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, err := callMCPTool("state/read", map[string]any{"deck": linkDeck})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected MCP state/read deck symlink rejection, got %v", err)
	}
}

func TestExtractPDFImageStreamsRejectsOversizedDimensions(t *testing.T) {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n1 0 obj\n")
	fmt.Fprintf(&pdf, "<< /Type /XObject /Subtype /Image /Width %d /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\n", maxRenderedPNGPixels+1, compressed.Len())
	pdf.WriteString("stream\n")
	pdf.Write(compressed.Bytes())
	pdf.WriteString("\nendstream\nendobj\n")
	path := filepath.Join(t.TempDir(), "final_deck.pdf")
	if err := os.WriteFile(path, pdf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := extractPDFImageStreams(path)
	if err == nil || !strings.Contains(err.Error(), "maximum pixel count") {
		t.Fatalf("expected PDF image stream dimension budget rejection, got %v", err)
	}
}

func TestExtractPDFImageStreamsRejectsAggregateDecodedBudget(t *testing.T) {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	writePDFImageStreamForTest(t, &pdf, 1, 2, 2, bytes.Repeat([]byte{0}, 12))
	writePDFImageStreamForTest(t, &pdf, 2, 2, 2, bytes.Repeat([]byte{1}, 12))
	path := filepath.Join(t.TempDir(), "final_deck.pdf")
	if err := os.WriteFile(path, pdf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := extractPDFImageStreamsWithBudget(path, 10, 12)
	if err == nil || !strings.Contains(err.Error(), "decoded byte budget") {
		t.Fatalf("expected aggregate PDF image stream budget rejection, got %v", err)
	}
}

func writePDFImageStreamForTest(t *testing.T, pdf *bytes.Buffer, objectID, width, height int, rawRGB []byte) {
	t.Helper()
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(rawRGB); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(pdf, "%d 0 obj\n", objectID)
	fmt.Fprintf(pdf, "<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\n", width, height, compressed.Len())
	pdf.WriteString("stream\n")
	pdf.Write(compressed.Bytes())
	pdf.WriteString("\nendstream\nendobj\n")
}

func writeSolidPNGForTest(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestWritePDFFromPNGsWithBudgetRejectsTooManyImages(t *testing.T) {
	err := writePDFFromPNGsWithBudget(
		filepath.Join(t.TempDir(), "final_deck.pdf"),
		[]string{"slide_01.png", "slide_02.png"},
		540,
		304,
		pdfWriteBudget{MaxImageStreams: 1, MaxFinalBytes: 1 << 20},
	)
	if err == nil || !strings.Contains(err.Error(), "too many PDF image streams") {
		t.Fatalf("expected image stream count rejection, got %v", err)
	}
}

func TestWritePDFFromPNGsWithBudgetRejectsImageStreamCap(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "slide_01.png")
	writeSolidPNGForTest(t, pngPath, color.RGBA{R: 255, A: 255})
	err := writePDFFromPNGsWithBudget(
		filepath.Join(dir, "final_deck.pdf"),
		[]string{pngPath},
		540,
		304,
		pdfWriteBudget{
			MaxImageStreams:     1,
			MaxImageStreamBytes: 1,
			MaxImageStreamTotal: 1 << 20,
			MaxObjectBytes:      1 << 20,
			MaxFinalBytes:       1 << 20,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "PDF image stream") {
		t.Fatalf("expected image stream size rejection, got %v", err)
	}
}

func TestWritePDFFromPNGsWithBudgetRejectsFinalPDFCap(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "slide_01.png")
	writeSolidPNGForTest(t, pngPath, color.RGBA{B: 255, A: 255})
	err := writePDFFromPNGsWithBudget(
		filepath.Join(dir, "final_deck.pdf"),
		[]string{pngPath},
		540,
		304,
		pdfWriteBudget{
			MaxImageStreams:     1,
			MaxImageStreamBytes: 1 << 20,
			MaxImageStreamTotal: 1 << 20,
			MaxObjectBytes:      1 << 20,
			MaxFinalBytes:       64,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "PDF exceeds maximum size") {
		t.Fatalf("expected final PDF size rejection, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "final_deck.pdf")); !os.IsNotExist(statErr) {
		t.Fatalf("over-budget PDF should not be installed, stat err=%v", statErr)
	}
}

func TestArtifactFromPathRelativeWithMaxBytesStrictRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "final_deck.pdf")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := artifactFromPathRelativeWithMaxBytesStrict(path, dir, 16)
	if err == nil || !strings.Contains(err.Error(), "maximum hash size") {
		t.Fatalf("expected strict artifact hash rejection, got artifact=%#v err=%v", artifact, err)
	}
	if artifact.SHA256 != "" {
		t.Fatalf("strict artifact should not invent hash on failure: %#v", artifact)
	}
}

func TestRunIntakeInteractiveAppliesAnswers(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	if err := os.WriteFile(filepath.Join(deck, "brief.md"), []byte("TODO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = writer.WriteString("회사소개서\n임원진의 투자 검토 결정\n검증된 매출 수치는 제외하고 제품 범위만 사용\n")
	_ = writer.Close()
	os.Stdin = reader
	defer func() {
		os.Stdin = oldStdin
		_ = reader.Close()
	}()

	if err := runIntake([]string{"--deck", deck, "--interactive"}); err != nil {
		t.Fatal(err)
	}
	brief := readFileOrEmpty(filepath.Join(deck, "brief.md"))
	if !strings.Contains(brief, "회사소개서") || !strings.Contains(brief, "임원진의 투자 검토 결정") {
		t.Fatalf("interactive answers were not appended to brief.md:\n%s", brief)
	}
	intake := readFileOrEmpty(filepath.Join(deck, "out", "intake_questions.md"))
	if !strings.Contains(intake, "Status: `complete`") {
		t.Fatalf("interactive intake should mark questions complete:\n%s", intake)
	}
}

func TestRunIntakeRejectsEmptyAndPartialAnswers(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	if err := os.WriteFile(filepath.Join(deck, "brief.md"), []byte("TODO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := filepath.Join(t.TempDir(), "answers.md")
	if err := os.WriteFile(answers, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = runIntake([]string{"--deck", deck, "--answers", answers})
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) || coded.ExitCode() != 3 {
		t.Fatalf("empty answers should fail with exit 3, got %v", err)
	}
	if err := os.WriteFile(answers, []byte(`{"metadata":{"a":"b","c":"d"},"answers":["회사소개서"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err = runIntake([]string{"--deck", deck, "--answers", answers})
	if !errors.As(err, &coded) || coded.ExitCode() != 3 {
		t.Fatalf("metadata plus partial JSON answers should fail with exit 3, got %v", err)
	}
	if err := os.WriteFile(answers, []byte(`{"answers":{"metadata":{"a":"b","c":"d"},"notes":"not an answer"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err = runIntake([]string{"--deck", deck, "--answers", answers})
	if !errors.As(err, &coded) || coded.ExitCode() != 3 {
		t.Fatalf("nested metadata-only JSON answers should fail with exit 3, got %v", err)
	}

	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = writer.WriteString("회사소개서\n")
	_ = writer.Close()
	os.Stdin = reader
	defer func() {
		os.Stdin = oldStdin
		_ = reader.Close()
	}()
	err = runIntake([]string{"--deck", deck, "--interactive"})
	if !errors.As(err, &coded) || coded.ExitCode() != 3 {
		t.Fatalf("partial interactive answers should fail with exit 3, got %v", err)
	}
	if strings.Contains(readFileOrEmpty(filepath.Join(deck, "brief.md")), "회사소개서") {
		t.Fatal("partial interactive answers must not close intake or append to brief.md")
	}
}

func TestRunIntakeRejectsOversizedAnswersFile(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	if err := os.WriteFile(filepath.Join(deck, "brief.md"), []byte("TODO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := filepath.Join(t.TempDir(), "answers.md")
	f, err := os.Create(answers)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(answers, maxDeckMarkdownBytes+1); err != nil {
		t.Fatal(err)
	}

	err = runIntake([]string{"--deck", deck, "--answers", answers})
	if err == nil || !strings.Contains(err.Error(), "file exceeds maximum allowed size") {
		t.Fatalf("oversized answers should fail before validation, got %v", err)
	}
}

func TestRunIntakeRejectsSymlinkAnswersFile(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	deck := t.TempDir()
	if err := os.WriteFile(filepath.Join(deck, "brief.md"), []byte("TODO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.md")
	if err := os.WriteFile(target, []byte("valid-looking answers\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := filepath.Join(t.TempDir(), "answers.md")
	if err := os.Symlink(target, answers); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	err = runIntake([]string{"--deck", deck, "--answers", answers})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("symlink answers should fail before validation, got %v", err)
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

func TestValidateSpecRejectsNonPDFPrimaryArtifact(t *testing.T) {
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
	if _, err := ensureSpec(deck, true); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(deck, "out", "deck_spec.json")
	var spec map[string]any
	if err := json.Unmarshal([]byte(readFileOrEmpty(specPath)), &spec); err != nil {
		t.Fatal(err)
	}
	contract, ok := spec["outputContract"].(map[string]any)
	if !ok {
		t.Fatal("expected outputContract object")
	}
	contract["primaryPdf"] = "out/final_deck.html"
	if err := writeJSONFile(specPath, spec); err != nil {
		t.Fatal(err)
	}
	findings, err := validateSpecFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFindingCheck(findings, "schema.outputContract.primaryPdf") {
		t.Fatalf("expected primaryPdf contract finding, got %#v", findings)
	}
}

func TestMigrationFindingsUseCompatibilityLanguage(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "final_deck.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := strings.Join(migrationFindings(deck, "html-pdf"), "\n")
	for _, forbidden := range []string{"leg" + "acy", "depre" + "cated"} {
		if strings.Contains(strings.ToLower(findings), forbidden) {
			t.Fatalf("migration findings should use compatibility language, got %q", findings)
		}
	}
}

func TestFinalizeCreatesRuntimeArtifactsForStageByStagePackage(t *testing.T) {
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
	if err := runFinalize([]string{"--deck", deck}); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(deck, "out", "slidex_state.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("finalize should create slidex_state.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(deck, "out", "codex_threads.json")); err != nil {
		t.Fatalf("finalize should create codex_threads.json: %v", err)
	}
	if findings := verifyRiskPolicy(statePath); hasFailures(findings) {
		t.Fatalf("finalize-created state should satisfy risk policy: %#v", findings)
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
		"schemaVersion":  "slidex.reviewFindings.v1",
		"stage":          "delivery",
		"round":          1,
		"mode":           "structured_turn",
		"status":         "pass",
		"imageEvidence":  []map[string]any{},
		"artifactHashes": structuredReviewArtifactHashes(filepath.Join(root, "fixtures", "minimal_deck")),
		"findings":       []map[string]any{},
	}
	if err := validatePayloadAgainstSchema(reviewPayload, filepath.Join("schemas", "app_review_findings.strict.schema.json")); err != nil {
		t.Fatal(err)
	}
	authoringPayload := map[string]any{
		"schemaVersion":    "slidex.appAuthoringResult.v1",
		"stage":            "strategy",
		"status":           "pass",
		"summary":          "authored",
		"strategyMarkdown": "## Purpose\n\n검증 가능한 목적.",
		"slideBlueprints":  []map[string]any{},
		"htmlNotes":        []string{},
		"layoutContract":   defaultLayoutContract(),
		"claimPolicy":      "unsupported claims are assumptions",
		"risks":            []map[string]any{},
	}
	if err := validatePayloadAgainstSchema(authoringPayload, filepath.Join("schemas", "app_authoring_result.strict.schema.json")); err != nil {
		t.Fatal(err)
	}
}

func TestAuthoringMaterialityRejectsEmptyPassPayload(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	empty := map[string]any{
		"schemaVersion":    "slidex.appAuthoringResult.v1",
		"stage":            "spec",
		"status":           "pass",
		"summary":          "empty",
		"strategyMarkdown": "",
		"slideBlueprints":  []map[string]any{},
		"htmlNotes":        []string{},
		"layoutContract":   defaultLayoutContract(),
		"claimPolicy":      "unsupported claims are assumptions",
		"risks":            []map[string]any{},
	}
	if err := validateAuthoringMateriality("spec", empty); err == nil {
		t.Fatal("schema-valid empty spec authoring should not be material")
	}
	if err := validatePayloadAgainstSchema(empty, filepath.Join("schemas", "app_authoring_result.strict.schema.json")); err == nil {
		t.Fatal("strict authoring schema should reject empty spec slideBlueprints")
	}
}

func TestEnsureStrategyAndSpecConsumeCodexAuthoring(t *testing.T) {
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
	strategyPayload := map[string]any{
		"schemaVersion":    "slidex.appAuthoringResult.v1",
		"stage":            "strategy",
		"status":           "pass",
		"summary":          "Codex strategy summary",
		"strategyMarkdown": "## Codex Authored Purpose\n\n현재 입력만 근거로 의사결정 목적을 정리합니다.",
		"slideBlueprints":  []map[string]any{},
		"htmlNotes":        []string{},
		"layoutContract":   defaultLayoutContract(),
		"claimPolicy":      "unsupported claims are assumptions",
		"risks":            []map[string]any{},
	}
	specPayload := map[string]any{
		"schemaVersion":    "slidex.appAuthoringResult.v1",
		"stage":            "spec",
		"status":           "pass",
		"summary":          "Codex spec summary",
		"strategyMarkdown": "",
		"slideBlueprints": []map[string]any{{
			"sectionRole":  "codex_cover",
			"headline":     "Codex가 검증된 입력으로 첫 판단을 정리합니다",
			"keyMessage":   "입력 기반 가정과 근거를 분리합니다.",
			"bodyContent":  []string{"근거", "가정", "다음 행동"},
			"evidenceRefs": []string{"brief.md"},
			"claims":       []string{"claim_001"},
		}},
		"htmlNotes":      []string{},
		"layoutContract": defaultLayoutContract(),
		"claimPolicy":    "unsupported claims are assumptions",
		"risks":          []map[string]any{},
	}
	htmlPayload := map[string]any{
		"schemaVersion":    "slidex.appAuthoringResult.v1",
		"stage":            "build_html",
		"status":           "pass",
		"summary":          "Codex HTML summary",
		"strategyMarkdown": "",
		"slideBlueprints":  []map[string]any{},
		"htmlNotes":        []string{"Use a decision panel."},
		"layoutContract":   map[string]string{"layoutMode": "decision_panel", "panelLabel": "Codex Panel", "panelText": "Codex layout contract text.", "primaryColor": "#123456", "accentColor": "#abcdef"},
		"claimPolicy":      "unsupported claims are assumptions",
		"risks":            []map[string]any{},
	}
	if err := writeAuthoringTurnForTest(deck, "strategy", strategyPayload); err != nil {
		t.Fatal(err)
	}
	if err := writeAuthoringTurnForTest(deck, "spec", specPayload); err != nil {
		t.Fatal(err)
	}
	if err := writeAuthoringTurnForTest(deck, "build_html", htmlPayload); err != nil {
		t.Fatal(err)
	}
	strategyPath, err := ensureStrategy(deck, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFileOrEmpty(strategyPath), "Codex Authored Purpose") {
		t.Fatalf("strategy did not consume Codex authoring: %s", readFileOrEmpty(strategyPath))
	}
	specPath, err := ensureSpec(deck, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFileOrEmpty(specPath), "Codex가 검증된 입력") {
		t.Fatalf("spec did not consume Codex slide blueprint: %s", readFileOrEmpty(specPath))
	}
	htmlPath, err := ensureHTML(deck, true)
	if err != nil {
		t.Fatal(err)
	}
	html := readFileOrEmpty(htmlPath)
	for _, want := range []string{"Codex Panel", "Codex layout contract text.", "#123456", "#abcdef", "decision_panel"} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML did not consume Codex layout contract %q: %s", want, html)
		}
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

func TestDangerousAppServerMethodsRequireStageAllowlist(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	blocked := &appServerClient{stage: "build"}
	if _, _, err := blocked.request("process/spawn", map[string]any{}, time.Second); err == nil {
		t.Fatal("dangerous process/spawn should be blocked without slidex.toml allowlist")
	}
	cfg := `[codex.app_server.dangerous_api_allowlist]
qa = ["process/spawn"]
build = ["mcpServer/tool/call"]
`
	if err := os.WriteFile(filepath.Join(dir, "slidex.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if allowed, err := dangerousAppServerMethodAllowed("process/spawn", "build"); err != nil || allowed {
		t.Fatalf("process/spawn should not inherit another stage allowlist, allowed=%v err=%v", allowed, err)
	}
	allowedClient := &appServerClient{stdin: &testWriteCloser{}, lines: make(chan map[string]any, 1), stage: "build"}
	go func() {
		allowedClient.lines <- map[string]any{"id": 1, "result": map[string]any{"ok": true}}
	}()
	if _, _, err := allowedClient.request("mcpServer/tool/call", map[string]any{}, time.Second); err != nil {
		t.Fatalf("stage-allowlisted dangerous method should be sent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "slidex.toml"), []byte("[codex.app_server]\nallow_dangerous_methods = [\"process/spawn\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dangerousAppServerMethodAllowed("process/spawn", "build"); err == nil {
		t.Fatal("global dangerous App Server allowlist should be rejected")
	}
}

func TestLoadDangerousAppServerAllowlistRejectsOversizedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slidex.toml")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", int(maxProjectConfigBytes)+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadDangerousAppServerAllowlist(path)
	if err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected oversized config rejection, got %v", err)
	}
}

func TestAppServerPluginSmokeHelpers(t *testing.T) {
	params := appServerWorkbenchToolCallParams("thread-1", "workbench.start", "/tmp/workspace", "demo")
	if params["server"] != "slidex" || params["tool"] != "workbench.start" || params["threadId"] != "thread-1" {
		t.Fatalf("unexpected tool params: %#v", params)
	}
	args, _ := params["arguments"].(map[string]any)
	if args["workspace"] != "/tmp/workspace" || args["deckId"] != "demo" {
		t.Fatalf("unexpected tool arguments: %#v", args)
	}

	resp := map[string]any{"result": map[string]any{"structuredContent": map[string]any{"status": "running"}}}
	if got := structuredContentFromMCPToolCall(resp)["status"]; got != "running" {
		t.Fatalf("structured content status = %#v", got)
	}
	toolResult := mcpToolCallResult(map[string]any{
		"browserOpen": map[string]any{
			"url":             "http://127.0.0.1:49152/workbench/session",
			"preferredAction": "browser_plugin_navigation",
		},
	})
	content, _ := toolResult["content"].([]map[string]any)
	if len(content) != 1 || !strings.Contains(metadataString(content[0]["text"]), "Open in Codex App Browser now: http://127.0.0.1:49152/workbench/session") {
		t.Fatalf("MCP tool result should surface browser-open instruction before JSON: %#v", toolResult)
	}
	summary := summarizeJSONForEvidence(map[string]any{"token": "secret-value", "safe": "ok"})
	raw, _ := json.Marshal(summary)
	if strings.Contains(string(raw), "secret-value") {
		t.Fatalf("summary leaked secret: %s", raw)
	}
	plugin := map[string]any{
		"plugin": map[string]any{
			"summary": map[string]any{
				"name":         "slidex",
				"localVersion": toolVersion + "+codex.test",
				"installed":    true,
				"enabled":      true,
				"source":       map[string]any{"path": "/opt/slidex/plugins/slidex"},
			},
		},
	}
	version, path := pluginReadVersionAndPath(plugin)
	if version != toolVersion+"+codex.test" || path != "/opt/slidex/plugins/slidex" {
		t.Fatalf("plugin read details = %q / %q", version, path)
	}
	installed, enabled, found := pluginReadInstallState(plugin, "slidex")
	if !found || !installed || !enabled {
		t.Fatalf("plugin install state = found:%v installed:%v enabled:%v", found, installed, enabled)
	}
}

func TestPostRestartPluginVerificationClearsRestartState(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := markPluginRestartRequired(installRoot, toolVersion, "v"+toolVersion); err != nil {
		t.Fatal(err)
	}
	writePostRestartPluginFilesForTest(t, installRoot, toolVersion+"+codex.test", toolVersion)
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	result := appServerPluginSmokeResult{
		Status:                  "pass",
		PluginReadOK:            true,
		PluginInstallStateFound: true,
		PluginInstalled:         true,
		PluginEnabled:           true,
		PluginVersion:           toolVersion + "+codex.test",
		PluginPath:              filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex")),
		StartSkillFound:         true,
		StartSkillPath:          filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")),
		Checks:                  map[string]any{},
	}
	applyPostRestartPluginVerification(&result)
	if result.PluginVerificationStatus != "verified" {
		t.Fatalf("verification status = %q, checks = %#v", result.PluginVerificationStatus, result.Checks)
	}
	if !result.RestartRequiredBefore || result.RestartRequiredAfter {
		t.Fatalf("restart state before/after = %v/%v", result.RestartRequiredBefore, result.RestartRequiredAfter)
	}
	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.RestartRequired || status.PluginVerificationStatus != "verified" {
		t.Fatalf("persisted status = %#v", status)
	}
	if status.VerifiedPluginVersion != toolVersion+"+codex.test" {
		t.Fatalf("verified plugin version = %q", status.VerifiedPluginVersion)
	}
	if status.VerifiedPluginPath != filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex")) {
		t.Fatalf("verified plugin path = %q", status.VerifiedPluginPath)
	}
	if status.VerifiedStartSkillPath != filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")) {
		t.Fatalf("verified start skill path = %q", status.VerifiedStartSkillPath)
	}
}

func TestPostRestartPluginVerificationAcceptsCodexCacheSkillPath(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	canaryVersion := toolVersion + "-canary.20260610010000"
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, canaryVersion))
	writePostRestartPluginFilesForTest(t, installRoot, toolVersion+"+codex.test", toolVersion)
	cacheInstallRoot := t.TempDir()
	writePostRestartPluginFilesForTest(t, cacheInstallRoot, toolVersion+"+codex.test", toolVersion)
	cacheSkillPath := filepath.Join(cacheInstallRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	result := appServerPluginSmokeResult{
		Status:                  "pass",
		PluginReadOK:            true,
		PluginInstallStateFound: true,
		PluginInstalled:         true,
		PluginEnabled:           true,
		PluginVersion:           toolVersion + "+codex.test",
		PluginPath:              filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex")),
		StartSkillFound:         true,
		StartSkillPath:          filepath.ToSlash(cacheSkillPath),
		Checks:                  map[string]any{},
	}
	applyPostRestartPluginVerification(&result)
	if result.PluginVerificationStatus != "verified" {
		t.Fatalf("cache skill verification status = %q, checks = %#v", result.PluginVerificationStatus, result.Checks)
	}
	if result.RestartRequiredAfter {
		t.Fatalf("cache skill verification should clear restart state: %#v", result)
	}
	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.VerifiedStartSkillPath != filepath.ToSlash(cacheSkillPath) || status.PluginVerificationStatus != "verified" {
		t.Fatalf("verified cache skill status = %#v", status)
	}
	if status.CurrentVersion != canaryVersion || status.TargetVersion != canaryVersion || status.TargetTag != "v"+canaryVersion {
		t.Fatalf("verified canary package identity not preserved: %#v", status)
	}
}

func TestPostRestartPluginVerificationKeepsRestartStateForDrift(t *testing.T) {
	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := markPluginRestartRequired(installRoot, toolVersion, "v"+toolVersion); err != nil {
		t.Fatal(err)
	}
	writePostRestartPluginFilesForTest(t, installRoot, toolVersion+"+codex.test", toolVersion)
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	result := appServerPluginSmokeResult{
		Status:                  "pass",
		PluginReadOK:            true,
		PluginInstallStateFound: true,
		PluginInstalled:         true,
		PluginEnabled:           true,
		PluginVersion:           toolVersion + "+codex.test",
		PluginPath:              filepath.ToSlash(filepath.Join(t.TempDir(), "plugins", "slidex")),
		StartSkillFound:         true,
		StartSkillPath:          filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")),
		Checks:                  map[string]any{},
	}
	applyPostRestartPluginVerification(&result)
	if result.PluginVerificationStatus != "drift" {
		t.Fatalf("verification status = %q", result.PluginVerificationStatus)
	}
	if !result.RestartRequiredAfter {
		t.Fatalf("restart-required should remain after drift: %#v", result)
	}
	status, err := currentUpdateStatus(installRoot, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.PluginVerificationStatus != "drift" || !status.RestartRequired {
		t.Fatalf("drift should persist in update status: %#v", status)
	}
	if !hasStatusBannerForTest(updateStatusBanners(status), "codex_plugin_drift") {
		t.Fatalf("drift banner missing: %#v", updateStatusBanners(status))
	}
}

func TestPostRestartPluginVerificationRequiresInstalledEnabledPlugin(t *testing.T) {
	installRoot := t.TempDir()
	writePostRestartPluginFilesForTest(t, installRoot, toolVersion+"+codex.test", toolVersion)
	result := appServerPluginSmokeResult{
		Status:                  "pass",
		PluginReadOK:            true,
		PluginInstallStateFound: true,
		PluginInstalled:         true,
		PluginEnabled:           false,
		PluginVersion:           toolVersion + "+codex.test",
		PluginPath:              filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex")),
		StartSkillFound:         true,
		StartSkillPath:          filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")),
	}
	if got := postRestartPluginVerificationStatus(result, installRoot); got != "not_verified" {
		t.Fatalf("disabled plugin verification status = %q", got)
	}
	result.PluginEnabled = true
	result.PluginInstalled = false
	if got := postRestartPluginVerificationStatus(result, installRoot); got != "not_verified" {
		t.Fatalf("uninstalled plugin verification status = %q", got)
	}
}

func TestPostRestartPluginVerificationDetectsManifestDrift(t *testing.T) {
	installRoot := t.TempDir()
	writePostRestartPluginFilesForTest(t, installRoot, toolVersion+"+codex.other", toolVersion)
	result := appServerPluginSmokeResult{
		Status:                  "pass",
		PluginReadOK:            true,
		PluginInstallStateFound: true,
		PluginInstalled:         true,
		PluginEnabled:           true,
		PluginVersion:           toolVersion + "+codex.test",
		PluginPath:              filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex")),
		StartSkillFound:         true,
		StartSkillPath:          filepath.ToSlash(filepath.Join(installRoot, "plugins", "slidex", "skills", "slidex-start", "SKILL.md")),
	}
	if got := postRestartPluginVerificationStatus(result, installRoot); got != "drift" {
		t.Fatalf("manifest drift verification status = %q", got)
	}
	writePostRestartPluginFilesForTest(t, installRoot, toolVersion+"+codex.test", "0.0.0")
	if got := postRestartPluginVerificationStatus(result, installRoot); got != "drift" {
		t.Fatalf("version lock drift verification status = %q", got)
	}
}

func TestAppServerPluginSmokeUsesDocumentedPluginSurfaces(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	if err := markPluginRestartRequired(installRoot, toolVersion, "v"+toolVersion); err != nil {
		t.Fatal(err)
	}
	writePostRestartPluginFilesForTest(t, installRoot, toolVersion+"+codex.test", toolVersion)
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	fakeBin := buildFakeCodexAppServerForTest(t)
	logPath := filepath.Join(t.TempDir(), "fake-codex-transcript.jsonl")
	pluginPath := filepath.Join(installRoot, "plugins", "slidex")
	skillPath := filepath.Join(pluginPath, "skills", "slidex-start", "SKILL.md")
	t.Setenv("PATH", filepath.Dir(fakeBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CODEX_LOG", logPath)
	t.Setenv("FAKE_CODEX_PLUGIN_VERSION", toolVersion+"+codex.test")
	t.Setenv("FAKE_CODEX_PLUGIN_PATH", pluginPath)
	t.Setenv("FAKE_CODEX_SKILL_PATH", skillPath)

	result, err := appServerWorkbenchPluginSmoke(t.TempDir(), "plugin-smoke")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" || result.PluginVerificationStatus != "verified" || result.RestartRequiredAfter {
		t.Fatalf("plugin smoke result should pass post-restart verification: %#v", result)
	}
	if !result.PluginInstalled || !result.PluginEnabled || !result.StartSkillFound {
		t.Fatalf("plugin install/skill evidence missing: %#v", result)
	}

	entries := readFakeCodexTranscriptForTest(t, logPath)
	methods := transcriptMethodsForTest(entries)
	wantMethods := []string{
		"initialize",
		"initialized",
		"plugin/read",
		"skills/list",
		"thread/start",
		"mcpServerStatus/list",
		"mcpServer/tool/call",
		"mcpServer/tool/call",
		"mcpServer/tool/call",
	}
	if !reflect.DeepEqual(methods, wantMethods) {
		t.Fatalf("App Server method transcript = %#v, want %#v", methods, wantMethods)
	}
	pluginRead := transcriptEntryForMethodForTest(t, entries, "plugin/read")
	pluginParams, _ := pluginRead["params"].(map[string]any)
	if pluginParams["pluginName"] != "slidex" || !strings.HasSuffix(filepath.ToSlash(metadataString(pluginParams["marketplacePath"])), filepath.ToSlash(filepath.Join(".agents", "plugins", "marketplace.json"))) {
		t.Fatalf("plugin/read params should use bundled marketplace: %#v", pluginParams)
	}
	skillsList := transcriptEntryForMethodForTest(t, entries, "skills/list")
	skillsParams, _ := skillsList["params"].(map[string]any)
	if skillsParams["forceReload"] != true {
		t.Fatalf("skills/list must force reload bundled skills: %#v", skillsParams)
	}
	var tools []string
	for _, entry := range entries {
		if entry["method"] != "mcpServer/tool/call" {
			continue
		}
		params, _ := entry["params"].(map[string]any)
		tools = append(tools, metadataString(params["tool"]))
	}
	if !reflect.DeepEqual(tools, []string{"workbench.start", "workbench.status", "workbench.stop"}) {
		t.Fatalf("unexpected workbench tool calls: %#v", tools)
	}
	for _, method := range methods {
		if method == "plugin/install" || method == "plugin/uninstall" {
			t.Fatalf("plugin smoke should not mutate plugin installs through %s", method)
		}
	}
}

func writePostRestartPluginFilesForTest(t *testing.T, installRoot, manifestVersion, lockVersion string) {
	t.Helper()
	pluginRoot := filepath.Join(installRoot, "plugins", "slidex")
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".codex-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginRoot, "skills", "slidex-start"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "skills", "slidex-start", "SKILL.md"), []byte("# slidex-start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"name":    toolName,
		"version": manifestVersion,
	}
	if err := writeSourceJSONFile(filepath.Join(pluginRoot, ".codex-plugin", "plugin.json"), manifest); err != nil {
		t.Fatal(err)
	}
	lock := map[string]any{
		"pluginVersion":           lockVersion,
		"slidexCliVersion":        lockVersion,
		"requiredCodexCliVersion": requiredCodexVersion,
	}
	if err := writeSourceJSONFile(filepath.Join(pluginRoot, ".codex-plugin", "version-lock.json"), lock); err != nil {
		t.Fatal(err)
	}
}

func buildFakeCodexAppServerForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "fake_codex.go")
	code := `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("codex 0.138.0")
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	buffer := make([]byte, 0, 1024*1024)
	scanner.Buffer(buffer, 16*1024*1024)
	for scanner.Scan() {
		var req map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		appendTranscript(req)
		id, ok := req["id"]
		if !ok {
			continue
		}
		method, _ := req["method"].(string)
		params, _ := req["params"].(map[string]any)
		result := resultFor(method, params)
		raw, _ := json.Marshal(map[string]any{"id": id, "result": result})
		fmt.Println(string(raw))
	}
}

func appendTranscript(req map[string]any) {
	path := os.Getenv("FAKE_CODEX_LOG")
	if path == "" {
		return
	}
	raw, _ := json.Marshal(req)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, string(raw))
}

func resultFor(method string, params map[string]any) map[string]any {
	pluginPath := firstNonEmpty(os.Getenv("FAKE_CODEX_PLUGIN_PATH"), filepath.Join(mustCWD(), "plugins", "slidex"))
	skillPath := firstNonEmpty(os.Getenv("FAKE_CODEX_SKILL_PATH"), filepath.Join(pluginPath, "skills", "slidex-start", "SKILL.md"))
	pluginVersion := firstNonEmpty(os.Getenv("FAKE_CODEX_PLUGIN_VERSION"), "0.1.0+codex.test")
	switch method {
	case "initialize":
		return map[string]any{"serverInfo": map[string]any{"name": "fake-codex"}}
	case "plugin/read":
		return map[string]any{"plugin": map[string]any{"summary": map[string]any{
			"name": "slidex", "localVersion": pluginVersion, "installed": true, "enabled": true,
			"source": map[string]any{"path": pluginPath},
		}}}
	case "skills/list":
		return map[string]any{"skills": []any{map[string]any{"name": "slidex:slidex-start", "metadata": map[string]any{"path": skillPath}}}}
	case "thread/start":
		return map[string]any{"thread": map[string]any{"id": "thread-1"}}
	case "mcpServerStatus/list":
		return map[string]any{"servers": []any{map[string]any{"name": "slidex", "tools": []string{"workbench.start", "workbench.status", "workbench.stop"}}}}
	case "mcpServer/tool/call":
		tool, _ := params["tool"].(string)
		switch tool {
		case "workbench.start":
			return map[string]any{"structuredContent": map[string]any{
				"status": "running",
				"workbench": map[string]any{"status": "running", "url": "http://127.0.0.1:49152/workbench/session", "serverBind": "127.0.0.1", "browserOpenStrategy": "manual"},
				"proprietaryCanvasAPI": "not_used",
			}}
		case "workbench.status":
			return map[string]any{"structuredContent": map[string]any{"workbench": map[string]any{"status": "running"}}}
		case "workbench.stop":
			return map[string]any{"structuredContent": map[string]any{"status": "stopped"}}
		}
	}
	return map[string]any{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func mustCWD() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
`
	if err := os.WriteFile(source, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name = "codex.exe"
	}
	path := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", path, source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fake codex build failed: %v\n%s", err, out)
	}
	return path
}

func readFakeCodexTranscriptForTest(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid fake codex transcript line: %v\n%s", err, line)
		}
		entries = append(entries, entry)
	}
	return entries
}

func transcriptMethodsForTest(entries []map[string]any) []string {
	methods := make([]string, 0, len(entries))
	for _, entry := range entries {
		methods = append(methods, metadataString(entry["method"]))
	}
	return methods
}

func transcriptEntryForMethodForTest(t *testing.T, entries []map[string]any, method string) map[string]any {
	t.Helper()
	for _, entry := range entries {
		if entry["method"] == method {
			return entry
		}
	}
	t.Fatalf("transcript missing method %s: %#v", method, entries)
	return nil
}

func TestAppServerSkillSmokeHelpers(t *testing.T) {
	skills := map[string]any{
		"skills": []any{
			map[string]any{"name": "other:skill", "path": "/tmp/other/SKILL.md"},
			map[string]any{
				"name": "slidex:slidex-start",
				"metadata": map[string]any{
					"path": "/home/me/.codex/plugins/cache/slidex/skills/slidex-start/SKILL.md",
				},
			},
		},
	}
	path, ok := findSkillPathInSkillsList(skills, "slidex:slidex-start")
	if !ok {
		t.Fatal("expected slidex skill path to be found")
	}
	if !strings.HasSuffix(path, "skills/slidex-start/SKILL.md") {
		t.Fatalf("unexpected skill path: %q", path)
	}
	if _, ok := findSkillPathInSkillsList(skills, "slidex:missing"); ok {
		t.Fatal("missing skill should not be found")
	}

	command := appServerSkillSmokeWorkbenchCommandForOS("linux", "/tmp/slidex workspace", "demo")
	if !strings.Contains(command, "'/tmp/slidex workspace'") || !strings.Contains(command, "--deck-id demo") {
		t.Fatalf("command was not shell quoted as expected: %s", command)
	}
	dangerousWindowsPath := `C:\Users\Me\slidex %workspace%!^&() O'Hare`
	windowsCommand := appServerSkillSmokeWorkbenchCommandForOS("windows", dangerousWindowsPath, "demo")
	if !strings.HasPrefix(windowsCommand, "powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand ") {
		t.Fatalf("windows command should force encoded PowerShell execution: %s", windowsCommand)
	}
	if strings.Contains(windowsCommand, dangerousWindowsPath) {
		t.Fatalf("windows command should not expose shell-metacharacter path outside encoded payload: %s", windowsCommand)
	}
	windowsScript := decodeWindowsPowerShellCommandForTest(t, windowsCommand)
	for _, want := range []string{"& 'slidex'", "'workbench'", "'start'", "'--workspace'", "'C:\\Users\\Me\\slidex %workspace%!^&() O''Hare'", "'--deck-id'", "'demo'"} {
		if !strings.Contains(windowsScript, want) {
			t.Fatalf("decoded windows command missing %q:\n%s", want, windowsScript)
		}
	}
	windowsCommandWithDir := windowsPowerShellCommandInDir(`C:\Safe Root`, `C:\Tools\slidex.exe`, "version")
	windowsScriptWithDir := decodeWindowsPowerShellCommandForTest(t, windowsCommandWithDir)
	wantWindowsScriptWithDir := "Set-Location -LiteralPath 'C:\\Safe Root'; & 'C:\\Tools\\slidex.exe' 'version'"
	if !strings.Contains(windowsScriptWithDir, wantWindowsScriptWithDir) {
		t.Fatalf("decoded windows command should set working directory before invocation:\n%s", windowsScriptWithDir)
	}
	windowsInlineCommandWithDir := windowsPowerShellInlineCommandInDir(`C:\Safe Root`, `C:\Tools\slidex.exe`, "version")
	wantWindowsInlineCommandWithDir := "$ErrorActionPreference='Stop'; Set-Location -LiteralPath 'C:\\Safe Root'; & 'C:\\Tools\\slidex.exe' 'version'"
	if windowsInlineCommandWithDir != wantWindowsInlineCommandWithDir {
		t.Fatalf("inline windows command = %s, want %s", windowsInlineCommandWithDir, wantWindowsInlineCommandWithDir)
	}
	if strings.Contains(windowsInlineCommandWithDir, "powershell.exe") || strings.Contains(windowsInlineCommandWithDir, "-EncodedCommand") {
		t.Fatalf("inline windows command should not spawn a child PowerShell: %s", windowsInlineCommandWithDir)
	}
	pendingInlineCommand := windowsPendingActivationPowerShellCommand(`C:\Activator Root`, `C:\Install Root`, `C:\Activator Root\slidex.exe`, "update", "activate-pending")
	for _, want := range []string{
		"& { $slidexPreviousErrorActionPreference = $ErrorActionPreference; $ErrorActionPreference='Stop'",
		"$slidexActivationExitCode = 0",
		"try { Set-Location -LiteralPath 'C:\\Activator Root'; & 'C:\\Activator Root\\slidex.exe' 'update' 'activate-pending'",
		"} finally { Set-Location -LiteralPath 'C:\\Install Root'; $ErrorActionPreference = $slidexPreviousErrorActionPreference }",
		"throw ('slidex pending activation failed with exit code ' + $slidexActivationExitCode)",
	} {
		if !strings.Contains(pendingInlineCommand, want) {
			t.Fatalf("pending inline windows command missing %q:\n%s", want, pendingInlineCommand)
		}
	}
	if strings.Contains(pendingInlineCommand, "powershell.exe") || strings.Contains(pendingInlineCommand, "exit $LASTEXITCODE") {
		t.Fatalf("pending inline windows command should not spawn or exit caller shell: %s", pendingInlineCommand)
	}
	prompt := appServerSkillSmokePrompt("/tmp/slidex workspace", "demo", command)
	if !strings.Contains(prompt, "Do not run render, QA, package") {
		t.Fatalf("prompt must keep skill smoke scoped: %s", prompt)
	}
	if !isLoopbackWorkbenchURL("http://127.0.0.1:49152/workbench/session") {
		t.Fatal("expected loopback workbench URL to pass")
	}
	if isLoopbackWorkbenchURL("http://localhost:49152/workbench/session") {
		t.Fatal("localhost should not satisfy the 127.0.0.1 bind contract")
	}

	result := appServerSkillSmokeResult{
		PluginReadOK:                    true,
		SkillFound:                      true,
		TurnSandboxPolicy:               "dangerFullAccess",
		TurnStatus:                      "completed",
		DeckCreated:                     true,
		ManifestExists:                  true,
		StartStatus:                     "running",
		DraftStatus:                     "draft_saved",
		SaveStatus:                      "saved",
		StopStatus:                      "stopped",
		ServerBind:                      "127.0.0.1",
		TokenRedacted:                   true,
		RawTokenAbsentFromArtifacts:     true,
		SavedInputVerified:              true,
		ProprietaryCanvasAPI:            "not_used",
		IsActualCodexAppBrowserEvidence: false,
		WorkbenchURL:                    "http://127.0.0.1:49152/workbench/session",
		VerifiedFiles: map[string]artifact{
			"brief":    {Path: "brief.md", SHA256: strings.Repeat("a", 64), Size: 1},
			"draft":    {Path: "workbench_draft.json", SHA256: strings.Repeat("b", 64), Size: 1},
			"manifest": {Path: "workbench_manifest.json", SHA256: strings.Repeat("c", 64), Size: 1},
		},
	}
	if status := appServerSkillSmokeStatus(result); status != "pass" {
		t.Fatalf("status = %q, want pass", status)
	}
	result.IsActualCodexAppBrowserEvidence = true
	if status := appServerSkillSmokeStatus(result); status != "fail" {
		t.Fatalf("actual browser evidence flag must not pass skill smoke: %q", status)
	}
	events := summarizeAppServerEventsForEvidence([]map[string]any{
		{"method": "turn/started", "params": map[string]any{"token": "secret-value"}},
		{"method": "turn/completed"},
	})
	if events["count"] != 2 {
		t.Fatalf("unexpected event count summary: %#v", events)
	}
	if raw, _ := json.Marshal(events); strings.Contains(string(raw), "secret-value") {
		t.Fatalf("event summary leaked secret: %s", raw)
	}
}

func TestCodexPluginCacheIsNotMutatedDirectly(t *testing.T) {
	root := repoRootForTest(t)
	var offenders []string
	err := filepath.Walk(filepath.Join(root, "cmd", "slidex"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		for _, forbidden := range []string{".codex/plugins/cache", `.codex\plugins\cache`, "plugins/cache"} {
			if strings.Contains(text, forbidden) {
				offenders = append(offenders, filepath.ToSlash(path)+": "+forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("slidex must verify Codex plugins through documented App Server/CLI surfaces, not direct cache paths: %v", offenders)
	}
}

func decodeWindowsPowerShellCommandForTest(t *testing.T, command string) string {
	t.Helper()
	_, encoded, ok := strings.Cut(command, "-EncodedCommand ")
	if !ok {
		t.Fatalf("encoded command missing: %s", command)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw)%2 != 0 {
		t.Fatalf("encoded PowerShell command has odd byte length: %d", len(raw))
	}
	words := make([]uint16, len(raw)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(words))
}

func TestAppServerSkillSmokeSchemaRequiresSavedInputProof(t *testing.T) {
	root := repoRootForTest(t)
	deckAbs := filepath.Join(root, "decks", "doctor-workbench-contract")
	manifest := newWorkbenchManifest(deckAbs, root, "doctor-session", "doctor-token", 49152, os.Getpid(), "running")
	payload := doctorAppServerSkillSmokeEvidence(deckAbs, manifest)
	schemaPath := filepath.Join(root, "schemas", "app_server_skill_smoke.schema.json")
	if err := validatePayloadAgainstSchema(payload, schemaPath); err != nil {
		t.Fatal(err)
	}

	payload.SavedInputVerified = false
	if err := validatePayloadAgainstSchema(payload, schemaPath); err == nil {
		t.Fatal("pass skill smoke evidence without savedInputVerified should fail schema validation")
	}

	payload = doctorAppServerSkillSmokeEvidence(deckAbs, manifest)
	delete(payload.VerifiedFiles, "draft")
	if err := validatePayloadAgainstSchema(payload, schemaPath); err == nil {
		t.Fatal("pass skill smoke evidence without verifiedFiles.draft should fail schema validation")
	}

	payload = doctorAppServerSkillSmokeEvidence(deckAbs, manifest)
	payload.Checks["workbenchSave"] = map[string]any{"status": "fail"}
	if err := validatePayloadAgainstSchema(payload, schemaPath); err == nil {
		t.Fatal("pass skill smoke evidence without passing workbenchSave summary should fail schema validation")
	}
}

func TestWorkbenchDoctorSnapshotRecordsBrowserCapabilityDecision(t *testing.T) {
	snapshot := workbenchDoctorSnapshot()
	if snapshot["browserOpenMechanism"] != "agent_explicit_browser_plugin_instruction_by_default" {
		t.Fatalf("unexpected browser open mechanism: %#v", snapshot)
	}
	if snapshot["browserOpenPreferredAction"] != "packaged MCP sets SLIDEX_BROWSER_OPEN=agent so the agent explicitly uses @Browser without emitting the legacy browserOpen intent" {
		t.Fatalf("unexpected browser preferred action: %#v", snapshot)
	}
	if snapshot["browserOpenFallback"] != "pass browserOpenMode=manual to return only the URL, or browserOpen=true to emit the legacy browserOpen intent" {
		t.Fatalf("unexpected browser fallback: %#v", snapshot)
	}
	if snapshot["browserOpenSuppressionEnv"] != workbenchBrowserOpenEnv {
		t.Fatalf("unexpected browser suppression env: %#v", snapshot)
	}
	if snapshot["directBrowserOpenRequestAPI"] != "not_found_in_codex_app_server_0.138.0" {
		t.Fatalf("doctor snapshot should not claim a direct browser-open request API: %#v", snapshot)
	}
	if snapshot["clientRequestSchemaScanned"] != true {
		t.Fatalf("doctor snapshot should scan ClientRequest schema: %#v", snapshot)
	}
	if count, _ := snapshot["clientRequestMethodCount"].(int); count == 0 {
		t.Fatalf("doctor snapshot should report ClientRequest methods: %#v", snapshot)
	}
	if methods, _ := snapshot["directBrowserOpenRequestMethods"].([]string); len(methods) != 0 {
		t.Fatalf("doctor snapshot should not find direct browser-open request methods: %#v", methods)
	}
	if snapshot["schemaOpenPageActionScope"] != "web_search_action_only" {
		t.Fatalf("doctor snapshot should scope openPage to web search actions: %#v", snapshot)
	}
	if snapshot["proprietaryCanvasMountAPI"] != "not_claimed" {
		t.Fatalf("doctor snapshot must not claim proprietary canvas mount support: %#v", snapshot)
	}
	if snapshot["skillSmokeCommand"] != "slidex codex app-server skill-smoke --workspace <tmp-workspace> --deck-id <deck_id>" {
		t.Fatalf("doctor snapshot should expose the App Server skill smoke command: %#v", snapshot)
	}
	if snapshot["skillSmokeEvidenceSchema"] != "schemas/app_server_skill_smoke.schema.json" {
		t.Fatalf("doctor snapshot should expose the App Server skill smoke schema: %#v", snapshot)
	}
	if snapshot["skillSmokeSavesInput"] != true {
		t.Fatalf("skill smoke must be reported as saving workbench input: %#v", snapshot)
	}
	if snapshot["skillSmokeIsBrowserEvidence"] != false {
		t.Fatalf("skill smoke must not be reported as browser evidence: %#v", snapshot)
	}
	if snapshot["saveSmokeCommand"] != "slidex workbench save-smoke --workspace <tmp-workspace> --deck-id <deck_id> --screenshot" {
		t.Fatalf("doctor snapshot should expose the pre-GUI save smoke command: %#v", snapshot)
	}
	if snapshot["saveSmokeScreenshotEvidence"] != "out/workbench_save_smoke.png" {
		t.Fatalf("doctor snapshot should expose save-smoke screenshot evidence: %#v", snapshot)
	}
	if snapshot["saveSmokeIsBrowserEvidence"] != false {
		t.Fatalf("save smoke must not be reported as browser evidence: %#v", snapshot)
	}
	if snapshot["browserEvidenceRequired"] != true {
		t.Fatalf("doctor snapshot must require actual browser evidence: %#v", snapshot)
	}
	if snapshot["postRestartVerificationCommand"] != "slidex codex app-server plugin-smoke --json" {
		t.Fatalf("doctor snapshot should expose post-restart verification command: %#v", snapshot)
	}
}

func TestDoctorReportIncludesUpdateSnapshot(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	installRoot := t.TempDir()
	metadataPath := installMetadataPath(installRoot)
	writeInstallMetadataForTest(t, metadataPath, releaseInstallMetadataForTest(t, toolVersion))
	t.Setenv(updateInstallRootEnv, installRoot)
	t.Setenv(updateInstallMetadataEnv, metadataPath)

	report := doctorReport("", false, false)
	update, ok := report["update"].(map[string]any)
	if !ok {
		t.Fatalf("doctor report missing update snapshot: %#v", report)
	}
	if update["channel"] != updateChannelProduction || update["updatesEnabled"] != true {
		t.Fatalf("unexpected update snapshot: %#v", update)
	}
}

func TestClientRequestMethodsFromSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ClientRequest.json")
	raw := []byte(`{
	  "oneOf": [
	    {"properties": {"method": {"enum": ["thread/start"]}}},
	    {"properties": {"method": {"enum": ["turn/start"]}}},
	    {"properties": {"method": {"enum": ["browser/openUrl"]}}}
	  ]
	}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	methods, err := clientRequestMethodsFromSchema(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"browser/openUrl", "thread/start", "turn/start"}) {
		t.Fatalf("methods = %#v", methods)
	}
	direct := browserOpenLikeClientRequestMethods(methods)
	if !reflect.DeepEqual(direct, []string{"browser/openUrl"}) {
		t.Fatalf("direct browser-like methods = %#v", direct)
	}
}

func TestRepoConfigAllowsOnlyPluginSmokeMCPToolCall(t *testing.T) {
	root := repoRootForTest(t)
	path := filepath.Join(root, "slidex.toml")
	allowed, err := dangerousAppServerMethodAllowedAtPath(path, "mcpServer/tool/call", "plugin_smoke")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("plugin_smoke should allow mcpServer/tool/call")
	}
	allowed, err = dangerousAppServerMethodAllowedAtPath(path, "process/spawn", "plugin_smoke")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("plugin_smoke should not allow process/spawn")
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
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	state := newState(filepath.Join(t.TempDir(), "deck"), "app-server", false)
	state.CodexRuntime.InstalledVersion = "0.0.0"
	if err := enforceCodexRuntimeGate(state); err == nil {
		t.Fatal("expected below-minimum version to fail")
	}
	state.CodexRuntime.InstalledVersion = requiredCodexVersion
	if err := enforceCodexRuntimeGate(state); err != nil {
		t.Fatalf("exact minimum version should pass: %v", err)
	}
	state.CodexRuntime.InstalledVersion = "0.137.0"
	if err := enforceCodexRuntimeGate(state); err == nil {
		t.Fatal("below 0.138.0 should fail minimum gate")
	}
	state.CodexRuntime.InstalledVersion = "0.139.0"
	if err := enforceCodexRuntimeGate(state); err != nil {
		t.Fatalf("higher Codex CLI version should pass minimum gate: %v", err)
	}
	state.CodexRuntime.InstalledVersion = "0.0.0"
	state.CodexRuntime.AllowMismatch = true
	if err := enforceCodexRuntimeGate(state); err != nil {
		t.Fatalf("allow mismatch should bypass version gate: %v", err)
	}
}

func TestCodexVersionAtLeast(t *testing.T) {
	tests := []struct {
		installed string
		minimum   string
		want      bool
	}{
		{"codex-cli 0.138.0", "0.138.0", true},
		{"0.139.0", "0.138.0", true},
		{"0.137.0", "0.138.0", false},
		{"missing", "0.138.0", false},
		{"0.138", "0.138.0", true},
	}
	for _, tc := range tests {
		if got := codexVersionAtLeast(tc.installed, tc.minimum); got != tc.want {
			t.Fatalf("codexVersionAtLeast(%q, %q) = %v, want %v", tc.installed, tc.minimum, got, tc.want)
		}
	}
}

func TestRuntimeReferenceDocsAvoidRangeVersionLanguage(t *testing.T) {
	root := repoRootForTest(t)
	files := []string{
		filepath.Join(root, "commands.md"),
		filepath.Join(root, "plugins", "slidex", "README.md"),
		filepath.Join(root, ".agents", "skills", "slidex", "references", "commands.md"),
	}
	disallowed := []string{
		"versions at or above",
		"version at or above",
		"or newer",
		"or later",
		">=",
		"<=",
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(raw))
		for _, phrase := range disallowed {
			if strings.Contains(lower, phrase) {
				t.Fatalf("%s contains range/floating version language %q", filepath.ToSlash(path), phrase)
			}
		}
	}
}

func TestDistributionPipelineFilesExposeReleaseInstallPath(t *testing.T) {
	root := repoRootForTest(t)
	checks := []struct {
		path string
		want []string
	}{
		{
			path: filepath.Join(root, ".github", "workflows", "release.yml"),
			want: []string{"workflow_dispatch", "build_channel", "canary", "develop", "production", "main", "release_version=\"${base_version}-canary.${release_timestamp}\"", "release_timestamp", "RELEASE_TIMESTAMP", "scripts/write-release-notes-body.sh", "jdx/mise-action@dba19683ed58901619b14f395a24841710cb4925", "mise exec -- pnpm --dir workbench install --frozen-lockfile", "mise exec -- pnpm --dir workbench check", "mise exec -- pnpm --dir workbench build", "Release Binaries", "SLIDEX_BUILD_CHANNEL", "SLIDEX_RELEASE_TAG", "scripts/package-release.sh", "Smoke release package before publish", "sha256sum -c", "COMMIT_SHA", "\"commit\": commit", "buildTime", "datetime.fromisoformat", "tarfile", "zipfile", "node_modules", "update status --json", "refusing to overwrite immutable release assets", "gh release create", "gh release view", "diff -u", "contents: write"},
		},
		{
			path: filepath.Join(root, ".mise.toml"),
			want: []string{"go = \"1.26.3\"", "node = \"24.16.0\"", "\"npm:pnpm\" = \"11.5.3\"", "[tasks.\"version:bump\"]", "scripts/bump-version.sh", "[tasks.\"release-notes:canary\"]", "scripts/create-canary-release-note.sh"},
		},
		{
			path: filepath.Join(root, "workbench", "package.json"),
			want: []string{"\"packageManager\": \"pnpm@11.5.3\"", "\"node\": \"24.16.0\"", "\"pnpm\": \"11.5.3\"", "\"@solidjs/start\": \"1.3.2\"", "\"solid-js\": \"1.9.13\"", "\"vinxi\": \"0.5.11\"", "\"typescript\": \"6.0.3\"", "\"build\": \"vinxi build && node scripts/copy-assets.mjs\""},
		},
		{
			path: filepath.Join(root, "workbench", "pnpm-lock.yaml"),
			want: []string{"lockfileVersion: '9.0'", "importers:", "specifier: 1.3.2", "specifier: 6.0.3"},
		},
		{
			path: filepath.Join(root, "workbench", "pnpm-workspace.yaml"),
			want: []string{"allowBuilds:", "'@parcel/watcher': true", "esbuild: true"},
		},
		{
			path: filepath.Join(root, "workbench", "app.config.ts"),
			want: []string{"@solidjs/start/config", "ssr: false"},
		},
		{
			path: filepath.Join(root, "cmd", "slidex", "workbench_assets", "slidex-workbench-build.json"),
			want: []string{"\"schemaVersion\": \"slidex.workbench.assets.v1\"", "\"sourcePackage\": \"@shiinamachi/slidex-workbench\"", "\"framework\": \"solidstart\"", "\"csr\": true", "\"modulePreloads\"", "\"inlineScripts\"", "window.manifest", "\"sourceSha256\""},
		},
		{
			path: filepath.Join(root, "scripts", "package-release.sh"),
			want: []string{"SLIDEX_TARGETS", "SLIDEX_BUILD_CHANNEL", "decks/_template", "schemas", "plugins/slidex", ".agents/plugins/marketplace.json", ".slidex/install.json", "LICENSE", "VERSIONING.md", "\"VERSION\"", "go run ./cmd/slidex version", "canary_pattern", "checksums.txt"},
		},
		{
			path: filepath.Join(root, "scripts", "bump-version.sh"),
			want: []string{"patch|minor|major|<version>", "release-notes/_template.md", "release-notes/${next}.md", "go run ./cmd/slidex sync-version-metadata"},
		},
		{
			path: filepath.Join(root, "release-notes", "_template.md"),
			want: []string{"# slidex {{VERSION}}", "## Highlights", "## Verification Notes"},
		},
		{
			path: filepath.Join(root, "release-notes", "canary", "_template.md"),
			want: []string{"# slidex {{RELEASE_VERSION}}", "{{BASE_VERSION}}", "{{TIMESTAMP}}", "{{COMMIT_SHA}}", "## Verification Notes"},
		},
		{
			path: filepath.Join(root, "release-notes", toolVersion+".md"),
			want: []string{"# slidex " + toolVersion, "## Highlights", "## Verification Notes"},
		},
		{
			path: filepath.Join(root, "scripts", "create-canary-release-note.sh"),
			want: []string{"release-notes/canary/<base-version>/<timestamp>.md", "release-notes/canary/_template.md", "YYYYMMDDHHMMSS", "BASE_VERSION", "RELEASE_VERSION", "COMMIT_SHA"},
		},
		{
			path: filepath.Join(root, "scripts", "write-release-notes-body.sh"),
			want: []string{"BUILD_CHANNEL", "RELEASE_TIMESTAMP", "${notes_dir}/canary/${base_version}/${release_timestamp}.md", "Release notes source", "release notes still contain template placeholders", "missing %s release notes"},
		},
		{
			path: filepath.Join(root, "INSTALL.md"),
			want: []string{"Internal Install Instructions for Codex", "Step 1", "Step 8", "release tag", "public GitHub Releases API", "SHA-256", "Do not install GitHub CLI", "restart Codex", "pluginVerificationStatus: \"verified\"", "verifiedPluginPath", "Code signing is deferred", "canary install", "immutable channel", "--candidate", "checksum before extraction or activation"},
		},
		{
			path: filepath.Join(root, "CODEX_INSTALL_PROMPT.md"),
			want: []string{"How to use", "사용법", "Production Prompt", "Canary Prompt", "What this prompt does", "https://github.com/shiinamachi/slidex", "Read INSTALL.md", "production channel", "canary channel", "does not require GitHub CLI"},
		},
	}
	for _, check := range checks {
		raw, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, want := range check.want {
			if !strings.Contains(text, want) {
				t.Fatalf("%s does not contain %q", filepath.ToSlash(check.path), want)
			}
		}
	}
	for _, path := range []string{"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("root JS workspace file should not exist: %s (err=%v)", path, err)
		}
	}
	packageScript := readFileOrEmpty(filepath.Join(root, "scripts", "package-release.sh"))
	for _, forbidden := range []string{`"package.json"`, `"pnpm-lock.yaml"`, `"pnpm-workspace.yaml"`, `"workbench"`} {
		if strings.Contains(packageScript, forbidden) {
			t.Fatalf("release package runtime paths must not include Workbench development file %s", forbidden)
		}
	}
	info, err := os.Stat(filepath.Join(root, "scripts", "package-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix executable mode bits")
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("scripts/package-release.sh must be executable")
	}
	workflow := readFileOrEmpty(filepath.Join(root, ".github", "workflows", "release.yml"))
	if strings.Contains(workflow, "release_notes_path=\"release-notes/${BASE_VERSION}.md\"") {
		t.Fatal("release workflow must not hard-code the production release note path for every build channel")
	}
	if strings.Contains(workflow, "--clobber") {
		t.Fatal("release workflow must not clobber existing release assets")
	}
	for _, forbidden := range []string{"actions/attest", "attest" + "ations: write", "gh attest" + "ation verify"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release workflow must use SHA-256 checks only: found %q", forbidden)
		}
	}
}

func TestJavascriptWorkspacePinsAndWorkbenchAssetsSatisfyDoctor(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	nodeVersion := readMiseToolVersion(".mise.toml", "node")
	pnpmVersion := readMiseToolVersion(".mise.toml", "npm:pnpm")
	if nodeVersion != "24.16.0" || pnpmVersion != "11.5.3" {
		t.Fatalf("unexpected JS runtime pins: node=%q pnpm=%q", nodeVersion, pnpmVersion)
	}
	if findings := doctorRuntimePinFindings(nodeVersion, pnpmVersion); hasFailures(findings) {
		t.Fatalf("JS runtime/package pins should satisfy doctor: %#v", findings)
	}
	if findings := doctorWorkbenchAssetFindings(); hasFailures(findings) {
		t.Fatalf("Workbench assets should satisfy doctor: %#v", findings)
	}
	manifest, err := readWorkbenchAssetManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Framework != "solidstart" || !manifest.CSR || manifest.SourcePackage != "@shiinamachi/slidex-workbench" || len(manifest.Scripts) == 0 {
		t.Fatalf("unexpected Workbench asset manifest: %#v", manifest)
	}
}

func TestCanaryReleaseNoteScriptCreatesTimestampedNote(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash script contract is covered on Unix release runners")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash unavailable: %v", err)
	}
	root := repoRootForTest(t)
	notesDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(notesDir, "canary"), 0o755); err != nil {
		t.Fatal(err)
	}
	template := readFileOrEmpty(filepath.Join(root, "release-notes", "canary", "_template.md"))
	if err := os.WriteFile(filepath.Join(notesDir, "canary", "_template.md"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(root, "scripts", "create-canary-release-note.sh")
	timestamp := "20260611032635"
	releaseVersion := "9.8.7-canary." + timestamp
	cmd := exec.Command("bash", script, "--base-version", "9.8.7", "--timestamp", timestamp, "--commit-sha", "abcdef0123456789", "--release-version", releaseVersion, "--notes-dir", notesDir)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create canary release note failed: %v\n%s", err, out)
	}
	note := filepath.Join(notesDir, "canary", "9.8.7", timestamp+".md")
	raw := readFileOrEmpty(note)
	for _, want := range []string{"# slidex " + releaseVersion, "Base version: 9.8.7", "Canary timestamp: " + timestamp, "Source commit: abcdef0123456789"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("generated canary note missing %q:\n%s", want, raw)
		}
	}
	for _, forbidden := range []string{"{{BASE_VERSION}}", "{{TIMESTAMP}}", "{{RELEASE_VERSION}}", "{{COMMIT_SHA}}"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("generated canary note still contains %q:\n%s", forbidden, raw)
		}
	}
	if _, err := os.Stat(filepath.Join(notesDir, "9.8.7.md")); !os.IsNotExist(err) {
		t.Fatalf("canary helper should not create production note path, stat err=%v", err)
	}

	cmd = exec.Command("bash", script, "--base-version", "9.8.7", "--timestamp", timestamp, "--commit-sha", "abcdef0123456789", "--notes-dir", notesDir)
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "canary release note already exists") {
		t.Fatalf("canary helper should refuse overwrite, err=%v out=%s", err, out)
	}

	headRaw, err := exec.Command("git", "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		t.Fatalf("resolve HEAD failed: %v", err)
	}
	head := strings.TrimSpace(string(headRaw))
	timestampCmd := exec.Command("git", "show", "-s", "--format=%cd", "--date=format-local:%Y%m%d%H%M%S", head)
	timestampCmd.Dir = root
	timestampCmd.Env = append(os.Environ(), "TZ=UTC")
	derivedRaw, err := timestampCmd.Output()
	if err != nil {
		t.Fatalf("derive HEAD timestamp failed: %v", err)
	}
	derivedTimestamp := strings.TrimSpace(string(derivedRaw))
	cmd = exec.Command("bash", script, "--base-version", "9.8.8", "--commit-sha", head, "--notes-dir", notesDir)
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create derived canary release note failed: %v\n%s", err, out)
	}
	derivedNote := filepath.Join(notesDir, "canary", "9.8.8", derivedTimestamp+".md")
	derived := readFileOrEmpty(derivedNote)
	if !strings.Contains(derived, "9.8.8-canary."+derivedTimestamp) || !strings.Contains(derived, head) {
		t.Fatalf("derived canary note used wrong timestamp or commit:\n%s", derived)
	}
}

func TestReleaseNotesBodyScriptUsesChannelSpecificSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash script contract is covered on Unix release runners")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash unavailable: %v", err)
	}
	root := repoRootForTest(t)
	notesDir := t.TempDir()
	timestamp := "20260611032635"
	productionNote := filepath.Join(notesDir, "1.2.3.md")
	canaryNote := filepath.Join(notesDir, "canary", "1.2.3", timestamp+".md")
	if err := os.MkdirAll(filepath.Dir(canaryNote), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(productionNote, []byte("# slidex 1.2.3\n\n## Highlights\n\n- Production ready.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canaryNote, []byte("# slidex 1.2.3-canary.20260611032635\n\n## Highlights\n\n- Canary ready.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(root, "scripts", "write-release-notes-body.sh")
	runBody := func(t *testing.T, channel, releaseVersion, releaseTimestamp string) (string, string, error) {
		t.Helper()
		outFile := filepath.Join(t.TempDir(), "notes.md")
		cmd := exec.Command("bash", script, outFile)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"SLIDEX_RELEASE_NOTES_DIR="+notesDir,
			"BASE_VERSION=1.2.3",
			"BUILD_CHANNEL="+channel,
			"COMMIT_SHA=abcdef0123456789",
			"RELEASE_TIMESTAMP="+releaseTimestamp,
			"RELEASE_VERSION="+releaseVersion,
			"SOURCE_REF=testing",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", string(out), err
		}
		return readFileOrEmpty(outFile), string(out), nil
	}

	body, out, err := runBody(t, "production", "1.2.3", "")
	if err != nil {
		t.Fatalf("production release body failed: %v\n%s", err, out)
	}
	if !strings.Contains(body, "Production ready.") || !strings.Contains(body, "Release notes source: "+productionNote) {
		t.Fatalf("production body used wrong note:\n%s", body)
	}
	if strings.Contains(body, "Canary ready.") {
		t.Fatalf("production body should not include canary note:\n%s", body)
	}

	body, out, err = runBody(t, "canary", "1.2.3-canary."+timestamp, timestamp)
	if err != nil {
		t.Fatalf("canary release body failed: %v\n%s", err, out)
	}
	if !strings.Contains(body, "Canary ready.") || !strings.Contains(body, "Release notes source: "+canaryNote) {
		t.Fatalf("canary body used wrong note:\n%s", body)
	}
	if strings.Contains(body, "Production ready.") {
		t.Fatalf("canary body must not reuse production note:\n%s", body)
	}

	if err := os.Remove(canaryNote); err != nil {
		t.Fatal(err)
	}
	_, out, err = runBody(t, "canary", "1.2.3-canary."+timestamp, timestamp)
	if err == nil || !strings.Contains(out, "missing canary release notes") {
		t.Fatalf("missing canary note should fail clearly, err=%v out=%s", err, out)
	}
	if err := os.WriteFile(canaryNote, []byte("# slidex 1.2.3-canary.20260611032635\n\n- TODO: Fill this.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, out, err = runBody(t, "canary", "1.2.3-canary."+timestamp, timestamp)
	if err == nil || !strings.Contains(out, "canary release notes still contain template placeholders") {
		t.Fatalf("placeholder canary note should fail clearly, err=%v out=%s", err, out)
	}
	_, out, err = runBody(t, "canary", "1.2.3-canary.20260611000000", timestamp)
	if err == nil || !strings.Contains(out, "canary RELEASE_VERSION must be 1.2.3-canary."+timestamp) {
		t.Fatalf("timestamp/version mismatch should fail clearly, err=%v out=%s", err, out)
	}
}

func TestUserFacingInstallDocsExposeCanonicalOneShotPrompt(t *testing.T) {
	root := repoRootForTest(t)
	prompt := canonicalInstallPromptForTest(t, root)
	for _, phrase := range []string{
		"https://github.com/shiinamachi/slidex",
		"Read INSTALL.md",
		"production channel install instructions",
	} {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("canonical install prompt is missing %q", phrase)
		}
	}
	if len(prompt) > 180 {
		t.Fatalf("canonical install prompt is too long: %d chars", len(prompt))
	}
	for _, phrase := range []string{
		"detect the local OS and architecture",
		"GitHub CLI",
		"SHA-256 checksum",
		"register the Codex plugin",
		"plugin-smoke",
		"slidex --help",
		"slidex doctor --render",
	} {
		if strings.Contains(prompt, phrase) {
			t.Fatalf("canonical install prompt exposes detailed install guidance %q", phrase)
		}
	}
	checks := []struct {
		path  string
		start string
		end   string
	}{
		{filepath.Join(root, "README.md"), "## ⚡ Install with Codex App", "## What is slidex?"},
		{filepath.Join(root, "README.ko.md"), "## ⚡ Codex App으로 설치", "## slidex란?"},
		{filepath.Join(root, "commands.md"), "## 설치와 배포", "개발자 source build:"},
	}
	reject := []string{
		"codex plugin marketplace add",
		"mise exec -- go install",
		"Source build fallback",
		"요구 사항",
		"Requirements",
	}
	for _, check := range checks {
		raw, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatal(err)
		}
		section := markdownSectionBetween(string(raw), check.start, check.end)
		if !strings.Contains(section, prompt) {
			t.Fatalf("%s install section must expose the one-shot prompt", filepath.ToSlash(check.path))
		}
		for _, phrase := range reject {
			if strings.Contains(section, phrase) {
				t.Fatalf("%s install section exposes detailed install guidance %q", filepath.ToSlash(check.path), phrase)
			}
		}
	}
}

func canonicalInstallPromptForTest(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "CODEX_INSTALL_PROMPT.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "```text")
	if start < 0 {
		t.Fatal("CODEX_INSTALL_PROMPT.md must contain a text fenced prompt")
	}
	start += len("```text")
	end := strings.Index(text[start:], "```")
	if end < 0 {
		t.Fatal("CODEX_INSTALL_PROMPT.md prompt fence is not closed")
	}
	return strings.TrimSpace(text[start : start+end])
}

func markdownSectionBetween(text, start, end string) string {
	startIdx := strings.Index(text, start)
	if startIdx < 0 {
		return ""
	}
	rest := text[startIdx:]
	endIdx := strings.Index(rest, end)
	if endIdx < 0 {
		return rest
	}
	return rest[:endIdx]
}

func TestDoctorGoalMethodsAndRequiredMCPGates(t *testing.T) {
	protocol := map[string]any{
		"ok": true,
		"optionalMethods": map[string]bool{
			"thread/goal/set":   true,
			"thread/goal/get":   false,
			"thread/goal/clear": true,
		},
	}
	if findings := doctorGoalMethodFindings(protocol); !hasFailures(findings) {
		t.Fatalf("missing goal method should fail doctor gate: %#v", findings)
	}

	cfgDir := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	cfg := `[mcp_servers.docs]
command = "docs"
required = true

[mcp_servers.optional]
command = "optional"
required = false
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	required := requiredMCPServersFromConfig(cfgPath)
	if !reflect.DeepEqual(required, []string{"docs"}) {
		t.Fatalf("required MCP servers = %#v", required)
	}
	if !mcpServerListedHealthy("docs running", "docs") {
		t.Fatal("healthy required MCP line should be accepted")
	}
	if mcpServerListedHealthy("docs failed", "docs") {
		t.Fatal("failed required MCP line should not be accepted")
	}
}

func TestDoctorPluginPackageFindingsValidateLocalManifests(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	if findings := doctorPluginPackageFindings("slidex"); hasFailures(findings) {
		t.Fatalf("local plugin package should satisfy doctor gate: %#v", findings)
	}
}

func TestPluginMCPConfigSuppressesBrowserOpenByDefault(t *testing.T) {
	root := repoRootForTest(t)
	raw, err := os.ReadFile(filepath.Join(root, "plugins", "slidex", ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	servers, _ := config["mcpServers"].(map[string]any)
	server, _ := servers["slidex"].(map[string]any)
	env, _ := server["env"].(map[string]any)
	if env[workbenchBrowserOpenEnv] != "agent" {
		t.Fatalf("plugin MCP config should request agent Browser use by default, got %#v", env)
	}
}

func TestVersionMetadataContractStaysAligned(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	manifestPath := filepath.Join("plugins", "slidex", ".codex-plugin", "plugin.json")
	lockPath := filepath.Join("plugins", "slidex", ".codex-plugin", "version-lock.json")
	marketplacePath := filepath.Join(".agents", "plugins", "marketplace.json")
	if findings := validateVersionSourceFile("VERSION"); hasFailures(findings) {
		t.Fatalf("version source drifted: %#v", findings)
	}
	if findings := validatePluginJSONManifest(manifestPath); hasFailures(findings) {
		t.Fatalf("plugin manifest metadata drifted: %#v", findings)
	}
	if findings := validatePluginVersionLock(lockPath); hasFailures(findings) {
		t.Fatalf("plugin version lock drifted: %#v", findings)
	}
	if findings := validateMarketplaceManifest(marketplacePath); hasFailures(findings) {
		t.Fatalf("marketplace metadata drifted: %#v", findings)
	}

	var manifest map[string]any
	readJSONForTest(t, manifestPath, &manifest)
	if got := pluginVersionBase(metadataString(manifest["version"])); got != toolVersion {
		t.Fatalf("plugin manifest version base = %q, want %q", got, toolVersion)
	}
	author, _ := manifest["author"].(map[string]any)
	if got := metadataString(author["name"]); got != toolDeveloperName {
		t.Fatalf("plugin author = %q, want %q", got, toolDeveloperName)
	}
	if got := metadataString(manifest["license"]); got != toolLicenseIdentifier {
		t.Fatalf("plugin license = %q, want %q", got, toolLicenseIdentifier)
	}
	if got := strings.TrimSpace(readFileOrEmpty(filepath.Join(root, "VERSION"))); got != toolVersion {
		t.Fatalf("VERSION = %q, want %q", got, toolVersion)
	}

	licenseText := readFileOrEmpty(filepath.Join(root, "LICENSE"))
	if !strings.Contains(licenseText, toolLicenseName) || !strings.Contains(licenseText, toolDeveloperName) {
		t.Fatal("LICENSE must declare MIT License for shiinamachi")
	}
	versioningText := readFileOrEmpty(filepath.Join(root, "VERSIONING.md"))
	for _, want := range []string{"`VERSION`", "sync-version-metadata", "version-lock.json", "marketplace.json", "scripts/package-release.sh"} {
		if !strings.Contains(versioningText, want) {
			t.Fatalf("VERSIONING.md is missing %q", want)
		}
	}
}

func TestSyncVersionMetadataUsesVersionSource(t *testing.T) {
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "plugin.json")
	lockPath := filepath.Join(dir, "version-lock.json")
	if err := os.WriteFile(manifestPath, []byte(`{
  "name": "slidex",
  "version": "9.9.9+codex.keep",
  "author": {"name": "old"},
  "license": "UNLICENSED",
  "interface": {"developerName": "old"},
  "skills": "./skills/",
  "mcpServers": "./.mcp.json"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(`{
  "schemaVersion": "slidex.pluginVersionLock.v1",
  "pluginName": "slidex",
  "pluginVersion": "9.9.9",
  "requiredCodexCliVersion": "0.0.0",
  "slidexCliVersion": "9.9.9",
  "goVersion": "0.0.0"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	updated, err := syncVersionMetadataFiles(manifestPath, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 {
		t.Fatalf("updated files = %#v, want manifest and lock", updated)
	}
	var manifest map[string]any
	readJSONForTest(t, manifestPath, &manifest)
	if got := metadataString(manifest["version"]); got != toolVersion+"+codex.keep" {
		t.Fatalf("manifest version = %q", got)
	}
	author, _ := manifest["author"].(map[string]any)
	if got := metadataString(author["name"]); got != toolDeveloperName {
		t.Fatalf("manifest author = %q", got)
	}
	if got := metadataString(manifest["license"]); got != toolLicenseIdentifier {
		t.Fatalf("manifest license = %q", got)
	}
	var lock map[string]any
	readJSONForTest(t, lockPath, &lock)
	if got := metadataString(lock["pluginVersion"]); got != toolVersion {
		t.Fatalf("lock pluginVersion = %q", got)
	}
	if got := metadataString(lock["slidexCliVersion"]); got != toolVersion {
		t.Fatalf("lock slidexCliVersion = %q", got)
	}
	if got := metadataString(lock["marketplaceName"]); got != pluginMarketplaceName {
		t.Fatalf("lock marketplaceName = %q", got)
	}
}

func readJSONForTest(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("%s: %v", filepath.ToSlash(path), err)
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
	root := repoRootForTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

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
	state.CodexRuntime.InstalledVersion = "0.139.0"
	if risk := protocolMismatchAcceptedRisk(state); risk != nil {
		t.Fatalf("higher Codex CLI version should not record an accepted risk: %#v", risk)
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

func TestQAStatusSeparatesDeterministicAndVisualResults(t *testing.T) {
	if got := combinedQAStatus("pass", "pass_with_risks", nil); got != "pass_with_risks" {
		t.Fatalf("visual risk should affect overall status, got %q", got)
	}
	if got := combinedQAStatus("pass", "blocked", nil); got != "fail" {
		t.Fatalf("blocked visual review should fail overall status, got %q", got)
	}
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(outDir, "qa_report.md")
	result := qaResult{
		ToolName:            toolName,
		Version:             toolVersion,
		DeckDir:             deck,
		Status:              "pass_with_risks",
		DeterministicStatus: "pass",
		VisualStatus:        "pass_with_risks",
		RuntimeMode:         "deterministic",
	}
	if err := writeQAReport(reportPath, result); err != nil {
		t.Fatal(err)
	}
	report := readFileOrEmpty(reportPath)
	for _, want := range []string{"deterministicStatus: pass", "visualStatus: pass_with_risks", "Overall status: pass_with_risks"} {
		if !strings.Contains(report, want) {
			t.Fatalf("qa report missing %q:\n%s", want, report)
		}
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

func TestChromeDiscoveryPolicyCoversSupportedPlatforms(t *testing.T) {
	envVars := chromeEnvironmentVariables()
	for _, name := range []string{"CHROME_BIN", "GOOGLE_CHROME_BIN", "CHROMIUM_BIN", "MSEDGE_BIN", "CHROME_FOR_TESTING_BIN", "PLAYWRIGHT_CHROMIUM_BIN", "PLAYWRIGHT_CHROME_BIN", "PUPPETEER_EXECUTABLE_PATH"} {
		if !testStringSliceContains(envVars, name) {
			t.Fatalf("chrome environment variables missing %s: %#v", name, envVars)
		}
	}
	if got := cleanExecutablePath(` "C:\Program Files\Google\Chrome\Application\chrome.exe" `); got != `C:\Program Files\Google\Chrome\Application\chrome.exe` {
		t.Fatalf("clean executable path did not strip quoting: %q", got)
	}

	linux := chromeExecutableCandidates("linux")
	for _, name := range []string{"google-chrome-stable", "chromium-browser", "microsoft-edge-stable"} {
		if !testStringSliceContains(linux, name) {
			t.Fatalf("linux chrome candidates missing %s: %#v", name, linux)
		}
	}
	darwin := chromeExecutableCandidates("darwin")
	if !testStringSliceContains(darwin, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome") {
		t.Fatalf("darwin chrome candidates missing app bundle path: %#v", darwin)
	}
	t.Setenv("ProgramW6432", `C:\Program Files`)
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramFiles(x86)", `C:\Program Files (x86)`)
	t.Setenv("LOCALAPPDATA", `C:\Users\Me\AppData\Local`)
	windows := chromeExecutableCandidates("windows")
	for _, name := range []string{"chrome.exe", "msedge.exe", "chromium.exe"} {
		if !testStringSliceContains(windows, name) {
			t.Fatalf("windows chrome candidates missing %s: %#v", name, windows)
		}
	}
	if want := filepath.Join(`C:\Program Files`, "Google", "Chrome", "Application", "chrome.exe"); !testStringSliceContains(windows, want) {
		t.Fatalf("windows chrome candidates missing ProgramW6432 Chrome path: %#v", windows)
	}
}

func TestChromeDiscoveryIncludesManagedLinuxCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.UserHomeDir does not use HOME on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	chromePath := filepath.Join(home, ".cache", "ms-playwright", "chromium-1234", "chrome-linux", "chrome")
	if err := os.MkdirAll(filepath.Dir(chromePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chromePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if candidates := chromeExecutableCandidates("linux"); !testStringSliceContains(candidates, chromePath) {
		t.Fatalf("managed Playwright Chromium candidate missing %s: %#v", chromePath, candidates)
	}
}

func TestWindowsChromeExecutableExtensionRejectsBatchWrappers(t *testing.T) {
	for _, path := range []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Chrome\chrome.com`} {
		if !windowsChromeExecutableExtensionSupported(path) {
			t.Fatalf("expected Windows Chrome executable path to be accepted: %s", path)
		}
	}
	for _, path := range []string{`C:\Tools\chrome.cmd`, `C:\Tools\chrome.bat`, `C:\Tools\chrome.ps1`, `C:\Tools\chrome`} {
		if windowsChromeExecutableExtensionSupported(path) {
			t.Fatalf("expected Windows Chrome wrapper path to be rejected: %s", path)
		}
	}
}

func TestResolveChromeAcceptsMacOSAppBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS app bundle executability is not represented on Windows")
	}
	bundle := filepath.Join(t.TempDir(), "Google Chrome.app")
	exe := filepath.Join(bundle, "Contents", "MacOS", "Google Chrome")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveChrome(bundle); err != nil || got != exe {
		t.Fatalf("explicit app bundle resolved to %q, %v; want %q", got, err, exe)
	}
	t.Setenv("CHROME_BIN", bundle)
	if got, err := resolveChrome(""); err != nil || got != exe {
		t.Fatalf("env app bundle resolved to %q, %v; want %q", got, err, exe)
	}
}

func TestResolveChromeRejectsNonExecutableExplicitFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executability is not represented by Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(path, []byte("not executable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveChrome(path); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("expected non-executable Chrome path to fail clearly, got %v", err)
	}
}

func TestChromeHeadlessBaseArgsUsesIsolatedProfile(t *testing.T) {
	args, cleanup, err := chromeHeadlessBaseArgs(true)
	if err != nil {
		t.Fatal(err)
	}
	if !testStringSliceContains(args, "--no-sandbox") {
		t.Fatalf("chrome args missing --no-sandbox: %#v", args)
	}
	for _, want := range []string{"--headless=new", "--no-first-run", "--disable-background-networking"} {
		if !testStringSliceContains(args, want) {
			t.Fatalf("chrome args missing %s: %#v", want, args)
		}
	}
	profileDir := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "--user-data-dir=") {
			profileDir = strings.TrimPrefix(arg, "--user-data-dir=")
			break
		}
	}
	if profileDir == "" {
		t.Fatalf("chrome args missing user data dir: %#v", args)
	}
	if info, err := os.Stat(profileDir); err != nil || !info.IsDir() {
		t.Fatalf("chrome profile dir was not created: %s %v", profileDir, err)
	}
	cleanup()
	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Fatalf("chrome profile dir was not cleaned up: %s %v", profileDir, err)
	}

	args, cleanup, err = chromeHeadlessBaseArgs(false)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if testStringSliceContains(args, "--no-sandbox") {
		t.Fatalf("chrome args unexpectedly included --no-sandbox: %#v", args)
	}
}

func testStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestStructuredReviewTurnCanNormalizeSchemaDrift(t *testing.T) {
	if !canNormalizeStructuredReviewTurn(errors.New("app-server final message is not JSON: invalid character")) {
		t.Fatal("expected non-JSON final message to be normalizable")
	}
	if canNormalizeStructuredReviewTurn(errors.New("app-server turn failed")) {
		t.Fatal("failed turns must not be silently normalized")
	}
}

func TestWebSocketRiskIsRecordedInStateAndDeliverySummary(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	risk := "Non-loopback WebSocket App Server requires explicit auth and external TLS or SSH tunnel."
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
	if !strings.Contains(summary, "Risk state hash:") {
		t.Fatalf("delivery summary did not include risk state hash: %s", summary)
	}
	if !strings.Contains(summary, "## Assumptions And Blockers") {
		t.Fatalf("delivery summary did not include assumptions/blockers: %s", summary)
	}
	if hasFailures(verifyDeliverySummaryPolicy(path)) {
		t.Fatalf("delivery summary should satisfy package policy: %#v", verifyDeliverySummaryPolicy(path))
	}
	findings := verifyTextArtifactFreshness("delivery_summary", path, filepath.Join(outDir, "render_manifest.json"), []string{mustSHA256(filepath.Join(outDir, "render_manifest.json")), mustSHA256(filepath.Join(outDir, "qa_report.md")), riskStateHashForDeck(deck)})
	if hasFailures(findings) {
		t.Fatalf("delivery summary should be fresh for manifest, QA, and state hashes: %#v", findings)
	}
	if err := recordWebSocketTransportRisk(deck, "new risk", filepath.Join(outDir, "codex-app-server.json")); err != nil {
		t.Fatal(err)
	}
	findings = verifyTextArtifactFreshness("delivery_summary", path, filepath.Join(outDir, "render_manifest.json"), []string{riskStateHashForDeck(deck)})
	if !hasFailures(findings) {
		t.Fatalf("delivery summary should be stale after state risk change")
	}
}

func TestTransportRiskForListenTreatsLoopbackWebSocketAsSupported(t *testing.T) {
	if risk := transportRiskForListen("ws://127.0.0.1:49200/app"); risk != "" {
		t.Fatalf("loopback websocket should not record transport risk: %q", risk)
	}
	if risk := transportRiskForListen("ws://0.0.0.0:49200/app"); !strings.Contains(risk, "Non-loopback") {
		t.Fatalf("non-loopback websocket should require explicit network protection, got %q", risk)
	}
	if risk := transportRiskForListen("wss://0.0.0.0:49200/app"); !strings.Contains(risk, "Non-loopback") {
		t.Fatalf("non-loopback secure websocket should still require explicit auth policy, got %q", risk)
	}
}

func TestVerifyDeliverySummaryPolicyRequiresHashesQAAndBlockers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery_summary.md")
	good := strings.Join([]string{
		"# Delivery Summary",
		"- Render manifest hash: `manifest`",
		"- QA report hash: `qa`",
		"- Risk state hash: `risk`",
		"- PNG set hash: `png`",
		"## QA Status",
		"- Deterministic QA report: `out/qa_report.md`",
		"## Accepted Risks",
		"- None recorded.",
		"## Unresolved Risks",
		"- None recorded.",
		"## Assumptions And Blockers",
		"- Approved assumptions: none recorded.",
		"- Blockers: none recorded.",
	}, "\n")
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if findings := verifyDeliverySummaryPolicy(path); hasFailures(findings) {
		t.Fatalf("good delivery summary should pass policy: %#v", findings)
	}
	bad := strings.ReplaceAll(good, "## Assumptions And Blockers\n- Approved assumptions: none recorded.\n- Blockers: none recorded.", "")
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if findings := verifyDeliverySummaryPolicy(path); !hasFindingCheck(findings, "ED-PACKAGE-002") {
		t.Fatalf("bad delivery summary should fail ED-PACKAGE-002: %#v", findings)
	}
}

func TestWriteJSONFileUsesSecurePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "payload.json")
	if err := writeJSONFile(path, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	assertSecureModeForTest(t, path, info, 0o600)
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	assertSecureModeForTest(t, filepath.Dir(path), dirInfo, 0o700)
}

func TestCleanLogsKeepsReviewArtifacts(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	for _, rel := range []string{"run_log.jsonl", filepath.Join("agent_runs", "turn.json"), filepath.Join("agent_reviews", "round_01", "reviewer_delivery.json"), filepath.Join("visual_reviews", "latest_review.json")} {
		path := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Dir(path), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(filepath.Join(outDir, "agent_runs"), time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := runClean([]string{"--deck", deck, "--logs", "--older-than", "1h"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "agent_runs")); !os.IsNotExist(err) {
		t.Fatalf("agent_runs should be removed, stat err=%v", err)
	}
	for _, rel := range []string{filepath.Join("agent_reviews", "round_01", "reviewer_delivery.json"), filepath.Join("visual_reviews", "latest_review.json")} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("delivery artifact should remain %s: %v", rel, err)
		}
	}
}

func TestCleanLogsKeepsAgentRunsWithFreshNestedFiles(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	runDir := filepath.Join(deck, "out", "agent_runs")
	nested := filepath.Join(runDir, "nested", "fresh.json")
	if err := os.MkdirAll(filepath.Dir(nested), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(runDir, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Dir(nested), old, old); err != nil {
		t.Fatal(err)
	}
	if err := runClean([]string{"--deck", deck, "--logs", "--older-than", "1h"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("fresh nested agent run should remain: %v", err)
	}
}

func TestCleanLogsRetainsLatestSuccessfulAndFailedRuns(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	runDir := filepath.Join(outDir, "agent_runs")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	lines := []string{
		runLogLine("old-success", "resolve_workspace", "pass", now.Add(-8*time.Hour)),
		runLogLine("old-success", "package", "pass", now.Add(-7*time.Hour)),
		runLogLine("latest-failed", "resolve_workspace", "pass", now.Add(-6*time.Hour)),
		runLogLine("latest-failed", "qa", "fail", now.Add(-5*time.Hour)),
		runLogLine("latest-success", "resolve_workspace", "pass", now.Add(-4*time.Hour)),
		runLogLine("latest-success", "package", "pass", now.Add(-3*time.Hour)),
		runLogLine("old-failed", "resolve_workspace", "pass", now.Add(-10*time.Hour)),
		runLogLine("old-failed", "render", "fail", now.Add(-9*time.Hour)),
	}
	if err := os.WriteFile(filepath.Join(outDir, "run_log.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "turn.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(outDir, "run_log.jsonl"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(runDir, old, old); err != nil {
		t.Fatal(err)
	}
	if err := runClean([]string{"--deck", deck, "--logs", "--older-than", "1h"}); err != nil {
		t.Fatal(err)
	}
	logText := readFileOrEmpty(filepath.Join(outDir, "run_log.jsonl"))
	for _, want := range []string{"latest-success", "latest-failed"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("retained log missing %s: %s", want, logText)
		}
	}
	for _, removed := range []string{"old-success", "old-failed"} {
		if strings.Contains(logText, removed) {
			t.Fatalf("stale non-retained run remained %s: %s", removed, logText)
		}
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("agent_runs should remain while retained run logs exist: %v", err)
	}
}

func TestCleanLogsRejectsSymlinkedOutDirectory(t *testing.T) {
	root := t.TempDir()
	deck := filepath.Join(root, "deck")
	victim := filepath.Join(root, "victim")
	if err := os.MkdirAll(deck, 0o700); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(victim, "agent_runs")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runLog := filepath.Join(victim, "run_log.jsonl")
	if err := os.WriteFile(runLog, []byte(runLogLine("old-run", "package", "pass", time.Now().Add(-2*time.Hour))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFile := filepath.Join(runDir, "turn.json")
	if err := os.WriteFile(runFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, path := range []string{victim, runDir, runFile, runLog} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(victim, filepath.Join(deck, "out")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	err := runClean([]string{"--deck", deck, "--logs", "--older-than", "1h"})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlinked out directory to be rejected, got %v", err)
	}
	for _, path := range []string{victim, runDir, runFile, runLog} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("clean should not remove outside target %s: %v", path, statErr)
		}
	}
}

func TestWebSocketAuthRequiresPrivateFilesAndTunnelAck(t *testing.T) {
	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("token"), 0o644); err != nil {
		t.Fatal(err)
	}
	tokenHash := sha256Bytes([]byte("token"))
	err := validateWebSocketAuth("ws://10.0.0.2:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: tokenHash})
	if runtime.GOOS == "windows" {
		if err == nil || (!strings.Contains(err.Error(), "tunnel") && !strings.Contains(err.Error(), "ACL")) {
			t.Fatalf("expected Windows public token file to fail ACL check or tunnel ack, got %v", err)
		}
	} else if err == nil {
		t.Fatal("expected public token file to fail")
	}
	if err := os.Chmod(token, 0o600); err != nil {
		t.Fatal(err)
	}
	makeTestPrivateFile(t, token)
	if err := validateWebSocketAuth("ws://127.0.0.1:1234", webSocketAuthConfig{}); err != nil {
		t.Fatalf("loopback websocket without auth should pass: %v", err)
	}
	if err := validateWebSocketAuth("wss://127.0.0.1:1234", webSocketAuthConfig{}); err != nil {
		t.Fatalf("loopback secure websocket without auth should pass: %v", err)
	}
	err = validateWebSocketAuth("ws://127.0.0.1:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: strings.Repeat("0", 64)})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("expected loopback capability token hash mismatch to fail, got %v", err)
	}
	if err := validateWebSocketAuth("ws://127.0.0.1:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: tokenHash}); err != nil {
		t.Fatalf("loopback capability token with matching hash should pass without tunnel acknowledgement: %v", err)
	}
	t.Setenv("SLIDEX_WS_TUNNEL_ACK", "")
	err = validateWebSocketAuth("ws://10.0.0.2:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: tokenHash})
	if err == nil {
		t.Fatal("expected capability token without tunnel acknowledgement to fail")
	}
	err = validateWebSocketAuth("wss://10.0.0.2:1234", webSocketAuthConfig{})
	if err == nil || !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("expected non-loopback secure websocket without auth to fail, got %v", err)
	}
	t.Setenv("SLIDEX_WS_TUNNEL_ACK", "1")
	err = validateWebSocketAuth("ws://10.0.0.2:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: strings.Repeat("0", 64)})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("expected mismatched capability token hash to fail, got %v", err)
	}
	if err := validateWebSocketAuth("ws://10.0.0.2:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: tokenHash}); err != nil {
		t.Fatalf("private capability token with tunnel acknowledgement should pass: %v", err)
	}
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	makeTestPrivateFile(t, secret)
	t.Setenv("SLIDEX_WS_TUNNEL_ACK", "")
	err = validateWebSocketAuth("ws://10.0.0.2:1234", webSocketAuthConfig{Mode: "signed-bearer-token", SharedSecretFile: secret, Issuer: "slidex", Audience: "codex", MaxClockSkewSeconds: 30})
	if err == nil {
		t.Fatal("expected signed bearer without tunnel acknowledgement to fail")
	}
	t.Setenv("SLIDEX_WS_TUNNEL_ACK", "1")
	if err := validateWebSocketAuth("ws://10.0.0.2:1234", webSocketAuthConfig{Mode: "signed-bearer-token", SharedSecretFile: secret, Issuer: "slidex", Audience: "codex", MaxClockSkewSeconds: 30}); err != nil {
		t.Fatalf("signed bearer with tunnel acknowledgement should pass: %v", err)
	}
}

func TestWebSocketAuthRejectsOversizedCredentialFiles(t *testing.T) {
	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte(strings.Repeat("x", int(maxWebSocketCredentialBytes)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	makeTestPrivateFile(t, token)
	err := validateWebSocketAuth("ws://127.0.0.1:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: strings.Repeat("0", 64)})
	if err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected oversized token rejection, got %v", err)
	}

	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte(strings.Repeat("s", int(maxWebSocketCredentialBytes)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	makeTestPrivateFile(t, secret)
	err = validateWebSocketAuth("ws://127.0.0.1:1234", webSocketAuthConfig{Mode: "signed-bearer-token", SharedSecretFile: secret, Issuer: "slidex", Audience: "codex", MaxClockSkewSeconds: 30})
	if err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected oversized shared secret rejection, got %v", err)
	}
}

func TestPrivateFileModePolicyIsPlatformAware(t *testing.T) {
	if privateFileModeAllowed("linux", 0o644) {
		t.Fatal("linux token files must reject group/world readable permissions")
	}
	if !privateFileModeAllowed("linux", 0o600) {
		t.Fatal("linux token files should allow owner-only permissions")
	}
	if !privateFileModeAllowed("darwin", 0o600) {
		t.Fatal("darwin token files should allow owner-only permissions")
	}
	if !privateFileModeAllowed("windows", 0o644) {
		t.Fatal("windows token files should not be rejected based on Unix permission bits")
	}
}

func TestSecureWriteFileReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := secureWriteFile(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := secureWriteFile(path, []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readFileOrEmpty(path); got != "second\n" {
		t.Fatalf("secure write did not replace existing file: %q", got)
	}
}

func TestCreateMontageRejectsTooManySlides(t *testing.T) {
	paths := make([]string, maxMontageSlides+1)
	for i := range paths {
		paths[i] = filepath.Join("missing", fmt.Sprintf("slide_%03d.png", i+1))
	}

	_, err := createMontage(filepath.Join(t.TempDir(), "qa_montage.png"), paths)
	if err == nil || !strings.Contains(err.Error(), "too many PNG files") {
		t.Fatalf("expected montage slide-count budget rejection, got %v", err)
	}
}

func TestCreateMontageRejectsCanvasBudget(t *testing.T) {
	pngPath := filepath.Join(t.TempDir(), "slide_01.png")
	writeSolidPNGForTest(t, pngPath, color.RGBA{R: 255, A: 255})
	paths := make([]string, maxMontageSlides)
	for i := range paths {
		paths[i] = pngPath
	}

	_, err := createMontage(filepath.Join(t.TempDir(), "qa_montage.png"), paths)
	if err == nil || !strings.Contains(err.Error(), "montage canvas exceeds maximum pixel count") {
		t.Fatalf("expected montage canvas budget rejection, got %v", err)
	}
}

func TestDeliveryOutputsRejectSymlinkTargets(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "deck", "out")
	renderedDir := filepath.Join(outDir, "rendered_slides")
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(renderedDir, "slide_01.png")
	writeSolidPNGForTest(t, pngPath, color.RGBA{R: 255, A: 255})

	cases := []struct {
		name  string
		path  string
		write func(string) error
	}{
		{
			name: "pdf",
			path: filepath.Join(outDir, "final_deck.pdf"),
			write: func(path string) error {
				return writePDFFromPNGs(path, []string{pngPath}, 540, 540)
			},
		},
		{
			name: "montage",
			path: filepath.Join(outDir, "qa_montage.png"),
			write: func(path string) error {
				_, err := createMontage(path, []string{pngPath})
				return err
			},
		},
		{
			name: "qa_report",
			path: filepath.Join(outDir, "qa_report.md"),
			write: func(path string) error {
				return writeQAReport(path, qaResult{ToolName: toolName, Version: toolVersion, DeckDir: filepath.Dir(outDir), Status: "pass"})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outside := filepath.Join(t.TempDir(), tc.name+".outside")
			if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, tc.path); err != nil {
				t.Skipf("symlink unavailable on this platform: %v", err)
			}
			err := tc.write(tc.path)
			if err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("expected symlink rejection, got %v", err)
			}
			if got := readFileOrEmpty(outside); got != "outside\n" {
				t.Fatalf("outside target was modified: %q", got)
			}
		})
	}
}

func TestTextArtifactsRejectSymlinkTargets(t *testing.T) {
	t.Chdir(repoRootForTest(t))
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		path  string
		write func(string) error
	}{
		{
			name: "source_inventory",
			path: filepath.Join(outDir, "source_inventory.md"),
			write: func(path string) error {
				return writeSourceInventory(inventory{ToolName: toolName, Version: toolVersion, DeckDir: deck, OutDir: filepath.Dir(path)})
			},
		},
		{
			name: "strategy",
			path: filepath.Join(outDir, "strategy.md"),
			write: func(string) error {
				_, err := ensureStrategy(deck, true)
				return err
			},
		},
		{
			name: "intake_questions",
			path: filepath.Join(outDir, "intake_questions.md"),
			write: func(string) error {
				return writeIntakeQuestions(deck, []string{"Question?"}, "user_input_required")
			},
		},
		{
			name: "brief_append",
			path: filepath.Join(deck, "brief.md"),
			write: func(string) error {
				return appendIntakeAnswers(deck, []byte(`{"answers":["A"]}`))
			},
		},
		{
			name: "notes_append",
			path: filepath.Join(outDir, "notes.md"),
			write: func(path string) error {
				return appendNotes(path, "Test", []string{"change"})
			},
		},
		{
			name: "sync_report",
			path: filepath.Join(outDir, "html_edit_sync.md"),
			write: func(path string) error {
				return writeSyncReport(path, map[string]any{"comparisonSource": "test", "renderStatus": "not_run", "qaStatus": "not_run"})
			},
		},
		{
			name: "source_json",
			path: filepath.Join(outDir, "source.json"),
			write: func(path string) error {
				return writeSourceJSONFile(path, map[string]any{"ok": true})
			},
		},
		{
			name: "delivery_summary",
			path: filepath.Join(outDir, "delivery_summary.md"),
			write: func(string) error {
				_, err := writeDeliverySummary(deck)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(tc.path), 0o755); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), tc.name+".outside")
			if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_ = os.Remove(tc.path)
			if err := os.Symlink(outside, tc.path); err != nil {
				t.Skipf("symlink unavailable on this platform: %v", err)
			}
			err := tc.write(tc.path)
			if err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("expected symlink rejection, got %v", err)
			}
			if got := readFileOrEmpty(outside); got != "outside\n" {
				t.Fatalf("outside target was modified: %q", got)
			}
			_ = os.Remove(tc.path)
		})
	}
}

func TestAuthoringPromptRejectsSymlinkedDeckText(t *testing.T) {
	t.Chdir(repoRootForTest(t))
	deck := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(filepath.Join(deck, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("PRIVATE_SENTINEL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(deck, "brief.md")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	prompt, err := authoringPrompt(deck, slidexState{}, "strategy", "exec")
	if err == nil {
		t.Fatalf("expected symlinked brief to be rejected; prompt contained sentinel=%v", strings.Contains(prompt, "PRIVATE_SENTINEL"))
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if strings.Contains(prompt, "PRIVATE_SENTINEL") {
		t.Fatalf("symlinked outside content was read into prompt:\n%s", prompt)
	}
}

func TestFileSnapshotTransactionRollbackRestoresFiles(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.md")
	missing := filepath.Join(dir, "created.md")
	if err := os.WriteFile(existing, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tx, err := beginFileSnapshotTransaction(existing, missing)
	if err != nil {
		t.Fatal(err)
	}
	if err := secureWriteFile(existing, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := secureWriteFile(missing, []byte("created\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := readFileOrEmpty(existing); got != "before\n" {
		t.Fatalf("existing file was not restored: %q", got)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("created file should have been removed, got %v", err)
	}
}

func TestSyncHTMLEditsRollsBackMetadataOnRenderConfigFailure(t *testing.T) {
	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baselineHTML := `<!doctype html><html><body><section class="slide" id="slide_01"><h1>Old headline</h1><p>Old body</p></section></body></html>`
	currentHTML := `<!doctype html><html><body><section class="slide" id="slide_01"><h1>New headline</h1><p>New body</p></section></body></html>`
	specRaw := `{"slides":[{"id":"intro","htmlId":"slide_01","headline":"Old headline","bodyContent":["Old body"]}]}`
	notesRaw := "original notes\n"
	qaRaw := "original qa report\n"
	files := map[string]string{
		filepath.Join(outDir, "final_deck.html"):                    currentHTML,
		filepath.Join(outDir, "final_deck.generated_baseline.html"): baselineHTML,
		filepath.Join(outDir, "deck_spec.json"):                     specRaw,
		filepath.Join(outDir, "notes.md"):                           notesRaw,
		filepath.Join(outDir, "qa_report.md"):                       qaRaw,
	}
	for path, raw := range files {
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	report, err := syncHTMLEdits(deck, 0, 1080, "pretendard", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if report["renderStatus"] != "failed" {
		t.Fatalf("renderStatus = %v, want failed", report["renderStatus"])
	}
	for path, want := range map[string]string{
		filepath.Join(outDir, "deck_spec.json"):                     specRaw,
		filepath.Join(outDir, "notes.md"):                           notesRaw,
		filepath.Join(outDir, "final_deck.generated_baseline.html"): baselineHTML,
		filepath.Join(outDir, "qa_report.md"):                       qaRaw,
	} {
		if got := readFileOrEmpty(path); got != want {
			t.Fatalf("%s was not rolled back:\n got %q\nwant %q", filepath.Base(path), got, want)
		}
	}
	syncReport := readFileOrEmpty(filepath.Join(outDir, "html_edit_sync.md"))
	if !strings.Contains(syncReport, "Render status: `failed`") || !strings.Contains(syncReport, "render or QA did not complete") {
		t.Fatalf("sync report should retain render failure after rollback:\n%s", syncReport)
	}
}

func TestSyncHTMLEditsUpdatesBaselineBeforeQA(t *testing.T) {
	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baselineHTML := `<!doctype html><html><body><section class="slide" id="slide_01"><h1>Old headline</h1><p>Old body</p></section></body></html>`
	currentHTML := `<!doctype html><html><body><section class="slide" id="slide_01"><h1>New headline</h1><p>New body</p></section></body></html>`
	specRaw := `{"slides":[{"id":"intro","htmlId":"slide_01","headline":"Old headline","bodyContent":["Old body"]}]}`
	for path, raw := range map[string]string{
		filepath.Join(outDir, "final_deck.html"):                    currentHTML,
		filepath.Join(outDir, "final_deck.generated_baseline.html"): baselineHTML,
		filepath.Join(outDir, "deck_spec.json"):                     specRaw,
		filepath.Join(outDir, "notes.md"):                           "original notes\n",
		filepath.Join(outDir, "qa_report.md"):                       "original qa report\n",
	} {
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	oldRenderHTML := syncRenderHTML
	oldQADeck := syncQADeck
	t.Cleanup(func() {
		syncRenderHTML = oldRenderHTML
		syncQADeck = oldQADeck
	})
	syncRenderHTML = func(renderConfig) (renderManifest, error) {
		return renderManifest{}, nil
	}
	syncQADeck = func(deckAbs string, writeReport bool) (qaResult, error) {
		if got, want := mustSHA256(filepath.Join(deckAbs, "out", "final_deck.generated_baseline.html")), mustSHA256(filepath.Join(deckAbs, "out", "final_deck.html")); got != want {
			t.Fatalf("QA saw stale baseline hash %s, want current HTML hash %s", got, want)
		}
		if err := os.WriteFile(filepath.Join(deckAbs, "out", "qa_report.md"), []byte("qa pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return qaResult{Status: "pass"}, nil
	}

	report, err := syncHTMLEdits(deck, 1920, 1080, "pretendard", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if report["qaStatus"] != "pass" {
		t.Fatalf("qaStatus = %v, want pass", report["qaStatus"])
	}
	if got, want := readFileOrEmpty(filepath.Join(outDir, "final_deck.generated_baseline.html")), currentHTML; got != want {
		t.Fatalf("baseline was not updated to current HTML:\n got %q\nwant %q", got, want)
	}
	accepted, _ := report["acceptedChanges"].([]string)
	if len(accepted) == 0 {
		t.Fatalf("expected accepted changes in report: %#v", report)
	}
}

func TestSyncHTMLEditsReportsRestoredBaselineHashAfterQAFailure(t *testing.T) {
	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baselineHTML := `<!doctype html><html><body><section class="slide" id="slide_01"><h1>Old headline</h1><p>Old body</p></section></body></html>`
	currentHTML := `<!doctype html><html><body><section class="slide" id="slide_01"><h1>New headline</h1><p>New body</p></section></body></html>`
	specRaw := `{"slides":[{"id":"intro","htmlId":"slide_01","headline":"Old headline","bodyContent":["Old body"]}]}`
	for path, raw := range map[string]string{
		filepath.Join(outDir, "final_deck.html"):                    currentHTML,
		filepath.Join(outDir, "final_deck.generated_baseline.html"): baselineHTML,
		filepath.Join(outDir, "deck_spec.json"):                     specRaw,
		filepath.Join(outDir, "notes.md"):                           "original notes\n",
		filepath.Join(outDir, "qa_report.md"):                       "original qa report\n",
	} {
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	oldRenderHTML := syncRenderHTML
	oldQADeck := syncQADeck
	t.Cleanup(func() {
		syncRenderHTML = oldRenderHTML
		syncQADeck = oldQADeck
	})
	syncRenderHTML = func(renderConfig) (renderManifest, error) {
		return renderManifest{}, nil
	}
	syncQADeck = func(deckAbs string, writeReport bool) (qaResult, error) {
		if got, want := mustSHA256(filepath.Join(deckAbs, "out", "final_deck.generated_baseline.html")), mustSHA256(filepath.Join(deckAbs, "out", "final_deck.html")); got != want {
			t.Fatalf("QA should validate candidate baseline before rollback: got %s, want %s", got, want)
		}
		if err := os.WriteFile(filepath.Join(deckAbs, "out", "qa_report.md"), []byte("qa fail\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return qaResult{Status: "fail"}, nil
	}

	report, err := syncHTMLEdits(deck, 1920, 1080, "pretendard", "", false)
	if err != nil {
		t.Fatal(err)
	}
	restoredHash := sha256Bytes([]byte(baselineHTML))
	currentHash := sha256Bytes([]byte(currentHTML))
	if got := readFileOrEmpty(filepath.Join(outDir, "final_deck.generated_baseline.html")); got != baselineHTML {
		t.Fatalf("baseline was not restored after QA failure:\n got %q\nwant %q", got, baselineHTML)
	}
	if report["newBaselineHash"] != restoredHash {
		t.Fatalf("newBaselineHash = %v, want restored hash %s", report["newBaselineHash"], restoredHash)
	}
	if report["newBaselineHash"] == currentHash {
		t.Fatalf("newBaselineHash should not report rejected candidate hash: %#v", report)
	}
	syncReport := readFileOrEmpty(filepath.Join(outDir, "html_edit_sync.md"))
	if !strings.Contains(syncReport, restoredHash) {
		t.Fatalf("sync report should describe restored baseline hash, got:\n%s", syncReport)
	}
}

func TestUpdateSpecFromHTMLPreservesLogicalID(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "deck_spec.json")
	spec := map[string]any{
		"slides": []any{
			map[string]any{
				"id":           "intro",
				"htmlId":       "slide_01",
				"sectionRole":  "cover",
				"slideType":    "cover",
				"headline":     "Old headline",
				"keyMessage":   "Old headline",
				"bodyContent":  []any{"Old body"},
				"evidenceRefs": []any{},
			},
		},
	}
	if err := writeJSONFile(specPath, spec); err != nil {
		t.Fatal(err)
	}
	if err := updateSpecFromHTML(specPath, []slideInfo{{ID: "slide_01", Headline: "New headline", Text: "New headline. New body"}}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	slides, _ := got["slides"].([]any)
	if len(slides) != 1 {
		t.Fatalf("slides length = %d", len(slides))
	}
	slide, _ := slides[0].(map[string]any)
	if slide["id"] != "intro" || slide["htmlId"] != "slide_01" {
		t.Fatalf("logical id/htmlId not preserved: %#v", slide)
	}
	if slide["headline"] != "New headline" {
		t.Fatalf("headline not updated: %#v", slide)
	}
}

func TestCopyDirPreservesExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix executable mode bits")
	}
	src := filepath.Join(t.TempDir(), "src")
	script := filepath.Join(src, "scripts", "doctor.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dst, "scripts", "doctor.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("copied executable mode = %o, want 755", got)
	}
}

func TestCopyDirRejectsSymlinkSources(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(src, "link.txt")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	err := copyDir(src, filepath.Join(t.TempDir(), "dst"))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected copyDir symlink source rejection, got %v", err)
	}
}

func TestCopyDirRejectsSymlinkDestinationDirectories(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(filepath.Join(src, "nested", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dst, "nested")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	err := copyDir(src, dst)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected copyDir symlink destination rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "empty")); !os.IsNotExist(err) {
		t.Fatalf("copyDir created directory through symlink, stat err=%v", err)
	}
}

func TestCopyDirWithBudgetRejectsEntryAndTotalLimits(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "first.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "second.txt"), []byte("67890"), 0o644); err != nil {
		t.Fatal(err)
	}

	entryErr := copyDirWithBudget(src, filepath.Join(t.TempDir(), "entry-dst"), copyDirBudget{label: "test", maxEntries: 1, maxFileBytes: 10, maxTotalBytes: 20})
	if entryErr == nil || !strings.Contains(entryErr.Error(), "too many entries") {
		t.Fatalf("expected entry budget error, got %v", entryErr)
	}
	totalErr := copyDirWithBudget(src, filepath.Join(t.TempDir(), "total-dst"), copyDirBudget{label: "test", maxEntries: 10, maxFileBytes: 10, maxTotalBytes: 8})
	if totalErr == nil || !strings.Contains(totalErr.Error(), "maximum total size") {
		t.Fatalf("expected total budget error, got %v", totalErr)
	}
}

func TestCopyFileRejectsSymlinkEndpoints(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(src, []byte("src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dst); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	if err := copyFile(src, dst); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected copyFile symlink destination rejection, got %v", err)
	}
	linkSrc := filepath.Join(dir, "link-src.txt")
	if err := os.Symlink(src, linkSrc); err != nil {
		t.Skipf("second symlink unavailable on this platform: %v", err)
	}
	if err := copyFile(linkSrc, filepath.Join(dir, "copy.txt")); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected copyFile symlink source rejection, got %v", err)
	}
}

func TestCopyFileReplacesHardlinkedDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(src, []byte("src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hardlinkOrSkip(t, outside, dst)
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if got := readFileOrEmpty(outside); got != "outside\n" {
		t.Fatalf("outside hardlinked file was modified: %q", got)
	}
	if got := readFileOrEmpty(dst); got != "src\n" {
		t.Fatalf("copy destination = %q, want source contents", got)
	}
}

func TestOpenSecureTruncateFileReplacesHardlinkedTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.log")
	target := filepath.Join(dir, "target.log")
	if err := os.WriteFile(outside, []byte("outside log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hardlinkOrSkip(t, outside, target)
	f, err := openSecureTruncateFile(target, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("target log\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readFileOrEmpty(outside); got != "outside log\n" {
		t.Fatalf("outside hardlinked file was modified: %q", got)
	}
	if got := readFileOrEmpty(target); got != "target log\n" {
		t.Fatalf("truncate target = %q, want new target contents", got)
	}
}

func TestOpenSecureAppendFileRejectsHardlinkedTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.log")
	target := filepath.Join(dir, "target.log")
	if err := os.WriteFile(outside, []byte("outside log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hardlinkOrSkip(t, outside, target)
	f, err := openSecureAppendFile(target, 0o600)
	if err == nil {
		_ = f.Close()
		t.Fatal("expected hardlinked append target to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "hardlink") {
		t.Fatalf("expected hardlink rejection, got %v", err)
	}
	if got := readFileOrEmpty(outside); got != "outside log\n" {
		t.Fatalf("outside hardlinked file was modified: %q", got)
	}
}

func hardlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Link(oldname, newname); err != nil {
		t.Skipf("hardlink unavailable on this platform or filesystem: %v", err)
	}
}

func TestReadHashesRejectSymlinkTargets(t *testing.T) {
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	if _, err := sha256File(link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected sha256File symlink rejection, got %v", err)
	}
	if _, err := readRegularFile(link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected readRegularFile symlink rejection, got %v", err)
	}
	artifact := artifactFromPath(link)
	if artifact.SHA256 != "" || artifact.Size != 0 {
		t.Fatalf("artifact should not hash symlink target: %#v", artifact)
	}
	if got := artifactsForExisting([]string{link}); len(got) != 0 {
		t.Fatalf("artifactsForExisting should skip symlink target: %#v", got)
	}
}

func TestSHA256FileWithMaxBytesRejectsOversizedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sha256FileWithMaxBytes(path, 16); err == nil || !strings.Contains(err.Error(), "maximum hash size") {
		t.Fatalf("expected bounded hash rejection, got %v", err)
	}
}

func TestLocalDependenciesDoNotHashOutsideDeckWorkspace(t *testing.T) {
	root := t.TempDir()
	deck := filepath.Join(root, "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(outDir, "final_deck.html")
	deps, findings := localDependenciesWithFindings(htmlPath, `<img src="../../outside.txt">`)
	if len(findings) != 0 {
		t.Fatalf("dependency scan findings = %#v", findings)
	}
	if len(deps) != 1 {
		t.Fatalf("expected one dependency, got %#v", deps)
	}
	if deps[0].SHA256 != "" {
		t.Fatalf("outside dependency should not be hashed: %#v", deps[0])
	}
	if !strings.Contains(deps[0].Risk, "outside the deck workspace") {
		t.Fatalf("outside dependency risk missing: %#v", deps[0])
	}
}

func TestVerifyManifestDependenciesRejectsOutsidePathBeforeHash(t *testing.T) {
	root := t.TempDir()
	deck := filepath.Join(root, "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := sha256Bytes([]byte("outside"))
	findings := verifyManifestDependencies("asset", []dependency{{Kind: "asset", Path: outside, SHA256: hash}}, nil, filepath.Join(outDir, "render_manifest.json"))
	if !findingCheckPresent(findings, "package.asset_dependency") {
		t.Fatalf("outside manifest dependency finding missing: %#v", findings)
	}
	if !strings.Contains(findings[0].Message, "outside the deck workspace") {
		t.Fatalf("outside dependency message missing: %#v", findings)
	}
}

func TestRenderConfigRejectsOversizedViewport(t *testing.T) {
	_, err := renderConfigFromFlags("deck/out/final_deck.html", "", "", "", "paginated", ".slide", int(maxRenderedPNGPixels)+1, 1, "pretendard", "", false)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum pixel count") {
		t.Fatalf("expected oversized viewport rejection, got %v", err)
	}
}

func TestExtractSlidesWithChromeRejectsTooManySlidesBeforeChrome(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body>`)
	for i := 0; i < maxRenderSlides+1; i++ {
		fmt.Fprintf(&b, `<section class="slide" id="slide_%03d"><h1>Slide</h1></section>`, i)
	}
	b.WriteString(`</body></html>`)
	_, _, err := extractSlidesWithChromeFromHTML(filepath.Join(t.TempDir(), "missing-chrome"), filepath.Join(t.TempDir(), "final_deck.html"), b.String(), ".slide", false)
	if err == nil || !strings.Contains(err.Error(), "too many slides") {
		t.Fatalf("expected slide limit before Chrome, got %v", err)
	}
}

func TestRenderHTMLPropagatesSlidePolicyErrorWithoutFallback(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body>`)
	for i := 0; i < maxRenderSlides+1; i++ {
		fmt.Fprintf(&b, `<section class="slide" id="slide_%03d"><h1>Slide</h1></section>`, i)
	}
	b.WriteString(`</body></html>`)
	htmlPath := filepath.Join(dir, "final_deck.html")
	if err := os.WriteFile(htmlPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	_, err = renderHTML(renderConfig{
		HTMLPath:     htmlPath,
		OutDir:       filepath.Join(dir, "rendered_slides"),
		PDFPath:      filepath.Join(dir, "final_deck.pdf"),
		ManifestPath: filepath.Join(dir, "render_manifest.json"),
		MontagePath:  filepath.Join(dir, "qa_montage.png"),
		PDFMode:      "paginated",
		Selector:     ".slide",
		Width:        160,
		Height:       90,
		FontPreset:   "pretendard",
		ChromePath:   exe,
	})
	if err == nil || !isSlideEnumerationPolicyError(err) || !strings.Contains(err.Error(), "too many slides") {
		t.Fatalf("expected terminal slide policy error, got %v", err)
	}
}

func TestExtractSlidesRegexWithLimitStopsAtSlideCap(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b, `<section class="slide" id="slide_%d"><h1>Slide</h1></section>`, i)
	}
	slides, err := extractSlidesRegexWithLimit(b.String(), 2)
	if err == nil || !isSlideEnumerationPolicyError(err) {
		t.Fatalf("expected regex slide cap error, got slides=%d err=%v", len(slides), err)
	}
	if len(slides) != 3 {
		t.Fatalf("expected cap sentinel slide to be observed, got %d", len(slides))
	}
}

func TestDecodeChromeSlideEnumerationReportsPolicyError(t *testing.T) {
	payload := fmt.Sprintf(`{"slides":[],"totalSlides":%d,"error":"slide enumeration payload exceeds maximum size"}`, maxRenderSlides+1)
	_, err := decodeChromeSlideEnumeration(payload)
	if err == nil || !isSlideEnumerationPolicyError(err) || !strings.Contains(err.Error(), "payload exceeds") {
		t.Fatalf("expected Chrome policy error, got %v", err)
	}
}

func TestRunChromeCommandRejectsOversizedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper script is Unix-specific")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-chrome")
	body := "#!/bin/sh\n" +
		"i=0\n" +
		"while [ \"$i\" -lt 2048 ]; do printf x; i=$((i+1)); done\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := runChromeCommandWithMaxOutput(time.Second, 128, script)
	if err == nil || !strings.Contains(err.Error(), "output exceeded") {
		t.Fatalf("expected bounded chrome output error, got %v", err)
	}
	if len(out) > 128 {
		t.Fatalf("bounded output retained %d bytes", len(out))
	}
}

func TestRunChromeCommandHonorsParentContextDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper script is Unix-specific")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-chrome")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runChromeCommandContext(ctx, time.Hour, script)
	if err == nil || !isChromeCommandTimeout(err) {
		t.Fatalf("expected parent context chrome timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("parent context deadline was not honored quickly: %s", elapsed)
	}
}

func TestRenderHTMLStopsAtGlobalDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper script is Unix-specific")
	}
	oldTimeout := renderHTMLTimeout
	renderHTMLTimeout = 120 * time.Millisecond
	t.Cleanup(func() { renderHTMLTimeout = oldTimeout })

	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "final_deck.html")
	html := `<!doctype html><html><head><title>Deadline</title></head><body><section class="slide" data-slide-id="slide_01"><h1>Deadline</h1></section></body></html>`
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	chromePath := filepath.Join(dir, "fake-chrome")
	body := "#!/bin/sh\n" +
		"for arg in \"$@\"; do if [ \"$arg\" = \"--version\" ]; then echo 'Chromium 123.0.0.0'; exit 0; fi; done\n" +
		"sleep 2\n"
	if err := os.WriteFile(chromePath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := renderConfig{
		HTMLPath:        htmlPath,
		OutDir:          filepath.Join(dir, "rendered_slides"),
		PDFPath:         filepath.Join(dir, "final_deck.pdf"),
		ManifestPath:    filepath.Join(dir, "render_manifest.json"),
		MontagePath:     filepath.Join(dir, "qa_montage.png"),
		PDFMode:         "paginated",
		Selector:        ".slide",
		Width:           16,
		Height:          9,
		FontPreset:      "pretendard",
		ChromePath:      chromePath,
		ChromeNoSandbox: false,
	}
	start := time.Now()
	_, err := renderHTML(cfg)
	if err == nil || !strings.Contains(err.Error(), "render deadline exceeded") {
		t.Fatalf("expected render deadline error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("render deadline was not honored quickly: %s", elapsed)
	}
}

func TestHTMLFreshnessFindingsFailClosedOnHashErrors(t *testing.T) {
	target := filepath.Join(t.TempDir(), "outside.html")
	if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "final_deck.html")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	renderFindings := renderManifestHTMLFreshnessFindings(link, strings.Repeat("0", 64))
	if !hasFindingCheck(renderFindings, "manifest.freshness") || !hasFindingCheck(renderFindings, "ED-RENDER-001") {
		t.Fatalf("render freshness should fail closed on hash errors, got %#v", renderFindings)
	}
	packageFindings := packageManifestHTMLFreshnessFindings(link, filepath.Join(t.TempDir(), "render_manifest.json"), strings.Repeat("0", 64))
	if !hasFindingCheck(packageFindings, "package.manifest_freshness") {
		t.Fatalf("package freshness should fail closed on hash errors, got %#v", packageFindings)
	}
	if missingHash := packageManifestHTMLFreshnessFindings(target, "manifest.json", ""); !hasFindingCheck(missingHash, "package.manifest_freshness") {
		t.Fatalf("package freshness should require source hash, got %#v", missingHash)
	}
}

func TestCaptureURLScreenshotRejectsSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "slide.png")
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	err := captureURLScreenshot("missing-chrome", "about:blank", target, 16, 9, false)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlink screenshot target rejection before Chrome launch, got %v", err)
	}
	if got := readFileOrEmpty(outside); got != "outside\n" {
		t.Fatalf("outside symlink target was modified: %q", got)
	}
}

func TestVerifyTextArtifactFreshnessRejectsSymlinkArtifact(t *testing.T) {
	reference := filepath.Join(t.TempDir(), "render_manifest.json")
	if err := os.WriteFile(reference, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "qa_report.md")
	if err := os.WriteFile(outside, []byte("report\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "qa_report.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	findings := verifyTextArtifactFreshness("qa_report", link, reference, nil)
	if !hasFindingCheck(findings, "package.qa_report_freshness") || !hasFindingCheck(findings, "symlink") {
		t.Fatalf("expected symlink text artifact freshness failure, got %#v", findings)
	}
}

func TestInspectDeckWarnsAndSkipsSymlinkEntries(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	assetDir := filepath.Join(deck, "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deck, "brief.md"), []byte("brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(assetDir, "link.txt")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	inv, err := inspectDeck(deck)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range append(inv.Inputs, inv.Outputs...) {
		if entry.Path == "assets/link.txt" {
			t.Fatalf("symlink entry should not be inventoried: %#v", inv)
		}
	}
	if len(inv.Warnings) == 0 || !strings.Contains(strings.Join(inv.Warnings, "\n"), "symlink") {
		t.Fatalf("symlink warning missing: %#v", inv.Warnings)
	}
}

func TestInspectDeckRejectsExcessiveWalkEntries(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(deck, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(deck, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := inspectDeckWithBudget(deck, inspectBudget{
		MaxWalkEntries:    2,
		MaxHashBytes:      maxInspectHashTotalBytes,
		MaxInventoryBytes: maxInspectInventoryBytes,
	})
	if err == nil || !strings.Contains(err.Error(), "entry limit exceeded") {
		t.Fatalf("expected entry limit error, got %v", err)
	}
}

func TestInspectDeckRejectsAggregateHashBudget(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(deck, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deck, "a.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deck, "b.txt"), []byte("67890"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := inspectDeckWithBudget(deck, inspectBudget{
		MaxWalkEntries:    10,
		MaxHashBytes:      6,
		MaxInventoryBytes: maxInspectInventoryBytes,
	})
	if err == nil || !strings.Contains(err.Error(), "inspect budget exceeded") {
		t.Fatalf("expected hash budget error, got %v", err)
	}
}

func TestInspectDeckRejectsOversizedInventoryOutput(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(deck, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deck, "brief.md"), []byte("brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := inspectDeckWithBudget(deck, inspectBudget{
		MaxWalkEntries:    10,
		MaxHashBytes:      maxInspectHashTotalBytes,
		MaxInventoryBytes: 64,
	})
	if err == nil || !strings.Contains(err.Error(), "inventory output exceeds") {
		t.Fatalf("expected inventory output limit error, got %v", err)
	}
}

func TestWriteSourceInventoryRejectsOversizedMarkdown(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	inv := inventory{
		ToolName: toolName,
		Version:  toolVersion,
		DeckDir:  filepath.Dir(outDir),
		OutDir:   outDir,
		Inputs: []fileEntry{{
			Path:   strings.Repeat("a", 128) + ".txt",
			Kind:   "file",
			Size:   1,
			SHA256: "abc123",
		}},
	}

	err := writeSourceInventoryWithBudget(inv, 64)
	if err == nil || !strings.Contains(err.Error(), "source inventory exceeds") {
		t.Fatalf("expected source inventory limit error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "source_inventory.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("source_inventory.md should not be written, stat err=%v", statErr)
	}
}

func TestFileURLFromPathForSupportedPlatforms(t *testing.T) {
	cases := []struct {
		goos string
		path string
		want string
	}{
		{goos: "linux", path: "/tmp/slidex deck/a b.html", want: "file:///tmp/slidex%20deck/a%20b.html"},
		{goos: "darwin", path: "/private/var/folders/slidex deck/a b.html", want: "file:///private/var/folders/slidex%20deck/a%20b.html"},
		{goos: "windows", path: `C:\Users\Me\slidex deck\a b.html`, want: "file:///C:/Users/Me/slidex%20deck/a%20b.html"},
		{goos: "windows", path: `\\server\share\slidex deck\a b.html`, want: "file://server/share/slidex%20deck/a%20b.html"},
	}
	for _, tc := range cases {
		if got := fileURLFromPathForOS(tc.goos, tc.path); got != tc.want {
			t.Fatalf("%s file URL = %q, want %q", tc.goos, got, tc.want)
		}
	}
}

func TestInjectDocumentBaseUsesSourceDirectory(t *testing.T) {
	htmlPath := filepath.Join(t.TempDir(), "out", "final_deck.html")
	baseHref := documentBaseHrefForHTMLPath(htmlPath)
	src := `<html><head><title>Deck</title></head><body><img src="assets/hero.png"></body></html>`
	got := injectDocumentBase(src, baseHref)
	requireCanonicalBaseFirstInParsedHead(t, got, baseHref)
	again := injectDocumentBase(got, baseHref)
	requireCanonicalBaseFirstInParsedHead(t, again, baseHref)
}

func TestInjectHeadBaseKeepsSingleCanonicalBase(t *testing.T) {
	head := `<base href="file:///deck/out/"><title>Deck</title>`
	got := injectHeadBase(head, "file:///deck/out/")
	if strings.Count(got, `<base href="file:///deck/out/">`) != 1 || !strings.HasPrefix(got, `<base href="file:///deck/out/">`) {
		t.Fatalf("head should contain one canonical leading base, got %q", got)
	}
}

func TestInjectDocumentBaseReplacesOnlyActualUnacceptableBase(t *testing.T) {
	baseHref := "file:///deck/out/"
	src := `<!doctype html><html><head><!-- <base href="file:///bad/"> --><base href="./"><title>Deck</title></head><body></body></html>`
	got := injectDocumentBase(src, baseHref)
	requireCanonicalBaseFirstInParsedHead(t, got, baseHref)
	if strings.Contains(got, `<base href="./">`) {
		t.Fatalf("relative base should be removed:\n%s", got)
	}
	if !strings.Contains(got, `<!-- <base href="file:///bad/"> -->`) {
		t.Fatalf("commented base should be preserved as inert text:\n%s", got)
	}
}

func TestInjectDocumentBaseNormalizesMixedBaseTags(t *testing.T) {
	baseHref := "file:///deck/out/"
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "bad_before_expected",
			src:  `<!doctype html><html><head><base href="https://evil.example/"><base href="file:///deck/out/"><title>Deck</title></head><body></body></html>`,
		},
		{
			name: "expected_before_relative",
			src:  `<!doctype html><html><head><base href="file:///deck/out/"><base href="../relative/"><title>Deck</title></head><body></body></html>`,
		},
		{
			name: "duplicate_expected",
			src:  `<!doctype html><html><head><base href="file:///deck/out/"><base href="file:///deck/out/"><title>Deck</title></head><body></body></html>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := injectDocumentBase(tc.src, baseHref)
			requireCanonicalBaseFirstInParsedHead(t, got, baseHref)
			for _, forbidden := range []string{`https://evil.example/`, `../relative/`} {
				if strings.Contains(got, forbidden) {
					t.Fatalf("unacceptable base survived:\n%s", got)
				}
			}
		})
	}
}

func TestInjectHeadBaseReplacesRelativeBase(t *testing.T) {
	got := injectHeadBase(`<base href="./"><title>Deck</title>`, "file:///deck/out/")
	if !strings.HasPrefix(got, `<base href="file:///deck/out/">`) {
		t.Fatalf("head base should start with source base, got %q", got)
	}
	if strings.Contains(got, `<base href="./">`) {
		t.Fatalf("relative base should be removed, got %q", got)
	}
}

func TestInjectHeadBaseMovesLateCanonicalBaseBeforeURLContent(t *testing.T) {
	got := injectHeadBase(`<link rel="stylesheet" href="deck.css"><base href="file:///deck/out/"><title>Deck</title>`, "file:///deck/out/")
	if !strings.HasPrefix(got, `<base href="file:///deck/out/">`) {
		t.Fatalf("canonical base should be moved before URL-bearing content, got %q", got)
	}
	if strings.Count(got, `<base href="file:///deck/out/">`) != 1 {
		t.Fatalf("head should contain exactly one canonical base, got %q", got)
	}
	if strings.Index(got, `<base href="file:///deck/out/">`) > strings.Index(got, `<link rel="stylesheet"`) {
		t.Fatalf("base should precede stylesheet, got %q", got)
	}
}

func TestInjectDocumentBaseMovesLateCanonicalBaseBeforeURLContent(t *testing.T) {
	baseHref := "file:///deck/out/"
	src := `<!doctype html><html><head><link rel="stylesheet" href="deck.css"><base href="file:///deck/out/"><title>Deck</title></head><body></body></html>`
	got := injectDocumentBase(src, baseHref)
	requireCanonicalBaseFirstInParsedHead(t, got, baseHref)
}

func TestInjectDocumentBaseNormalizesParserRecognizedPreHeadBase(t *testing.T) {
	baseHref := "file:///deck/out/"
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "before_html",
			src:  `<!doctype html><base href="https://evil.example/"><html><head><link rel="stylesheet" href="deck.css"></head><body></body></html>`,
		},
		{
			name: "between_html_and_head",
			src:  `<!doctype html><html><base href="https://evil.example/"><head><link rel="stylesheet" href="deck.css"></head><body></body></html>`,
		},
		{
			name: "implicit_head",
			src:  `<!doctype html><html><base href="https://evil.example/"><link rel="stylesheet" href="deck.css"><body></body></html>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := injectDocumentBase(tc.src, baseHref)
			requireCanonicalBaseFirstInParsedHead(t, got, baseHref)
			if strings.Contains(got, "https://evil.example/") {
				t.Fatalf("parser-recognized attacker base survived:\n%s", got)
			}
		})
	}
}

func TestInjectDocumentBaseIgnoresCommentedHeadBoundary(t *testing.T) {
	baseHref := "file:///deck/out/"
	src := `<!-- <head> --><html><head><link rel="stylesheet" href="deck.css"></head><body></body></html>`
	got := injectDocumentBase(src, baseHref)
	if !strings.Contains(got, `<!-- <head> -->`) {
		t.Fatalf("commented head marker should remain inert text:\n%s", got)
	}
	requireCanonicalBaseFirstInParsedHead(t, got, baseHref)
}

func TestRenderDocumentHeadWithBaseIgnoresCommentedFakeHead(t *testing.T) {
	baseHref := "file:///deck/out/"
	src := `<!-- <head><style id="fake">body{background:red}</style></head> --><html><head><style id="real">body{background:green}</style></head><body></body></html>`
	got := renderDocumentHeadWithBase(src, baseHref)
	requireCanonicalBaseFirstInParsedHead(t, wrapHeadFragmentForTest(got), baseHref)
	if countHeadElementsByIDForTest(t, got, "style", "fake") != 0 {
		t.Fatalf("commented fake head style became active:\n%s", got)
	}
	if countHeadElementsByIDForTest(t, got, "style", "real") != 1 {
		t.Fatalf("real head style missing:\n%s", got)
	}
}

func TestRenderDocumentHeadWithBaseKeepsScriptFakeHeadInert(t *testing.T) {
	baseHref := "file:///deck/out/"
	src := `<script>const fake = "<head><style id='fake'>body{background:red}</style></head>";</script><html><head><style id="real">body{background:green}</style></head><body></body></html>`
	got := renderDocumentHeadWithBase(src, baseHref)
	requireCanonicalBaseFirstInParsedHead(t, wrapHeadFragmentForTest(got), baseHref)
	if countHeadElementsByIDForTest(t, got, "style", "fake") != 0 {
		t.Fatalf("script-contained fake head style became active:\n%s", got)
	}
	if countHeadElementsByIDForTest(t, got, "style", "real") != 1 {
		t.Fatalf("real head style missing:\n%s", got)
	}
}

func TestRenderDocumentHeadWithBasePreservesImplicitHeadContent(t *testing.T) {
	baseHref := "file:///deck/out/"
	src := `<!doctype html><html><base href="https://evil.example/"><link rel="stylesheet" href="deck.css"><body></body></html>`
	got := renderDocumentHeadWithBase(src, baseHref)
	requireCanonicalBaseFirstInParsedHead(t, wrapHeadFragmentForTest(got), baseHref)
	if strings.Contains(got, "https://evil.example/") {
		t.Fatalf("untrusted base survived:\n%s", got)
	}
	if countHeadElementsByIDForTest(t, got, "link", "") != 1 || !strings.Contains(got, `href="deck.css"`) {
		t.Fatalf("implicit head stylesheet missing:\n%s", got)
	}
}

func requireCanonicalBaseFirstInParsedHead(t *testing.T, src, baseHref string) {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	hrefs := collectActualBaseHrefsForTest(doc)
	if len(hrefs) != 1 || hrefs[0] != baseHref {
		t.Fatalf("expected exactly one actual canonical base, got %#v in:\n%s", hrefs, src)
	}
	head := findFirstElementForTest(doc, "head")
	if head == nil {
		t.Fatalf("document head missing:\n%s", src)
	}
	for child := head.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != xhtml.ElementNode {
			continue
		}
		if !strings.EqualFold(child.Data, "base") {
			t.Fatalf("first head element should be canonical base, got <%s> in:\n%s", child.Data, src)
		}
		if href := attrValueForTest(child, "href"); href != baseHref {
			t.Fatalf("first head base href = %q, want %q", href, baseHref)
		}
		return
	}
	t.Fatalf("head contains no element children:\n%s", src)
}

func collectActualBaseHrefsForTest(node *xhtml.Node) []string {
	var hrefs []string
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && strings.EqualFold(n.Data, "base") {
			hrefs = append(hrefs, attrValueForTest(n, "href"))
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return hrefs
}

func findFirstElementForTest(node *xhtml.Node, name string) *xhtml.Node {
	if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstElementForTest(child, name); found != nil {
			return found
		}
	}
	return nil
}

func attrValueForTest(node *xhtml.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func wrapHeadFragmentForTest(head string) string {
	return `<!doctype html><html><head>` + head + `</head><body></body></html>`
}

func countHeadElementsByIDForTest(t *testing.T, head, tag, id string) int {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(wrapHeadFragmentForTest(head)))
	if err != nil {
		t.Fatal(err)
	}
	headNode := findFirstElementForTest(doc, "head")
	if headNode == nil {
		t.Fatalf("head missing after parsing fragment:\n%s", head)
	}
	count := 0
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && strings.EqualFold(n.Data, tag) {
			if id == "" || attrValueForTest(n, "id") == id {
				count++
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for child := headNode.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	return count
}

func TestCollectDependenciesDecodesLocalURLs(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "final_deck.html")
	stylesheet := filepath.Join(dir, "styles", "style one.css")
	asset := filepath.Join(dir, "assets", "hero image.png")
	fileAsset := filepath.Join(dir, "absolute image.png")
	font := filepath.Join(dir, "fonts", "brand font.woff2")
	inlineBG := filepath.Join(dir, "assets", "inline bg.png")
	attrBG := filepath.Join(dir, "assets", "attr bg.png")
	linkedBG := filepath.Join(dir, "styles", "images", "linked bg.png")
	for _, item := range []struct {
		path string
		body string
	}{
		{stylesheet, `body{background-image:url("images/linked%20bg.png?rev=1")}` + "\n"},
		{asset, "asset\n"},
		{fileAsset, "file asset\n"},
		{font, "font\n"},
		{inlineBG, "inline bg\n"},
		{attrBG, "attr bg\n"},
		{linkedBG, "linked bg\n"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(item.path, []byte(item.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := fmt.Sprintf(`<!doctype html>
<link rel="stylesheet" href="styles/style%%20one.css?v=1">
<img src="assets/hero%%20image.png#view">
<img src="%s">
<style>
.hero { background-image: url("assets/inline%%20bg.png"); }
@font-face { font-family: Brand; src: url("fonts/brand%%20font.woff2?x=1"); }
</style>
<section style="background-image: url('assets/attr%%20bg.png#shape')"></section>`, fileURLFromPath(fileAsset))
	styles, assets, fonts := collectDependencies(htmlPath, src, "pretendard")
	for _, path := range []string{stylesheet, asset, fileAsset, font, inlineBG, attrBG, linkedBG} {
		dep, ok := findDependencyByPath(append(append(styles, assets...), fonts...), path)
		if !ok {
			t.Fatalf("dependency path %q not found\nstyles=%#v\nassets=%#v\nfonts=%#v", path, styles, assets, fonts)
		}
		if dep.SHA256 == "" || dep.Risk != "" {
			t.Fatalf("dependency %q did not resolve cleanly: %#v", path, dep)
		}
	}
}

func TestCollectDependenciesRecordsImportedCSSAndSrcsets(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "out", "final_deck.html")
	stylesDir := filepath.Join(dir, "styles")
	assetsDir := filepath.Join(dir, "assets")
	for _, path := range []string{stylesDir, assetsDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	baseCSS := filepath.Join(stylesDir, "base.css")
	themeCSS := filepath.Join(stylesDir, "theme.css")
	bg := filepath.Join(assetsDir, "bg.png")
	hero2x := filepath.Join(assetsDir, "hero-2x.png")
	preload := filepath.Join(assetsDir, "preload.png")
	preload2x := filepath.Join(assetsDir, "preload-2x.png")
	files := map[string]string{
		baseCSS:   `@import "theme.css";` + "\n",
		themeCSS:  `.slide { background-image: url("../assets/bg.png"); }` + "\n",
		bg:        "bg\n",
		hero2x:    "hero\n",
		preload:   "preload\n",
		preload2x: "preload 2x\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := `<!doctype html>
<link rel="stylesheet" href="../styles/base.css">
<img srcset="../assets/hero-2x.png 2x">
<link rel="preload" as="image" href="../assets/preload.png" imagesrcset="../assets/preload-2x.png 2x">`
	styles, assets, _ := collectDependencies(htmlPath, src, "pretendard")
	for _, path := range []string{baseCSS, themeCSS} {
		dep, ok := findDependencyByPath(styles, path)
		if !ok {
			t.Fatalf("stylesheet dependency %q not found\nstyles=%#v", path, styles)
		}
		if dep.Kind != "stylesheet" || dep.SHA256 == "" || dep.Risk != "" {
			t.Fatalf("stylesheet dependency %q misclassified: %#v", path, dep)
		}
	}
	for _, path := range []string{bg, hero2x, preload, preload2x} {
		dep, ok := findDependencyByPath(assets, path)
		if !ok {
			t.Fatalf("asset dependency %q not found\nassets=%#v", path, assets)
		}
		if dep.Kind != "asset" || dep.SHA256 == "" || dep.Risk != "" {
			t.Fatalf("asset dependency %q misclassified: %#v", path, dep)
		}
	}
	if dep, ok := findDependencyByPath(styles, preload); ok {
		t.Fatalf("image preload should not be classified as stylesheet: %#v", dep)
	}
}

func TestCollectDependenciesRejectsCSSImportFileBudget(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	stylesDir := filepath.Join(deck, "styles")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		next := ""
		if i < 2 {
			next = fmt.Sprintf(`@import "%05d.css";`, i+1)
		}
		if err := os.WriteFile(filepath.Join(stylesDir, fmt.Sprintf("%05d.css", i)), []byte(next+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := `<!doctype html><link rel="stylesheet" href="../styles/00000.css">`
	_, _, _, err := collectDependenciesWithBudget(htmlPath, src, "pretendard", renderResourcePreflightBudget{
		MaxCSSFiles:    2,
		MaxCSSBytes:    1 << 20,
		MaxResourceRef: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "maximum local stylesheet files") {
		t.Fatalf("expected dependency collection CSS file budget rejection, got %v", err)
	}
}

func TestCollectDependenciesRejectsResourceRefBudget(t *testing.T) {
	htmlPath := filepath.Join(t.TempDir(), "final_deck.html")
	src := `<!doctype html><style>
.a{background:url("a.png")}
.b{background:url("b.png")}
.c{background:url("c.png")}
</style>`
	_, _, _, err := collectDependenciesWithBudget(htmlPath, src, "pretendard", renderResourcePreflightBudget{
		MaxCSSFiles:    10,
		MaxCSSBytes:    1 << 20,
		MaxResourceRef: 2,
	})
	if err == nil || !strings.Contains(err.Error(), "maximum resource references") {
		t.Fatalf("expected dependency collection resource reference budget rejection, got %v", err)
	}
}

func TestVerifyManifestDependenciesDetectsImportedCSSMutation(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "out", "final_deck.html")
	stylesDir := filepath.Join(dir, "styles")
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseCSS := filepath.Join(stylesDir, "base.css")
	themeCSS := filepath.Join(stylesDir, "theme.css")
	if err := os.WriteFile(baseCSS, []byte(`@import "theme.css";`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(themeCSS, []byte(`.slide { color: red; }`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `<!doctype html><link rel="stylesheet" href="../styles/base.css">`
	styles, _, _ := collectDependencies(htmlPath, src, "pretendard")
	if _, ok := findDependencyByPath(styles, themeCSS); !ok {
		t.Fatalf("imported stylesheet dependency was not recorded: %#v", styles)
	}
	if err := os.WriteFile(themeCSS, []byte(`.slide { color: blue; }`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	currentStyles, _, _ := collectDependencies(htmlPath, src, "pretendard")
	findings := verifyManifestDependencies("styles", styles, currentStyles, filepath.Join(dir, "out", "render_manifest.json"))
	if len(findings) == 0 {
		t.Fatalf("expected stale imported stylesheet dependency finding")
	}
}

func TestCollectDependenciesDoesNotScanSymlinkedStylesheet(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "final_deck.html")
	stylesDir := filepath.Join(dir, "styles")
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	targetCSS := filepath.Join(targetDir, "target.css")
	if err := os.WriteFile(targetCSS, []byte(`body{background:url("../secret.png")}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stylesheet := filepath.Join(stylesDir, "style.css")
	if err := os.Symlink(targetCSS, stylesheet); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	src := `<!doctype html><link rel="stylesheet" href="styles/style.css">`
	styles, assets, _ := collectDependencies(htmlPath, src, "pretendard")
	dep, ok := findDependencyByPath(styles, stylesheet)
	if !ok {
		t.Fatalf("stylesheet dependency was not recorded: %#v", styles)
	}
	if dep.SHA256 != "" || !strings.Contains(dep.Risk, "symlink") {
		t.Fatalf("symlinked stylesheet should be risky and unhashed: %#v", dep)
	}
	if len(assets) != 0 {
		t.Fatalf("nested URL from symlinked stylesheet should not be collected: %#v", assets)
	}
}

func TestPrepareRenderedSlidesDirRejectsSymlinkBeforeCleanup(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	externalDir := t.TempDir()
	externalSlide := filepath.Join(externalDir, "slide_01.png")
	if err := os.WriteFile(externalSlide, []byte("external slide"), 0o644); err != nil {
		t.Fatal(err)
	}
	renderedDir := filepath.Join(outDir, "rendered_slides")
	if err := os.Symlink(externalDir, renderedDir); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	err := prepareRenderedSlidesDir(renderedDir)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected rendered_slides symlink rejection, got %v", err)
	}
	if got := readFileOrEmpty(externalSlide); got != "external slide" {
		t.Fatalf("external slide was modified or deleted: %q", got)
	}
}

func TestCollectDependenciesFlagsExternalLocalFiles(t *testing.T) {
	dir := t.TempDir()
	deck := filepath.Join(dir, "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	external := filepath.Join(dir, "external.png")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, []byte("asset\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := fmt.Sprintf(`<!doctype html><img src="%s">`, fileURLFromPath(external))
	_, assets, _ := collectDependencies(htmlPath, src, "pretendard")
	dep, ok := findDependencyByPath(assets, external)
	if !ok {
		t.Fatalf("external dependency not collected: %#v", assets)
	}
	if dep.SHA256 != "" {
		t.Fatalf("external dependency should not be hashed: %#v", dep)
	}
	if !strings.Contains(dep.Risk, "outside the deck workspace") {
		t.Fatalf("external dependency should record portability risk: %#v", dep)
	}
}

func TestFillDependencyRejectsUnsupportedURLSchemes(t *testing.T) {
	dep := dependency{Kind: "asset"}
	fillDependency(&dep, t.TempDir(), "ftp://example.com/asset.png")
	if dep.URL != "ftp://example.com/asset.png" || !strings.Contains(dep.Risk, "unsupported") {
		t.Fatalf("unsupported URL scheme should be recorded as risk, got %#v", dep)
	}
}

func TestCollectDependenciesFlagsProtocolRelativeURLs(t *testing.T) {
	htmlPath := filepath.Join(t.TempDir(), "final_deck.html")
	src := `<!doctype html>
<link rel="stylesheet" href="//cdn.example.com/lib.css">
<style>.hero{background:url(//cdn.example.com/font.woff2)}</style>`
	styles, assets, _ := collectDependencies(htmlPath, src, "pretendard")
	for _, tc := range []struct {
		name string
		deps []dependency
		url  string
	}{
		{name: "stylesheet", deps: styles, url: "//cdn.example.com/lib.css"},
		{name: "css_url", deps: assets, url: "//cdn.example.com/font.woff2"},
	} {
		dep, ok := findDependencyByURL(tc.deps, tc.url)
		if !ok {
			t.Fatalf("%s dependency URL %q not found\nstyles=%#v\nassets=%#v", tc.name, tc.url, styles, assets)
		}
		if dep.Path != "" || dep.Version != "" || !strings.Contains(dep.Risk, "protocol-relative") {
			t.Fatalf("%s protocol-relative dependency misclassified: %#v", tc.name, dep)
		}
	}
	findings := dependencyPinFindings("test.protocol_relative", append(styles, assets...), htmlPath)
	if len(findings) == 0 {
		t.Fatalf("protocol-relative dependencies should fail pinning checks")
	}
}

func TestRenderResourcePreflightRejectsRemoteFetches(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `<!doctype html><html><head>
<link rel="stylesheet" href="http://127.0.0.1:8765/probe.css">
<style>
@import url("//cdn.example.com/import.css");
.hero{background-image:url("https://cdn.example.com/hero.png")}
</style>
</head><body>
<section class="slide">
  <img srcset="local.png 1x, http://127.0.0.1:8765/beacon.png 2x">
</section>
</body></html>`

	err := renderResourceRequestPreflight(htmlPath, src)
	if err == nil {
		t.Fatal("remote render resources should fail preflight")
	}
	msg := err.Error()
	for _, want := range []string{"http://127.0.0.1:8765/probe.css", "//cdn.example.com/import.css", "https://cdn.example.com/hero.png", "http://127.0.0.1:8765/beacon.png"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("preflight error missing %q:\n%s", want, msg)
		}
	}
}

func TestRenderResourcePreflightRejectsCSSEscapedRemoteFetches(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	assets := filepath.Join(deck, "assets")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "styles.css"), []byte(`
@import "https\3a //127.0.0.1:8765/import.css";
.linked{background-image:url(https\3a //127.0.0.1:8765/linked.png)}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `<!doctype html><html><head>
<link rel="stylesheet" href="../assets/styles.css">
<style>.slide{background-image:url(https\3a //127.0.0.1:8765/inline.png)}</style>
</head><body>
<section class="slide" style='background-image:url("https\3a //127.0.0.1:8765/attr.png")'>OK</section>
</body></html>`

	err := renderResourceRequestPreflight(htmlPath, src)
	if err == nil {
		t.Fatal("CSS-escaped remote render resources should fail preflight")
	}
	msg := err.Error()
	for _, want := range []string{
		"https://127.0.0.1:8765/import.css",
		"https://127.0.0.1:8765/linked.png",
		"https://127.0.0.1:8765/inline.png",
		"https://127.0.0.1:8765/attr.png",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("preflight error missing decoded CSS URL %q:\n%s", want, msg)
		}
	}
}

func TestRenderResourcePreflightRejectsEscapedCSSTokens(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	src := `<!doctype html><html><head><style>
.slide { background-image: \75\72\6c("http://127.0.0.1:8765/escaped-function.png"); }
@\69mport "http://127.0.0.1:8765/escaped-import.css";
</style></head><body><section class="slide">OK</section></body></html>`

	err := renderResourceRequestPreflight(htmlPath, src)
	if err == nil {
		t.Fatal("CSS-escaped url/import tokens should fail preflight")
	}
	msg := err.Error()
	for _, want := range []string{"escaped-function.png", "escaped-import.css"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("preflight error missing escaped CSS token URL %q:\n%s", want, msg)
		}
	}
}

func TestRenderResourcePreflightRejectsCSSImageSetStringURLs(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	src := `<!doctype html><html><head><style>
.slide {
  background-image: image-set("http://127.0.0.1:8765/beacon.png" 1x);
  border-image-source: -webkit-\69 mage-set("https\3a //127.0.0.1:8765/escaped-webkit.png" 1x);
}
</style></head><body><section class="slide">OK</section></body></html>`

	err := renderResourceRequestPreflight(htmlPath, src)
	if err == nil {
		t.Fatal("CSS image-set string URLs should fail preflight")
	}
	msg := err.Error()
	for _, want := range []string{"http://127.0.0.1:8765/beacon.png", "https://127.0.0.1:8765/escaped-webkit.png"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("preflight error missing image-set string URL %q:\n%s", want, msg)
		}
	}
}

func TestRenderResourcePreflightRejectsDataCSSNestedFetches(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	src := `<!doctype html><html><head>
<link rel="stylesheet" href='data:text/css,%40import%20url(%22http%3A%2F%2F127.0.0.1%3A8765%2Fprobe.css%22)%3B'>
<style>@import url("data:text/css,%40import%20url(%22http%3A%2F%2F127.0.0.1%3A8765%2Finline.css%22)%3B");</style>
</head><body><section class="slide">OK</section></body></html>`

	err := renderResourceRequestPreflight(htmlPath, src)
	if err == nil {
		t.Fatal("data:text/css stylesheets with nested fetches should fail preflight")
	}
	msg := err.Error()
	for _, want := range []string{"data:text/css", "data stylesheet"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("preflight error missing data stylesheet evidence %q:\n%s", want, msg)
		}
	}
}

func TestRenderResourcePreflightRejectsLinkImagePreloadSrcsetRemote(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	assets := filepath.Join(deck, "assets")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "fallback.png"), []byte("fallback"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `<!doctype html><html><head>
<link rel="preload" as="image" href="../assets/fallback.png" imagesrcset="http://127.0.0.1:8765/beacon.png 1x">
</head><body><section class="slide">OK</section></body></html>`

	err := renderResourceRequestPreflight(htmlPath, src)
	if err == nil || !strings.Contains(err.Error(), "http://127.0.0.1:8765/beacon.png") {
		t.Fatalf("remote image preload imagesrcset should fail preflight, got %v", err)
	}
}

func TestRenderResourcePreflightRejectsCSSImportFileBudget(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	styles := filepath.Join(deck, "styles")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(styles, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		next := ""
		if i < 2 {
			next = fmt.Sprintf(`@import "%05d.css";`, i+1)
		}
		if err := os.WriteFile(filepath.Join(styles, fmt.Sprintf("%05d.css", i)), []byte(next+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := `<!doctype html><link rel="stylesheet" href="../styles/00000.css"><section class="slide">OK</section>`
	err := renderResourceRequestPreflightWithBudget(htmlPath, src, renderResourcePreflightBudget{
		MaxCSSFiles:    2,
		MaxCSSBytes:    1 << 20,
		MaxResourceRef: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "maximum local stylesheet files") {
		t.Fatalf("expected CSS file budget rejection, got %v", err)
	}
}

func TestRenderResourcePreflightRejectsCSSImportByteBudget(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	styles := filepath.Join(deck, "styles")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(styles, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(styles, "base.css"), []byte(`@import "theme.css";`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(styles, "theme.css"), []byte(strings.Repeat("a", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `<!doctype html><link rel="stylesheet" href="../styles/base.css"><section class="slide">OK</section>`
	err := renderResourceRequestPreflightWithBudget(htmlPath, src, renderResourcePreflightBudget{
		MaxCSSFiles:    10,
		MaxCSSBytes:    16,
		MaxResourceRef: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "maximum local stylesheet bytes") {
		t.Fatalf("expected CSS byte budget rejection, got %v", err)
	}
}

func TestRenderResourcePreflightAllowsLargeImagePreload(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	assets := filepath.Join(deck, "assets")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "hero.jpg"), bytes.Repeat([]byte{0x7f}, maxDependencyScanBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `<!doctype html><html><head>
<link rel="preload" as="image" href="../assets/hero.jpg">
</head><body><section class="slide"><img src="../assets/hero.jpg"></section></body></html>`

	if err := renderResourceRequestPreflight(htmlPath, src); err != nil {
		t.Fatalf("large image preload should not be scanned as CSS: %v", err)
	}
}

func TestRenderResourcePreflightRejectsExternalLocalFiles(t *testing.T) {
	root := t.TempDir()
	deck := filepath.Join(root, "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	external := filepath.Join(root, "external.png")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := fmt.Sprintf(`<!doctype html><section class="slide"><img src="%s"></section>`, fileURLFromPath(external))
	err := renderResourceRequestPreflight(htmlPath, src)
	if err == nil || !strings.Contains(err.Error(), "outside the deck workspace") {
		t.Fatalf("external local file should fail render preflight, got %v", err)
	}
}

func TestRenderHTMLRejectsRemoteResourcesBeforeChrome(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	htmlPath := filepath.Join(deck, "out", "final_deck.html")
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(`<!doctype html><section class="slide"><img src="http://127.0.0.1:8765/beacon.png"></section>`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := renderHTML(renderConfig{HTMLPath: htmlPath, Selector: ".slide"})
	if err == nil || !strings.Contains(err.Error(), "render resource preflight") {
		t.Fatalf("renderHTML should fail on unsafe resources before resolving Chrome, got %v", err)
	}
}

func findDependencyByPath(deps []dependency, path string) (dependency, bool) {
	for _, dep := range deps {
		if dep.Path == path {
			return dep, true
		}
	}
	return dependency{}, false
}

func findDependencyByURL(deps []dependency, url string) (dependency, bool) {
	for _, dep := range deps {
		if dep.URL == url {
			return dep, true
		}
	}
	return dependency{}, false
}

func TestRenderManifestPortablePathsRoundTrip(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	renderedDir := filepath.Join(outDir, "rendered_slides")
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(outDir, "final_deck.html")
	pngPath := filepath.Join(renderedDir, "slide_01.png")
	pdfPath := filepath.Join(outDir, "final_deck.pdf")
	montagePath := filepath.Join(outDir, "qa_montage.png")
	for _, path := range []string{htmlPath, pngPath, pdfPath, montagePath} {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(outDir, "render_manifest.json")
	base := renderManifestBaseDir(manifestPath)
	manifest := renderManifest{
		RepoRelativePaths: true,
		SourceHTML:        artifactFromPathRelative(htmlPath, base),
		PNGFiles: []renderedImage{{
			SlideID: "slide_01",
			Path:    portableManifestPath(base, pngPath),
			SHA256:  mustSHA256(pngPath),
		}},
		PDF:       artifactFromPathRelative(pdfPath, base),
		QAMontage: artifactFromPathRelative(montagePath, base),
	}
	if manifest.SourceHTML.Path != "out/final_deck.html" ||
		manifest.PNGFiles[0].Path != "out/rendered_slides/slide_01.png" ||
		manifest.PDF.Path != "out/final_deck.pdf" ||
		manifest.QAMontage.Path != "out/qa_montage.png" {
		t.Fatalf("manifest paths should be deck-relative slash paths: %#v", manifest)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeRenderManifest(raw, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SourceHTML.Path != htmlPath ||
		decoded.PNGFiles[0].Path != pngPath ||
		decoded.PDF.Path != pdfPath ||
		decoded.QAMontage.Path != montagePath {
		t.Fatalf("decoded manifest paths were not restored to native paths: %#v", decoded)
	}
}

func TestRenderWrapperFilenameDoesNotUseRawSlideID(t *testing.T) {
	name := renderWrapperFilename(0, `../bad\slide:id?*`)
	if strings.ContainsAny(name, `/\:?*`) {
		t.Fatalf("wrapper filename contains unsafe characters: %q", name)
	}
	if !strings.HasPrefix(name, "slide_01_") || !strings.HasSuffix(name, ".html") {
		t.Fatalf("wrapper filename lost stable prefix/suffix: %q", name)
	}
}

func TestLoopbackWebSocketListenParsing(t *testing.T) {
	for _, listen := range []string{"ws://127.0.0.1:1234/app", "ws://[::1]:1234/app", "ws://localhost:1234/app", "wss://localhost:1234/app"} {
		if !isLoopbackWebSocketListen(listen) {
			t.Fatalf("expected loopback websocket listen to pass: %s", listen)
		}
		if risk := transportRiskForListen(listen); risk != "" {
			t.Fatalf("loopback listen should not record transport risk: %s => %q", listen, risk)
		}
	}
	for _, listen := range []string{"ws://10.0.0.2:1234/app", "ws://[2001:db8::1]:1234/app", "http://127.0.0.1:1234/app"} {
		if isLoopbackWebSocketListen(listen) {
			t.Fatalf("expected non-loopback websocket listen to fail: %s", listen)
		}
	}
}

func TestSystemSymlinkAncestorPolicyAllowsOnlyDarwinSystemLinks(t *testing.T) {
	if !systemSymlinkAncestorAllowed("darwin", "/var") || !systemSymlinkAncestorAllowed("darwin", "/tmp") {
		t.Fatal("darwin standard system symlink ancestors should be allowed")
	}
	if systemSymlinkAncestorAllowed("darwin", "/tmp/deck/out") {
		t.Fatal("darwin user-controlled descendants must not be broadly allowed")
	}
	if systemSymlinkAncestorAllowed("linux", "/var") || systemSymlinkAncestorAllowed("windows", `C:\Temp`) {
		t.Fatal("system symlink exception should be darwin-specific")
	}
}

func TestWebSocketHealthProbeUsesHTTPAndPing(t *testing.T) {
	listen := startFakeWebSocketServer(t,
		func(conn net.Conn) { handleFakeHTTPHealth(t, conn) },
		func(conn net.Conn) { handleFakeHTTPHealth(t, conn) },
		func(conn net.Conn) { handleFakeWebSocketPing(t, conn) },
	)
	health, err := probeManagedAppServer(map[string]any{"listen": listen, "websocketAuth": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if health["status"] != "pass" {
		t.Fatalf("websocket health status = %#v", health)
	}
	ping, _ := health["websocketPing"].(map[string]any)
	if ping["status"] != "pass" {
		t.Fatalf("websocket ping did not pass: %#v", health)
	}
}

func TestManagedListenURLRejectsUnsupportedUnixSocketPaths(t *testing.T) {
	if err := validateManagedListenURLForOS("windows", "unix:///tmp/slidex.sock"); err == nil {
		t.Fatal("windows should reject explicit unix socket listen URLs")
	}
	if err := validateManagedListenURLForOS("linux", "unix:///tmp/slidex.sock"); err != nil {
		t.Fatalf("linux should accept short unix socket paths: %v", err)
	}
	longSocket := "unix:///" + strings.Repeat("a", 140) + ".sock"
	if err := validateManagedListenURLForOS("linux", longSocket); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("linux should reject overlong unix socket path, got %v", err)
	}
	if err := validateManagedListenURLForOS("darwin", "unix://relative.sock"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("darwin should require absolute unix socket paths, got %v", err)
	}
	if err := validateManagedListenURLForOS("windows", "ws://127.0.0.1:49200/app"); err != nil {
		t.Fatalf("windows should accept loopback websocket listen URLs: %v", err)
	}
	if err := validateManagedListenURLForOS("linux", "wss://127.0.0.1:49200/app"); err != nil {
		t.Fatalf("linux should accept secure websocket listen URLs: %v", err)
	}
	if err := validateManagedListenURLForOS("linux", "http://127.0.0.1:49200/app"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("managed listen should reject unsupported schemes, got %v", err)
	}
	if err := validateManagedListenURLForOS("linux", "ws:///app"); err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("managed websocket listen should require a host, got %v", err)
	}
}

func TestUnixSocketListenURLDecodesPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket path decoding uses native absolute path rules")
	}
	u, err := url.Parse("unix:///tmp/slidex%20sock/app.sock")
	if err != nil {
		t.Fatal(err)
	}
	socketPath, err := unixSocketPathFromListenURL(u)
	if err != nil {
		t.Fatal(err)
	}
	if socketPath != "/tmp/slidex sock/app.sock" {
		t.Fatalf("socket path = %q", socketPath)
	}
}

func TestWebSocketSignedBearerAuthIsSentToHealthAndPing(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := webSocketAuthConfig{Mode: "signed-bearer-token", SharedSecretFile: secret, Issuer: "slidex", Audience: "codex", MaxClockSkewSeconds: 30}
	listen := startFakeWebSocketServer(t,
		func(conn net.Conn) { handleFakeHTTPHealthWithSignedAuth(t, conn) },
		func(conn net.Conn) { handleFakeHTTPHealthWithSignedAuth(t, conn) },
		func(conn net.Conn) { handleFakeWebSocketPingWithSignedAuth(t, conn) },
	)
	health := probeWebSocketAppServer(listen, auth)
	if health["status"] != "pass" {
		t.Fatalf("signed bearer websocket health should pass: %#v", health)
	}
}

func TestWebSocketCapabilityProbeRejectsTokenHashDrift(t *testing.T) {
	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: sha256Bytes([]byte("original"))}
	if err := os.WriteFile(token, []byte("rotated"), 0o600); err != nil {
		t.Fatal(err)
	}
	health := probeWebSocketHTTPHealth("ws://127.0.0.1:1/app", "/readyz", auth, time.Millisecond)
	if health["status"] != "fail" || !strings.Contains(fmt.Sprint(health["error"]), "sha256") {
		t.Fatalf("websocket HTTP probe should reject capability token hash drift, got %#v", health)
	}
	if err := webSocketPingOnce("ws://127.0.0.1:1/app", auth, time.Millisecond); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("websocket ping should reject capability token hash drift before dialing, got %v", err)
	}
}

func TestWebSocketHealthProbeDegradesOnHTTPFailure(t *testing.T) {
	listen := startFakeWebSocketServer(t,
		func(conn net.Conn) { handleFakeHTTPFailure(t, conn) },
		func(conn net.Conn) { handleFakeHTTPHealth(t, conn) },
		func(conn net.Conn) { handleFakeWebSocketPing(t, conn) },
	)
	health := probeWebSocketAppServer(listen, webSocketAuthConfig{})
	if health["status"] != "fail" {
		t.Fatalf("websocket health should fail on readyz failure: %#v", health)
	}
}

func TestManagedAppServerStatusFailsDeadProcess(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	path := appServerMetadataPath()
	if err := secureWriteJSON(path, map[string]any{"pid": 99999999, "listen": "ws://127.0.0.1:1/app"}); err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	err = statusManagedAppServer()
	_ = writer.Close()
	os.Stdout = oldStdout
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("status output is not JSON: %s", raw)
	}
	if payload["status"] != "fail" {
		t.Fatalf("dead managed process should fail status: %#v", payload)
	}
}

func TestWebSocketPingRetriesOverload(t *testing.T) {
	listen := startFakeWebSocketServer(t,
		func(conn net.Conn) { handleFakeWebSocketOverload(t, conn) },
		func(conn net.Conn) { handleFakeWebSocketPing(t, conn) },
	)
	ping := webSocketPingWithRetry(listen, webSocketAuthConfig{}, webSocketProbePolicy{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, MaxAttempts: 2, PingTimeout: time.Second})
	if ping["status"] != "pass" {
		t.Fatalf("retrying websocket ping should pass: %#v", ping)
	}
	if attempts, _ := numberAsInt(ping["attempts"]); attempts != 2 {
		t.Fatalf("websocket ping attempts = %d, want 2", attempts)
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

func TestRedactSecretsInAnyFailsClosedAndPreservesSafeTokenMetadata(t *testing.T) {
	tokenHash := strings.Repeat("a", 64)
	payload := map[string]any{
		"token":             "secret-value",
		"hashicorpToken":    "hashicorp-secret-value",
		"secretUsage":       "usage-secret-value",
		"passwordHashPlain": "password-secret-value",
		"tokenUsage":        map[string]any{"total": 123},
		"tokenSha256":       tokenHash,
		"tokenWindow":       200000,
		"tokenRedacted":     true,
		"message":           "CODEX_API_KEY=secret-token Authorization: Bearer raw-token",
	}
	raw, err := json.Marshal(redactSecretsInAny(payload))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{"secret-value", "hashicorp-secret-value", "usage-secret-value", "password-secret-value", "secret-token", "raw-token"} {
		if strings.Contains(text, secret) {
			t.Fatalf("redacted payload leaked %q: %s", secret, text)
		}
	}
	for _, expected := range []string{`"tokenUsage"`, `"total":123`, tokenHash, `"tokenWindow":200000`, `"tokenRedacted":true`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("redacted payload lost safe metadata %q: %s", expected, text)
		}
	}

	raw, err = json.Marshal(redactSecretsInAny(map[string]any{
		"token": "secret-value",
		"bad":   func() {},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-value") {
		t.Fatalf("failed redaction should not return original payload: %s", raw)
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
		assertSecureModeForTest(t, path, info, 0o600)
	}
	outInfo, err := os.Stat(outDir)
	if err != nil {
		t.Fatal(err)
	}
	assertSecureModeForTest(t, outDir, outInfo, 0o700)
	logText := readFileOrEmpty(filepath.Join(outDir, "run_log.jsonl"))
	if strings.Contains(logText, "secret-token") || strings.Contains(logText, "raw-token") {
		t.Fatalf("run log was not redacted: %s", logText)
	}
}

func assertSecureModeForTest(t *testing.T, path string, info os.FileInfo, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if err := requirePlatformPrivateFile(path, "test fixture"); err != nil {
			t.Fatalf("%s should be private on Windows: %v", path, err)
		}
		return
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), want)
	}
}

func TestSafeFilenameComponentIsWindowsPortable(t *testing.T) {
	cases := map[string]string{
		"qa":                   "qa",
		"review start":         "review_start",
		`../bad\stage:CON*?`:   "bad_stage_CON",
		"CON":                  "_CON",
		"aux.txt":              "_aux.txt",
		" LPT1. ":              "_LPT1",
		"한글":                   "stage",
		`name"with<bad>|chars`: "name_with_bad_chars",
	}
	for input, want := range cases {
		if got := safeFilenameComponent(input); got != want {
			t.Fatalf("safeFilenameComponent(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAppServerTurnArtifactsUseSafeStageFilename(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	outDir := filepath.Join(deck, "out")
	path, result, err := writeAppServerTurnResult(outDir, appServerTurnResult{
		Stage:    `../bad\stage:CON*?`,
		ThreadID: "thread-1",
		TurnID:   "turn-1",
		Events:   []map[string]any{{"event": "ok"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(outDir, "agent_runs")
	if !pathWithin(runDir, path) {
		t.Fatalf("turn path escaped agent_runs: %s", path)
	}
	if filepath.Base(path) != "bad_stage_CON_appserver_turn.json" {
		t.Fatalf("turn filename = %q", filepath.Base(path))
	}
	if result.EventLog == "" || !strings.HasSuffix(result.EventLog, "bad_stage_CON_appserver_events.jsonl") {
		t.Fatalf("event log should use safe stage filename: %#v", result)
	}
	if strings.ContainsAny(filepath.Base(path), `\/:*?"<>|`) {
		t.Fatalf("turn filename is not Windows-safe: %q", filepath.Base(path))
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
		"schemaVersion":  "slidex.reviewFindings.v1",
		"stage":          "delivery",
		"round":          1,
		"mode":           "structured_turn",
		"status":         "pass",
		"imageEvidence":  []map[string]any{},
		"artifactHashes": structuredReviewArtifactHashes(deck),
		"findings":       []map[string]any{},
	}
	if err := validatePayloadAgainstSchema(reviewPayload, filepath.Join("schemas", "app_review_findings.strict.schema.json")); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizedReviewImageEvidenceUsesRenderManifest(t *testing.T) {
	deck := t.TempDir()
	outDir := filepath.Join(deck, "out")
	imagePath := filepath.Join(outDir, "rendered_slides", "slide_01.png")
	specPath := filepath.Join(outDir, "deck_spec.json")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := renderManifest{
		PNGFiles: []renderedImage{{
			SlideID:    "slide_01",
			Path:       imagePath,
			SHA256:     strings.Repeat("a", 64),
			Dimensions: dimension{Width: 1920, Height: 1080},
			Blank:      false,
		}},
	}
	if err := secureWriteJSON(filepath.Join(outDir, "render_manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	evidence := normalizedReviewImageEvidence(deck)
	if len(evidence) != 1 {
		t.Fatalf("image evidence count = %d, want 1", len(evidence))
	}
	if evidence[0]["slideId"] != "slide_01" || evidence[0]["sha256"] != strings.Repeat("a", 64) || evidence[0]["fidelity"] != "original" {
		t.Fatalf("unexpected image evidence: %#v", evidence[0])
	}
	payload := map[string]any{
		"artifactHashes": map[string]any{"deckSpecSha256": "wrong"},
		"imageEvidence":  []map[string]any{{"slideId": "wrong"}},
	}
	attachStructuredReviewRuntimeEvidence(deck, payload)
	hashes, _ := payload["artifactHashes"].(map[string]any)
	if hashes["deckSpecSha256"] != mustSHA256(specPath) {
		t.Fatalf("runtime evidence did not refresh deckSpecSha256: %#v", hashes)
	}
	refreshed, _ := payload["imageEvidence"].([]map[string]any)
	if len(refreshed) != 1 || refreshed[0]["slideId"] != "slide_01" {
		t.Fatalf("runtime evidence did not refresh imageEvidence: %#v", payload["imageEvidence"])
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

func TestRunCodexExecStructuredRejectsSymlinkedSessionFile(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	runDir := filepath.Join(deck, "out", "agent_runs")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private-session.txt")
	if err := os.WriteFile(outside, []byte("PRIVATE_SESSION\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(runDir, "codex_exec_last_session.txt")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	_, _, err := runCodexExecStructured(deck, "resolve_workspace", "{}", filepath.Join("schemas", "app_stage_result.strict.schema.json"), true, "last", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlinked session file rejection, got %v", err)
	}
}

func TestRunCodexExecStructuredRejectsSymlinkedLastMessage(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	runDir := filepath.Join(deck, "out", "agent_runs")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-last.json")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lastMessage := filepath.Join(runDir, "resolve_workspace_codex_exec_fresh.last.json")
	if err := os.Symlink(outside, lastMessage); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	_, _, err := runCodexExecStructured(deck, "resolve_workspace", "{}", filepath.Join("schemas", "app_stage_result.strict.schema.json"), false, "", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlinked last-message rejection, got %v", err)
	}
}

func TestPrepareCodexOutputMessagePathRejectsHardlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-last.json")
	target := filepath.Join(dir, "last.json")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hardlinkOrSkip(t, outside, target)
	err := prepareCodexOutputMessagePath(target)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hardlink") {
		t.Fatalf("expected hardlinked last-message rejection, got %v", err)
	}
	if got := readFileOrEmpty(outside); got != "outside\n" {
		t.Fatalf("outside hardlinked file was modified: %q", got)
	}
}

func TestRunCodexExecVisualReviewRejectsSymlinkedLastMessage(t *testing.T) {
	deck := filepath.Join(t.TempDir(), "deck")
	reviewDir := filepath.Join(deck, "out", "visual_reviews")
	if err := os.MkdirAll(reviewDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-review.json")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(reviewDir, "codex_visual_review.last.json")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	_, err := runCodexExecVisualReview(deck, renderManifest{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("expected symlinked visual review last-message rejection, got %v", err)
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

func TestReviewStartNormalizedRejectsFailedOrEmptyNativeReview(t *testing.T) {
	cases := []struct {
		name       string
		completion map[string]any
		threadRead map[string]any
		want       string
	}{
		{
			name:       "failed completion",
			completion: map[string]any{"threadId": "review-thread", "turn": map[string]any{"id": "review-turn", "status": "failed", "error": "boom"}},
			threadRead: map[string]any{"thread": map[string]any{"turns": []any{map[string]any{"id": "review-turn", "items": []any{map[string]any{"type": "agentMessage", "phase": "final_answer", "text": "No blocker or major findings remain."}}}}}},
			want:       "did not complete successfully",
		},
		{
			name:       "empty final",
			completion: map[string]any{"threadId": "review-thread", "turn": map[string]any{"id": "review-turn", "status": "completed"}},
			threadRead: map[string]any{"thread": map[string]any{"turns": []any{}}},
			want:       "without a final agent message",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deck := t.TempDir()
			if err := os.MkdirAll(filepath.Join(deck, "out"), 0o700); err != nil {
				t.Fatal(err)
			}
			client := &appServerClient{stdin: &testWriteCloser{}, lines: make(chan map[string]any, 4)}
			client.lines <- map[string]any{"id": 1, "result": map[string]any{"reviewThreadId": "review-thread", "turn": map[string]any{"id": "review-turn"}}}
			client.lines <- map[string]any{"method": "turn/completed", "params": tc.completion}
			client.lines <- map[string]any{"id": 2, "result": tc.threadRead}
			appRun := &appServerWorkflowRun{client: client, deckAbs: deck, threadID: "main-thread"}
			_, err := writeReviewStartNormalized(deck, "delivery", 1, appRun)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			rawPath := filepath.Join(deck, "out", "agent_runs", "review_start_delivery_appserver_turn.json")
			if _, statErr := os.Stat(rawPath); statErr != nil {
				t.Fatalf("raw review/start artifact should be written before failure: %v", statErr)
			}
		})
	}
}

func TestReviewStartRiskSummaryIgnoresNegatedPhrases(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{text: "No blocker or major findings remain.", want: false},
		{text: "No blocker/major findings remain.", want: false},
		{text: "No blockers and no majors remain.", want: false},
		{text: "No major issues found.", want: false},
		{text: "No blocker, but one major issue remains.", want: true},
		{text: "Blocker: rendered PNG set is stale.", want: true},
	}
	for _, tc := range cases {
		if got := reviewStartMentionsBlockingRisk(tc.text); got != tc.want {
			t.Fatalf("reviewStartMentionsBlockingRisk(%q) = %v, want %v", tc.text, got, tc.want)
		}
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

type testWriteCloser struct {
	strings.Builder
}

func (w *testWriteCloser) Close() error {
	return nil
}

func runLogLine(label, stage, status string, ts time.Time) string {
	raw, _ := json.Marshal(map[string]any{
		"event":         "stage_completed",
		"runLabel":      label,
		"stage":         stage,
		"status":        status,
		"stopCondition": map[string]string{"pass": "pass", "fail": "blocked"}[status],
		"timestamp":     ts.Format(time.RFC3339),
	})
	return string(raw)
}

func writeAuthoringTurnForTest(deck, stage string, payload map[string]any) error {
	path := filepath.Join(deck, "out", "agent_runs", "authoring_"+stage+"_appserver_turn.json")
	return secureWriteJSON(path, map[string]any{
		"schemaVersion":    "slidex.appServerTurn.v1",
		"stage":            "authoring_" + stage,
		"structuredOutput": payload,
	})
}

func startFakeWebSocketServer(t *testing.T, handlers ...func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for _, handler := range handlers {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			handler(conn)
		}
	}()
	return "ws://" + ln.Addr().String() + "/app"
}

func readHTTPRequestForTest(t *testing.T, conn net.Conn) string {
	t.Helper()
	reader := bufio.NewReader(conn)
	var b strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(line)
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	return b.String()
}

func handleFakeHTTPHealth(t *testing.T, conn net.Conn) {
	defer conn.Close()
	_ = readHTTPRequestForTest(t, conn)
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
}

func handleFakeHTTPHealthWithSignedAuth(t *testing.T, conn net.Conn) {
	defer conn.Close()
	request := readHTTPRequestForTest(t, conn)
	requireSignedBearerAuth(t, request)
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
}

func handleFakeHTTPFailure(t *testing.T, conn net.Conn) {
	defer conn.Close()
	_ = readHTTPRequestForTest(t, conn)
	_, _ = io.WriteString(conn, "HTTP/1.1 500 Internal Server Error\r\nContent-Length: 5\r\n\r\nerror")
}

func handleFakeWebSocketOverload(t *testing.T, conn net.Conn) {
	defer conn.Close()
	_ = readHTTPRequestForTest(t, conn)
	_, _ = io.WriteString(conn, "HTTP/1.1 503 Service Unavailable\r\nContent-Length: 25\r\n\r\n{\"code\":-32001,\"msg\":\"x\"}")
}

func handleFakeWebSocketPing(t *testing.T, conn net.Conn) {
	defer conn.Close()
	request := readHTTPRequestForTest(t, conn)
	if !strings.Contains(strings.ToLower(request), "upgrade: websocket") {
		t.Fatalf("fake websocket expected upgrade request, got %s", request)
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	frame := make([]byte, 6)
	if _, err := io.ReadFull(conn, frame); err != nil {
		t.Fatal(err)
	}
	if frame[0]&0x0f != 0x09 {
		t.Fatalf("expected ping opcode, got %d", frame[0]&0x0f)
	}
	_, _ = conn.Write([]byte{0x8a, 0x00})
}

func handleFakeWebSocketPingWithSignedAuth(t *testing.T, conn net.Conn) {
	defer conn.Close()
	request := readHTTPRequestForTest(t, conn)
	requireSignedBearerAuth(t, request)
	if !strings.Contains(strings.ToLower(request), "upgrade: websocket") {
		t.Fatalf("fake websocket expected upgrade request, got %s", request)
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	frame := make([]byte, 6)
	if _, err := io.ReadFull(conn, frame); err != nil {
		t.Fatal(err)
	}
	if frame[0]&0x0f != 0x09 {
		t.Fatalf("expected ping opcode, got %d", frame[0]&0x0f)
	}
	_, _ = conn.Write([]byte{0x8a, 0x00})
}

func requireSignedBearerAuth(t *testing.T, request string) {
	t.Helper()
	auth := requestHeaderValue(request, "authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Fatalf("expected bearer authorization header, got %q in %s", auth, request)
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		t.Fatalf("expected signed bearer token with three segments, got %q", token)
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("signed bearer payload is not base64url: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadRaw, &claims); err != nil {
		t.Fatalf("signed bearer payload is not JSON: %v", err)
	}
	if claims["iss"] != "slidex" || claims["aud"] != "codex" {
		t.Fatalf("unexpected signed bearer claims: %#v", claims)
	}
}

func requestHeaderValue(request, name string) string {
	name = strings.ToLower(name)
	for _, line := range strings.Split(request, "\n") {
		line = strings.TrimRight(line, "\r")
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(key)) == name {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
