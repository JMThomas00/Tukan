package database

import (
	"testing"
	"time"

	"github.com/JMThomas00/tukan/internal/models"
)

func newTestLane(t *testing.T, db *DB) models.Lane {
	t.Helper()
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	board, err := db.CreateBoard("Test Board", 0)
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	lane, err := db.CreateLane(board.ID, "Test Lane", 0)
	if err != nil {
		t.Fatalf("create lane: %v", err)
	}
	return lane
}

func TestCardDueDateRoundTrip(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)

	due := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	created, err := db.CreateCard(models.Card{
		LaneID:  lane.ID,
		Title:   "Ship it",
		DueDate: &due,
	})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	if created.DueDate == nil || !created.DueDate.Equal(due) {
		t.Fatalf("created.DueDate = %v, want %v", created.DueDate, due)
	}

	cards, err := db.ListCardsByLane(lane.ID)
	if err != nil {
		t.Fatalf("list cards: %v", err)
	}
	if len(cards) != 1 || cards[0].DueDate == nil || !cards[0].DueDate.Equal(due) {
		t.Fatalf("listed card due date = %v, want %v", cards[0].DueDate, due)
	}

	// Update to a new due date.
	old := cards[0]
	updated := old
	newDue := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	updated.DueDate = &newDue
	if err := db.UpdateCard(old, updated); err != nil {
		t.Fatalf("update card: %v", err)
	}
	cards, err = db.ListCardsByLane(lane.ID)
	if err != nil {
		t.Fatalf("list cards after update: %v", err)
	}
	if cards[0].DueDate == nil || !cards[0].DueDate.Equal(newDue) {
		t.Fatalf("card due date after update = %v, want %v", cards[0].DueDate, newDue)
	}

	// Clear the due date.
	beforeClear := cards[0]
	cleared := beforeClear
	cleared.DueDate = nil
	if err := db.UpdateCard(beforeClear, cleared); err != nil {
		t.Fatalf("update card (clear due date): %v", err)
	}
	cards, err = db.ListCardsByLane(lane.ID)
	if err != nil {
		t.Fatalf("list cards after clear: %v", err)
	}
	if cards[0].DueDate != nil {
		t.Fatalf("card due date after clear = %v, want nil", cards[0].DueDate)
	}
}

func TestCardWithoutDueDate(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)

	created, err := db.CreateCard(models.Card{
		LaneID: lane.ID,
		Title:  "No due date",
	})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	if created.DueDate != nil {
		t.Fatalf("created.DueDate = %v, want nil", created.DueDate)
	}

	cards, err := db.ListCardsByLane(lane.ID)
	if err != nil {
		t.Fatalf("list cards: %v", err)
	}
	if cards[0].DueDate != nil {
		t.Fatalf("listed card due date = %v, want nil", cards[0].DueDate)
	}
}

// TestCreateCardAssignsSequentialTicketNumbers confirms ticket numbers start
// at 1 per board and increment by one per card, in creation order.
func TestCreateCardAssignsSequentialTicketNumbers(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)

	for i, want := range []int{1, 2, 3} {
		created, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Card"})
		if err != nil {
			t.Fatalf("create card %d: %v", i, err)
		}
		if created.TicketNo != want {
			t.Fatalf("card %d TicketNo = %d, want %d", i, created.TicketNo, want)
		}
	}
}

// TestTicketNumbersAreNeverReused guards the exact reason a stored counter
// (boards.next_ticket_no) was used instead of MAX(ticket_no)+1: deleting the
// highest-numbered card must not let the next card reuse that number — a
// ticket number is a permanent reference, not just a display position.
func TestTicketNumbersAreNeverReused(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)

	first, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "First"})
	if err != nil {
		t.Fatalf("create first card: %v", err)
	}
	second, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Second"})
	if err != nil {
		t.Fatalf("create second card: %v", err)
	}
	if first.TicketNo != 1 || second.TicketNo != 2 {
		t.Fatalf("initial ticket numbers = %d, %d, want 1, 2", first.TicketNo, second.TicketNo)
	}

	if err := db.DeleteCard(second.ID); err != nil {
		t.Fatalf("delete second card: %v", err)
	}

	third, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Third"})
	if err != nil {
		t.Fatalf("create third card: %v", err)
	}
	if third.TicketNo != 3 {
		t.Fatalf("third card TicketNo = %d, want 3 (not reusing deleted card 2's number)", third.TicketNo)
	}
}

// TestTicketNumbersAreIndependentPerBoard confirms two boards each start
// their own sequence at 1 rather than sharing one global counter.
func TestTicketNumbersAreIndependentPerBoard(t *testing.T) {
	db := openTestDB(t)
	laneA := newTestLane(t, db) // its own new board, via newTestLane

	boardB, err := db.CreateBoard("Board B", 1)
	if err != nil {
		t.Fatalf("create board B: %v", err)
	}
	laneB, err := db.CreateLane(boardB.ID, "Lane B", 0)
	if err != nil {
		t.Fatalf("create lane B: %v", err)
	}

	cardA, err := db.CreateCard(models.Card{LaneID: laneA.ID, Title: "A1"})
	if err != nil {
		t.Fatalf("create card on board A: %v", err)
	}
	cardB, err := db.CreateCard(models.Card{LaneID: laneB.ID, Title: "B1"})
	if err != nil {
		t.Fatalf("create card on board B: %v", err)
	}
	if cardA.TicketNo != 1 || cardB.TicketNo != 1 {
		t.Fatalf("first card on each board should both be ticket 1, got board A=%d board B=%d", cardA.TicketNo, cardB.TicketNo)
	}
}
