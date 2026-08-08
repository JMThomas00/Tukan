package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/models"
)

// TestLaneManagerRefusesToDeleteLastLane mirrors the board switcher's own
// "refuse to delete the last board" guard — a board needs at least one lane
// to hold cards.
func TestLaneManagerRefusesToDeleteLastLane(t *testing.T) {
	m := NewLaneManager(1, []models.Lane{{ID: 1, Name: "Only Lane"}}, nil, 80, 24)

	updated, _ := m.updateBrowsing(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, DefaultKeyMap)
	if updated.mode != laneManagerBrowsing {
		t.Fatalf("mode = %v after 'd' on the last lane, want laneManagerBrowsing (delete refused)", updated.mode)
	}
}

// TestLaneManagerReorderAtEdgesIsANoOp confirms moving the first lane left
// (or the last lane right) doesn't emit a swap command — there's no
// neighbor on that side to swap with.
func TestLaneManagerReorderAtEdgesIsANoOp(t *testing.T) {
	lanes := []models.Lane{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	m := NewLaneManager(1, lanes, nil, 80, 24)
	m.cursor = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd != nil {
		t.Fatal("expected no reorder command moving the first lane further left")
	}

	m.cursor = 1
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd != nil {
		t.Fatal("expected no reorder command moving the last lane further right")
	}
}

// TestLaneManagerReorderEmitsSwap confirms moving a lane right emits a
// reorder doneMsg naming both lanes involved in the swap.
func TestLaneManagerReorderEmitsSwap(t *testing.T) {
	lanes := []models.Lane{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	m := NewLaneManager(1, lanes, nil, 80, 24)
	m.cursor = 0

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd == nil {
		t.Fatal("expected a reorder command")
	}
	done, ok := cmd().(laneManagerDoneMsg)
	if !ok {
		t.Fatalf("expected laneManagerDoneMsg, got %T", cmd())
	}
	if done.action != laneManagerReordered || done.laneID != 1 || done.otherID != 2 {
		t.Fatalf("laneManagerDoneMsg = %+v, want reordered lane 1 with lane 2", done)
	}
	if updated.cursor != 1 {
		t.Fatalf("cursor after moving right = %d, want 1 (follows the lane it moved)", updated.cursor)
	}
}

// TestLaneManagerDoneMsgHandledBeforeActiveGuard guards the same footgun
// documented for every other modal in this codebase: laneManagerDoneMsg
// arrives as a Cmd result while laneManagerActive is still true, so a
// non-cancel action (create, here) must be intercepted before the
// laneManagerActive guard routes it into LaneManagerModel.Update
// (tea.KeyMsg only) and drops it.
func TestLaneManagerDoneMsgHandledBeforeActiveGuard(t *testing.T) {
	db, b := newTestBoard(t)
	b.laneManager = NewLaneManager(b.currentBoardID, b.lanes, cardCountsByLane(b.cards), b.width, b.height)
	b.laneManagerActive = true

	updated, cmd := b.Update(laneManagerDoneMsg{action: laneManagerCreated, name: "New Lane"})
	if !updated.laneManagerActive {
		t.Fatal("laneManagerActive became false after a create message — only cancel should close the modal")
	}
	if cmd == nil {
		t.Fatal("expected a create-lane command, got nil")
	}

	msg := cmd()
	reloaded, ok := msg.(lanesReloadedMsg)
	if !ok {
		t.Fatalf("expected lanesReloadedMsg, got %T", msg)
	}

	final, _ := updated.Update(reloaded)
	found := false
	for _, l := range final.lanes {
		if l.Name == "New Lane" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lanes after create = %+v, want 'New Lane' among them", final.lanes)
	}
	if !final.laneManagerActive {
		t.Fatal("laneManagerActive should still be true after a create — it only closes on cancel")
	}
	if _, err := db.ListLanesByBoard(b.currentBoardID); err != nil {
		t.Fatalf("list lanes for board: %v", err)
	}
}

// TestLaneManagerCancelClosesTheModal confirms esc from browsing mode is
// the one action that actually closes the modal.
func TestLaneManagerCancelClosesTheModal(t *testing.T) {
	_, b := newTestBoard(t)
	b.laneManager = NewLaneManager(b.currentBoardID, b.lanes, cardCountsByLane(b.cards), b.width, b.height)
	b.laneManagerActive = true

	updated, cmd := b.Update(laneManagerDoneMsg{cancelled: true})
	if updated.laneManagerActive {
		t.Fatal("laneManagerActive should become false on cancel")
	}
	if cmd != nil {
		t.Fatal("expected no command on cancel")
	}
}
