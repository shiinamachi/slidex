package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func builtinSchemaPath(schemaName string) string {
	return bundledSchemaPath(schemaName)
}

func resolveBuiltInSchemaPath(schemaPath string) string {
	trimmed := strings.TrimSpace(schemaPath)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return schemaPath
	}
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	slash := filepath.ToSlash(cleaned)
	const prefix = "schemas/"
	if !strings.HasPrefix(slash, prefix) {
		return schemaPath
	}
	schemaName := strings.TrimPrefix(slash, prefix)
	if schemaName == "" || schemaName == "." || strings.HasPrefix(schemaName, "../") || schemaName == ".." {
		return schemaPath
	}
	return builtinSchemaPath(schemaName)
}

func sourceRelativePath(rel string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok || file == "" {
		return ""
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	candidate := filepath.Join(root, rel)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}
