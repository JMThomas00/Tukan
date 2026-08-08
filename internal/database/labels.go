package database

import (
	"fmt"

	"github.com/JMThomas00/tukan/internal/models"
)

const labelColumns = `id, board_id, name, color`

func scanLabel(scan func(...any) error) (models.Label, error) {
	var l models.Label
	err := scan(&l.ID, &l.BoardID, &l.Name, &l.Color)
	return l, err
}

// ListLabelsByBoard returns a board's label palette, ordered by name.
func (d *DB) ListLabelsByBoard(boardID int64) ([]models.Label, error) {
	rows, err := d.sql.Query(`SELECT `+labelColumns+` FROM labels WHERE board_id=? ORDER BY name`, boardID)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	defer rows.Close()

	var labels []models.Label
	for rows.Next() {
		l, err := scanLabel(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// CreateLabel adds a new label to a board's palette.
func (d *DB) CreateLabel(boardID int64, name, color string) (models.Label, error) {
	res, err := d.sql.Exec(
		`INSERT INTO labels (board_id, name, color) VALUES (?, ?, ?)`,
		boardID, name, color,
	)
	if err != nil {
		return models.Label{}, fmt.Errorf("create label: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.Label{ID: id, BoardID: boardID, Name: name, Color: color}, nil
}

// DeleteLabel removes a label from its board's palette. Every assignment of
// it to a card is removed too, via card_labels' CASCADE.
func (d *DB) DeleteLabel(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM labels WHERE id=?`, id)
	return err
}

// SetCardLabels replaces a card's full label assignment in one transaction
// (delete-then-bulk-insert), mirroring MoveCard's transactional style. Diffs
// the old assignment against the new one to log one activity entry per
// label added/removed.
func (d *DB) SetCardLabels(cardID int64, labelIDs []int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	oldRows, err := tx.Query(
		`SELECT l.id, l.name FROM card_labels cl JOIN labels l ON l.id = cl.label_id WHERE cl.card_id=?`,
		cardID,
	)
	if err != nil {
		return fmt.Errorf("snapshot old labels: %w", err)
	}
	old := make(map[int64]string)
	for oldRows.Next() {
		var id int64
		var name string
		if err := oldRows.Scan(&id, &name); err != nil {
			oldRows.Close()
			return fmt.Errorf("scan old label: %w", err)
		}
		old[id] = name
	}
	if err := oldRows.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM card_labels WHERE card_id=?`, cardID); err != nil {
		return fmt.Errorf("clear card labels: %w", err)
	}

	newSet := make(map[int64]bool, len(labelIDs))
	for _, labelID := range labelIDs {
		newSet[labelID] = true
		if _, err := tx.Exec(
			`INSERT INTO card_labels (card_id, label_id) VALUES (?, ?)`,
			cardID, labelID,
		); err != nil {
			return fmt.Errorf("set card label: %w", err)
		}
	}

	for id, name := range old {
		if !newSet[id] {
			if err := logActivity(tx, cardID, fmt.Sprintf("label %q removed", name)); err != nil {
				return fmt.Errorf("log activity: %w", err)
			}
		}
	}
	for _, id := range labelIDs {
		if _, existed := old[id]; existed {
			continue
		}
		var name string
		if err := tx.QueryRow(`SELECT name FROM labels WHERE id=?`, id).Scan(&name); err != nil {
			return fmt.Errorf("lookup new label name: %w", err)
		}
		if err := logActivity(tx, cardID, fmt.Sprintf("label %q added", name)); err != nil {
			return fmt.Errorf("log activity: %w", err)
		}
	}

	return tx.Commit()
}

// ListLabelsForBoard bulk-loads every card's assigned labels on a board in
// one query (cardID -> labels), so rendering a board doesn't need a query
// per card.
func (d *DB) ListLabelsForBoard(boardID int64) (map[int64][]models.Label, error) {
	rows, err := d.sql.Query(
		`SELECT cl.card_id, l.id, l.board_id, l.name, l.color
		 FROM card_labels cl
		 JOIN labels l ON l.id = cl.label_id
		 JOIN cards c  ON c.id = cl.card_id
		 JOIN lanes ln ON ln.id = c.lane_id
		 WHERE ln.board_id = ?
		 ORDER BY l.name`,
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list labels for board: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]models.Label)
	for rows.Next() {
		var cardID int64
		var l models.Label
		if err := rows.Scan(&cardID, &l.ID, &l.BoardID, &l.Name, &l.Color); err != nil {
			return nil, fmt.Errorf("scan card label: %w", err)
		}
		result[cardID] = append(result[cardID], l)
	}
	return result, rows.Err()
}
