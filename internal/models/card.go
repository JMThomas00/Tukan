package models

import "time"

// Card represents a task on the Kanban board.
type Card struct {
	ID        int64
	LaneID    int64
	Title     string
	Assignee  string
	Note      string // optional
	Position  int    // 0-based vertical order within its lane
	CreatedAt time.Time
	UpdatedAt time.Time
}
