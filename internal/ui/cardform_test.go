package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/models"
)

// TestCardFormViewDoesNotPanicAcrossSizes guards the note-height/body-height
// arithmetic in View() (subtraction-heavy, driven by lipgloss.Height() on
// dynamically-built content) against negative or zero terminal sizes —
// a tiny, a typical, and a very large window all need to render without
// panicking, not just look reasonable in a typical one.
func TestCardFormViewDoesNotPanicAcrossSizes(t *testing.T) {
	labels := []models.Label{{ID: 1, Name: "Bug", Color: "#f7768e"}}
	sizes := [][2]int{{0, 0}, {1, 1}, {40, 10}, {80, 24}, {200, 60}}
	for _, s := range sizes {
		f := NewCardForm(1, nil, labels, s[0], s[1])
		_ = f.View() // must not panic
		f.err = "Title is required"
		_ = f.View() // must not panic with the error block present either
	}
}

// TestCardFormSectionCyclingWraps confirms the cycle order matches the
// two-column visual layout (Title/StartDate/DueDate/Note stacked on the
// left, then Assignees/Labels/Checklist stacked on the right) — reading
// order, top-to-bottom then left-to-right, not the historical field order.
// Uses EditCardForm (an already-saved card) since sectionChecklist is only
// reachable at all once the card has a real ID — see
// TestCardFormSectionCyclingSkipsChecklistWhileCreating for the create-mode
// behavior this would otherwise mask.
func TestCardFormSectionCyclingWraps(t *testing.T) {
	f := EditCardForm(models.Card{ID: 1, LaneID: 1}, nil, nil, nil, nil, nil, 80, 24)

	for _, want := range []int{sectionStartDate, sectionDueDate, sectionNote, sectionAssignees, sectionLabels, sectionChecklist, sectionTitle} {
		f = f.advanceSection(1)
		if f.section != want {
			t.Fatalf("section after advance = %d, want %d", f.section, want)
		}
	}

	// And backwards from sectionTitle should wrap to sectionChecklist.
	f = f.advanceSection(-1)
	if f.section != sectionChecklist {
		t.Fatalf("section after advance(-1) from title = %d, want sectionChecklist (%d)", f.section, sectionChecklist)
	}
}

// TestCardFormSectionCyclingSkipsChecklistWhileCreating is the regression
// test for the bug found live: a not-yet-created card has ID 0, and
// checklist_items.card_id is a foreign-key column, so adding an item to a
// new card's checklist before saving always failed — silently, since the
// resulting error sets the board's status bar, which isn't visible behind
// this modal. sectionChecklist must not be reachable via Tab/shift+tab
// while creating; it becomes reachable again once the card is saved and
// reopened via EditCardForm (see TestCardFormSectionCyclingWraps).
func TestCardFormSectionCyclingSkipsChecklistWhileCreating(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)

	for _, want := range []int{sectionStartDate, sectionDueDate, sectionNote, sectionAssignees, sectionLabels, sectionTitle} {
		f = f.advanceSection(1)
		if f.section != want {
			t.Fatalf("section after advance = %d, want %d (sectionChecklist should be skipped entirely)", f.section, want)
		}
	}

	// And backwards from sectionTitle should skip straight to sectionLabels,
	// not land on sectionChecklist either.
	f = f.advanceSection(-1)
	if f.section != sectionLabels {
		t.Fatalf("section after advance(-1) from title = %d, want sectionLabels (%d) — sectionChecklist should be skipped", f.section, sectionLabels)
	}
}

// TestCardFormEscInAssigneeSubModeBacksOutOneLevel mirrors the label case
// for the embedded assignee picker's own create-assignee sub-mode.
func TestCardFormEscInAssigneeSubModeBacksOutOneLevel(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.section = sectionAssignees
	f.assignees.mode = assigneePickerCreatingAssignee

	updated, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("expected no cardFormDoneMsg command — esc should back out of the sub-mode, not close the editor")
	}
	if updated.assignees.mode != assigneePickerBrowsing {
		t.Fatalf("assignees.mode = %v after esc, want assigneePickerBrowsing (backed out)", updated.assignees.mode)
	}
}

// TestCardFormEscInLabelSubModeBacksOutOneLevel confirms esc while the
// embedded picker's own create-label sub-mode is open is forwarded down to
// back out of that sub-mode, not intercepted to close the whole editor.
func TestCardFormEscInLabelSubModeBacksOutOneLevel(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.section = sectionLabels
	f.labels.mode = labelPickerCreatingLabel

	updated, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("expected no cardFormDoneMsg command — esc should back out of the sub-mode, not close the editor")
	}
	if updated.labels.mode != labelPickerBrowsing {
		t.Fatalf("labels.mode = %v after esc, want labelPickerBrowsing (backed out)", updated.labels.mode)
	}
}

// TestCardFormEscInChecklistSubModeBacksOutOneLevel mirrors the label case
// for the embedded checklist's own add-item sub-mode.
func TestCardFormEscInChecklistSubModeBacksOutOneLevel(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.section = sectionChecklist
	f.checklist.mode = checklistAddingItem

	updated, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("expected no cardFormDoneMsg command — esc should back out of the sub-mode, not close the editor")
	}
	if updated.checklist.mode != checklistBrowsing {
		t.Fatalf("checklist.mode = %v after esc, want checklistBrowsing (backed out)", updated.checklist.mode)
	}
}

