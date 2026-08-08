package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

const testBg = "#2d3f76" // an arbitrary "card background" for these tests, e.g. ColorHighlight

func TestRenderCardBadgesEmpty(t *testing.T) {
	out := renderCardBadges(models.Card{}, nil, nil, 20, testBg)
	// Unlike before the background-bleed fix, an empty badge line is now
	// genuinely an empty string — renderCard's own "\n" + this concatenation
	// is what reserves the line's height, not padding done in here. See
	// renderCard's outer cardStyle.Width(width).Render(content) call.
	if out != "" {
		t.Fatalf("expected an empty string when there's nothing to show, got: %q", out)
	}
}

func TestRenderCardBadgesWithLabels(t *testing.T) {
	labels := []models.Label{
		{ID: 1, Name: "Bug", Color: "#f7768e"},
		{ID: 2, Name: "UI", Color: "#7aa2f7"},
	}
	out := renderCardBadges(models.Card{}, labels, nil, 40, testBg)
	if strings.Count(out, "●") != 2 {
		t.Fatalf("expected 2 chip markers for 2 labels within budget, got: %q", out)
	}
}

func TestRenderCardBadgesTruncatesExcessLabels(t *testing.T) {
	labels := []models.Label{
		{ID: 1, Name: "A", Color: "#f7768e"},
		{ID: 2, Name: "B", Color: "#7aa2f7"},
		{ID: 3, Name: "C", Color: "#9ece6a"},
		{ID: 4, Name: "D", Color: "#e0af68"},
	}
	// width/2 = 5 chips would fit numerically, so use a narrow width to force truncation.
	out := renderCardBadges(models.Card{}, labels, nil, 4, testBg)
	if !strings.Contains(out, "+") {
		t.Fatalf("expected a '+N' truncation marker when labels exceed the chip budget, got: %q", out)
	}
}

func TestRenderCardBadgesDueDate(t *testing.T) {
	future := time.Now().UTC().AddDate(0, 0, 7)
	out := renderCardBadges(models.Card{DueDate: &future}, nil, nil, 40, testBg)
	if !strings.Contains(out, "due "+future.Format(dueDateLayout)) {
		t.Fatalf("expected due-date text in badge line, got: %q", out)
	}
}

func TestRenderCardBadgesOverdue(t *testing.T) {
	past := time.Now().UTC().AddDate(0, 0, -7)
	out := renderCardBadges(models.Card{DueDate: &past}, nil, nil, 40, testBg)
	if !strings.Contains(out, "due "+past.Format(dueDateLayout)) {
		t.Fatalf("expected due-date text in badge line, got: %q", out)
	}
}

func TestRenderCardBadgesLabelsAndDueDateTogether(t *testing.T) {
	labels := []models.Label{{ID: 1, Name: "Bug", Color: "#f7768e"}}
	due := time.Now().UTC().AddDate(0, 0, 1)
	out := renderCardBadges(models.Card{DueDate: &due}, labels, nil, 40, testBg)
	if !strings.Contains(out, "●") {
		t.Fatalf("expected a label chip, got: %q", out)
	}
	if !strings.Contains(out, "due ") {
		t.Fatalf("expected due-date text alongside the label chip, got: %q", out)
	}
}

func TestRenderCardBadgesChecklistCounter(t *testing.T) {
	items := []models.ChecklistItem{
		{ID: 1, Text: "a", Done: true},
		{ID: 2, Text: "b", Done: true},
		{ID: 3, Text: "c", Done: false},
	}
	out := renderCardBadges(models.Card{}, nil, items, 40, testBg)
	if !strings.Contains(out, "[2/3]") {
		t.Fatalf("expected checklist counter [2/3], got: %q", out)
	}
}

func TestRenderCardBadgesEmptyChecklistShowsNoCounter(t *testing.T) {
	out := renderCardBadges(models.Card{}, nil, nil, 40, testBg)
	if strings.Contains(out, "[") {
		t.Fatalf("unexpected checklist counter with no items: %q", out)
	}
}

