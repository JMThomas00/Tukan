package database

import (
	"fmt"
	"strings"

	"github.com/JMThomas00/tukan/internal/models"
)

const assigneeColumns = `id, name`

func scanAssignee(scan func(...any) error) (models.Assignee, error) {
	var a models.Assignee
	err := scan(&a.ID, &a.Name)
	return a, err
}

// ListAssignees returns the full global assignee registry, ordered by name.
func (d *DB) ListAssignees() ([]models.Assignee, error) {
	rows, err := d.sql.Query(`SELECT ` + assigneeColumns + ` FROM assignees ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list assignees: %w", err)
	}
	defer rows.Close()

	var out []models.Assignee
	for rows.Next() {
		a, err := scanAssignee(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan assignee: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetOrCreateAssignee registers a name in the global registry if it isn't
// already there, and returns its (new or existing) row either way — the
// same person typed on two different cards resolves to one identity.
func (d *DB) GetOrCreateAssignee(name string) (models.Assignee, error) {
	name = strings.TrimSpace(name)
	if _, err := d.sql.Exec(
		`INSERT INTO assignees (name) VALUES (?) ON CONFLICT(name) DO NOTHING`, name,
	); err != nil {
		return models.Assignee{}, fmt.Errorf("register assignee: %w", err)
	}
	var a models.Assignee
	err := d.sql.QueryRow(`SELECT `+assigneeColumns+` FROM assignees WHERE name=?`, name).Scan(&a.ID, &a.Name)
	if err != nil {
		return models.Assignee{}, fmt.Errorf("lookup assignee: %w", err)
	}
	return a, nil
}

// SetCardAssignees replaces a card's full assignee list in one transaction
// (delete-then-bulk-insert), mirroring SetCardLabels. Diffs the old
// assignment against the new one to log one activity entry per person
// added/removed.
func (d *DB) SetCardAssignees(cardID int64, assigneeIDs []int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	oldRows, err := tx.Query(
		`SELECT a.id, a.name FROM card_assignees ca JOIN assignees a ON a.id = ca.assignee_id WHERE ca.card_id=?`,
		cardID,
	)
	if err != nil {
		return fmt.Errorf("snapshot old assignees: %w", err)
	}
	old := make(map[int64]string)
	for oldRows.Next() {
		var id int64
		var name string
		if err := oldRows.Scan(&id, &name); err != nil {
			oldRows.Close()
			return fmt.Errorf("scan old assignee: %w", err)
		}
		old[id] = name
	}
	if err := oldRows.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM card_assignees WHERE card_id=?`, cardID); err != nil {
		return fmt.Errorf("clear card assignees: %w", err)
	}

	newSet := make(map[int64]bool, len(assigneeIDs))
	for _, id := range assigneeIDs {
		newSet[id] = true
		if _, err := tx.Exec(
			`INSERT INTO card_assignees (card_id, assignee_id) VALUES (?, ?)`,
			cardID, id,
		); err != nil {
			return fmt.Errorf("set card assignee: %w", err)
		}
	}

	for id, name := range old {
		if !newSet[id] {
			if err := logActivity(tx, cardID, fmt.Sprintf("assignee %q removed", name)); err != nil {
				return fmt.Errorf("log activity: %w", err)
			}
		}
	}
	for _, id := range assigneeIDs {
		if _, existed := old[id]; existed {
			continue
		}
		var name string
		if err := tx.QueryRow(`SELECT name FROM assignees WHERE id=?`, id).Scan(&name); err != nil {
			return fmt.Errorf("lookup new assignee name: %w", err)
		}
		if err := logActivity(tx, cardID, fmt.Sprintf("assignee %q added", name)); err != nil {
			return fmt.Errorf("log activity: %w", err)
		}
	}

	return tx.Commit()
}

// ListAssigneesForCard returns one card's currently assigned people.
func (d *DB) ListAssigneesForCard(cardID int64) ([]models.Assignee, error) {
	rows, err := d.sql.Query(
		`SELECT a.id, a.name FROM card_assignees ca JOIN assignees a ON a.id = ca.assignee_id
		 WHERE ca.card_id=? ORDER BY a.name`,
		cardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list assignees for card: %w", err)
	}
	defer rows.Close()

	var out []models.Assignee
	for rows.Next() {
		a, err := scanAssignee(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan assignee: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAssigneesForBoard bulk-loads every card's assignees on a board in one
// query (cardID -> assignees), so rendering a board doesn't need a query per
// card — mirrors ListLabelsForBoard.
func (d *DB) ListAssigneesForBoard(boardID int64) (map[int64][]models.Assignee, error) {
	rows, err := d.sql.Query(
		`SELECT ca.card_id, a.id, a.name
		 FROM card_assignees ca
		 JOIN assignees a  ON a.id = ca.assignee_id
		 JOIN cards c      ON c.id = ca.card_id
		 JOIN lanes ln     ON ln.id = c.lane_id
		 WHERE ln.board_id = ?
		 ORDER BY a.name`,
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list assignees for board: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]models.Assignee)
	for rows.Next() {
		var cardID int64
		var a models.Assignee
		if err := rows.Scan(&cardID, &a.ID, &a.Name); err != nil {
			return nil, fmt.Errorf("scan card assignee: %w", err)
		}
		result[cardID] = append(result[cardID], a)
	}
	return result, rows.Err()
}
