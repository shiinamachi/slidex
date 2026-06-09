package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	toolName    = "slidex"
	toolVersion = "0.1.0"
)

type fileEntry struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256,omitempty"`
	ModTime string `json:"modTime"`
}

type inventory struct {
	ToolName string      `json:"toolName"`
	Version  string      `json:"version"`
	DeckDir  string      `json:"deckDir"`
	OutDir   string      `json:"outDir"`
	Inputs   []fileEntry `json:"inputs"`
	Outputs  []fileEntry `json:"outputs"`
	Warnings []string    `json:"warnings,omitempty"`
}

type slideInfo struct {
	ID       string
	Attrs    string
	HTML     string
	FullHTML string
	Headline string
	Text     string
}

type artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

type dimension struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type dependency struct {
	ID        string `json:"id,omitempty"`
	Kind      string `json:"kind"`
	Path      string `json:"path,omitempty"`
	URL       string `json:"url,omitempty"`
	Version   string `json:"version,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Risk      string `json:"risk,omitempty"`
	Retrieved string `json:"retrievalTimestamp,omitempty"`
}

type renderedImage struct {
	SlideID    string    `json:"slideId"`
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	Dimensions dimension `json:"dimensions"`
	Blank      bool      `json:"blank"`
}

type renderManifest struct {
	ToolName               string          `json:"toolName"`
	Version                string          `json:"version"`
	RenderTimestamp        string          `json:"renderTimestamp"`
	SourceHTML             artifact        `json:"sourceHtml"`
	Stylesheets            []dependency    `json:"stylesheets"`
	Assets                 []dependency    `json:"assets"`
	Fonts                  []dependency    `json:"fonts"`
	FontPreset             string          `json:"fontPreset"`
	SlideSelector          string          `json:"slideSelector"`
	OrderedSlideIDs        []string        `json:"orderedSlideIds"`
	ExpectedDimensions     dimension       `json:"expectedDimensions"`
	ActualDimensions       []dimension     `json:"actualSlideImageDimensions"`
	PNGFiles               []renderedImage `json:"pngFiles"`
	PDF                    artifact        `json:"pdf"`
	PDFMode                string          `json:"pdfMode"`
	PDFPageCount           int             `json:"pdfPageCount"`
	PDFPageSizePoints      dimension       `json:"pdfPageSizePoints"`
	PDFImageFit            string          `json:"pdfImageFit"`
	QAMontage              artifact        `json:"qaMontage"`
	QAMontageDimensions    dimension       `json:"qaMontageDimensions"`
	ChromeVersion          string          `json:"chromeVersion"`
	ChromeSandbox          string          `json:"chromeSandbox"`
	ChromeNoSandboxReason  string          `json:"chromeNoSandboxReason,omitempty"`
	OperatingSystem        string          `json:"operatingSystem"`
	RenderMethod           string          `json:"renderMethod"`
	SlideEnumerationMethod string          `json:"slideEnumerationMethod"`
	RepoRelativePaths      bool            `json:"repoRelativePaths"`
	Warnings               []string        `json:"unresolvedRenderWarnings,omitempty"`
}

type qaFinding struct {
	Severity string `json:"severity"`
	Check    string `json:"check"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

type qaResult struct {
	ToolName         string      `json:"toolName"`
	Version          string      `json:"version"`
	DeckDir          string      `json:"deckDir"`
	Status           string      `json:"status"`
	RuntimeMode      string      `json:"runtimeMode,omitempty"`
	RuntimeReason    string      `json:"runtimeReason,omitempty"`
	VisualReviewMode string      `json:"visualReviewMode,omitempty"`
	VisualStatus     string      `json:"visualStatus,omitempty"`
	FilesChecked     []string    `json:"filesChecked"`
	SlideCount       int         `json:"slideCount"`
	PDFPageCount     int         `json:"pdfPageCount"`
	RenderMethod     string      `json:"renderMethod,omitempty"`
	Findings         []qaFinding `json:"findings"`
	GeneratedReport  string      `json:"generatedReport,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h" {
		usage()
		return
	}

	var err error
	switch os.Args[1] {
	case "init":
		err = runInit(os.Args[2:])
	case "doctor":
		err = runDoctor(os.Args[2:])
	case "inspect":
		err = runInspect(os.Args[2:])
	case "intake":
		err = runIntake(os.Args[2:])
	case "strategy":
		err = runStrategy(os.Args[2:])
	case "spec":
		err = runSpec(os.Args[2:])
	case "build":
		err = runBuild(os.Args[2:])
	case "validate-spec":
		err = runValidateSpec(os.Args[2:])
	case "render":
		err = runRender(os.Args[2:])
	case "qa":
		err = runQA(os.Args[2:])
	case "revise":
		err = runRevise(os.Args[2:])
	case "sync-html-edits":
		err = runSyncHTMLEdits(os.Args[2:])
	case "finalize":
		err = runFinalize(os.Args[2:])
	case "package":
		err = runPackage(os.Args[2:])
	case "clean":
		err = runClean(os.Args[2:])
	case "run":
		err = runPipeline(os.Args[2:])
	case "codex":
		err = runCodex(os.Args[2:])
	case "goal":
		err = runGoal(os.Args[2:])
	case "mcp-server":
		err = runMCPServer(os.Args[2:])
	case "migrate":
		err = runMigrate(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("%s %s\n", toolName, toolVersion)
		return
	default:
		usage()
		err = exitCodeError(2, "unknown command: %s", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code := 1
		var coded interface{ ExitCode() int }
		if errors.As(err, &coded) {
			code = coded.ExitCode()
		}
		os.Exit(code)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `%s %s

Commands:
  init <deck_id>
  doctor [--deck decks/<deck_id>] [--codex] [--render] [--json]
  inspect --deck decks/<deck_id>
  intake --deck decks/<deck_id> [--interactive|--answers FILE]
  strategy --deck decks/<deck_id>
  spec --deck decks/<deck_id>
  build --deck decks/<deck_id>
  validate-spec --spec decks/<deck_id>/out/deck_spec.json
  render --deck decks/<deck_id>
  render --html decks/<deck_id>/out/final_deck.html --pdf decks/<deck_id>/out/final_deck.pdf
  qa --deck decks/<deck_id> [--visual-review codex|manual|none]
  revise --deck decks/<deck_id>
  sync-html-edits --deck decks/<deck_id>
  finalize --deck decks/<deck_id>
  package --deck decks/<deck_id>
  clean --deck decks/<deck_id> [--logs] [--older-than DURATION]
  run --deck decks/<deck_id> [--until package|qa|render] [--non-interactive]
  codex doctor|app-server|schema|exec|models|features|mcp|plugins|threads|turn|review|remote-control
  goal set|status|pause|resume|complete|clear --deck decks/<deck_id>
  mcp-server --stdio
  migrate --deck decks/<deck_id> [--from html-pdf] [--write]
`, toolName, toolVersion)
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	write := fs.Bool("write", false, "also write out/source_inventory.md")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return errors.New("--deck is required")
	}
	inv, err := inspectDeck(*deck)
	if err != nil {
		return err
	}
	if *write {
		if err := writeSourceInventory(inv); err != nil {
			return err
		}
	}
	return printJSON(inv)
}

func inspectDeck(deck string) (inventory, error) {
	deckAbs, err := filepath.Abs(deck)
	if err != nil {
		return inventory{}, err
	}
	info, err := os.Stat(deckAbs)
	if err != nil {
		return inventory{}, err
	}
	if !info.IsDir() {
		return inventory{}, fmt.Errorf("deck path is not a directory: %s", deck)
	}
	outDir := filepath.Join(deckAbs, "out")
	inv := inventory{
		ToolName: toolName,
		Version:  toolVersion,
		DeckDir:  deckAbs,
		OutDir:   outDir,
	}
	err = filepath.WalkDir(deckAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			inv.Warnings = append(inv.Warnings, walkErr.Error())
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		entry, err := makeFileEntry(deckAbs, path)
		if err != nil {
			inv.Warnings = append(inv.Warnings, err.Error())
			return nil
		}
		if strings.HasPrefix(path, outDir+string(os.PathSeparator)) {
			inv.Outputs = append(inv.Outputs, entry)
		} else {
			inv.Inputs = append(inv.Inputs, entry)
		}
		return nil
	})
	sort.Slice(inv.Inputs, func(i, j int) bool { return inv.Inputs[i].Path < inv.Inputs[j].Path })
	sort.Slice(inv.Outputs, func(i, j int) bool { return inv.Outputs[i].Path < inv.Outputs[j].Path })
	return inv, err
}

func makeFileEntry(root, path string) (fileEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileEntry{}, err
	}
	rel, _ := filepath.Rel(root, path)
	hash, _ := sha256File(path)
	return fileEntry{
		Path:    filepath.ToSlash(rel),
		Kind:    classifyPath(rel),
		Size:    info.Size(),
		SHA256:  hash,
		ModTime: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

func classifyPath(path string) string {
	p := filepath.ToSlash(strings.ToLower(path))
	ext := strings.TrimPrefix(filepath.Ext(p), ".")
	switch {
	case p == "brief.md":
		return "brief"
	case p == "design.md":
		return "design_guidance"
	case strings.HasPrefix(p, "brand/"):
		return "brand"
	case strings.HasPrefix(p, "data/"):
		return "data_source"
	case strings.HasPrefix(p, "source/"):
		return "source_document"
	case strings.HasPrefix(p, "assets/reference_docs/"):
		return "reference_document"
	case strings.HasPrefix(p, "assets/images/") || strings.Contains(p, "logo."):
		return "image_asset"
	case strings.HasPrefix(p, "out/rendered_slides/"):
		return "rendered_slide"
	case p == "out/final_deck.html":
		return "source_html"
	case p == "out/final_deck.generated_baseline.html":
		return "generated_baseline_html"
	case p == "out/final_deck.pdf":
		return "primary_pdf"
	case p == "out/render_manifest.json":
		return "render_manifest"
	case p == "out/qa_montage.png":
		return "qa_montage"
	case p == "out/deck_spec.json":
		return "deck_spec"
	case p == "out/qa_report.md":
		return "qa_report"
	case ext != "":
		return ext
	default:
		return "file"
	}
}

func writeSourceInventory(inv inventory) error {
	if err := os.MkdirAll(inv.OutDir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Source Inventory\n\n")
	b.WriteString(fmt.Sprintf("- Tool: `%s %s`\n", inv.ToolName, inv.Version))
	b.WriteString(fmt.Sprintf("- Deck directory: `%s`\n", inv.DeckDir))
	b.WriteString(fmt.Sprintf("- Output directory: `%s`\n\n", inv.OutDir))
	b.WriteString("## Inputs\n\n")
	for _, e := range inv.Inputs {
		b.WriteString(fmt.Sprintf("- `%s` (%s, %d bytes, sha256 `%s`)\n", e.Path, e.Kind, e.Size, e.SHA256))
	}
	b.WriteString("\n## Existing Outputs\n\n")
	for _, e := range inv.Outputs {
		b.WriteString(fmt.Sprintf("- `%s` (%s, %d bytes, sha256 `%s`)\n", e.Path, e.Kind, e.Size, e.SHA256))
	}
	if len(inv.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range inv.Warnings {
			b.WriteString(fmt.Sprintf("- %s\n", w))
		}
	}
	return os.WriteFile(filepath.Join(inv.OutDir, "source_inventory.md"), []byte(b.String()), 0o644)
}

func runValidateSpec(args []string) error {
	fs := flag.NewFlagSet("validate-spec", flag.ContinueOnError)
	spec := fs.String("spec", "", "deck_spec.json path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *spec == "" {
		return errors.New("--spec is required")
	}
	findings, err := validateSpecFile(*spec)
	if err != nil {
		return err
	}
	result := map[string]any{
		"toolName": toolName,
		"version":  toolVersion,
		"spec":     *spec,
		"status":   statusFromFindings(findings),
		"findings": findings,
	}
	if err := printJSON(result); err != nil {
		return err
	}
	if statusFromFindings(findings) == "fail" {
		return errors.New("spec validation failed")
	}
	return nil
}

func validateSpecFile(path string) ([]qaFinding, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec any
	if err := json.Unmarshal(raw, &spec); err != nil {
		return []qaFinding{{Severity: "fail", Check: "json_parse", Message: err.Error(), Path: path}}, nil
	}
	obj, ok := spec.(map[string]any)
	if !ok {
		return []qaFinding{{Severity: "fail", Check: "schema", Message: "spec root must be an object", Path: path}}, nil
	}

	var findings []qaFinding
	if schema, err := loadSpecSchema(); err != nil {
		findings = append(findings, qaFinding{Severity: "warn", Check: "schema.load", Message: "could not load schemas/deck_spec.schema.json for full schema validation: " + err.Error(), Path: path})
	} else {
		findings = append(findings, validateWithFullJSONSchema(raw, schema, path)...)
	}
	required := []string{
		"metadata", "documentType", "audience", "objective", "desiredOutcome", "tone",
		"sourceInventory", "intakeStatus", "outputContract", "renderConfig", "pdfConfig",
		"designSystem", "storyArc", "slides", "claimProvenance", "businessQa", "userEditPolicy",
	}
	for _, key := range required {
		if _, ok := obj[key]; !ok {
			findings = append(findings, fail("schema.required", "missing required field: "+key, path))
		}
	}
	if dt, _ := obj["documentType"].(string); dt != "" && !in(dt, []string{"business_plan", "ir_deck", "company_profile", "proposal", "government_grant_plan", "executive_report", "custom"}) {
		findings = append(findings, fail("schema.documentType", "unsupported documentType: "+dt, path))
	}
	if slides, ok := obj["slides"].([]any); !ok || len(slides) == 0 {
		findings = append(findings, fail("schema.slides", "slides must be a non-empty array", path))
	} else {
		for i, rawSlide := range slides {
			slide, ok := rawSlide.(map[string]any)
			if !ok {
				findings = append(findings, fail("schema.slide", fmt.Sprintf("slide %d must be an object", i+1), path))
				continue
			}
			for _, key := range []string{"id", "htmlId", "sectionRole", "headline", "keyMessage", "bodyContent", "layoutIntent", "visualIntent", "evidenceRefs", "claims", "renderRisks", "qaChecks"} {
				if _, ok := slide[key]; !ok {
					findings = append(findings, fail("schema.slide.required", fmt.Sprintf("slide %d missing %s", i+1, key), path))
				}
			}
		}
	}
	if rc, ok := obj["renderConfig"].(map[string]any); ok {
		if engine, _ := rc["engine"].(string); engine != "slidex-cli" {
			findings = append(findings, fail("schema.renderConfig.engine", "renderConfig.engine must be slidex-cli", path))
		}
		if width, ok := numberAsInt(rc["widthPx"]); !ok || width <= 0 {
			findings = append(findings, fail("schema.renderConfig.widthPx", "widthPx must be positive", path))
		}
		if height, ok := numberAsInt(rc["heightPx"]); !ok || height <= 0 {
			findings = append(findings, fail("schema.renderConfig.heightPx", "heightPx must be positive", path))
		}
	}
	if pdf, ok := obj["pdfConfig"].(map[string]any); ok {
		if mode, _ := pdf["mode"].(string); mode != "paginated" {
			findings = append(findings, fail("schema.pdfConfig.mode", "pdfConfig.mode must be paginated", path))
		}
		if source, _ := pdf["source"].(string); source != "rendered_images" {
			findings = append(findings, fail("schema.pdfConfig.source", "pdfConfig.source must be rendered_images", path))
		}
	}
	if contract, ok := obj["outputContract"].(map[string]any); ok {
		if source, _ := contract["sourceHtml"].(string); source != "" && !strings.HasSuffix(source, ".html") {
			findings = append(findings, fail("schema.outputContract.sourceHtml", "sourceHtml must point to an HTML artifact", path))
		}
		if primary, _ := contract["primaryPdf"].(string); primary != "" && !strings.HasSuffix(primary, ".pdf") {
			findings = append(findings, fail("schema.outputContract.primaryPdf", "primaryPdf must point to a PDF artifact", path))
		}
		if manifest, _ := contract["renderManifest"].(string); manifest != "" && !strings.HasSuffix(manifest, ".json") {
			findings = append(findings, fail("schema.outputContract.renderManifest", "renderManifest must point to a JSON artifact", path))
		}
	}
	if cp, ok := obj["claimProvenance"].(map[string]any); ok {
		if required, ok := cp["required"].(bool); !ok || !required {
			findings = append(findings, fail("claimProvenance.required", "claim provenance must be required", path))
		}
		if claims, ok := cp["claims"].([]any); ok {
			for _, rawClaim := range claims {
				claim, _ := rawClaim.(map[string]any)
				if status, _ := claim["status"].(string); status == "unsupported" {
					findings = append(findings, fail("claimProvenance.unsupported", "unsupported claim remains: "+fmt.Sprint(claim["id"]), path))
				}
			}
		}
	}
	return findings, nil
}

func numberAsInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func loadSpecSchema() (map[string]any, error) {
	cwd, _ := os.Getwd()
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "schemas", "deck_spec.schema.json")
		raw, err := os.ReadFile(candidate)
		if err == nil {
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil {
				return nil, err
			}
			return schema, nil
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return nil, errors.New("schemas/deck_spec.schema.json not found from current working directory")
}

func validateWithFullJSONSchema(instanceRaw []byte, schema map[string]any, sourcePath string) []qaFinding {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(instanceRaw))
	if err != nil {
		return []qaFinding{fail("schema.full_json_schema", "could not decode instance for full JSON Schema validation: "+err.Error(), sourcePath)}
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("deck_spec.schema.json", schema); err != nil {
		return []qaFinding{fail("schema.full_json_schema", "could not load schema resource: "+err.Error(), sourcePath)}
	}
	compiled, err := compiler.Compile("deck_spec.schema.json")
	if err != nil {
		return []qaFinding{fail("schema.full_json_schema", "could not compile schema: "+err.Error(), sourcePath)}
	}
	if err := compiled.Validate(instance); err != nil {
		return []qaFinding{fail("schema.full_json_schema", err.Error(), sourcePath)}
	}
	return nil
}

func validateJSONSchema(value any, schema map[string]any, root map[string]any, path string, sourcePath string) []qaFinding {
	if ref, _ := schema["$ref"].(string); ref != "" {
		resolved, err := resolveJSONPointer(root, ref)
		if err != nil {
			return []qaFinding{fail("schema.$ref", err.Error(), sourcePath)}
		}
		return validateJSONSchema(value, resolved, root, path, sourcePath)
	}

	var findings []qaFinding
	if enumValues, ok := schema["enum"].([]any); ok {
		matched := false
		for _, allowed := range enumValues {
			if reflect.DeepEqual(value, allowed) {
				matched = true
				break
			}
		}
		if !matched {
			findings = append(findings, fail("schema.enum", fmt.Sprintf("%s value %q is not in enum", path, value), sourcePath))
		}
	}
	if constValue, ok := schema["const"]; ok && !reflect.DeepEqual(value, constValue) {
		findings = append(findings, fail("schema.const", fmt.Sprintf("%s value %q does not match const %q", path, value, constValue), sourcePath))
	}

	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return append(findings, fail("schema.type", path+" must be object", sourcePath))
		}
		if required, ok := schema["required"].([]any); ok {
			for _, rawKey := range required {
				key, _ := rawKey.(string)
				if key != "" {
					if _, exists := obj[key]; !exists {
						findings = append(findings, fail("schema.required", path+" missing required field "+key, sourcePath))
					}
				}
			}
		}
		props := map[string]any{}
		if rawProps, ok := schema["properties"].(map[string]any); ok {
			props = rawProps
			for key, childSchema := range rawProps {
				if child, exists := obj[key]; exists {
					if childMap, ok := childSchema.(map[string]any); ok {
						findings = append(findings, validateJSONSchema(child, childMap, root, path+"."+key, sourcePath)...)
					}
				}
			}
		}
		if additional, exists := schema["additionalProperties"]; exists {
			switch add := additional.(type) {
			case bool:
				if !add {
					for key := range obj {
						if _, allowed := props[key]; !allowed {
							findings = append(findings, fail("schema.additionalProperties", path+" has unsupported field "+key, sourcePath))
						}
					}
				}
			case map[string]any:
				for key, child := range obj {
					if _, known := props[key]; !known {
						findings = append(findings, validateJSONSchema(child, add, root, path+"."+key, sourcePath)...)
					}
				}
			}
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return append(findings, fail("schema.type", path+" must be array", sourcePath))
		}
		if min, ok := numberAsFloat(schema["minItems"]); ok && float64(len(arr)) < min {
			findings = append(findings, fail("schema.minItems", fmt.Sprintf("%s must have at least %.0f items", path, min), sourcePath))
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for i, item := range arr {
				findings = append(findings, validateJSONSchema(item, itemSchema, root, fmt.Sprintf("%s[%d]", path, i), sourcePath)...)
			}
		}
	case "string":
		s, ok := value.(string)
		if !ok {
			return append(findings, fail("schema.type", path+" must be string", sourcePath))
		}
		if min, ok := numberAsFloat(schema["minLength"]); ok && float64(len(s)) < min {
			findings = append(findings, fail("schema.minLength", fmt.Sprintf("%s must be at least %.0f characters", path, min), sourcePath))
		}
		if pattern, _ := schema["pattern"].(string); pattern != "" {
			re, err := regexp.Compile(pattern)
			if err != nil {
				findings = append(findings, fail("schema.pattern", "invalid schema pattern at "+path+": "+err.Error(), sourcePath))
			} else if !re.MatchString(s) {
				findings = append(findings, fail("schema.pattern", path+" does not match pattern "+pattern, sourcePath))
			}
		}
	case "integer":
		n, ok := value.(float64)
		if !ok || math.Trunc(n) != n {
			return append(findings, fail("schema.type", path+" must be integer", sourcePath))
		}
		findings = append(findings, validateNumberBounds(n, schema, path, sourcePath)...)
	case "number":
		n, ok := value.(float64)
		if !ok {
			return append(findings, fail("schema.type", path+" must be number", sourcePath))
		}
		findings = append(findings, validateNumberBounds(n, schema, path, sourcePath)...)
	case "boolean":
		if _, ok := value.(bool); !ok {
			return append(findings, fail("schema.type", path+" must be boolean", sourcePath))
		}
	}
	return findings
}

func resolveJSONPointer(root map[string]any, ref string) (map[string]any, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("unsupported ref: %s", ref)
	}
	var cur any = root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ref %s cannot resolve through non-object", ref)
		}
		next, ok := obj[part]
		if !ok {
			return nil, fmt.Errorf("ref %s missing part %s", ref, part)
		}
		cur = next
	}
	resolved, ok := cur.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ref %s did not resolve to schema object", ref)
	}
	return resolved, nil
}

func validateNumberBounds(n float64, schema map[string]any, path string, sourcePath string) []qaFinding {
	var findings []qaFinding
	if min, ok := numberAsFloat(schema["minimum"]); ok && n < min {
		findings = append(findings, fail("schema.minimum", fmt.Sprintf("%s must be >= %v", path, min), sourcePath))
	}
	if min, ok := numberAsFloat(schema["exclusiveMinimum"]); ok && n <= min {
		findings = append(findings, fail("schema.exclusiveMinimum", fmt.Sprintf("%s must be > %v", path, min), sourcePath))
	}
	return findings
}

func numberAsFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func runRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	htmlPath := fs.String("html", "", "source HTML path")
	outDir := fs.String("out", "", "rendered slide output directory")
	pdfPath := fs.String("pdf", "", "PDF output path")
	manifestPath := fs.String("manifest", "", "render manifest path")
	pdfMode := fs.String("pdf-mode", "paginated", "PDF mode")
	selector := fs.String("selector", ".slide", "slide selector")
	width := fs.Int("width", 1920, "render width")
	height := fs.Int("height", 1080, "render height")
	fontPreset := fs.String("font-preset", "pretendard", "font preset")
	chromePath := fs.String("chrome", "", "Chrome/Chromium binary")
	chromeNoSandbox := fs.Bool("chrome-no-sandbox", false, "run Chrome with --no-sandbox and record the risk")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck != "" && *htmlPath == "" {
		out := filepath.Join(mustAbs(*deck), "out")
		*htmlPath = filepath.Join(out, "final_deck.html")
		if *outDir == "" {
			*outDir = filepath.Join(out, "rendered_slides")
		}
		if *pdfPath == "" {
			*pdfPath = filepath.Join(out, "final_deck.pdf")
		}
		if *manifestPath == "" {
			*manifestPath = filepath.Join(out, "render_manifest.json")
		}
	}
	cfg, err := renderConfigFromFlags(*htmlPath, *outDir, *pdfPath, *manifestPath, *pdfMode, *selector, *width, *height, *fontPreset, *chromePath, *chromeNoSandbox)
	if err != nil {
		return err
	}
	manifest, err := renderHTML(cfg)
	if err != nil {
		return err
	}
	return printJSON(manifest)
}

type renderConfig struct {
	HTMLPath        string
	OutDir          string
	PDFPath         string
	ManifestPath    string
	MontagePath     string
	PDFMode         string
	Selector        string
	Width           int
	Height          int
	FontPreset      string
	ChromePath      string
	ChromeNoSandbox bool
}

func renderConfigFromFlags(htmlPath, outDir, pdfPath, manifestPath, pdfMode, selector string, width, height int, fontPreset, chromePath string, chromeNoSandbox bool) (renderConfig, error) {
	if htmlPath == "" {
		return renderConfig{}, errors.New("--html or --deck is required")
	}
	if pdfMode != "paginated" {
		return renderConfig{}, errors.New("--pdf-mode must be paginated")
	}
	if width <= 0 || height <= 0 {
		return renderConfig{}, errors.New("--width and --height must be positive")
	}
	htmlAbs, err := filepath.Abs(htmlPath)
	if err != nil {
		return renderConfig{}, err
	}
	htmlDir := filepath.Dir(htmlAbs)
	if outDir == "" {
		outDir = filepath.Join(htmlDir, "rendered_slides")
	}
	if pdfPath == "" {
		pdfPath = filepath.Join(htmlDir, "final_deck.pdf")
	}
	if manifestPath == "" {
		manifestPath = filepath.Join(htmlDir, "render_manifest.json")
	}
	return renderConfig{
		HTMLPath:        htmlAbs,
		OutDir:          mustAbs(outDir),
		PDFPath:         mustAbs(pdfPath),
		ManifestPath:    mustAbs(manifestPath),
		MontagePath:     filepath.Join(filepath.Dir(mustAbs(manifestPath)), "qa_montage.png"),
		PDFMode:         pdfMode,
		Selector:        selector,
		Width:           width,
		Height:          height,
		FontPreset:      fontPreset,
		ChromePath:      chromePath,
		ChromeNoSandbox: chromeNoSandbox,
	}, nil
}

func renderHTML(cfg renderConfig) (renderManifest, error) {
	raw, err := os.ReadFile(cfg.HTMLPath)
	if err != nil {
		return renderManifest{}, err
	}
	chromePath, err := resolveChrome(cfg.ChromePath)
	if err != nil {
		return renderManifest{}, err
	}
	if cfg.Selector != ".slide" {
		return renderManifest{}, errors.New("only .slide selector is currently supported")
	}
	slides, enumMethod, enumErr := extractSlidesWithChrome(chromePath, cfg.HTMLPath, cfg.Selector, cfg.ChromeNoSandbox)
	if enumErr != nil || len(slides) == 0 {
		parserSlides := extractSlides(string(raw))
		if len(parserSlides) == 0 {
			if enumErr != nil {
				return renderManifest{}, fmt.Errorf("no .slide elements found in HTML; chrome DOM enumeration failed: %w", enumErr)
			}
			return renderManifest{}, errors.New("no .slide elements found in HTML")
		}
		slides = parserSlides
		enumMethod = "go-html-parser-fallback"
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return renderManifest{}, err
	}
	if err := cleanRenderedSlides(cfg.OutDir); err != nil {
		return renderManifest{}, err
	}

	head := extractHead(string(raw))
	tmpDir, err := os.MkdirTemp("", "slidex-render-*")
	if err != nil {
		return renderManifest{}, err
	}
	defer os.RemoveAll(tmpDir)

	var manifest renderManifest
	manifest.ToolName = toolName
	manifest.Version = toolVersion
	manifest.RenderTimestamp = time.Now().UTC().Format(time.RFC3339)
	manifest.SourceHTML = artifactFromPath(cfg.HTMLPath)
	manifest.Stylesheets, manifest.Assets, manifest.Fonts = collectDependencies(cfg.HTMLPath, string(raw), cfg.FontPreset)
	manifest.FontPreset = cfg.FontPreset
	manifest.SlideSelector = cfg.Selector
	manifest.ExpectedDimensions = dimension{Width: cfg.Width, Height: cfg.Height}
	manifest.PDFMode = cfg.PDFMode
	manifest.PDFImageFit = "exact"
	manifest.OperatingSystem = runtime.GOOS + "/" + runtime.GOARCH
	manifest.ChromeVersion = chromeVersion(chromePath)
	if cfg.ChromeNoSandbox {
		manifest.ChromeSandbox = "disabled"
		manifest.ChromeNoSandboxReason = "explicit --chrome-no-sandbox flag"
		manifest.Warnings = append(manifest.Warnings, "Chrome sandbox disabled by explicit flag; use only in root/container CI fallback contexts.")
	} else {
		manifest.ChromeSandbox = "enabled"
	}
	manifest.RenderMethod = "headless Chrome DOM enumeration and element-isolated wrapper screenshots, then PNG-to-PDF assembly"
	manifest.SlideEnumerationMethod = enumMethod
	manifest.RepoRelativePaths = true

	for i, slide := range slides {
		if slide.ID == "" {
			slide.ID = fmt.Sprintf("slide_%02d", i+1)
			manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("slide %d lacked data-slide-id; assigned %s", i+1, slide.ID))
		}
		manifest.OrderedSlideIDs = append(manifest.OrderedSlideIDs, slide.ID)
		wrapper := buildSlideWrapper(head, slide.FullHTML, cfg.Width, cfg.Height, cfg.FontPreset)
		wrapperPath := filepath.Join(tmpDir, fmt.Sprintf("%s.html", slide.ID))
		if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o644); err != nil {
			return manifest, err
		}
		overflowIssues, err := checkOverflowWithChrome(chromePath, wrapperPath, cfg.ChromeNoSandbox)
		if err != nil {
			manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("overflow check could not run for %s: %v", slide.ID, err))
		}
		if len(overflowIssues) > 0 {
			return manifest, fmt.Errorf("visible clipping or overflow risk on %s: %s", slide.ID, strings.Join(overflowIssues, "; "))
		}
		pngPath := filepath.Join(cfg.OutDir, fmt.Sprintf("slide_%02d.png", i+1))
		if err := captureScreenshot(chromePath, wrapperPath, pngPath, cfg.Width, cfg.Height, cfg.ChromeNoSandbox); err != nil {
			return manifest, err
		}
		dim, blank, err := validatePNG(pngPath, cfg.Width, cfg.Height)
		if err != nil {
			return manifest, err
		}
		if blank {
			return manifest, fmt.Errorf("rendered slide appears blank: %s", pngPath)
		}
		img := renderedImage{
			SlideID:    slide.ID,
			Path:       pngPath,
			SHA256:     mustSHA256(pngPath),
			Dimensions: dim,
			Blank:      blank,
		}
		manifest.PNGFiles = append(manifest.PNGFiles, img)
		manifest.ActualDimensions = append(manifest.ActualDimensions, dim)
	}

	pageW, pageH := pdfPageSizePoints(cfg.Width, cfg.Height)
	if err := writePDFFromPNGs(cfg.PDFPath, pngPaths(manifest.PNGFiles), pageW, pageH); err != nil {
		return manifest, err
	}
	manifest.PDF = artifactFromPath(cfg.PDFPath)
	manifest.PDFPageCount = len(manifest.PNGFiles)
	manifest.PDFPageSizePoints = dimension{Width: int(math.Round(pageW)), Height: int(math.Round(pageH))}

	montageDim, err := createMontage(cfg.MontagePath, pngPaths(manifest.PNGFiles))
	if err != nil {
		return manifest, err
	}
	manifest.QAMontage = artifactFromPath(cfg.MontagePath)
	manifest.QAMontageDimensions = montageDim

	if err := os.MkdirAll(filepath.Dir(cfg.ManifestPath), 0o755); err != nil {
		return manifest, err
	}
	if err := writeJSONFile(cfg.ManifestPath, manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func extractSlides(src string) []slideInfo {
	if slides := extractSlidesHTMLParser(src); len(slides) > 0 {
		return slides
	}
	return extractSlidesRegex(src)
}

func extractSlidesRegex(src string) []slideInfo {
	re := regexp.MustCompile(`(?is)<section\b([^>]*)>(.*?)</section>`)
	var slides []slideInfo
	matches := re.FindAllStringSubmatchIndex(src, -1)
	for _, m := range matches {
		full := src[m[0]:m[1]]
		attrs := src[m[2]:m[3]]
		inner := src[m[4]:m[5]]
		if !hasClass(attrs, "slide") {
			continue
		}
		id := attrValue(attrs, "data-slide-id")
		if id == "" {
			id = attrValue(attrs, "id")
		}
		text := normalizeText(stripTags(inner))
		slides = append(slides, slideInfo{
			ID:       id,
			Attrs:    attrs,
			HTML:     inner,
			FullHTML: full,
			Headline: extractHeadline(inner),
			Text:     text,
		})
	}
	return slides
}

func hasClass(attrs, className string) bool {
	cls := attrValue(attrs, "class")
	for _, part := range strings.Fields(cls) {
		if part == className {
			return true
		}
	}
	return false
}

func attrValue(attrs, name string) string {
	re := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(name) + `\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	m := re.FindStringSubmatch(attrs)
	if len(m) == 0 {
		return ""
	}
	for i := 2; i <= 4; i++ {
		if m[i] != "" {
			return strings.TrimSpace(m[i])
		}
	}
	return ""
}

func extractHeadline(inner string) string {
	re := regexp.MustCompile(`(?is)<h[1-3]\b[^>]*>(.*?)</h[1-3]>`)
	m := re.FindStringSubmatch(inner)
	if len(m) < 2 {
		return ""
	}
	return normalizeText(stripTags(m[1]))
}

func stripTags(s string) string {
	s = regexp.MustCompile(`(?is)<script\b.*?</script>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<style\b.*?</style>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<br\s*/?>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " ")
	return s
}

func normalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func extractHead(src string) string {
	re := regexp.MustCompile(`(?is)<head\b[^>]*>(.*?)</head>`)
	m := re.FindStringSubmatch(src)
	if len(m) > 1 {
		return m[1]
	}
	return `<meta charset="utf-8">`
}

func buildSlideWrapper(head, slideHTML string, width, height int, fontPreset string) string {
	fontFamily := fontFamilyForPreset(fontPreset)
	return fmt.Sprintf(`<!doctype html>
<html lang="ko">
<head>
%s
<style id="slidex-render-wrapper">
:root { --slide-width: %dpx; --slide-height: %dpx; --font-body: %s; }
html, body { margin: 0 !important; padding: 0 !important; width: %dpx !important; height: %dpx !important; overflow: hidden !important; background: #fff; }
body { font-family: var(--font-body); }
.deck { width: %dpx !important; height: %dpx !important; margin: 0 !important; padding: 0 !important; overflow: hidden !important; }
.slide { box-sizing: border-box !important; width: %dpx !important; height: %dpx !important; min-width: %dpx !important; min-height: %dpx !important; max-width: %dpx !important; max-height: %dpx !important; overflow: hidden !important; margin: 0 !important; }
* { scrollbar-width: none !important; }
*::-webkit-scrollbar { display: none !important; }
</style>
<script>
async function slidexReady() {
  if (document.fonts && document.fonts.ready) {
    await document.fonts.ready;
  }
  document.documentElement.setAttribute('data-fonts-ready', 'true');
  const issues = [];
  document.querySelectorAll('.slide, .slide *').forEach((el, index) => {
    const className = String(el.className || '');
    if (/(^|\s)(arrow|line|top-rule|footer-rule|connector)(\s|$)/.test(className)) {
      return;
    }
    const overflowX = el.scrollWidth > el.clientWidth + 1;
    const overflowY = el.scrollHeight > el.clientHeight + 1;
    if (overflowX || overflowY) {
      issues.push({
        index,
        tag: el.tagName,
        id: el.id || '',
        className,
        clientWidth: el.clientWidth,
        clientHeight: el.clientHeight,
        scrollWidth: el.scrollWidth,
        scrollHeight: el.scrollHeight
      });
    }
  });
  const report = document.createElement('script');
  report.id = 'slidex-overflow-data';
  report.type = 'application/json';
  report.textContent = JSON.stringify(issues);
  document.body.appendChild(report);
}
document.addEventListener('DOMContentLoaded', () => { slidexReady(); });
</script>
</head>
<body><div class="deck">%s</div></body>
</html>`, head, width, height, fontFamily, width, height, width, height, width, height, width, height, width, height, slideHTML)
}

func fontFamilyForPreset(preset string) string {
	switch preset {
	case "pretendard":
		return `"Pretendard", "Noto Sans KR", "Apple SD Gothic Neo", Arial, sans-serif`
	case "noto-sans-kr":
		return `"Noto Sans KR", "Apple SD Gothic Neo", Arial, sans-serif`
	case "noto-sans-cjk-kr":
		return `"Noto Sans CJK KR", "Noto Sans KR", Arial, sans-serif`
	case "ibm-plex-sans-kr":
		return `"IBM Plex Sans KR", "Pretendard", Arial, sans-serif`
	case "suit":
		return `"SUIT", "Pretendard", Arial, sans-serif`
	default:
		return `"Pretendard", "Noto Sans KR", Arial, sans-serif`
	}
}

func resolveChrome(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit, nil
		}
		return "", fmt.Errorf("chrome binary not found: %s", explicit)
	}
	if env := os.Getenv("CHROME_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}
	for _, candidate := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("Chrome/Chromium binary not found")
}

func chromeVersion(chromePath string) string {
	out, err := exec.Command(chromePath, "--version").CombinedOutput()
	if err != nil {
		return "unknown: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

func captureScreenshot(chromePath, htmlPath, pngPath string, width, height int, chromeNoSandbox bool) error {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--force-device-scale-factor=1",
		"--virtual-time-budget=3000",
		"--screenshot=" + pngPath,
		u.String(),
	}
	if chromeNoSandbox {
		args = append([]string{"--no-sandbox"}, args...)
	}
	cmd := exec.Command(chromePath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("chrome screenshot failed: %w\n%s", err, string(out))
	}
	if info, err := os.Stat(pngPath); err != nil || info.Size() == 0 {
		return fmt.Errorf("chrome did not create screenshot: %s", pngPath)
	}
	return nil
}

func checkOverflowWithChrome(chromePath, htmlPath string, chromeNoSandbox bool) ([]string, error) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--hide-scrollbars",
		"--virtual-time-budget=3000",
		"--dump-dom",
		u.String(),
	}
	if chromeNoSandbox {
		args = append([]string{"--no-sandbox"}, args...)
	}
	cmd := exec.Command(chromePath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("chrome overflow probe failed: %w\n%s", err, string(out))
	}
	re := regexp.MustCompile(`(?is)<script id="slidex-overflow-data" type="application/json">(.*?)</script>`)
	m := re.FindStringSubmatch(string(out))
	if len(m) < 2 {
		return nil, errors.New("overflow report missing from dumped DOM")
	}
	var rawIssues []map[string]any
	if err := json.Unmarshal([]byte(m[1]), &rawIssues); err != nil {
		return nil, err
	}
	issues := make([]string, 0, len(rawIssues))
	for _, issue := range rawIssues {
		issues = append(issues, fmt.Sprintf("%s#%v.%v scroll=%vx%v client=%vx%v",
			issue["tag"], issue["id"], issue["className"],
			issue["scrollWidth"], issue["scrollHeight"],
			issue["clientWidth"], issue["clientHeight"],
		))
	}
	return issues, nil
}

func validatePNG(path string, expectedW, expectedH int) (dimension, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return dimension{}, false, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return dimension{}, false, err
	}
	b := img.Bounds()
	dim := dimension{Width: b.Dx(), Height: b.Dy()}
	if dim.Width != expectedW || dim.Height != expectedH {
		return dim, false, fmt.Errorf("wrong screenshot dimensions for %s: got %dx%d expected %dx%d", path, dim.Width, dim.Height, expectedW, expectedH)
	}
	blank := isBlank(img)
	return dim, blank, nil
}

func isBlank(img image.Image) bool {
	b := img.Bounds()
	if b.Empty() {
		return true
	}
	total := 0
	nonWhite := 0
	stepX := max(1, b.Dx()/200)
	stepY := max(1, b.Dy()/200)
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			total++
			if r < 0xf500 || g < 0xf500 || bl < 0xf500 {
				nonWhite++
			}
		}
	}
	if total == 0 {
		return true
	}
	return float64(nonWhite)/float64(total) < 0.002
}

func cleanRenderedSlides(outDir string) error {
	entries, err := os.ReadDir(outDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "slide_") && strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			if err := os.Remove(filepath.Join(outDir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectDependencies(htmlPath, src, fontPreset string) ([]dependency, []dependency, []dependency) {
	base := filepath.Dir(htmlPath)
	var styles []dependency
	var assets []dependency
	var fonts []dependency

	styleRe := regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)</style>`)
	for i, m := range styleRe.FindAllStringSubmatch(src, -1) {
		block := []byte(m[1])
		styles = append(styles, dependency{
			ID:     fmt.Sprintf("inline_style_%02d", i+1),
			Kind:   "inline_css",
			SHA256: sha256Bytes(block),
		})
	}
	linkRe := regexp.MustCompile(`(?is)<link\b[^>]*href\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))[^>]*>`)
	for _, m := range linkRe.FindAllStringSubmatch(src, -1) {
		href := firstNonEmpty(m[2], m[3], m[4])
		if href == "" {
			continue
		}
		dep := dependency{Kind: "stylesheet"}
		fillDependency(&dep, base, href)
		styles = append(styles, dep)
	}
	imgRe := regexp.MustCompile(`(?is)<(?:img|image|source)\b[^>]*(?:src|href)\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	for _, m := range imgRe.FindAllStringSubmatch(src, -1) {
		ref := firstNonEmpty(m[2], m[3], m[4])
		if ref == "" || strings.HasPrefix(ref, "data:") {
			continue
		}
		dep := dependency{Kind: "asset"}
		fillDependency(&dep, base, ref)
		assets = append(assets, dep)
	}
	fontURLRe := regexp.MustCompile(`(?is)@font-face\s*{.*?url\(\s*["']?([^'")]+)["']?\s*\).*?}`)
	for i, m := range fontURLRe.FindAllStringSubmatch(src, -1) {
		ref := strings.TrimSpace(m[1])
		if ref == "" || strings.HasPrefix(ref, "data:") {
			continue
		}
		dep := dependency{ID: fmt.Sprintf("font_face_%02d", i+1), Kind: "font_file"}
		fillDependency(&dep, base, ref)
		fonts = append(fonts, dep)
	}
	fonts = append(fonts, fontPresetDependency(fontPreset))
	return styles, assets, fonts
}

func fontPresetDependency(fontPreset string) dependency {
	dep := dependency{
		ID:        fontPreset,
		Kind:      "font_preset",
		Version:   "slidex-preset-v1",
		Retrieved: time.Now().UTC().Format(time.RFC3339),
	}
	switch fontPreset {
	case "pretendard":
	case "noto-sans-kr":
	case "noto-sans-cjk-kr":
		dep.Risk = "system/local Noto Sans CJK KR availability must be verified on the render machine"
	case "ibm-plex-sans-kr":
	case "suit":
	case "custom":
		dep.Risk = "custom font preset must be backed by brand/guidelines.md, brand/fonts, or deck_spec.json and verified during QA"
	default:
		dep.Risk = "unknown font preset; verify CSS and rendered output manually"
	}
	return dep
}

func fillDependency(dep *dependency, base, ref string) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		dep.URL = ref
		dep.Version = inferRemoteVersion(ref)
		dep.Retrieved = time.Now().UTC().Format(time.RFC3339)
		if dep.Version == "" {
			dep.Risk = "remote dependency must use an exact pinned version or be vendored locally"
		}
		return
	}
	cleanRef := strings.Split(ref, "#")[0]
	cleanRef = strings.Split(cleanRef, "?")[0]
	path := cleanRef
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, filepath.FromSlash(path))
	}
	dep.Path = path
	if hash, err := sha256File(path); err == nil {
		dep.SHA256 = hash
	} else {
		dep.Risk = "local dependency missing or unreadable: " + err.Error()
	}
}

