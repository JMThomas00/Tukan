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
	default: // modeNormal
		parts = []string{
			key("n", "new"),
			key("e", "edit"),
			key("d", "del"),
			key("m", "move"),
			key("←/→", "lane"),
			key("↑/↓", "card"),
			key("q", "quit"),
		}
	}

	line := strings.Join(parts, styles.HelpDescStyle.Render("  "))
	return styles.HelpBarStyle.Width(width).Render(line)
}
