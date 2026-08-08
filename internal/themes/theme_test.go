package themes

import (
	"regexp"
	"testing"
)

// validColorValue matches every color format lipgloss.Color actually
// accepts that these theme files use: a #RRGGBB hex string, a bare ANSI
// palette index (0-255, as used by e.g. terminal-default.toml), or an
// empty string (terminal-default.toml's deliberate "inherit the
// terminal's own background/foreground" value for those two fields).
var validColorValue = regexp.MustCompile(`^(#[0-9a-fA-F]{6}|[0-9]{1,3})?$`)

func TestListAvailableThemesIsNonEmpty(t *testing.T) {
	names := ListAvailableThemes()
	if len(names) < 40 {
		t.Fatalf("len(ListAvailableThemes()) = %d, want at least 40 (45 themes ported from Concord)", len(names))
	}
}

// TestEveryThemeLoadsWithValidColors loads every available theme and
// asserts all 12 raw palette fields parse as valid hex colors — this is
// the guard that would have caught an embed-path mistake or a botched
// copy of one of the 45 source TOML files immediately.
func TestEveryThemeLoadsWithValidColors(t *testing.T) {
	for _, name := range ListAvailableThemes() {
		t.Run(name, func(t *testing.T) {
			theme, err := GetTheme(name)
			if err != nil {
				t.Fatalf("GetTheme(%q): %v", name, err)
			}
			if theme.Meta.Name == "" {
				t.Errorf("theme %q has an empty Meta.Name", name)
			}

			fields := map[string]string{
				"background":   theme.Colors.Background,
				"current_line": theme.Colors.CurrentLine,
				"selection":    theme.Colors.Selection,
				"foreground":   theme.Colors.Foreground,
				"comment":      theme.Colors.Comment,
				"red":          theme.Colors.Red,
				"orange":       theme.Colors.Orange,
				"yellow":       theme.Colors.Yellow,
				"green":        theme.Colors.Green,
				"cyan":         theme.Colors.Cyan,
				"purple":       theme.Colors.Purple,
				"pink":         theme.Colors.Pink,
			}
			for field, value := range fields {
				if !validColorValue.MatchString(value) {
					t.Errorf("theme %q field %q = %q, want a #RRGGBB hex color, a bare ANSI index, or empty", name, field, value)
				}
			}
		})
	}
}

func TestGetThemeUnknownNameErrors(t *testing.T) {
	if _, err := GetTheme("definitely-not-a-real-theme"); err == nil {
		t.Fatal("expected an error for an unknown theme name")
	}
}

func TestGetThemeEmptyNameReturnsDefault(t *testing.T) {
	theme, err := GetTheme("")
	if err != nil {
		t.Fatalf("GetTheme(\"\"): %v", err)
	}
	if theme.Meta.Name != GetDefaultTheme().Meta.Name {
		t.Fatalf("GetTheme(\"\") = %+v, want the default theme", theme)
	}
}

func TestGetThemeDisplayNameFallsBackToSlug(t *testing.T) {
	if got := GetThemeDisplayName("definitely-not-a-real-theme"); got != "definitely-not-a-real-theme" {
		t.Fatalf("GetThemeDisplayName for an unknown slug = %q, want the slug itself as fallback", got)
	}
}

func TestGetThemeDisplayNameKnownTheme(t *testing.T) {
	got := GetThemeDisplayName("dracula")
	if got != "Dracula" {
		t.Fatalf("GetThemeDisplayName(\"dracula\") = %q, want %q", got, "Dracula")
	}
}
