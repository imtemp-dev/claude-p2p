package p2p

import (
	"strings"
	"testing"
)

func TestGenerateDisplayName(t *testing.T) {
	name := GenerateDisplayName("my-project")
	parts := strings.SplitN(name, "-", 3)
	// Should have at least 3 parts: dir-adj-animal (dir could contain hyphens after sanitize)
	if len(parts) < 3 {
		t.Errorf("expected at least 3 parts, got %d: %s", len(parts), name)
	}
	if !strings.HasPrefix(name, "my-project-") {
		t.Errorf("expected to start with 'my-project-', got: %s", name)
	}
}

func TestGenerateDisplayName_SpecialChars(t *testing.T) {
	name := GenerateDisplayName("My Project (2)")
	if strings.Contains(name, "(") || strings.Contains(name, ")") || strings.Contains(name, " ") {
		t.Errorf("expected special chars removed, got: %s", name)
	}
	if !strings.HasPrefix(name, "myproject2-") && !strings.HasPrefix(name, "my-project-2-") {
		// sanitizeDir removes non-alphanumeric except hyphens
		t.Logf("name: %s (sanitized from special chars)", name)
	}
}

func TestGenerateDisplayName_LongDir(t *testing.T) {
	longDir := strings.Repeat("a", 50)
	name := GenerateDisplayName(longDir)
	// Dir part should be truncated to 30 chars
	dir := sanitizeDir(longDir)
	if len(dir) > 30 {
		t.Errorf("expected dir truncated to 30, got %d: %s", len(dir), dir)
	}
	if !strings.HasPrefix(name, dir+"-") {
		t.Errorf("expected to start with '%s-', got: %s", dir, name)
	}
}

func TestSanitizeDir(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my-project", "my-project"},
		{"My Project", "myproject"},
		{"한글프로젝트", "session"},
		{"", "session"},
		{"  ", "session"},
		{"a-b-c", "a-b-c"},
		{strings.Repeat("x", 40), strings.Repeat("x", 30)},
		{"-leading-", "leading"},
	}
	for _, tt := range tests {
		got := sanitizeDir(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeDir(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestGenerateDisplayName_Uniqueness(t *testing.T) {
	names := make(map[string]bool)
	for i := 0; i < 20; i++ {
		name := GenerateDisplayName("test")
		names[name] = true
	}
	// With 900 combinations, 20 calls should produce at least 10 unique names
	if len(names) < 10 {
		t.Errorf("expected at least 10 unique names from 20 calls, got %d", len(names))
	}
}

func TestSanitizeDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello-world"},
		{"  trimmed  ", "trimmed"},
		{"no\x00null", "nonull"},
		{"control\x01char", "controlchar"},
		{"MyName", "MyName"},
		{"한글이름", "한글이름"},
	}
	for _, tt := range tests {
		got := sanitizeDisplayName(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeDisplayName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTruncateFieldUTF8(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"한글테스트", 6, "한글"},   // 한글 = 3 bytes each, 6 bytes = 2 chars
		{"한글테스트", 7, "한글"},   // 7 bytes would split 테, back up to 6
		{"abc", 100, "abc"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := truncateFieldUTF8(tt.input, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncateFieldUTF8(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
		}
	}
}