func inferRemoteVersion(ref string) string {
	if u, err := url.Parse(ref); err == nil {
		for _, key := range []string{"version", "ver", "v"} {
			if value := u.Query().Get(key); isExactVersion(value) {
				return value
			}
		}
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`@((?:v)?[0-9]+(?:\.[0-9]+){1,3}(?:[-+][0-9A-Za-z.-]+)?)`),
		regexp.MustCompile(`/(v?[0-9]+(?:\.[0-9]+){1,3}(?:[-+][0-9A-Za-z.-]+)?)(?:/|$)`),
	}
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(ref); len(match) > 1 && isExactVersion(match[1]) {
			return match[1]
		}
	}
	return ""
}

func dependencyPinFindings(check string, deps []dependency, sourcePath string) []qaFinding {
	var findings []qaFinding
	for _, dep := range deps {
		where := dependencyLocation(dep, sourcePath)
		if dep.Version != "" && !isExactVersion(dep.Version) {
			findings = append(findings, fail(check, "dependency version must be exact, not a range or floating label: "+dep.Version, where))
		}
		if dep.URL != "" && dep.Version == "" {
			findings = append(findings, fail(check, "remote dependency must record an exact pinned version or be vendored locally", where))
		}
	}
	return findings
}

func dependencyLocation(dep dependency, fallback string) string {
	return firstNonEmpty(dep.Path, dep.URL, dep.ID, fallback)
}

