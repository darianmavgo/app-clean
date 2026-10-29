package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"appcleaner/internal/matcher"
)

// CalculateSizesConcurrently computes folder sizes and file counts across candidate folders using a worker pool.
func CalculateSizesConcurrently(ctx context.Context, candidates []*matcher.Candidate, workers int) {
	if len(candidates) == 0 {
		return
	}
	if workers <= 0 {
		workers = runtime.NumCPU() * 2
		if workers < 4 {
			workers = 4
		}
	}

	jobs := make(chan *matcher.Candidate, len(candidates))
	for _, c := range candidates {
		jobs <- c
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cand := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				populateSizeAndSamples(cand)
			}
		}()
	}
	wg.Wait()
}

func populateSizeAndSamples(cand *matcher.Candidate) {
	var totalSize int64
	var fileCount int
	var samples []string

	_ = filepath.WalkDir(cand.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // continue walking other files even if individual file has permission issue
		}

		if !d.IsDir() {
			fileCount++
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
			}
			if len(samples) < 5 {
				rel, err := filepath.Rel(cand.Path, path)
				if err == nil && !strings.HasPrefix(rel, ".") {
					samples = append(samples, rel)
				}
			}
		}
		return nil
	})

	cand.Size = totalSize
	cand.FileCount = fileCount
	cand.SampleFiles = samples
}
