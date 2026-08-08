package database

import (
	"testing"

	"github.com/JMThomas00/tukan/internal/models"
)

func TestChecklistItemCRUD(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	a, err := db.CreateChecklistItem(card.ID, "Write tests")
	if err != nil {
		t.Fatalf("create item a: %v", err)
	}
	b, err := db.CreateChecklistItem(card.ID, "Update docs")
	if err != nil {
		t.Fatalf("create item b: %v", err)
	}
	if a.Position != 0 || b.Position != 1 {
		t.Fatalf("positions = %d, %d, want 0, 1", a.Position, b.Position)
	}
	if a.Done || b.Done {
		t.Fatal("new items should start undone")
	}

	items, err := db.ListChecklistItems(card.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	if err := db.ToggleChecklistItem(a.ID); err != nil {
		t.Fatalf("toggle item a: %v", err)
	}
	items, err = db.ListChecklistItems(card.ID)
	if err != nil {
		t.Fatalf("list items after toggle: %v", err)
	}
	if !items[0].Done {
		t.Fatal("item a should be done after toggle")
	}
	if err := db.ToggleChecklistItem(a.ID); err != nil {
		t.Fatalf("toggle item a back: %v", err)
	}
	items, err = db.ListChecklistItems(card.ID)
	if err != nil {
		t.Fatalf("list items after second toggle: %v", err)
	}
	if items[0].Done {
		t.Fatal("item a should be undone after toggling twice")
	}

	if err := db.UpdateChecklistItemText(b.ID, "Update the README"); err != nil {
		t.Fatalf("update item text: %v", err)
	}
	items, err = db.ListChecklistItems(card.ID)
	if err != nil {
		t.Fatalf("list items after rename: %v", err)
	}
	if items[1].Text != "Update the README" {
		t.Fatalf("item b text = %q, want %q", items[1].Text, "Update the README")
	}

	// Deleting the first item should close the position gap for the second.
	if err := db.DeleteChecklistItem(a.ID); err != nil {
		t.Fatalf("delete item a: %v", err)
	}
	items, err = db.ListChecklistItems(card.ID)
	if err != nil {
		t.Fatalf("list items after delete: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d after delete, want 1", len(items))
	}
	if items[0].ID != b.ID || items[0].Position != 0 {
		t.Fatalf("remaining item = %+v, want item b at position 0", items[0])
	}
}

// TestChecklistItemsAreCardScopedAndCascade confirms bulk board-level
// loading only returns items for cards on that board, and that deleting a
// card removes its checklist items too (cascade via cards.id FK).
func TestChecklistItemsAreCardScopedAndCascade(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)

	cardA, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Card A"})
	if err != nil {
		t.Fatalf("create card a: %v", err)
	}
	cardB, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Card B"})
	if err != nil {
		t.Fatalf("create card b: %v", err)
	}

	if _, err := db.CreateChecklistItem(cardA.ID, "A1"); err != nil {
		t.Fatalf("create item on card a: %v", err)
	}
	if _, err := db.CreateChecklistItem(cardB.ID, "B1"); err != nil {
		t.Fatalf("create item on card b: %v", err)
	}

	byBoard, err := db.ListChecklistItemsForBoard(lane.BoardID)
	if err != nil {
		t.Fatalf("list checklist items for board: %v", err)
	}
	if len(byBoard[cardA.ID]) != 1 || byBoard[cardA.ID][0].Text != "A1" {
		t.Fatalf("card A items = %+v, want just 'A1'", byBoard[cardA.ID])
	}
	if len(byBoard[cardB.ID]) != 1 || byBoard[cardB.ID][0].Text != "B1" {
		t.Fatalf("card B items = %+v, want just 'B1'", byBoard[cardB.ID])
	}

	if err := db.DeleteCard(cardA.ID); err != nil {
		t.Fatalf("delete card a: %v", err)
	}
	byBoard, err = db.ListChecklistItemsForBoard(lane.BoardID)
	if err != nil {
		t.Fatalf("list checklist items for board after card delete: %v", err)
	}
	if len(byBoard[cardA.ID]) != 0 {
		t.Fatalf("card A items survived card deletion: %+v", byBoard[cardA.ID])
	}
	if len(byBoard[cardB.ID]) != 1 {
		t.Fatalf("card B items changed after deleting a different card: %+v", byBoard[cardB.ID])
	}
}
