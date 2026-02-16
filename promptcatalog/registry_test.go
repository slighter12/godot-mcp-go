package promptcatalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPromptFromSkillFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: scene-review
description: "Review scenes"
---

Review {{scene_path}} with policy rules.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	prompt, err := PromptFromSkillFile(path)
	if err != nil {
		t.Fatalf("PromptFromSkillFile: %v", err)
	}
	if prompt.Name != "scene-review" {
		t.Fatalf("expected name scene-review, got %q", prompt.Name)
	}
	if prompt.Description != "Review scenes" {
		t.Fatalf("expected description Review scenes, got %q", prompt.Description)
	}
	if prompt.Template == "" {
		t.Fatal("expected non-empty template")
	}
}

func TestLoadFromPaths_Recursive(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "A", "SKILL.md")
	secondary := filepath.Join(root, "B", "Nested", "SKILL.md")

	if err := os.MkdirAll(filepath.Dir(primary), 0755); err != nil {
		t.Fatalf("mkdir primary: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(secondary), 0755); err != nil {
		t.Fatalf("mkdir secondary: %v", err)
	}

	if err := os.WriteFile(primary, []byte("---\nname: alpha\ndescription: alpha\n---\nalpha template\n"), 0644); err != nil {
		t.Fatalf("write primary: %v", err)
	}
	if err := os.WriteFile(secondary, []byte("---\nname: beta\ndescription: beta\n---\nbeta template\n"), 0644); err != nil {
		t.Fatalf("write secondary: %v", err)
	}

	reg := NewRegistry(true)
	if err := reg.LoadFromPaths([]string{root}); err != nil {
		t.Fatalf("LoadFromPaths: %v", err)
	}
	if reg.PromptCount() != 2 {
		t.Fatalf("expected 2 prompts, got %d", reg.PromptCount())
	}
	if _, ok := reg.GetPrompt("alpha"); !ok {
		t.Fatal("expected prompt alpha")
	}
	if _, ok := reg.GetPrompt("beta"); !ok {
		t.Fatal("expected prompt beta")
	}
}

func TestLoadFromPaths_CollectsErrors(t *testing.T) {
	reg := NewRegistry(true)
	missing := filepath.Join(t.TempDir(), "missing", "SKILL.md")

	if err := reg.LoadFromPaths([]string{missing}); err == nil {
		t.Fatal("expected load error for missing skill file")
	}
	if got := len(reg.LoadErrors()); got == 0 {
		t.Fatal("expected load errors to be recorded")
	}
}

func TestGetPrompt_CaseInsensitive(t *testing.T) {
	reg := NewRegistry(true)
	reg.RegisterPrompt(Prompt{
		Name:        "Scene-Review",
		Description: "desc",
		Template:    "body",
	})

	prompt, ok := reg.GetPrompt("scene-review")
	if !ok {
		t.Fatal("expected case-insensitive lookup to find prompt")
	}
	if prompt.Name != "Scene-Review" {
		t.Fatalf("expected original name Scene-Review, got %q", prompt.Name)
	}
}

func TestLoadFromPaths_DuplicatePromptNameCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "A", "SKILL.md")
	second := filepath.Join(root, "B", "SKILL.md")

	if err := os.MkdirAll(filepath.Dir(first), 0755); err != nil {
		t.Fatalf("mkdir first: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(second), 0755); err != nil {
		t.Fatalf("mkdir second: %v", err)
	}

	if err := os.WriteFile(first, []byte("---\nname: Scene-Review\ndescription: first\n---\nfirst template\n"), 0644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte("---\nname: scene-review\ndescription: second\n---\nsecond template\n"), 0644); err != nil {
		t.Fatalf("write second: %v", err)
	}

	reg := NewRegistry(true)
	if err := reg.LoadFromPaths([]string{root}); err == nil {
		t.Fatal("expected duplicate prompt load error")
	}
	if reg.PromptCount() != 1 {
		t.Fatalf("expected exactly one prompt after duplicate filtering, got %d", reg.PromptCount())
	}
	prompt, ok := reg.GetPrompt("scene-review")
	if !ok {
		t.Fatal("expected prompt to be discoverable")
	}
	if prompt.Description != "first" {
		t.Fatalf("expected first prompt to win, got %q", prompt.Description)
	}
}

func TestLoadFromPaths_ReplacesPreviousState(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "A", "SKILL.md")
	second := filepath.Join(root, "B", "SKILL.md")

	if err := os.MkdirAll(filepath.Dir(first), 0755); err != nil {
		t.Fatalf("mkdir first: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(second), 0755); err != nil {
		t.Fatalf("mkdir second: %v", err)
	}

	if err := os.WriteFile(first, []byte("---\nname: first\ndescription: first\n---\nfirst template\n"), 0644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte("---\nname: second\ndescription: second\n---\nsecond template\n"), 0644); err != nil {
		t.Fatalf("write second: %v", err)
	}

	reg := NewRegistry(true)
	missing := filepath.Join(t.TempDir(), "missing", "SKILL.md")
	if err := reg.LoadFromPaths([]string{missing}); err == nil {
		t.Fatal("expected initial load to fail")
	}
	if got := len(reg.LoadErrors()); got == 0 {
		t.Fatal("expected initial load errors")
	}

	if err := reg.LoadFromPaths([]string{filepath.Dir(second)}); err != nil {
		t.Fatalf("second load should succeed, got %v", err)
	}
	if reg.PromptCount() != 1 {
		t.Fatalf("expected one prompt after reload, got %d", reg.PromptCount())
	}
	if _, ok := reg.GetPrompt("second"); !ok {
		t.Fatal("expected second prompt after reload")
	}
	if _, ok := reg.GetPrompt("first"); ok {
		t.Fatal("did not expect stale first prompt after reload")
	}
	if got := len(reg.LoadErrors()); got != 0 {
		t.Fatalf("expected load errors to be reset after successful reload, got %d", got)
	}
}

func TestLoadFromPaths_RecordsWalkErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based walk error test is platform-specific")
	}

	root := t.TempDir()
	skillPath := filepath.Join(root, "A_skill", "SKILL.md")
	blockedDir := filepath.Join(root, "Z_blocked")

	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: alpha\ndescription: alpha\n---\nalpha template\n"), 0644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(blockedDir, "nested"), 0755); err != nil {
		t.Fatalf("mkdir blocked dir: %v", err)
	}
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Skipf("chmod not supported in this environment: %v", err)
	}
	defer func() {
		_ = os.Chmod(blockedDir, 0755)
	}()

	reg := NewRegistry(true)
	err := reg.LoadFromPaths([]string{root})
	if err == nil {
		t.Fatal("expected load error from walk failure")
	}
	if !strings.Contains(err.Error(), "walk skill path") {
		t.Fatalf("expected walk error details, got %v", err)
	}
	if reg.PromptCount() != 1 {
		t.Fatalf("expected prompt discovered before walk error, got %d", reg.PromptCount())
	}
	if _, ok := reg.GetPrompt("alpha"); !ok {
		t.Fatal("expected discovered prompt alpha")
	}
}
