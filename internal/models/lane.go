package models

// Lane represents a horizontal column on the Kanban board.
type Lane struct {
	ID       int64
	Name     string
	Position int    // 0-based display order
	Color    string // hex color, empty = use theme default
}
