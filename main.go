// Tukan: a kanban board for the terminal, and for Concord channels.
//
// Run from a terminal, it's a full-screen board app. Launched by a Concord
// server (which sets CONCORD_* variables), the same program hosts a board
// in each of its channels, shared live by everyone viewing it.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/Concord/sdk/plugin"

	"github.com/JMThomas00/tukan/internal/concord"
	"github.com/JMThomas00/tukan/internal/config"
	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/styles"
	"github.com/JMThomas00/tukan/internal/themes"
	"github.com/JMThomas00/tukan/internal/ui"
)

func main() {
	fileCfg, err := config.LoadFileConfig()
	if err != nil {
		log.Fatalf("tukan: load config: %v", err)
	}
	theme, err := themes.GetTheme(fileCfg.Theme)
	if err != nil {
		theme = themes.GetDefaultTheme()
	}
	styles.Apply(theme)

	cfg := config.Default()
	pcfg, underConcord := plugin.ConfigFromEnv()
	if underConcord {
		cfg.DBPath = pluginDBPath(pcfg.DataDir, cfg.DBPath)
	}
	if err := os.MkdirAll(cfg.DBDir(), 0o755); err != nil {
		log.Fatalf("tukan: create data directory: %v", err)
	}
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("tukan: open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		log.Fatalf("tukan: migrate: %v", err)
	}

	if underConcord {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		log.Printf("tukan: boards in %s", cfg.DBPath)
		if err := plugin.Run(ctx, pcfg, concord.New(db, fileCfg.Theme).Handler()); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
		return
	}
	standalone(db, cfg, fileCfg.Theme)
}

// pluginDBPath is where the plugin keeps its boards: the plugin's data
// folder, which survives updates. An install from before that existed kept
// them in the same database as standalone Tukan; if that's where they are,
// keep using it rather than start empty.
func pluginDBPath(dataDir, legacy string) string {
	if dataDir == "" {
		return legacy
	}
	path := filepath.Join(dataDir, "tukan.db")
	if _, err := os.Stat(path); err != nil {
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	return path
}

func standalone(db *database.DB, cfg config.Config, themeName string) {
	boards, err := db.ListBoards()
	if err != nil {
		log.Fatalf("tukan: list boards: %v", err)
	}
	var boardID int64
	if len(boards) == 0 {
		board, err := db.CreateBoard("Main Board", 0)
		if err != nil {
			log.Fatalf("tukan: create default board: %v", err)
		}
		boardID = board.ID
	} else {
		boardID = boards[0].ID
	}
	if err := db.SeedDefaultLanes(boardID); err != nil {
		log.Fatalf("tukan: seed: %v", err)
	}
	app, err := ui.New(db, cfg, themeName)
	if err != nil {
		log.Fatalf("tukan: init ui: %v", err)
	}
	if _, err := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		log.Fatalf("tukan: %v", err)
	}
}
