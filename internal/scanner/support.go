package scanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"appcleaner/internal/matcher"
)

// ScanTargetConfig configuration for scanning support directories.
type ScanTargetConfig struct {
	TargetDirs        []string
	IncludeCaches     bool
	IncludeContainers bool
	IncludeXcode      bool
	IncludePrefs      bool
	MinSizeBytes      int64
	Workers           int
	LogVerbose        func(format string, args ...any)
	PermissionWarn    func(path string, err error)
}

// DefaultSupportDirs returns the standard user and system Application Support directories.
func DefaultSupportDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Application Support"))
	}
	dirs = append(dirs, "/Library/Application Support")
	return dirs
}

// CacheDirs returns user and system Caches directories.
func CacheDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Caches"))
	}
	dirs = append(dirs, "/Library/Caches")
	return dirs
}

// ContainerDirs returns user Sandboxed Containers and Group Containers directories.
func ContainerDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, "Library", "Containers"),
			filepath.Join(home, "Library", "Group Containers"),
		)
	}
	return dirs
}

// PrefDirs returns user Preferences directory.
func PrefDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Preferences"))
	}
	return dirs
}

// XcodeCleanupTargets returns standard Xcode cache, build, and simulator directories safe for cleanup.
func XcodeCleanupTargets() []*matcher.Candidate {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}

	targets := []struct {
		name   string
		rel    string
		reason string
	}{
		{
			name:   "Xcode DerivedData",
			rel:    filepath.Join("Library", "Developer", "Xcode", "DerivedData"),
			reason: "Xcode intermediate build artifacts and indexes (safe to regenerate)",
		},
		{
			name:   "Xcode iOS DeviceSupport",
			rel:    filepath.Join("Library", "Developer", "Xcode", "iOS DeviceSupport"),
			reason: "Xcode iOS device debug symbols (re-downloaded when device is attached)",
		},
		{
			name:   "Xcode watchOS DeviceSupport",
			rel:    filepath.Join("Library", "Developer", "Xcode", "watchOS DeviceSupport"),
			reason: "Xcode watchOS device debug symbols",
		},
		{
			name:   "Xcode CoreSimulator Caches",
			rel:    filepath.Join("Library", "Developer", "CoreSimulator", "Caches"),
			reason: "Xcode Simulator runtime caches",
		},
		{
			name:   "Xcode CoreSimulator Devices",
			rel:    filepath.Join("Library", "Developer", "CoreSimulator", "Devices"),
			reason: "Xcode iOS/watchOS/tvOS Simulator device instances",
		},
	}

	var candidates []*matcher.Candidate
	for _, t := range targets {
		fullPath := filepath.Join(home, t.rel)
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
			candidates = append(candidates, &matcher.Candidate{
				AppName: t.name,
				Path:    fullPath,
				Reason:  t.reason,
			})
		}
	}
	return candidates
}

// ScanResult contains discovered candidates and any access warnings.
type ScanResult struct {
	Candidates []*matcher.Candidate
	Warnings   []string
}

// ScanTargets scans the configured directories concurrently for orphaned application support folders.
func ScanTargets(ctx context.Context, m *matcher.Matcher, cfg ScanTargetConfig) (*ScanResult, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU() * 2
		if cfg.Workers < 4 {
			cfg.Workers = 4
		}
	}
	if cfg.LogVerbose == nil {
		cfg.LogVerbose = func(string, ...any) {}
	}
	if cfg.PermissionWarn == nil {
		cfg.PermissionWarn = func(string, error) {}
	}

	dirsToScan := append([]string{}, cfg.TargetDirs...)
	if len(dirsToScan) == 0 {
		dirsToScan = DefaultSupportDirs()
		if cfg.IncludeCaches {
			dirsToScan = append(dirsToScan, CacheDirs()...)
		}
		if cfg.IncludeContainers {
			dirsToScan = append(dirsToScan, ContainerDirs()...)
		}
		if cfg.IncludePrefs {
			dirsToScan = append(dirsToScan, PrefDirs()...)
		}
	}

	var (
		resMu      sync.Mutex
		candidates []*matcher.Candidate
		warnings   []string
		dirWg      sync.WaitGroup
	)

	cfg.LogVerbose("Scanning %d target directories for orphaned folders...", len(dirsToScan))

	for _, dir := range dirsToScan {
		dir := dir
		dirWg.Add(1)
		go func() {
			defer dirWg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "operation not permitted") {
					warnMsg := fmt.Sprintf("Permission denied accessing %s (macOS Full Disk Access may be needed)", dir)
					resMu.Lock()
					warnings = append(warnings, warnMsg)
					resMu.Unlock()
					cfg.PermissionWarn(dir, err)
				}
				return
			}

			var localCandidates []*matcher.Candidate
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				name := entry.Name()
				if strings.HasPrefix(name, ".") {
					continue
				}

				fullPath := filepath.Join(dir, name)
				cands := m.AnalyzeDirectory(fullPath)
				if len(cands) > 0 {
					localCandidates = append(localCandidates, cands...)
				}
			}

			if len(localCandidates) > 0 {
				resMu.Lock()
				candidates = append(candidates, localCandidates...)
				resMu.Unlock()
			}
		}()
	}

	dirWg.Wait()

	// Append Xcode cleanup targets if enabled
	if cfg.IncludeXcode && len(cfg.TargetDirs) == 0 {
		xcodeCands := XcodeCleanupTargets()
		candidates = append(candidates, xcodeCands...)
	}

	cfg.LogVerbose("Discovered %d candidate folders. Calculating sizes concurrently...", len(candidates))

	// Concurrently calculate folder sizes and gather sample files
	CalculateSizesConcurrently(ctx, candidates, cfg.Workers)

	// Filter out 0-byte empty directories and respect MinSizeBytes
	var filtered []*matcher.Candidate
	for _, c := range candidates {
		if c.Size > 0 && c.Size >= cfg.MinSizeBytes {
			filtered = append(filtered, c)
		}
	}

	return &ScanResult{
		Candidates: filtered,
		Warnings:   warnings,
	}, nil
}
