package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"appcleaner/internal/appindex"
	"appcleaner/internal/matcher"
)

func TestScanTargetsAndSizing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scanner_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy installed app
	installed := []*appindex.App{
		{
			Path:       "/Applications/InstalledApp.app",
			BundleName: "InstalledApp",
		},
	}
	idx := appindex.NewIndex(installed)
	m := matcher.NewMatcher(idx, nil, nil)

	// Create 1 installed support folder and 1 orphan support folder
	installedSupport := filepath.Join(tmpDir, "InstalledApp")
	orphanSupport := filepath.Join(tmpDir, "OldDeletedApp")
	os.MkdirAll(installedSupport, 0755)
	os.MkdirAll(orphanSupport, 0755)

	// Put files in the orphan folder
	os.WriteFile(filepath.Join(orphanSupport, "config.json"), []byte("{\"active\": false}"), 0644)
	os.WriteFile(filepath.Join(orphanSupport, "state.db"), []byte("dummy binary data state"), 0644)

	res, err := ScanTargets(context.Background(), m, ScanTargetConfig{
		TargetDirs: []string{tmpDir},
		Workers:    2,
	})
	if err != nil {
		t.Fatalf("ScanTargets failed: %v", err)
	}

	if len(res.Candidates) != 1 {
		t.Fatalf("Expected 1 candidate, got %d", len(res.Candidates))
	}

	cand := res.Candidates[0]
	if cand.AppName != "OldDeletedApp" {
		t.Errorf("Expected candidate 'OldDeletedApp', got %q", cand.AppName)
	}
	if cand.FileCount != 2 {
		t.Errorf("Expected 2 files, got %d", cand.FileCount)
	}
	if cand.Size <= 0 {
		t.Errorf("Expected positive size, got %d", cand.Size)
	}
	if len(cand.SampleFiles) != 2 {
		t.Errorf("Expected 2 sample files, got %d", len(cand.SampleFiles))
	}
}
