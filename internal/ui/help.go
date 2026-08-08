package ui

import (
	"strings"

	"github.com/JMThomas00/tukan/internal/styles"
)

type boardMode int

const (
	modeNormal        boardMode = iota
	modeMoving
	modeConfirmDelete
	modeFilterEdit
)

// RenderHelp returns a context-sensitive one-line help string.
func RenderHelp(mode boardMode, width int) string {
	var parts []string

	key := func(k, desc string) string {
		return styles.HelpKeyStyle.Render(k) + " " + styles.HelpDescStyle.Render(desc)
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
			key("←/→", "lane"),
			key("↑/↓", "card"),
			key("q", "quit"),
		}
	}

	line := strings.Join(parts, styles.HelpDescStyle.Render("  "))
	return styles.HelpBarStyle.Width(width).Render(line)
}
