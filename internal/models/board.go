package models

import "time"

// Board represents an independent kanban board: its own set of lanes and cards.
type Board struct {
	ID        int64
	Name      string
	Position  int
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
