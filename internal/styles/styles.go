package styles

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/themes"
)

// Color palette — package vars (not consts) so Apply can rebuild them from
// a loaded theme, including live at runtime while the theme switcher's
// preview is open. Every value here is set by Apply(); the literals below
// only exist as a documented starting point until the first Apply() call.
var (
	ColorBg        = "#1a1b26"
	ColorSurface   = "#24283b"
	ColorBorder    = "#414868"
	ColorAccent    = "#7aa2f7"
	ColorText      = "#c0caf5"
	ColorSubtext   = "#565f89"
	ColorDanger    = "#f7768e"
	ColorSuccess   = "#9ece6a"
	ColorWarning   = "#e0af68"
	ColorInfo      = "#7dcfff"
	ColorMuted     = "#3b4261"
	ColorHighlight = "#2d3f76"
	ColorAssignee  = "#bb9af7"
)

// LaneColors maps default lane positions to their header accent colors.
var LaneColors = [4]string{
	"#7aa2f7", // To-Do    — blue
	"#e0af68", // In Progress — yellow
	"#f7768e", // On Hold  — red
	"#9ece6a", // Done     — green
}

// LabelPalette is the fixed swatch list offered when creating a new label,
// so users pick a color rather than typing a hex value.
var LabelPalette = [8]string{
	"#f7768e", // red
	"#e0af68", // yellow
	"#9ece6a", // green
	"#7dcfff", // cyan
	"#7aa2f7", // blue
	"#bb9af7", // purple
	"#c0caf5", // text
	"#565f89", // subtext
}

// Pre-computed Lip Gloss styles. Rebuilt by Apply(), never allocated per-frame.
var (
	// Lane column styles
	LaneStyle        lipgloss.Style
	LaneFocusedStyle lipgloss.Style
	LaneHeaderStyle  lipgloss.Style

	// Card styles
	CardStyle            lipgloss.Style
	CardFocusedStyle     lipgloss.Style
	CardMovingStyle      lipgloss.Style
	CardTitleStyle       lipgloss.Style
	CardTicketStyle      lipgloss.Style
	CardNoteStyle        lipgloss.Style
	CardNoteFocusedStyle lipgloss.Style
	CardDueDateStyle     lipgloss.Style
	CardOverdueStyle     lipgloss.Style

	// Form / modal styles
	ModalBoxStyle      lipgloss.Style
	FormLabelStyle     lipgloss.Style
	SectionActiveStyle lipgloss.Style
	FormErrorStyle     lipgloss.Style

	// Help bar
	HelpBarStyle  lipgloss.Style
	HelpKeyStyle  lipgloss.Style
	HelpDescStyle lipgloss.Style

	// Splash
	SplashStyle lipgloss.Style

	// Status bar
	StatusBarStyle lipgloss.Style
	StatusErrStyle lipgloss.Style
)

