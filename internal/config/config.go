package config

import (
	"path/filepath"
	"time"
)

// Config holds application-level configuration.
type Config struct {
	DBPath         string
	SplashDuration time.Duration
}

// Default returns the default application configuration.
// The database is stored in the user's config directory.
func Default() Config {
	return Config{
		DBPath:         defaultDBPath(),
		SplashDuration: 4 * time.Second,
	}
}

// DBDir returns the directory that contains the database file.
func (c Config) DBDir() string {
	return filepath.Dir(c.DBPath)
}

func defaultDBPath() string {
	return filepath.Join(tukanDir(), "tukan.db")
}
