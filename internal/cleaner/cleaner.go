package cleaner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"appcleaner/internal/matcher"
)

// DeleteMode whether to remove permanently or move to Trash.
type DeleteMode int

const (
	ModePermanent DeleteMode = iota
	ModeTrash
)

// CleanOptions configures folder deletion.
type CleanOptions struct {
	Mode       DeleteMode
	DryRun     bool
	Workers    int
	OnProgress func(completed, total int, cand *matcher.Candidate, err error)
}

// CleanResult summary of the deletion operation.
type CleanResult struct {
	TotalFreedBytes int64
	SuccessCount    int
	FailedCount     int
	Errors          []error
}

// DeleteCandidates concurrently deletes or trashes candidate directories recursively.
func DeleteCandidates(ctx context.Context, candidates []*matcher.Candidate, opts CleanOptions) *CleanResult {
	res := &CleanResult{}
	if len(candidates) == 0 {
		return res
	}

	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU() * 2
		if opts.Workers < 4 {
			opts.Workers = 4
		}
	}

	var (
		resMu        sync.Mutex
		completedNum int64
		total        = len(candidates)
	)

	jobs := make(chan *matcher.Candidate, total)
	for _, c := range candidates {
		jobs <- c
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cand := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				var err error
				if !opts.DryRun {
					if opts.Mode == ModeTrash {
						err = moveToTrash(cand.Path)
					} else {
						err = os.RemoveAll(cand.Path)
					}
				}

				done := int(atomic.AddInt64(&completedNum, 1))

				resMu.Lock()
				if err != nil {
					res.FailedCount++
					res.Errors = append(res.Errors, fmt.Errorf("%s: %w", cand.Path, err))
				} else {
					res.SuccessCount++
					res.TotalFreedBytes += cand.Size
				}
				resMu.Unlock()

				if opts.OnProgress != nil {
					opts.OnProgress(done, total, cand, err)
				}
			}
		}()
	}

	wg.Wait()
	return res
}

func moveToTrash(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.RemoveAll(path) // fallback to remove
	}
	trashDir := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trashDir, 0700); err != nil {
		return os.RemoveAll(path)
	}

	base := filepath.Base(path)
	target := filepath.Join(trashDir, base)

	// Avoid collision in Trash
	if _, err := os.Stat(target); err == nil {
		target = filepath.Join(trashDir, fmt.Sprintf("%s_%d", base, os.Getpid()))
	}

	if err := os.Rename(path, target); err != nil {
		// If cross-device or permission fails, fallback to RemoveAll
		return os.RemoveAll(path)
	}
	return nil
}
