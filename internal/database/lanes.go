package database

import (
	"database/sql"
	"fmt"

	"github.com/JMThomas00/tukan/internal/models"
)

// ListLanes returns all lanes ordered by position.
func (d *DB) ListLanes() ([]models.Lane, error) {
	rows, err := d.sql.Query(
		`SELECT id, name, position, COALESCE(color,'') FROM lanes ORDER BY position`,
	)
	if err != nil {
		return nil, fmt.Errorf("list lanes: %w", err)
	}
	defer rows.Close()

	var lanes []models.Lane
	for rows.Next() {
		var l models.Lane
		if err := rows.Scan(&l.ID, &l.Name, &l.Position, &l.Color); err != nil {
			return nil, fmt.Errorf("scan lane: %w", err)
		}
		lanes = append(lanes, l)
	}
	return lanes, rows.Err()
}

// CreateLane inserts a new lane and returns it with its assigned ID.
func (d *DB) CreateLane(name string, position int) (models.Lane, error) {
	res, err := d.sql.Exec(
		`INSERT INTO lanes (name, position) VALUES (?, ?)`,
		name, position,
	)
	if err != nil {
		return models.Lane{}, fmt.Errorf("create lane: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.Lane{ID: id, Name: name, Position: position}, nil
}

// UpdateLanePosition updates the position of a lane.
func (d *DB) UpdateLanePosition(id int64, newPos int) error {
	_, err := d.sql.Exec(`UPDATE lanes SET position=? WHERE id=?`, newPos, id)
	return err
}

// DeleteLane deletes a lane and all its cards (CASCADE).
func (d *DB) DeleteLane(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM lanes WHERE id=?`, id)
	return err
}

// SeedDefaultLanes inserts the four default lanes if the table is empty.
func (d *DB) SeedDefaultLanes() error {
	var count int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM lanes`).Scan(&count); err != nil {
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
			`INSERT INTO lanes (name, position, color) VALUES (?, ?, ?)`,
			d2.name, i, d2.color,
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
