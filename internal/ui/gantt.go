package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

// Fixed column budget for every Gantt row: a label column (card title), a
// ticket-number column, then whatever's left for the timeline itself. Kept
// as named consts (not scattered magic numbers) since renderGanttCardRow,
// ganttDateAxis, and ganttWindow all need to agree on the same layout, or
// the date axis header won't line up with the bars underneath it.
const (
	ganttLabelWidth  = 22
	ganttTicketWidth = 6
	ganttColsPerDay  = 3
	// ganttChrome accounts for every row consumed outside the scrollable
	// row list itself: the 2-line board header, the 1-line date axis, and
	// the 1-line status/help bars below — mirrors boardChrome's role for
	// kanban, but Gantt rows have no per-row border to add back (unlike a
	// kanban lane's bordered box), so there's no "+2" term here.
	ganttChrome = 5 // header (2) + date axis (1) + status bar (1) + help bar (1)
)

type ganttRowKind int

const (
	ganttRowLaneHeader ganttRowKind = iota
	ganttRowCard
)

// ganttRow is one physical line in the Gantt view's row list — either a
// lane-group divider or a card. Cursor and scroll both index into this same
// list (not a cards-only list), which is what lets ganttMoveCursor skip
// divider rows while walking the list in one direction without needing a
// second, separately-indexed structure to keep in sync.
type ganttRow struct {
	kind ganttRowKind
	lane models.Lane
	card models.Card
}

// ganttRows flattens every lane's visible cards (post-filter, via the same
// visibleCards used everywhere else) into one ordered list, lane by lane —
// so the board's existing "/" search applies identically in both views for
// free, and the "row per card, grouped by lane" layout is just this list
// rendered in order.
func (b BoardModel) ganttRows() []ganttRow {
	var rows []ganttRow
	for _, lane := range b.lanes {
		rows = append(rows, ganttRow{kind: ganttRowLaneHeader, lane: lane})
		for _, c := range b.visibleCards(lane.ID) {
			rows = append(rows, ganttRow{kind: ganttRowCard, lane: lane, card: c})
		}
	}
	return rows
}

func firstCardRow(rows []ganttRow) int {
	for i, r := range rows {
		if r.kind == ganttRowCard {
			return i
		}
	}
	return -1
}

// ganttMoveCursor steps the cursor by one card row in the given direction,
// skipping over any lane-header rows in between — it never rests on a
// divider. Bounded by len(rows) so it can't loop forever if something about
// the row list is unexpectedly malformed.
func (b *BoardModel) ganttMoveCursor(delta int) {
	rows := b.ganttRows()
	if len(rows) == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	i := b.ganttCursor
	for range rows {
		i += step
		if i < 0 || i >= len(rows) {
			return
		}
		if rows[i].kind == ganttRowCard {
			b.ganttCursor = i
			b.ganttAdjustScroll(len(rows))
			return
		}
	}
}

// ganttClampCursor keeps the cursor pointing at a real card row after the
// underlying data changes (a card created/deleted, a filter applied, a
// lane added/removed) — called from clampCursor() alongside its kanban
// counterparts, so every existing call site that already keeps
// focusLane/focusCard valid keeps the Gantt cursor valid too, with no
// additional call sites needed.
func (b *BoardModel) ganttClampCursor() {
	rows := b.ganttRows()
	if len(rows) == 0 {
		b.ganttCursor = 0
		b.ganttScroll = 0
		return
	}
	if b.ganttCursor < 0 || b.ganttCursor >= len(rows) || rows[b.ganttCursor].kind != ganttRowCard {
		fc := firstCardRow(rows)
		if fc < 0 {
			fc = 0
		}
		b.ganttCursor = fc
	}
	b.ganttAdjustScroll(len(rows))
}

func (b BoardModel) ganttVisibleRowCount() int {
	n := b.height - ganttChrome
	if n < 1 {
		n = 1
	}
	return n
}

func (b *BoardModel) ganttAdjustScroll(totalRows int) {
	visible := b.ganttVisibleRowCount()
	clampScrollToCursor(b.ganttCursor, visible, &b.ganttScroll)
	maxScroll := totalRows - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if b.ganttScroll > maxScroll {
		b.ganttScroll = maxScroll
	}
	if b.ganttScroll < 0 {
		b.ganttScroll = 0
	}
}

// ganttWindow computes the visible timeline: colsPerDay is fixed, daysVisible
// derives from however much width is left after the label/ticket columns,
// and windowStart centers on today (with a bit of past context, biased
// toward the future) shifted by ganttDayOffset — panning left/right moves
// this window earlier/later, matching the bars' own left-to-right time
// direction.
func (b BoardModel) ganttWindow() (windowStart time.Time, daysVisible int) {
	timelineWidth := b.width - ganttLabelWidth - ganttTicketWidth - 1
	daysVisible = timelineWidth / ganttColsPerDay
	if daysVisible < 7 {
		daysVisible = 7
	}
	pastContext := daysVisible / 4
	windowStart = startOfToday().AddDate(0, 0, -pastContext+b.ganttDayOffset)
	return windowStart, daysVisible
}

