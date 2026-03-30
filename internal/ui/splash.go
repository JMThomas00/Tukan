package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/config"
	"github.com/JMThomas00/tukan/internal/styles"
)

// splashTickMsg is sent every 250ms during the splash screen.
type splashTickMsg time.Time

// splashDoneMsg signals that the splash duration has elapsed.
type splashDoneMsg struct{}

// SplashModel is the Bubble Tea model for the startup splash screen.
type SplashModel struct {
	logo     string
	elapsed  time.Duration
	duration time.Duration
	width    int
	height   int
}

// NewSplash creates a SplashModel using the embedded logo and config duration.
func NewSplash(cfg config.Config) SplashModel {
	return SplashModel{
		logo:     config.LogoContent,
		duration: cfg.SplashDuration,
	}
}

func (s SplashModel) Init() tea.Cmd {
	return tickSplash()
}

func (s SplashModel) Update(msg tea.Msg) (SplashModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
	case splashTickMsg:
		s.elapsed += 250 * time.Millisecond
		if s.elapsed >= s.duration {
			return s, func() tea.Msg { return splashDoneMsg{} }
		}
		return s, tickSplash()
	}
	return s, nil
}

func (s SplashModel) View() string {
	logo := styles.SplashStyle.Render(s.logo)
	if s.width == 0 || s.height == 0 {
		return logo
	}
	return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, logo)
}

func tickSplash() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return splashTickMsg(t)
	})
}
