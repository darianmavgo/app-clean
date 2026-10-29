package matcher

import (
	"strings"
)

// ProtectedSystemFolders list of macOS system and framework directories
// that must NEVER be flagged for deletion.
var ProtectedSystemFolders = map[string]struct{}{
	"addressbook":                 {},
	"animoji":                     {},
	"apple":                       {},
	"applepushservice":            {},
	"appstore":                    {},
	"arfilecache":                 {},
	"automator":                   {},
	"biome":                       {},
	"btserver":                    {},
	"callhistorydb":               {},
	"callhistorytransactions":     {},
	"carkey":                      {},
	"certificateprotection":       {},
	"clouddocs":                   {},
	"cloudkit":                    {},
	"colorsync":                   {},
	"conversationcache":           {},
	"coredata":                    {},
	"coreparsec":                  {},
	"crashreporter":               {},
	"desktop pictures":            {},
	"desktoppictures":             {},
	"diagnosticreports":           {},
	"differentialprivacy":         {},
	"dock":                        {},
	"facetime":                    {},
	"family":                      {},
	"familycircle":                {},
	"familycircled":               {},
	"feedbackreporter":            {},
	"fileprovider":                {},
	"fontbook":                    {},
	"gamekit":                     {},
	"geoservices":                 {},
	"health":                      {},
	"home":                        {},
	"homekit":                     {},
	"icloud":                      {},
	"identityservicesd":           {},
	"ilifemediabrowser":           {},
	"imdpersistence":              {},
	"jetpackcache":                {},
	"knowledge":                   {},
	"livefsd":                     {},
	"lsmimagecache":               {},
	"mail":                        {},
	"maps":                        {},
	"mbuseragent":                 {},
	"messages":                    {},
	"mobilesync":                  {},
	"music":                       {},
	"notificationcenter":          {},
	"opensso":                     {},
	"passbooks":                   {},
	"passkit":                     {},
	"photos":                      {},
	"podcasts":                    {},
	"proapps":                     {},
	"quick look":                  {},
	"quicklook":                   {},
	"reminders":                   {},
	"safari":                      {},
	"screen time":                 {},
	"screentime":                  {},
	"script editor":               {},
	"scripteditor":                {},
	"sharedimagecache":            {},
	"siri":                        {},
	"soundflower":                 {},
	"spindump":                    {},
	"spotlight":                   {},
	"stocks":                      {},
	"suggestd":                    {},
	"syncservices":                {},
	"syncserviceslocalpush":       {},
	"system":                      {},
	"systemstatus":                {},
	"telephonyutilities":          {},
	"textinput":                   {},
	"tv":                          {},
	"useractivity":                {},
	"voicememos":                  {},
	"weather":                     {},
}

// ProtectedDeveloperTools common programming languages, package managers,
// and developer CLI ecosystems that store configs/state in Application Support & Caches.
var ProtectedDeveloperTools = map[string]struct{}{
	"2fa":                         {},
	"agy":                         {},
	"antigravity":                 {},
	"aws":                         {},
	"az":                          {},
	"azure":                       {},
	"bash":                        {},
	"brew":                        {},
	"bun":                         {},
	"cargo":                       {},
	"claude":                      {},
	"colima":                      {},
	"composer":                    {},
	"containerd":                  {},
	"deno":                        {},
	"docker":                      {},
	"docker-secrets-engine":       {},
	"dotnet":                      {},
	"duckdb":                      {},
	"emacs":                       {},
	"fish":                        {},
	"flutter":                     {},
	"flutter_engine":              {},
	"gcloud":                      {},
	"gem":                         {},
	"gemini":                      {},
	"gh":                          {},
	"git":                         {},
	"github":                      {},
	"glab":                        {},
	"glow":                        {},
	"gnupg":                       {},
	"go":                          {},
	"go-build":                    {},
	"goimports":                   {},
	"golang":                      {},
	"gopls":                       {},
	"gpg":                         {},
	"gradle":                      {},
	"hatch-image-thumbnail-cache": {},
	"helm":                        {},
	"homebrew":                    {},
	"hugo":                        {},
	"hugo_cache":                  {},
	"io.sentry":                   {},
	"java":                        {},
	"jdk":                         {},
	"jna":                         {},
	"k9s":                         {},
	"kubectl":                     {},
	"maven":                       {},
	"minikube":                    {},
	"ms-playwright":               {},
	"ms-playwright-go":            {},
	"mysql":                       {},
	"neovim":                      {},
	"node":                        {},
	"node-gyp":                    {},
	"npm":                         {},
	"nuget":                       {},
	"nvm":                         {},
	"nvim":                        {},
	"ollama":                      {},
	"org.swift.swiftpm":           {},
	"php":                         {},
	"pip":                         {},
	"pipx":                        {},
	"pnpm":                        {},
	"podman":                      {},
	"postgres":                    {},
	"postgresql":                  {},
	"pulumi":                      {},
	"py-yfinance":                 {},
	"python":                      {},
	"python3":                     {},
	"redis":                       {},
	"ripgrep-all":                 {},
	"ruby":                        {},
	"rust":                        {},
	"rustup":                      {},
	"sqlite":                      {},
	"sqlite3":                     {},
	"swiftpm":                     {},
	"terraform":                   {},
	"terragrunt":                  {},
	"tmux":                        {},
	"tofu":                        {},
	"typescript":                  {},
	"vim":                         {},
	"virtualenv":                  {},
	"yarn":                        {},
	"zsh":                         {},
}

// IsSystemProtected checks if a folder name belongs to a macOS protected system service.
func IsSystemProtected(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))

	// Any com.apple.*, group.com.apple.*, or *.com.apple.* container/bundle
	if strings.HasPrefix(lower, "com.apple.") ||
		lower == "com.apple" ||
		strings.HasPrefix(lower, "group.com.apple.") ||
		strings.Contains(lower, ".com.apple.") ||
		strings.HasPrefix(lower, "group.is.workflow.") ||
		strings.HasPrefix(lower, "group.tvappservices.") {
		return true
	}

	// Protect system UUID containers (e.g. 29AEFFF9-EF3A-4037-8A67-9F96055B0C9C)
	if isUUIDFormat(lower) {
		return true
	}

	// Any folder ending in .savedState or system logs
	if strings.HasSuffix(lower, ".savedstate") {
		if strings.Contains(lower, "apple") {
			return true
		}
	}

	if _, ok := ProtectedSystemFolders[lower]; ok {
		return true
	}

	// Check without spaces
	stripped := strings.ReplaceAll(lower, " ", "")
	if _, ok := ProtectedSystemFolders[stripped]; ok {
		return true
	}

	return false
}

// IsDeveloperToolProtected checks if a folder name belongs to a protected developer tool or language runtime.
func IsDeveloperToolProtected(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if _, ok := ProtectedDeveloperTools[lower]; ok {
		return true
	}
	stripped := strings.ReplaceAll(lower, " ", "")
	if _, ok := ProtectedDeveloperTools[stripped]; ok {
		return true
	}
	return false
}

func isUUIDFormat(s string) bool {
	// 8-4-4-4-12 = 36 chars
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
