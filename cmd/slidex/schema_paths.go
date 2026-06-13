package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var resolveSourceRelativePath = sourceRelativePath

func builtinSchemaPath(schemaName string) string {
	return bundledSchemaPath(schemaName)
}

func builtinSchemaPathStrict(schemaName string) (string, error) {
	return bundledSchemaPathStrict(schemaName)
}

func resolveBuiltInSchemaPath(schemaPath string) string {
	resolved, ok, err := resolveBuiltInSchemaPathStrict(schemaPath)
	if ok && err == nil {
		return resolved
	}
	if ok {
		schemaName, _ := builtInSchemaName(schemaPath)
		return missingBundledSchemaPath(schemaName)
	}
	return schemaPath
}

func resolveBuiltInSchemaPathStrict(schemaPath string) (string, bool, error) {
	schemaName, ok := builtInSchemaName(schemaPath)
	if !ok {
		return schemaPath, false, nil
	}
	resolved, err := builtinSchemaPathStrict(schemaName)
	return resolved, true, err
}

func builtInSchemaName(schemaPath string) (string, bool) {
	trimmed := strings.TrimSpace(schemaPath)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", false
	}
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	slash := filepath.ToSlash(cleaned)
	const prefix = "schemas/"
	if !strings.HasPrefix(slash, prefix) {
		return "", false
	}
	schemaName := strings.TrimPrefix(slash, prefix)
	if schemaName == "" || schemaName == "." || strings.HasPrefix(schemaName, "../") || schemaName == ".." {
		return "", false
	}
	return schemaName, true
}

func trustedSchemaResolutionError(schemaName string, cause error) error {
	if cause != nil {
		return fmt.Errorf("built-in schema %s could not be resolved from a trusted install or source root: %w", schemaName, cause)
	}
	return fmt.Errorf("built-in schema %s could not be resolved from a trusted install or source root", schemaName)
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
