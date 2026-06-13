package main

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
)

const (
	defaultDeckTemplatePath = "decks/_template"
	embeddedTemplateRoot    = "template_assets/decks/_template"
)

//go:embed template_assets/decks/_template/README.md
//go:embed template_assets/decks/_template/DESIGN.md
//go:embed template_assets/decks/_template/brief.md
//go:embed template_assets/decks/_template/assets/README.md
//go:embed template_assets/decks/_template/assets/reference_docs/README.md
//go:embed template_assets/decks/_template/brand/README.md
//go:embed template_assets/decks/_template/data/README.md
//go:embed template_assets/decks/_template/source/README.md
var embeddedTemplateAssets embed.FS

func copyEmbeddedDefaultTemplate(dst string) error {
	cleanDst := filepath.Clean(dst)
	budget := defaultDeckTemplateCopyBudget()
	budget.label = "embedded deck template"
	var entries int
	var totalBytes int64
	return fs.WalkDir(embeddedTemplateAssets, embeddedTemplateRoot, func(assetPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(filepath.FromSlash(embeddedTemplateRoot), filepath.FromSlash(assetPath))
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		target := filepath.Join(cleanDst, filepath.FromSlash(rel))
		if !pathWithin(cleanDst, target) {
			return fmt.Errorf("embedded deck template target escapes destination root: %s", filepath.ToSlash(target))
		}
		if rel != "." {
			entries++
			if budget.maxEntries > 0 && entries > budget.maxEntries {
				return fmt.Errorf("embedded deck template contains too many entries: %d > %d at %s", entries, budget.maxEntries, rel)
			}
		}
		if d.IsDir() {
			return ensureSecureDirMode(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("embedded deck template contains unsupported file type: %s", filepath.ToSlash(assetPath))
		}
		raw, err := embeddedTemplateAssets.ReadFile(assetPath)
		if err != nil {
			return err
		}
		size := int64(len(raw))
		if budget.maxFileBytes > 0 && size > budget.maxFileBytes {
			return fmt.Errorf("embedded deck template file exceeds maximum size: %s is %d bytes > %d", rel, size, budget.maxFileBytes)
		}
		if budget.maxTotalBytes > 0 && totalBytes > budget.maxTotalBytes-size {
			return fmt.Errorf("embedded deck template exceeds maximum total size at %s: %d bytes > %d", rel, totalBytes+size, budget.maxTotalBytes)
		}
		totalBytes += size
		return secureWriteFile(target, raw, 0o644)
	})
}

func isDefaultTemplateRef(fromTemplate string) bool {
	if fromTemplate == "" {
		return true
	}
	return filepath.Clean(filepath.FromSlash(fromTemplate)) == filepath.Clean(filepath.FromSlash(defaultDeckTemplatePath))
}
