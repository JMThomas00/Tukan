package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempUserConfigDir points os.UserConfigDir()'s underlying env var at a
// temp dir for the duration of the test, so LoadFileConfig/Save don't touch
// the developer's real %APPDATA%\tukan.
func withTempUserConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir) // os.UserConfigDir() on Windows reads %APPDATA%
	return dir
}

func TestLoadFileConfigCreatesOnMissing(t *testing.T) {
	withTempUserConfigDir(t)

	if _, err := os.Stat(ConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("expected no config file yet, stat err = %v", err)
	}

	fc, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if fc.Theme != "" {
		t.Fatalf("fresh config Theme = %q, want empty (built-in default)", fc.Theme)
	}
	if _, err := os.Stat(ConfigPath()); err != nil {
		t.Fatalf("expected config file to be created, stat err = %v", err)
	}
}

func TestFileConfigSaveAndReloadRoundTrip(t *testing.T) {
	withTempUserConfigDir(t)

	fc := FileConfig{Theme: "dracula"}
	if err := fc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if reloaded.Theme != "dracula" {
		t.Fatalf("reloaded.Theme = %q, want %q", reloaded.Theme, "dracula")
	}
}

func TestConfigPathAndThemesDirShareBaseDir(t *testing.T) {
	dir := withTempUserConfigDir(t)

	wantConfig := filepath.Join(dir, "tukan", "config.toml")
	if ConfigPath() != wantConfig {
		t.Fatalf("ConfigPath() = %q, want %q", ConfigPath(), wantConfig)
	}
	wantThemes := filepath.Join(dir, "tukan", "themes")
	if ThemesDir() != wantThemes {
		t.Fatalf("ThemesDir() = %q, want %q", ThemesDir(), wantThemes)
	}
}
