package models

// Assignee is a registered person, global across boards — the same name is
// the same identity (and the same color) everywhere, not a free-text string
// that has to match byte-for-byte between cards.
type Assignee struct {
	ID   int64
	Name string
}
