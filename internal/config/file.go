package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// FileConfig holds the user-editable settings persisted to disk, as
// opposed to Config (config.go), which is built once in-process and never
// written to a file.
type FileConfig struct {
	// Theme is a theme slug (e.g. "tokyo-night"); empty means the built-in
	// default (see internal/themes.GetDefaultTheme).
	Theme string `toml:"theme"`
}

// ConfigPath returns the path to Tukan's user config file:
// %APPDATA%\tukan\config.toml on Windows, ~/.config/tukan/config.toml
// elsewhere — same base directory defaultDBPath uses for tukan.db.
func ConfigPath() string {
	return filepath.Join(tukanDir(), "config.toml")
}

// ThemesDir returns the directory a user can drop custom/override theme
// TOML files into: %APPDATA%\tukan\themes.
func ThemesDir() string {
	return filepath.Join(tukanDir(), "themes")
}

func tukanDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "tukan")
}

// LoadFileConfig reads config.toml, creating it with zero-value defaults
// if it doesn't exist yet. Returns the loaded (or newly created) config.
func LoadFileConfig() (FileConfig, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fc := FileConfig{}
		if err := fc.Save(); err != nil {
			return fc, err
		}
		return fc, nil
	}
	if err != nil {
		return FileConfig{}, err
	}

	var fc FileConfig
	if err := toml.Unmarshal(data, &fc); err != nil {
		return FileConfig{}, err
	}
	return fc, nil
}

// Save persists fc to ConfigPath(), creating the containing directory if needed.
func (fc FileConfig) Save() error {
	if err := os.MkdirAll(filepath.Dir(ConfigPath()), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(fc)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0o644)
}
