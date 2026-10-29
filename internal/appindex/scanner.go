package appindex

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ScanConfig options for scanning installed applications.
type ScanConfig struct {
	Workers          int
	CustomAppDirs    []string
	UseSpotlight     bool
	LogVerbose       func(format string, args ...any)
}

// DefaultAppDirs standard macOS locations containing application bundles.
var DefaultAppDirs = []string{
	"/Applications",
	"/System/Applications",
	"/System/Applications/Utilities",
	"/Library/Apple/System/Library/CoreServices",
	"/System/Library/CoreServices",
}

// ScanInstalledApps concurrently discovers and parses all installed applications.
func ScanInstalledApps(ctx context.Context, cfg ScanConfig) (*Index, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU() * 2
		if cfg.Workers < 4 {
			cfg.Workers = 4
		}
	}
	if cfg.LogVerbose == nil {
		cfg.LogVerbose = func(string, ...any) {}
	}

	appPaths := make(map[string]struct{})
	var mu sync.Mutex

	addPath := func(p string) {
		p = filepath.Clean(p)
		if !strings.HasSuffix(p, ".app") {
			return
		}
		mu.Lock()
		appPaths[p] = struct{}{}
		mu.Unlock()
	}

	// 1. Scan standard directories
	dirsToScan := append([]string{}, DefaultAppDirs...)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirsToScan = append(dirsToScan, filepath.Join(home, "Applications"))
	}
	dirsToScan = append(dirsToScan, cfg.CustomAppDirs...)

	var dirWg sync.WaitGroup
	for _, dir := range dirsToScan {
		dir := dir
		dirWg.Add(1)
		go func() {
			defer dirWg.Done()
			scanDirForApps(dir, 3, addPath)
		}()
	}
	dirWg.Wait()

	// 2. Query Spotlight (mdfind) if enabled and available
	if cfg.UseSpotlight {
		if spotPaths, err := runSpotlightSearch(ctx); err == nil {
			for _, p := range spotPaths {
				addPath(p)
			}
		}
	}

	cfg.LogVerbose("Discovered %d candidate application bundles. Parsing concurrently...", len(appPaths))

	// 3. Worker pool to parse Info.plist files concurrently
	jobs := make(chan string, len(appPaths))
	for p := range appPaths {
		jobs <- p
	}
	close(jobs)

	var (
		workerWg sync.WaitGroup
		appMu    sync.Mutex
		apps     []*App
	)

	for i := 0; i < cfg.Workers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for path := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				app := parseAppBundle(path)
				if app != nil {
					appMu.Lock()
					apps = append(apps, app)
					appMu.Unlock()
				}
			}
		}()
	}

	workerWg.Wait()

	cfg.LogVerbose("Successfully indexed %d installed applications.", len(apps))
	return NewIndex(apps), nil
}

func scanDirForApps(root string, maxDepth int, addPath func(string)) {
	if maxDepth <= 0 {
		return
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(root, name)
		if strings.HasSuffix(name, ".app") {
			addPath(fullPath)
			// Do not descend into .app bundles looking for other apps
			continue
		}
		if entry.IsDir() {
			scanDirForApps(fullPath, maxDepth-1, addPath)
		}
	}
}

func runSpotlightSearch(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/mdfind", "kMDItemContentType == 'com.apple.application-bundle'")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var results []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.HasSuffix(line, ".app") {
			results = append(results, line)
		}
	}
	return results, scanner.Err()
}

func parseAppBundle(bundlePath string) *App {
	baseName := strings.TrimSuffix(filepath.Base(bundlePath), ".app")
	app := &App{
		Path:       bundlePath,
		BundleName: baseName,
	}

	plistPath := filepath.Join(bundlePath, "Contents", "Info.plist")
	dict, err := ParsePlistFile(plistPath)
	if err == nil && dict != nil {
		app.CFBundleName = dict["CFBundleName"]
		app.DisplayName = dict["CFBundleDisplayName"]
		app.BundleID = dict["CFBundleIdentifier"]
		app.Executable = dict["CFBundleExecutable"]
	}

	return app
}
