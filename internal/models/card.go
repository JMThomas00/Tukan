package models

import "time"

// Card represents a task on the Kanban board. Assignees, like labels and
// checklist items, aren't carried on the struct itself — they're a
// many-to-many relation loaded and rendered separately (see
// database.ListAssigneesForCard/ForBoard), the same pattern models.Label
// already uses here.
type Card struct {
	ID        int64
	LaneID    int64
	Title     string
	Note      string     // optional
	Position  int        // 0-based vertical order within its lane
	StartDate *time.Time // optional, date-only (no time-of-day)
	DueDate   *time.Time // optional, date-only (no time-of-day)
	TicketNo  int        // permanent, per-board, assigned once at creation — never reused
	CreatedAt time.Time
	UpdatedAt time.Time
}
