package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JMThomas00/tukan/internal/models"
)

const cardEventColumns = `id, card_id, kind, actor, body, created_at`

func scanCardEvent(scan func(...any) error) (models.CardEvent, error) {
	var e models.CardEvent
	var kind string
	var actor sql.NullString
	var createdStr string
	err := scan(&e.ID, &e.CardID, &kind, &actor, &e.Body, &createdStr)
	if err != nil {
		return e, err
	}
	e.Kind = models.CardEventKind(kind)
	if actor.Valid {
		a := actor.String
		e.Actor = &a
	}
	e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
	return e, nil
}

// ListCardEvents returns a card's combined comment + activity thread,
// oldest first.
func (d *DB) ListCardEvents(cardID int64) ([]models.CardEvent, error) {
	rows, err := d.sql.Query(
		`SELECT `+cardEventColumns+` FROM card_events WHERE card_id=? ORDER BY created_at, id`,
		cardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list card events: %w", err)
	}
	defer rows.Close()

	var events []models.CardEvent
	for rows.Next() {
		e, err := scanCardEvent(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan card event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// AddComment appends a user-authored comment to a card's thread. actor is
// nullable free text — there's no user/auth concept yet.
func (d *DB) AddComment(cardID int64, actor *string, body string) (models.CardEvent, error) {
	res, err := d.sql.Exec(
		`INSERT INTO card_events (card_id, kind, actor, body) VALUES (?, ?, ?, ?)`,
		cardID, string(models.EventComment), actor, body,
	)
	if err != nil {
		return models.CardEvent{}, fmt.Errorf("add comment: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.CardEvent{
		ID: id, CardID: cardID, Kind: models.EventComment, Actor: actor, Body: body,
		CreatedAt: time.Now(),
	}, nil
}

// logActivity records an auto-generated activity entry within an existing
// transaction, so it commits atomically with whatever mutation triggered
// it. Unexported — only the mutators in this package call it.
func logActivity(tx *sql.Tx, cardID int64, body string) error {
	_, err := tx.Exec(
		`INSERT INTO card_events (card_id, kind, body) VALUES (?, ?, ?)`,
		cardID, string(models.EventActivity), body,
	)
	return err
}
