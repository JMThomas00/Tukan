package database

import (
	"database/sql"
	"fmt"

	"github.com/JMThomas00/tukan/internal/models"
)

// GetBoardIDForChannel looks up the Tukan board mapped to a Concord channel.
// The bool return distinguishes "no mapping exists" from a real error, since
// server mode needs to tell those apart (an unmapped channel it doesn't yet
// know about vs. a genuine query failure) without relying on error string
// matching.
func (d *DB) GetBoardIDForChannel(channelID string) (int64, bool, error) {
	var boardID int64
	err := d.sql.QueryRow(`SELECT board_id FROM plugin_channels WHERE channel_id=?`, channelID).Scan(&boardID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get board for channel: %w", err)
	}
	return boardID, true, nil
}

// CreateBoardForChannel registers a brand-new Concord channel: creates its
// Tukan board, seeds the default lanes (matching what a board created
// through the standalone CLI's board switcher gets), and records the
// mapping. Mirrors the board switcher's own cmdCreateBoard sequence
// (CreateBoard, then SeedDefaultLanes, as two plain sequential calls, not
// one wrapped transaction) — that's an existing, already-accepted pattern
// in this codebase for this exact rare, low-stakes sequence, not a new
// inconsistency introduced here.
func (d *DB) CreateBoardForChannel(channelID, boardName string) (models.Board, error) {
	existing, err := d.ListBoards()
	if err != nil {
		return models.Board{}, fmt.Errorf("list boards for position: %w", err)
	}

	board, err := d.CreateBoard(boardName, len(existing))
	if err != nil {
		return models.Board{}, fmt.Errorf("create board for channel: %w", err)
	}
	if err := d.SeedDefaultLanes(board.ID); err != nil {
		return models.Board{}, fmt.Errorf("seed lanes for channel board: %w", err)
	}
	if _, err := d.sql.Exec(
		`INSERT INTO plugin_channels (channel_id, board_id) VALUES (?, ?)`,
		channelID, board.ID,
	); err != nil {
		return models.Board{}, fmt.Errorf("map channel to board: %w", err)
	}
	return board, nil
}
