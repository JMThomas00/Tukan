package database

import (
	"testing"

	"github.com/JMThomas00/tukan/internal/models"
)

func TestLabelCRUDAndAssignment(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)

	bug, err := db.CreateLabel(lane.BoardID, "Bug", "#f7768e")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	ui, err := db.CreateLabel(lane.BoardID, "UI", "#7aa2f7")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}

	labels, err := db.ListLabelsByBoard(lane.BoardID)
	if err != nil {
		t.Fatalf("list labels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("len(labels) = %d, want 2", len(labels))
	}

	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Fix login"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	if err := db.SetCardLabels(card.ID, []int64{bug.ID, ui.ID}); err != nil {
		t.Fatalf("set card labels: %v", err)
	}

	byBoard, err := db.ListLabelsForBoard(lane.BoardID)
	if err != nil {
		t.Fatalf("list labels for board: %v", err)
	}
	if len(byBoard[card.ID]) != 2 {
		t.Fatalf("card labels = %+v, want 2", byBoard[card.ID])
	}

	// Replace the assignment with just one label.
	if err := db.SetCardLabels(card.ID, []int64{bug.ID}); err != nil {
		t.Fatalf("set card labels (replace): %v", err)
	}
	byBoard, err = db.ListLabelsForBoard(lane.BoardID)
	if err != nil {
		t.Fatalf("list labels for board after replace: %v", err)
	}
	if len(byBoard[card.ID]) != 1 || byBoard[card.ID][0].ID != bug.ID {
		t.Fatalf("card labels after replace = %+v, want just Bug", byBoard[card.ID])
	}

	// Deleting a label removes it from the palette and from any card it was on.
	if err := db.DeleteLabel(bug.ID); err != nil {
		t.Fatalf("delete label: %v", err)
	}
	labels, err = db.ListLabelsByBoard(lane.BoardID)
	if err != nil {
		t.Fatalf("list labels after delete: %v", err)
	}
	if len(labels) != 1 || labels[0].ID != ui.ID {
		t.Fatalf("labels after delete = %+v, want just UI", labels)
	}
	byBoard, err = db.ListLabelsForBoard(lane.BoardID)
	if err != nil {
		t.Fatalf("list labels for board after label delete: %v", err)
	}
	if len(byBoard[card.ID]) != 0 {
		t.Fatalf("card labels after deleting its only label = %+v, want none", byBoard[card.ID])
	}
}

// TestLabelsAreBoardScoped confirms a label created on one board never
// shows up in another board's palette or bulk card-label query.
func TestLabelsAreBoardScoped(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	boards, err := db.ListBoards()
	if err != nil || len(boards) != 1 {
		t.Fatalf("list boards: %v (%+v)", err, boards)
	}
	mainBoard := boards[0]
	workBoard, err := db.CreateBoard("Work", 1)
	if err != nil {
		t.Fatalf("create work board: %v", err)
	}

	if _, err := db.CreateLabel(mainBoard.ID, "Personal Label", "#9ece6a"); err != nil {
		t.Fatalf("create main label: %v", err)
	}
	if _, err := db.CreateLabel(workBoard.ID, "Work Label", "#e0af68"); err != nil {
		t.Fatalf("create work label: %v", err)
	}

	mainLabels, err := db.ListLabelsByBoard(mainBoard.ID)
	if err != nil {
		t.Fatalf("list main labels: %v", err)
	}
	workLabels, err := db.ListLabelsByBoard(workBoard.ID)
	if err != nil {
		t.Fatalf("list work labels: %v", err)
	}
	if len(mainLabels) != 1 || mainLabels[0].Name != "Personal Label" {
		t.Fatalf("main board labels = %+v, want just 'Personal Label'", mainLabels)
	}
	if len(workLabels) != 1 || workLabels[0].Name != "Work Label" {
		t.Fatalf("work board labels = %+v, want just 'Work Label'", workLabels)
	}
}
