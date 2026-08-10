package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

// dueDateLayout is the accepted/displayed format for the due-date field.
const dueDateLayout = "2006-01-02"

type formMode int

const (
	formCreate formMode = iota
	formEdit
)

// Section indices for CardFormModel.section. sectionTitle..sectionNote are
// the left column's plain textinput/textarea fields; sectionAssignees..
// sectionChecklist are the right column's embedded pickers — grouping them
// this way (rather than interleaving) is what makes tab order match reading
// order (top-to-bottom, left column then right) once the layout is a real
// two-column grid instead of one stacked list.
const (
	sectionTitle = iota
	sectionStartDate
	sectionDueDate
	sectionNote
	sectionAssignees
	sectionLabels
	sectionChecklist
	sectionCount // number of sections, for the cycle modulus
)

// cardFormDoneMsg is returned when the form is submitted or cancelled.
// assigneeIDs/labelIDs are the embedded pickers' final toggled selections
// at save time — committing them is deferred to the outer save (unlike
// checklist mutations, which fire immediately as they happen) since both
// use the same batch-on-close semantics they always have, just scoped to
// the whole editor's close now instead of each picker's own.
type cardFormDoneMsg struct {
	card        models.Card
	assigneeIDs []int64
	labelIDs    []int64
	cancelled   bool
}

// CardFormModel is the unified create/edit card screen: title/note/due-date
// plus embedded assignee, label, and checklist management, replacing what
// used to be three separate modals (this form, plus LabelPickerModel and
// ChecklistModel opened standalone from the board view). All three pickers
// remain real, independently usable, independently tested types —
// CardFormModel is just another caller of them now, nested one level
// deeper than BoardModel used to call them directly.
type CardFormModel struct {
	mode      formMode
	card      models.Card
	laneID    int64
	section   int // sectionTitle..sectionChecklist
	title     textinput.Model
	note      textarea.Model
	startDate textinput.Model
	dueDate   textinput.Model
	assignees AssigneePickerModel
	labels    LabelPickerModel
	checklist ChecklistModel
	err       string
	width     int
	height    int
}

// NewCardForm creates a blank card form for a given lane. allAssignees and
// boardLabels are the global assignee registry and the board's label
// palette, so the embedded pickers have something to offer even though a
// not-yet-created card has none of its own yet.
func NewCardForm(laneID int64, allAssignees []models.Assignee, boardLabels []models.Label, w, h int) CardFormModel {
	return buildForm(formCreate, models.Card{LaneID: laneID}, allAssignees, nil, boardLabels, nil, nil, w, h)
}

// EditCardForm creates a pre-populated card form.
func EditCardForm(card models.Card, allAssignees []models.Assignee, cardAssignees []models.Assignee, boardLabels []models.Label, cardLabels []models.Label, checklistItems []models.ChecklistItem, w, h int) CardFormModel {
	return buildForm(formEdit, card, allAssignees, cardAssignees, boardLabels, cardLabels, checklistItems, w, h)
}

func buildForm(mode formMode, card models.Card, allAssignees []models.Assignee, cardAssignees []models.Assignee, boardLabels []models.Label, cardLabels []models.Label, checklistItems []models.ChecklistItem, w, h int) CardFormModel {
	ti := textinput.New()
	ti.Placeholder = "Task title"
	ti.CharLimit = 120
	ti.SetValue(card.Title)
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = "Optional note..."
	ta.CharLimit = 500
	ta.SetValue(card.Note)
	ta.ShowLineNumbers = false

	si := textinput.New()
	si.Placeholder = "YYYY-MM-DD (optional)"
	si.CharLimit = 10
	if card.StartDate != nil {
		si.SetValue(card.StartDate.Format(dueDateLayout))
	}

	di := textinput.New()
	di.Placeholder = "YYYY-MM-DD (optional)"
	di.CharLimit = 10
	if card.DueDate != nil {
		di.SetValue(card.DueDate.Format(dueDateLayout))
	}

	return CardFormModel{
		mode:      mode,
		card:      card,
		laneID:    card.LaneID,
		section:   sectionTitle,
		title:     ti,
		note:      ta,
		startDate: si,
		dueDate:   di,
		assignees: NewAssigneePicker(card.ID, allAssignees, cardAssignees, w, h),
		labels:    NewLabelPicker(card.ID, boardLabels, cardLabels, w, h),
		checklist: NewChecklist(card.ID, checklistItems, w, h),
		width:     w,
		height:    h,
	}
}

func (f CardFormModel) Init() tea.Cmd {
	return textinput.Blink
}

