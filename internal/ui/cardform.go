package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

type formMode int

const (
	formCreate formMode = iota
	formEdit
)

// cardFormDoneMsg is returned when the form is submitted or cancelled.
type cardFormDoneMsg struct {
	card      models.Card
	cancelled bool
}

// CardFormModel is the create/edit card modal.
type CardFormModel struct {
	mode     formMode
	card     models.Card
	laneID   int64
	fieldIdx int // 0=title, 1=assignee, 2=note
	title    textinput.Model
	assignee textinput.Model
	note     textarea.Model
	err      string
	width    int
	height   int
}

// NewCardForm creates a blank card form for a given lane.
func NewCardForm(laneID int64, w, h int) CardFormModel {
	return buildForm(formCreate, models.Card{LaneID: laneID}, w, h)
}

// EditCardForm creates a pre-populated card form.
func EditCardForm(card models.Card, w, h int) CardFormModel {
	return buildForm(formEdit, card, w, h)
}

func buildForm(mode formMode, card models.Card, w, h int) CardFormModel {
	ti := textinput.New()
	ti.Placeholder = "Task title"
	ti.CharLimit = 120
	ti.SetValue(card.Title)
	ti.Focus()

	ai := textinput.New()
	ai.Placeholder = "Assigned to"
	ai.CharLimit = 60
	ai.SetValue(card.Assignee)

	ta := textarea.New()
	ta.Placeholder = "Optional note..."
	ta.CharLimit = 500
	ta.SetWidth(40)
	ta.SetHeight(4)
	ta.SetValue(card.Note)
	ta.ShowLineNumbers = false

	return CardFormModel{
		mode:     mode,
		card:     card,
		laneID:   card.LaneID,
		fieldIdx: 0,
		title:    ti,
		assignee: ai,
		note:     ta,
		width:    w,
		height:   h,
	}
}

func (f CardFormModel) Init() tea.Cmd {
	return textinput.Blink
}

func (f CardFormModel) Update(msg tea.Msg) (CardFormModel, tea.Cmd) {
	km := DefaultKeyMap

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, km.Cancel):
			return f, func() tea.Msg {
				return cardFormDoneMsg{cancelled: true}
			}

		case key.Matches(msg, km.Submit):
			return f.submit()

		case key.Matches(msg, km.NextField):
			f = f.advanceField(1)
			return f, nil

		case key.Matches(msg, km.PrevField):
			f = f.advanceField(-1)
			return f, nil
		}
	}

	// Route input to the focused field.
	var cmd tea.Cmd
	switch f.fieldIdx {
	case 0:
		f.title, cmd = f.title.Update(msg)
	case 1:
		f.assignee, cmd = f.assignee.Update(msg)
	case 2:
		f.note, cmd = f.note.Update(msg)
	}
	return f, cmd
}

func (f CardFormModel) submit() (CardFormModel, tea.Cmd) {
	title := strings.TrimSpace(f.title.Value())
	assignee := strings.TrimSpace(f.assignee.Value())

	if title == "" {
		f.err = "Title is required"
		return f, nil
	}
	if assignee == "" {
		f.err = "Assignee is required"
		return f, nil
	}

	f.card.Title = title
	f.card.Assignee = assignee
	f.card.Note = strings.TrimSpace(f.note.Value())
	if f.card.LaneID == 0 {
		f.card.LaneID = f.laneID
	}

	card := f.card
	return f, func() tea.Msg {
		return cardFormDoneMsg{card: card}
	}
}

func (f CardFormModel) advanceField(delta int) CardFormModel {
	f.fieldIdx = (f.fieldIdx + delta + 3) % 3
	f.title.Blur()
	f.assignee.Blur()
	f.note.Blur()
	switch f.fieldIdx {
	case 0:
		f.title.Focus()
	case 1:
		f.assignee.Focus()
	case 2:
		f.note.Focus()
	}
	return f
}

func (f CardFormModel) View() string {
	title := f.mode == formCreate
	heading := "New Card"
	if !title {
		heading = "Edit Card"
	}

	var b strings.Builder
	b.WriteString(styles.FormLabelStyle.Render(heading) + "\n\n")

	// Title field
	label0 := styles.FormLabelStyle.Render("Title")
	if f.fieldIdx == 0 {
		label0 = styles.HelpKeyStyle.Render("Title")
	}
	b.WriteString(label0 + "\n")
	b.WriteString(f.title.View() + "\n\n")

	// Assignee field
	label1 := styles.FormLabelStyle.Render("Assignee")
	if f.fieldIdx == 1 {
		label1 = styles.HelpKeyStyle.Render("Assignee")
	}
	b.WriteString(label1 + "\n")
	b.WriteString(f.assignee.View() + "\n\n")

	// Note field
	label2 := styles.FormLabelStyle.Render("Note (optional)")
	if f.fieldIdx == 2 {
		label2 = styles.HelpKeyStyle.Render("Note (optional)")
	}
	b.WriteString(label2 + "\n")
	b.WriteString(f.note.View() + "\n\n")

	// Error
	if f.err != "" {
		b.WriteString(styles.FormErrorStyle.Render("⚠ "+f.err) + "\n\n")
	}

	// Hints
	b.WriteString(styles.HelpDescStyle.Render("tab next  shift+tab prev  ctrl+s save  esc cancel"))

	box := styles.ModalBoxStyle.Width(50).Render(b.String())

	if f.width > 0 && f.height > 0 {
		return lipgloss.Place(f.width, f.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
