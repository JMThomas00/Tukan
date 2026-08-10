package pluginserver

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
)

// TestChannelBoardEnterRendersInitialFrame confirms Enter loads the mapped
// board (not some other one) and returns a Seq-0 frame containing its lane
// names.
func TestChannelBoardEnterRendersInitialFrame(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	cb := newChannelBoard(boardID)

	push, err := cb.Enter(db, uuid.New(), 80, 24, "")
	if err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if push.Seq != 0 {
		t.Fatalf("Seq = %d, want 0 for a fresh viewer", push.Seq)
	}
	if !strings.Contains(push.Frame, "To-Do") {
		t.Fatalf("frame doesn't contain a default lane name, want to see To-Do:\n%s", push.Frame)
	}
}

// TestChannelBoardInputAppliesBatchedSaveAndPersists drives the same
// n -> type title -> ctrl+s flow drive_test.go exercises directly, but
// through ChannelBoard.Input, confirming the batched card-form save
// actually lands in the database when driven this way and Seq increments
// on each keypress.
func TestChannelBoardInputAppliesBatchedSaveAndPersists(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	cb := newChannelBoard(boardID)
	viewerID := uuid.New()

	if _, err := cb.Enter(db, viewerID, 80, 24, ""); err != nil {
		t.Fatalf("Enter: %v", err)
	}

	steps := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X1")},
		tea.KeyMsg{Type: tea.KeyCtrlS},
	}
	var lastSeq int64
	for _, msg := range steps {
		push, _, _, ok := cb.Input(viewerID, msg)
		if !ok {
			t.Fatalf("Input(%v) reported unknown viewer", msg)
		}
		if push.Seq <= lastSeq {
			t.Fatalf("Seq = %d, want > previous %d", push.Seq, lastSeq)
		}
		lastSeq = push.Seq
	}

	cards, err := db.ListCardsByBoard(boardID)
	if err != nil {
		t.Fatalf("list cards: %v", err)
	}
	found := false
	for _, c := range cards {
		if c.Title == "X1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cards = %+v, want to find the card saved through ChannelBoard.Input", cards)
	}
}

// TestChannelBoardBroadcastReloadUpdatesOtherViewersNotSelf is the core
// multi-viewer collaboration guarantee at the ChannelBoard level: viewer A
// creates a card; BroadcastReload(exceptA) must push a refreshed frame to
// viewer B (who sees the new card) while leaving A alone (A already saw its
// own change via Input's own return).
func TestChannelBoardBroadcastReloadUpdatesOtherViewersNotSelf(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	cb := newChannelBoard(boardID)
	viewerA, viewerB := uuid.New(), uuid.New()

	if _, err := cb.Enter(db, viewerA, 80, 24, ""); err != nil {
		t.Fatalf("Enter A: %v", err)
	}
	if _, err := cb.Enter(db, viewerB, 80, 24, ""); err != nil {
		t.Fatalf("Enter B: %v", err)
	}

	for _, msg := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Made by A")},
		tea.KeyMsg{Type: tea.KeyCtrlS},
	} {
		if _, _, _, ok := cb.Input(viewerA, msg); !ok {
			t.Fatalf("Input(A, %v) reported unknown viewer", msg)
		}
	}

	pushes, err := cb.BroadcastReload(viewerA)
	if err != nil {
		t.Fatalf("BroadcastReload: %v", err)
	}
	if len(pushes) != 1 || pushes[0].ViewerID != viewerB {
		t.Fatalf("pushes = %+v, want exactly one push for viewer B", pushes)
	}
	if !strings.Contains(pushes[0].Frame, "Made by A") {
		t.Fatalf("viewer B's reloaded frame doesn't contain the new card:\n%s", pushes[0].Frame)
	}
	if pushes[0].Seq != 1 {
		t.Fatalf("viewer B's Seq = %d, want 1 (first push after Enter's Seq 0)", pushes[0].Seq)
	}
}

// TestChannelBoardInputSnapshotsOnlyDifferOnRealMutation is the
// correctness half of the fix that replaced per-keystroke database
// re-querying with a cheap in-memory diff (the DB-querying version made
// every keystroke — including plain text typing that never touches the
// database — noticeably laggy): opening the form and typing into the
// title field must report before==after (nothing saved yet), while the
// ctrl+s save step must report a real difference.
func TestChannelBoardInputSnapshotsOnlyDifferOnRealMutation(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	cb := newChannelBoard(boardID)
	viewerID := uuid.New()

	if _, err := cb.Enter(db, viewerID, 80, 24, ""); err != nil {
		t.Fatalf("Enter: %v", err)
	}

	_, before, after, ok := cb.Input(viewerID, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !ok {
		t.Fatal("Input('n') reported unknown viewer")
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("opening the new-card form changed the snapshot, want unchanged:\nbefore=%+v\nafter=%+v", before, after)
	}

	_, before, after, ok = cb.Input(viewerID, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Snapshot Test")})
	if !ok {
		t.Fatal("Input(typing title) reported unknown viewer")
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("typing into the title field changed the snapshot before saving, want unchanged:\nbefore=%+v\nafter=%+v", before, after)
	}

	_, before, after, ok = cb.Input(viewerID, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !ok {
		t.Fatal("Input(ctrl+s) reported unknown viewer")
	}
	if reflect.DeepEqual(before, after) {
		t.Fatal("saving the card left the snapshot unchanged, want a real difference")
	}
}

// TestChannelBoardResizeUnknownViewerReturnsFalse and
// TestChannelBoardInputUnknownViewerReturnsFalse confirm a stray event for a
// viewer that already left (or never entered) is reported, not silently
// ignored or panicking — server.go needs this to know not to push a frame
// nobody's listening for.
func TestChannelBoardResizeUnknownViewerReturnsFalse(t *testing.T) {
	_, _, boardID := newTestBoard(t)
	cb := newChannelBoard(boardID)
	if _, ok := cb.Resize(uuid.New(), 80, 24); ok {
		t.Fatal("Resize for an unknown viewer reported ok=true, want false")
	}
}

func TestChannelBoardInputUnknownViewerReturnsFalse(t *testing.T) {
	_, _, boardID := newTestBoard(t)
	cb := newChannelBoard(boardID)
	if _, _, _, ok := cb.Input(uuid.New(), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}); ok {
		t.Fatal("Input for an unknown viewer reported ok=true, want false")
	}
}

// TestChannelBoardLeaveDropsViewer confirms Leave actually removes the
// viewer (ViewerCount drops, and a subsequent Input reports unknown) rather
// than just marking it inactive.
func TestChannelBoardLeaveDropsViewer(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	cb := newChannelBoard(boardID)
	viewerID := uuid.New()
	if _, err := cb.Enter(db, viewerID, 80, 24, ""); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if got := cb.ViewerCount(); got != 1 {
		t.Fatalf("ViewerCount after Enter = %d, want 1", got)
	}

	cb.Leave(viewerID)
	if got := cb.ViewerCount(); got != 0 {
		t.Fatalf("ViewerCount after Leave = %d, want 0", got)
	}
	if _, _, _, ok := cb.Input(viewerID, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}); ok {
		t.Fatal("Input after Leave reported ok=true, want false")
	}
}
