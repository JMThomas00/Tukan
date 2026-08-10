package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/config"
	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/styles"
	"github.com/JMThomas00/tukan/internal/themes"
	"github.com/JMThomas00/tukan/internal/ui"
)

func main() {
	cfg := config.Default()

	fileCfg, err := config.LoadFileConfig()
	if err != nil {
		log.Fatalf("tukan: load config: %v", err)
	}
	theme, err := themes.GetTheme(fileCfg.Theme)
	if err != nil {
		theme = themes.GetDefaultTheme()
	}
	styles.Apply(theme)

	if err := os.MkdirAll(cfg.DBDir(), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "tukan: could not create data directory: %v\n", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("tukan: open database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		log.Fatalf("tukan: migrate: %v", err)
	}

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

	app, err := ui.New(db, cfg, fileCfg.Theme)
	if err != nil {
		log.Fatalf("tukan: init ui: %v", err)
	}

	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		log.Fatalf("tukan: %v", err)
	}
}