// effectiveDates collapses a card's (optional) start and due dates into a
// single (start, end) pair for bar placement: either both set (a real
// duration bar), or one set and the other mirrored from it (a single-day
// marker), or both nil (no bar at all — see ganttTimelineCells).
func effectiveDates(card models.Card) (*time.Time, *time.Time) {
	start, end := card.StartDate, card.DueDate
	if start == nil {
		start = end
	}
	if end == nil {
		end = start
	}
	return start, end
}

// ganttBarPlacement converts a card's effective date range into a
// (startCol, width) span within the visible timeline, clipped to it —
// inView is false when the whole span falls outside the current window
// (panned away from), not just when the card has no dates at all.
func ganttBarPlacement(windowStart time.Time, daysVisible int, start, end *time.Time) (startCol, width int, inView bool) {
	totalCols := daysVisible * ganttColsPerDay

	dayStart := int(start.Sub(windowStart).Hours() / 24)
	dayEnd := int(end.Sub(windowStart).Hours() / 24)
	if dayEnd < dayStart {
		dayEnd = dayStart
	}

	startCol = dayStart * ganttColsPerDay
	width = (dayEnd - dayStart + 1) * ganttColsPerDay

	if startCol+width <= 0 || startCol >= totalCols {
		return 0, 0, false
	}
	if startCol < 0 {
		width += startCol
		startCol = 0
	}
	if startCol+width > totalCols {
		width = totalCols - startCol
	}
	if width < 1 {
		width = 1
	}
	return startCol, width, true
}

// ganttTimelineCells renders one card's bar (or an "unscheduled" placeholder)
// across the full timeline width. Every segment is directly concatenated
// with no literal separator text between them — each carries its own
// explicit Background(bg), so unlike renderCardBadges' segments (joined
// with a literal space/double-space that needed its own styling via
// bgJoin), there's no unstyled glue text here for the background-bleed bug
// to hide in.
func ganttTimelineCells(card models.Card, bg string, windowStart time.Time, daysVisible int) string {
	totalCols := daysVisible * ganttColsPerDay
	bgColor := lipgloss.Color(bg)

	start, end := effectiveDates(card)
	if start == nil {
		placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorSubtext)).Background(bgColor).Render("—")
		pad := totalCols - lipgloss.Width(placeholder)
		if pad < 0 {
			pad = 0
		}
		return placeholder + lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", pad))
	}

	barColor := styles.ColorAccent
	startCol, width, inView := ganttBarPlacement(windowStart, daysVisible, start, end)
	if card.DueDate != nil && card.DueDate.Before(startOfToday()) {
		barColor = styles.ColorDanger
	}

	if !inView {
		return lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", totalCols))
	}

	leading := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", startCol))
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(barColor)).Background(bgColor).Render(strings.Repeat("█", width))
	trailingWidth := totalCols - startCol - width
	if trailingWidth < 0 {
		trailingWidth = 0
	}
	trailing := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", trailingWidth))

	return leading + bar + trailing
}

// laneAccentColor picks the same header color a kanban lane column would —
// the lane's own explicit color if it has one, else a rotating default from
// styles.LaneColors by position. Shared by the lane divider and every
// card's bar within it, so a card's bar color visually ties back to its
// lane the same way the kanban view already does.
func laneAccentColor(lane models.Lane, idx int) string {
	if lane.Color != "" {
		return lane.Color
	}
	if idx < len(styles.LaneColors) {
		return styles.LaneColors[idx]
	}
	return styles.ColorAccent
}

// renderGanttLaneDivider deliberately does NOT reuse styles.LaneHeaderStyle
// — that style has MarginBottom(1) baked in, tailored for its one existing
// use site (renderLane, where the blank line it adds is exactly the gap
// wanted between a kanban lane's header and its first card). Reusing it
// here made every divider silently render as 2 physical lines instead of
// 1, throwing off ganttChrome's row accounting by one line per lane.
func renderGanttLaneDivider(lane models.Lane, idx int) string {
	color := laneAccentColor(lane, idx)
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(lane.Name)
}

