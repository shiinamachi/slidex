package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	qaReportPath := filepath.Join(outDir, "qa_report.md")
	visualReviewPath := filepath.Join(outDir, "visual_reviews", "latest_review.json")
	originalSpec := readFileOrEmpty(specPath)
	originalQAReport := readFileOrEmpty(qaReportPath)

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
	if findings := verifyVisualReviewEvidence(reviewPath, manifest); len(findings) > 0 {
		t.Fatalf("visual review evidence should verify, got %#v", findings)
	}
	if !strings.Contains(readFileOrEmpty(reviewPath), "montage and PDF inspected") {
		t.Fatalf("manual notes were not recorded: %s", readFileOrEmpty(reviewPath))
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
	summary := summarizeJSONForEvidence(map[string]any{"token": "secret-value", "safe": "ok"})
	raw, _ := json.Marshal(summary)
	if strings.Contains(string(raw), "secret-value") {
		t.Fatalf("summary leaked secret: %s", raw)
	}
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

	command := appServerSkillSmokeWorkbenchCommand("/tmp/slidex workspace", "demo", "/repo/decks/_template")
	if !strings.Contains(command, "'/tmp/slidex workspace'") || !strings.Contains(command, "--deck-id demo") {
		t.Fatalf("command was not shell quoted as expected: %s", command)
	}
	windowsCommand := appServerSkillSmokeWorkbenchCommandForOS("windows", `C:\Users\Me\slidex workspace`, "demo", `C:\repo\decks\_template`)
	if !strings.Contains(windowsCommand, `--workspace "C:\Users\Me\slidex workspace"`) || !strings.Contains(windowsCommand, `--from-template C:\repo\decks\_template`) {
		t.Fatalf("windows command was not quoted as expected: %s", windowsCommand)
	}
	if got := windowsShellQuote(`C:\Users\Me\slidex workspace`); got != `"C:\Users\Me\slidex workspace"` {
		t.Fatalf("windows shell quote did not preserve spaced path: %s", got)
	}
	dangerousWindowsPath := `C:\Users\Me\slidex %workspace%!^`
	if got := windowsShellQuote(dangerousWindowsPath); got != `"C:\Users\Me\slidex ^%workspace^%^!^^"` {
		t.Fatalf("windows shell quote did not escape cmd metacharacters: %s", got)
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
	if snapshot["browserOpenMechanism"] != "codex_in_app_browser_url_click_manual_navigation_or_browser_plugin" {
		t.Fatalf("unexpected browser open mechanism: %#v", snapshot)
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
	for _, name := range []string{"CHROME_BIN", "GOOGLE_CHROME_BIN", "CHROMIUM_BIN", "MSEDGE_BIN"} {
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
	windows := chromeExecutableCandidates("windows")
	for _, name := range []string{"chrome.exe", "msedge.exe", "chromium.exe"} {
		if !testStringSliceContains(windows, name) {
			t.Fatalf("windows chrome candidates missing %s: %#v", name, windows)
		}
	}
}

func TestResolveChromeAcceptsMacOSAppBundle(t *testing.T) {
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
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("json file mode = %o, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("json dir mode = %o, want 0700", dirInfo.Mode().Perm())
	}
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

func TestWebSocketAuthRequiresPrivateFilesAndTunnelAck(t *testing.T) {
	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("token"), 0o644); err != nil {
		t.Fatal(err)
	}
	tokenHash := sha256Bytes([]byte("token"))
	err := validateWebSocketAuth("ws://10.0.0.2:1234", webSocketAuthConfig{Mode: "capability-token", TokenFile: token, TokenSHA256: tokenHash})
	if runtime.GOOS == "windows" {
		if err == nil || !strings.Contains(err.Error(), "tunnel") {
			t.Fatalf("expected Windows public token file to pass mode check and fail tunnel ack, got %v", err)
		}
	} else if err == nil {
		t.Fatal("expected public token file to fail")
	}
	if err := os.Chmod(token, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWebSocketAuth("ws://127.0.0.1:1234", webSocketAuthConfig{}); err != nil {
		t.Fatalf("loopback websocket without auth should pass: %v", err)
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

func TestCollectDependenciesDecodesLocalURLs(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "final_deck.html")
	stylesheet := filepath.Join(dir, "style one.css")
	asset := filepath.Join(dir, "assets", "hero image.png")
	fileAsset := filepath.Join(dir, "absolute image.png")
	font := filepath.Join(dir, "fonts", "brand font.woff2")
	for _, item := range []struct {
		path string
		body string
	}{
		{stylesheet, "body{}\n"},
		{asset, "asset\n"},
		{fileAsset, "file asset\n"},
		{font, "font\n"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(item.path, []byte(item.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := fmt.Sprintf(`<!doctype html>
<link rel="stylesheet" href="style%%20one.css?v=1">
<img src="assets/hero%%20image.png#view">
<img src="%s">
<style>@font-face { font-family: Brand; src: url("fonts/brand%%20font.woff2?x=1"); }</style>`, fileURLFromPath(fileAsset))
	styles, assets, fonts := collectDependencies(htmlPath, src, "pretendard")
	for _, path := range []string{stylesheet, asset, fileAsset, font} {
		dep, ok := findDependencyByPath(append(append(styles, assets...), fonts...), path)
		if !ok {
			t.Fatalf("dependency path %q not found\nstyles=%#v\nassets=%#v\nfonts=%#v", path, styles, assets, fonts)
		}
		if dep.SHA256 == "" || dep.Risk != "" {
			t.Fatalf("dependency %q did not resolve cleanly: %#v", path, dep)
		}
	}
}

func TestFillDependencyRejectsUnsupportedURLSchemes(t *testing.T) {
	dep := dependency{Kind: "asset"}
	fillDependency(&dep, t.TempDir(), "ftp://example.com/asset.png")
	if dep.URL != "ftp://example.com/asset.png" || !strings.Contains(dep.Risk, "unsupported") {
		t.Fatalf("unsupported URL scheme should be recorded as risk, got %#v", dep)
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
	for _, listen := range []string{"ws://127.0.0.1:1234/app", "ws://[::1]:1234/app", "ws://localhost:1234/app"} {
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
