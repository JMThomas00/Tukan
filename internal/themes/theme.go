// Package themes holds Tukan's theme palettes: 45 themes ported from
// Concord (same 12-raw-color TOML format), plus the loading/lookup
// machinery. Only the raw palette is reused from Concord's format —
// Concord's theme files also carry a [semantic] block for its own
// chat-specific UI roles (sidebar/chat/input/presence), which Tukan's
// Theme struct deliberately doesn't declare a field for. go-toml/v2
// ignores unrecognized TOML tables, so the 45 files copy byte-for-byte
// with no transformation.
package themes

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// ThemeMeta describes a theme's identity.
type ThemeMeta struct {
	Name        string `toml:"name"`
	Author      string `toml:"author"`
	Variant     string `toml:"variant"`
	Description string `toml:"description"`
}

// ThemeColors is the raw 12-color palette every theme defines, in the same
// Dracula-derived naming scheme Concord uses.
type ThemeColors struct {
	Background  string `toml:"background"`
	CurrentLine string `toml:"current_line"`
	Selection   string `toml:"selection"`
	Foreground  string `toml:"foreground"`
	Comment     string `toml:"comment"`
	Red         string `toml:"red"`
	Orange      string `toml:"orange"`
	Yellow      string `toml:"yellow"`
	Green       string `toml:"green"`
	Cyan        string `toml:"cyan"`
	Purple      string `toml:"purple"`
	Pink        string `toml:"pink"`
}

// Theme is a named, loadable color palette.
type Theme struct {
	Meta   ThemeMeta   `toml:"meta"`
	Colors ThemeColors `toml:"colors"`
}

// LoadTheme parses a theme TOML file from disk (used for user-override
// themes; embedded themes are loaded via embedded.go instead).
func LoadTheme(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseTheme(data)
}

// parseTheme unmarshals a theme TOML document. Only [meta] and [colors] are
// declared on Theme, so a [semantic] block (present in files copied
// verbatim from Concord) is silently ignored rather than erroring.
func parseTheme(data []byte) (*Theme, error) {
	var t Theme
	if err := toml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse theme: %w", err)
	}
	return &t, nil
}

// GetDefaultTheme returns Tukan's original hardcoded Tokyo-Night-ish
// palette as a Theme value. Used as the last-resort fallback if the
// configured theme can't be found or loaded, so a fresh install (or any
// load failure) reproduces Tukan's original look with zero regression.
func GetDefaultTheme() *Theme {
	return &Theme{
		Meta: ThemeMeta{Name: "Tukan Default", Author: "Tukan", Variant: "dark"},
		Colors: ThemeColors{
			Background:  "#1a1b26",
			CurrentLine: "#414868",
			Selection:   "#2d3f76",
			Foreground:  "#c0caf5",
			Comment:     "#565f89",
			Red:         "#f7768e",
			Orange:      "#e0af68",
			Yellow:      "#e0af68",
			Green:       "#9ece6a",
			Cyan:        "#7dcfff",
			Purple:      "#bb9af7",
			Pink:        "#bb9af7",
		},
	}
}
