package ui

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.bytes)
		if got != tt.expected {
			t.Errorf("FormatBytes(%d) = %q, expected %q", tt.bytes, got, tt.expected)
		}
	}
}

func TestParseSelection(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected []int
		wantErr  bool
	}{
		{"1", 5, []int{0}, false},
		{"1, 3, 5", 5, []int{0, 2, 4}, false},
		{"2-4", 5, []int{1, 2, 3}, false},
		{"1, 3-5", 5, []int{0, 2, 3, 4}, false},
		{"all", 3, []int{0, 1, 2}, false},
		{"0", 5, nil, true},       // Out of bounds
		{"6", 5, nil, true},       // Out of bounds
		{"invalid", 5, nil, true}, // Invalid format
		{"4-2", 5, nil, true},     // Inverse range
	}

	for _, tt := range tests {
		got, err := ParseSelection(tt.input, tt.max)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSelection(%q, %d) error = %v, wantErr %v", tt.input, tt.max, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("ParseSelection(%q, %d) = %v, expected %v", tt.input, tt.max, got, tt.expected)
		}
	}
}

func TestPromptConfirmation(t *testing.T) {
	// Test "y" input
	in := strings.NewReader("y\n")
	var out strings.Builder
	action, sel := PromptConfirmation(in, &out, 3)
	if action != "y" || len(sel) != 3 {
		t.Errorf("PromptConfirmation with 'y' expected ('y', [0,1,2]), got (%q, %v)", action, sel)
	}

	// Test "n" input
	in = strings.NewReader("n\n")
	action, _ = PromptConfirmation(in, &out, 3)
	if action != "n" {
		t.Errorf("PromptConfirmation with 'n' expected 'n', got %q", action)
	}
}
