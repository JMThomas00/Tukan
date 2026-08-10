package database

import (
	"fmt"
	"time"

	"github.com/JMThomas00/tukan/internal/models"
)

const boardColumns = `id, name, position, COALESCE(color,''), created_at, updated_at`

func scanBoard(scan func(...any) error) (models.Board, error) {
	var b models.Board
	var createdStr, updatedStr string
	err := scan(&b.ID, &b.Name, &b.Position, &b.Color, &createdStr, &updatedStr)
	if err != nil {
		return b, err
	}
	b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
	b.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr)
	return b, nil
}

// ListBoards returns all boards ordered by position.
func (d *DB) ListBoards() ([]models.Board, error) {
	rows, err := d.sql.Query(`SELECT ` + boardColumns + ` FROM boards ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("list boards: %w", err)
	}
	defer rows.Close()

	var boards []models.Board
	for rows.Next() {
		b, err := scanBoard(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan board: %w", err)
		}
		boards = append(boards, b)
	}
	return boards, rows.Err()
}

// GetBoardByID returns a single board, for callers that already know which
// board they want (e.g. server mode resolving a Concord channel to its
// mapped board) rather than needing the full ListBoards result just to
// filter it down to one row.
func (d *DB) GetBoardByID(id int64) (models.Board, error) {
	row := d.sql.QueryRow(`SELECT `+boardColumns+` FROM boards WHERE id=?`, id)
	b, err := scanBoard(row.Scan)
	if err != nil {
		return models.Board{}, fmt.Errorf("get board: %w", err)
	}
	return b, nil
}

// CreateBoard inserts a new board and returns it with its assigned ID.
func (d *DB) CreateBoard(name string, position int) (models.Board, error) {
	res, err := d.sql.Exec(
		`INSERT INTO boards (name, position) VALUES (?, ?)`,
		name, position,
	)
	if err != nil {
		return models.Board{}, fmt.Errorf("create board: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.Board{ID: id, Name: name, Position: position}, nil
}

// UpdateBoard renames a board (and/or changes its color).
func (d *DB) UpdateBoard(b models.Board) error {
	_, err := d.sql.Exec(`UPDATE boards SET name=?, color=? WHERE id=?`, b.Name, b.Color, b.ID)
	return err
}

// DeleteBoard deletes a board and everything in it (lanes → cards, CASCADE).
func (d *DB) DeleteBoard(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM boards WHERE id=?`, id)
	return err
}
