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

type labelPickerSubMode int

const (
	labelPickerBrowsing labelPickerSubMode = iota
	labelPickerCreatingLabel
)

type labelPickerAction int

const (
	labelPickerCommit labelPickerAction = iota // ctrl+s: commit the toggled selection
	labelPickerCreate                          // n: create a new label, then close
	labelPickerDelete                          // d: delete the label under the cursor, then close
)

// labelPickerDoneMsg is returned when the picker closes: cancelled (no
// change committed at all), or with an action the board needs to act on.
// labelIDs is always the final locally-toggled selection for the card,
// regardless of action, so a create/delete still commits whatever
// assignment changes were made in the same session.
type labelPickerDoneMsg struct {
	cardID    int64
	action    labelPickerAction
	labelIDs  []int64
	name      string // labelPickerCreate
	color     string // labelPickerCreate
	deleteID  int64  // labelPickerDelete
	cancelled bool
}

// LabelPickerModel lets the user toggle which of a board's labels are
// assigned to a card, and manage the board's label palette (create/delete).
type LabelPickerModel struct {
	cardID    int64
	all       []models.Label
	selected  map[int64]bool
	cursor    int
	mode      labelPickerSubMode
	nameInput textinput.Model
	colorIdx  int
	width     int
	height    int
}

// NewLabelPicker creates a picker for a card, pre-selecting its current labels.
func NewLabelPicker(cardID int64, all []models.Label, current []models.Label, w, h int) LabelPickerModel {
	selected := make(map[int64]bool, len(current))
	for _, l := range current {
		selected[l.ID] = true
	}

	ni := textinput.New()
	ni.Placeholder = "Label name"
	ni.CharLimit = 40

	return LabelPickerModel{
		cardID:    cardID,
		all:       all,
		selected:  selected,
		nameInput: ni,
		width:     w,
		height:    h,
	}
}

func (p LabelPickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (p LabelPickerModel) Update(msg tea.Msg) (LabelPickerModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}
	km := DefaultKeyMap

	if p.mode == labelPickerCreatingLabel {
		return p.updateCreating(keyMsg, km)
	}
	return p.updateBrowsing(keyMsg, km)
}

func (p LabelPickerModel) updateBrowsing(msg tea.KeyMsg, km KeyMap) (LabelPickerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		return p, func() tea.Msg {
			return labelPickerDoneMsg{cardID: p.cardID, cancelled: true}
		}

	case key.Matches(msg, km.Submit):
		return p, func() tea.Msg {
			return labelPickerDoneMsg{cardID: p.cardID, action: labelPickerCommit, labelIDs: p.selectedIDs()}
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
		p.mode = labelPickerCreatingLabel
		p.colorIdx = 0
		p.nameInput.SetValue("")
		p.nameInput.Focus()

	case msg.String() == "d":
		if p.cursor < len(p.all) {
			id := p.all[p.cursor].ID
			return p, func() tea.Msg {
				return labelPickerDoneMsg{
					cardID:   p.cardID,
					action:   labelPickerDelete,
					labelIDs: p.selectedIDs(),
					deleteID: id,
				}
			}
		}
	}
	return p, nil
}

func (p LabelPickerModel) updateCreating(msg tea.KeyMsg, km KeyMap) (LabelPickerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		p.mode = labelPickerBrowsing
		p.nameInput.Blur()
		return p, nil

	case msg.String() == "left":
		p.colorIdx = (p.colorIdx - 1 + len(styles.LabelPalette)) % len(styles.LabelPalette)
		return p, nil

	case msg.String() == "right":
		p.colorIdx = (p.colorIdx + 1) % len(styles.LabelPalette)
		return p, nil

	case msg.String() == "enter":
		name := strings.TrimSpace(p.nameInput.Value())
		if name == "" {
			return p, nil
		}
		color := styles.LabelPalette[p.colorIdx]
		p.mode = labelPickerBrowsing
		p.nameInput.Blur()
		return p, func() tea.Msg {
			return labelPickerDoneMsg{
				cardID:   p.cardID,
				action:   labelPickerCreate,
				labelIDs: p.selectedIDs(),
				name:     name,
				color:    color,
			}
		}
	}

	var cmd tea.Cmd
	p.nameInput, cmd = p.nameInput.Update(msg)
	return p, cmd
}

func (p LabelPickerModel) selectedIDs() []int64 {
	var ids []int64
	for _, l := range p.all {
		if p.selected[l.ID] {
			ids = append(ids, l.ID)
		}
	}
	return ids
}

// viewBare renders just the picker's content, with no surrounding box or
// placement — used when embedded inside another screen (CardFormModel).
// The standalone View() below wraps this in the same box+place treatment
// every other top-level modal in this codebase uses.
func (p LabelPickerModel) viewBare(focused bool) string {
	var b strings.Builder
	headerStyle := styles.FormLabelStyle
	if focused {
		headerStyle = styles.SectionActiveStyle
	}
	b.WriteString(headerStyle.Render("Labels") + "\n\n")

	switch p.mode {
	case labelPickerCreatingLabel:
		b.WriteString(styles.FormLabelStyle.Render("New label name") + "\n")
		b.WriteString(p.nameInput.View() + "\n\n")
		swatch := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.LabelPalette[p.colorIdx])).Render("●●●")
		b.WriteString("Color: " + swatch + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("←/→ color  enter save  esc cancel"))

	default:
		if len(p.all) == 0 {
			b.WriteString(styles.CardNoteStyle.Render("No labels yet — press n to create one") + "\n")
		}
		for i, l := range p.all {
			mark := "[ ]"
			if p.selected[l.ID] {
				mark = "[x]"
			}
			chip := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color)).Render("●")
			line := fmt.Sprintf("%s %s %s", mark, chip, l.Name)
			if i == p.cursor {
				line = styles.HelpKeyStyle.Render("› " + line)
			} else {
				line = styles.FormLabelStyle.Render("  " + line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + styles.HelpDescStyle.Render("↑/↓ select  space toggle  n new  d delete  ctrl+s save  esc cancel"))
	}

	return b.String()
}

func (p LabelPickerModel) View() string {
	box := styles.ModalBoxStyle.Width(50).Render(p.viewBare(true))
	if p.width > 0 && p.height > 0 {
		return lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
