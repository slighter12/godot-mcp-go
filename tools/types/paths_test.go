package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestReadProjectFile_CacheHitAndMiss(t *testing.T) {
	resetProjectFileReadCacheForTest()

	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	target := filepath.Join(projectRoot, "scripts", "player.gd")
	if err := os.WriteFile(target, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)

	first, _, err := ReadProjectFile("res://scripts/player.gd", []string{".gd"})
	if err != nil {
		t.Fatalf("first read: %v", err)
	}

	hits, misses := projectFileReadCache.stats()
	if hits != 0 || misses != 1 {
		t.Fatalf("expected hits=0 misses=1, got hits=%d misses=%d", hits, misses)
	}

	first[0] = 'z'

	second, _, err := ReadProjectFile("res://scripts/player.gd", []string{".gd"})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}

	hits, misses = projectFileReadCache.stats()
	if hits != 1 || misses != 1 {
		t.Fatalf("expected hits=1 misses=1, got hits=%d misses=%d", hits, misses)
	}
	if string(second) != "abc" {
		t.Fatalf("expected cached content to be unchanged, got %q", string(second))
	}
}

func TestReadProjectFile_CacheInvalidatesOnFileUpdate(t *testing.T) {
	resetProjectFileReadCacheForTest()

	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	target := filepath.Join(projectRoot, "scripts", "enemy.gd")
	if err := os.WriteFile(target, []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write v1: %v", err)
	}
	firstTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(target, firstTime, firstTime); err != nil {
		t.Fatalf("set v1 mtime: %v", err)
	}

	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)

	if _, _, err := ReadProjectFile("res://scripts/enemy.gd", []string{".gd"}); err != nil {
		t.Fatalf("first read: %v", err)
	}

	if err := os.WriteFile(target, []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	secondTime := firstTime.Add(2 * time.Second)
	if err := os.Chtimes(target, secondTime, secondTime); err != nil {
		t.Fatalf("set v2 mtime: %v", err)
	}

	updated, _, err := ReadProjectFile("res://scripts/enemy.gd", []string{".gd"})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}

	hits, misses := projectFileReadCache.stats()
	if hits != 0 || misses != 2 {
		t.Fatalf("expected hits=0 misses=2 after update, got hits=%d misses=%d", hits, misses)
	}
	if string(updated) != "v2\n" {
		t.Fatalf("expected updated content, got %q", string(updated))
	}
}

func TestReadProjectFile_CacheDoesNotBypassSymlinkEscapeCheck(t *testing.T) {
	resetProjectFileReadCacheForTest()

	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}

	target := filepath.Join(projectRoot, "scripts", "swap.gd")
	if err := os.WriteFile(target, []byte("safe"), 0o644); err != nil {
		t.Fatalf("write safe file: %v", err)
	}

	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)
	if _, _, err := ReadProjectFile("res://scripts/swap.gd", []string{".gd"}); err != nil {
		t.Fatalf("warm cache with safe file: %v", err)
	}

	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "outside.gd")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	if err := os.Remove(target); err != nil {
		t.Fatalf("remove original file: %v", err)
	}
	if err := os.Symlink(outsideFile, target); err != nil {
		t.Skipf("symlink not supported in current environment: %v", err)
	}

	_, _, err := ReadProjectFile("res://scripts/swap.gd", []string{".gd"})
	if err == nil {
		t.Fatal("expected symlink escape to be rejected even after cache warmup")
	}
	if !strings.Contains(err.Error(), "path escapes project root") {
		t.Fatalf("expected path escapes project root error, got %v", err)
	}
}
