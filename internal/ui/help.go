package ui

import (
	"slices"
	"strings"

	"github.com/JMThomas00/tukan/internal/styles"
)

type boardMode int

const (
	modeNormal boardMode = iota
	modeMoving
	modeConfirmDelete
	modeFilterEdit
)

// RenderHelp returns a context-sensitive one-line help string. viewMode
// affects only the modeNormal hint list — left/right and move-card mean
// different things in Gantt view (panning the timeline, not lane
// navigation), and "m" (move between lanes) has no meaning there at all.
// showTheme is false when the host picks the theme (Concord), so "T" does
// nothing and isn't advertised.
func RenderHelp(st *styles.Styles, mode boardMode, viewMode boardViewMode, width int, showTheme bool) string {
	var parts []string

	key := func(k, desc string) string {
		return st.HelpKeyStyle.Render(k) + " " + st.HelpDescStyle.Render(desc)
	}

	switch mode {
	case modeMoving:
		parts = []string{
			key("←/→", "pick lane"),
			key("enter", "drop"),
			key("esc", "cancel"),
		}
	case modeConfirmDelete:
		parts = []string{
			key("y", "confirm delete"),
			key("esc", "cancel"),
		}
	case modeFilterEdit:
		parts = []string{
			key("enter", "apply"),
			key("esc", "clear"),
		}
	default: // modeNormal
		if viewMode == viewGantt {
			parts = []string{
				key("n", "new"),
				key("e", "edit"),
				key("d", "del"),
				key("b", "boards"),
				key("S", "lanes"),
				key("L", "labels"),
				key("c", "checklist"),
				key("v", "history"),
				key("/", "search"),
				key("T", "theme"),
				key("g", "kanban"),
				key("←/→", "pan"),
				key("↑/↓", "card"),
				key("q", "quit"),
			}
		} else {
			parts = []string{
				key("n", "new"),
				key("e", "edit"),
				key("d", "del"),
				key("m", "move"),
				key("b", "boards"),
				key("S", "lanes"),
				key("L", "labels"),
				key("c", "checklist"),
				key("v", "history"),
				key("/", "search"),
				key("T", "theme"),
				key("g", "gantt"),
				key("←/→", "lane"),
				key("↑/↓", "card"),
				key("q", "quit"),
			}
		}
	}

	if !showTheme {
		parts = slices.DeleteFunc(parts, func(p string) bool { return p == key("T", "theme") })
	}

	line := strings.Join(parts, st.HelpDescStyle.Render("  "))
	return st.HelpBarStyle.Width(width).Render(line)
}