func (f CardFormModel) Update(msg tea.Msg) (CardFormModel, tea.Cmd) {
	km := DefaultKeyMap

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// If the focused section has its own open sub-mode (creating a new
		// label, adding a checklist item), esc belongs to that sub-mode —
		// it should back out one level there, not close the whole editor.
		// Forward the key down and let the sub-model's own Cancel handling
		// take it from there, exactly as it does when used standalone.
		inSubMode := (f.section == sectionAssignees && f.assignees.mode == assigneePickerCreatingAssignee) ||
			(f.section == sectionLabels && f.labels.mode == labelPickerCreatingLabel) ||
			(f.section == sectionChecklist && f.checklist.mode == checklistAddingItem)

		switch {
		case key.Matches(keyMsg, km.Cancel) && !inSubMode:
			return f, func() tea.Msg {
				return cardFormDoneMsg{cancelled: true}
			}

		case key.Matches(keyMsg, km.Submit):
			// ctrl+s always saves the whole card now, regardless of which
			// section has focus — neither embedded sub-model binds it, so
			// this never shadows a sub-mode's own action.
			return f.submit()

		case key.Matches(keyMsg, km.NextField):
			f = f.advanceSection(1)
			return f, nil

		case key.Matches(keyMsg, km.PrevField):
			f = f.advanceSection(-1)
			return f, nil
		}
	}

	// Route input to the focused section.
	var cmd tea.Cmd
	switch f.section {
	case sectionTitle:
		f.title, cmd = f.title.Update(msg)
	case sectionNote:
		f.note, cmd = f.note.Update(msg)
	case sectionStartDate:
		f.startDate, cmd = f.startDate.Update(msg)
	case sectionDueDate:
		f.dueDate, cmd = f.dueDate.Update(msg)
	case sectionAssignees:
		f.assignees, cmd = f.assignees.Update(msg)
	case sectionLabels:
		f.labels, cmd = f.labels.Update(msg)
	case sectionChecklist:
		f.checklist, cmd = f.checklist.Update(msg)
	}
	return f, cmd
}

func (f CardFormModel) submit() (CardFormModel, tea.Cmd) {
	title := strings.TrimSpace(f.title.Value())
	startRaw := strings.TrimSpace(f.startDate.Value())
	dueRaw := strings.TrimSpace(f.dueDate.Value())

	if title == "" {
		f.err = "Title is required"
		return f, nil
	}

	var start *time.Time
	if startRaw != "" {
		t, err := time.Parse(dueDateLayout, startRaw)
		if err != nil {
			f.err = "Invalid start date, use YYYY-MM-DD"
			return f, nil
		}
		start = &t
	}

	var due *time.Time
	if dueRaw != "" {
		t, err := time.Parse(dueDateLayout, dueRaw)
		if err != nil {
			f.err = "Invalid due date, use YYYY-MM-DD"
			return f, nil
		}
		due = &t
	}

	if start != nil && due != nil && start.After(*due) {
		f.err = "Start date must be before the due date"
		return f, nil
	}

	f.card.Title = title
	f.card.Note = strings.TrimSpace(f.note.Value())
	f.card.StartDate = start
	f.card.DueDate = due
	if f.card.LaneID == 0 {
		f.card.LaneID = f.laneID
	}

	card := f.card
	assigneeIDs := f.assignees.selectedIDs()
	labelIDs := f.labels.selectedIDs()
	return f, func() tea.Msg {
		return cardFormDoneMsg{card: card, assigneeIDs: assigneeIDs, labelIDs: labelIDs}
	}
}

func (f CardFormModel) advanceSection(delta int) CardFormModel {
	f.section = (f.section + delta + sectionCount) % sectionCount
	if f.mode == formCreate && f.section == sectionChecklist {
		// Checklist items need a real card ID to save against
		// (checklist_items.card_id is a foreign key) — a not-yet-created
		// card has none, so a checklist add here would always fail
		// silently (the resulting dbErrMsg sets the board's status bar,
		// which isn't visible behind this modal). Skip past it exactly
		// like Tab would skip a disabled field; it's reachable again once
		// the card exists (EditCardForm).
		f.section = (f.section + delta + sectionCount) % sectionCount
	}
	f.title.Blur()
	f.note.Blur()
	f.startDate.Blur()
	f.dueDate.Blur()
	switch f.section {
	case sectionTitle:
		f.title.Focus()
	case sectionNote:
		f.note.Focus()
	case sectionStartDate:
		f.startDate.Focus()
	case sectionDueDate:
		f.dueDate.Focus()
	}
	return f
}

