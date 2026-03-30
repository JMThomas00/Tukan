package database

import (
	"fmt"
	"time"

	"github.com/JMThomas00/tukan/internal/models"
)

const cardColumns = `id, lane_id, title, assignee, COALESCE(note,''), position, created_at, updated_at`

func scanCard(scan func(...any) error) (models.Card, error) {
	var c models.Card
	var createdStr, updatedStr string
	err := scan(&c.ID, &c.LaneID, &c.Title, &c.Assignee, &c.Note, &c.Position, &createdStr, &updatedStr)
	if err != nil {
		return c, err
	}
	c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
	c.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr)
	return c, nil
}

// ListCardsByLane returns all cards in a lane ordered by position.
func (d *DB) ListCardsByLane(laneID int64) ([]models.Card, error) {
	rows, err := d.sql.Query(
		`SELECT `+cardColumns+` FROM cards WHERE lane_id=? ORDER BY position`,
		laneID,
	)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		c, err := scanCard(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan card: %w", err)
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// ListAllCards returns every card, useful for bulk loading on startup.
func (d *DB) ListAllCards() ([]models.Card, error) {
	rows, err := d.sql.Query(
		`SELECT ` + cardColumns + ` FROM cards ORDER BY lane_id, position`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all cards: %w", err)
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		c, err := scanCard(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan card: %w", err)
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// CreateCard inserts a new card at the end of its lane.
func (d *DB) CreateCard(c models.Card) (models.Card, error) {
	// Assign next position within the lane.
	var maxPos int
	_ = d.sql.QueryRow(
		`SELECT COALESCE(MAX(position),-1) FROM cards WHERE lane_id=?`, c.LaneID,
	).Scan(&maxPos)
	c.Position = maxPos + 1

	res, err := d.sql.Exec(
		`INSERT INTO cards (lane_id, title, assignee, note, position) VALUES (?,?,?,?,?)`,
		c.LaneID, c.Title, c.Assignee, c.Note, c.Position,
	)
	if err != nil {
		return c, fmt.Errorf("create card: %w", err)
	}
	c.ID, _ = res.LastInsertId()
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	return c, nil
}

// UpdateCard updates the mutable fields of a card (title, assignee, note).
func (d *DB) UpdateCard(c models.Card) error {
	_, err := d.sql.Exec(
		`UPDATE cards SET title=?, assignee=?, note=? WHERE id=?`,
		c.Title, c.Assignee, c.Note, c.ID,
	)
	return err
}

// DeleteCard removes a card and reorders siblings to fill the gap.
func (d *DB) DeleteCard(id int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get lane and position of the card being deleted.
	var laneID int64
	var pos int
	if err := tx.QueryRow(`SELECT lane_id, position FROM cards WHERE id=?`, id).Scan(&laneID, &pos); err != nil {
		return fmt.Errorf("delete card lookup: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM cards WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete card: %w", err)
	}

	// Shift down positions of cards that came after the deleted one.
	if _, err := tx.Exec(
		`UPDATE cards SET position=position-1 WHERE lane_id=? AND position>?`,
		laneID, pos,
	); err != nil {
		return fmt.Errorf("reorder after delete: %w", err)
	}

	return tx.Commit()
}

// MoveCard moves a card to a different lane and appends it at the end.
// It also reorders the source lane to fill the gap.
func (d *DB) MoveCard(id int64, toLaneID int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var fromLaneID int64
	var fromPos int
	if err := tx.QueryRow(`SELECT lane_id, position FROM cards WHERE id=?`, id).Scan(&fromLaneID, &fromPos); err != nil {
		return fmt.Errorf("move card lookup: %w", err)
	}

	// Get the next position in the destination lane.
	var maxPos int
	_ = tx.QueryRow(
		`SELECT COALESCE(MAX(position),-1) FROM cards WHERE lane_id=?`, toLaneID,
	).Scan(&maxPos)
	newPos := maxPos + 1

	if _, err := tx.Exec(
		`UPDATE cards SET lane_id=?, position=? WHERE id=?`,
		toLaneID, newPos, id,
	); err != nil {
		return fmt.Errorf("move card update: %w", err)
	}

	// Close the gap in the source lane.
	if _, err := tx.Exec(
		`UPDATE cards SET position=position-1 WHERE lane_id=? AND position>?`,
		fromLaneID, fromPos,
	); err != nil {
		return fmt.Errorf("move card reorder source: %w", err)
	}

	return tx.Commit()
}
