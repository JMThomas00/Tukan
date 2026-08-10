package ui

import (
	"reflect"
	"testing"

	"github.com/JMThomas00/tukan/internal/models"
)

// TestNewBoardForIDLoadsTheSpecificBoard confirms it loads whichever board
// ID was asked for — not "the first board in the file" the way NewBoard's
// own cmdLoad does, which would be wrong once a database holds more than
// one board (as server mode's does, one per Concord channel).
func TestNewBoardForIDLoadsTheSpecificBoard(t *testing.T) {
	db, seed := newTestBoard(t)
	first, second := seed.boards[0], seed.boards[1]

	loaded, err := NewBoardForID(db, second.ID, 80, 24, "")
	if err != nil {
		t.Fatalf("NewBoardForID: %v", err)
	}
	if loaded.currentBoardID != second.ID {
		t.Fatalf("currentBoardID = %d, want %d (the second board, not the first)", loaded.currentBoardID, second.ID)
	}
	if len(loaded.boards) != 1 || loaded.boards[0].ID != second.ID {
		t.Fatalf("boards = %+v, want just the requested board", loaded.boards)
	}
	if loaded.currentBoardName() != second.Name {
		t.Fatalf("currentBoardName() = %q, want %q", loaded.currentBoardName(), second.Name)
	}

	// And loading the first board by ID directly should NOT silently
	// return the same instance/data as the second — confirms this isn't
	// accidentally still hardcoded to "whichever board is first".
	loadedFirst, err := NewBoardForID(db, first.ID, 80, 24, "")
	if err != nil {
		t.Fatalf("NewBoardForID (first): %v", err)
	}
	if loadedFirst.currentBoardID != first.ID {
		t.Fatalf("currentBoardID = %d, want %d", loadedFirst.currentBoardID, first.ID)
	}
}

// TestReloadPicksUpAnotherViewersMutation is the actual multi-viewer
// collaboration guarantee server mode depends on: two independent
// BoardModel instances (standing in for two different Concord viewers)
// pointed at the same database. A mutation made through one is invisible
// to the other until Reload() is called — proving both that Reload() works,
// and that it's genuinely necessary (BoardModel doesn't somehow share state
// automatically just by using the same *database.DB).
func TestReloadPicksUpAnotherViewersMutation(t *testing.T) {
	db, seed := newTestBoard(t)
	boardID := seed.boards[0].ID
	laneID := seed.lanes[0].ID

	viewerA, err := NewBoardForID(db, boardID, 80, 24, "")
	if err != nil {
		t.Fatalf("load viewer A: %v", err)
	}
	viewerB, err := NewBoardForID(db, boardID, 80, 24, "")
	if err != nil {
		t.Fatalf("load viewer B: %v", err)
	}

	created, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Made by viewer A"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	// Before either reloads, neither viewer's in-memory copy has the new
	// card — confirms they're genuinely independent, not aliased.
	if len(viewerA.cards[laneID]) != 0 || len(viewerB.cards[laneID]) != 0 {
		t.Fatalf("viewers should not see the new card before Reload(): A=%+v B=%+v", viewerA.cards[laneID], viewerB.cards[laneID])
	}

	if err := viewerB.Reload(); err != nil {
		t.Fatalf("viewer B Reload: %v", err)
	}
	found := false
	for _, c := range viewerB.cards[laneID] {
		if c.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("viewer B's cards after Reload = %+v, want to include the card viewer A created", viewerB.cards[laneID])
	}

	// Viewer A, who hasn't reloaded, still shouldn't see it — Reload() is
	// per-instance, not a global side effect.
	if len(viewerA.cards[laneID]) != 0 {
		t.Fatalf("viewer A's cards changed without calling Reload(): %+v", viewerA.cards[laneID])
	}
}

// TestReloadPreservesCursorAndMode confirms Reload() is the "quiet resync"
// variant, not a fresh boardLoadedMsg-style load — a viewer's own cursor
// position and interaction mode must survive a reload triggered by someone
// else's action, or every other viewer's screen would jump around every
// time anyone made a change.
func TestReloadPreservesCursorAndMode(t *testing.T) {
	db, seed := newTestBoard(t)
	boardID := seed.boards[0].ID
	laneID := seed.lanes[0].ID

	if _, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Card 1"}); err != nil {
		t.Fatalf("create card 1: %v", err)
	}
	if _, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Card 2"}); err != nil {
		t.Fatalf("create card 2: %v", err)
	}

	viewer, err := NewBoardForID(db, boardID, 80, 24, "")
	if err != nil {
		t.Fatalf("load viewer: %v", err)
	}
	viewer.focusCard = 1
	viewer.mode = modeFilterEdit
	viewer.filterQuery = "card" // matches both cards, so index 1 stays a valid position

	if err := viewer.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if viewer.focusCard != 1 {
		t.Fatalf("focusCard after Reload = %d, want unchanged (1)", viewer.focusCard)
	}
	if viewer.mode != modeFilterEdit {
		t.Fatalf("mode after Reload = %v, want unchanged (modeFilterEdit)", viewer.mode)
	}
	if viewer.filterQuery != "card" {
		t.Fatalf("filterQuery after Reload = %q, want unchanged", viewer.filterQuery)
	}
}

