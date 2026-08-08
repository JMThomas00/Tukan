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

type switcherSubMode int

const (
	switcherBrowsing switcherSubMode = iota
	switcherNaming
	switcherConfirmDelete
)

type switcherAction int

const (
	switcherSelect switcherAction = iota
	switcherCreated
	switcherRenamed
	switcherDeleted
)

// boardSwitcherDoneMsg is returned when the switcher closes: either
// cancelled outright, or having made a change the board needs to act on.
type boardSwitcherDoneMsg struct {
	action    switcherAction
	boardID   int64
	name      string
	cancelled bool
}

// BoardSwitcherModel is the board-switching modal: browse, select, create,
// rename, and delete boards.
type BoardSwitcherModel struct {
	boards    []models.Board
	cursor    int
	mode      switcherSubMode
	renaming  bool // true = naming submode is for rename, false = create
	nameInput textinput.Model
	width     int
	height    int
}

// NewBoardSwitcher creates a switcher pre-focused on the currently active board.
func NewBoardSwitcher(boards []models.Board, currentID int64, w, h int) BoardSwitcherModel {
	cursor := 0
	for i, b := range boards {
		if b.ID == currentID {
			cursor = i
			break
		}
	}

	ni := textinput.New()
	ni.Placeholder = "Board name"
	ni.CharLimit = 60

	return BoardSwitcherModel{
		boards:    boards,
		cursor:    cursor,
		mode:      switcherBrowsing,
		nameInput: ni,
		width:     w,
		height:    h,
	}
}

func (s BoardSwitcherModel) Init() tea.Cmd {
	return textinput.Blink
}

func (s BoardSwitcherModel) Update(msg tea.Msg) (BoardSwitcherModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	km := DefaultKeyMap

	switch s.mode {
	case switcherNaming:
		return s.updateNaming(keyMsg, km)
	case switcherConfirmDelete:
		return s.updateConfirmDelete(keyMsg, km)
	default:
		return s.updateBrowsing(keyMsg, km)
	}
}

func (s BoardSwitcherModel) updateNaming(msg tea.KeyMsg, km KeyMap) (BoardSwitcherModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		s.mode = switcherBrowsing
		s.nameInput.Blur()
		return s, nil

	case msg.String() == "enter":
		name := strings.TrimSpace(s.nameInput.Value())
		if name == "" {
			return s, nil
		}
		action := switcherCreated
		var boardID int64
		if s.renaming {
			action = switcherRenamed
			boardID = s.boards[s.cursor].ID
		}
		s.mode = switcherBrowsing
		s.nameInput.Blur()
		return s, func() tea.Msg {
			return boardSwitcherDoneMsg{action: action, boardID: boardID, name: name}
		}
	}

	var cmd tea.Cmd
	s.nameInput, cmd = s.nameInput.Update(msg)
	return s, cmd
}

func (s BoardSwitcherModel) updateConfirmDelete(msg tea.KeyMsg, km KeyMap) (BoardSwitcherModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Confirm):
		id := s.boards[s.cursor].ID
		s.mode = switcherBrowsing
		return s, func() tea.Msg {
			return boardSwitcherDoneMsg{action: switcherDeleted, boardID: id}
		}
	default:
		s.mode = switcherBrowsing
	}
	return s, nil
}

func (s BoardSwitcherModel) updateBrowsing(msg tea.KeyMsg, km KeyMap) (BoardSwitcherModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		return s, func() tea.Msg {
			return boardSwitcherDoneMsg{cancelled: true}
		}

	case key.Matches(msg, km.MoveUp):
		if len(s.boards) > 0 {
			s.cursor = clamp(s.cursor-1, 0, len(s.boards)-1)
		}

	case key.Matches(msg, km.MoveDown):
		if len(s.boards) > 0 {
			s.cursor = clamp(s.cursor+1, 0, len(s.boards)-1)
		}

	case msg.String() == "enter":
		if len(s.boards) > 0 {
			id := s.boards[s.cursor].ID
			return s, func() tea.Msg {
				return boardSwitcherDoneMsg{action: switcherSelect, boardID: id}
			}
		}

	case msg.String() == "n":
		s.mode = switcherNaming
		s.renaming = false
		s.nameInput.SetValue("")
		s.nameInput.Focus()

	case msg.String() == "r":
		if len(s.boards) > 0 {
			s.mode = switcherNaming
			s.renaming = true
			s.nameInput.SetValue(s.boards[s.cursor].Name)
			s.nameInput.Focus()
		}

	case msg.String() == "d":
		// Refuse to delete the last remaining board — there must always be one.
		if len(s.boards) > 1 {
			s.mode = switcherConfirmDelete
		}
	}
	return s, nil
}

func (s BoardSwitcherModel) View() string {
	var b strings.Builder
	b.WriteString(styles.FormLabelStyle.Render("Boards") + "\n\n")

	switch s.mode {
	case switcherNaming:
		label := "New board name"
		if s.renaming {
			label = "Rename board"
		}
		b.WriteString(styles.FormLabelStyle.Render(label) + "\n")
		b.WriteString(s.nameInput.View() + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("enter save  esc cancel"))

	case switcherConfirmDelete:
		name := ""
		if s.cursor < len(s.boards) {
			name = s.boards[s.cursor].Name
		}
		warning := fmt.Sprintf("Delete board %q? This deletes all its lanes and cards.", name)
		b.WriteString(styles.FormErrorStyle.Render(warning) + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("y confirm  esc cancel"))

	default:
		for i, board := range s.boards {
			line := "  " + board.Name
			if i == s.cursor {
				line = styles.HelpKeyStyle.Render("› " + board.Name)
			} else {
				line = styles.FormLabelStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + styles.HelpDescStyle.Render("↑/↓ select  enter open  n new  r rename  d delete  esc close"))
	}

	box := styles.ModalBoxStyle.Width(50).Render(b.String())
	if s.width > 0 && s.height > 0 {
		return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
