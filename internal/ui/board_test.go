package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/models"
)

// newTestBoard builds a BoardModel against a real temp-file database with
// two seeded boards, for exercising Update() message routing directly —
// without a terminal — the same way CLAUDE.md's documented
// "doneMsg before the xActive guard" rule needs verifying for every modal.
func newTestBoard(t *testing.T) (*database.DB, BoardModel) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	boards, err := db.ListBoards()
	if err != nil || len(boards) != 1 {
		t.Fatalf("list boards: %v (%+v)", err, boards)
	}
	if err := db.SeedDefaultLanes(boards[0].ID); err != nil {
		t.Fatalf("seed lanes: %v", err)
	}
	second, err := db.CreateBoard("Second Board", 1)
	if err != nil {
		t.Fatalf("create second board: %v", err)
	}
	if err := db.SeedDefaultLanes(second.ID); err != nil {
		t.Fatalf("seed second board lanes: %v", err)
	}

	b, _ := NewBoard(db, 80, 24, "")
	b.boards = []models.Board{boards[0], second}
	b.currentBoardID = boards[0].ID
	lanes, err := db.ListLanesByBoard(boards[0].ID)
	if err != nil {
		t.Fatalf("list lanes: %v", err)
	}
	b.lanes = lanes
	b.cards = make(map[int64][]models.Card)
	b.cardLabels = make(map[int64][]models.Label)
	b.checklists = make(map[int64][]models.ChecklistItem)
	for _, l := range lanes {
		b.cards[l.ID] = nil
	}
	b.laneScroll = make([]int, len(lanes))
	return db, b
}

// TestBoardViewRendersExactlyItsOwnHeight guards the exact regression this
// codebase already hit once: boardChrome was briefly (and incorrectly)
// changed from 4 to 2 under the assumption that lipgloss's Style.Height(N)
// already includes a style's own border in that N — it doesn't (Height
// pads content, and applyBorder runs afterward, adding 2 more rows on top)
// — which made every lane render 2 rows taller than intended and pushed
// the whole board's rendered output past the terminal height, scrolling
// the top border out of view. If View()'s total line count ever drifts
// from b.height again, in either direction, this catches it immediately
// instead of only being visible as "the top looks cut off" in a screenshot.
//
// Width is fixed at 130 — wide enough that the normal-mode help bar's key
// hints (119 runes) don't word-wrap to a second line, which would change
// the expected height for a completely unrelated reason (a narrow
// terminal's help bar wrapping is a real but separate concern from
// boardChrome's row math, and isn't what this test is guarding).
func TestBoardViewRendersExactlyItsOwnHeight(t *testing.T) {
	_, b := newTestBoard(t)
	b.width = 130

	for _, h := range []int{24, 40, 10} {
		b.height = h
		got := lipgloss.Height(b.View())
		if got != h {
			t.Fatalf("lipgloss.Height(b.View()) = %d at b.height=%d, want exactly %d", got, h, h)
		}
	}
}

// TestRenderBoardHeaderIsAlwaysExactlyTwoLines guards boardChrome's fixed
// assumption directly: renderBoardHeader must never wrap to 3+ lines,
// however long the board name or however narrow the terminal, since
// boardChrome is a constant that can't react to the header growing.
func TestRenderBoardHeaderIsAlwaysExactlyTwoLines(t *testing.T) {
	_, b := newTestBoard(t)
	b.boards[0].Name = strings.Repeat("a very long board name ", 10)

	for _, w := range []int{0, 1, 10, 80, 200} {
		b.width = w
		if got := lipgloss.Height(b.renderBoardHeader()); got != 2 {
			t.Fatalf("renderBoardHeader height = %d at width=%d, want exactly 2", got, w)
		}
	}
}

// TestBoardStatsLineCountsAcrossLanes confirms the rollup counts (total,
// overdue, due-this-week, unassigned) are computed correctly across every
// lane on the board, not just the focused one.
func TestBoardStatsLineCountsAcrossLanes(t *testing.T) {
	db, b := newTestBoard(t)
	laneID := b.lanes[0].ID

	overdue, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Overdue"})
	if err != nil {
		t.Fatalf("create overdue card: %v", err)
	}
	past := startOfToday().AddDate(0, 0, -1)
	overdue.DueDate = &past
	soon, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Due soon"})
	if err != nil {
		t.Fatalf("create due-soon card: %v", err)
	}
	inThreeDays := startOfToday().AddDate(0, 0, 3)
	soon.DueDate = &inThreeDays
	plain, err := db.CreateCard(models.Card{LaneID: laneID, Title: "No due date"})
	if err != nil {
		t.Fatalf("create plain card: %v", err)
	}

	b.cards[laneID] = []models.Card{overdue, soon, plain}
	b.cardAssignees[plain.ID] = []models.Assignee{{ID: 1, Name: "Jordan"}}

	stats := b.boardStatsLine()
	if !strings.Contains(stats, "3 card(s)") {
		t.Fatalf("stats = %q, want '3 card(s)'", stats)
	}
	if !strings.Contains(stats, "1 overdue") {
		t.Fatalf("stats = %q, want '1 overdue'", stats)
	}
	if !strings.Contains(stats, "1 due this week") {
		t.Fatalf("stats = %q, want '1 due this week'", stats)
	}
	if !strings.Contains(stats, "2 unassigned") {
		t.Fatalf("stats = %q, want '2 unassigned' (overdue and due-soon cards have no assignee set)", stats)
	}
}

