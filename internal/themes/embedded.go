package themes

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed themes/*.toml
var embeddedThemes embed.FS

// userThemesDir returns the directory a user can drop custom/override theme
// TOML files into: %APPDATA%\tukan\themes on Windows, ~/.config/tukan/themes
// elsewhere — mirrors internal/config's own os.UserConfigDir()-based
// DB/config path logic, kept independent here rather than importing
// internal/config, to avoid coupling this package to Tukan's app config.
func userThemesDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "tukan", "themes")
}

// GetTheme looks up a theme by slug (its TOML filename without extension).
// Lookup order: user override directory, then the embedded built-ins, then
// (only if name is empty or nothing else matched) the hardcoded default.
func GetTheme(name string) (*Theme, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return GetDefaultTheme(), nil
	}

	userPath := filepath.Join(userThemesDir(), name+".toml")
	if t, err := LoadTheme(userPath); err == nil {
		return t, nil
	}

	data, err := embeddedThemes.ReadFile("themes/" + name + ".toml")
	if err == nil {
		return parseTheme(data)
	}

	return nil, fmt.Errorf("theme %q not found", name)
}

// ListAvailableThemes returns every theme slug (embedded + user override,
// deduplicated, sorted) — what a theme switcher should offer.
func ListAvailableThemes() []string {
	seen := make(map[string]bool)
	var names []string

	entries, _ := embeddedThemes.ReadDir("themes")
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".toml")
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	if userEntries, err := os.ReadDir(userThemesDir()); err == nil {
		for _, e := range userEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".toml")
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	sort.Strings(names)
	return names
}

// GetThemeDisplayName returns a theme's human-readable Meta.Name, falling
// back to the slug itself if the theme can't be loaded.
func GetThemeDisplayName(slug string) string {
	t, err := GetTheme(slug)
	if err != nil || t.Meta.Name == "" {
		return slug
	}
	return t.Meta.Name
}
