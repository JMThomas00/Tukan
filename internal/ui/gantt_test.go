package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

func daysFromToday(n int) *time.Time {
	t := startOfToday().AddDate(0, 0, n)
	return &t
}

// TestEffectiveDates confirms the (start, end) collapsing rule: both dates
// set stay as-is; either one alone mirrors onto the other (a single-day
// marker); neither set yields nil, nil (the "unscheduled" case).
func TestEffectiveDates(t *testing.T) {
	start := daysFromToday(1)
	due := daysFromToday(5)

	s, e := effectiveDates(models.Card{StartDate: start, DueDate: due})
	if s != start || e != due {
		t.Fatalf("both set: got (%v, %v), want (%v, %v)", s, e, start, due)
	}

	s, e = effectiveDates(models.Card{DueDate: due})
	if s == nil || !s.Equal(*due) || e != due {
		t.Fatalf("due only: got (%v, %v), want both = %v", s, e, due)
	}

	s, e = effectiveDates(models.Card{StartDate: start})
	if e == nil || !e.Equal(*start) || s != start {
		t.Fatalf("start only: got (%v, %v), want both = %v", s, e, start)
	}

	s, e = effectiveDates(models.Card{})
	if s != nil || e != nil {
		t.Fatalf("neither set: got (%v, %v), want (nil, nil)", s, e)
	}
}

// TestGanttBarPlacement covers the core timeline-math cases: a bar fully
// inside the window, one clipped at the left/right edges, and one entirely
// outside the visible window (panned away from).
func TestGanttBarPlacement(t *testing.T) {
	windowStart := startOfToday()
	daysVisible := 10 // totalCols = 10 * ganttColsPerDay

	t.Run("fully inside the window", func(t *testing.T) {
		start := windowStart.AddDate(0, 0, 2)
		end := windowStart.AddDate(0, 0, 4)
		col, width, inView := ganttBarPlacement(windowStart, daysVisible, &start, &end)
		if !inView {
			t.Fatal("expected inView=true")
		}
		wantCol := 2 * ganttColsPerDay
		wantWidth := 3 * ganttColsPerDay // days 2,3,4 inclusive = 3 days
		if col != wantCol || width != wantWidth {
			t.Fatalf("got (col=%d, width=%d), want (col=%d, width=%d)", col, width, wantCol, wantWidth)
		}
	})

	t.Run("clipped at the left edge", func(t *testing.T) {
		start := windowStart.AddDate(0, 0, -3) // starts before the window
		end := windowStart.AddDate(0, 0, 1)
		col, width, inView := ganttBarPlacement(windowStart, daysVisible, &start, &end)
		if !inView {
			t.Fatal("expected inView=true (bar overlaps the window even though it starts before it)")
		}
		if col != 0 {
			t.Fatalf("col = %d, want 0 (clipped to the window's left edge)", col)
		}
		wantWidth := 2 * ganttColsPerDay // only days 0,1 are actually visible
		if width != wantWidth {
			t.Fatalf("width = %d, want %d", width, wantWidth)
		}
	})

	t.Run("clipped at the right edge", func(t *testing.T) {
		start := windowStart.AddDate(0, 0, 8)
		end := windowStart.AddDate(0, 0, 20) // ends far past the window
		col, width, inView := ganttBarPlacement(windowStart, daysVisible, &start, &end)
		if !inView {
			t.Fatal("expected inView=true")
		}
		totalCols := daysVisible * ganttColsPerDay
		if col+width != totalCols {
			t.Fatalf("bar extends to col %d, want it clipped exactly to totalCols=%d", col+width, totalCols)
		}
	})

	t.Run("entirely outside the window", func(t *testing.T) {
		start := windowStart.AddDate(0, 0, -30)
		end := windowStart.AddDate(0, 0, -20)
		_, _, inView := ganttBarPlacement(windowStart, daysVisible, &start, &end)
		if inView {
			t.Fatal("expected inView=false for a bar entirely before the window")
		}
	})
}

