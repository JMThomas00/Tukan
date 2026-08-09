package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/JMThomas00/tukan/internal/models"
)

const cardColumns = `id, lane_id, title, COALESCE(note,''), position, created_at, updated_at, due_date, ticket_no, start_date`

// dueDateLayout is the storage format for cards.due_date (date-only, no
// time-of-day), matching the format used by internal/ui's card form.
const dueDateLayout = "2006-01-02"

// dateParam converts a *time.Time into a driver value: nil clears the
// column, a formatted string sets it. Shared by due_date and start_date —
// both are date-only (no time-of-day) columns in the same storage format.
func dateParam(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(dueDateLayout)
}

func scanCard(scan func(...any) error) (models.Card, error) {
	var c models.Card
	var createdStr, updatedStr string
	var dueStr, startStr sql.NullString
	var ticketNo sql.NullInt64
	err := scan(&c.ID, &c.LaneID, &c.Title, &c.Note, &c.Position, &createdStr, &updatedStr, &dueStr, &ticketNo, &startStr)
	if err != nil {
		return c, err
	}
	if ticketNo.Valid {
		c.TicketNo = int(ticketNo.Int64)
	}
	c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
	c.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr)
	if dueStr.Valid {
		if t, err := time.Parse(dueDateLayout, dueStr.String); err == nil {
			c.DueDate = &t
		}
	}
	if startStr.Valid {
		if t, err := time.Parse(dueDateLayout, startStr.String); err == nil {
			c.StartDate = &t
		}
	}
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

// ListCardsByBoard returns every card on a board (joined through its lanes),
// useful for bulk loading a board's full state at once.
func (d *DB) ListCardsByBoard(boardID int64) ([]models.Card, error) {
	cols := `c.id, c.lane_id, c.title, COALESCE(c.note,''), c.position, c.created_at, c.updated_at, c.due_date, c.ticket_no, c.start_date`
	rows, err := d.sql.Query(
		`SELECT `+cols+` FROM cards c
		 JOIN lanes l ON l.id = c.lane_id
		 WHERE l.board_id = ?
		 ORDER BY c.lane_id, c.position`,
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list cards by board: %w", err)
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

// CreateCard inserts a new card at the end of its lane. Wrapped in a
// transaction (unlike the original implementation's non-transactional
// MAX(position) lookup — a latent race two concurrent creates in the same
// lane could hit) since it needs one anyway to log the "created" activity
// atomically with the insert.
func (d *DB) CreateCard(c models.Card) (models.Card, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return c, err
	}
	defer tx.Rollback()

	var maxPos int
	_ = tx.QueryRow(
		`SELECT COALESCE(MAX(position),-1) FROM cards WHERE lane_id=?`, c.LaneID,
	).Scan(&maxPos)
	c.Position = maxPos + 1

	// Ticket numbers are a stored per-board counter (boards.next_ticket_no),
	// not MAX(ticket_no)+1 — deleting the highest-numbered card would let a
	// MAX-based scan hand that number out again, which a permanent ticket
	// reference must never do.
	var boardID int64
	if err := tx.QueryRow(`SELECT board_id FROM lanes WHERE id=?`, c.LaneID).Scan(&boardID); err != nil {
		return c, fmt.Errorf("look up board for ticket number: %w", err)
	}
	if err := tx.QueryRow(`SELECT next_ticket_no FROM boards WHERE id=?`, boardID).Scan(&c.TicketNo); err != nil {
		return c, fmt.Errorf("get next ticket number: %w", err)
	}
	if _, err := tx.Exec(`UPDATE boards SET next_ticket_no=next_ticket_no+1 WHERE id=?`, boardID); err != nil {
		return c, fmt.Errorf("increment ticket number: %w", err)
	}

	res, err := tx.Exec(
		`INSERT INTO cards (lane_id, title, note, position, due_date, ticket_no, start_date) VALUES (?,?,?,?,?,?,?)`,
		c.LaneID, c.Title, c.Note, c.Position, dateParam(c.DueDate), c.TicketNo, dateParam(c.StartDate),
	)
	if err != nil {
		return c, fmt.Errorf("create card: %w", err)
	}
	c.ID, _ = res.LastInsertId()

	if err := logActivity(tx, c.ID, "created"); err != nil {
		return c, fmt.Errorf("log activity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return c, err
	}

	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	return c, nil
}

// UpdateCard updates the mutable fields of a card (title, note, start/due
// date), diffing old against new to log one combined activity entry (e.g.
// "updated: title, due date") rather than one row per field.
func (d *DB) UpdateCard(old, new models.Card) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE cards SET title=?, note=?, due_date=?, start_date=? WHERE id=?`,
		new.Title, new.Note, dateParam(new.DueDate), dateParam(new.StartDate), new.ID,
	); err != nil {
		return err
	}

	var changed []string
	if old.Title != new.Title {
		changed = append(changed, "title")
	}
	if old.Note != new.Note {
		changed = append(changed, "note")
	}
	if !datesEqual(old.StartDate, new.StartDate) {
		changed = append(changed, "start date")
	}
	if !datesEqual(old.DueDate, new.DueDate) {
		changed = append(changed, "due date")
	}
	if len(changed) > 0 {
		if err := logActivity(tx, new.ID, "updated: "+strings.Join(changed, ", ")); err != nil {
			return fmt.Errorf("log activity: %w", err)
		}
	}

	return tx.Commit()
}

func datesEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
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

	var fromName, toName string
	if err := tx.QueryRow(`SELECT name FROM lanes WHERE id=?`, fromLaneID).Scan(&fromName); err != nil {
		return fmt.Errorf("move card lookup from lane: %w", err)
	}
	if err := tx.QueryRow(`SELECT name FROM lanes WHERE id=?`, toLaneID).Scan(&toName); err != nil {
		return fmt.Errorf("move card lookup to lane: %w", err)
	}
	if err := logActivity(tx, id, fmt.Sprintf("moved from %q to %q", fromName, toName)); err != nil {
		return fmt.Errorf("log activity: %w", err)
	}

	return tx.Commit()
}
