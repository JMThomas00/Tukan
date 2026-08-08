package ui

import (
	"strings"

	"github.com/JMThomas00/tukan/internal/models"
)

// filterCriteria is the parsed form of the current filter query.
//
// Only the bare substring MVP is implemented in this commit (title,
// assignee, and note, case-insensitive). Token syntax (@name, #label,
// due:overdue|today|week) and a cross-board search mode are a deliberately
// separate follow-on — see the roadmap note for 1.6b. Keeping this as its
// own struct now (rather than a bare string) means that follow-on doesn't
// have to change cardMatchesFilter's call sites, only parseFilterQuery and
// this struct's fields.
type filterCriteria struct {
	text string
}

func parseFilterQuery(q string) filterCriteria {
	return filterCriteria{text: strings.ToLower(strings.TrimSpace(q))}
}

func (fc filterCriteria) empty() bool {
	return fc.text == ""
}

// cardMatchesFilter reports whether a card matches the current filter. An
// empty filter matches everything. assignees is the card's current
// assignee list — no longer a field on models.Card itself (it's a
// many-to-many relation, the same as labels), so callers pass in whatever
// BoardModel.cardAssignees currently holds for this card.
func cardMatchesFilter(card models.Card, assignees []models.Assignee, fc filterCriteria) bool {
	if fc.empty() {
		return true
	}
	var names strings.Builder
	for _, a := range assignees {
		names.WriteString(a.Name)
		names.WriteByte(' ')
	}
	haystack := strings.ToLower(card.Title + " " + names.String() + card.Note)
	return strings.Contains(haystack, fc.text)
}
