package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/styles"
)

// TestThemePreviewOnlyRecolorsItsOwnBoard is issue #8's regression test:
// browsing themes on one board must not repaint any other board in the
// same process (in Concord mode, every other viewer on the server).
func TestThemePreviewOnlyRecolorsItsOwnBoard(t *testing.T) {
	_, a := newTestBoard(t)
	_, other := newTestBoard(t)
	before := *other.st

	a, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	if !a.themeSwitcherActive {
		t.Fatal("T didn't open the theme switcher")
	}
	a, _ = a.Update(tea.KeyMsg{Type: tea.KeyDown})
	previewed := a.themeSwitcher.selectedName()

	if a.st.ColorAccent == before.ColorAccent {
		t.Fatalf("theme %q has the default accent, so this test proves nothing; preview a different one", previewed)
	}
	if want := styles.ForName(previewed).ColorAccent; a.st.ColorAccent != want {
		t.Fatalf("previewing board's accent = %q, want %q (theme %q)", a.st.ColorAccent, want, previewed)
	}
	if other.st.ColorAccent != before.ColorAccent || other.st.ColorBg != before.ColorBg {
		t.Fatalf("another board's colors changed while %q was previewed", previewed)
	}
}

// A board whose theme comes from the host (Concord) ignores T and doesn't
// advertise it.
func TestThemeLockedBoardHasNoThemeSwitcher(t *testing.T) {
	db, seed := newTestBoard(t)
	b, err := NewBoardForID(db, seed.currentBoardID, 80, 24, styles.Default())
	if err != nil {
		t.Fatal(err)
	}
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	if b.themeSwitcherActive {
		t.Fatal("T opened the theme switcher on a host-themed board")
	}
	if help := RenderHelp(testStyles, modeNormal, viewKanban, 200, false); strings.Contains(help, "theme") {
		t.Fatalf("help advertises the theme switcher: %q", help)
	}
	if help := RenderHelp(testStyles, modeNormal, viewKanban, 200, true); !strings.Contains(help, "theme") {
		t.Fatalf("standalone help lost the theme switcher: %q", help)
	}
}

// SetStyles recolors overlays that are already open, not just the board.
func TestSetStylesReachesOpenOverlays(t *testing.T) {
	db, seed := newTestBoard(t)
	b, err := NewBoardForID(db, seed.currentBoardID, 80, 24, styles.Default())
	if err != nil {
		t.Fatal(err)
	}
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if !b.laneManagerActive {
		t.Fatal("S didn't open the lane manager")
	}

	dracula := styles.ForName("dracula")
	b.SetStyles(dracula)
	if b.st.ColorAccent != dracula.ColorAccent || b.laneManager.st.ColorAccent != dracula.ColorAccent {
		t.Fatalf("accent: board %q, lane manager %q, want %q", b.st.ColorAccent, b.laneManager.st.ColorAccent, dracula.ColorAccent)
	}
}
