package appindex

import (
	"path/filepath"
	"strings"
	"unicode"
)

// App represents an installed macOS application bundle and its metadata.
type App struct {
	Path         string
	BundleName   string // e.g. "Google Chrome" from "Google Chrome.app"
	CFBundleName string
	DisplayName  string
	BundleID     string
	Executable   string
}

// Normalize transforms a string into lowercase alphanumeric only.
// e.g. "Google Chrome", "google-chrome", "Google_Chrome" -> "googlechrome"
func Normalize(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// Index provides fast multi-attribute lookup for installed applications.
type Index struct {
	Apps          []*App
	byBundleID    map[string]*App
	byExactName   map[string]*App
	byNormalized  map[string]*App
}

// NewIndex builds an Index from a slice of installed Apps.
func NewIndex(apps []*App) *Index {
	idx := &Index{
		Apps:         apps,
		byBundleID:   make(map[string]*App),
		byExactName:  make(map[string]*App),
		byNormalized: make(map[string]*App),
	}

	for _, app := range apps {
		idx.registerApp(app)
	}

	return idx
}

func (idx *Index) registerApp(app *App) {
	candidates := []string{
		app.BundleName,
		app.CFBundleName,
		app.DisplayName,
		app.Executable,
	}

	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		idx.byExactName[strings.ToLower(c)] = app
		idx.byNormalized[Normalize(c)] = app
	}

	if app.BundleID != "" {
		bid := strings.TrimSpace(strings.ToLower(app.BundleID))
		idx.byBundleID[bid] = app
		idx.byNormalized[Normalize(bid)] = app

		// Also register the last component of the bundle ID (e.g. "Chrome" in "com.google.Chrome")
		parts := strings.Split(bid, ".")
		if len(parts) > 1 {
			last := parts[len(parts)-1]
			if len(last) > 2 {
				if _, exists := idx.byExactName[last]; !exists {
					idx.byExactName[last] = app
				}
				normLast := Normalize(last)
				if _, exists := idx.byNormalized[normLast]; !exists {
					idx.byNormalized[normLast] = app
				}
			}
		}
	}

	// Register bundle folder basename without extension
	base := strings.TrimSuffix(filepath.Base(app.Path), ".app")
	if base != "" {
		idx.byExactName[strings.ToLower(base)] = app
		idx.byNormalized[Normalize(base)] = app
	}
}

// Match checks if an Application Support, Cache, or Container directory belongs to an installed app.
// Returns the matched App if found, or nil if not installed.
func (idx *Index) Match(target string) *App {
	clean := strings.TrimSpace(target)
	if clean == "" {
		return nil
	}

	lower := strings.ToLower(clean)
	norm := Normalize(clean)

	// 1. Direct bundle ID check
	if app, ok := idx.byBundleID[lower]; ok {
		return app
	}

	// 2. Exact name check (case-insensitive)
	if app, ok := idx.byExactName[lower]; ok {
		return app
	}

	// 3. Normalized check (including singular/plural trailing 's')
	if app, ok := idx.byNormalized[norm]; ok {
		return app
	}
	if strings.HasSuffix(norm, "s") && len(norm) > 4 {
		if app, ok := idx.byNormalized[strings.TrimSuffix(norm, "s")]; ok {
			return app
		}
	}

	// 4. Container & Group Container bundle ID prefix/substring checks:
	// Examples:
	// - "com.amazon.Lassen.SendToKindleExtension" (starts with installed "com.amazon.lassen.")
	// - "group.com.amazon.Lassen" or "group.net.whatsapp.WhatsApp.shared"
	// - "PZYM8XX95Q.com.automattic.SimplenoteMac" (TeamID prefix)
	strippedGroup := stripGroupAndTeamPrefix(lower)
	if strippedGroup != lower {
		if app := idx.Match(strippedGroup); app != nil {
			return app
		}
	}

	for bid, app := range idx.byBundleID {
		if strings.HasPrefix(lower, bid+".") || strings.HasPrefix(strippedGroup, bid+".") {
			return app
		}
		// Group container family match (e.g. group.net.whatsapp.family when net.whatsapp.WhatsApp is installed)
		bidParts := strings.Split(bid, ".")
		if len(bidParts) >= 3 {
			vendorPrefix := bidParts[0] + "." + bidParts[1] + "."
			if strings.HasPrefix(strippedGroup, vendorPrefix) {
				return app
			}
		}
	}

	// 5. If target is a path like "Vendor/AppName" (e.g. "Google/Chrome")
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) > 1 {
		lastPart := parts[len(parts)-1]
		if app := idx.Match(lastPart); app != nil {
			return app
		}
		combined := strings.Join(parts, "")
		if app := idx.Match(combined); app != nil {
			return app
		}
	}

	return nil
}

// stripGroupAndTeamPrefix strips "group." or 10-char Apple Team ID prefixes like "PZYM8XX95Q."
func stripGroupAndTeamPrefix(s string) string {
	res := s
	// Check for 10-character alphanumeric Team ID prefix (e.g. "pzym8xx95q.com.automattic...")
	if idx := strings.IndexByte(res, '.'); idx == 10 {
		prefix := res[:10]
		isAlnum := true
		for _, r := range prefix {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				isAlnum = false
				break
			}
		}
		if isAlnum {
			res = res[11:]
		}
	}
	res = strings.TrimPrefix(res, "groups.")
	res = strings.TrimPrefix(res, "group.")
	return res
}
