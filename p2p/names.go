package p2p

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxDisplayNameLength = 100

var adjectives = []string{
	"swift", "bright", "calm", "bold", "keen",
	"warm", "cool", "fair", "glad", "wise",
	"blue", "red", "green", "gold", "silver",
	"quiet", "brave", "quick", "sharp", "light",
	"proud", "kind", "free", "pure", "deep",
	"wild", "soft", "dark", "pale", "crisp",
}

var animals = []string{
	"fox", "owl", "wolf", "bear", "deer",
	"hawk", "lynx", "puma", "seal", "wren",
	"crow", "dove", "hare", "moth", "newt",
	"pike", "swan", "toad", "vole", "wasp",
	"orca", "ibis", "kiwi", "mule", "lion",
	"crab", "frog", "goat", "lark", "mink",
}

var dirSanitizeRegex = regexp.MustCompile(`[^a-z0-9-]`)

// GenerateDisplayName creates a human-readable name: {dir}-{adj}-{animal}.
func GenerateDisplayName(dir string) string {
	d := sanitizeDir(dir)
	adj := adjectives[rand.IntN(len(adjectives))]
	animal := animals[rand.IntN(len(animals))]
	return d + "-" + adj + "-" + animal
}

// sanitizeDir lowercases, keeps [a-z0-9-], truncates to 30 chars.
// Returns "session" if result is empty after sanitization.
func sanitizeDir(dir string) string {
	s := strings.ToLower(dir)
	s = dirSanitizeRegex.ReplaceAllString(s, "")
	if len(s) > 30 {
		s = s[:30]
	}
	s = strings.Trim(s, "-")
	if s == "" {
		return "session"
	}
	return s
}

// loadOrGenerateDisplayName loads a saved name from the per-cwd identity directory,
// or generates a new one and saves it.
func loadOrGenerateDisplayName(dir string) string {
	idDir := identityDir()
	if idDir == "" {
		return GenerateDisplayName(dir)
	}

	nameFile := filepath.Join(idDir, "name")
	data, err := os.ReadFile(nameFile)
	if err == nil {
		name := strings.TrimSpace(string(data))
		if name != "" {
			return name
		}
	}

	name := GenerateDisplayName(dir)
	os.MkdirAll(idDir, 0700)
	os.WriteFile(nameFile, []byte(name), 0600)
	return name
}

// sanitizeDisplayName cleans a user-provided display name.
func sanitizeDisplayName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0 {
			continue
		}
		b.WriteRune(r)
	}
	s := strings.TrimSpace(b.String())
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// truncateFieldUTF8 truncates a string to maxLen bytes without splitting
// multi-byte UTF-8 characters.
func truncateFieldUTF8(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Back up to last valid rune boundary
	for maxLen > 0 && !utf8.RuneStart(s[maxLen]) {
		maxLen--
	}
	return s[:maxLen]
}
