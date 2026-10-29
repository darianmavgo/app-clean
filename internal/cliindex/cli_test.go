package cliindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIToolMatching(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cli_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy executable files: 2fa, custom-tool, go-dummy
	tool2fa := filepath.Join(tmpDir, "2fa")
	toolCustom := filepath.Join(tmpDir, "custom-tool")
	toolNonExec := filepath.Join(tmpDir, "readme.txt")

	os.WriteFile(tool2fa, []byte("#!/bin/sh\necho 2fa"), 0755)
	os.WriteFile(toolCustom, []byte("#!/bin/sh\necho custom"), 0755)
	os.WriteFile(toolNonExec, []byte("text"), 0644) // Not executable

	idx, err := ScanInstalledCLITools(context.Background(), ScanConfig{
		ExtraDirs: []string{tmpDir},
	})
	if err != nil {
		t.Fatalf("ScanInstalledCLITools failed: %v", err)
	}

	// 1. Should match "2fa"
	if m := idx.Match("2fa"); m == nil {
		t.Errorf("Expected Match('2fa') to find tool, got nil")
	} else if m.Name != "2fa" {
		t.Errorf("Expected name '2fa', got %q", m.Name)
	}

	// 2. Should match "custom-tool"
	if m := idx.Match("custom-tool"); m == nil {
		t.Errorf("Expected Match('custom-tool') to find tool, got nil")
	}

	// 3. Should NOT match non-executable
	if m := idx.Match("readme.txt"); m != nil {
		t.Errorf("Expected non-executable to be ignored, got %+v", m)
	}

	// 4. Should match built-in system CLI like "sh" or "ls"
	if m := idx.Match("sh"); m == nil {
		t.Errorf("Expected Match('sh') to find system binary, got nil")
	}
}
