package models

// Label is a board-scoped, color-coded tag that can be assigned to cards.
type Label struct {
	ID      int64
	BoardID int64
	Name    string
	Color   string
}
