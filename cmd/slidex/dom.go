package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

type chromeEnumeratedSlide struct {
	ID        string `json:"id"`
	OuterHTML string `json:"outerHTML"`
	Text      string `json:"text"`
	Headline  string `json:"headline"`
}

var (
	headOpenElementRe = regexp.MustCompile(`(?is)<head\b[^>]*>`)
	htmlOpenElementRe = regexp.MustCompile(`(?is)<html\b[^>]*>`)
)

func extractSlidesWithChrome(chromePath, htmlPath, selector string, chromeNoSandbox bool) ([]slideInfo, string, error) {
	if selector != ".slide" {
		return nil, "", fmt.Errorf("unsupported selector for Chrome enumeration: %s", selector)
	}
	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil, "", err
	}
	injected := injectDocumentBase(injectSlideEnumerationScript(string(raw)), documentBaseHrefForHTMLPath(htmlPath))
	tmpDir, err := os.MkdirTemp("", "slidex-dom-enum-*")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(tmpDir)
	tmpHTML := filepath.Join(tmpDir, "enumerate.html")
	if err := os.WriteFile(tmpHTML, []byte(injected), 0o644); err != nil {
		return nil, "", err
	}
	args, cleanup, err := chromeHeadlessBaseArgs(chromeNoSandbox)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()
	args = append(args,
		"--virtual-time-budget=3000",
		"--dump-dom",
		fileURLFromPath(tmpHTML),
	)
	out, err := runChromeCommand(chromeCommandTimeout, chromePath, args...)
	re := regexp.MustCompile(`(?is)<script id="slidex-slide-enumeration" type="application/json">(.*?)</script>`)
	m := re.FindStringSubmatch(string(out))
	if err != nil && !(isChromeCommandTimeout(err) && len(m) >= 2) {
		return nil, "", fmt.Errorf("chrome DOM enumeration failed: %w\n%s", err, string(out))
	}
	if len(m) < 2 {
		return nil, "", errorsNew("slide enumeration data missing from dumped DOM")
	}
	payload := strings.TrimSpace(xhtml.UnescapeString(m[1]))
	var enumerated []chromeEnumeratedSlide
	if err := json.Unmarshal([]byte(payload), &enumerated); err != nil {
		return nil, "", err
	}
	slides := make([]slideInfo, 0, len(enumerated))
	for _, item := range enumerated {
		parsed := extractSlidesHTMLParser(item.OuterHTML)
		if len(parsed) == 0 {
			continue
		}
		slide := parsed[0]
		if item.ID != "" {
			slide.ID = item.ID
		}
		if item.Text != "" {
			slide.Text = normalizeText(item.Text)
		}
		if item.Headline != "" {
			slide.Headline = normalizeText(item.Headline)
		}
		slides = append(slides, slide)
	}
	if len(slides) == 0 {
		return nil, "", errorsNew("Chrome DOM enumeration found no .slide elements")
	}
	return slides, "chrome-dom", nil
}

func documentBaseHrefForHTMLPath(htmlPath string) string {
	dir := filepath.Dir(htmlPath)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	dir = filepath.Clean(dir)
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(dir, sep) {
		dir += sep
	}
	return fileURLFromPath(dir)
}

func injectDocumentBase(src, baseHref string) string {
	if baseHref == "" {
		return src
	}
	inspection := inspectBaseElements(src, true)
	if len(inspection.ranges) > 0 {
		src = removeStringRanges(src, inspection.ranges)
	}
	base := `<base href="` + xhtml.EscapeString(baseHref) + `">`
	if loc := headOpenElementRe.FindStringIndex(src); loc != nil {
		return src[:loc[1]] + "\n" + base + src[loc[1]:]
	}
	if loc := htmlOpenElementRe.FindStringIndex(src); loc != nil {
		head := "\n<head>\n" + base + "\n</head>"
		return src[:loc[1]] + head + src[loc[1]:]
	}
	return "<head>\n" + base + "\n</head>\n" + src
}

func injectHeadBase(head, baseHref string) string {
	if baseHref == "" {
		return head
	}
	inspection := inspectBaseElements(head, false)
	if len(inspection.ranges) > 0 {
		head = removeStringRanges(head, inspection.ranges)
	}
	return `<base href="` + xhtml.EscapeString(baseHref) + `">` + "\n" + head
}

type baseInspection struct {
	ranges []stringRange
}

type stringRange struct {
	start int
	end   int
}

