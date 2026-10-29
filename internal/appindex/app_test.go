package appindex

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Google Chrome", "googlechrome"},
		{"com.google.Chrome", "comgooglechrome"},
		{"VS Code 1.85", "vscode185"},
		{"!@#Discord_App---", "discordapp"},
	}

	for _, tt := range tests {
		got := Normalize(tt.input)
		if got != tt.expected {
			t.Errorf("Normalize(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestIndexMatch(t *testing.T) {
	apps := []*App{
		{
			Path:         "/Applications/Google Chrome.app",
			BundleName:   "Google Chrome",
			CFBundleName: "Chrome",
			DisplayName:  "Google Chrome",
			BundleID:     "com.google.Chrome",
			Executable:   "Google Chrome",
		},
		{
			Path:         "/Applications/Slack.app",
			BundleName:   "Slack",
			CFBundleName: "Slack",
			BundleID:     "com.tinyspeck.slackmacgap",
		},
		{
			Path:         "/Applications/ExpressVPN.app",
			BundleName:   "ExpressVPN",
			BundleID:     "com.express.vpn",
		},
	}

	idx := NewIndex(apps)

	// Positive matches
	testCases := []string{
		"Google Chrome",
		"google chrome",
		"Chrome",
		"com.google.Chrome",
		"com.google.chrome",
		"Google/Chrome",
		"Slack",
		"slack",
		"com.tinyspeck.slackmacgap",
		"ExpressVPN",
		"com.express.vpn",
	}

	for _, tc := range testCases {
		if matched := idx.Match(tc); matched == nil {
			t.Errorf("Expected Match(%q) to find app, but got nil", tc)
		}
	}

	// Negative matches (uninstalled / unknown apps)
	uninstalledCases := []string{
		"BraveSoftware",
		"Mozilla",
		"NonExistentApp123",
		"com.unknown.software",
	}

	for _, tc := range uninstalledCases {
		if matched := idx.Match(tc); matched != nil {
			t.Errorf("Expected Match(%q) to be nil, but matched %s", tc, matched.BundleName)
		}
	}
}
