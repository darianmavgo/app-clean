package cliindex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// Tool represents an installed or user-created CLI executable.
type Tool struct {
	Name string
	Path string
}

// Normalize transforms a string into lowercase alphanumeric only.
func Normalize(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// Index holds discovered CLI tools and provides fast matching.
type Index struct {
	mu           sync.RWMutex
	tools        map[string]*Tool
	byNormalized map[string]*Tool
	searchDirs   []string
}

// ScanConfig options for scanning CLI tools.
type ScanConfig struct {
	ExtraDirs  []string
	LogVerbose func(format string, args ...any)
}

// DefaultCLIDirectories returns standard paths where CLI tools are installed or created.
func DefaultCLIDirectories() []string {
	var dirs []string
	seen := make(map[string]bool)

	addDir := func(d string) {
		d = filepath.Clean(d)
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}

	// 1. Add all directories in PATH
	pathEnv := os.Getenv("PATH")
	for _, p := range filepath.SplitList(pathEnv) {
		if p != "" {
			addDir(p)
		}
	}

	// 2. Add standard user binary and developer directories
	home, _ := os.UserHomeDir()
	if home != "" {
		addDir(filepath.Join(home, "bin"))
		addDir(filepath.Join(home, ".local", "bin"))
		addDir(filepath.Join(home, "go", "bin"))
		addDir(filepath.Join(home, ".cargo", "bin"))
		addDir(filepath.Join(home, ".bun", "bin"))
		addDir(filepath.Join(home, ".deno", "bin"))
		addDir(filepath.Join(home, ".antigravity-ide", "antigravity-ide", "bin"))
		addDir(filepath.Join(home, ".gemini", "antigravity", "bin"))
		addDir(filepath.Join(home, "google-cloud-sdk", "bin"))
	}

	// GOPATH/bin if set
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		addDir(filepath.Join(gopath, "bin"))
	}

	// Standard system & package manager binary directories
	addDir("/opt/homebrew/bin")
	addDir("/opt/homebrew/sbin")
	addDir("/usr/local/bin")
	addDir("/usr/local/sbin")
	addDir("/usr/local/go/bin")
	addDir("/usr/bin")
	addDir("/bin")

	return dirs
}

// ScanInstalledCLITools concurrently discovers all executable binaries.
func ScanInstalledCLITools(ctx context.Context, cfg ScanConfig) (*Index, error) {
	if cfg.LogVerbose == nil {
		cfg.LogVerbose = func(string, ...any) {}
	}

	dirs := DefaultCLIDirectories()
	for _, extra := range cfg.ExtraDirs {
		extra = filepath.Clean(strings.TrimSpace(extra))
		if extra != "" {
			dirs = append(dirs, extra)
		}
	}

	idx := &Index{
		tools:        make(map[string]*Tool),
		byNormalized: make(map[string]*Tool),
		searchDirs:   dirs,
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	cfg.LogVerbose("Scanning %d directories for installed CLI tools and user binaries...", len(dirs))

	for _, dir := range dirs {
		dir := dir
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				return
			}

			localTools := make([]*Tool, 0, len(entries))
			for _, entry := range entries {
				name := entry.Name()
				if strings.HasPrefix(name, ".") {
					continue
				}

				fullPath := filepath.Join(dir, name)
				info, err := entry.Info()
				if err != nil {
					continue
				}

				// Check if regular executable or symlink to executable
				mode := info.Mode()
				if mode.IsRegular() && (mode.Perm()&0111 != 0) {
					localTools = append(localTools, &Tool{Name: name, Path: fullPath})
				} else if mode&os.ModeSymlink != 0 {
					// Check symlink target
					if targetInfo, err := os.Stat(fullPath); err == nil && !targetInfo.IsDir() && (targetInfo.Mode().Perm()&0111 != 0) {
						localTools = append(localTools, &Tool{Name: name, Path: fullPath})
					}
				}
			}

			if len(localTools) > 0 {
				mu.Lock()
				for _, t := range localTools {
					idx.registerTool(t)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	cfg.LogVerbose("Indexed %d unique CLI executable tools.", len(idx.tools))
	return idx, nil
}

func (idx *Index) registerTool(t *Tool) {
	lower := strings.ToLower(t.Name)
	if _, exists := idx.tools[lower]; !exists {
		idx.tools[lower] = t
		idx.byNormalized[Normalize(t.Name)] = t
	}
}

// Match tests whether a directory name corresponds to an installed or user-created CLI tool.
func (idx *Index) Match(target string) *Tool {
	clean := strings.TrimSpace(target)
	if clean == "" {
		return nil
	}

	lower := strings.ToLower(clean)
	norm := Normalize(clean)

	idx.mu.RLock()
	// 1. Direct name match (e.g. "2fa", "go", "gh")
	if t, ok := idx.tools[lower]; ok {
		idx.mu.RUnlock()
		return t
	}

	// 2. Normalized match
	if t, ok := idx.byNormalized[norm]; ok {
		idx.mu.RUnlock()
		return t
	}

	// 3. Suffix variations (e.g. folder "foo-cli", "foo-cli-nodejs", or "foo_cli" -> binary "foo")
	for _, suffix := range []string{"-cli-nodejs", "-cli", "_cli", "-tool", "_tool"} {
		if strings.HasSuffix(lower, suffix) {
			base := strings.TrimSuffix(lower, suffix)
			if t, ok := idx.tools[base]; ok {
				idx.mu.RUnlock()
				return t
			}
		}
	}
	// Or folder "foo" -> binary "foo-cli"
	if t, ok := idx.tools[lower+"-cli"]; ok {
		idx.mu.RUnlock()
		return t
	}
	if t, ok := idx.tools[lower+"_cli"]; ok {
		idx.mu.RUnlock()
		return t
	}
	idx.mu.RUnlock()

	// 4. Dynamic fallback check via exec.LookPath
	if path, err := exec.LookPath(clean); err == nil && path != "" {
		t := &Tool{Name: clean, Path: path}
		idx.mu.Lock()
		idx.registerTool(t)
		idx.mu.Unlock()
		return t
	}
	if lower != clean {
		if path, err := exec.LookPath(lower); err == nil && path != "" {
			t := &Tool{Name: lower, Path: path}
			idx.mu.Lock()
			idx.registerTool(t)
			idx.mu.Unlock()
			return t
		}
	}

	return nil
}

// TotalCount returns the number of indexed CLI tools.
func (idx *Index) TotalCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.tools)
}
