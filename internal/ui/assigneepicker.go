package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

type assigneePickerSubMode int

const (
	assigneePickerBrowsing assigneePickerSubMode = iota
	assigneePickerCreatingAssignee
)

// assigneePickerDoneMsg mirrors labelPickerDoneMsg's shape, minus a delete
// action (deleting a globally-shared identity off every card and board it's
// on is a heavier action than this picker exposes) and minus a color choice
// (see styles.AssigneeColor — colors are derived from the registry id, not
// picked). assigneeIDs is always the current locally-toggled selection, so
// a create still carries whatever assignment changes were already made.
type assigneePickerDoneMsg struct {
	cardID      int64
	create      bool // true: register a new assignee (name set) in the global registry
	assigneeIDs []int64
	name        string // create
	cancelled   bool
}

// AssigneePickerModel lets the user toggle which registered people are
// assigned to a card, and register a new person on the fly. Assignment is
// batched to the outer editor's own ctrl+s (like labels); registering a new
// person is an immediate, board-independent action instead, since the
// registry is global — see board.go's handling, shaped like
// labelPickerCreate's.
type AssigneePickerModel struct {
	cardID    int64
	all       []models.Assignee
	selected  map[int64]bool
	cursor    int
	mode      assigneePickerSubMode
	nameInput textinput.Model
	width     int
	height    int
}

// NewAssigneePicker creates a picker for a card, pre-selecting whoever's
// already assigned. all is the full global registry, not scoped to a board
// — the same person is the same identity (and color) everywhere.
func NewAssigneePicker(cardID int64, all []models.Assignee, current []models.Assignee, w, h int) AssigneePickerModel {
	selected := make(map[int64]bool, len(current))
	for _, a := range current {
		selected[a.ID] = true
	}

	ni := textinput.New()
	ni.Placeholder = "Assignee name"
	ni.CharLimit = 60

	return AssigneePickerModel{
		cardID:    cardID,
		all:       all,
		selected:  selected,
		nameInput: ni,
		width:     w,
		height:    h,
	}
}

func (p AssigneePickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (p AssigneePickerModel) Update(msg tea.Msg) (AssigneePickerModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}
	km := DefaultKeyMap

	if p.mode == assigneePickerCreatingAssignee {
		return p.updateCreating(keyMsg, km)
	}
	return p.updateBrowsing(keyMsg, km)
}

func (p AssigneePickerModel) updateBrowsing(msg tea.KeyMsg, km KeyMap) (AssigneePickerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		return p, func() tea.Msg {
			return assigneePickerDoneMsg{cardID: p.cardID, cancelled: true}
		}

	case key.Matches(msg, km.MoveUp):
		if len(p.all) > 0 {
			p.cursor = clamp(p.cursor-1, 0, len(p.all)-1)
		}

	case key.Matches(msg, km.MoveDown):
		if len(p.all) > 0 {
			p.cursor = clamp(p.cursor+1, 0, len(p.all)-1)
		}

	case msg.String() == " ", msg.String() == "enter":
		if p.cursor < len(p.all) {
			id := p.all[p.cursor].ID
			p.selected[id] = !p.selected[id]
		}

	case msg.String() == "n":
		p.mode = assigneePickerCreatingAssignee
		p.nameInput.SetValue("")
		p.nameInput.Focus()
	}
	return p, nil
}

func (p AssigneePickerModel) updateCreating(msg tea.KeyMsg, km KeyMap) (AssigneePickerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		p.mode = assigneePickerBrowsing
		p.nameInput.Blur()
		return p, nil

	case msg.String() == "enter":
		name := strings.TrimSpace(p.nameInput.Value())
		if name == "" {
			return p, nil
		}
		p.mode = assigneePickerBrowsing
		p.nameInput.Blur()
		return p, func() tea.Msg {
			return assigneePickerDoneMsg{cardID: p.cardID, create: true, assigneeIDs: p.selectedIDs(), name: name}
		}
	}

	var cmd tea.Cmd
	p.nameInput, cmd = p.nameInput.Update(msg)
	return p, cmd
}

func (p AssigneePickerModel) selectedIDs() []int64 {
	var ids []int64
	for _, a := range p.all {
		if p.selected[a.ID] {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// viewBare renders just the picker's content, with no surrounding box or
// placement — used when embedded inside another screen (CardFormModel), the
// same convention LabelPickerModel/ChecklistModel use.
func (p AssigneePickerModel) viewBare(focused bool) string {
	var b strings.Builder
	headerStyle := styles.FormLabelStyle
	if focused {
		headerStyle = styles.SectionActiveStyle
	}
	b.WriteString(headerStyle.Render("Assignees") + "\n\n")

	switch p.mode {
	case assigneePickerCreatingAssignee:
		b.WriteString(styles.FormLabelStyle.Render("New assignee name") + "\n")
		b.WriteString(p.nameInput.View() + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("enter save  esc cancel"))

	default:
		if len(p.all) == 0 {
			b.WriteString(styles.CardNoteStyle.Render("No assignees yet — press n to add one") + "\n")
		}
		for i, a := range p.all {
			mark := "[ ]"
			if p.selected[a.ID] {
				mark = "[x]"
			}
			chip := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.AssigneeColor(a.ID))).Render("●")
			line := fmt.Sprintf("%s %s %s", mark, chip, a.Name)
			if i == p.cursor {
				line = styles.HelpKeyStyle.Render("› " + line)
			} else {
				line = styles.FormLabelStyle.Render("  " + line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + styles.HelpDescStyle.Render("↑/↓ select  space toggle  n new  esc close"))
	}

	return b.String()
}
