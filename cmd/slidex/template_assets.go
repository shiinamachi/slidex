package main

import (
	"embed"
	"io/fs"
	"os"
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
	return fs.WalkDir(embeddedTemplateAssets, embeddedTemplateRoot, func(assetPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(filepath.FromSlash(embeddedTemplateRoot), filepath.FromSlash(assetPath))
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := embeddedTemplateAssets.ReadFile(assetPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}

func isDefaultTemplateRef(fromTemplate string) bool {
	if fromTemplate == "" {
		return true
	}
	return filepath.Clean(filepath.FromSlash(fromTemplate)) == filepath.Clean(filepath.FromSlash(defaultDeckTemplatePath))
}
