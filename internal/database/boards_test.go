package database

import (
	"testing"

	"github.com/JMThomas00/tukan/internal/models"
)

func TestBoardCRUD(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// A fresh database already has the seeded "Main Board".
	boards, err := db.ListBoards()
	if err != nil {
		t.Fatalf("list boards: %v", err)
	}
	if len(boards) != 1 || boards[0].Name != "Main Board" {
		t.Fatalf("boards = %+v, want a single seeded Main Board", boards)
	}

	second, err := db.CreateBoard("Work", 1)
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	if second.ID == 0 || second.Name != "Work" {
		t.Fatalf("created board = %+v", second)
	}

	boards, err = db.ListBoards()
	if err != nil {
		t.Fatalf("list boards after create: %v", err)
	}
	if len(boards) != 2 {
		t.Fatalf("len(boards) = %d, want 2", len(boards))
	}

	second.Name = "Work Projects"
	if err := db.UpdateBoard(second); err != nil {
		t.Fatalf("update board: %v", err)
	}
	boards, err = db.ListBoards()
	if err != nil {
		t.Fatalf("list boards after rename: %v", err)
	}
	if boards[1].Name != "Work Projects" {
		t.Fatalf("boards[1].Name = %q, want %q", boards[1].Name, "Work Projects")
	}

	if err := db.DeleteBoard(second.ID); err != nil {
		t.Fatalf("delete board: %v", err)
	}
	boards, err = db.ListBoards()
	if err != nil {
		t.Fatalf("list boards after delete: %v", err)
	}
	if len(boards) != 1 {
		t.Fatalf("len(boards) = %d after delete, want 1", len(boards))
	}
}

// TestBoardsAreIsolated confirms lanes/cards on one board never leak into
// another board's queries, and deleting a board cascades to its own lanes
// and cards without touching a sibling board.
func TestBoardsAreIsolated(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	boards, err := db.ListBoards()
	if err != nil || len(boards) != 1 {
		t.Fatalf("list boards: %v (boards=%+v)", err, boards)
	}
	mainBoard := boards[0]

	workBoard, err := db.CreateBoard("Work", 1)
	if err != nil {
		t.Fatalf("create work board: %v", err)
	}

	if err := db.SeedDefaultLanes(mainBoard.ID); err != nil {
		t.Fatalf("seed main board lanes: %v", err)
	}
	if err := db.SeedDefaultLanes(workBoard.ID); err != nil {
		t.Fatalf("seed work board lanes: %v", err)
	}

	mainLanes, err := db.ListLanesByBoard(mainBoard.ID)
	if err != nil {
		t.Fatalf("list main lanes: %v", err)
	}
	workLanes, err := db.ListLanesByBoard(workBoard.ID)
	if err != nil {
		t.Fatalf("list work lanes: %v", err)
	}
	if len(mainLanes) != 4 || len(workLanes) != 4 {
		t.Fatalf("expected 4 lanes each, got main=%d work=%d", len(mainLanes), len(workLanes))
	}

	if _, err := db.CreateCard(models.Card{LaneID: mainLanes[0].ID, Title: "Main card"}); err != nil {
		t.Fatalf("create main card: %v", err)
	}
	if _, err := db.CreateCard(models.Card{LaneID: workLanes[0].ID, Title: "Work card"}); err != nil {
		t.Fatalf("create work card: %v", err)
	}

	mainCards, err := db.ListCardsByBoard(mainBoard.ID)
	if err != nil {
		t.Fatalf("list main cards: %v", err)
	}
	workCards, err := db.ListCardsByBoard(workBoard.ID)
	if err != nil {
		t.Fatalf("list work cards: %v", err)
	}
	if len(mainCards) != 1 || mainCards[0].Title != "Main card" {
		t.Fatalf("main board cards = %+v, want just 'Main card'", mainCards)
	}
	if len(workCards) != 1 || workCards[0].Title != "Work card" {
		t.Fatalf("work board cards = %+v, want just 'Work card'", workCards)
	}

	// Deleting the work board must cascade to its lanes and cards only.
	if err := db.DeleteBoard(workBoard.ID); err != nil {
		t.Fatalf("delete work board: %v", err)
	}
	workLanesAfter, err := db.ListLanesByBoard(workBoard.ID)
	if err != nil {
		t.Fatalf("list work lanes after delete: %v", err)
	}
	if len(workLanesAfter) != 0 {
		t.Fatalf("work board lanes survived board deletion: %+v", workLanesAfter)
	}
	mainCardsAfter, err := db.ListCardsByBoard(mainBoard.ID)
	if err != nil {
		t.Fatalf("list main cards after work board delete: %v", err)
	}
	if len(mainCardsAfter) != 1 || mainCardsAfter[0].Title != "Main card" {
		t.Fatalf("main board cards changed after deleting a different board: %+v", mainCardsAfter)
	}
}