func TestRenderCardBadgesAllThreeTogether(t *testing.T) {
	labels := []models.Label{{ID: 1, Name: "Bug", Color: "#f7768e"}}
	due := time.Now().UTC().AddDate(0, 0, 1)
	items := []models.ChecklistItem{{ID: 1, Text: "a", Done: true}, {ID: 2, Text: "b"}}
	out := renderCardBadges(models.Card{DueDate: &due}, labels, items, 60, testBg)
	if !strings.Contains(out, "●") {
		t.Fatalf("expected a label chip, got: %q", out)
	}
	if !strings.Contains(out, "due ") {
		t.Fatalf("expected due-date text, got: %q", out)
	}
	if !strings.Contains(out, "[1/2]") {
		t.Fatalf("expected checklist counter [1/2], got: %q", out)
	}
}

// TestRenderCardBadgesLeafStylesUseCardBackground is the regression guard
// for the background-bleed bug: every leaf style renderCardBadges builds
// (label chips, due-date/overdue, checklist counter) must set the card's
// own current background explicitly, or its own embedded ANSI reset would
// revert that segment to the terminal's default background instead of the
// card's — most visible with 2+ label chips, since each chip's reset used
// to re-break the background again mid-line. This compares
// renderCardBadges' actual output against independently-constructed
// reference renders using the exact style+background the fix applies, so
// it holds regardless of whether the test environment's terminal profile
// happens to emit real ANSI codes or not (both sides use the same lipgloss
// call path).
func TestRenderCardBadgesLeafStylesUseCardBackground(t *testing.T) {
	bgColor := lipgloss.Color(testBg)

	t.Run("due date", func(t *testing.T) {
		due := time.Now().UTC().AddDate(0, 0, 1)
		out := renderCardBadges(models.Card{DueDate: &due}, nil, nil, 40, testBg)
		want := styles.CardDueDateStyle.Background(bgColor).Render("due " + due.Format(dueDateLayout))
		if out != want {
			t.Fatalf("due-date segment = %q, want %q", out, want)
		}
	})

	t.Run("overdue", func(t *testing.T) {
		past := time.Now().UTC().AddDate(0, 0, -1)
		out := renderCardBadges(models.Card{DueDate: &past}, nil, nil, 40, testBg)
		want := styles.CardOverdueStyle.Background(bgColor).Render("due " + past.Format(dueDateLayout))
		if out != want {
			t.Fatalf("overdue segment = %q, want %q", out, want)
		}
	})

	t.Run("checklist counter", func(t *testing.T) {
		items := []models.ChecklistItem{{ID: 1, Text: "a", Done: true}, {ID: 2, Text: "b"}}
		out := renderCardBadges(models.Card{}, nil, items, 40, testBg)
		want := styles.CardDueDateStyle.Background(bgColor).Render("[1/2]")
		if out != want {
			t.Fatalf("checklist segment = %q, want %q", out, want)
		}
	})

	t.Run("label chip", func(t *testing.T) {
		labels := []models.Label{{ID: 1, Name: "Bug", Color: "#f7768e"}}
		out := renderCardBadges(models.Card{}, labels, nil, 40, testBg)
		want := lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Background(bgColor).Render("●")
		if out != want {
			t.Fatalf("label chip segment = %q, want %q", out, want)
		}
	})

	// The gap BETWEEN two top-level segments (a label chip and a checklist
	// counter here) is exactly where the background used to bleed even
	// after every individual segment got its own Background(bg) — the "  "
	// joining them was a bare, unstyled string.Join separator, so once the
	// first segment's own trailing ANSI reset fired, that separator (and
	// anything after it, until the next segment re-established color)
	// rendered in the terminal's default background instead of the card's.
	t.Run("separator between two segments carries the card background too", func(t *testing.T) {
		labels := []models.Label{{ID: 1, Name: "Bug", Color: "#f7768e"}}
		items := []models.ChecklistItem{{ID: 1, Text: "a", Done: true}, {ID: 2, Text: "b"}}
		out := renderCardBadges(models.Card{}, labels, items, 40, testBg)

		chip := lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Background(bgColor).Render("●")
		counter := styles.CardDueDateStyle.Background(bgColor).Render("[1/2]")
		sep := lipgloss.NewStyle().Background(bgColor).Render("  ")
		want := chip + sep + counter
		if out != want {
			t.Fatalf("badges with a chip and a counter = %q, want %q (background-styled separator between them)", out, want)
		}
	})
}