// renderGanttCardRow renders one card's full row: title (truncated to fit,
// same rune-slicing convention renderCardTitleRow established), ticket
// number, then its bar. focused swaps the row's background exactly like a
// kanban card's own focused-background swap — the background change IS the
// focus indicator, no separate text-style change layered on top of it.
func (b BoardModel) renderGanttCardRow(card models.Card, focused bool, windowStart time.Time, daysVisible int) string {
	bg := styles.ColorSurface
	if focused {
		bg = styles.ColorHighlight
	}
	bgColor := lipgloss.Color(bg)

	title := card.Title
	if r := []rune(title); len(r) > ganttLabelWidth-1 {
		title = string(r[:ganttLabelWidth-1])
	}
	label := styles.CardTitleStyle.Background(bgColor).Width(ganttLabelWidth).Render(title)

	ticket := lipgloss.NewStyle().
		Foreground(lipgloss.Color(styles.ColorSubtext)).
		Background(bgColor).
		Width(ganttTicketWidth).
		Render(fmt.Sprintf("#%d", card.TicketNo))

	timeline := ganttTimelineCells(card, bg, windowStart, daysVisible)

	return label + ticket + timeline
}

// ganttDateAxis renders the header ruler above the rows: a date label at
// the start of each week, and a distinct marker for whichever column is
// "today". Built as plain text first, then split into three styled spans
// around the today column — no separator-gap concern here (see
// ganttTimelineCells' own comment for the pattern this deliberately does
// NOT need): this line is never background-highlighted the way a focused
// row is, so an embedded reset between spans just falls back to the
// terminal's own default background, which is already what the rest of
// this line is rendered against.
func ganttDateAxis(windowStart time.Time, daysVisible int) string {
	totalCols := daysVisible * ganttColsPerDay
	ruler := make([]byte, totalCols)
	for i := range ruler {
		ruler[i] = ' '
	}

	today := startOfToday()
	todayCol := -1
	for d := 0; d < daysVisible; d++ {
		day := windowStart.AddDate(0, 0, d)
		col := d * ganttColsPerDay
		if day.Equal(today) {
			todayCol = col
		}
		if d%7 == 0 {
			label := day.Format("1/2")
			for i := 0; i < len(label) && col+i < totalCols; i++ {
				ruler[col+i] = label[i]
			}
		}
	}

	prefix := strings.Repeat(" ", ganttLabelWidth+ganttTicketWidth)
	axisText := string(ruler)

	if todayCol < 0 || todayCol >= totalCols {
		return prefix + styles.HelpDescStyle.Render(axisText)
	}

	before := axisText[:todayCol]
	marker := axisText[todayCol : todayCol+1]
	if marker == " " {
		marker = "▏"
	}
	after := axisText[todayCol+1:]

	return prefix +
		styles.HelpDescStyle.Render(before) +
		styles.CardOverdueStyle.Render(marker) +
		styles.HelpDescStyle.Render(after)
}

// renderGanttView renders the full scrollable body: the date axis, then
// whatever slice of ganttRows() fits in the current scroll window.
func (b BoardModel) renderGanttView() string {
	windowStart, daysVisible := b.ganttWindow()
	rows := b.ganttRows()
	visible := b.ganttVisibleRowCount()

	lines := []string{ganttDateAxis(windowStart, daysVisible)}

	if len(rows) == 0 {
		lines = append(lines, styles.CardNoteStyle.Render("No cards yet — press n to add one"))
	} else {
		end := b.ganttScroll + visible
		if end > len(rows) {
			end = len(rows)
		}
		start := b.ganttScroll
		if start > end {
			start = end
		}

		laneIdx := make(map[int64]int, len(b.lanes))
		for i, l := range b.lanes {
			laneIdx[l.ID] = i
		}

		for i := start; i < end; i++ {
			row := rows[i]
			switch row.kind {
			case ganttRowLaneHeader:
				lines = append(lines, renderGanttLaneDivider(row.lane, laneIdx[row.lane.ID]))
			case ganttRowCard:
				lines = append(lines, b.renderGanttCardRow(row.card, i == b.ganttCursor, windowStart, daysVisible))
			}
		}
	}

	// Pad to the fixed row budget so the Gantt body always fills the
	// available height — kanban's lane boxes get this for free from
	// lipgloss's Height() padding each bordered box; a plain joined string
	// of lines has no equivalent, so a sparse board (few cards, or none)
	// would otherwise render a body far shorter than b.height, shrinking
	// View()'s total output out from under ganttChrome's row accounting.
	for len(lines) < 1+visible {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// newCardLaneID resolves which lane a new card (the "n" key) should land
// in. Kanban keeps its existing behavior exactly (the cursor's lane column,
// even when that lane is empty and so has no focused card) — Gantt has no
// equivalent "empty column" concept to point at, so it falls back through
// the focused row's lane, then the first lane, then no lane at all (an
// empty board).
func (b BoardModel) newCardLaneID() int64 {
	if b.viewMode == viewGantt {
		if card := b.focusedCard(); card != nil {
			return card.LaneID
		}
		if len(b.lanes) > 0 {
			return b.lanes[0].ID
		}
		return 0
	}
	if len(b.lanes) == 0 {
		return 0
	}
	return b.lanes[b.focusLane].ID
}
