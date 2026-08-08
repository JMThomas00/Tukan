package models

import "time"

// ChecklistItem is a sub-task within a card.
type ChecklistItem struct {
	ID        int64
	CardID    int64
	Text      string
	Done      bool
	Position  int // 0-based, append-only order within its card
	CreatedAt time.Time
	UpdatedAt time.Time
}
