package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/config"
	"github.com/JMThomas00/tukan/internal/database"
)

type appScreen int

const (
	screenSplash appScreen = iota
	screenBoard
)

// App is the root Bubble Tea model. It routes between splash and board.
type App struct {
	screen appScreen
	splash SplashModel
	board  BoardModel
	width  int
	height int
}

// New creates the root application model.
func New(db *database.DB, cfg config.Config) (App, error) {
	splash := NewSplash(cfg)
	board, _ := NewBoard(db, 0, 0) // dimensions filled in on first WindowSizeMsg
	return App{
		screen: screenSplash,
		splash: splash,
		board:  board,
	}, nil
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.splash.Init(),
		a.board.Init(),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		var splashCmd, boardCmd tea.Cmd
		a.splash, splashCmd = a.splash.Update(msg)
		a.board.width = msg.Width
		a.board.height = msg.Height
		a.board.form.width = msg.Width
		a.board.form.height = msg.Height
		return a, tea.Batch(splashCmd, boardCmd)

	case splashDoneMsg:
		a.screen = screenBoard
		return a, nil

	case splashTickMsg:
		// Splash-only message — only ever goes to the splash model.
		var cmd tea.Cmd
		a.splash, cmd = a.splash.Update(msg)
		return a, cmd

	case tea.KeyMsg:
		if a.screen == screenSplash {
			// Any key skips the splash.
			a.screen = screenBoard
			return a, nil
		}
		var cmd tea.Cmd
		a.board, cmd = a.board.Update(msg)
		return a, cmd
	}

	// All other messages (boardLoadedMsg, cardCreatedMsg, dbErrMsg, etc.)
	// always go to the board — even while the splash is still showing.
	// This ensures DB results are never lost during the splash phase.
	var cmd tea.Cmd
	a.board, cmd = a.board.Update(msg)
	return a, cmd
}

func (a App) View() string {
	switch a.screen {
	case screenSplash:
		return a.splash.View()
	case screenBoard:
		return a.board.View()
	}
	return ""
}
