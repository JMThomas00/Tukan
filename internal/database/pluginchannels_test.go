package database

import "testing"

func TestGetBoardIDForChannelUnmapped(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, ok, err := db.GetBoardIDForChannel("nonexistent-channel")
	if err != nil {
		t.Fatalf("get board for unmapped channel: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for a channel with no mapping")
	}
}

// TestCreateBoardForChannelRoundTrip confirms the full sequence: a new
// board is created, seeded with the standard default lanes, and mapped to
// the channel so a later lookup resolves it.
func TestCreateBoardForChannelRoundTrip(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	board, err := db.CreateBoardForChannel("channel-abc", "Team Kanban")
	if err != nil {
		t.Fatalf("create board for channel: %v", err)
	}
	if board.ID == 0 || board.Name != "Team Kanban" {
		t.Fatalf("created board = %+v", board)
	}

	lanes, err := db.ListLanesByBoard(board.ID)
	if err != nil {
		t.Fatalf("list lanes: %v", err)
	}
	if len(lanes) != 4 {
		t.Fatalf("len(lanes) = %d, want 4 (the standard default lanes)", len(lanes))
	}

	boardID, ok, err := db.GetBoardIDForChannel("channel-abc")
	if err != nil {
		t.Fatalf("get board for channel: %v", err)
	}
	if !ok || boardID != board.ID {
		t.Fatalf("GetBoardIDForChannel = (%d, %v), want (%d, true)", boardID, ok, board.ID)
	}
}

// TestCreateBoardForChannelPicksNextPosition confirms a channel-created
// board doesn't collide with an existing board's position — matters since
// boards are ordered by position in the switcher/header.
func TestCreateBoardForChannelPicksNextPosition(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// A fresh database already seeds one board ("Main Board", position 0).

	board, err := db.CreateBoardForChannel("channel-xyz", "Second Board")
	if err != nil {
		t.Fatalf("create board for channel: %v", err)
	}
	if board.Position != 1 {
		t.Fatalf("board.Position = %d, want 1 (after the seeded Main Board at position 0)", board.Position)
	}
}

func TestGetBoardIDForChannelIsolatesUnrelatedChannels(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	boardA, err := db.CreateBoardForChannel("channel-a", "Board A")
	if err != nil {
		t.Fatalf("create board A: %v", err)
	}
	boardB, err := db.CreateBoardForChannel("channel-b", "Board B")
	if err != nil {
		t.Fatalf("create board B: %v", err)
	}

	idA, _, err := db.GetBoardIDForChannel("channel-a")
	if err != nil {
		t.Fatalf("get board A: %v", err)
	}
	idB, _, err := db.GetBoardIDForChannel("channel-b")
	if err != nil {
		t.Fatalf("get board B: %v", err)
	}
	if idA != boardA.ID || idB != boardB.ID || idA == idB {
		t.Fatalf("channel-a -> %d, channel-b -> %d, want %d and %d respectively", idA, idB, boardA.ID, boardB.ID)
	}
}
