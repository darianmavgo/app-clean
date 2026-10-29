package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"appcleaner/internal/matcher"
)

func TestDeleteCandidates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cleaner_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create 3 dummy folders
	var candidates []*matcher.Candidate
	for i := 1; i <= 3; i++ {
		p := filepath.Join(tmpDir, filepath.Join("folder", string(rune('0'+i))))
		os.MkdirAll(p, 0755)
		filePath := filepath.Join(p, "data.txt")
		os.WriteFile(filePath, []byte("hello world"), 0644)
		candidates = append(candidates, &matcher.Candidate{
			AppName:   "App" + string(rune('0'+i)),
			Path:      p,
			Size:      11,
			FileCount: 1,
		})
	}

	// 1. Dry run
	dryRes := DeleteCandidates(context.Background(), candidates, CleanOptions{
		DryRun:  true,
		Workers: 2,
	})
	if dryRes.SuccessCount != 3 {
		t.Errorf("Expected 3 successes in dry run, got %d", dryRes.SuccessCount)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c.Path); os.IsNotExist(err) {
			t.Errorf("Path %s was deleted during dry run!", c.Path)
		}
	}

	// 2. Real deletion
	realRes := DeleteCandidates(context.Background(), candidates, CleanOptions{
		DryRun:  false,
		Mode:    ModePermanent,
		Workers: 2,
	})
	if realRes.SuccessCount != 3 {
		t.Errorf("Expected 3 successes, got %d (failed: %d)", realRes.SuccessCount, realRes.FailedCount)
	}
	if realRes.TotalFreedBytes != 33 {
		t.Errorf("Expected 33 bytes freed, got %d", realRes.TotalFreedBytes)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c.Path); !os.IsNotExist(err) {
			t.Errorf("Path %s was NOT deleted during actual deletion!", c.Path)
		}
	}
}
