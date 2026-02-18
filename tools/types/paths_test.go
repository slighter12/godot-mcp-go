package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProjectFilePath_AllowsResAndRelativePaths(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scenes"), 0o755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	filePath := filepath.Join(projectRoot, "scenes", "Main.tscn")
	if err := os.WriteFile(filePath, []byte("[gd_scene format=3]"), 0o644); err != nil {
		t.Fatalf("write scene: %v", err)
	}

	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)

	abs, res, err := ResolveProjectFilePath("res://scenes/Main.tscn", []string{".tscn"})
	if err != nil {
		t.Fatalf("resolve res path: %v", err)
	}
	if abs != filePath {
		t.Fatalf("expected %s, got %s", filePath, abs)
	}
	if res != "res://scenes/Main.tscn" {
		t.Fatalf("expected canonical res path, got %s", res)
	}

	_, relRes, err := ResolveProjectFilePath("scenes/Main.tscn", []string{".tscn"})
	if err != nil {
		t.Fatalf("resolve relative path: %v", err)
	}
	if relRes != "res://scenes/Main.tscn" {
		t.Fatalf("expected canonical relative res path, got %s", relRes)
	}
}

func TestResolveProjectFilePath_RejectsEscapeAndAbsolutePaths(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)

	if _, _, err := ResolveProjectFilePath("../outside.gd", []string{".gd"}); err == nil {
		t.Fatal("expected traversal path to fail")
	}

	absPath := filepath.Join(projectRoot, "outside.gd")
	if _, _, err := ResolveProjectFilePath(absPath, []string{".gd"}); err == nil {
		t.Fatal("expected absolute path to fail")
	}
}

func TestReadProjectFile_RejectsSymlinkEscape(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}

	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "outside.gd")
	if err := os.WriteFile(outsideFile, []byte("extends Node\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	linkPath := filepath.Join(projectRoot, "scripts", "linked.gd")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported in current environment: %v", err)
	}

	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)
	_, _, err := ReadProjectFile("res://scripts/linked.gd", []string{".gd"})
	if err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
	if !strings.Contains(err.Error(), "path escapes project root") {
		t.Fatalf("expected path escapes project root error, got %v", err)
	}
}