// Apply rebuilds every package-level color and Style var from theme. Called
// once at startup (after the config/theme is loaded) and again on every
// keystroke while the theme switcher's live preview is open — cheap and
// safe to call repeatedly, since every styles.XyzStyle reference elsewhere
// in this codebase reads these package vars fresh at render time (Bubble
// Tea redraws every frame), so a reassignment here is visible on the very
// next View() call with no other plumbing needed.
//
// The mapping from a theme's 12 raw palette colors to Tukan's own UI roles
// is a judgment call (none of the 12 fields is explicitly "a highlighted
// surface tone") — spot-checked against several of the 45 ported themes,
// including a light-variant one, rather than derived mechanically.
func Apply(theme *themes.Theme) {
	c := theme.Colors

	ColorBg = c.Background
	ColorSurface = c.Background
	ColorBorder = c.CurrentLine
	ColorAccent = c.Purple
	ColorText = c.Foreground
	ColorSubtext = c.Comment
	ColorDanger = c.Red
	ColorSuccess = c.Green
	ColorWarning = c.Orange
	ColorInfo = c.Cyan
	ColorMuted = c.Selection
	ColorHighlight = c.Selection
	ColorAssignee = c.Pink

	LaneColors = [4]string{c.Cyan, c.Orange, c.Red, c.Green}
	LabelPalette = [8]string{c.Red, c.Orange, c.Green, c.Cyan, c.Purple, c.Pink, c.Foreground, c.Comment}

	// Lane column
	LaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorBorder)).
		Padding(0, 1)

	LaneFocusedStyle = LaneStyle.
		BorderForeground(lipgloss.Color(ColorAccent))

	LaneHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorAccent)).
		MarginBottom(1)

	// Card
	CardStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorBorder)).
		Background(lipgloss.Color(ColorSurface)).
		Padding(0, 1).
		MarginBottom(1)

	CardFocusedStyle = CardStyle.
		BorderForeground(lipgloss.Color(ColorAccent)).
		Background(lipgloss.Color(ColorHighlight))

	CardMovingStyle = CardStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorWarning))

	CardTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorText))

	// CardTicketStyle is deliberately muted (unlike the bold title it sits
	// beside) — the ticket number is a reference/identity marker, not the
	// primary thing drawing the eye on a card.
	CardTicketStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSubtext))

	CardNoteStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color(ColorMuted))

	// CardNoteFocusedStyle is used instead of CardNoteStyle whenever the
	// card itself is focused: ColorMuted and ColorHighlight are both
	// mapped from the same theme field (there's no dedicated "muted text"
	// tone separate from "selection background" in a theme's 12 raw
	// colors), so CardNoteStyle's own foreground is illegible against a
	// focused card's background — same color on both sides. Concord's own
	// selected-item styling (SidebarSelected) resolves the identical
	// problem by keeping full-contrast foreground text on a selected
	// background rather than trying to find a second muted tone, so this
	// does the same: full ColorText contrast, keeping only the italic to
	// preserve "this is a note" as a visual cue.
	CardNoteFocusedStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color(ColorText))

	CardDueDateStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSubtext))

	CardOverdueStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorDanger))

	// Modal
	ModalBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorAccent)).
		Background(lipgloss.Color(ColorSurface)).
		Padding(1, 2)

	FormLabelStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorAccent))

	// SectionActiveStyle marks whichever field/section currently has focus
	// in the card editor. It used to just be HelpKeyStyle — Bold +
	// Foreground(ColorAccent) — which is byte-for-byte identical to
	// FormLabelStyle's own definition above, so the "focused" header never
	// actually looked any different from an unfocused one. An
	// inverted/pill treatment (background instead of just a text color)
	// can't collide with the unfocused style by coincidence the way two
	// same-shaped foreground-only styles did.
	SectionActiveStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorBg)).
		Background(lipgloss.Color(ColorAccent)).
		Padding(0, 1)

	FormErrorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDanger))

	// Help bar — no background: a filled strip here only reached the
	// terminal's own default background for however many rows were left
	// over below it (see boardChrome's comment for why that gap existed),
	// which read as the highlight visibly "cutting off" rather than
	// reaching the bottom of the window.
	HelpBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSubtext)).
		Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccent)).
		Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSubtext))

	// Splash screen
	SplashStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccent))

	// Status bar — no background, same reasoning as HelpBarStyle above.
	StatusBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSubtext)).
		Padding(0, 1)

	StatusErrStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDanger)).
		Padding(0, 1)
}

// AssigneeColor deterministically picks a color for a registered assignee
// from the current theme's label palette, keyed by their registry ID —
// round-robin by insertion order (SQLite's autoincrement id), not stored
// anywhere. That's what makes the same person always render in the same
// color across every card and board (the id doesn't change) while staying
// automatically theme-consistent (LabelPalette is rebuilt by Apply(), so a
// theme switch recolors assignees too, the same as it does labels) —
// storing an explicit hex value per assignee would go stale the moment the
// theme changed.
func AssigneeColor(assigneeID int64) string {
	if assigneeID <= 0 {
		return ColorAssignee
	}
	idx := int((assigneeID - 1) % int64(len(LabelPalette)))
	return LabelPalette[idx]
}

func init() {
	Apply(themes.GetDefaultTheme())
}