// TestRenderCardAssigneesUsesBackgroundedSeparator is renderCardBadges'
// same separator regression guard, for the assignee line — each name is
// its own Render() call (so the same person can have a different color per
// theme swap), so the plain space joining two names is exactly as exposed
// to the background-bleed bug as renderCardBadges' segment separators were.
func TestRenderCardAssigneesUsesBackgroundedSeparator(t *testing.T) {
	bgColor := lipgloss.Color(testBg)
	assignees := []models.Assignee{{ID: 1, Name: "Jordan"}, {ID: 2, Name: "Claude"}}

	out := renderCardAssignees(assignees, false, false, testBg)

	name1 := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.AssigneeColor(1))).Background(bgColor).Render("@Jordan")
	name2 := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.AssigneeColor(2))).Background(bgColor).Render("@Claude")
	sep := lipgloss.NewStyle().Background(bgColor).Render(" ")
	want := name1 + sep + name2
	if out != want {
		t.Fatalf("renderCardAssignees = %q, want %q (background-styled separator between names)", out, want)
	}
}

// TestRenderCardTitleRowRightAlignsTicketNumber confirms the ticket badge
// sits at the card's right edge, with a background-styled gap (not a bare
// unstyled string) between it and the title — the same background-bleed
// concern as renderCardBadges' segment separators.
func TestRenderCardTitleRowRightAlignsTicketNumber(t *testing.T) {
	bgColor := lipgloss.Color(testBg)
	out := renderCardTitleRow("Fix login", 42, 20, testBg)

	titleSeg := styles.CardTitleStyle.Background(bgColor).Render("Fix login")
	ticketSeg := styles.CardTicketStyle.Background(bgColor).Render("#42")
	gap := 20 - lipgloss.Width("Fix login") - lipgloss.Width("#42")
	spacer := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", gap))
	want := titleSeg + spacer + ticketSeg

	if out != want {
		t.Fatalf("renderCardTitleRow = %q, want %q", out, want)
	}
	if lipgloss.Width(out) != 20 {
		t.Fatalf("rendered title row width = %d, want exactly 20 (the card's width)", lipgloss.Width(out))
	}
}

// TestRenderCardTitleRowTruncatesLongTitles confirms a title too long to
// share its line with the ticket badge gets truncated rather than pushing
// the badge onto a word-wrapped second line (or overflowing the card's
// width outright).
func TestRenderCardTitleRowTruncatesLongTitles(t *testing.T) {
	longTitle := strings.Repeat("x", 50)
	out := renderCardTitleRow(longTitle, 7, 20, testBg)

	if lipgloss.Width(out) != 20 {
		t.Fatalf("rendered title row width = %d, want exactly 20 even with a title far longer than the card", lipgloss.Width(out))
	}
	if !strings.Contains(out, "#7") {
		t.Fatalf("truncated title row = %q, still expected to contain the ticket badge '#7'", out)
	}
}

// TestNoteStyleForCard is the regression guard for the note-text-illegible-
// when-highlighted bug: CardNoteStyle's foreground (ColorMuted) and
// CardFocusedStyle's background (ColorHighlight) are both mapped from the
// same theme field, so a focused card's note text must switch to
// CardNoteFocusedStyle (full-contrast foreground) rather than staying on
// CardNoteStyle, or it renders unreadable against its own background under
// every theme (since the mapping collision isn't theme-specific). Compares
// lipgloss.Style values directly rather than rendered ANSI output, since
// lipgloss.Style is a plain comparable struct and this sidesteps the test
// environment's terminal color-profile entirely.
func TestNoteStyleForCard(t *testing.T) {
	cases := []struct {
		name              string
		focused, isMoving bool
		want              lipgloss.Style
	}{
		{"unfocused", false, false, styles.CardNoteStyle},
		{"focused", true, false, styles.CardNoteFocusedStyle},
		{"focused but moving keeps the default background", true, true, styles.CardNoteStyle},
		{"unfocused and moving", false, true, styles.CardNoteStyle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := noteStyleForCard(c.focused, c.isMoving); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("noteStyleForCard(%v, %v) = %+v, want %+v", c.focused, c.isMoving, got, c.want)
			}
		})
	}
}
