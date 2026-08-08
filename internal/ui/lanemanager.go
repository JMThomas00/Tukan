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

type laneManagerSubMode int

const (
	laneManagerBrowsing laneManagerSubMode = iota
	laneManagerNaming
	laneManagerConfirmDelete
)

type laneManagerAction int

const (
	laneManagerCreated laneManagerAction = iota
	laneManagerRenamed
	laneManagerDeleted
	laneManagerReordered
)

// laneManagerDoneMsg is returned for every action the board needs to act
// on — unlike BoardSwitcherModel (which closes on select/rename/delete),
// this modal stays open across each one, the same "stay open across a DB
// round trip" shape checklist/labels already use, since configuring lanes
// is naturally a multi-step session (add a few, rename one, nudge the
// order) rather than a single in-and-out action.
type laneManagerDoneMsg struct {
	action    laneManagerAction
	laneID    int64
	otherID   int64 // laneManagerReordered: the lane swapped with
	name      string
	cancelled bool
}

// LaneManagerModel is the swim-lane configuration modal: add, rename,
// delete, and reorder the current board's lanes.
type LaneManagerModel struct {
	boardID   int64
	lanes     []models.Lane
	cardCount map[int64]int // laneID -> number of cards in it, for the delete-warning text
	cursor    int
	mode      laneManagerSubMode
	renaming  bool // true = naming submode is for rename, false = create
	nameInput textinput.Model
	width     int
	height    int
}

// NewLaneManager creates a lane manager for a board. cardCount is used only
// to word the delete confirmation ("this deletes N card(s)") — it's the
// caller's current in-memory counts, not re-queried here.
func NewLaneManager(boardID int64, lanes []models.Lane, cardCount map[int64]int, w, h int) LaneManagerModel {
	ni := textinput.New()
	ni.Placeholder = "Lane name"
	ni.CharLimit = 40

	return LaneManagerModel{
		boardID:   boardID,
		lanes:     lanes,
		cardCount: cardCount,
		mode:      laneManagerBrowsing,
		nameInput: ni,
		width:     w,
		height:    h,
	}
}

func (m LaneManagerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m LaneManagerModel) Update(msg tea.Msg) (LaneManagerModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	km := DefaultKeyMap

	switch m.mode {
	case laneManagerNaming:
		return m.updateNaming(keyMsg, km)
	case laneManagerConfirmDelete:
		return m.updateConfirmDelete(keyMsg, km)
	default:
		return m.updateBrowsing(keyMsg, km)
	}
}

func (m LaneManagerModel) updateNaming(msg tea.KeyMsg, km KeyMap) (LaneManagerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		m.mode = laneManagerBrowsing
		m.nameInput.Blur()
		return m, nil

	case msg.String() == "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			return m, nil
		}
		action := laneManagerCreated
		var laneID int64
		if m.renaming {
			action = laneManagerRenamed
			laneID = m.lanes[m.cursor].ID
		}
		m.mode = laneManagerBrowsing
		m.nameInput.Blur()
		return m, func() tea.Msg {
			return laneManagerDoneMsg{action: action, laneID: laneID, name: name}
		}
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m LaneManagerModel) updateConfirmDelete(msg tea.KeyMsg, km KeyMap) (LaneManagerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Confirm):
		id := m.lanes[m.cursor].ID
		m.mode = laneManagerBrowsing
		return m, func() tea.Msg {
			return laneManagerDoneMsg{action: laneManagerDeleted, laneID: id}
		}
	default:
		m.mode = laneManagerBrowsing
	}
	return m, nil
}

func (m LaneManagerModel) updateBrowsing(msg tea.KeyMsg, km KeyMap) (LaneManagerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		return m, func() tea.Msg {
			return laneManagerDoneMsg{cancelled: true}
		}

	case key.Matches(msg, km.MoveUp):
		if len(m.lanes) > 0 {
			m.cursor = clamp(m.cursor-1, 0, len(m.lanes)-1)
		}

	case key.Matches(msg, km.MoveDown):
		if len(m.lanes) > 0 {
			m.cursor = clamp(m.cursor+1, 0, len(m.lanes)-1)
		}

	case key.Matches(msg, km.MoveLeft):
		if m.cursor > 0 {
			this, other := m.lanes[m.cursor].ID, m.lanes[m.cursor-1].ID
			m.cursor--
			return m, func() tea.Msg {
				return laneManagerDoneMsg{action: laneManagerReordered, laneID: this, otherID: other}
			}
		}

	case key.Matches(msg, km.MoveRight):
		if m.cursor < len(m.lanes)-1 {
			this, other := m.lanes[m.cursor].ID, m.lanes[m.cursor+1].ID
			m.cursor++
			return m, func() tea.Msg {
				return laneManagerDoneMsg{action: laneManagerReordered, laneID: this, otherID: other}
			}
		}

	case msg.String() == "n":
		m.mode = laneManagerNaming
		m.renaming = false
		m.nameInput.SetValue("")
		m.nameInput.Focus()

	case msg.String() == "r":
		if len(m.lanes) > 0 {
			m.mode = laneManagerNaming
			m.renaming = true
			m.nameInput.SetValue(m.lanes[m.cursor].Name)
			m.nameInput.Focus()
		}

	case msg.String() == "d":
		// Refuse to delete the last remaining lane — a board needs at
		// least one place to put cards.
		if len(m.lanes) > 1 {
			m.mode = laneManagerConfirmDelete
		}
	}
	return m, nil
}

func (m LaneManagerModel) View() string {
	var b strings.Builder
	b.WriteString(styles.FormLabelStyle.Render("Lanes") + "\n\n")

	switch m.mode {
	case laneManagerNaming:
		label := "New lane name"
		if m.renaming {
			label = "Rename lane"
		}
		b.WriteString(styles.FormLabelStyle.Render(label) + "\n")
		b.WriteString(m.nameInput.View() + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("enter save  esc cancel"))

	case laneManagerConfirmDelete:
		name, count := "", 0
		if m.cursor < len(m.lanes) {
			lane := m.lanes[m.cursor]
			name = lane.Name
			count = m.cardCount[lane.ID]
		}
		warning := fmt.Sprintf("Delete lane %q? This deletes %d card(s) in it.", name, count)
		b.WriteString(styles.FormErrorStyle.Render(warning) + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("y confirm  esc cancel"))

	default:
		for i, lane := range m.lanes {
			line := fmt.Sprintf("%s (%d)", lane.Name, m.cardCount[lane.ID])
			if i == m.cursor {
				line = styles.HelpKeyStyle.Render("› " + line)
			} else {
				line = styles.FormLabelStyle.Render("  " + line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + styles.HelpDescStyle.Render("↑/↓ select  ←/→ reorder  n new  r rename  d delete  esc close"))
	}

	box := styles.ModalBoxStyle.Width(50).Render(b.String())
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