func isExactVersion(version string) bool {
	v := strings.TrimSpace(version)
	if v == "" {
		return false
	}
	lower := strings.ToLower(v)
	if strings.Contains(lower, "latest") ||
		strings.Contains(lower, "unpinned") ||
		strings.Contains(lower, "range") ||
		strings.Contains(lower, "main") ||
		strings.Contains(lower, "master") ||
		strings.Contains(lower, "head") ||
		strings.Contains(lower, "nightly") ||
		strings.Contains(lower, "snapshot") ||
		strings.Contains(lower, "dev") ||
		strings.Contains(lower, "*") ||
		strings.Contains(lower, "||") ||
		strings.Contains(lower, " - ") ||
		strings.ContainsAny(lower, "<>^~") {
		return false
	}
	if regexp.MustCompile(`(^|[._-])x([._-]|$)`).MatchString(lower) {
		return false
	}
	return true
}

func hasExactVersionToken(version string) bool {
	matches := regexp.MustCompile(`\b[0-9]+(?:\.[0-9]+){2,3}\b`).FindAllString(version, -1)
	for _, match := range matches {
		if isExactVersion(match) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func writePDFFromPNGs(pdfPath string, paths []string, pageW, pageH float64) error {
	if len(paths) == 0 {
		return errors.New("no PNG files for PDF")
	}
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		return err
	}
	var objects [][]byte
	objects = append(objects, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	kids := make([]string, len(paths))
	for i := range paths {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i*3)
	}
	objects = append(objects, []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(paths))))
	for i, path := range paths {
		pageObjID := 3 + i*3
		imageObjID := pageObjID + 1
		contentObjID := pageObjID + 2
		rgb, w, h, err := pngToCompressedRGB(path)
		if err != nil {
			return err
		}
		page := fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.3f %.3f] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>", pageW, pageH, imageObjID, contentObjID)
		imageObj := streamObject(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>", w, h, len(rgb)), rgb)
		content := []byte(fmt.Sprintf("q\n%.3f 0 0 %.3f 0 0 cm\n/Im0 Do\nQ\n", pageW, pageH))
		contentObj := streamObject(fmt.Sprintf("<< /Length %d >>", len(content)), content)
		objects = append(objects, []byte(page), imageObj, contentObj)
	}
	return writePDFObjects(pdfPath, objects)
}

