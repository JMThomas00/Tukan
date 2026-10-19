package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/config"
	"github.com/JMThomas00/tukan/internal/styles"
	"github.com/JMThomas00/tukan/internal/themes"
)

// themeSwitcherDoneMsg is returned when the switcher closes: cancelled
// (the board's styles are reverted to whatever theme was active before opening),
// or committed to a new theme (already persisted to disk by the time this
// fires).
type themeSwitcherDoneMsg struct {
	themeName string
	cancelled bool
}

// ThemeSwitcherModel lets the user browse and live-preview every available
// theme. Every ↑/↓ keypress rebuilds the board's *styles.Styles in place —
// every overlay of the board shares that pointer and reads it fresh on each
// View(), so this is the entire live-preview mechanism. It only ever touches
// this board's styles, never another viewer's.
type ThemeSwitcherModel struct {
	st           *styles.Styles // the owning board's, shared
	names        []string
	cursor       int
	originalName string // theme active when the switcher opened — for esc-revert
	width        int
	height       int
}

// NewThemeSwitcher creates a switcher pre-focused on the currently active theme.
func NewThemeSwitcher(st *styles.Styles, currentName string, w, h int) ThemeSwitcherModel {
	names := themes.ListAvailableThemes()
	cursor := 0
	for i, n := range names {
		if n == currentName {
			cursor = i
			break
		}
	}
	return ThemeSwitcherModel{
		st:           st,
		names:        names,
		cursor:       cursor,
		originalName: currentName,
		width:        w,
		height:       h,
	}
}

func (s ThemeSwitcherModel) Init() tea.Cmd {
	return nil
}

func (s ThemeSwitcherModel) Update(msg tea.Msg) (ThemeSwitcherModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	km := DefaultKeyMap

	switch {
	case key.Matches(keyMsg, km.Cancel):
		s.applyPreview(s.originalName)
		return s, func() tea.Msg {
			return themeSwitcherDoneMsg{cancelled: true}
		}

	case key.Matches(keyMsg, km.Submit), keyMsg.String() == "enter":
		name := s.selectedName()
		fc := config.FileConfig{Theme: name}
		_ = fc.Save() // best-effort; a save failure just means the choice won't persist across restarts
		return s, func() tea.Msg {
			return themeSwitcherDoneMsg{themeName: name}
		}

	case key.Matches(keyMsg, km.MoveUp):
		if len(s.names) > 0 {
			s.cursor = clamp(s.cursor-1, 0, len(s.names)-1)
			s.applyPreview(s.selectedName())
		}

	case key.Matches(keyMsg, km.MoveDown):
		if len(s.names) > 0 {
			s.cursor = clamp(s.cursor+1, 0, len(s.names)-1)
			s.applyPreview(s.selectedName())
		}
	}
	return s, nil
}

func (s ThemeSwitcherModel) selectedName() string {
	if s.cursor < len(s.names) {
		return s.names[s.cursor]
	}
	return ""
}

func (s ThemeSwitcherModel) applyPreview(name string) {
	*s.st = *styles.ForName(name)
}

func (s ThemeSwitcherModel) View() string {
	var b strings.Builder
	b.WriteString(s.st.FormLabelStyle.Render("Theme") + "\n\n")

	if len(s.names) == 0 {
		b.WriteString(s.st.CardNoteStyle.Render("No themes available") + "\n")
	}
	for i, name := range s.names {
		display := themes.GetThemeDisplayName(name)
		line := "  " + display
		if i == s.cursor {
			line = s.st.HelpKeyStyle.Render("› " + display)
		} else {
			line = s.st.FormLabelStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + s.st.HelpDescStyle.Render("↑/↓ preview  enter save  esc cancel"))

	box := s.st.ModalBoxStyle.Width(50).Render(b.String())
	if s.width > 0 && s.height > 0 {
		return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
