package matcher

import (
	"os"
	"path/filepath"
	"testing"

	"appcleaner/internal/appindex"
	"appcleaner/internal/cliindex"
)

func TestSystemAndDeveloperProtection(t *testing.T) {
	protected := []string{
		"AddressBook",
		"addressbook",
		"CrashReporter",
		"com.apple.TCC",
		"com.apple.sharedfilelist",
		"Dock",
		"Siri",
		"Quick Look",
	}

	for _, p := range protected {
		if !IsSystemProtected(p) {
			t.Errorf("Expected %q to be protected, but was not", p)
		}
	}

	devTools := []string{
		"go",
		"2fa",
		"git",
		"docker",
		"kubectl",
		"gh",
		"node",
		"cargo",
	}

	for _, dt := range devTools {
		if !IsDeveloperToolProtected(dt) {
			t.Errorf("Expected dev tool %q to be protected, but was not", dt)
		}
	}

	notProtected := []string{
		"BraveSoftware",
		"Slack",
		"Discord",
		"com.spotify.client",
	}

	for _, np := range notProtected {
		if IsSystemProtected(np) || IsDeveloperToolProtected(np) {
			t.Errorf("Expected %q NOT to be protected, but was", np)
		}
	}
}

func TestMatcherWithCLI(t *testing.T) {
	// Setup mock installed GUI apps
	installedApps := []*appindex.App{
		{
			Path:         "/Applications/Google Chrome.app",
			BundleName:   "Google Chrome",
			CFBundleName: "Chrome",
			BundleID:     "com.google.Chrome",
		},
	}
	appIdx := appindex.NewIndex(installedApps)

	// Create a temp directory for mock CLI binaries
	cliDir, err := os.MkdirTemp("", "test_cli_bin_*")
	if err != nil {
		t.Fatalf("Failed to create cli dir: %v", err)
	}
	defer os.RemoveAll(cliDir)

	// Create user-created CLI tool "my-custom-cli" and "2fa"
	toolPath := filepath.Join(cliDir, "my-custom-cli")
	os.WriteFile(toolPath, []byte("#!/bin/sh\necho hi"), 0755)

	cliIdx, err := cliindex.ScanInstalledCLITools(t.Context(), cliindex.ScanConfig{
		ExtraDirs: []string{cliDir},
	})
	if err != nil {
		t.Fatalf("Failed to scan CLI tools: %v", err)
	}

	m := NewMatcher(appIdx, cliIdx, []string{"my_custom_keep"})

	// Create a temp directory to simulate Application Support
	tmpDir, err := os.MkdirTemp("", "test_app_support_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Folders:
	// 1. Google Chrome (installed GUI -> skip)
	// 2. 2fa (developer tool / CLI -> skip)
	// 3. go (developer tool / CLI -> skip)
	// 4. my-custom-cli (user created CLI -> skip!)
	// 5. CrashReporter (system -> skip)
	// 6. Slack (uninstalled -> flag!)
	os.Mkdir(filepath.Join(tmpDir, "Google Chrome"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "2fa"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "go"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "my-custom-cli"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "CrashReporter"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "Slack"), 0755)

	// Test 2fa
	if c := m.AnalyzeDirectory(filepath.Join(tmpDir, "2fa")); len(c) != 0 {
		t.Errorf("Expected 2fa to be protected, but got candidate: %+v", c)
	}

	// Test go
	if c := m.AnalyzeDirectory(filepath.Join(tmpDir, "go")); len(c) != 0 {
		t.Errorf("Expected go to be protected, but got candidate: %+v", c)
	}

	// Test user created CLI: my-custom-cli
	if c := m.AnalyzeDirectory(filepath.Join(tmpDir, "my-custom-cli")); len(c) != 0 {
		t.Errorf("Expected user created CLI 'my-custom-cli' to be protected, but got candidate: %+v", c)
	}

	// Test CrashReporter
	if c := m.AnalyzeDirectory(filepath.Join(tmpDir, "CrashReporter")); len(c) != 0 {
		t.Errorf("Expected CrashReporter to be protected, got candidate")
	}

	// Test Slack (should be flagged)
	c := m.AnalyzeDirectory(filepath.Join(tmpDir, "Slack"))
	if len(c) != 1 {
		t.Fatalf("Expected Slack to be candidate, got %d candidates", len(c))
	}
	if c[0].AppName != "Slack" {
		t.Errorf("Expected app name 'Slack', got %q", c[0].AppName)
	}
}
