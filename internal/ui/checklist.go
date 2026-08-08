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

type checklistSubMode int

const (
	checklistBrowsing checklistSubMode = iota
	checklistAddingItem
)

type checklistActionKind int

const (
	checklistAdd checklistActionKind = iota
	checklistToggle
	checklistDelete
)

// checklistActionMsg requests a DB mutation while the modal stays open —
// unlike the other modals' doneMsg, this does NOT close the checklist.
type checklistActionMsg struct {
	kind   checklistActionKind
	cardID int64
	itemID int64  // checklistToggle, checklistDelete
	text   string // checklistAdd
}

// checklistClosedMsg is emitted on esc; mutations are already committed
// individually by the time this fires, so it carries no payload.
type checklistClosedMsg struct{}

// ChecklistModel is the per-card checklist modal: toggle, add, and delete
// items. Every action fires its own DB command immediately (unlike the
// label picker's batch-on-close), so the modal renders whatever
// BoardModel's checklists map currently holds for this card.
type ChecklistModel struct {
	cardID  int64
	items   []models.ChecklistItem
	cursor  int
	mode    checklistSubMode
	newItem textinput.Model
	width   int
	height  int
}

// NewChecklist creates a checklist modal for a card.
func NewChecklist(cardID int64, items []models.ChecklistItem, w, h int) ChecklistModel {
	ni := textinput.New()
	ni.Placeholder = "Checklist item"
	ni.CharLimit = 120

	return ChecklistModel{
		cardID:  cardID,
		items:   items,
		newItem: ni,
		width:   w,
		height:  h,
	}
}

func (c ChecklistModel) Init() tea.Cmd {
	return textinput.Blink
}

func (c ChecklistModel) Update(msg tea.Msg) (ChecklistModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	km := DefaultKeyMap

	if c.mode == checklistAddingItem {
		return c.updateAdding(keyMsg, km)
	}
	return c.updateBrowsing(keyMsg, km)
}

func (c ChecklistModel) updateBrowsing(msg tea.KeyMsg, km KeyMap) (ChecklistModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		return c, func() tea.Msg { return checklistClosedMsg{} }

	case key.Matches(msg, km.MoveUp):
		if len(c.items) > 0 {
			c.cursor = clamp(c.cursor-1, 0, len(c.items)-1)
		}

	case key.Matches(msg, km.MoveDown):
		if len(c.items) > 0 {
			c.cursor = clamp(c.cursor+1, 0, len(c.items)-1)
		}

	case msg.String() == " ", msg.String() == "enter":
		if c.cursor < len(c.items) {
			id := c.items[c.cursor].ID
			return c, func() tea.Msg {
				return checklistActionMsg{kind: checklistToggle, cardID: c.cardID, itemID: id}
			}
		}

	case msg.String() == "a":
		c.mode = checklistAddingItem
		c.newItem.SetValue("")
		c.newItem.Focus()

	case msg.String() == "d":
		if c.cursor < len(c.items) {
			id := c.items[c.cursor].ID
			return c, func() tea.Msg {
				return checklistActionMsg{kind: checklistDelete, cardID: c.cardID, itemID: id}
			}
		}
	}
	return c, nil
}

func (c ChecklistModel) updateAdding(msg tea.KeyMsg, km KeyMap) (ChecklistModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		c.mode = checklistBrowsing
		c.newItem.Blur()
		return c, nil

	case msg.String() == "enter":
		text := strings.TrimSpace(c.newItem.Value())
		if text == "" {
			return c, nil
		}
		c.mode = checklistBrowsing
		c.newItem.Blur()
		return c, func() tea.Msg {
			return checklistActionMsg{kind: checklistAdd, cardID: c.cardID, text: text}
		}
	}

	var cmd tea.Cmd
	c.newItem, cmd = c.newItem.Update(msg)
	return c, cmd
}

// viewBare renders just the checklist's content, with no surrounding box or
// placement — used when embedded inside another screen (CardFormModel).
// The standalone View() below wraps this in the same box+place treatment
// every other top-level modal in this codebase uses.
func (c ChecklistModel) viewBare(focused bool) string {
	var b strings.Builder

	headerStyle := styles.FormLabelStyle
	if focused {
		headerStyle = styles.SectionActiveStyle
	}
	b.WriteString(headerStyle.Render("Checklist") + "\n\n")

	if len(c.items) > 0 {
		done := 0
		for _, it := range c.items {
			if it.Done {
				done++
			}
		}
		b.WriteString(styles.HelpDescStyle.Render(fmt.Sprintf("%d/%d done", done, len(c.items))) + "\n\n")
	} else {
		b.WriteString(styles.CardNoteStyle.Render("No items yet — press a to add one") + "\n")
	}

	for i, it := range c.items {
		mark := "[ ]"
		if it.Done {
			mark = "[x]"
		}
		line := fmt.Sprintf("%s %s", mark, it.Text)
		if i == c.cursor {
			line = styles.HelpKeyStyle.Render("› " + line)
		} else {
			line = styles.FormLabelStyle.Render("  " + line)
		}
		b.WriteString(line + "\n")
	}

	if c.mode == checklistAddingItem {
		b.WriteString("\n" + styles.FormLabelStyle.Render("New item") + "\n")
		b.WriteString(c.newItem.View() + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("enter add  esc cancel"))
	} else {
		b.WriteString("\n" + styles.HelpDescStyle.Render("↑/↓ select  space toggle  a add  d delete  esc close"))
	}

	return b.String()
}

func (c ChecklistModel) View() string {
	box := styles.ModalBoxStyle.Width(50).Render(c.viewBare(true))
	if c.width > 0 && c.height > 0 {
		return lipgloss.Place(c.width, c.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
