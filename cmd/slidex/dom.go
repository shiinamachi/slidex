package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func extractSlidesWithChrome(chromePath, htmlPath, selector string, chromeNoSandbox bool) ([]slideInfo, string, error) {
	if selector != ".slide" {
		return nil, "", fmt.Errorf("unsupported selector for Chrome enumeration: %s", selector)
	}
	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil, "", err
	}
	injected := injectSlideEnumerationScript(string(raw))
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
