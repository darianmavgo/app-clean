package matcher

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"appcleaner/internal/appindex"
	"appcleaner/internal/cliindex"
)

// Candidate represents an orphaned support folder identified for potential deletion.
type Candidate struct {
	AppName     string   // Inferred human-friendly application name
	Path        string   // Full path to the directory
	Reason      string   // Explanation of why it's orphaned
	Size        int64    // Total size in bytes
	FileCount   int      // Total number of files
	SampleFiles []string // First few files for preview/details
}

// KnownVendors that typically group multiple applications in subdirectories.
var KnownVendors = map[string]struct{}{
	"adobe":       {},
	"google":      {},
	"microsoft":   {},
	"jetbrains":   {},
	"mozilla":     {},
	"oracle":      {},
	"vmware":      {},
	"autodesk":    {},
	"valve":       {},
	"riot games":  {},
	"epic games":  {},
}

// Matcher evaluates directories against installed applications and CLI tools.
type Matcher struct {
	index    *appindex.Index
	cliIndex *cliindex.Index
	excludes map[string]struct{}
}

// NewMatcher creates a new Matcher with an app index, CLI index, and optional user excludes.
func NewMatcher(index *appindex.Index, cliIndex *cliindex.Index, customExcludes []string) *Matcher {
	ex := make(map[string]struct{})
	for _, e := range customExcludes {
		e = strings.TrimSpace(strings.ToLower(e))
		if e != "" {
			ex[e] = struct{}{}
		}
	}
	return &Matcher{
		index:    index,
		cliIndex: cliIndex,
		excludes: ex,
	}
}

// IsExcluded checks if a name or path is in the user's custom exclusion list.
func (m *Matcher) IsExcluded(name, path string) bool {
	lowerName := strings.ToLower(name)
	if _, ok := m.excludes[lowerName]; ok {
		return true
	}
	for ex := range m.excludes {
		if strings.Contains(strings.ToLower(path), ex) {
			return true
		}
	}
	return false
}

// AnalyzeDirectory examines a folder entry in Application Support, Caches, or Containers.
// It returns any candidate orphaned directories found (either the folder itself, or subfolders for vendors).
func (m *Matcher) AnalyzeDirectory(fullPath string) []*Candidate {
	folderName := filepath.Base(fullPath)

	// 1. Check system protection, developer tools whitelist, and user exclusions
	if IsSystemProtected(folderName) || IsDeveloperToolProtected(folderName) || m.IsExcluded(folderName, fullPath) {
		return nil
	}

	lowerName := strings.ToLower(folderName)

	// 2. Check if this is a known multi-application vendor directory
	if _, isVendor := KnownVendors[lowerName]; isVendor {
		return m.analyzeVendorDirectory(fullPath, folderName)
	}

	// 3. Check if directly matches an installed GUI application (.app bundle)
	if m.index != nil {
		if app := m.index.Match(folderName); app != nil {
			return nil
		}
	}

	// 4. Check if matches an installed or user-created CLI tool in PATH or bin directories
	if m.cliIndex != nil {
		if tool := m.cliIndex.Match(folderName); tool != nil {
			return nil
		}
		// Also check last segment if reverse-DNS (e.g. group.com.docker -> docker)
		parts := strings.Split(folderName, ".")
		if len(parts) >= 2 {
			lastPart := parts[len(parts)-1]
			if IsDeveloperToolProtected(lastPart) || m.cliIndex.Match(lastPart) != nil {
				return nil
			}
		}
	}

	// 5. Candidate orphan found!
	friendlyName := cleanAppName(folderName)
	return []*Candidate{
		{
			AppName: friendlyName,
			Path:    fullPath,
			Reason:  "No installed application or CLI tool matches name or bundle ID",
		},
	}
}

// analyzeVendorDirectory inspects vendor subfolders (e.g. Google/Chrome, Adobe/Acrobat)
func (m *Matcher) analyzeVendorDirectory(vendorPath, vendorName string) []*Candidate {
	entries, err := os.ReadDir(vendorPath)
	if err != nil {
		return nil
	}

	var candidates []*Candidate
	hasInstalledApp := false
	subfolderCount := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subName := entry.Name()
		if strings.HasPrefix(subName, ".") {
			continue
		}
		subfolderCount++
		subPath := filepath.Join(vendorPath, subName)

		if IsSystemProtected(subName) || IsDeveloperToolProtected(subName) || m.IsExcluded(subName, subPath) {
			hasInstalledApp = true
			continue
		}

		// Try matching GUI app: Vendor/SubName or just SubName
		combined := vendorName + "/" + subName
		if m.index != nil {
			if app := m.index.Match(combined); app != nil {
				hasInstalledApp = true
				continue
			}
			if app := m.index.Match(subName); app != nil {
				hasInstalledApp = true
				continue
			}
		}

		// Try matching CLI tool
		if m.cliIndex != nil {
			if tool := m.cliIndex.Match(subName); tool != nil {
				hasInstalledApp = true
				continue
			}
			if tool := m.cliIndex.Match(combined); tool != nil {
				hasInstalledApp = true
				continue
			}
		}

		// Subfolder is orphaned
		candidates = append(candidates, &Candidate{
			AppName: vendorName + " " + cleanAppName(subName),
			Path:    subPath,
			Reason:  "Uninstalled component of " + vendorName,
		})
	}

	// If the entire vendor directory has NO installed apps and all subfolders are orphaned
	// (or empty), return the entire vendor directory or its subfolders
	if !hasInstalledApp && len(candidates) == subfolderCount && subfolderCount > 0 {
		return []*Candidate{
			{
				AppName: vendorName + " (All Products)",
				Path:    vendorPath,
				Reason:  "No installed applications for vendor " + vendorName,
			},
		}
	}

	return candidates
}

// cleanAppName generates a human-friendly application name from a folder name or bundle ID.
func cleanAppName(name string) string {
	// Strip leading 10-char Team ID or "group." prefix if present
	clean := name
	if idx := strings.IndexByte(clean, '.'); idx == 10 {
		clean = clean[11:]
	}
	clean = strings.TrimPrefix(clean, "groups.")
	clean = strings.TrimPrefix(clean, "group.")

	parts := strings.Split(clean, ".")
	if len(parts) >= 2 && (parts[0] == "com" || parts[0] == "org" || parts[0] == "net" || parts[0] == "io" || parts[0] == "is") {
		var meaningful []string
		for _, p := range parts[1:] {
			p = strings.TrimSpace(p)
			if p != "" {
				r := []rune(p)
				r[0] = unicode.ToUpper(r[0])
				meaningful = append(meaningful, string(r))
			}
		}
		if len(meaningful) > 0 {
			return strings.Join(meaningful, " ")
		}
	}
	return name
}