// TestIsMainViewReflectsModalAndModeState confirms IsMainView() matches
// exactly the condition standalone's own 'q' quit binding requires —
// server mode relies on this to decide when a 'q' keypress safely means
// "leave the pane" instead of doing nothing useful or being swallowed as
// ordinary typed text inside a form field.
func TestIsMainViewReflectsModalAndModeState(t *testing.T) {
	db, seed := newTestBoard(t)
	boardID := seed.boards[0].ID

	viewer, err := NewBoardForID(db, boardID, 80, 24, "")
	if err != nil {
		t.Fatalf("load viewer: %v", err)
	}
	if !viewer.IsMainView() {
		t.Fatal("a freshly loaded board should be on its main view")
	}

	withFormActive := viewer
	withFormActive.formActive = true
	if withFormActive.IsMainView() {
		t.Fatal("IsMainView() should be false while the card form is open")
	}

	withConfirmDelete := viewer
	withConfirmDelete.mode = modeConfirmDelete
	if withConfirmDelete.IsMainView() {
		t.Fatal("IsMainView() should be false during confirm-delete")
	}

	withFilterEdit := viewer
	withFilterEdit.mode = modeFilterEdit
	if withFilterEdit.IsMainView() {
		t.Fatal("IsMainView() should be false while editing the filter query")
	}
}

// TestSnapshotDetectsCardMutation confirms the basic contract: a snapshot
// taken before a real mutation (created via Reload after a direct DB
// write, standing in for what Update()'s own mutation handlers do) differs
// from one taken after, and an unrelated pair of snapshots of an unchanged
// board compare equal.
func TestSnapshotDetectsCardMutation(t *testing.T) {
	db, seed := newTestBoard(t)
	boardID := seed.boards[0].ID
	laneID := seed.lanes[0].ID

	viewer, err := NewBoardForID(db, boardID, 80, 24, "")
	if err != nil {
		t.Fatalf("load viewer: %v", err)
	}

	before := viewer.Snapshot()
	if !reflect.DeepEqual(before, viewer.Snapshot()) {
		t.Fatal("two snapshots of an unchanged board should be equal")
	}

	if _, err := db.CreateCard(models.Card{LaneID: laneID, Title: "New Card"}); err != nil {
		t.Fatalf("create card: %v", err)
	}
	if err := viewer.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	after := viewer.Snapshot()

	if reflect.DeepEqual(before, after) {
		t.Fatal("snapshot after a card was created should differ from before")
	}
	if len(after.Cards) != len(before.Cards)+1 {
		t.Fatalf("after.Cards has %d cards, want %d (before) + 1", len(after.Cards), len(before.Cards))
	}
}

// TestSnapshotIsNotAliasedByLaterMutation is the direct regression test for
// the exact bug Snapshot() must avoid: cardAssignees/cardLabels/checklists
// are maps, and BoardModel's own Update() mutation handlers write into them
// in place (e.g. `b.cardAssignees[c.ID] = msg.assignees`) rather than
// replacing the map wholesale. If Snapshot() returned those maps directly
// instead of copying them, a "before" snapshot would alias the same
// backing map a later in-place mutation touches — so taking a snapshot,
// then mutating the BoardModel's own map directly (simulating what
// Update() does), must NOT retroactively change the already-taken
// snapshot.
func TestSnapshotIsNotAliasedByLaterMutation(t *testing.T) {
	db, seed := newTestBoard(t)
	boardID := seed.boards[0].ID
	laneID := seed.lanes[0].ID

	created, err := db.CreateCard(models.Card{LaneID: laneID, Title: "Card"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	viewer, err := NewBoardForID(db, boardID, 80, 24, "")
	if err != nil {
		t.Fatalf("load viewer: %v", err)
	}

	before := viewer.Snapshot()
	if len(before.Assignees[created.ID]) != 0 {
		t.Fatalf("before.Assignees[%d] = %+v, want empty", created.ID, before.Assignees[created.ID])
	}

	// Mutate the BoardModel's own map in place, the same way Update()'s
	// cardAssigneesChangedMsg handler does — NOT through Snapshot()'s
	// returned copy.
	viewer.cardAssignees[created.ID] = []models.Assignee{{ID: 1, Name: "Jordan"}}

	if len(before.Assignees[created.ID]) != 0 {
		t.Fatalf("before.Assignees[%d] = %+v after a later in-place mutation, want still empty (snapshot was aliased, not copied)", created.ID, before.Assignees[created.ID])
	}
}