// TestGanttRowsFlattensLanesAndRespectsFilter confirms the row list is
// lane-header, then that lane's cards, per lane in order — and that the
// board's existing filter query narrows it exactly like kanban's
// visibleCards does, since both go through the same function.
func TestGanttRowsFlattensLanesAndRespectsFilter(t *testing.T) {
	db, b := newTestBoard(t)
	laneA, laneB := b.lanes[0], b.lanes[1]

	cardA, err := db.CreateCard(models.Card{LaneID: laneA.ID, Title: "Fix login bug"})
	if err != nil {
		t.Fatalf("create card A: %v", err)
	}
	cardB, err := db.CreateCard(models.Card{LaneID: laneB.ID, Title: "Update docs"})
	if err != nil {
		t.Fatalf("create card B: %v", err)
	}
	b.cards[laneA.ID] = []models.Card{cardA}
	b.cards[laneB.ID] = []models.Card{cardB}

	rows := b.ganttRows()
	// newTestBoard seeds 4 lanes total; every lane gets a header row
	// regardless of whether it has cards, plus one row per card.
	if len(rows) != len(b.lanes)+2 {
		t.Fatalf("len(rows) = %d, want %d (%d lane headers + 2 cards)", len(rows), len(b.lanes)+2, len(b.lanes))
	}
	if rows[0].kind != ganttRowLaneHeader || rows[0].lane.ID != laneA.ID {
		t.Fatalf("rows[0] = %+v, want laneA's header first", rows[0])
	}
	if rows[1].kind != ganttRowCard || rows[1].card.ID != cardA.ID {
		t.Fatalf("rows[1] = %+v, want cardA right after laneA's header", rows[1])
	}

	b.filterQuery = "bug"
	filtered := b.ganttRows()
	foundB := false
	for _, r := range filtered {
		if r.kind == ganttRowCard && r.card.ID == cardB.ID {
			foundB = true
		}
	}
	if foundB {
		t.Fatal("filtered ganttRows still contains a card that doesn't match the filter query")
	}
}

// TestGanttMoveCursorSkipsLaneHeaders confirms the cursor only ever rests
// on card rows, stepping over lane-header dividers automatically.
func TestGanttMoveCursorSkipsLaneHeaders(t *testing.T) {
	db, b := newTestBoard(t)
	laneA, laneB := b.lanes[0], b.lanes[1]

	cardA, err := db.CreateCard(models.Card{LaneID: laneA.ID, Title: "A"})
	if err != nil {
		t.Fatalf("create card A: %v", err)
	}
	cardB, err := db.CreateCard(models.Card{LaneID: laneB.ID, Title: "B"})
	if err != nil {
		t.Fatalf("create card B: %v", err)
	}
	b.cards[laneA.ID] = []models.Card{cardA}
	b.cards[laneB.ID] = []models.Card{cardB}

	b.viewMode = viewGantt
	b.ganttClampCursor()

	rows := b.ganttRows()
	if rows[b.ganttCursor].card.ID != cardA.ID {
		t.Fatalf("initial cursor card = %d, want cardA (%d)", rows[b.ganttCursor].card.ID, cardA.ID)
	}

	b.ganttMoveCursor(1)
	rows = b.ganttRows()
	if rows[b.ganttCursor].kind != ganttRowCard || rows[b.ganttCursor].card.ID != cardB.ID {
		t.Fatalf("after moving down once, cursor should land on cardB (skipping laneB's header row), got %+v", rows[b.ganttCursor])
	}

	// Moving down again should stay put — there's nothing after the last card.
	before := b.ganttCursor
	b.ganttMoveCursor(1)
	if b.ganttCursor != before {
		t.Fatalf("cursor moved past the last card row: %d -> %d", before, b.ganttCursor)
	}
}

// TestGanttClampCursorHandlesEmptyBoard confirms clamping a Gantt cursor
// against a board with no cards at all doesn't panic and leaves the cursor
// at a safe zero value.
func TestGanttClampCursorHandlesEmptyBoard(t *testing.T) {
	_, b := newTestBoard(t)
	b.viewMode = viewGantt
	b.ganttCursor = 99 // deliberately invalid

	b.ganttClampCursor()

	if b.focusedCard() != nil {
		t.Fatal("focusedCard() should be nil on a board with no cards")
	}
}

// TestGanttViewToggleAndFocusedCardParity drives the 'g' keybinding through
// BoardModel.Update and confirms focusedCard() correctly resolves via the
// Gantt cursor once toggled — the same function every handleKey action
// (edit, delete, etc.) already calls, so this is what makes interaction
// parity work without duplicating any mutation logic.
func TestGanttViewToggleAndFocusedCardParity(t *testing.T) {
	db, b := newTestBoard(t)
	laneID := b.lanes[0].ID
	card, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Only card"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	b.cards[laneID] = []models.Card{card}

	updated, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if updated.viewMode != viewGantt {
		t.Fatalf("viewMode after 'g' = %v, want viewGantt", updated.viewMode)
	}
	focused := updated.focusedCard()
	if focused == nil || focused.ID != card.ID {
		t.Fatalf("focusedCard() after toggling to Gantt = %+v, want card %d", focused, card.ID)
	}

	backToKanban, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if backToKanban.viewMode != viewKanban {
		t.Fatalf("viewMode after second 'g' = %v, want viewKanban", backToKanban.viewMode)
	}
}