func pngToCompressedRGB(path string) ([]byte, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	raw := make([]byte, 0, b.Dx()*b.Dy()*3)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			raw = append(raw, byte(r>>8), byte(g>>8), byte(bl>>8))
		}
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw); err != nil {
		return nil, 0, 0, err
	}
	if err := zw.Close(); err != nil {
		return nil, 0, 0, err
	}
	return compressed.Bytes(), b.Dx(), b.Dy(), nil
}

func streamObject(dict string, data []byte) []byte {
	var b bytes.Buffer
	b.WriteString(dict)
	b.WriteString("\nstream\n")
	b.Write(data)
	b.WriteString("\nendstream")
	return b.Bytes()
}

func writePDFObjects(path string, objects [][]byte) error {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", i+1)
		b.Write(obj)
		b.WriteString("\nendobj\n")
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(objects)+1)
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func pdfPageSizePoints(width, height int) (float64, float64) {
	ratio := float64(width) / float64(height)
	h := 540.0
	w := h * ratio
	return w, h
}

func createMontage(outPath string, paths []string) (dimension, error) {
	if len(paths) == 0 {
		return dimension{}, errors.New("no PNG files for montage")
	}
	var imgs []image.Image
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return dimension{}, err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return dimension{}, err
		}
		imgs = append(imgs, img)
	}
	cols := int(math.Ceil(math.Sqrt(float64(len(imgs)))))
	rows := int(math.Ceil(float64(len(imgs)) / float64(cols)))
	thumbW := 480
	thumbH := int(math.Round(float64(thumbW) * float64(imgs[0].Bounds().Dy()) / float64(imgs[0].Bounds().Dx())))
	gap := 24
	canvasW := cols*thumbW + (cols+1)*gap
	canvasH := rows*thumbH + (rows+1)*gap
	dst := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.RGBA{R: 245, G: 247, B: 250, A: 255}}, image.Point{}, draw.Src)
	for i, img := range imgs {
		col := i % cols
		row := i / cols
		x := gap + col*(thumbW+gap)
		y := gap + row*(thumbH+gap)
		scaled := scaleNearest(img, thumbW, thumbH)
		draw.Draw(dst, image.Rect(x, y, x+thumbW, y+thumbH), scaled, image.Point{}, draw.Src)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return dimension{}, err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return dimension{}, err
	}
	defer f.Close()
	if err := png.Encode(f, dst); err != nil {
		return dimension{}, err
	}
	return dimension{Width: canvasW, Height: canvasH}, nil
}

