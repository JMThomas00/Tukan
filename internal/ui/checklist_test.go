package ui

import (
	"strings"
	"testing"

	"github.com/JMThomas00/tukan/internal/styles"
)

// TestChecklistActionMsgHandledBeforeActiveGuard guards a variant of the
// doneMsg-before-guard footgun, now for the embedded checklist: the
// checklist stays open across each action's DB round trip (every keypress
// fires its own command instead of batching on close), and its result
// messages must be intercepted before the formActive guard — otherwise
// they'd be routed into CardFormModel.Update -> ChecklistModel.Update
// (tea.KeyMsg only) and silently dropped, so the DB write's result would
// never patch b.checklists or the still-open editor's own displayed list.
func TestChecklistActionMsgHandledBeforeActiveGuard(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.form = EditCardForm(card, nil, nil, nil, nil, nil, b.width, b.height)
	b.form.section = sectionChecklist
	b.formActive = true

	updated, cmd := b.Update(checklistActionMsg{kind: checklistAdd, cardID: card.ID, text: "Write tests"})
	if !updated.formActive {
		t.Fatal("formActive became false on an action message — only cardFormDoneMsg should close the editor")
	}
	if cmd == nil {
		t.Fatal("expected a create-item command, got nil")
	}

	msg := cmd()
	added, ok := msg.(checklistItemAddedMsg)
	if !ok {
		t.Fatalf("expected checklistItemAddedMsg, got %T", msg)
	}
	if added.cardID != card.ID || added.item.Text != "Write tests" {
		t.Fatalf("checklistItemAddedMsg = %+v, want card %d item 'Write tests'", added, card.ID)
	}

	final, _ := updated.Update(added)
	if len(final.checklists[card.ID]) != 1 {
		t.Fatalf("checklists[%d] = %+v after add, want 1 item", card.ID, final.checklists[card.ID])
	}
	// The still-open editor's own embedded checklist must also reflect the
	// new item — this is the "stay open" half of the contract, not just
	// the board-level map.
	if len(final.form.checklist.items) != 1 || final.form.checklist.items[0].Text != "Write tests" {
		t.Fatalf("form.checklist.items = %+v after add, want 1 item reflected into the open editor", final.form.checklist.items)
	}
	if !final.formActive {
		t.Fatal("formActive should still be true after an add — it only closes on esc/save")
	}
}

// TestChecklistClosedMsgIsANoOpWhenEmbedded confirms checklistClosedMsg —
// which the embedded checklist can now never actually emit (CardFormModel
// intercepts esc itself unless the checklist's own add-item sub-mode is
// open) — is handled as a harmless no-op rather than closing the editor,
// since ChecklistModel remains a standalone-usable type whose own
// browsing-mode esc still produces this message when used outside a form.
func TestChecklistClosedMsgIsANoOpWhenEmbedded(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.form = EditCardForm(card, nil, nil, nil, nil, nil, b.width, b.height)
	b.form.section = sectionChecklist
	b.formActive = true

	updated, cmd := b.Update(checklistClosedMsg{})
	if !updated.formActive {
		t.Fatal("formActive became false from checklistClosedMsg — it should be a no-op when the checklist is embedded")
	}
	if cmd != nil {
		t.Fatal("expected no command from checklistClosedMsg")
	}
}

// TestChecklistToggleAndDeleteFlow exercises the full add -> toggle ->
// delete cycle through BoardModel.Update, confirming each result message
// patches both the board-level map and the still-open editor's embedded
// checklist.
func TestChecklistToggleAndDeleteFlow(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.form = EditCardForm(card, nil, nil, nil, nil, nil, b.width, b.height)
	b.form.section = sectionChecklist
	b.formActive = true

	// Add.
	updated, cmd := b.Update(checklistActionMsg{kind: checklistAdd, cardID: card.ID, text: "Do the thing"})
	added := cmd().(checklistItemAddedMsg)
	updated, _ = updated.Update(added)
	itemID := added.item.ID

	// Toggle.
	updated, cmd = updated.Update(checklistActionMsg{kind: checklistToggle, cardID: card.ID, itemID: itemID})
	if cmd == nil {
		t.Fatal("expected a toggle command")
	}
	toggled := cmd().(checklistItemToggledMsg)
	updated, _ = updated.Update(toggled)
	if !updated.checklists[card.ID][0].Done {
		t.Fatal("item should be done after toggle")
	}
	if !updated.form.checklist.items[0].Done {
		t.Fatal("open editor's item should reflect done after toggle")
	}

	// Delete.
	updated, cmd = updated.Update(checklistActionMsg{kind: checklistDelete, cardID: card.ID, itemID: itemID})
	if cmd == nil {
		t.Fatal("expected a delete command")
	}
	deleted := cmd().(checklistItemDeletedMsg)
	updated, _ = updated.Update(deleted)
	if len(updated.checklists[card.ID]) != 0 {
		t.Fatalf("checklists[%d] = %+v after delete, want none", card.ID, updated.checklists[card.ID])
	}
	if len(updated.form.checklist.items) != 0 {
		t.Fatalf("open editor's items = %+v after delete, want none", updated.form.checklist.items)
	}
	if !updated.formActive {
		t.Fatal("formActive should still be true — deleting an item doesn't close the editor")
	}
}

// TestChecklistViewBareHeader guards two things at once: the header text is
// just "Checklist" (not "Checklist — <card title>", redundant once embedded
// in an editor that already shows the title elsewhere), and it's the only
// visual cue distinguishing "tab landed on this section" from "this is just
// background" — since tabbing into the checklist section doesn't change
// anything else about how it looks. Uses exact-prefix comparison against a
// reference built the same way viewBare itself does, so it holds regardless
// of whether the test environment's terminal profile emits real ANSI or not.
func TestChecklistViewBareHeader(t *testing.T) {
	c := NewChecklist(1, nil, 80, 24)

	unfocused := c.viewBare(false)
	headerLine, _, _ := strings.Cut(unfocused, "\n")
	if headerLine != styles.FormLabelStyle.Render("Checklist") {
		t.Fatalf("header line = %q, want just 'Checklist' (no card-title suffix)", headerLine)
	}

	focused := c.viewBare(true)
	if !strings.HasPrefix(focused, styles.SectionActiveStyle.Render("Checklist")) {
		t.Fatalf("focused header doesn't start with the highlighted 'Checklist' label: %q", focused)
	}
}
