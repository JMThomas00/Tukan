package styles

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/themes"
)

// Styles is one theme's full set of colors and pre-computed Lip Gloss
// styles. Each board owns its own *Styles rather than sharing package
// state: in Concord mode one process serves every viewer on a server, each
// in their own Concord theme, so a theme change (or the standalone theme
// switcher's live preview) must only ever repaint the board it belongs to.
// Every sub-model of a board holds the same pointer, so replacing the value
// it points at (*st = *New(theme)) recolors the whole board on its next
// View() with no other plumbing.
type Styles struct {
	// Color palette — UI roles mapped from the theme's 12 raw colors.
	ColorBg        string
	ColorSurface   string
	ColorBorder    string
	ColorAccent    string
	ColorText      string
	ColorSubtext   string
	ColorDanger    string
	ColorSuccess   string
	ColorWarning   string
	ColorInfo      string
	ColorMuted     string
	ColorHighlight string
	ColorAssignee  string

	// LaneColors maps default lane positions to their header accent colors
	// (To-Do, In Progress, On Hold, Done).
	LaneColors [4]string

	// LabelPalette is the fixed swatch list offered when creating a new
	// label, so users pick a color rather than typing a hex value.
	LabelPalette [8]string

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
}

// ForName builds the styles for a theme slug, falling back to Tukan's
// default palette when the name is empty or can't be loaded.
func ForName(name string) *Styles {
	theme, err := themes.GetTheme(name)
	if err != nil {
		theme = themes.GetDefaultTheme()
	}
	return New(theme)
}

// Default is Tukan's original palette.
func Default() *Styles { return New(themes.GetDefaultTheme()) }

// New builds every color and Style from theme. Cheap enough to call on
// every keystroke while the theme switcher's preview is open.
//
// The mapping from a theme's 12 raw palette colors to Tukan's own UI roles
// is a judgment call (none of the 12 fields is explicitly "a highlighted
// surface tone") — spot-checked against several of the 45 ported themes,
// including a light-variant one, rather than derived mechanically.
func New(theme *themes.Theme) *Styles {
	c := theme.Colors
	s := &Styles{}

	s.ColorBg = c.Background
	s.ColorSurface = c.Background
	// Borders and muted text are foreground on the terminal's background,
	// so they use comment (what Concord draws its own borders in) — not
	// current_line/selection, which are background tones and vanish on a
	// dark terminal (e.g. Orange's #2D1A10).
	s.ColorBorder = c.Comment
	s.ColorAccent = c.Purple
	s.ColorText = c.Foreground
	s.ColorSubtext = c.Comment
	s.ColorDanger = c.Red
	s.ColorSuccess = c.Green
	s.ColorWarning = c.Orange
	s.ColorInfo = c.Cyan
	s.ColorMuted = c.Comment
	s.ColorHighlight = c.Selection
	s.ColorAssignee = c.Pink

	s.LaneColors = [4]string{c.Cyan, c.Orange, c.Red, c.Green}
	s.LabelPalette = [8]string{c.Red, c.Orange, c.Green, c.Cyan, c.Purple, c.Pink, c.Foreground, c.Comment}

	// Lane column
	s.LaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(s.ColorBorder)).
		Padding(0, 1)

	s.LaneFocusedStyle = s.LaneStyle.
		BorderForeground(lipgloss.Color(s.ColorAccent))

	s.LaneHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(s.ColorAccent)).
		MarginBottom(1)

	// Card
	s.CardStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(s.ColorBorder)).
		Background(lipgloss.Color(s.ColorSurface)).
		Padding(0, 1).
		MarginBottom(1)

	s.CardFocusedStyle = s.CardStyle.
		BorderForeground(lipgloss.Color(s.ColorAccent)).
		Background(lipgloss.Color(s.ColorHighlight))

	s.CardMovingStyle = s.CardStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(s.ColorWarning))

	s.CardTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(s.ColorText))

	// CardTicketStyle is deliberately muted (unlike the bold title it sits
	// beside) — the ticket number is a reference/identity marker, not the
	// primary thing drawing the eye on a card.
	s.CardTicketStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorSubtext))

	s.CardNoteStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color(s.ColorMuted))

	// CardNoteFocusedStyle is used instead of CardNoteStyle whenever the
	// card itself is focused: a muted note foreground is low-contrast
	// against a focused card's highlight background. Concord's own
	// selected-item styling (SidebarSelected) resolves the identical
	// problem by keeping full-contrast foreground text on a selected
	// background rather than trying to find a second muted tone, so this
	// does the same: full ColorText contrast, keeping only the italic to
	// preserve "this is a note" as a visual cue.
	s.CardNoteFocusedStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color(s.ColorText))

	s.CardDueDateStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorSubtext))

	s.CardOverdueStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(s.ColorDanger))

	// Modal
	s.ModalBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(s.ColorAccent)).
		Background(lipgloss.Color(s.ColorSurface)).
		Padding(1, 2)

	s.FormLabelStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(s.ColorAccent))

	// SectionActiveStyle marks whichever field/section currently has focus
	// in the card editor. It used to just be HelpKeyStyle — Bold +
	// Foreground(ColorAccent) — which is byte-for-byte identical to
	// FormLabelStyle's own definition above, so the "focused" header never
	// actually looked any different from an unfocused one. An
	// inverted/pill treatment (background instead of just a text color)
	// can't collide with the unfocused style by coincidence the way two
	// same-shaped foreground-only styles did.
	s.SectionActiveStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(s.ColorBg)).
		Background(lipgloss.Color(s.ColorAccent)).
		Padding(0, 1)

	s.FormErrorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorDanger))

	// Help bar — no background: a filled strip here only reached the
	// terminal's own default background for however many rows were left
	// over below it (see boardChrome's comment for why that gap existed),
	// which read as the highlight visibly "cutting off" rather than
	// reaching the bottom of the window.
	s.HelpBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorSubtext)).
		Padding(0, 1)

	s.HelpKeyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorAccent)).
		Bold(true)

	s.HelpDescStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorSubtext))

	// Splash screen
	s.SplashStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorAccent))

	// Status bar — no background, same reasoning as HelpBarStyle above.
	s.StatusBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorSubtext)).
		Padding(0, 1)

	s.StatusErrStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.ColorDanger)).
		Padding(0, 1)
	return s
}

// AssigneeColor deterministically picks a color for a registered assignee
// from the current theme's label palette, keyed by their registry ID —
// round-robin by insertion order (SQLite's autoincrement id), not stored
// anywhere. That's what makes the same person always render in the same
// color across every card and board (the id doesn't change) while staying
// automatically theme-consistent (LabelPalette comes from the theme, so a
// theme switch recolors assignees too, the same as it does labels) —
// storing an explicit hex value per assignee would go stale the moment the
// theme changed.
func (s *Styles) AssigneeColor(assigneeID int64) string {
	if assigneeID <= 0 {
		return s.ColorAssignee
	}
	idx := int((assigneeID - 1) % int64(len(s.LabelPalette)))
	return s.LabelPalette[idx]
}