func scaleNearest(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + y*sb.Dy()/h
		for x := 0; x < w; x++ {
			sx := sb.Min.X + x*sb.Dx()/w
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func runQA(args []string) error {
	fs := flag.NewFlagSet("qa", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	writeReport := fs.Bool("write-report", true, "write out/qa_report.md")
	visualReview := fs.String("visual-review", "codex", "visual review mode: codex, manual, or none")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return errors.New("--deck is required")
	}
	result, err := qaDeckWithVisualReview(*deck, *writeReport, *visualReview)
	if err != nil {
		return err
	}
	if err := printJSON(result); err != nil {
		return err
	}
	if result.Status == "fail" {
		return errors.New("qa failed")
	}
	return nil
}

func qaDeck(deck string, writeReport bool) (qaResult, error) {
	return qaDeckWithVisualReview(deck, writeReport, "none")
}

func qaDeckWithVisualReview(deck string, writeReport bool, visualReview string) (qaResult, error) {
	return qaDeckWithVisualReviewRunner(deck, writeReport, visualReview, runVisualReview)
}

func qaDeckWithAppServerVisualReview(deck string, writeReport bool, visualReview string, appRun *appServerWorkflowRun) (qaResult, error) {
	runner := func(deckAbs string, manifest renderManifest, mode string) (string, []qaFinding) {
		if strings.EqualFold(strings.TrimSpace(mode), "codex") && appRun != nil {
			return runAppServerVisualReview(deckAbs, manifest, appRun)
		}
		return runVisualReview(deckAbs, manifest, mode)
	}
	return qaDeckWithVisualReviewRunner(deck, writeReport, visualReview, runner)
}

func qaDeckWithVisualReviewRunner(deck string, writeReport bool, visualReview string, visualRunner func(string, renderManifest, string) (string, []qaFinding)) (qaResult, error) {
	deckAbs := mustAbs(deck)
	outDir := filepath.Join(deckAbs, "out")
	specPath := filepath.Join(outDir, "deck_spec.json")
	htmlPath := filepath.Join(outDir, "final_deck.html")
	manifestPath := filepath.Join(outDir, "render_manifest.json")
	pdfPath := filepath.Join(outDir, "final_deck.pdf")
	montagePath := filepath.Join(outDir, "qa_montage.png")
	renderedDir := filepath.Join(outDir, "rendered_slides")

	result := qaResult{
		ToolName:         toolName,
		Version:          toolVersion,
		DeckDir:          deckAbs,
		RuntimeMode:      "deterministic",
		VisualReviewMode: visualReview,
	}
	if mode, reason := qaRuntimeForDeck(deckAbs); mode != "" {
		result.RuntimeMode = mode
		result.RuntimeReason = reason
	}
	for _, p := range []string{specPath, htmlPath, manifestPath, pdfPath, montagePath} {
		if _, err := os.Stat(p); err != nil {
			result.Findings = append(result.Findings, fail("file.exists", "missing required file", p))
		} else {
			result.FilesChecked = append(result.FilesChecked, p)
		}
	}
	if findings, err := validateSpecFile(specPath); err == nil {
		result.Findings = append(result.Findings, findings...)
	} else {
		result.Findings = append(result.Findings, fail("schema", err.Error(), specPath))
	}

	var htmlSlides []slideInfo
	if raw, err := os.ReadFile(htmlPath); err == nil {
		htmlString := string(raw)
		htmlLower := strings.ToLower(htmlString)
		if !strings.Contains(htmlLower, "<!doctype html") {
			result.Findings = append(result.Findings, fail("html.doctype", "HTML must include <!doctype html>", htmlPath))
		}
		if !regexp.MustCompile(`(?is)<html\b[^>]*\blang\s*=`).MatchString(htmlString) {
			result.Findings = append(result.Findings, fail("html.lang", "HTML root must declare lang", htmlPath))
		}
		if !strings.Contains(htmlLower, "font-family") {
			result.Findings = append(result.Findings, qaFinding{Severity: "warn", Check: "html.font", Message: "HTML/CSS does not explicitly declare font-family", Path: htmlPath})
		}
		htmlSlides = extractSlides(htmlString)
		result.SlideCount = len(htmlSlides)
		if len(htmlSlides) == 0 {
			result.Findings = append(result.Findings, fail("html.slides", "no .slide elements found", htmlPath))
		}
		htmlDeps := localDependencies(htmlPath, htmlString)
		for _, dep := range htmlDeps {
			if dep.Risk != "" {
				result.Findings = append(result.Findings, qaFinding{Severity: "warn", Check: "html.dependency", Message: dep.Risk, Path: dep.Path})
			}
		}
		result.Findings = append(result.Findings, dependencyPinFindings("html.dependency_pin", htmlDeps, htmlPath)...)
		if !strings.Contains(htmlString, "word-break: keep-all") {
			result.Findings = append(result.Findings, qaFinding{Severity: "warn", Check: "html.korean_wrapping", Message: "CSS does not explicitly include word-break: keep-all", Path: htmlPath})
		}
	} else {
		result.Findings = append(result.Findings, fail("html.read", err.Error(), htmlPath))
	}

	pngs, _ := filepath.Glob(filepath.Join(renderedDir, "slide_*.png"))
	sort.Strings(pngs)
	if len(pngs) == 0 {
		result.Findings = append(result.Findings, fail("rendered_slides.exists", "no rendered slide PNGs found", renderedDir))
	}
	if len(htmlSlides) > 0 && len(pngs) != len(htmlSlides) {
		result.Findings = append(result.Findings, fail("parity.html_png", fmt.Sprintf("HTML slide count %d does not match PNG count %d", len(htmlSlides), len(pngs)), renderedDir))
	}
	expected := dimension{}
	var manifest renderManifest
	if raw, err := os.ReadFile(manifestPath); err == nil {
		if err := json.Unmarshal(raw, &manifest); err != nil {
			result.Findings = append(result.Findings, fail("manifest.parse", err.Error(), manifestPath))
		} else {
			expected = manifest.ExpectedDimensions
			if currentHash, err := sha256File(htmlPath); err == nil && currentHash != manifest.SourceHTML.SHA256 {
				result.Findings = append(result.Findings, fail("manifest.freshness", "current HTML hash does not match render manifest", htmlPath))
			}
			result.RenderMethod = manifest.RenderMethod
			if !hasExactVersionToken(manifest.ChromeVersion) {
				result.Findings = append(result.Findings, fail("runtime.chrome_version", "render manifest must record an exact Chrome/Chromium version", manifestPath))
			}
			if manifest.PDFPageSizePoints.Width <= 0 || manifest.PDFPageSizePoints.Height <= 0 {
				result.Findings = append(result.Findings, fail("pdf.page_size", "render manifest is missing PDF page size", manifestPath))
			}
			if manifest.FontPreset == "" {
				result.Findings = append(result.Findings, qaFinding{Severity: "warn", Check: "font.preset", Message: "render manifest does not record a font preset", Path: manifestPath})
			}
			if manifest.ChromeSandbox == "disabled" {
				result.Findings = append(result.Findings, qaFinding{Severity: "warn", Check: "runtime.chrome_sandbox", Message: "Chrome ran with --no-sandbox: " + manifest.ChromeNoSandboxReason, Path: manifestPath})
			} else if manifest.ChromeSandbox == "" {
				result.Findings = append(result.Findings, fail("runtime.chrome_sandbox", "render manifest must record chromeSandbox", manifestPath))
			}
			if manifest.SlideEnumerationMethod == "" {
				result.Findings = append(result.Findings, fail("render.slide_enumeration", "render manifest must record slide enumeration method", manifestPath))
			}
			result.Findings = append(result.Findings, dependencyPinFindings("manifest.stylesheet_pin", manifest.Stylesheets, manifestPath)...)
			result.Findings = append(result.Findings, dependencyPinFindings("manifest.asset_pin", manifest.Assets, manifestPath)...)
			result.Findings = append(result.Findings, dependencyPinFindings("manifest.font_pin", manifest.Fonts, manifestPath)...)
		}
	}
	for _, p := range pngs {
		dim, blank, err := validatePNG(p, coalesceInt(expected.Width, 1920), coalesceInt(expected.Height, 1080))
		if err != nil {
			result.Findings = append(result.Findings, fail("png.valid", err.Error(), p))
			continue
		}
		if blank {
			result.Findings = append(result.Findings, fail("png.blank", "rendered slide appears blank", p))
		}
		_ = dim
		result.FilesChecked = append(result.FilesChecked, p)
	}
	if pages, err := countPDFPages(pdfPath); err == nil {
		result.PDFPageCount = pages
		if len(htmlSlides) > 0 && pages != len(htmlSlides) {
			result.Findings = append(result.Findings, fail("parity.html_pdf", fmt.Sprintf("HTML slide count %d does not match PDF page count %d", len(htmlSlides), pages), pdfPath))
		}
	} else {
		result.Findings = append(result.Findings, fail("pdf.read", err.Error(), pdfPath))
	}

	if hasFailures(result.Findings) {
		result.Status = "fail"
	} else if hasWarnings(result.Findings) {
		result.Status = "pass_with_risks"
	} else {
		result.Status = "pass"
	}
	if err := writeVisualReviewImageSet(filepath.Join(outDir, "visual_reviews", "image_set.json"), manifest); err != nil {
		result.Findings = append(result.Findings, fail("visual_review.image_set", err.Error(), filepath.Join(outDir, "visual_reviews", "image_set.json")))
		result.Status = "fail"
	}
	visualStatus, visualFindings := visualRunner(deckAbs, manifest, visualReview)
	result.VisualStatus = visualStatus
	result.Findings = append(result.Findings, visualFindings...)
	if hasFailures(result.Findings) {
		result.Status = "fail"
	} else if hasWarnings(result.Findings) {
		result.Status = "pass_with_risks"
	} else {
		result.Status = "pass"
	}
	if writeReport {
		reportPath := filepath.Join(outDir, "qa_report.md")
		if err := writeQAReport(reportPath, result); err != nil {
			return result, err
		}
		result.GeneratedReport = reportPath
	}
	return result, nil
}

func localDependencies(htmlPath, src string) []dependency {
	styles, assets, _ := collectDependencies(htmlPath, src, "pretendard")
	return append(styles, assets...)
}

func writeQAReport(path string, result qaResult) error {
	var b strings.Builder
	outDir := filepath.Dir(path)
	htmlPath := filepath.Join(outDir, "final_deck.html")
	manifestPath := filepath.Join(outDir, "render_manifest.json")
	pngSetHash := hashFileSet(filepath.Join(outDir, "rendered_slides", "slide_*.png"))
	b.WriteString("# QA Report\n\n")
	b.WriteString("```yaml\n")
	b.WriteString("slidexQaReport:\n")
	b.WriteString("  schemaVersion: slidex.qaReport.v1\n")
	b.WriteString("  generatedAt: " + time.Now().UTC().Format(time.RFC3339) + "\n")
	b.WriteString("  htmlSha256: " + firstNonEmpty(mustSHA256(htmlPath), "missing") + "\n")
	b.WriteString("  renderManifestSha256: " + firstNonEmpty(mustSHA256(manifestPath), "missing") + "\n")
	b.WriteString("  pngSetSha256: " + firstNonEmpty(pngSetHash, "missing") + "\n")
	b.WriteString("  deterministicStatus: " + result.Status + "\n")
	b.WriteString("  runtimeMode: " + firstNonEmpty(result.RuntimeMode, "deterministic") + "\n")
	if result.RuntimeReason != "" {
		b.WriteString("  runtimeReason: " + quoteYAMLScalar(result.RuntimeReason) + "\n")
	}
	b.WriteString("  visualStatus: " + firstNonEmpty(result.VisualStatus, "not_run") + "\n")
	b.WriteString("  visualReviewMode: " + firstNonEmpty(result.VisualReviewMode, "none") + "\n")
	b.WriteString("```\n\n")
	b.WriteString(fmt.Sprintf("- Tool: `%s %s`\n", result.ToolName, result.Version))
	b.WriteString(fmt.Sprintf("- Deck directory: `%s`\n", result.DeckDir))
	b.WriteString(fmt.Sprintf("- Overall status: %s\n", result.Status))
	b.WriteString(fmt.Sprintf("- Runtime mode: `%s`\n", firstNonEmpty(result.RuntimeMode, "deterministic")))
	b.WriteString(fmt.Sprintf("- Render method: %s\n", firstNonEmpty(result.RenderMethod, "not recorded in render_manifest.json")))
	b.WriteString(fmt.Sprintf("- Slide count: %d\n", result.SlideCount))
	b.WriteString(fmt.Sprintf("- PDF page count: %d\n\n", result.PDFPageCount))
	b.WriteString("## Files Checked\n\n")
	for _, f := range result.FilesChecked {
		b.WriteString(fmt.Sprintf("- `%s`\n", f))
	}
	b.WriteString("\n## Findings\n\n")
	if len(result.Findings) == 0 {
		b.WriteString("- No automated findings. Visual inspection is still required before final delivery.\n")
	} else {
		for _, f := range result.Findings {
			b.WriteString(fmt.Sprintf("- `%s` `%s`: %s", f.Severity, f.Check, f.Message))
			if f.Path != "" {
				b.WriteString(fmt.Sprintf(" (`%s`)", f.Path))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## Slide-By-Slide Findings\n\n")
	if result.SlideCount == 0 {
		b.WriteString("- No HTML slides detected.\n")
	} else {
		for i := 1; i <= result.SlideCount; i++ {
			b.WriteString(fmt.Sprintf("- Slide %02d: automated render/spec parity checks completed; manual visual inspection required.\n", i))
		}
	}
	b.WriteString("\n## Business Logic Findings\n\n")
	writeFindingsForPrefix(&b, result.Findings, []string{"schema.", "claimProvenance.", "business"})
	b.WriteString("\n## Claim Provenance Findings\n\n")
	writeFindingsForPrefix(&b, result.Findings, []string{"claimProvenance.", "schema.claim"})
	b.WriteString("\n## Visual And Accessibility Findings\n\n")
	writeFindingsForPrefix(&b, result.Findings, []string{"html.", "png.", "font.", "manifest.", "parity.html_png"})
	b.WriteString("\n## PDF Findings\n\n")
	writeFindingsForPrefix(&b, result.Findings, []string{"pdf.", "parity.html_pdf"})
	b.WriteString("\n## User-Edit Sync Findings\n\n")
	writeFindingsForPrefix(&b, result.Findings, []string{"sync.", "package.manifest_freshness"})
	b.WriteString("\n## Required Revisions\n\n")
	if hasFailures(result.Findings) {
		b.WriteString("- Resolve all `fail` findings above, re-render from current HTML, rebuild PDF and montage, and rerun QA.\n")
	} else {
		b.WriteString("- No automated blocking revisions. Complete manual visual/business review before final delivery.\n")
	}
	b.WriteString("\n## Unresolved Risks\n\n")
	if hasWarnings(result.Findings) {
		b.WriteString("- Review all `warn` findings above and decide whether to fix or explicitly accept them.\n")
	} else {
		b.WriteString("- Manual visual inspection, business meaning review, and claim provenance review remain required workflow gates.\n")
	}
	b.WriteString("\n## Visual Inspection\n\n")
	b.WriteString("- Manual inspection of rendered slides and montage must be recorded by the Codex workflow before final delivery.\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func qaRuntimeForDeck(deckAbs string) (string, string) {
	raw, err := os.ReadFile(filepath.Join(deckAbs, "out", "slidex_state.json"))
	if err != nil {
		return "", ""
	}
	var state slidexState
	if json.Unmarshal(raw, &state) != nil {
		return "", ""
	}
	return state.CodexRuntime.Mode, state.CodexRuntime.Reason
}

func quoteYAMLScalar(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func writeFindingsForPrefix(b *strings.Builder, findings []qaFinding, prefixes []string) {
	wrote := false
	for _, f := range findings {
		for _, prefix := range prefixes {
			if strings.HasPrefix(f.Check, prefix) {
				b.WriteString(fmt.Sprintf("- `%s` `%s`: %s", f.Severity, f.Check, f.Message))
				if f.Path != "" {
					b.WriteString(fmt.Sprintf(" (`%s`)", f.Path))
				}
				b.WriteString("\n")
				wrote = true
				break
			}
		}
	}
	if !wrote {
		b.WriteString("- No automated findings in this category.\n")
	}
}

func runSyncHTMLEdits(args []string) error {
	fs := flag.NewFlagSet("sync-html-edits", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	width := fs.Int("width", 1920, "render width")
	height := fs.Int("height", 1080, "render height")
	fontPreset := fs.String("font-preset", "pretendard", "font preset")
	chromePath := fs.String("chrome", "", "Chrome/Chromium binary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return errors.New("--deck is required")
	}
	result, err := syncHTMLEdits(*deck, *width, *height, *fontPreset, *chromePath)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func syncHTMLEdits(deck string, width, height int, fontPreset, chromePath string) (map[string]any, error) {
	deckAbs := mustAbs(deck)
	outDir := filepath.Join(deckAbs, "out")
	htmlPath := filepath.Join(outDir, "final_deck.html")
	baselinePath := filepath.Join(outDir, "final_deck.generated_baseline.html")
	specPath := filepath.Join(outDir, "deck_spec.json")
	notesPath := filepath.Join(outDir, "notes.md")
	syncPath := filepath.Join(outDir, "html_edit_sync.md")

	currentRaw, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil, err
	}
	currentHash := sha256Bytes(currentRaw)
	baseRaw, baseErr := os.ReadFile(baselinePath)
	baseHash := ""
	if baseErr == nil {
		baseHash = sha256Bytes(baseRaw)
	}
	previousBaselineHash := baseHash
	newBaselineHash := baseHash
	currentSlides := extractSlides(string(currentRaw))
	baselineSource := baselinePath
	baselineSlides := extractSlides(string(baseRaw))
	if baseErr != nil {
		baselineSource = specPath
		baselineSlides = slidesFromSpec(specPath)
	}
	changes := compareSlides(baselineSlides, currentSlides)
	if baseErr == nil {
		changes = append(changes, compareHTMLMetadata(baselinePath, string(baseRaw), htmlPath, string(currentRaw))...)
	} else {
		changes = append(changes, compareHTMLMetadata("", "", htmlPath, string(currentRaw))...)
	}
	if baseErr != nil {
		changes = append(changes, "No generated baseline was available; compared HTML against deck_spec.json.")
	}
	changes = uniqueStrings(changes)

	backupPath := ""
	specBackupPath := ""
	notesBackupPath := ""
	var acceptedChanges []string
	var correctedOrRejected []string
	var derivativeUpdated []string
	var derivativeStale []string
	changeDetected := currentHash != baseHash || baseErr != nil
	if currentHash != baseHash {
		backupPath = filepath.Join(outDir, "final_deck.pre_sync_"+time.Now().Format("20060102_150405")+".html")
		if err := copyFile(htmlPath, backupPath); err != nil {
			return nil, err
		}
		specBackupPath = filepath.Join(outDir, "deck_spec.pre_sync_"+time.Now().Format("20060102_150405")+".json")
		if _, err := os.Stat(specPath); err == nil {
			if err := copyFile(specPath, specBackupPath); err != nil {
				return nil, err
			}
		}
		notesBackupPath = filepath.Join(outDir, "notes.pre_sync_"+time.Now().Format("20060102_150405")+".md")
		if _, err := os.Stat(notesPath); err == nil {
			if err := copyFile(notesPath, notesBackupPath); err != nil {
				return nil, err
			}
		}
		if err := updateSpecFromHTML(specPath, currentSlides); err != nil {
			return nil, err
		}
		if err := appendNotes(notesPath, "HTML edit sync", changes); err != nil {
			return nil, err
		}
		derivativeUpdated = append(derivativeUpdated, "deck_spec.json", "notes.md")
	}

	renderStatus := "not_needed"
	qaStatus := "not_run"
	var renderErr string
	var qaErr string
	if changeDetected {
		cfg, err := renderConfigFromFlags(htmlPath, filepath.Join(outDir, "rendered_slides"), filepath.Join(outDir, "final_deck.pdf"), filepath.Join(outDir, "render_manifest.json"), "paginated", ".slide", width, height, fontPreset, chromePath, false)
		if err != nil {
			renderStatus = "failed"
			renderErr = err.Error()
		} else if _, err := renderHTML(cfg); err != nil {
			renderStatus = "failed"
			renderErr = err.Error()
		} else {
			renderStatus = "completed"
			derivativeUpdated = append(derivativeUpdated, "rendered_slides/*.png", "final_deck.pdf", "render_manifest.json", "qa_montage.png")
			if qa, err := qaDeck(deckAbs, true); err != nil && qa.Status == "fail" {
				qaStatus = qa.Status
				qaErr = err.Error()
			} else if err != nil {
				qaStatus = "failed"
				qaErr = err.Error()
			} else {
				qaStatus = qa.Status
			}
			if qaStatus == "pass" || qaStatus == "pass_with_risks" {
				acceptedChanges = changes
				derivativeUpdated = append(derivativeUpdated, "qa_report.md", "final_deck.generated_baseline.html")
				if err := copyFile(htmlPath, baselinePath); err != nil {
					return nil, err
				}
				baseRaw, _ = os.ReadFile(baselinePath)
				newBaselineHash = sha256Bytes(baseRaw)
			} else {
				correctedOrRejected = append(correctedOrRejected, "HTML edits were not accepted into the generated baseline because render or QA did not pass.")
				if specBackupPath != "" {
					_ = copyFile(specBackupPath, specPath)
				}
				if notesBackupPath != "" {
					_ = copyFile(notesBackupPath, notesPath)
				}
			}
		}
	}
	if len(acceptedChanges) == 0 && changeDetected && len(correctedOrRejected) == 0 {
		correctedOrRejected = append(correctedOrRejected, "HTML edits require review because render or QA did not complete.")
	}
	if containsMeaningChange(changes) {
		derivativeStale = append(derivativeStale, "brief.md: HTML text changed; confirm whether brief facts, audience, objective, or desired outcome changed.")
		derivativeStale = append(derivativeStale, "strategy.md: HTML story or slide structure changed; strategy may need a human update.")
		derivativeStale = append(derivativeStale, "source_inventory.md: asset, dependency, or evidence references may need refresh.")
		derivativeStale = append(derivativeStale, "delivery_summary.md: final delivery summary must be regenerated after accepted edits.")
	}
	if changeDetected {
		if err := appendSyncFindingsToQAReport(filepath.Join(outDir, "qa_report.md"), derivativeStale, acceptedChanges, correctedOrRejected); err != nil {
			qaErr = strings.TrimSpace(qaErr + "; failed to append sync findings to QA report: " + err.Error())
		} else if len(derivativeStale) > 0 {
			derivativeUpdated = append(derivativeUpdated, "qa_report.md")
		}
	}
	report := map[string]any{
		"toolName":             toolName,
		"version":              toolVersion,
		"deckDir":              deckAbs,
		"syncReport":           syncPath,
		"comparisonSource":     baselineSource,
		"currentHtmlHash":      currentHash,
		"previousBaselineHash": previousBaselineHash,
		"newBaselineHash":      newBaselineHash,
		"changes":              changes,
		"acceptedChanges":      acceptedChanges,
		"correctedOrRejected":  correctedOrRejected,
		"derivativeUpdated":    uniqueStrings(derivativeUpdated),
		"derivativeStale":      uniqueStrings(derivativeStale),
		"backup":               backupPath,
		"renderStatus":         renderStatus,
		"renderError":          renderErr,
		"qaStatus":             qaStatus,
		"qaError":              qaErr,
	}
	if err := writeSyncReport(syncPath, report); err != nil {
		return nil, err
	}
	return report, nil
}

func compareSlides(oldSlides, newSlides []slideInfo) []string {
	var changes []string
	oldByID := map[string]slideInfo{}
	newByID := map[string]slideInfo{}
	for _, s := range oldSlides {
		oldByID[s.ID] = s
	}
	for _, s := range newSlides {
		newByID[s.ID] = s
	}
	if len(oldSlides) != len(newSlides) {
		changes = append(changes, fmt.Sprintf("Slide count changed from %d to %d.", len(oldSlides), len(newSlides)))
	}
	for i, s := range newSlides {
		if i >= len(oldSlides) || oldSlides[i].ID != s.ID {
			changes = append(changes, "Slide order changed.")
			break
		}
	}
	for _, s := range newSlides {
		old, ok := oldByID[s.ID]
		if !ok {
			changes = append(changes, "Added slide: "+s.ID)
			continue
		}
		if old.Headline != s.Headline {
			changes = append(changes, fmt.Sprintf("Headline changed on %s: %q -> %q", s.ID, old.Headline, s.Headline))
		}
		if old.Text != s.Text {
			changes = append(changes, "Body or visual text changed on "+s.ID)
		}
	}
	for _, s := range oldSlides {
		if _, ok := newByID[s.ID]; !ok {
			changes = append(changes, "Removed slide: "+s.ID)
		}
	}
	if len(changes) == 0 {
		changes = append(changes, "No HTML changes detected against baseline.")
	}
	return uniqueStrings(changes)
}

func slidesFromSpec(specPath string) []slideInfo {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil
	}
	arr, _ := spec["slides"].([]any)
	slides := make([]slideInfo, 0, len(arr))
	for _, item := range arr {
		obj, _ := item.(map[string]any)
		id, _ := obj["htmlId"].(string)
		if id == "" {
			id, _ = obj["id"].(string)
		}
		headline, _ := obj["headline"].(string)
		text := headline
		if body, ok := obj["bodyContent"].([]any); ok {
			for _, part := range body {
				text += " " + fmt.Sprint(part)
			}
		}
		slides = append(slides, slideInfo{ID: id, Headline: headline, Text: normalizeText(text)})
	}
	return slides
}

func compareHTMLMetadata(oldPath, oldHTML, newPath, newHTML string) []string {
	var changes []string
	if oldHTML != "" {
		oldS, oldA, oldF := collectDependencies(oldPath, oldHTML, "unknown")
		newS, newA, newF := collectDependencies(newPath, newHTML, "unknown")
		changes = append(changes, compareDependencySet("stylesheet", dependencySignatures(oldS), dependencySignatures(newS))...)
		changes = append(changes, compareDependencySet("asset", dependencySignatures(oldA), dependencySignatures(newA))...)
		changes = append(changes, compareDependencySet("font", dependencySignatures(oldF), dependencySignatures(newF))...)
		if oldCount, newCount := countQARequired(oldHTML), countQARequired(newHTML); newCount < oldCount {
			changes = append(changes, fmt.Sprintf("Removed QA-required elements: count changed from %d to %d.", oldCount, newCount))
		}
	} else {
		_, assets, fonts := collectDependencies(newPath, newHTML, "unknown")
		if len(assets) > 0 {
			changes = append(changes, fmt.Sprintf("Asset dependencies present in current HTML: %d.", len(assets)))
		}
		if len(fonts) > 0 {
			changes = append(changes, fmt.Sprintf("Font dependencies present in current HTML: %d.", len(fonts)))
		}
	}
	if oldFont, newFont := extractFontFamilies(oldHTML), extractFontFamilies(newHTML); oldHTML != "" && oldFont != newFont {
		changes = append(changes, "Font-family declarations changed.")
	}
	return changes
}

func dependencySignatures(deps []dependency) map[string]bool {
	out := map[string]bool{}
	for _, dep := range deps {
		key := dep.Kind + "|" + dep.ID + "|" + dep.Path + "|" + dep.URL + "|" + dep.SHA256
		out[key] = true
	}
	return out
}

func compareDependencySet(kind string, oldSet, newSet map[string]bool) []string {
	var changes []string
	for key := range newSet {
		if !oldSet[key] {
			changes = append(changes, strings.Title(kind)+" dependency added or changed: "+key)
		}
	}
	for key := range oldSet {
		if !newSet[key] {
			changes = append(changes, strings.Title(kind)+" dependency removed or changed: "+key)
		}
	}
	return changes
}

func countQARequired(html string) int {
	return strings.Count(html, "data-qa-required") + strings.Count(html, "data-claim-id") + strings.Count(html, "aria-label")
}

func extractFontFamilies(html string) string {
	re := regexp.MustCompile(`(?is)font-family\s*:\s*([^;}]+)`)
	matches := re.FindAllStringSubmatch(html, -1)
	var families []string
	for _, m := range matches {
		families = append(families, normalizeText(m[1]))
	}
	sort.Strings(families)
	return strings.Join(families, "|")
}

func containsMeaningChange(changes []string) bool {
	for _, c := range changes {
		lower := strings.ToLower(c)
		if strings.Contains(lower, "headline") ||
			strings.Contains(lower, "body") ||
			strings.Contains(lower, "slide count") ||
			strings.Contains(lower, "slide order") ||
			strings.Contains(lower, "asset") ||
			strings.Contains(lower, "font") ||
			strings.Contains(lower, "dependency") {
			return true
		}
	}
	return false
}

func updateSpecFromHTML(specPath string, slides []slideInfo) error {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		return err
	}
	oldSlides := map[string]map[string]any{}
	if arr, ok := spec["slides"].([]any); ok {
		for _, rawSlide := range arr {
			if s, ok := rawSlide.(map[string]any); ok {
				if id, _ := s["id"].(string); id != "" {
					oldSlides[id] = s
				}
			}
		}
	}
	var newSlides []any
	for i, htmlSlide := range slides {
		id := htmlSlide.ID
		if id == "" {
			id = fmt.Sprintf("slide_%02d", i+1)
		}
		slide := oldSlides[id]
		if slide == nil {
			slide = map[string]any{}
		}
		slide["id"] = id
		slide["htmlId"] = id
		if slide["sectionRole"] == nil || slide["sectionRole"] == "" {
			slide["sectionRole"] = "content"
		}
		if htmlSlide.Headline != "" {
			slide["headline"] = htmlSlide.Headline
			if slide["keyMessage"] == nil || slide["keyMessage"] == "" {
				slide["keyMessage"] = htmlSlide.Headline
			}
		}
		slide["bodyContent"] = splitBodyContent(htmlSlide.Text, htmlSlide.Headline)
		for _, key := range []string{"layoutIntent", "visualIntent"} {
			if slide[key] == nil {
				slide[key] = "Updated from approved HTML during sync."
			}
		}
		for _, key := range []string{"evidenceRefs", "claims", "renderRisks", "qaChecks"} {
			if slide[key] == nil {
				slide[key] = []any{}
			}
		}
		newSlides = append(newSlides, slide)
	}
	spec["slides"] = newSlides
	return writeJSONFile(specPath, spec)
}

func splitBodyContent(text, headline string) []any {
	text = strings.TrimSpace(strings.TrimPrefix(text, headline))
	if text == "" {
		return []any{}
	}
	parts := strings.Split(text, ". ")
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = append(out, text)
	}
	return out
}

func appendNotes(path, heading string, lines []string) error {
	var b strings.Builder
	if existing, err := os.ReadFile(path); err == nil {
		b.Write(existing)
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## " + heading + "\n\n")
	b.WriteString("- Sync date: " + time.Now().UTC().Format(time.RFC3339) + "\n")
	for _, line := range lines {
		b.WriteString("- " + line + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeSyncReport(path string, report map[string]any) error {
	var b strings.Builder
	b.WriteString("# HTML Edit Sync\n\n")
	b.WriteString(fmt.Sprintf("- Sync date: %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Tool: `%s %s`\n", toolName, toolVersion))
	b.WriteString(fmt.Sprintf("- Comparison source: `%s`\n", report["comparisonSource"]))
	b.WriteString(fmt.Sprintf("- Current HTML hash: `%s`\n", report["currentHtmlHash"]))
	b.WriteString(fmt.Sprintf("- Previous baseline hash: `%s`\n", report["previousBaselineHash"]))
	b.WriteString(fmt.Sprintf("- New baseline hash: `%s`\n", report["newBaselineHash"]))
	if backup, _ := report["backup"].(string); backup != "" {
		b.WriteString(fmt.Sprintf("- Backup: `%s`\n", backup))
	}
	b.WriteString(fmt.Sprintf("- Render status: `%s`\n", report["renderStatus"]))
	if errText, _ := report["renderError"].(string); errText != "" {
		b.WriteString(fmt.Sprintf("- Render error: %s\n", errText))
	}
	b.WriteString(fmt.Sprintf("- QA status: `%s`\n", report["qaStatus"]))
	if errText, _ := report["qaError"].(string); errText != "" {
		b.WriteString(fmt.Sprintf("- QA error: %s\n", errText))
	}
	b.WriteString("\n## Detected Changes\n\n")
	if changes, ok := report["changes"].([]string); ok {
		for _, c := range changes {
			b.WriteString("- " + c + "\n")
		}
	}
	b.WriteString("\n## Accepted Changes\n\n")
	if changes, ok := report["acceptedChanges"].([]string); ok {
		for _, c := range changes {
			b.WriteString("- " + c + "\n")
		}
	}
	b.WriteString("\n## Corrected Or Rejected Changes\n\n")
	if changes, ok := report["correctedOrRejected"].([]string); ok && len(changes) > 0 {
		for _, c := range changes {
			b.WriteString("- " + c + "\n")
		}
	} else {
		b.WriteString("- None recorded by automated sync. QA findings must be reviewed before final delivery.\n")
	}
	b.WriteString("\n## Derivative Files\n\n")
	b.WriteString("### Updated\n\n")
	if files, ok := report["derivativeUpdated"].([]string); ok && len(files) > 0 {
		for _, f := range files {
			b.WriteString("- `" + f + "`\n")
		}
	} else {
		b.WriteString("- None\n")
	}
	b.WriteString("\n### Marked Stale\n\n")
	if files, ok := report["derivativeStale"].([]string); ok && len(files) > 0 {
		for _, f := range files {
			b.WriteString("- " + f + "\n")
		}
	} else {
		b.WriteString("- None\n")
	}
	b.WriteString("\n## Remaining Risks\n\n")
	b.WriteString("- Manual review is required for business meaning changes, claim provenance, and visual inspection of the regenerated montage.\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func appendSyncFindingsToQAReport(path string, stale []string, accepted []string, corrected []string) error {
	var b strings.Builder
	if existing, err := os.ReadFile(path); err == nil {
		b.Write(existing)
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## User-Edit Sync Findings\n\n")
	if len(accepted) > 0 {
		b.WriteString("### Accepted Changes\n\n")
		for _, item := range accepted {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(corrected) > 0 {
		b.WriteString("\n### Corrected Or Rejected Changes\n\n")
		for _, item := range corrected {
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("\n### Stale Derivative Files\n\n")
	if len(stale) == 0 {
		b.WriteString("- None\n")
	} else {
		for _, item := range stale {
			b.WriteString("- " + item + "\n")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func runPackage(args []string) error {
	fs := flag.NewFlagSet("package", flag.ContinueOnError)
	deck := fs.String("deck", "", "deck workspace directory")
	includeLogs := fs.Bool("include-logs", false, "include only sanitized log excerpts in package verification")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *deck == "" {
		return errors.New("--deck is required")
	}
	result, err := packageDeck(*deck, *includeLogs)
	if err != nil {
		return err
	}
	if err := printJSON(result); err != nil {
		return err
	}
	if status, _ := result["status"].(string); status == "fail" {
		if findings, ok := result["findings"].([]qaFinding); ok && packageHasStaleFinding(findings) {
			return exitCodeError(5, "package verification failed: stale artifacts")
		}
		return errors.New("package verification failed")
	} else if status == "pass_with_risks" {
		return exitCodeError(6, "package verification has unresolved or unaccepted risks")
	}
	return nil
}

func packageDeck(deck string, includeLogs bool) (map[string]any, error) {
	deckAbs := mustAbs(deck)
	outDir := filepath.Join(deckAbs, "out")
	required := []string{
		"strategy.md",
		"deck_spec.json",
		"final_deck.html",
		"final_deck.generated_baseline.html",
		"final_deck.pdf",
		"render_manifest.json",
		"qa_montage.png",
		"qa_report.md",
		"notes.md",
		"delivery_summary.md",
	}
	var findings []qaFinding
	for _, rel := range required {
		path := filepath.Join(outDir, rel)
		if _, err := os.Stat(path); err != nil {
			findings = append(findings, fail("package.required_file", "missing required delivery file", path))
		}
	}
	pngs, _ := filepath.Glob(filepath.Join(outDir, "rendered_slides", "slide_*.png"))
	sort.Strings(pngs)
	if len(pngs) == 0 {
		findings = append(findings, fail("package.rendered_slides", "missing rendered slide images", filepath.Join(outDir, "rendered_slides")))
	}
	manifestPath := filepath.Join(outDir, "render_manifest.json")
	specPath := filepath.Join(outDir, "deck_spec.json")
	htmlPath := filepath.Join(outDir, "final_deck.html")
	qaReportPath := filepath.Join(outDir, "qa_report.md")
	deliverySummaryPath := filepath.Join(outDir, "delivery_summary.md")
	visualImageSetPath := filepath.Join(outDir, "visual_reviews", "image_set.json")
	visualReviewPath := filepath.Join(outDir, "visual_reviews", "latest_review.json")
	structuredReviewPaths := []string{}
	for _, stage := range structuredReviewStages() {
		structuredReviewPaths = append(structuredReviewPaths, filepath.Join(outDir, "agent_reviews", "round_01", "reviewer_"+stage+".json"))
	}
	findings = append(findings, verifyPackageSpec(specPath)...)
	if raw, err := os.ReadFile(manifestPath); err == nil {
		var manifest renderManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			findings = append(findings, fail("package.manifest_parse", err.Error(), manifestPath))
		} else if hash, err := sha256File(htmlPath); err == nil && hash != manifest.SourceHTML.SHA256 {
			findings = append(findings, fail("package.manifest_freshness", "manifest source HTML hash is stale", manifestPath))
		} else {
			if !hasExactVersionToken(manifest.ChromeVersion) {
				findings = append(findings, fail("package.runtime_chrome_version", "manifest must record an exact Chrome/Chromium version", manifestPath))
			}
			if currentHTML, err := os.ReadFile(htmlPath); err == nil {
				currentStyles, currentAssets, currentFonts := collectDependencies(htmlPath, string(currentHTML), manifest.FontPreset)
				findings = append(findings, verifyManifestDependencies("stylesheet", manifest.Stylesheets, currentStyles, manifestPath)...)
				findings = append(findings, verifyManifestDependencies("asset", manifest.Assets, currentAssets, manifestPath)...)
				findings = append(findings, verifyManifestDependencies("font", manifest.Fonts, currentFonts, manifestPath)...)
			}
			if len(manifest.PNGFiles) != len(pngs) {
				findings = append(findings, fail("package.png_count", fmt.Sprintf("manifest PNG count %d does not match files %d", len(manifest.PNGFiles), len(pngs)), manifestPath))
			}
			for _, img := range manifest.PNGFiles {
				if hash, err := sha256File(img.Path); err != nil {
					findings = append(findings, fail("package.png_hash", "manifest PNG file missing: "+err.Error(), img.Path))
				} else if hash != img.SHA256 {
					findings = append(findings, fail("package.png_hash", "PNG hash does not match manifest", img.Path))
				}
				if dim, blank, err := validatePNG(img.Path, manifest.ExpectedDimensions.Width, manifest.ExpectedDimensions.Height); err != nil {
					findings = append(findings, fail("package.png_valid", err.Error(), img.Path))
				} else if blank {
					findings = append(findings, fail("package.png_blank", "rendered PNG appears blank", img.Path))
				} else if dim.Width != img.Dimensions.Width || dim.Height != img.Dimensions.Height {
					findings = append(findings, fail("package.png_dimensions", "PNG dimensions differ from manifest", img.Path))
				}
			}
			if hash, err := sha256File(manifest.PDF.Path); err != nil {
				findings = append(findings, fail("package.pdf_hash", "manifest PDF missing: "+err.Error(), manifest.PDF.Path))
			} else if hash != manifest.PDF.SHA256 {
				findings = append(findings, fail("package.pdf_hash", "PDF hash does not match manifest", manifest.PDF.Path))
			}
			if pages, err := countPDFPages(manifest.PDF.Path); err != nil {
				findings = append(findings, fail("package.pdf_pages", err.Error(), manifest.PDF.Path))
			} else if pages != manifest.PDFPageCount || pages != len(manifest.PNGFiles) {
				findings = append(findings, fail("package.pdf_pages", fmt.Sprintf("PDF pages=%d manifestPages=%d pngs=%d", pages, manifest.PDFPageCount, len(manifest.PNGFiles)), manifest.PDF.Path))
			}
			if manifest.PDFPageSizePoints.Width <= 0 || manifest.PDFPageSizePoints.Height <= 0 {
				findings = append(findings, fail("package.pdf_page_size", "manifest PDF page size is missing", manifestPath))
			}
			if hash, err := sha256File(manifest.QAMontage.Path); err != nil {
				findings = append(findings, fail("package.montage_hash", "manifest montage missing: "+err.Error(), manifest.QAMontage.Path))
			} else if hash != manifest.QAMontage.SHA256 {
				findings = append(findings, fail("package.montage_hash", "QA montage hash does not match manifest", manifest.QAMontage.Path))
			}
			if manifest.ChromeSandbox == "disabled" {
				findings = append(findings, qaFinding{Severity: "warn", Check: "package.chrome_sandbox", Message: "Chrome sandbox was disabled: " + manifest.ChromeNoSandboxReason, Path: manifestPath})
			} else if manifest.ChromeSandbox == "" {
				findings = append(findings, fail("package.chrome_sandbox", "manifest must record Chrome sandbox status", manifestPath))
			}
			if manifest.SlideEnumerationMethod == "" {
				findings = append(findings, fail("package.slide_enumeration", "manifest must record slide enumeration method", manifestPath))
			}
			if reportFindings := verifyTextArtifactFreshness("qa_report", qaReportPath, manifestPath, []string{manifest.SourceHTML.SHA256, mustSHA256(manifestPath), hashFileSet(filepath.Join(outDir, "rendered_slides", "slide_*.png"))}); len(reportFindings) > 0 {
				findings = append(findings, reportFindings...)
			}
			findings = append(findings, verifyQAReportStatus(qaReportPath)...)
			if summaryFindings := verifyTextArtifactFreshness("delivery_summary", deliverySummaryPath, manifestPath, []string{mustSHA256(manifestPath), mustSHA256(qaReportPath), riskStateHashForDeck(filepath.Dir(outDir))}); len(summaryFindings) > 0 {
				findings = append(findings, summaryFindings...)
			}
			findings = append(findings, verifyVisualReviewImageSet(visualImageSetPath, manifest)...)
			if !visualReviewArtifactFresh(visualReviewPath, manifest) {
				findings = append(findings, fail("package.visual_review_freshness", "visual review result is missing, stale, or not pass", visualReviewPath))
			}
			findings = append(findings, verifyVisualReviewEvidence(visualReviewPath, manifest)...)
			reviewStages := structuredReviewStages()
			for i, structuredReviewPath := range structuredReviewPaths {
				findings = append(findings, verifyStructuredReviewGate(structuredReviewPath, reviewStages[i], manifest, deckAbs, htmlPath, qaReportPath, deliverySummaryPath)...)
			}
		}
	}
	if includeLogs {
		findings = append(findings, verifySanitizedLogs(outDir)...)
	}
	findings = append(findings, verifyRiskPolicy(filepath.Join(outDir, "slidex_state.json"))...)
	status := statusFromFindings(findings)
	return map[string]any{
		"toolName": toolName,
		"version":  toolVersion,
		"deckDir":  deckAbs,
		"outDir":   outDir,
		"status":   status,
		"findings": findings,
	}, nil
}

func verifyPackageSpec(path string) []qaFinding {
	specFindings, err := validateSpecFile(path)
	if err != nil {
		return []qaFinding{fail("package.deck_spec", err.Error(), path)}
	}
	for i := range specFindings {
		if !strings.HasPrefix(specFindings[i].Check, "package.") {
			specFindings[i].Check = "package.deck_spec." + specFindings[i].Check
		}
	}
	return specFindings
}

func verifyQAReportStatus(path string) []qaFinding {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []qaFinding{fail("package.qa_report_status", "QA report missing: "+err.Error(), path)}
	}
	text := string(raw)
	var findings []qaFinding
	for _, key := range []string{"deterministicStatus", "visualStatus"} {
		value := qaReportStatusField(text, key)
		if value == "" {
			findings = append(findings, fail("package.qa_report_status", key+" is missing from QA report", path))
			continue
		}
		if value != "pass" {
			findings = append(findings, fail("package.qa_report_status", key+" is "+value+", want pass", path))
		}
	}
	overall := qaReportStatusField(text, "Overall status")
	if overall == "" {
		findings = append(findings, fail("package.qa_report_status", "Overall status is missing from QA report", path))
	} else if overall != "pass" {
		findings = append(findings, fail("package.qa_report_status", "Overall status is "+overall+", want pass", path))
	}
	return findings
}

func qaReportStatusField(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, key+":")), "`")
	}
	return ""
}

func verifyRiskPolicy(path string) []qaFinding {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []qaFinding{fail("package.risk_policy", "missing state file: "+err.Error(), path)}
	}
	var state slidexState
	if err := json.Unmarshal(raw, &state); err != nil {
		return []qaFinding{fail("package.risk_policy", err.Error(), path)}
	}
	var findings []qaFinding
	if len(state.UnresolvedRisks) > 0 {
		findings = append(findings, fail("package.unresolved_risks", "unresolved risks remain in slidex_state.json", path))
	}
	for i, risk := range state.AcceptedRisks {
		prefix := fmt.Sprintf("accepted risk %d", i+1)
		if strings.TrimSpace(risk.Reason) == "" || strings.TrimSpace(risk.Owner) == "" || strings.TrimSpace(risk.Expiration) == "" || strings.TrimSpace(risk.ArtifactLink) == "" {
			findings = append(findings, fail("package.accepted_risk_policy", prefix+" must include reason, owner, expiration, and artifactLink", path))
			continue
		}
		expiration, err := time.Parse(time.RFC3339, risk.Expiration)
		if err != nil {
			findings = append(findings, fail("package.accepted_risk_policy", prefix+" expiration must be RFC3339", path))
		} else if time.Now().After(expiration) {
			findings = append(findings, fail("package.accepted_risk_policy", prefix+" is expired", path))
		}
	}
	return findings
}

func verifyManifestDependencies(kind string, manifestDeps []dependency, currentDeps []dependency, manifestPath string) []qaFinding {
	var findings []qaFinding
	findings = append(findings, dependencyPinFindings("package."+kind+"_pin", manifestDeps, manifestPath)...)
	manifestSet := map[string]dependency{}
	for _, dep := range manifestDeps {
		manifestSet[dependencyFreshnessKey(dep)] = dep
		if dep.Path != "" && dep.SHA256 != "" {
			if hash, err := sha256File(dep.Path); err != nil {
				findings = append(findings, fail("package."+kind+"_dependency", "dependency file missing: "+err.Error(), dep.Path))
			} else if hash != dep.SHA256 {
				findings = append(findings, fail("package."+kind+"_dependency", "dependency hash does not match manifest", dep.Path))
			}
		}
		if dep.Path == "" && dep.URL == "" && dep.Kind != "inline_css" && dep.Kind != "font_preset" {
			findings = append(findings, qaFinding{Severity: "warn", Check: "package." + kind + "_dependency", Message: "dependency has no path or URL", Path: manifestPath})
		}
	}
	currentSet := map[string]dependency{}
	for _, dep := range currentDeps {
		currentSet[dependencyFreshnessKey(dep)] = dep
	}
	for key := range currentSet {
		if _, ok := manifestSet[key]; !ok {
			findings = append(findings, fail("package."+kind+"_dependency", "current dependency is missing or changed in manifest: "+key, manifestPath))
		}
	}
	for key := range manifestSet {
		if _, ok := currentSet[key]; !ok {
			findings = append(findings, fail("package."+kind+"_dependency", "manifest dependency is no longer present in current HTML: "+key, manifestPath))
		}
	}
	return findings
}

func dependencyFreshnessKey(dep dependency) string {
	return dep.Kind + "|" + dep.ID + "|" + dep.Path + "|" + dep.URL + "|" + dep.Version + "|" + dep.SHA256
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mustSHA256(path string) string {
	h, _ := sha256File(path)
	return h
}

func artifactFromPath(path string) artifact {
	info, err := os.Stat(path)
	if err != nil {
		return artifact{Path: path}
	}
	return artifact{Path: path, SHA256: mustSHA256(path), Size: info.Size()}
}

func pngPaths(images []renderedImage) []string {
	out := make([]string, len(images))
	for i, img := range images {
		out[i] = img.Path
	}
	return out
}

func countPDFPages(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	re := regexp.MustCompile(`/Type\s*/Page\b`)
	return len(re.FindAll(raw, -1)), nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return secureWriteFile(path, raw, 0o600)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func fail(check, message, path string) qaFinding {
	return qaFinding{Severity: "fail", Check: check, Message: message, Path: path}
}

func statusFromFindings(findings []qaFinding) string {
	if hasFailures(findings) {
		return "fail"
	}
	if hasWarnings(findings) {
		return "pass_with_risks"
	}
	return "pass"
}

func hasFailures(findings []qaFinding) bool {
	for _, f := range findings {
		if f.Severity == "fail" {
			return true
		}
	}
	return false
}

func hasWarnings(findings []qaFinding) bool {
	for _, f := range findings {
		if f.Severity == "warn" {
			return true
		}
	}
	return false
}

func in(value string, options []string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func coalesceInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