// TestCardFormEscWithNoSubModeClosesEditor confirms esc closes the whole
// editor when no section has an open sub-mode — the common case.
func TestCardFormEscWithNoSubModeClosesEditor(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	// sectionTitle by default — no sub-mode concept there at all.

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a cardFormDoneMsg command")
	}
	done, ok := cmd().(cardFormDoneMsg)
	if !ok {
		t.Fatalf("expected cardFormDoneMsg, got different type")
	}
	if !done.cancelled {
		t.Fatal("expected cancelled=true")
	}
}

// TestCardFormEscOnLabelsSectionWithoutSubModeClosesEditor confirms that
// when the labels section is focused but browsing (no create-label
// sub-mode open), esc still closes the whole editor rather than reaching
// the picker's own browsing-mode esc handling (which would otherwise emit
// its own now-unreachable labelPickerDoneMsg{cancelled:true}).
func TestCardFormEscOnLabelsSectionWithoutSubModeClosesEditor(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.section = sectionLabels // browsing mode (the zero value), not creating

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a cardFormDoneMsg command")
	}
	done, ok := cmd().(cardFormDoneMsg)
	if !ok {
		t.Fatalf("expected cardFormDoneMsg (editor closing), got a different message — esc must not have reached the picker's own handling")
	}
	if !done.cancelled {
		t.Fatal("expected cancelled=true")
	}
}

// TestCardFormSubmitProducesCardAssigneeIDsAndLabelIDs confirms ctrl+s
// always saves the whole card, including whatever was toggled in both
// embedded pickers — even though neither picker's own ctrl+s handling
// (labelPickerCommit) ever gets a chance to fire, since CardFormModel
// intercepts Submit first.
func TestCardFormSubmitProducesCardAssigneeIDsAndLabelIDs(t *testing.T) {
	jordan := models.Assignee{ID: 7, Name: "jordan"}
	bug := models.Label{ID: 42, Name: "Bug", Color: "#f7768e"}
	f := NewCardForm(1, []models.Assignee{jordan}, []models.Label{bug}, 80, 24)
	f.title.SetValue("Fix login")

	// Toggle the one available assignee.
	f.section = sectionAssignees
	f.assignees, _ = f.assignees.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !f.assignees.selected[jordan.ID] {
		t.Fatal("test setup: assignee should be selected after space")
	}

	// Toggle the one available label.
	f.section = sectionLabels
	f.labels, _ = f.labels.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !f.labels.selected[bug.ID] {
		t.Fatal("test setup: label should be selected after space")
	}

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a cardFormDoneMsg command")
	}
	done, ok := cmd().(cardFormDoneMsg)
	if !ok {
		t.Fatalf("expected cardFormDoneMsg, got a different type")
	}
	if done.cancelled {
		t.Fatal("expected cancelled=false on save")
	}
	if done.card.Title != "Fix login" {
		t.Fatalf("card = %+v, want Title=Fix login", done.card)
	}
	if len(done.assigneeIDs) != 1 || done.assigneeIDs[0] != jordan.ID {
		t.Fatalf("assigneeIDs = %v, want [%d]", done.assigneeIDs, jordan.ID)
	}
	if len(done.labelIDs) != 1 || done.labelIDs[0] != bug.ID {
		t.Fatalf("labelIDs = %v, want [%d]", done.labelIDs, bug.ID)
	}
}

// TestCardFormSubmitWithNoAssigneeSucceeds confirms assignees are optional
// now that they're a multi-select registry pick rather than a required
// free-text field — only the title is still required.
func TestCardFormSubmitWithNoAssigneeSucceeds(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.title.SetValue("Fix login")

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a cardFormDoneMsg command")
	}
	done := cmd().(cardFormDoneMsg)
	if done.cancelled {
		t.Fatal("expected cancelled=false on save")
	}
	if len(done.assigneeIDs) != 0 {
		t.Fatalf("assigneeIDs = %v, want none", done.assigneeIDs)
	}
}

// TestCardFormSubmitWithStartAndDueDateSucceeds confirms a valid start date
// before the due date is accepted and carried onto the saved card.
func TestCardFormSubmitWithStartAndDueDateSucceeds(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.title.SetValue("Fix login")
	f.startDate.SetValue("2026-08-10")
	f.dueDate.SetValue("2026-08-20")

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a cardFormDoneMsg command")
	}
	done, ok := cmd().(cardFormDoneMsg)
	if !ok {
		t.Fatalf("expected cardFormDoneMsg, got a different type")
	}
	if done.cancelled {
		t.Fatal("expected cancelled=false on save")
	}
	if done.card.StartDate == nil || done.card.StartDate.Format(dueDateLayout) != "2026-08-10" {
		t.Fatalf("card.StartDate = %v, want 2026-08-10", done.card.StartDate)
	}
	if done.card.DueDate == nil || done.card.DueDate.Format(dueDateLayout) != "2026-08-20" {
		t.Fatalf("card.DueDate = %v, want 2026-08-20", done.card.DueDate)
	}
}

