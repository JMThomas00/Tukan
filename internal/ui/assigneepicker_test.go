package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/styles"
)

// TestAssigneePickerViewBareHeaderReflectsFocus is the only visual cue
// distinguishing "tab landed on the assignees section" from "this is just
// background". Uses exact-prefix comparison against a reference built the
// same way viewBare itself does, so it holds regardless of whether the test
// environment's terminal profile emits real ANSI or not.
func TestAssigneePickerViewBareHeaderReflectsFocus(t *testing.T) {
	p := NewAssigneePicker(1, nil, nil, 80, 24)

	unfocused := p.viewBare(false)
	if !strings.HasPrefix(unfocused, styles.FormLabelStyle.Render("Assignees")) {
		t.Fatalf("unfocused header doesn't start with the plain 'Assignees' label: %q", unfocused)
	}

	focused := p.viewBare(true)
	if !strings.HasPrefix(focused, styles.SectionActiveStyle.Render("Assignees")) {
		t.Fatalf("focused header doesn't start with the highlighted 'Assignees' label: %q", focused)
	}
}

// TestAssigneePickerCreateResetsToBrowsingMode guards a real bug: enter on
// the "new assignee name" sub-screen used to emit the create command but
// never reset p.mode back to browsing, so the picker stayed stuck showing
// the (now-stale) name input indefinitely — the create had actually already
// gone through in the background, but nothing on screen reflected it until
// esc was pressed separately, which looked like "enter does nothing, esc
// saves" from the outside.
func TestAssigneePickerCreateResetsToBrowsingMode(t *testing.T) {
	p := NewAssigneePicker(1, nil, nil, 80, 24)
	p.mode = assigneePickerCreatingAssignee
	p.nameInput.SetValue("Burt")

	updated, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.mode != assigneePickerBrowsing {
		t.Fatalf("mode after enter = %v, want assigneePickerBrowsing", updated.mode)
	}
	if cmd == nil {
		t.Fatal("expected a create command")
	}
	msg, ok := cmd().(assigneePickerDoneMsg)
	if !ok {
		t.Fatalf("expected assigneePickerDoneMsg, got %T", cmd())
	}
	if !msg.create || msg.name != "Burt" {
		t.Fatalf("assigneePickerDoneMsg = %+v, want create=true name=Burt", msg)
	}
}

// TestAssigneePickerCreateDoesNotCloseTheEditorAndAutoSelects mirrors the
// label picker's equivalent test, plus the one deliberate UX difference:
// registering a new assignee auto-selects them immediately (unlike a new
// label, which still requires a separate toggle), matching how a single
// free-text assignee field used to just work.
func TestAssigneePickerCreateDoesNotCloseTheEditorAndAutoSelects(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.form = EditCardForm(card, nil, nil, nil, nil, nil, b.width, b.height)
	b.form.section = sectionAssignees
	b.formActive = true

	updated, cmd := b.Update(assigneePickerDoneMsg{create: true, name: "Burt"})
	if !updated.formActive {
		t.Fatal("formActive became false after an assignee-create message — creating an assignee must not close the editor")
	}
	if cmd == nil {
		t.Fatal("expected a create-assignee command, got nil")
	}

	msg := cmd()
	reloaded, ok := msg.(assigneesReloadedMsg)
	if !ok {
		t.Fatalf("expected assigneesReloadedMsg, got %T", msg)
	}
	if len(reloaded.assignees) != 1 || reloaded.assignees[0].Name != "Burt" {
		t.Fatalf("reloaded assignees = %+v, want just 'Burt'", reloaded.assignees)
	}
	if reloaded.justCreated.Name != "Burt" {
		t.Fatalf("justCreated = %+v, want 'Burt'", reloaded.justCreated)
	}

	final, _ := updated.Update(reloaded)
	if len(final.allAssignees) != 1 || final.allAssignees[0].Name != "Burt" {
		t.Fatalf("allAssignees = %+v, want just 'Burt'", final.allAssignees)
	}
	if !final.form.assignees.selected[reloaded.justCreated.ID] {
		t.Fatal("newly registered assignee should be auto-selected in the still-open picker, unlike a newly created label")
	}
}
