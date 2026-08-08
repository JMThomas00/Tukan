package database

import (
	"database/sql"
	"fmt"

	"github.com/JMThomas00/tukan/internal/models"
)

// ListLanesByBoard returns all lanes on a board, ordered by position.
func (d *DB) ListLanesByBoard(boardID int64) ([]models.Lane, error) {
	rows, err := d.sql.Query(
		`SELECT id, board_id, name, position, COALESCE(color,'') FROM lanes WHERE board_id=? ORDER BY position`,
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list lanes: %w", err)
	}
	defer rows.Close()

	var lanes []models.Lane
	for rows.Next() {
		var l models.Lane
		if err := rows.Scan(&l.ID, &l.BoardID, &l.Name, &l.Position, &l.Color); err != nil {
			return nil, fmt.Errorf("scan lane: %w", err)
		}
		lanes = append(lanes, l)
	}
	return lanes, rows.Err()
}

// CreateLane inserts a new lane on a board and returns it with its assigned ID.
func (d *DB) CreateLane(boardID int64, name string, position int) (models.Lane, error) {
	res, err := d.sql.Exec(
		`INSERT INTO lanes (board_id, name, position) VALUES (?, ?, ?)`,
		boardID, name, position,
	)
	if err != nil {
		return models.Lane{}, fmt.Errorf("create lane: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.Lane{ID: id, BoardID: boardID, Name: name, Position: position}, nil
}

// UpdateLanePosition updates the position of a lane.
func (d *DB) UpdateLanePosition(id int64, newPos int) error {
	_, err := d.sql.Exec(`UPDATE lanes SET position=? WHERE id=?`, newPos, id)
	return err
}

// UpdateLane renames a lane (and/or changes its color).
func (d *DB) UpdateLane(l models.Lane) error {
	_, err := d.sql.Exec(`UPDATE lanes SET name=?, color=? WHERE id=?`, l.Name, l.Color, l.ID)
	return err
}

// SwapLanePositions exchanges two lanes' positions atomically — the
// mechanism behind reordering a lane left/right in the lane manager.
// Wrapped in a transaction so a failure partway through can't leave one
// lane moved and the other not (two separate non-transactional updates
// risk exactly that on a mid-write error).
func (d *DB) SwapLanePositions(id1, id2 int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var pos1, pos2 int
	if err := tx.QueryRow(`SELECT position FROM lanes WHERE id=?`, id1).Scan(&pos1); err != nil {
		return fmt.Errorf("look up lane %d position: %w", id1, err)
	}
	if err := tx.QueryRow(`SELECT position FROM lanes WHERE id=?`, id2).Scan(&pos2); err != nil {
		return fmt.Errorf("look up lane %d position: %w", id2, err)
	}
	if _, err := tx.Exec(`UPDATE lanes SET position=? WHERE id=?`, pos2, id1); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE lanes SET position=? WHERE id=?`, pos1, id2); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteLane deletes a lane and all its cards (CASCADE).
func (d *DB) DeleteLane(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM lanes WHERE id=?`, id)
	return err
}

// SeedDefaultLanes inserts the four default lanes into a board if it has none.
func (d *DB) SeedDefaultLanes(boardID int64) error {
	var count int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM lanes WHERE board_id=?`, boardID).Scan(&count); err != nil {
		return fmt.Errorf("seed check: %w", err)
	}
	if count > 0 {
		return nil
	}

	defaults := []struct {
		name  string
		color string
	}{
		{"To-Do", "#7aa2f7"},
		{"In Progress", "#e0af68"},
		{"On Hold", "#f7768e"},
		{"Done", "#9ece6a"},
	}

	for i, d2 := range defaults {
		if _, err := d.sql.Exec(
			`INSERT INTO lanes (board_id, name, position, color) VALUES (?, ?, ?, ?)`,
			boardID, d2.name, i, d2.color,
		); err != nil {
			return fmt.Errorf("seed lane %q: %w", d2.name, err)
		}
	}
	return nil
}

// laneExists is a helper used internally.
func (d *DB) laneExists(id int64) (bool, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM lanes WHERE id=?`, id).Scan(&n)
	return n > 0, err
}

// scanLane is a helper for single-row lane queries.
func scanLane(row *sql.Row) (models.Lane, error) {
	var l models.Lane
	err := row.Scan(&l.ID, &l.Name, &l.Position, &l.Color)
	return l, err
}
