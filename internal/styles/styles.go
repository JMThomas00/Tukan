package styles

import "github.com/charmbracelet/lipgloss"

// Color palette (Tokyo Night-inspired dark theme)
const (
	ColorBg         = "#1a1b26"
	ColorSurface    = "#24283b"
	ColorBorder     = "#414868"
	ColorAccent     = "#7aa2f7"
	ColorText       = "#c0caf5"
	ColorSubtext    = "#565f89"
	ColorDanger     = "#f7768e"
	ColorSuccess    = "#9ece6a"
	ColorWarning    = "#e0af68"
	ColorInfo       = "#7dcfff"
	ColorMuted      = "#3b4261"
	ColorHighlight  = "#2d3f76"
)

// LaneColors maps default lane positions to their header accent colors.
var LaneColors = [4]string{
	"#7aa2f7", // To-Do    — blue
	"#e0af68", // In Progress — yellow
	"#f7768e", // On Hold  — red
	"#9ece6a", // Done     — green
}

// Pre-computed Lip Gloss styles (allocated once in init, never per-frame).
var (
	// Lane column styles
	LaneStyle        lipgloss.Style
	LaneFocusedStyle lipgloss.Style
	LaneHeaderStyle  lipgloss.Style

	// Card styles
	CardStyle        lipgloss.Style
	CardFocusedStyle lipgloss.Style
	CardMovingStyle  lipgloss.Style
	CardTitleStyle   lipgloss.Style
	CardAssigneeStyle lipgloss.Style
	CardNoteStyle    lipgloss.Style

	// Form / modal styles
	ModalBoxStyle     lipgloss.Style
	FormLabelStyle    lipgloss.Style
	FormErrorStyle    lipgloss.Style

	// Help bar
	HelpBarStyle      lipgloss.Style
	HelpKeyStyle      lipgloss.Style
	HelpDescStyle     lipgloss.Style

	// Splash
	SplashStyle       lipgloss.Style

	// Status bar
	StatusBarStyle    lipgloss.Style
	StatusErrStyle    lipgloss.Style
)

func init() {
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

	CardAssigneeStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSubtext))

	CardNoteStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color(ColorMuted))

	// Modal
	ModalBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorAccent)).
		Background(lipgloss.Color(ColorSurface)).
		Padding(1, 2)

	FormLabelStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorAccent))

	FormErrorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDanger))

	// Help bar
	HelpBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorMuted)).
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

	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorMuted)).
		Foreground(lipgloss.Color(ColorSubtext)).
		Padding(0, 1)

	StatusErrStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorMuted)).
		Foreground(lipgloss.Color(ColorDanger)).
		Padding(0, 1)
}