func inspectBaseElements(src string, requireHead bool) baseInspection {
	z := xhtml.NewTokenizer(strings.NewReader(src))
	inHead := !requireHead
	cursor := 0
	var inspection baseInspection
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			if z.Err() == io.EOF {
				return inspection
			}
			return inspection
		}
		tokenRange := currentTokenRange(src, &cursor, string(z.Raw()))
		nameBytes, _ := z.TagName()
		name := strings.ToLower(string(nameBytes))
		switch tt {
		case xhtml.StartTagToken:
			if requireHead && name == "head" {
				inHead = true
				continue
			}
			if inHead && name == "base" {
				inspection.ranges = append(inspection.ranges, tokenRange)
			}
		case xhtml.SelfClosingTagToken:
			if inHead && name == "base" {
				inspection.ranges = append(inspection.ranges, tokenRange)
			}
		case xhtml.EndTagToken:
			if requireHead && name == "head" {
				inHead = false
			}
		}
	}
}

func currentTokenRange(src string, cursor *int, raw string) stringRange {
	if raw == "" {
		return stringRange{}
	}
	if *cursor > len(src) {
		*cursor = len(src)
	}
	if idx := strings.Index(src[*cursor:], raw); idx >= 0 {
		start := *cursor + idx
		end := start + len(raw)
		*cursor = end
		return stringRange{start: start, end: end}
	}
	return stringRange{}
}

func removeStringRanges(src string, ranges []stringRange) string {
	for i := len(ranges) - 1; i >= 0; i-- {
		start, end := ranges[i].start, ranges[i].end
		if start < 0 || end > len(src) || start >= end {
			continue
		}
		src = src[:start] + src[end:]
	}
	return src
}

func injectSlideEnumerationScript(src string) string {
	script := `<script>
(function() {
  function textOf(el) { return (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim(); }
  function headlineOf(el) {
    const h = el.querySelector('h1,h2,h3');
    return h ? textOf(h) : '';
  }
  function idOf(el, index) { return el.getAttribute('data-slide-id') || el.id || ('slide_' + String(index + 1).padStart(2, '0')); }
  function emit() {
    const slides = Array.from(document.querySelectorAll('.slide')).map((el, index) => ({
      id: idOf(el, index),
      outerHTML: el.outerHTML,
      text: textOf(el),
      headline: headlineOf(el)
    }));
    const report = document.createElement('script');
    report.id = 'slidex-slide-enumeration';
    report.type = 'application/json';
    report.textContent = JSON.stringify(slides);
    document.body.appendChild(report);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', emit);
  } else {
    emit();
  }
})();
</script>`
	lower := strings.ToLower(src)
	if idx := strings.LastIndex(lower, "</body>"); idx >= 0 {
		return src[:idx] + script + src[idx:]
	}
	return src + script
}

func extractSlidesHTMLParser(src string) []slideInfo {
	doc, err := xhtml.Parse(strings.NewReader(src))
	if err != nil {
		return nil
	}
	var slides []slideInfo
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && hasNodeClass(n, "slide") {
			slides = append(slides, slideInfoFromNode(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return slides
}

func slideInfoFromNode(n *xhtml.Node) slideInfo {
	var rendered bytes.Buffer
	_ = xhtml.Render(&rendered, n)
	full := rendered.String()
	attrs := renderAttrs(n.Attr)
	id := firstNonEmpty(nodeAttr(n, "data-slide-id"), nodeAttr(n, "id"))
	return slideInfo{
		ID:       id,
		Attrs:    attrs,
		HTML:     renderChildren(n),
		FullHTML: full,
		Headline: normalizeText(firstHeadlineText(n)),
		Text:     normalizeText(nodeText(n)),
	}
}

func renderChildren(n *xhtml.Node) string {
	var b bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = xhtml.Render(&b, c)
	}
	return b.String()
}

func renderAttrs(attrs []xhtml.Attribute) string {
	parts := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		name := attr.Key
		if attr.Namespace != "" {
			name = attr.Namespace + ":" + attr.Key
		}
		parts = append(parts, fmt.Sprintf(`%s="%s"`, name, xhtml.EscapeString(attr.Val)))
	}
	return strings.Join(parts, " ")
}

func hasNodeClass(n *xhtml.Node, className string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			for _, part := range strings.Fields(attr.Val) {
				if part == className {
					return true
				}
			}
		}
	}
	return false
}

func nodeAttr(n *xhtml.Node, name string) string {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func nodeText(n *xhtml.Node) string {
	if n.Type == xhtml.ElementNode && (n.Data == "script" || n.Data == "style") {
		return ""
	}
	if n.Type == xhtml.TextNode {
		return n.Data
	}
	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := nodeText(c); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

func firstHeadlineText(n *xhtml.Node) string {
	var found string
	var walk func(*xhtml.Node)
	walk = func(cur *xhtml.Node) {
		if found != "" {
			return
		}
		if cur.Type == xhtml.ElementNode && (cur.Data == "h1" || cur.Data == "h2" || cur.Data == "h3") {
			found = nodeText(cur)
			return
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}

func errorsNew(message string) error {
	return fmt.Errorf("%s", message)
}