// TestCardFormSubmitRejectsStartDateAfterDueDate confirms the new
// start-before-due validation blocks the save (mirroring the existing
// invalid-date-format validation's early-return-with-error shape) rather
// than silently accepting a nonsensical date range.
func TestCardFormSubmitRejectsStartDateAfterDueDate(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.title.SetValue("Fix login")
	f.startDate.SetValue("2026-08-20")
	f.dueDate.SetValue("2026-08-10")

	updated, cmd := f.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		t.Fatal("expected no cardFormDoneMsg command — start-after-due should block the save")
	}
	if updated.err == "" {
		t.Fatal("expected a validation error message")
	}
}

// TestCardFormSubmitWithInvalidStartDateFails mirrors the existing
// invalid-due-date-format test for the new start date field.
func TestCardFormSubmitWithInvalidStartDateFails(t *testing.T) {
	f := NewCardForm(1, nil, nil, 80, 24)
	f.title.SetValue("Fix login")
	f.startDate.SetValue("not-a-date")

	updated, cmd := f.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		t.Fatal("expected no cardFormDoneMsg command — invalid start date should block the save")
	}
	if updated.err == "" {
		t.Fatal("expected a validation error message")
	}
}

// TestCreateCardWithAssigneesAndLabelsEndToEnd drives the whole
// create-with-assignments path through BoardModel and the real database:
// toggling an assignee and a label before the card exists, saving, and
// confirming the DB actually has both attached to the newly created card —
// the exact ordering hazard cmdCreateCard's assigneeIDs/labelIDs parameters
// exist to avoid (a card's ID doesn't exist until CreateCard returns, so a
// naive separate SetCardAssignees/SetCardLabels call can't be batched
// concurrently with it).
func TestCreateCardWithAssigneesAndLabelsEndToEnd(t *testing.T) {
	db, b := newTestBoard(t)

	jordan, err := db.GetOrCreateAssignee("jordan")
	if err != nil {
		t.Fatalf("create assignee: %v", err)
	}
	b.allAssignees = []models.Assignee{jordan}

	bug, err := db.CreateLabel(b.currentBoardID, "Bug", "#f7768e")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	b.boardLabels = []models.Label{bug}

	b.form = NewCardForm(b.lanes[0].ID, b.allAssignees, b.boardLabels, b.width, b.height)
	b.form.title.SetValue("New card")
	b.form.section = sectionAssignees
	b.form.assignees, _ = b.form.assignees.Update(tea.KeyMsg{Type: tea.KeySpace})
	b.form.section = sectionLabels
	b.form.labels, _ = b.form.labels.Update(tea.KeyMsg{Type: tea.KeySpace})
	b.formActive = true

	updated, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a cardFormDoneMsg command from ctrl+s")
	}
	done := cmd().(cardFormDoneMsg)

	updated, cmd = updated.Update(done)
	if cmd == nil {
		t.Fatal("expected a cmdCreateCard command")
	}
	created := cmd().(cardCreatedMsg)
	if created.card.ID == 0 {
		t.Fatal("expected a real DB-assigned card ID")
	}
	if len(created.assignees) != 1 || created.assignees[0].ID != jordan.ID {
		t.Fatalf("cardCreatedMsg.assignees = %+v, want just jordan", created.assignees)
	}
	if len(created.labels) != 1 || created.labels[0].ID != bug.ID {
		t.Fatalf("cardCreatedMsg.labels = %+v, want just Bug", created.labels)
	}

	final, _ := updated.Update(created)
	if len(final.cardAssignees[created.card.ID]) != 1 {
		t.Fatalf("cardAssignees[%d] = %+v, want jordan reflected immediately (not just after a reload)", created.card.ID, final.cardAssignees[created.card.ID])
	}
	if len(final.cardLabels[created.card.ID]) != 1 {
		t.Fatalf("cardLabels[%d] = %+v, want the label reflected immediately (not just after a reload)", created.card.ID, final.cardLabels[created.card.ID])
	}

	// And confirm both are actually persisted, not just in the in-memory model.
	byBoardAssignees, err := db.ListAssigneesForBoard(b.currentBoardID)
	if err != nil {
		t.Fatalf("list assignees for board: %v", err)
	}
	if len(byBoardAssignees[created.card.ID]) != 1 || byBoardAssignees[created.card.ID][0].ID != jordan.ID {
		t.Fatalf("DB assignees for card %d = %+v, want just jordan", created.card.ID, byBoardAssignees[created.card.ID])
	}
	byBoardLabels, err := db.ListLabelsForBoard(b.currentBoardID)
	if err != nil {
		t.Fatalf("list labels for board: %v", err)
	}
	if len(byBoardLabels[created.card.ID]) != 1 || byBoardLabels[created.card.ID][0].ID != bug.ID {
		t.Fatalf("DB labels for card %d = %+v, want just Bug", created.card.ID, byBoardLabels[created.card.ID])
	}
}
