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

func TestPromptFromSkillFile_WithTitleAndArguments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: scene-review
title: Scene Review
description: "Review scenes"
---

Review {{scene_path}} with {{Line}} and {{line}}.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	prompt, err := PromptFromSkillFile(path)
	if err != nil {
		t.Fatalf("PromptFromSkillFile: %v", err)
	}
	if prompt.Title != "Scene Review" {
		t.Fatalf("expected title Scene Review, got %q", prompt.Title)
	}
	if len(prompt.Arguments) != 3 {
		t.Fatalf("expected 3 arguments, got %d", len(prompt.Arguments))
	}
	if prompt.Arguments[0].Name != "Line" || prompt.Arguments[1].Name != "line" || prompt.Arguments[2].Name != "scene_path" {
		t.Fatalf("unexpected argument order/content: %+v", prompt.Arguments)
	}
}

func TestPromptFromSkillFile_CRLFFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := "---\r\nname: scene-review\r\ndescription: \"Review scenes\"\r\n---\r\n\r\nReview {{scene_path}}\r\nWith checks.\r\n"
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
	if strings.Contains(prompt.Template, "\r") {
		t.Fatalf("expected normalized template line endings, got %q", prompt.Template)
	}
	if prompt.Template != "Review {{scene_path}}\nWith checks." {
		t.Fatalf("unexpected template content: %q", prompt.Template)
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

func TestRegisterPrompt_NormalizesArgumentsWithCaseSensitivity(t *testing.T) {
	reg := NewRegistry(true)
	reg.RegisterPrompt(Prompt{
		Name: "render",
		Arguments: []PromptArgument{
			{Name: "zeta"},
			{Name: "Alpha"},
			{Name: "alpha"},
			{Name: "alpha"},
		},
	})

	prompt, ok := reg.GetPrompt("render")
	if !ok {
		t.Fatal("expected prompt")
	}
	if len(prompt.Arguments) != 3 {
		t.Fatalf("expected 3 normalized arguments, got %d", len(prompt.Arguments))
	}
	if prompt.Arguments[0].Name != "Alpha" || prompt.Arguments[1].Name != "alpha" || prompt.Arguments[2].Name != "zeta" {
		t.Fatalf("unexpected normalized arguments: %+v", prompt.Arguments)
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

func TestLoadFromPathsWithAllowedRoots_FallbacksToPathsWhenAllowedRootsEmpty(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: fallback\ndescription: fallback\n---\nbody\n"), 0644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	reg := NewRegistry(true)
	if err := reg.LoadFromPathsWithAllowedRoots([]string{root}, []string{}); err != nil {
		t.Fatalf("LoadFromPathsWithAllowedRoots: %v", err)
	}
	if _, ok := reg.GetPrompt("fallback"); !ok {
		t.Fatal("expected prompt loaded when allowed roots are empty and fallback to paths is used")
	}
}

func TestLoadFromPathsWithAllowedRoots_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on windows and may require additional privileges")
	}

	root := t.TempDir()
	outside := t.TempDir()

	validSkill := filepath.Join(root, "valid", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(validSkill), 0755); err != nil {
		t.Fatalf("mkdir valid skill dir: %v", err)
	}
	if err := os.WriteFile(validSkill, []byte("---\nname: inside\ndescription: inside\n---\ninside body\n"), 0644); err != nil {
		t.Fatalf("write valid skill file: %v", err)
	}

	outsideSkill := filepath.Join(outside, "SKILL.md")
	if err := os.WriteFile(outsideSkill, []byte("---\nname: outside\ndescription: outside\n---\noutside body\n"), 0644); err != nil {
		t.Fatalf("write outside skill file: %v", err)
	}

	escapeDir := filepath.Join(root, "escape")
	if err := os.MkdirAll(escapeDir, 0755); err != nil {
		t.Fatalf("mkdir escape dir: %v", err)
	}
	linkPath := filepath.Join(escapeDir, "SKILL.md")
	if err := os.Symlink(outsideSkill, linkPath); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	reg := NewRegistry(true)
	err := reg.LoadFromPathsWithAllowedRoots([]string{root}, []string{root})
	if err == nil {
		t.Fatal("expected load error due to symlink escape")
	}
	if reg.PromptCount() != 1 {
		t.Fatalf("expected only in-root prompt to load, got %d", reg.PromptCount())
	}
	if _, ok := reg.GetPrompt("inside"); !ok {
		t.Fatal("expected in-root prompt")
	}
	if _, ok := reg.GetPrompt("outside"); ok {
		t.Fatal("did not expect escaped prompt to load")
	}
}

func TestLoadFromPathsWithAllowedRoots_LoadsValidAndSkipsInvalidPaths(t *testing.T) {
	allowedRoot := t.TempDir()
	disallowedRoot := t.TempDir()

	allowedSkill := filepath.Join(allowedRoot, "valid", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(allowedSkill), 0755); err != nil {
		t.Fatalf("mkdir allowed skill dir: %v", err)
	}
	if err := os.WriteFile(allowedSkill, []byte("---\nname: allowed\ndescription: allowed\n---\nallowed body\n"), 0644); err != nil {
		t.Fatalf("write allowed skill file: %v", err)
	}

	disallowedSkill := filepath.Join(disallowedRoot, "invalid", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(disallowedSkill), 0755); err != nil {
		t.Fatalf("mkdir disallowed skill dir: %v", err)
	}
	if err := os.WriteFile(disallowedSkill, []byte("---\nname: blocked\ndescription: blocked\n---\nblocked body\n"), 0644); err != nil {
		t.Fatalf("write disallowed skill file: %v", err)
	}

	reg := NewRegistry(true)
	err := reg.LoadFromPathsWithAllowedRoots([]string{allowedRoot, disallowedRoot}, []string{allowedRoot})
	if err == nil {
		t.Fatal("expected load warnings due to disallowed path")
	}
	if reg.PromptCount() != 1 {
		t.Fatalf("expected one prompt from allowed root, got %d", reg.PromptCount())
	}
	if _, ok := reg.GetPrompt("allowed"); !ok {
		t.Fatal("expected allowed prompt to load")
	}
	if _, ok := reg.GetPrompt("blocked"); ok {
		t.Fatal("did not expect blocked prompt to load")
	}
}

func TestSnapshotFingerprint_DetectsContentChangeWithSameSizeAndMtime(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "scene-review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}

	contentV1 := "---\nname: scene-review\ndescription: desc\n---\nReview A\n"
	contentV2 := "---\nname: scene-review\ndescription: desc\n---\nReview B\n"
	if len(contentV1) != len(contentV2) {
		t.Fatalf("expected equal-size test payloads, got %d vs %d", len(contentV1), len(contentV2))
	}

	if err := os.WriteFile(skill, []byte(contentV1), 0644); err != nil {
		t.Fatalf("write skill v1: %v", err)
	}
	firstFingerprint, warnings := SnapshotFingerprint([]string{root}, nil)
	if len(warnings) > 0 {
		t.Fatalf("expected no snapshot warnings, got %v", warnings)
	}

	info, err := os.Stat(skill)
	if err != nil {
		t.Fatalf("stat skill file: %v", err)
	}
	fixedModTime := info.ModTime()

	if err := os.WriteFile(skill, []byte(contentV2), 0644); err != nil {
		t.Fatalf("write skill v2: %v", err)
	}
	if err := os.Chtimes(skill, fixedModTime, fixedModTime); err != nil {
		t.Fatalf("set fixed modtime: %v", err)
	}

	secondFingerprint, warnings := SnapshotFingerprint([]string{root}, nil)
	if len(warnings) > 0 {
		t.Fatalf("expected no snapshot warnings, got %v", warnings)
	}
	if firstFingerprint == secondFingerprint {
		t.Fatal("expected content hash to change snapshot fingerprint even when size/mtime are fixed")
	}

	// Guard against filesystem timestamp granularity accidentally changing the fixture.
	infoAfter, err := os.Stat(skill)
	if err != nil {
		t.Fatalf("stat skill file after rewrite: %v", err)
	}
	if !infoAfter.ModTime().Equal(fixedModTime) {
		t.Fatalf("expected fixed modtime %v, got %v", fixedModTime, infoAfter.ModTime())
	}
	if infoAfter.Size() != int64(len(contentV2)) {
		t.Fatalf("expected size %d, got %d", len(contentV2), infoAfter.Size())
	}

}