// TestGanttViewRendersExactlyItsOwnHeight mirrors
// TestBoardViewRendersExactlyItsOwnHeight for ganttChrome — the same class
// of row-accounting bug (boardChrome) already happened once in this
// codebase, so the new Gantt-specific chrome constant gets the identical
// guard from day one instead of waiting to find out the hard way.
func TestGanttViewRendersExactlyItsOwnHeight(t *testing.T) {
	_, b := newTestBoard(t)
	b.width = 250
	b.viewMode = viewGantt

	for _, h := range []int{24, 40, 10} {
		b.height = h
		got := lipgloss.Height(b.View())
		if got != h {
			t.Fatalf("lipgloss.Height(b.View()) = %d at b.height=%d (Gantt view), want exactly %d", got, h, h)
		}
	}
}

// TestGanttMoveCardDisabledInGanttView confirms 'm' (move between lanes)
// is a no-op in Gantt view rather than entering modeMoving, which has no
// coherent meaning against a flat vertical list.
func TestGanttMoveCardDisabledInGanttView(t *testing.T) {
	db, b := newTestBoard(t)
	laneID := b.lanes[0].ID
	card, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Card"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	b.cards[laneID] = []models.Card{card}
	b.viewMode = viewGantt
	b.ganttClampCursor()

	updated, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if updated.mode == modeMoving {
		t.Fatal("'m' should not enter modeMoving while in Gantt view")
	}
}

// TestGanttTimelineCellsBarSegmentsCarryBackground guards the same class of
// bug renderCardBadges/renderCardAssignees were fixed for: every segment of
// a bar (leading blank run, the bar itself, trailing blank run) must carry
// its own explicit Background(bg), or an embedded ANSI reset partway
// through would revert to the terminal's default background instead of the
// row's actual (focused/unfocused) one. Unlike those two, the segments here
// are directly concatenated with no separator text between them, so there's
// no bgJoin call needed — just confirms each segment was actually built
// that way, by comparing against independently-constructed reference spans.
func TestGanttTimelineCellsBarSegmentsCarryBackground(t *testing.T) {
	windowStart := startOfToday()
	daysVisible := 10
	bg := "#2d3f76"
	bgColor := lipgloss.Color(bg)

	start := windowStart.AddDate(0, 0, 2)
	end := windowStart.AddDate(0, 0, 3)
	card := models.Card{StartDate: &start, DueDate: &end}

	out := ganttTimelineCells(card, bg, windowStart, daysVisible)

	startCol, width, inView := ganttBarPlacement(windowStart, daysVisible, &start, &end)
	if !inView {
		t.Fatal("test setup: expected the bar to be in view")
	}
	totalCols := daysVisible * ganttColsPerDay

	leading := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", startCol))
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorAccent)).Background(bgColor).Render(strings.Repeat("█", width))
	trailing := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", totalCols-startCol-width))
	want := leading + bar + trailing

	if out != want {
		t.Fatalf("ganttTimelineCells = %q, want %q", out, want)
	}
}

// TestGanttTimelineCellsOverdueBarUsesDangerColor confirms a card whose due
// date has passed renders its bar in the same overdue color
// renderCardBadges already uses, rather than the default accent color —
// via the same exact-comparison technique as the test above, since a plain
// substring search can't reliably find a color inside ANSI-encoded output.
func TestGanttTimelineCellsOverdueBarUsesDangerColor(t *testing.T) {
	windowStart := startOfToday().AddDate(0, 0, -5)
	daysVisible := 10
	bg := "#2d3f76"
	bgColor := lipgloss.Color(bg)

	due := startOfToday().AddDate(0, 0, -2)
	card := models.Card{DueDate: &due}

	out := ganttTimelineCells(card, bg, windowStart, daysVisible)

	startCol, width, inView := ganttBarPlacement(windowStart, daysVisible, &due, &due)
	if !inView {
		t.Fatal("test setup: expected the bar to be in view")
	}
	totalCols := daysVisible * ganttColsPerDay

	leading := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", startCol))
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorDanger)).Background(bgColor).Render(strings.Repeat("█", width))
	trailing := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", totalCols-startCol-width))
	want := leading + bar + trailing

	if out != want {
		t.Fatalf("overdue ganttTimelineCells = %q, want %q (bar styled with ColorDanger)", out, want)
	}
}

// TestGanttTimelineCellsUnscheduledCardShowsPlaceholder confirms a card
// with neither a start nor a due date renders the muted "—" placeholder
// instead of a bar, and still fills the full timeline width (so every
// row's timeline column stays the same width regardless of content, the
// same "always render something of a fixed size" convention
// renderCardBadges follows for its own line).
func TestGanttTimelineCellsUnscheduledCardShowsPlaceholder(t *testing.T) {
	out := ganttTimelineCells(models.Card{}, "#2d3f76", startOfToday(), 10)
	if !strings.Contains(out, "—") {
		t.Fatalf("expected the unscheduled placeholder '—' in %q", out)
	}
	if lipgloss.Width(out) != 10*ganttColsPerDay {
		t.Fatalf("width = %d, want %d (full timeline width even with no bar)", lipgloss.Width(out), 10*ganttColsPerDay)
	}
}
