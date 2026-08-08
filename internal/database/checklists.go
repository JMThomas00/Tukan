package database

import (
	"fmt"
	"time"

	"github.com/JMThomas00/tukan/internal/models"
)

const checklistItemColumns = `id, card_id, text, done, position, created_at, updated_at`

func scanChecklistItem(scan func(...any) error) (models.ChecklistItem, error) {
	var it models.ChecklistItem
	var done int
	var createdStr, updatedStr string
	err := scan(&it.ID, &it.CardID, &it.Text, &done, &it.Position, &createdStr, &updatedStr)
	if err != nil {
		return it, err
	}
	it.Done = done != 0
	it.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
	it.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr)
	return it, nil
}

// ListChecklistItems returns a card's checklist items ordered by position.
func (d *DB) ListChecklistItems(cardID int64) ([]models.ChecklistItem, error) {
	rows, err := d.sql.Query(
		`SELECT `+checklistItemColumns+` FROM checklist_items WHERE card_id=? ORDER BY position`,
		cardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list checklist items: %w", err)
	}
	defer rows.Close()

	var items []models.ChecklistItem
	for rows.Next() {
		it, err := scanChecklistItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan checklist item: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// ListChecklistItemsForBoard bulk-loads every card's checklist items on a
// board in one query (cardID -> items), the same JOIN-through-lanes shape as
// ListLabelsForBoard, so rendering a board doesn't need a query per card.
func (d *DB) ListChecklistItemsForBoard(boardID int64) (map[int64][]models.ChecklistItem, error) {
	cols := `ci.id, ci.card_id, ci.text, ci.done, ci.position, ci.created_at, ci.updated_at`
	rows, err := d.sql.Query(
		`SELECT `+cols+` FROM checklist_items ci
		 JOIN cards c ON c.id = ci.card_id
		 JOIN lanes l ON l.id = c.lane_id
		 WHERE l.board_id = ?
		 ORDER BY ci.card_id, ci.position`,
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list checklist items for board: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]models.ChecklistItem)
	for rows.Next() {
		it, err := scanChecklistItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan checklist item: %w", err)
		}
		result[it.CardID] = append(result[it.CardID], it)
	}
	return result, rows.Err()
}

// CreateChecklistItem appends a new item to a card's checklist. Wrapped in a
// transaction from the start — unlike CreateCard's original
// non-transactional MAX(position) lookup, there's no reason to repeat that
// latent race in new code.
func (d *DB) CreateChecklistItem(cardID int64, text string) (models.ChecklistItem, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return models.ChecklistItem{}, err
	}
	defer tx.Rollback()

	var maxPos int
	_ = tx.QueryRow(
		`SELECT COALESCE(MAX(position),-1) FROM checklist_items WHERE card_id=?`, cardID,
	).Scan(&maxPos)
	position := maxPos + 1

	res, err := tx.Exec(
		`INSERT INTO checklist_items (card_id, text, position) VALUES (?, ?, ?)`,
		cardID, text, position,
	)
	if err != nil {
		return models.ChecklistItem{}, fmt.Errorf("create checklist item: %w", err)
	}
	id, _ := res.LastInsertId()

	if err := logActivity(tx, cardID, fmt.Sprintf("checklist item added: %q", text)); err != nil {
		return models.ChecklistItem{}, fmt.Errorf("log activity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.ChecklistItem{}, err
	}

	now := time.Now()
	return models.ChecklistItem{
		ID: id, CardID: cardID, Text: text, Position: position,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// UpdateChecklistItemText renames an item's text.
func (d *DB) UpdateChecklistItemText(id int64, text string) error {
	_, err := d.sql.Exec(`UPDATE checklist_items SET text=? WHERE id=?`, text, id)
	return err
}

// ToggleChecklistItem flips an item's done state and logs whether it was
// checked off or unchecked.
func (d *DB) ToggleChecklistItem(id int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cardID int64
	var text string
	var wasDone int
	if err := tx.QueryRow(
		`SELECT card_id, text, done FROM checklist_items WHERE id=?`, id,
	).Scan(&cardID, &text, &wasDone); err != nil {
		return fmt.Errorf("toggle checklist item lookup: %w", err)
	}

	if _, err := tx.Exec(`UPDATE checklist_items SET done = NOT done WHERE id=?`, id); err != nil {
		return fmt.Errorf("toggle checklist item: %w", err)
	}

	verb := "checked off"
	if wasDone != 0 {
		verb = "unchecked"
	}
	if err := logActivity(tx, cardID, fmt.Sprintf("%s: %q", verb, text)); err != nil {
		return fmt.Errorf("log activity: %w", err)
	}

	return tx.Commit()
}

// DeleteChecklistItem removes an item and closes the position gap in its
// card's checklist, mirroring DeleteCard's transactional gap-close.
func (d *DB) DeleteChecklistItem(id int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cardID int64
	var pos int
	var text string
	if err := tx.QueryRow(
		`SELECT card_id, position, text FROM checklist_items WHERE id=?`, id,
	).Scan(&cardID, &pos, &text); err != nil {
		return fmt.Errorf("delete checklist item lookup: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM checklist_items WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete checklist item: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE checklist_items SET position=position-1 WHERE card_id=? AND position>?`,
		cardID, pos,
	); err != nil {
		return fmt.Errorf("reorder after delete: %w", err)
	}

	if err := logActivity(tx, cardID, fmt.Sprintf("checklist item removed: %q", text)); err != nil {
		return fmt.Errorf("log activity: %w", err)
	}

	return tx.Commit()
}
