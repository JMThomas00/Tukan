package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/models"
)

func typeString(t *testing.T, b BoardModel, s string) BoardModel {
	t.Helper()
	for _, r := range s {
		b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return b
}

// TestFilterOpenTypeApplyClear exercises the full inline-filter lifecycle:
// open with "/", live-narrow while typing, apply with enter (grid stays
// filtered while browsable again), then clear with esc.
func TestFilterOpenTypeApplyClear(t *testing.T) {
	db, b := newTestBoard(t)
	laneID := b.lanes[0].ID

	titles := []string{"Fix login bug", "Update docs", "Refactor auth"}
	for _, title := range titles {
		card, err := db.CreateCard(models.Card{LaneID: laneID, Title: title})
		if err != nil {
			t.Fatalf("create card %q: %v", title, err)
		}
		b.cards[laneID] = append(b.cards[laneID], card)
	}

	if len(b.visibleCards(laneID)) != 3 {
		t.Fatalf("visibleCards before filtering = %d, want 3", len(b.visibleCards(laneID)))
	}

	// Open the filter bar.
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if b.mode != modeFilterEdit {
		t.Fatalf("mode after '/' = %v, want modeFilterEdit", b.mode)
	}

	// Type "bug" — the grid should live-narrow while still editing.
	b = typeString(t, b, "bug")
	if b.filterQuery != "bug" {
		t.Fatalf("filterQuery while typing = %q, want %q", b.filterQuery, "bug")
	}
	visible := b.visibleCards(laneID)
	if len(visible) != 1 || visible[0].Title != "Fix login bug" {
		t.Fatalf("visibleCards while typing = %+v, want just 'Fix login bug'", visible)
	}

	// Apply with enter: back to modeNormal, filter stays applied.
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if b.mode != modeNormal {
		t.Fatalf("mode after enter = %v, want modeNormal", b.mode)
	}
	if b.filterQuery != "bug" {
		t.Fatalf("filterQuery after apply = %q, want it to stay %q", b.filterQuery, "bug")
	}
	if len(b.visibleCards(laneID)) != 1 {
		t.Fatalf("visibleCards after apply = %d, want 1", len(b.visibleCards(laneID)))
	}

	// Clear with esc while in modeNormal.
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if b.filterQuery != "" {
		t.Fatalf("filterQuery after esc = %q, want empty", b.filterQuery)
	}
	if len(b.visibleCards(laneID)) != 3 {
		t.Fatalf("visibleCards after clearing = %d, want 3", len(b.visibleCards(laneID)))
	}
}

// TestFilterEscWhileEditingClearsAndCloses confirms esc while still
// composing the query (not yet applied with enter) discards it entirely,
// rather than applying a half-typed query.
func TestFilterEscWhileEditingClearsAndCloses(t *testing.T) {
	_, b := newTestBoard(t)

	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	b = typeString(t, b, "partial")
	if b.filterQuery != "partial" {
		t.Fatalf("filterQuery while typing = %q, want %q", b.filterQuery, "partial")
	}

	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if b.mode != modeNormal {
		t.Fatalf("mode after esc = %v, want modeNormal", b.mode)
	}
	if b.filterQuery != "" {
		t.Fatalf("filterQuery after esc-while-editing = %q, want empty (discarded, not applied)", b.filterQuery)
	}
}

// TestFilterClampsCursorWhenSetShrinks confirms the cursor doesn't point
// past the end of the filtered set after typing narrows it.
func TestFilterClampsCursorWhenSetShrinks(t *testing.T) {
	db, b := newTestBoard(t)
	laneID := b.lanes[0].ID

	titles := []string{"Fix login bug", "Update docs", "Refactor auth"}
	for _, title := range titles {
		card, err := db.CreateCard(models.Card{LaneID: laneID, Title: title})
		if err != nil {
			t.Fatalf("create card %q: %v", title, err)
		}
		b.cards[laneID] = append(b.cards[laneID], card)
	}
	b.focusCard = 2 // "Refactor auth", the last card

	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	b = typeString(t, b, "bug") // narrows to 1 card — focusCard=2 is now out of range

	if b.focusCard != 0 {
		t.Fatalf("focusCard = %d after the filtered set shrank to 1, want 0 (clamped)", b.focusCard)
	}
}