// TestBoardSwitcherDoneMsgHandledBeforeActiveGuard guards the exact footgun
// CLAUDE.md documents for cardFormDoneMsg/formActive: boardSwitcherDoneMsg
// arrives as a Cmd result while switcherActive is still true, so it must be
// intercepted before the switcherActive guard — not routed back into the
// (already-closed) switcher and silently dropped.
func TestBoardSwitcherDoneMsgHandledBeforeActiveGuard(t *testing.T) {
	_, b := newTestBoard(t)
	target := b.boards[1].ID

	b.switcherActive = true
	b.switcher = NewBoardSwitcher(b.boards, b.currentBoardID, b.width, b.height)

	updated, cmd := b.Update(boardSwitcherDoneMsg{action: switcherSelect, boardID: target})

	if updated.switcherActive {
		t.Fatal("switcherActive still true after boardSwitcherDoneMsg — the done message was swallowed by the switcherActive guard instead of being handled first")
	}
	if cmd == nil {
		t.Fatal("expected a switch-board command, got nil")
	}

	msg := cmd()
	switched, ok := msg.(boardSwitchedMsg)
	if !ok {
		t.Fatalf("expected boardSwitchedMsg, got %T", msg)
	}
	if switched.boardID != target {
		t.Fatalf("switched to board %d, want %d", switched.boardID, target)
	}

	final, _ := updated.Update(switched)
	if final.currentBoardID != target {
		t.Fatalf("currentBoardID = %d after boardSwitchedMsg, want %d", final.currentBoardID, target)
	}
	if len(final.lanes) != 4 {
		t.Fatalf("len(lanes) after switch = %d, want 4", len(final.lanes))
	}
}

func TestBoardSwitcherCancelDoesNotSwitch(t *testing.T) {
	_, b := newTestBoard(t)
	b.switcherActive = true
	b.switcher = NewBoardSwitcher(b.boards, b.currentBoardID, b.width, b.height)
	original := b.currentBoardID

	updated, cmd := b.Update(boardSwitcherDoneMsg{cancelled: true})
	if updated.switcherActive {
		t.Fatal("switcherActive still true after cancel")
	}
	if cmd != nil {
		t.Fatal("expected no command on cancel")
	}
	if updated.currentBoardID != original {
		t.Fatalf("currentBoardID changed on cancel: %d -> %d", original, updated.currentBoardID)
	}
}

// TestBoardSwitcherCreateFlow exercises the full create-board path: the
// done message from the switcher, the resulting DB command, and the
// board-model state update, confirming the new board is seeded with
// default lanes and becomes the active board.
func TestBoardSwitcherCreateFlow(t *testing.T) {
	_, b := newTestBoard(t)
	b.switcherActive = true
	b.switcher = NewBoardSwitcher(b.boards, b.currentBoardID, b.width, b.height)

	updated, cmd := b.Update(boardSwitcherDoneMsg{action: switcherCreated, name: "New Board"})
	if updated.switcherActive {
		t.Fatal("switcherActive still true after create done msg")
	}
	if cmd == nil {
		t.Fatal("expected a create-board command")
	}

	msg := cmd()
	created, ok := msg.(boardCreatedMsg)
	if !ok {
		t.Fatalf("expected boardCreatedMsg, got %T", msg)
	}
	if created.board.Name != "New Board" {
		t.Fatalf("created board name = %q, want %q", created.board.Name, "New Board")
	}
	if len(created.lanes) != 4 {
		t.Fatalf("new board lanes = %d, want 4 (default seeded)", len(created.lanes))
	}

	final, _ := updated.Update(created)
	if final.currentBoardID != created.board.ID {
		t.Fatal("did not switch to the newly created board")
	}
	if len(final.boards) != 3 {
		t.Fatalf("len(boards) = %d, want 3", len(final.boards))
	}
}

// TestBoardDeletedMsgSwitchesAwayFromDeletedBoard confirms deleting the
// currently active board falls back to another remaining board rather than
// leaving the UI pointed at a board that no longer exists.
func TestBoardDeletedMsgSwitchesAwayFromDeletedBoard(t *testing.T) {
	_, b := newTestBoard(t)
	deletedID := b.currentBoardID
	remainingID := b.boards[1].ID

	updated, cmd := b.Update(boardDeletedMsg(deletedID))
	if len(updated.boards) != 1 {
		t.Fatalf("len(boards) after delete = %d, want 1", len(updated.boards))
	}
	if cmd == nil {
		t.Fatal("expected a switch-away command after deleting the active board")
	}
	msg := cmd()
	switched, ok := msg.(boardSwitchedMsg)
	if !ok {
		t.Fatalf("expected boardSwitchedMsg, got %T", msg)
	}
	if switched.boardID != remainingID {
		t.Fatalf("switched to board %d, want remaining board %d", switched.boardID, remainingID)
	}
}
