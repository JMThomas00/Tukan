package ui

import (
	"strings"
	"testing"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

// addTestCard creates a card in the board's first lane and reflects it into
// the in-memory model, the way a real cardCreatedMsg would.
func addTestCard(t *testing.T, db *database.DB, b *BoardModel) models.Card {
	t.Helper()
	card, err := db.CreateCard(models.Card{LaneID: b.lanes[0].ID, Title: "Test card"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	b.cards[card.LaneID] = append(b.cards[card.LaneID], card)
	return card
}

// TestLabelPickerCreateMsgDoesNotCloseTheEditor confirms that creating a
// new label from the embedded picker (the "n" key, unrelated to the
// editor's own ctrl+s save) is handled unconditionally — it doesn't need
// the editor to be in any particular guard state, since a labelPickerDoneMsg
// can only ever be produced by an already-open picker — and, critically,
// does not close the editor the way the old standalone modal's equivalent
// message used to.
func TestLabelPickerCreateMsgDoesNotCloseTheEditor(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.form = EditCardForm(card, nil, nil, nil, nil, nil, b.width, b.height)
	b.form.section = sectionLabels
	b.formActive = true

	updated, cmd := b.Update(labelPickerDoneMsg{action: labelPickerCreate, name: "Urgent", color: "#f7768e"})
	if !updated.formActive {
		t.Fatal("formActive became false after a label-create message — creating a label must not close the editor")
	}
	if cmd == nil {
		t.Fatal("expected a create-label command, got nil")
	}

	msg := cmd()
	reloaded, ok := msg.(labelsReloadedMsg)
	if !ok {
		t.Fatalf("expected labelsReloadedMsg, got %T", msg)
	}
	if len(reloaded.labels) != 1 || reloaded.labels[0].Name != "Urgent" {
		t.Fatalf("reloaded labels = %+v, want just 'Urgent'", reloaded.labels)
	}

	final, _ := updated.Update(reloaded)
	if len(final.boardLabels) != 1 || final.boardLabels[0].Name != "Urgent" {
		t.Fatalf("boardLabels = %+v, want just 'Urgent'", final.boardLabels)
	}
	// The still-open editor's embedded picker must see the new label too,
	// so it's immediately selectable without closing and reopening.
	if len(final.form.labels.all) != 1 || final.form.labels.all[0].Name != "Urgent" {
		t.Fatalf("form.labels.all = %+v, want the newly created label reflected", final.form.labels.all)
	}
}

func TestLabelPickerDeleteMsgDoesNotCloseTheEditor(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	bug, err := db.CreateLabel(b.currentBoardID, "Bug", "#f7768e")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	b.boardLabels = []models.Label{bug}

	b.form = EditCardForm(card, nil, nil, b.boardLabels, nil, nil, b.width, b.height)
	b.form.section = sectionLabels
	b.formActive = true

	updated, cmd := b.Update(labelPickerDoneMsg{action: labelPickerDelete, deleteID: bug.ID})
	if !updated.formActive {
		t.Fatal("formActive became false after a label-delete message")
	}
	if cmd == nil {
		t.Fatal("expected a delete-label command, got nil")
	}

	msg := cmd()
	reloaded, ok := msg.(labelsReloadedMsg)
	if !ok {
		t.Fatalf("expected labelsReloadedMsg, got %T", msg)
	}

	final, _ := updated.Update(reloaded)
	if len(final.boardLabels) != 0 {
		t.Fatalf("boardLabels = %+v after delete, want none", final.boardLabels)
	}
	if len(final.form.labels.all) != 0 {
		t.Fatalf("form.labels.all = %+v after delete, want none", final.form.labels.all)
	}
}

// TestLabelPickerCancelledDoneMsgIsANoOp mirrors
// TestChecklistClosedMsgIsANoOpWhenEmbedded: a cancelled labelPickerDoneMsg
// can't actually be produced by an embedded picker in normal operation
// (CardFormModel intercepts esc itself unless the picker's own
// create-label sub-mode is open), but board.go still handles it — as a
// no-op, not a form-closing action, since LabelPickerModel remains a
// standalone-usable type whose own browsing-mode esc still produces this
// message when used outside a form.
func TestLabelPickerCancelledDoneMsgIsANoOp(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.form = EditCardForm(card, nil, nil, nil, nil, nil, b.width, b.height)
	b.form.section = sectionLabels
	b.formActive = true

	updated, cmd := b.Update(labelPickerDoneMsg{cardID: card.ID, cancelled: true})
	if !updated.formActive {
		t.Fatal("formActive became false from a cancelled labelPickerDoneMsg — it should be a no-op when embedded")
	}
	if cmd != nil {
		t.Fatal("expected no command from a cancelled labelPickerDoneMsg")
	}
}

// TestLabelPickerViewBareHeaderReflectsFocus is the only visual cue
// distinguishing "tab landed on the labels section" from "this is just
// background" — everything else about the picker's own rendering is
// identical either way. Uses exact-prefix comparison against a reference
// built the same way viewBare itself does, so it holds regardless of
// whether the test environment's terminal profile emits real ANSI or not.
func TestLabelPickerViewBareHeaderReflectsFocus(t *testing.T) {
	p := NewLabelPicker(1, nil, nil, 80, 24)

	unfocused := p.viewBare(false)
	if !strings.HasPrefix(unfocused, styles.FormLabelStyle.Render("Labels")) {
		t.Fatalf("unfocused header doesn't start with the plain 'Labels' label: %q", unfocused)
	}

	focused := p.viewBare(true)
	if !strings.HasPrefix(focused, styles.SectionActiveStyle.Render("Labels")) {
		t.Fatalf("focused header doesn't start with the highlighted 'Labels' label: %q", focused)
	}
}
