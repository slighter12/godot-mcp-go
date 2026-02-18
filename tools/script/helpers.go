package script

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/slighter12/godot-mcp-go/tools/types"
)

func listProjectScripts() ([]string, []string, error) {
	projectRoot := types.ResolveProjectRootFromEnvOrCWD()
	scriptNames := make([]string, 0)
	scriptPaths := make([]string, 0)

	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".gd" && ext != ".rs" {
			return nil
		}
		relPath, relErr := filepath.Rel(projectRoot, path)
		if relErr != nil {
			return relErr
		}
		scriptNames = append(scriptNames, strings.TrimSuffix(filepath.Base(path), ext))
		scriptPaths = append(scriptPaths, "res://"+filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Strings(scriptNames)
	sort.Strings(scriptPaths)
	return scriptNames, scriptPaths, nil
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := 1
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

func countNonEmptyLines(content string) int {
	count := 0
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func filepathExt(path string) string {
	return strings.ToLower(filepath.Ext(path))
}

func countFunctionSignatures(content string, ext string) int {
	count := 0
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch strings.ToLower(ext) {
		case ".gd":
			if strings.HasPrefix(trimmed, "func ") {
				count++
			}
		case ".rs":
			if strings.HasPrefix(trimmed, "fn ") || strings.HasPrefix(trimmed, "pub fn ") {
				count++
			}
		}
	}
	return count
}
