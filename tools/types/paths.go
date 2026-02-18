package types

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveProjectFilePath(input string, allowedExts []string) (string, string, error) {
	cleanInput := strings.TrimSpace(input)
	if cleanInput == "" {
		return "", "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(cleanInput) {
		return "", "", fmt.Errorf("absolute paths are not allowed")
	}

	rel := cleanInput
	if after, ok := strings.CutPrefix(rel, "res://"); ok {
		rel = after
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", "", fmt.Errorf("path is required")
	}

	cleanRel := filepath.Clean(rel)
	if cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path escapes project root")
	}

	projectRoot := ResolveProjectRootFromEnvOrCWD()
	projectAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve project root: %w", err)
	}

	fullPath := filepath.Join(projectAbs, cleanRel)
	fullAbs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}

	if !isWithinRoot(fullAbs, projectAbs) {
		return "", "", fmt.Errorf("path escapes project root")
	}

	if len(allowedExts) > 0 {
		ext := strings.ToLower(filepath.Ext(fullAbs))
		allowed := false
		for _, candidate := range allowedExts {
			if ext == strings.ToLower(strings.TrimSpace(candidate)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", "", fmt.Errorf("unsupported file extension: %s", ext)
		}
	}

	resPath := "res://" + filepath.ToSlash(cleanRel)
	return fullAbs, resPath, nil
}

func ReadProjectFile(input string, allowedExts []string) ([]byte, string, error) {
	fullPath, resPath, err := ResolveProjectFilePath(input, allowedExts)
	if err != nil {
		return nil, "", err
	}

	projectRoot := ResolveProjectRootFromEnvOrCWD()
	projectAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, "", fmt.Errorf("resolve project root: %w", err)
	}
	projectReal := projectAbs
	if resolvedProjectRoot, resolveErr := filepath.EvalSymlinks(projectAbs); resolveErr == nil {
		projectReal = resolvedProjectRoot
	}

	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return nil, "", err
	}
	if !isWithinRoot(resolvedPath, projectReal) {
		return nil, "", fmt.Errorf("path escapes project root")
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, "", err
	}
	return data, resPath, nil
}

func isWithinRoot(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	rootWithSep := cleanRoot + string(filepath.Separator)
	if cleanPath == cleanRoot {
		return true
	}
	return strings.HasPrefix(cleanPath, rootWithSep)
}