// boxWidth/boxHeight size the editor proportionally to the terminal instead
// of the old fixed Width(50), which left most of a normal terminal unused.
// Height is enforced as a minimum (Lip Gloss pads, never truncates) so the
// editor reads as genuinely expanded even for a sparse card with no labels
// or checklist yet, not just wide.
func (f CardFormModel) boxWidth() int {
	w := f.width * 4 / 5
	if w < 60 {
		w = 60
	}
	if w > 120 {
		w = 120
	}
	return w
}

func (f CardFormModel) boxHeight() int {
	h := f.height * 3 / 4
	if h < 20 {
		h = 20
	}
	if h > 40 {
		h = 40
	}
	return h
}

func (f CardFormModel) View() string {
	heading := "New Card"
	if f.mode == formEdit {
		heading = "Edit Card"
	}

	label := func(text string, section int) string {
		if f.section == section {
			return styles.SectionActiveStyle.Render(text)
		}
		return styles.FormLabelStyle.Render(text)
	}

	colWidth := (f.boxWidth() - 6) / 2
	if colWidth < 20 {
		colWidth = 20
	}

	// Everything above the note box, in one block so its rendered height can
	// be measured — that's what lets the note textarea claim exactly
	// whatever vertical space is left in the column instead of a fixed
	// height that wastes most of a full-size terminal. The right column is
	// the three "pick from a registry" sections — assignees, labels,
	// checklist — grouped together since they share that shape and none of
	// them are plain single-line fields the way title/due-date are.
	var top strings.Builder
	top.WriteString(label("Title", sectionTitle) + "\n")
	top.WriteString(f.title.View() + "\n\n")
	top.WriteString(label("Start Date", sectionStartDate) + "\n")
	top.WriteString(f.startDate.View() + "\n\n")
	top.WriteString(label("Due Date", sectionDueDate) + "\n")
	top.WriteString(f.dueDate.View() + "\n\n")
	top.WriteString(label("Note (optional)", sectionNote))

	checklistView := f.checklist.viewBare(f.section == sectionChecklist)
	if f.mode == formCreate {
		checklistView = styles.FormLabelStyle.Render("Checklist") + "\n\n" +
			styles.CardNoteStyle.Render("Save the card first to add checklist items")
	}
	right := lipgloss.JoinVertical(lipgloss.Left,
		f.assignees.viewBare(f.section == sectionAssignees), "",
		f.labels.viewBare(f.section == sectionLabels), "",
		checklistView)

	footer := styles.HelpDescStyle.Render("tab next section  shift+tab prev  ctrl+s save  esc cancel")

	var errBlock string
	errLines := 0
	if f.err != "" {
		errBlock = styles.FormErrorStyle.Render("⚠ " + f.err)
		errLines = lipgloss.Height(errBlock) + 1 // +1 for the blank line after it
	}

	// interior = the box's content budget once ModalBoxStyle's vertical
	// padding (2 rows) is subtracted. The border does NOT come out of this
	// budget too, despite being part of the same style — lipgloss applies
	// Height() to the padded content and only adds the border afterward
	// (confirmed against lipgloss's own Render()), so boxHeight() itself
	// already excludes it; double-subtracting it here would just shrink
	// the body 2 rows short of what's actually available. reserved =
	// everything in the box besides the two-column body (heading + its
	// blank line, the optional error block, the blank line before the
	// footer, and the footer itself) — whatever's left goes to the body,
	// which is what lets it — and the note textarea inside it — actually
	// fill the space instead of floating in a box far taller than its content.
	interior := f.boxHeight() - 2
	reserved := 2 + errLines + 1 + lipgloss.Height(footer)
	bodyHeight := interior - reserved
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	noteHeight := bodyHeight - lipgloss.Height(top.String())
	if noteHeight < 3 {
		noteHeight = 3
	}
	f.note.SetWidth(colWidth - 2)
	f.note.SetHeight(noteHeight)

	left := top.String() + "\n" + f.note.View()

	leftCol := lipgloss.NewStyle().Width(colWidth).Height(bodyHeight).Render(left)
	rightCol := lipgloss.NewStyle().Width(colWidth).Height(bodyHeight).Render(right)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol)

	var b strings.Builder
	b.WriteString(styles.FormLabelStyle.Render(heading) + "\n\n")
	b.WriteString(body + "\n\n")
	if errBlock != "" {
		b.WriteString(errBlock + "\n\n")
	}
	b.WriteString(footer)

	box := styles.ModalBoxStyle.Width(f.boxWidth()).Height(f.boxHeight()).Render(b.String())

	if f.width > 0 && f.height > 0 {
		return lipgloss.Place(f.width, f.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
