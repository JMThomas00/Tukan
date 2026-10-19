package concord

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/JMThomas00/Concord/sdk/pane"
	"github.com/JMThomas00/Concord/sdk/wire"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/styles"
)

// A viewer's Concord palette drives their board's colors; anything it
// leaves out, or a missing theme, falls back to the server's theme.
func TestStylesForUsesTheViewersPalette(t *testing.T) {
	s := New(nil, "dracula")
	dracula := styles.ForName("dracula")

	got := s.stylesFor(&wire.PaneTheme{Palette: map[string]string{"purple": "#123456", "background": "#000000"}})
	if got.ColorAccent != "#123456" || got.ColorBg != "#000000" {
		t.Fatalf("accent %q, background %q: the viewer's palette wasn't used", got.ColorAccent, got.ColorBg)
	}
	if got.ColorText != dracula.ColorText {
		t.Fatalf("text %q, want the fallback theme's %q", got.ColorText, dracula.ColorText)
	}
	if got := s.stylesFor(nil); got.ColorAccent != dracula.ColorAccent {
		t.Fatalf("no theme: accent %q, want %q", got.ColorAccent, dracula.ColorAccent)
	}
	if s.theme.Colors.Purple != dracula.ColorAccent {
		t.Fatal("stylesFor changed the server's fallback theme")
	}
}

// When a viewer switches Concord theme (the host updates v.Theme, then
// resizes), only that viewer's board is recolored.
func TestThemeChangeRecolorsOnlyThatViewer(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "tukan.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	s := New(db, "")
	ch := uuid.New()
	if _, err := s.boardFor(ch, "sprint", ""); err != nil {
		t.Fatal(err)
	}

	open := func() (*pane.Viewer, *boardModel, *[]*wire.PaneTheme) {
		v := &pane.Viewer{ID: uuid.New(), ChannelID: ch, Width: 100, Height: 30}
		m := s.newModel(v).(*boardModel)
		var built []*wire.PaneTheme
		m.stylesFor = func(pt *wire.PaneTheme) *styles.Styles {
			built = append(built, pt)
			return s.stylesFor(pt)
		}
		return v, m, &built
	}
	alice, aliceBoard, aliceBuilt := open()
	_, bobBoard, bobBuilt := open()

	aliceBoard.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if len(*aliceBuilt) != 0 {
		t.Fatal("a plain resize rebuilt the styles")
	}

	alice.Theme = &wire.PaneTheme{Name: "new", Palette: map[string]string{"purple": "#123456"}}
	aliceBoard.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	bobBoard.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if len(*aliceBuilt) != 1 || (*aliceBuilt)[0] != alice.Theme {
		t.Fatalf("alice's board built styles for %v, want her new theme once", *aliceBuilt)
	}
	if len(*bobBuilt) != 0 {
		t.Fatal("bob's board was recolored by alice's theme change")
	}
}
