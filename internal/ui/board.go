package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

// -- async messages ----------------------------------------------------------

type boardLoadedMsg struct {
	boards        []models.Board
	boardID       int64
	lanes         []models.Lane
	cards         map[int64][]models.Card
	allAssignees  []models.Assignee
	cardAssignees map[int64][]models.Assignee
	labels        []models.Label
	cardLabels    map[int64][]models.Label
	checklists    map[int64][]models.ChecklistItem
}

type boardSwitchedMsg struct {
	boardID       int64
	lanes         []models.Lane
	cards         map[int64][]models.Card
	allAssignees  []models.Assignee
	cardAssignees map[int64][]models.Assignee
	labels        []models.Label
	cardLabels    map[int64][]models.Label
	checklists    map[int64][]models.ChecklistItem
}

type boardCreatedMsg struct {
	board models.Board
	lanes []models.Lane
}

type boardRenamedMsg models.Board
type boardDeletedMsg int64

type cardLabelsChangedMsg struct {
	cardID int64
	labels []models.Label
}

type labelsReloadedMsg struct {
	labels     []models.Label
	cardLabels map[int64][]models.Label
}

type cardAssigneesChangedMsg struct {
	cardID    int64
	assignees []models.Assignee
}

// assigneesReloadedMsg is the result of cmdCreateAssignee. justCreated lets
// the handler auto-select the newly registered person in whatever picker
// is still open — unlike labels (which require a separate deliberate
// toggle after creating one), a fresh assignee registration should read as
// "assigned" immediately, matching how a single free-text field used to
// work before it became a registry.
type assigneesReloadedMsg struct {
	assignees   []models.Assignee
	justCreated models.Assignee
}

// lanesReloadedMsg is the result of any lane-manager action (create,
// rename, delete, reorder) — a full board-state reload rather than an
// in-place patch, mirroring cmdCreateLabel/cmdDeleteLabel's own reasoning:
// a lane change can cascade to its cards (delete) or shift what's visible
// in every other lane (reorder), so reloading everything is simpler and
// safer than fine-grained reconciliation. Unlike boardSwitchedMsg, this
// deliberately does NOT reset focusLane/focusCard/laneVPOff to 0 — the user
// is mid-configuration on the board they're already looking at, not
// switching to a different one.
type lanesReloadedMsg struct {
	lanes         []models.Lane
	cards         map[int64][]models.Card
	allAssignees  []models.Assignee
	cardAssignees map[int64][]models.Assignee
	labels        []models.Label
	cardLabels    map[int64][]models.Label
	checklists    map[int64][]models.ChecklistItem
}

type checklistItemAddedMsg struct {
	cardID int64
	item   models.ChecklistItem
}

type checklistItemToggledMsg struct {
	cardID int64
	itemID int64
}

type checklistItemDeletedMsg struct {
	cardID int64
	itemID int64
}

type cardEventsLoadedMsg struct {
	cardID int64
	events []models.CardEvent
}

type cardEventAddedMsg struct {
	cardID int64
	event  models.CardEvent
}

// cardCreatedMsg carries assignees and labels alongside the created card —
// cmdCreateCard resolves any selected during creation (from the unified
// editor's embedded pickers) into the same DB round trip, since the card's
// ID doesn't exist until CreateCard returns, so a separate SetCardAssignees/
// SetCardLabels call can't run concurrently with it. Without this, a card
// created with people or labels already toggled would show nothing until
// the next board reload.
type cardCreatedMsg struct {
	card      models.Card
	assignees []models.Assignee
	labels    []models.Label
}
type cardUpdatedMsg models.Card
type cardDeletedMsg int64
type cardMovedMsg struct {
	card     models.Card
	toLaneID int64
}
type dbErrMsg struct{ err error }

// -- model -------------------------------------------------------------------

// boardViewMode selects which top-level renderer BoardModel.View() uses —
// orthogonal to boardMode (which governs what keys mean within whichever
// view is showing, e.g. modeMoving/modeConfirmDelete apply the same way in
// either view).
type boardViewMode int

const (
	viewKanban boardViewMode = iota
	viewGantt
)

// BoardModel is the main Kanban board Bubble Tea model.
type BoardModel struct {
	boards              []models.Board
	currentBoardID      int64
	lanes               []models.Lane
	cards               map[int64][]models.Card
	allAssignees        []models.Assignee // global registry, not board-scoped
	cardAssignees       map[int64][]models.Assignee
	boardLabels         []models.Label
	cardLabels          map[int64][]models.Label
	checklists          map[int64][]models.ChecklistItem
	focusLane           int
	focusCard           int
	mode                boardMode
	form                CardFormModel
	formActive          bool
	switcher            BoardSwitcherModel
	switcherActive      bool
	cardEvents          CardEventsModel
	cardEventsActive    bool
	filterQuery         string
	filterInput         textinput.Model
	themeSwitcher       ThemeSwitcherModel
	themeSwitcherActive bool
	currentThemeName    string
	laneManager         LaneManagerModel
	laneManagerActive   bool
	viewMode            boardViewMode
	ganttCursor         int // index into the flattened, lane-grouped Gantt row list
	ganttScroll         int // first visible Gantt row index
	ganttDayOffset      int // days the visible timeline window is shifted from its default centering
	moving              *models.Card
	laneScroll          []int // first visible card index per lane
	laneVPOff           int   // index of leftmost visible lane
	db                  *database.DB
	width               int
	height              int
	statusMsg           string
	statusErr           bool
	errTicks            int
}

// NewBoard creates a BoardModel and issues the initial DB load command.
// The per-card maps are pre-initialized (not left nil until boardLoadedMsg
// arrives) purely defensively: real startup always completes cmdLoad before
// any keypress could reach a map write, but a nil map here is a silent
// panic waiting for the next such map — direct construction (e.g. in
// tests) shouldn't have to remember to redo this by hand.
func NewBoard(db *database.DB, width, height int, themeName string) (BoardModel, tea.Cmd) {
	fi := textinput.New()
	fi.Placeholder = "search title / assignee / note..."

	b := BoardModel{
		db:               db,
		width:            width,
		height:           height,
		cards:            make(map[int64][]models.Card),
		cardAssignees:    make(map[int64][]models.Assignee),
		cardLabels:       make(map[int64][]models.Label),
		checklists:       make(map[int64][]models.ChecklistItem),
		filterInput:      fi,
		currentThemeName: themeName,
	}
	return b, b.cmdLoad()
}

func (b BoardModel) Init() tea.Cmd {
	return b.cmdLoad()
}

func (b BoardModel) Update(msg tea.Msg) (BoardModel, tea.Cmd) {
	// cardFormDoneMsg must be handled before the formActive guard —
	// it arrives as a Cmd result while formActive is still true.
	if done, ok := msg.(cardFormDoneMsg); ok {
		b.formActive = false
		if !done.cancelled {
			if b.form.mode == formCreate {
				return b, b.cmdCreateCard(done.card, done.assigneeIDs, done.labelIDs)
			}
			old := b.findCard(done.card.ID)
			if old == nil {
				old = &done.card // shouldn't happen — fall back to a no-op diff
			}
			saveCmd := b.cmdUpdateCard(*old, done.card)
			assigneesCmd := b.cmdSetCardAssignees(done.card.ID, done.assigneeIDs)
			labelsCmd := b.cmdSetCardLabels(done.card.ID, done.labelIDs)
			return b, tea.Batch(saveCmd, assigneesCmd, labelsCmd)
		}
		return b, nil
	}

	// boardSwitcherDoneMsg must be handled before the switcherActive guard,
	// for the identical reason as cardFormDoneMsg above.
	if done, ok := msg.(boardSwitcherDoneMsg); ok {
		b.switcherActive = false
		if done.cancelled {
			return b, nil
		}
		switch done.action {
		case switcherSelect:
			if done.boardID != b.currentBoardID {
				return b, b.cmdSwitchBoard(done.boardID)
			}
		case switcherCreated:
			return b, b.cmdCreateBoard(done.name)
		case switcherRenamed:
			for _, bd := range b.boards {
				if bd.ID == done.boardID {
					bd.Name = done.name
					return b, b.cmdRenameBoard(bd)
				}
			}
		case switcherDeleted:
			return b, b.cmdDeleteBoard(done.boardID)
		}
		return b, nil
	}

	// labelPickerDoneMsg: the label picker only ever lives embedded inside
	// CardFormModel now (see EditCardForm/NewCardForm) — CardFormModel
	// intercepts esc/ctrl+s itself unless the picker has its own
	// create-label sub-mode open, so the picker's own "commit"
	// (labelPickerCommit, its ctrl+s) and "cancelled" (its browsing-mode
	// esc) cases can never fire from an embedded picker; only
	// labelPickerCreate/labelPickerDelete (immediate board-palette
	// mutations, not deferred to the outer save) reach here in practice.
	// Handled without closing anything — CardFormModel stays open.
	if done, ok := msg.(labelPickerDoneMsg); ok {
		switch done.action {
		case labelPickerCreate:
			return b, b.cmdCreateLabel(done.name, done.color)
		case labelPickerDelete:
			return b, b.cmdDeleteLabel(done.deleteID)
		}
		return b, nil
	}

	// assigneePickerDoneMsg mirrors labelPickerDoneMsg's handling above: the
	// picker only ever lives embedded inside CardFormModel, which
	// intercepts esc/ctrl+s itself unless the picker has its own
	// create-assignee sub-mode open, so only a create (an immediate,
	// registry-wide action, not deferred to the outer save) reaches here in
	// practice. Handled without closing anything — CardFormModel stays open.
	if done, ok := msg.(assigneePickerDoneMsg); ok {
		if done.create {
			return b, b.cmdCreateAssignee(done.name)
		}
		return b, nil
	}

	// themeSwitcherDoneMsg must be handled before the themeSwitcherActive
	// guard, for the identical reason as cardFormDoneMsg above. Unlike
	// cancel, a committed theme is already persisted to disk by the
	// switcher itself (ThemeSwitcherModel.Update, the enter case) — this
	// just needs to remember the name for the switcher's cursor-seeding
	// next time it opens.
	if done, ok := msg.(themeSwitcherDoneMsg); ok {
		b.themeSwitcherActive = false
		if !done.cancelled {
			b.currentThemeName = done.themeName
		}
		return b, nil
	}

	// laneManagerDoneMsg must be handled before the laneManagerActive
	// guard, for the identical reason as cardFormDoneMsg above. Unlike the
	// picker doneMsgs, this one DOES sometimes close the modal (cancelled)
	// and sometimes doesn't (every other action) — configuring lanes is
	// naturally a multi-step session (add a few, rename one, nudge the
	// order), so only esc from browsing mode actually closes it.
	if done, ok := msg.(laneManagerDoneMsg); ok {
		if done.cancelled {
			b.laneManagerActive = false
			return b, nil
		}
		switch done.action {
		case laneManagerCreated:
			return b, b.cmdCreateLane(done.name)
		case laneManagerRenamed:
			return b, b.cmdRenameLane(done.laneID, done.name)
		case laneManagerDeleted:
			return b, b.cmdDeleteLane(done.laneID)
		case laneManagerReordered:
			return b, b.cmdSwapLanePositions(done.laneID, done.otherID)
		}
		return b, nil
	}

	// Checklist messages need the same before-the-guard treatment as the
	// doneMsg pattern above, for a related but distinct reason: unlike the
	// other modals, the checklist stays open across each action's DB round
	// trip (every keypress fires its own command instead of batching on
	// close), so its result messages — not just its "closed" message — must
	// be intercepted here too. If any of these fell through to the
	// formActive guard below, they'd be routed into CardFormModel.Update
	// and then ChecklistModel.Update (which only handles tea.KeyMsg) and
	// silently dropped: the action would never patch b.checklists, or the
	// form would never actually close. The checklist now only ever lives
	// embedded inside CardFormModel (see EditCardForm/NewCardForm), so
	// "is the checklist this message is about currently open" is
	// `b.formActive && b.form.checklist.cardID == m.cardID`, not a
	// dedicated top-level checklistActive flag.
	switch m := msg.(type) {
	case checklistActionMsg:
		switch m.kind {
		case checklistAdd:
			return b, b.cmdCreateChecklistItem(m.cardID, m.text)
		case checklistToggle:
			return b, b.cmdToggleChecklistItem(m.cardID, m.itemID)
		case checklistDelete:
			return b, b.cmdDeleteChecklistItem(m.cardID, m.itemID)
		}
		return b, nil

	case checklistClosedMsg:
		// Unreachable in normal operation now that the checklist is only
		// ever embedded (CardFormModel intercepts esc itself unless the
		// checklist's own add-item sub-mode is open, and this message only
		// fires from ChecklistModel's own browsing-mode esc handling) —
		// kept as a harmless no-op rather than removed, since ChecklistModel
		// remains a legitimately standalone-usable, independently tested type.
		return b, nil

	case checklistItemAddedMsg:
		b.checklists[m.cardID] = append(b.checklists[m.cardID], m.item)
		if b.formActive && b.form.checklist.cardID == m.cardID {
			b.form.checklist.items = b.checklists[m.cardID]
		}
		return b, nil

	case checklistItemToggledMsg:
		items := b.checklists[m.cardID]
		for i, it := range items {
			if it.ID == m.itemID {
				items[i].Done = !items[i].Done
				break
			}
		}
		b.checklists[m.cardID] = items
		if b.formActive && b.form.checklist.cardID == m.cardID {
			b.form.checklist.items = items
		}
		return b, nil

	case checklistItemDeletedMsg:
		items := b.checklists[m.cardID]
		for i, it := range items {
			if it.ID == m.itemID {
				items = append(items[:i], items[i+1:]...)
				break
			}
		}
		b.checklists[m.cardID] = items
		if b.formActive && b.form.checklist.cardID == m.cardID {
			b.form.checklist.items = items
		}
		return b, nil

	// labelsReloadedMsg is the result of cmdCreateLabel/cmdDeleteLabel,
	// triggered from the labelPickerDoneMsg handling above — which,
	// unlike the old standalone picker, deliberately does NOT close the
	// editor for a create/delete (only a save does). That means this
	// message can arrive while formActive is still true, so it needs the
	// same before-the-guard interception as the checklist messages above,
	// for the identical reason: otherwise it's routed into
	// CardFormModel.Update -> LabelPickerModel.Update (tea.KeyMsg only)
	// and silently dropped, leaving the board's label state stale.
	case labelsReloadedMsg:
		b.boardLabels = m.labels
		b.cardLabels = m.cardLabels
		if b.formActive {
			// Refresh what the still-open embedded picker can offer to
			// toggle, preserving whatever the user already has selected.
			b.form.labels.all = m.labels
		}
		return b, nil

	// assigneesReloadedMsg is the result of cmdCreateAssignee, triggered
	// from the assigneePickerDoneMsg handling above — same
	// before-the-guard requirement as labelsReloadedMsg, for the identical
	// reason. Unlike a new label, a newly registered assignee is
	// auto-selected in the still-open picker (see assigneesReloadedMsg's
	// own comment for why) rather than requiring a separate toggle.
	case assigneesReloadedMsg:
		b.allAssignees = m.assignees
		if b.formActive {
			b.form.assignees.all = m.assignees
			if m.justCreated.ID != 0 {
				b.form.assignees.selected[m.justCreated.ID] = true
			}
		}
		return b, nil

	// lanesReloadedMsg is the result of any lane-manager action, triggered
	// from the laneManagerDoneMsg handling above — same before-the-guard
	// requirement as labelsReloadedMsg/assigneesReloadedMsg, for the
	// identical reason: it arrives while laneManagerActive is still true
	// for every action except cancel (which already closed the modal
	// before this could fire).
	case lanesReloadedMsg:
		b.lanes = m.lanes
		b.cards = m.cards
		b.allAssignees = m.allAssignees
		b.cardAssignees = m.cardAssignees
		b.boardLabels = m.labels
		b.cardLabels = m.cardLabels
		b.checklists = m.checklists
		b.laneScroll = make([]int, len(b.lanes))
		b.clampCursor()
		if b.laneManagerActive {
			b.laneManager.lanes = m.lanes
			b.laneManager.cardCount = cardCountsByLane(m.cards)
			b.laneManager.cursor = clamp(b.laneManager.cursor, 0, len(m.lanes)-1)
		}
		return b, nil

	// Card-events messages get the same stay-open treatment as checklist's,
	// for the same reason: the thread stays open across each load/compose
	// round trip, so every message it produces — not just its close message
	// — must be caught here before cardEventsActive routes into
	// CardEventsModel.Update (tea.KeyMsg only) and drops it.
	case cardEventsComposeMsg:
		return b, b.cmdAddComment(m.cardID, m.body)

	case cardEventsClosedMsg:
		b.cardEventsActive = false
		return b, nil

	case cardEventsLoadedMsg:
		if b.cardEventsActive && b.cardEvents.cardID == m.cardID {
			b.cardEvents.loading = false
			b.cardEvents.events = m.events
		}
		return b, nil

	case cardEventAddedMsg:
		if b.cardEventsActive && b.cardEvents.cardID == m.cardID {
			b.cardEvents.events = append(b.cardEvents.events, m.event)
		}
		return b, nil
	}

	// If the form is active, delegate all other messages to it.
	if b.formActive {
		return b.updateForm(msg)
	}

	if b.switcherActive {
		return b.updateSwitcher(msg)
	}

	if b.cardEventsActive {
		return b.updateCardEvents(msg)
	}

	if b.themeSwitcherActive {
		return b.updateThemeSwitcher(msg)
	}

	if b.laneManagerActive {
		return b.updateLaneManager(msg)
	}

	switch msg := msg.(type) {

	case boardLoadedMsg:
		b.boards = msg.boards
		b.currentBoardID = msg.boardID
		b.lanes = msg.lanes
		b.cards = msg.cards
		b.allAssignees = msg.allAssignees
		b.cardAssignees = msg.cardAssignees
		b.boardLabels = msg.labels
		b.cardLabels = msg.cardLabels
		b.checklists = msg.checklists
		b.laneScroll = make([]int, len(b.lanes))
		b.clampCursor()

	case boardSwitchedMsg:
		b.currentBoardID = msg.boardID
		b.lanes = msg.lanes
		b.cards = msg.cards
		b.allAssignees = msg.allAssignees
		b.cardAssignees = msg.cardAssignees
		b.boardLabels = msg.labels
		b.cardLabels = msg.cardLabels
		b.checklists = msg.checklists
		b.laneScroll = make([]int, len(b.lanes))
		b.focusLane, b.focusCard, b.laneVPOff = 0, 0, 0
		b.clampCursor()

	case boardCreatedMsg:
		b.boards = append(b.boards, msg.board)
		b.currentBoardID = msg.board.ID
		b.lanes = msg.lanes
		b.cards = make(map[int64][]models.Card)
		for _, l := range msg.lanes {
			b.cards[l.ID] = nil
		}
		// allAssignees is the global registry — unaffected by a new board.
		b.cardAssignees = make(map[int64][]models.Assignee)
		b.boardLabels = nil
		b.cardLabels = make(map[int64][]models.Label)
		b.checklists = make(map[int64][]models.ChecklistItem)
		b.laneScroll = make([]int, len(b.lanes))
		b.focusLane, b.focusCard, b.laneVPOff = 0, 0, 0
		b.clampCursor()

	case boardRenamedMsg:
		renamed := models.Board(msg)
		for i, bd := range b.boards {
			if bd.ID == renamed.ID {
				b.boards[i].Name = renamed.Name
				break
			}
		}

	case boardDeletedMsg:
		id := int64(msg)
		for i, bd := range b.boards {
			if bd.ID == id {
				b.boards = append(b.boards[:i], b.boards[i+1:]...)
				break
			}
		}
		if id == b.currentBoardID && len(b.boards) > 0 {
			return b, b.cmdSwitchBoard(b.boards[0].ID)
		}

	case cardCreatedMsg:
		c := msg.card
		b.cards[c.LaneID] = append(b.cards[c.LaneID], c)
		if len(msg.assignees) > 0 {
			b.cardAssignees[c.ID] = msg.assignees
		}
		if len(msg.labels) > 0 {
			b.cardLabels[c.ID] = msg.labels
		}
		b.clampCursor()

	case cardUpdatedMsg:
		c := models.Card(msg)
		lane := b.cards[c.LaneID]
		for i, existing := range lane {
			if existing.ID == c.ID {
				lane[i] = c
				break
			}
		}
		b.cards[c.LaneID] = lane

	case cardDeletedMsg:
		id := int64(msg)
		for laneID, lane := range b.cards {
			for i, c := range lane {
				if c.ID == id {
					b.cards[laneID] = append(lane[:i], lane[i+1:]...)
					break
				}
			}
		}
		b.clampCursor()

	case cardMovedMsg:
		// Remove from old lane
		oldLaneID := msg.card.LaneID
		for i, c := range b.cards[oldLaneID] {
			if c.ID == msg.card.ID {
				b.cards[oldLaneID] = append(b.cards[oldLaneID][:i], b.cards[oldLaneID][i+1:]...)
				break
			}
		}
		// Append to new lane
		msg.card.LaneID = msg.toLaneID
		b.cards[msg.toLaneID] = append(b.cards[msg.toLaneID], msg.card)
		b.moving = nil
		b.mode = modeNormal
		b.clampCursor()

	case cardAssigneesChangedMsg:
		b.cardAssignees[msg.cardID] = msg.assignees

	case cardLabelsChangedMsg:
		b.cardLabels[msg.cardID] = msg.labels

	case dbErrMsg:
		b.statusMsg = "Error: " + msg.err.Error()
		b.statusErr = true
		b.errTicks = 12 // ~3 seconds at 250ms ticks

	case tea.KeyMsg:
		return b.handleKey(msg)
	}

	return b, nil
}

func (b BoardModel) updateForm(msg tea.Msg) (BoardModel, tea.Cmd) {
	var cmd tea.Cmd
	b.form, cmd = b.form.Update(msg)
	// cardFormDoneMsg will be routed back through the board's Update next tick.
	return b, cmd
}

func (b BoardModel) updateSwitcher(msg tea.Msg) (BoardModel, tea.Cmd) {
	var cmd tea.Cmd
	b.switcher, cmd = b.switcher.Update(msg)
	// boardSwitcherDoneMsg will be routed back through the board's Update next tick.
	return b, cmd
}

func (b BoardModel) updateCardEvents(msg tea.Msg) (BoardModel, tea.Cmd) {
	var cmd tea.Cmd
	b.cardEvents, cmd = b.cardEvents.Update(msg)
	// cardEvents* messages are intercepted before this guard runs.
	return b, cmd
}

func (b BoardModel) updateThemeSwitcher(msg tea.Msg) (BoardModel, tea.Cmd) {
	var cmd tea.Cmd
	b.themeSwitcher, cmd = b.themeSwitcher.Update(msg)
	// themeSwitcherDoneMsg is intercepted before this guard runs.
	return b, cmd
}

func (b BoardModel) updateLaneManager(msg tea.Msg) (BoardModel, tea.Cmd) {
	var cmd tea.Cmd
	b.laneManager, cmd = b.laneManager.Update(msg)
	// laneManagerDoneMsg/lanesReloadedMsg are intercepted before this guard runs.
	return b, cmd
}

func (b BoardModel) handleKey(msg tea.KeyMsg) (BoardModel, tea.Cmd) {
	km := DefaultKeyMap

	switch b.mode {
	case modeConfirmDelete:
		switch {
		case key.Matches(msg, km.Confirm):
			card := b.focusedCard()
			if card == nil {
				b.mode = modeNormal
				return b, nil
			}
			id := card.ID
			b.mode = modeNormal
			return b, b.cmdDeleteCard(id)
		default:
			b.mode = modeNormal
		}
		return b, nil

	case modeMoving:
		switch {
		case key.Matches(msg, km.MoveLeft):
			b.moveFocusLane(-1)
		case key.Matches(msg, km.MoveRight):
			b.moveFocusLane(1)
		case key.Matches(msg, km.DropCard):
			if b.moving != nil && len(b.lanes) > 0 {
				card := *b.moving
				toLaneID := b.lanes[b.focusLane].ID
				return b, b.cmdMoveCard(card, toLaneID)
			}
		case key.Matches(msg, km.Cancel):
			b.moving = nil
			b.mode = modeNormal
		}
		return b, nil

	case modeFilterEdit:
		switch {
		case key.Matches(msg, km.Cancel):
			b.filterQuery = ""
			b.filterInput.SetValue("")
			b.filterInput.Blur()
			b.mode = modeNormal
			b.clampCursor()
			return b, nil
		case msg.String() == "enter":
			b.filterInput.Blur()
			b.mode = modeNormal
			return b, nil
		}
		var cmd tea.Cmd
		b.filterInput, cmd = b.filterInput.Update(msg)
		b.filterQuery = b.filterInput.Value()
		b.clampCursor()
		return b, cmd
	}

	// modeNormal
	switch {
	case key.Matches(msg, km.Quit):
		return b, tea.Quit
	case key.Matches(msg, km.Cancel):
		if b.filterQuery != "" {
			b.filterQuery = ""
			b.filterInput.SetValue("")
			b.clampCursor()
		}
	case key.Matches(msg, km.MoveLeft):
		if b.viewMode == viewGantt {
			b.ganttDayOffset -= 7
		} else {
			b.moveFocusLane(-1)
		}
	case key.Matches(msg, km.MoveRight):
		if b.viewMode == viewGantt {
			b.ganttDayOffset += 7
		} else {
			b.moveFocusLane(1)
		}
	case key.Matches(msg, km.MoveUp):
		if b.viewMode == viewGantt {
			b.ganttMoveCursor(-1)
		} else {
			b.moveFocusCard(-1)
		}
	case key.Matches(msg, km.MoveDown):
		if b.viewMode == viewGantt {
			b.ganttMoveCursor(1)
		} else {
			b.moveFocusCard(1)
		}
	case key.Matches(msg, km.NewCard):
		if laneID := b.newCardLaneID(); laneID != 0 {
			b.form = NewCardForm(laneID, b.allAssignees, b.boardLabels, b.width, b.height)
			b.formActive = true
			return b, b.form.Init()
		}
	case key.Matches(msg, km.EditCard):
		card := b.focusedCard()
		if card != nil {
			b.form = EditCardForm(*card, b.allAssignees, b.cardAssignees[card.ID], b.boardLabels, b.cardLabels[card.ID], b.checklists[card.ID], b.width, b.height)
			b.formActive = true
			return b, b.form.Init()
		}
	case key.Matches(msg, km.DeleteCard):
		if b.focusedCard() != nil {
			b.mode = modeConfirmDelete
		}
	case key.Matches(msg, km.MoveCard):
		// No natural mapping onto Gantt's flat vertical list — disabled
		// there for now rather than inventing a new lane-reassignment UI.
		if b.viewMode != viewGantt {
			card := b.focusedCard()
			if card != nil {
				cp := *card
				b.moving = &cp
				b.mode = modeMoving
			}
		}
	case key.Matches(msg, km.SwitchBoard):
		if len(b.boards) > 0 {
			b.switcher = NewBoardSwitcher(b.boards, b.currentBoardID, b.width, b.height)
			b.switcherActive = true
			return b, b.switcher.Init()
		}
	case key.Matches(msg, km.LabelPicker):
		// L/c open the same unified editor as e — just pre-focused on the
		// relevant section — rather than a separate standalone modal, so
		// there's exactly one code path for editing a card's labels or
		// checklist, not two that have to stay behaviorally identical forever.
		card := b.focusedCard()
		if card != nil {
			b.form = EditCardForm(*card, b.allAssignees, b.cardAssignees[card.ID], b.boardLabels, b.cardLabels[card.ID], b.checklists[card.ID], b.width, b.height)
			b.form.section = sectionLabels
			b.formActive = true
			return b, b.form.Init()
		}
	case key.Matches(msg, km.Checklist):
		card := b.focusedCard()
		if card != nil {
			b.form = EditCardForm(*card, b.allAssignees, b.cardAssignees[card.ID], b.boardLabels, b.cardLabels[card.ID], b.checklists[card.ID], b.width, b.height)
			b.form.section = sectionChecklist
			b.formActive = true
			return b, b.form.Init()
		}
	case key.Matches(msg, km.CardEvents):
		card := b.focusedCard()
		if card != nil {
			b.cardEvents = NewCardEvents(card.ID, card.Title, b.width, b.height)
			b.cardEventsActive = true
			return b, tea.Batch(b.cardEvents.Init(), b.cmdLoadCardEvents(card.ID))
		}
	case key.Matches(msg, km.Filter):
		b.filterInput.SetValue(b.filterQuery)
		b.filterInput.CursorEnd()
		b.filterInput.Focus()
		b.mode = modeFilterEdit
		return b, textinput.Blink
	case key.Matches(msg, km.ThemeSwitcher):
		b.themeSwitcher = NewThemeSwitcher(b.currentThemeName, b.width, b.height)
		b.themeSwitcherActive = true
		return b, b.themeSwitcher.Init()
	case key.Matches(msg, km.LaneManager):
		b.laneManager = NewLaneManager(b.currentBoardID, b.lanes, cardCountsByLane(b.cards), b.width, b.height)
		b.laneManagerActive = true
		return b, b.laneManager.Init()
	case key.Matches(msg, km.GanttView):
		if b.viewMode == viewGantt {
			b.viewMode = viewKanban
		} else {
			b.viewMode = viewGantt
			b.ganttDayOffset = 0
			b.ganttClampCursor() // seeds ganttCursor at the first card row
		}
	}
	return b, nil
}

// View renders the full board.
func (b BoardModel) View() string {
	if b.formActive {
		// Render board behind the form overlay.
		return b.form.View()
	}

	if b.switcherActive {
		return b.switcher.View()
	}

	if b.cardEventsActive {
		return b.cardEvents.View()
	}

	if b.themeSwitcherActive {
		return b.themeSwitcher.View()
	}

	if b.laneManagerActive {
		return b.laneManager.View()
	}

	if len(b.lanes) == 0 {
		return lipgloss.Place(b.width, b.height, lipgloss.Center, lipgloss.Center,
			styles.CardTitleStyle.Render("Loading…"))
	}

	header := b.renderBoardHeader()

	var body string
	if b.viewMode == viewGantt {
		body = b.renderGanttView()
	} else {
		laneStrings := make([]string, 0, len(b.lanes))
		laneWidth := b.laneWidth()

		for i, lane := range b.lanes {
			focused := i == b.focusLane
			laneStrings = append(laneStrings, b.renderLane(i, lane, laneWidth, focused))
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, laneStrings...)
	}

	helpBar := RenderHelp(b.mode, b.viewMode, b.width)
	statusBar := b.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, statusBar, helpBar)
}

// SetSize updates the board's dimensions and cascades to whichever overlay
// exists, so app.go has one call to make on every tea.WindowSizeMsg instead
// of poking each modal's width/height fields individually.
func (b *BoardModel) SetSize(w, h int) {
	b.width = w
	b.height = h
	b.form.width = w
	b.form.height = h
	b.switcher.width = w
	b.switcher.height = h
	b.cardEvents.width = w
	b.cardEvents.height = h
	b.themeSwitcher.width = w
	b.themeSwitcher.height = h
	b.laneManager.width = w
	b.laneManager.height = h
}

// cardCountsByLane reduces a lane-id-keyed card map down to just its counts
// — used to seed/refresh the lane manager's per-lane card counts, which it
// needs only for wording the delete confirmation.
func cardCountsByLane(cards map[int64][]models.Card) map[int64]int {
	out := make(map[int64]int, len(cards))
	for laneID, cs := range cards {
		out[laneID] = len(cs)
	}
	return out
}

// -- rendering helpers -------------------------------------------------------

func (b BoardModel) laneWidth() int {
	if len(b.lanes) == 0 || b.width == 0 {
		return 30
	}
	gutters := len(b.lanes) - 1
	w := (b.width - gutters) / len(b.lanes)
	if w < 18 {
		w = 18
	}
	return w
}

const (
	cardRenderHeight = 6 // title + assignee + (note or blank) + badges + borders + margin
	// boardChrome accounts for every row consumed outside a lane's own
	// *content* area: lipgloss's Style.Height(N) sizes the content region
	// to N rows and adds the border on top of that afterward (confirmed
	// against lipgloss's own Render() — Height alignment runs before
	// applyBorder), so a lane's total rendered height is
	// (b.height-boardChrome) + 2 border rows, not boardChrome's namesake
	// value alone. That +2, plus the 2-line board header above the lanes
	// and the 1-line status bar and 1-line help bar below them, is 6 —
	// boardChrome was briefly (and incorrectly) computed without the
	// border's +2 at all once, which overflowed every lane by 2 rows and
	// pushed the top of the board off-screen; renderBoardHeader's own
	// comment explains why it's safe to treat its height as a fixed
	// constant here rather than measuring it dynamically.
	boardChrome = 6 // lane border (2) + header (2) + status bar (1) + help bar (1)
)

// renderBoardHeader renders the board name and a rollup stats line above
// the lanes. Always exactly 2 lines — boardChrome's row math depends on
// that being a guarantee, not just typical — so unlike the help bar's own
// key-hint text (which is left to word-wrap at narrow widths, a known,
// separate issue), both lines here are manually truncated instead of
// width-wrapped: lipgloss's Width()-driven wrapping would silently turn
// this into 3+ lines for a long board name or a narrow terminal, which
// boardChrome has no way to react to since it's a fixed constant.
func (b BoardModel) renderBoardHeader() string {
	name := b.currentBoardName()
	if b.width > 0 {
		if r := []rune(name); len(r) > b.width {
			name = string(r[:b.width])
		}
	}
	nameLine := styles.FormLabelStyle.Render(name)

	stats := b.boardStatsLine()
	if b.width > 0 {
		if r := []rune(stats); len(r) > b.width {
			stats = string(r[:b.width])
		}
	}
	statsLine := styles.HelpDescStyle.Render(stats)

	return nameLine + "\n" + statsLine
}

func (b BoardModel) currentBoardName() string {
	for _, bd := range b.boards {
		if bd.ID == b.currentBoardID {
			return bd.Name
		}
	}
	return ""
}

// boardStatsLine rolls up counts across every lane on the current board.
// "Due this week" and "overdue" are relative to today, computed once so
// every card's due date is compared against the same instant.
func (b BoardModel) boardStatsLine() string {
	today := startOfToday()
	weekOut := today.AddDate(0, 0, 7)

	total, overdue, dueSoon, unassigned := 0, 0, 0, 0
	for _, cards := range b.cards {
		for _, c := range cards {
			total++
			if c.DueDate != nil {
				switch {
				case c.DueDate.Before(today):
					overdue++
				case c.DueDate.Before(weekOut):
					dueSoon++
				}
			}
			if len(b.cardAssignees[c.ID]) == 0 {
				unassigned++
			}
		}
	}

	return fmt.Sprintf("%d card(s)  ·  %d overdue  ·  %d due this week  ·  %d unassigned", total, overdue, dueSoon, unassigned)
}

func (b BoardModel) renderLane(idx int, lane models.Lane, width int, focused bool) string {
	cards := b.visibleCards(lane.ID)

	// Choose lane header color
	headerColor := styles.ColorAccent
	if lane.Color != "" {
		headerColor = lane.Color
	} else if idx < len(styles.LaneColors) {
		headerColor = styles.LaneColors[idx]
	}

	headerStyle := styles.LaneHeaderStyle.Foreground(lipgloss.Color(headerColor))
	header := headerStyle.Render(fmt.Sprintf("%s (%d)", lane.Name, len(cards)))

	// Visible card area height
	innerWidth := width - 4 // account for border + padding
	cardAreaH := b.height - boardChrome
	if cardAreaH < 1 {
		cardAreaH = 1
	}
	visibleCount := cardAreaH / cardRenderHeight
	if visibleCount < 1 {
		visibleCount = 1
	}

	scroll := 0
	if idx < len(b.laneScroll) {
		scroll = b.laneScroll[idx]
	}

	var cardViews []string
	if len(cards) == 0 {
		placeholder := styles.CardNoteStyle.Width(innerWidth).Render("Press n to add a card")
		cardViews = append(cardViews, placeholder)
	} else {
		end := scroll + visibleCount
		if end > len(cards) {
			end = len(cards)
		}
		for ci := scroll; ci < end; ci++ {
			cardFocused := focused && ci == b.focusCard
			cardViews = append(cardViews, b.renderCard(cards[ci], cardFocused, innerWidth))
		}
	}

	inner := header + "\n" + strings.Join(cardViews, "")

	var laneStyle lipgloss.Style
	if focused {
		laneStyle = styles.LaneFocusedStyle
	} else {
		laneStyle = styles.LaneStyle
	}

	return laneStyle.Width(width).Height(b.height - boardChrome).Render(inner)
}

func (b BoardModel) renderCard(card models.Card, focused bool, width int) string {
	isMoving := b.moving != nil && b.moving.ID == card.ID

	// The card's background for this render pass — every leaf style used
	// below (including nested ones in renderCardBadges) must be colored
	// with this explicitly, or its own embedded ANSI reset will revert to
	// the terminal's default background wherever it appears, not the
	// card's actual (focused/default) highlight. See the comment on
	// renderCardBadges for why this matters more than it might look like.
	bg := styles.ColorSurface
	if focused && !isMoving {
		bg = styles.ColorHighlight
	}

	// No per-field .Width() here — padding to the card's width happens
	// exactly once, in the single outer cardStyle.Width(width).Render(content)
	// call below. Lip Gloss pads every line in a multi-line block to that
	// call's own width using that call's own background; padding each field
	// individually first (the old behavior) baked in the wrong background
	// for any trailing whitespace before the outer style ever ran.
	title := renderCardTitleRow(card.Title, card.TicketNo, width, bg)
	assignee := renderCardAssignees(b.cardAssignees[card.ID], focused, isMoving, bg)

	var note string
	if card.Note != "" {
		// Trim to one line for normal view
		line := card.Note
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		if len(line) > width-2 {
			line = line[:width-2]
		}
		note = noteStyleForCard(focused, isMoving).Render(line)
	}

	content := title + "\n" + assignee
	if note != "" {
		content += "\n" + note
	}
	content += "\n" + renderCardBadges(card, b.cardLabels[card.ID], b.checklists[card.ID], width, bg)

	var cardStyle lipgloss.Style
	switch {
	case isMoving:
		cardStyle = styles.CardMovingStyle
	case focused:
		cardStyle = styles.CardFocusedStyle
	default:
		cardStyle = styles.CardStyle
	}

	if b.mode == modeConfirmDelete && focused {
		cardStyle = cardStyle.BorderForeground(lipgloss.Color(styles.ColorDanger))
	}

	return cardStyle.Width(width).Render(content) + "\n"
}

// renderCardTitleRow renders the title on the left and the card's permanent
// ticket number, right-aligned, on the same line — "#N" pinned to the
// card's right edge rather than tucked in with the other metadata on the
// badges line, since it's meant to read as the card's identity (like a
// Jira/SalesForce ticket key), not just another attribute. The title is
// truncated (matching the note line's own existing truncation convention)
// whenever it's too long to leave room for the ticket badge plus at least
// one space of separation, so the badge never gets pushed onto a wrapped
// second line by width's word-wrap. Every segment — including the padding
// between them — carries an explicit Background(bg), for the same reason
// renderCardBadges' segments do.
func renderCardTitleRow(title string, ticketNo int, width int, bg string) string {
	bgColor := lipgloss.Color(bg)
	ticketText := fmt.Sprintf("#%d", ticketNo)
	ticketWidth := lipgloss.Width(ticketText)

	maxTitleWidth := width - ticketWidth - 1 // at least 1 space of separation
	if maxTitleWidth < 1 {
		maxTitleWidth = 1
	}
	if r := []rune(title); len(r) > maxTitleWidth {
		title = string(r[:maxTitleWidth])
	}

	titleSeg := styles.CardTitleStyle.Background(bgColor).Render(title)
	ticketSeg := styles.CardTicketStyle.Background(bgColor).Render(ticketText)

	gap := width - lipgloss.Width(title) - ticketWidth
	if gap < 1 {
		gap = 1
	}
	spacer := lipgloss.NewStyle().Background(bgColor).Render(strings.Repeat(" ", gap))

	return titleSeg + spacer + ticketSeg
}

// renderCardBadges renders the always-present metadata line beneath a
// card's title/assignee/note. Always rendered, even when there's nothing to
// show, so a card's height never varies based on which optional fields are
// populated. Order: label chips, due-date badge, checklist progress.
//
// Every leaf style constructed here sets an explicit Background(bg) — bg is
// the card's own current background (ColorSurface/ColorHighlight), passed
// in by renderCard. Each Render() call below emits its own ANSI reset at
// its end; without an explicit background, that reset would revert to the
// terminal's default background rather than the card's, which is exactly
// the bug this line used to have (most visible with 2+ label chips, since
// each chip's reset re-broke the background again mid-line). This applies
// just as much to the plain-space separators BETWEEN segments as to the
// segments themselves — a bare " " joined in with strings.Join carries no
// styling of its own, so it renders in the terminal's default background
// too, right in between two otherwise-correctly-colored segments. bgJoin
// below exists specifically so no separator is ever a naked, unstyled string.
func renderCardBadges(card models.Card, labels []models.Label, checklist []models.ChecklistItem, width int, bg string) string {
	var segs []string
	bgColor := lipgloss.Color(bg)

	if len(labels) > 0 {
		maxChips := width / 2
		if maxChips < 1 {
			maxChips = 1
		}
		shown := labels
		extra := 0
		if len(shown) > maxChips {
			extra = len(shown) - maxChips
			shown = shown[:maxChips]
		}
		chips := make([]string, 0, len(shown))
		for _, l := range shown {
			chips = append(chips, lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color)).Background(bgColor).Render("●"))
		}
		chipLine := bgJoin(chips, " ", bgColor)
		if extra > 0 {
			chipLine = bgJoin([]string{chipLine, styles.CardDueDateStyle.Background(bgColor).Render(fmt.Sprintf("+%d", extra))}, " ", bgColor)
		}
		segs = append(segs, chipLine)
	}

	if card.DueDate != nil {
		label := "due " + card.DueDate.Format(dueDateLayout)
		style := styles.CardDueDateStyle
		if card.DueDate.Before(startOfToday()) {
			style = styles.CardOverdueStyle
		}
		segs = append(segs, style.Background(bgColor).Render(label))
	}

	if len(checklist) > 0 {
		done := 0
		for _, it := range checklist {
			if it.Done {
				done++
			}
		}
		segs = append(segs, styles.CardDueDateStyle.Background(bgColor).Render(fmt.Sprintf("[%d/%d]", done, len(checklist))))
	}

	return bgJoin(segs, "  ", bgColor)
}

// renderCardAssignees renders a card's assignee line: each person's name in
// their own registry-derived color (styles.AssigneeColor), so the same
// person reads as the same color on every card and board at a glance. Always
// rendered, even with nobody assigned, so a card's height never varies —
// the same convention renderCardBadges documents for its own line. Every
// leaf style (including the separator, via bgJoin — see renderCardBadges'
// own comment for why that matters) sets an explicit Background(bg) for the
// identical reason renderCardBadges' segments do.
func renderCardAssignees(assignees []models.Assignee, focused, isMoving bool, bg string) string {
	if len(assignees) == 0 {
		return noteStyleForCard(focused, isMoving).Render("unassigned")
	}
	bgColor := lipgloss.Color(bg)
	parts := make([]string, 0, len(assignees))
	for _, a := range assignees {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(styles.AssigneeColor(a.ID))).Background(bgColor).Render("@"+a.Name))
	}
	return bgJoin(parts, " ", bgColor)
}

// bgJoin joins pre-rendered ANSI segments with a separator that's also
// explicitly background-colored, instead of strings.Join's bare separator
// text — see renderCardBadges' comment for why an unstyled separator
// between two independently-Render()'d segments breaks the background.
func bgJoin(segs []string, sep string, bg lipgloss.Color) string {
	if len(segs) == 0 {
		return ""
	}
	styledSep := lipgloss.NewStyle().Background(bg).Render(sep)
	return strings.Join(segs, styledSep)
}

// noteStyleForCard picks CardNoteStyle's muted foreground for the resting
// state, or CardNoteFocusedStyle's full-contrast foreground whenever this
// card is the one actually highlighted — ColorMuted and ColorHighlight are
// both mapped from the same theme field, so staying on CardNoteStyle while
// focused renders the note illegible against its own background. A moving
// card keeps the default (non-highlight) background, so it doesn't need the
// focused variant even while "focused" in cursor terms.
func noteStyleForCard(focused, isMoving bool) lipgloss.Style {
	if focused && !isMoving {
		return styles.CardNoteFocusedStyle
	}
	return styles.CardNoteStyle
}

func startOfToday() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (b BoardModel) renderStatusBar() string {
	if b.statusMsg == "" {
		if b.mode == modeMoving && b.moving != nil {
			msg := fmt.Sprintf("Moving: %q  →  use ←/→ to pick lane, enter to drop", b.moving.Title)
			return styles.StatusBarStyle.Width(b.width).Render(msg)
		}
		if b.mode == modeConfirmDelete {
			msg := "Delete this card? Press y to confirm, esc to cancel"
			return styles.StatusErrStyle.Width(b.width).Render(msg)
		}
		if b.mode == modeFilterEdit {
			msg := "Search: " + b.filterInput.View()
			return styles.StatusBarStyle.Width(b.width).Render(msg)
		}
		if b.filterQuery != "" {
			msg := fmt.Sprintf("filter: %q  (esc to clear)", b.filterQuery)
			return styles.StatusBarStyle.Width(b.width).Render(msg)
		}
		return styles.StatusBarStyle.Width(b.width).Render("")
	}
	st := styles.StatusBarStyle
	if b.statusErr {
		st = styles.StatusErrStyle
	}
	return st.Width(b.width).Render(b.statusMsg)
}

// -- cursor management -------------------------------------------------------

func (b *BoardModel) moveFocusLane(delta int) {
	if len(b.lanes) == 0 {
		return
	}
	b.focusLane = clamp(b.focusLane+delta, 0, len(b.lanes)-1)
	b.clampCard()
}

func (b *BoardModel) moveFocusCard(delta int) {
	if len(b.lanes) == 0 {
		return
	}
	laneID := b.lanes[b.focusLane].ID
	n := len(b.visibleCards(laneID))
	if n == 0 {
		return
	}
	b.focusCard = clamp(b.focusCard+delta, 0, n-1)
	b.adjustScroll()
}

func (b *BoardModel) clampCard() {
	if len(b.lanes) == 0 {
		return
	}
	laneID := b.lanes[b.focusLane].ID
	n := len(b.visibleCards(laneID))
	if n == 0 {
		b.focusCard = 0
		return
	}
	b.focusCard = clamp(b.focusCard, 0, n-1)
	b.adjustScroll()
}

func (b *BoardModel) clampCursor() {
	b.clampLane()
	b.clampCard()
	b.ganttClampCursor()
}

func (b *BoardModel) clampLane() {
	if len(b.lanes) == 0 {
		b.focusLane = 0
		return
	}
	b.focusLane = clamp(b.focusLane, 0, len(b.lanes)-1)
}

func (b *BoardModel) adjustScroll() {
	if b.focusLane >= len(b.laneScroll) {
		return
	}
	cardAreaH := b.height - boardChrome
	visibleCount := cardAreaH / cardRenderHeight
	if visibleCount < 1 {
		visibleCount = 1
	}
	clampScrollToCursor(b.focusCard, visibleCount, &b.laneScroll[b.focusLane])
}

// clampScrollToCursor keeps a scroll offset showing the cursor: scrolls up
// if the cursor moved above the visible window, down if below it. Shared by
// kanban's per-lane adjustScroll and Gantt's single-list ganttAdjustScroll —
// both are the same "keep this index in view within this window size"
// problem, just against different scroll state.
func clampScrollToCursor(cursor, visibleCount int, scroll *int) {
	if cursor < *scroll {
		*scroll = cursor
	} else if cursor >= *scroll+visibleCount {
		*scroll = cursor - visibleCount + 1
	}
}

// focusedCard resolves whichever card the cursor is currently on — in
// kanban that's the (focusLane, focusCard) grid position; in Gantt it's
// ganttCursor's position in the flattened row list. Every handleKey action
// (edit, delete, move, label/checklist shortcuts, history) calls this one
// function regardless of which view is active, so the actual mutation
// logic never needs to know which view triggered it.
func (b BoardModel) focusedCard() *models.Card {
	if b.viewMode == viewGantt {
		rows := b.ganttRows()
		if b.ganttCursor < 0 || b.ganttCursor >= len(rows) || rows[b.ganttCursor].kind != ganttRowCard {
			return nil
		}
		c := rows[b.ganttCursor].card
		return &c
	}
	if len(b.lanes) == 0 {
		return nil
	}
	laneID := b.lanes[b.focusLane].ID
	lane := b.visibleCards(laneID)
	if len(lane) == 0 || b.focusCard >= len(lane) {
		return nil
	}
	c := lane[b.focusCard]
	return &c
}

// visibleCards returns a lane's cards after applying the current filter
// query, if any — the single indirection point every lane-card iteration
// (rendering, cursor movement, focus lookup) goes through, so cursor
// position and what's on screen always agree about what's "visible".
func (b BoardModel) visibleCards(laneID int64) []models.Card {
	all := b.cards[laneID]
	if b.filterQuery == "" {
		return all
	}
	fc := parseFilterQuery(b.filterQuery)
	var out []models.Card
	for _, c := range all {
		if cardMatchesFilter(c, b.cardAssignees[c.ID], fc) {
			out = append(out, c)
		}
	}
	return out
}

// findCard looks up a card by ID across every lane, for callers (like the
// card-edit diff below) that need a card that isn't necessarily focused.
func (b BoardModel) findCard(id int64) *models.Card {
	for _, lane := range b.cards {
		for _, c := range lane {
			if c.ID == id {
				cp := c
				return &cp
			}
		}
	}
	return nil
}

// -- async DB commands -------------------------------------------------------

// boardState bundles everything needed to render a board, fetched together
// so load/switch/create messages all carry a consistent snapshot.
type boardState struct {
	lanes         []models.Lane
	cards         map[int64][]models.Card
	allAssignees  []models.Assignee // global registry, not board-scoped
	cardAssignees map[int64][]models.Assignee
	labels        []models.Label
	cardLabels    map[int64][]models.Label
	checklists    map[int64][]models.ChecklistItem
}

// loadBoardState fetches a board's lanes, cards, assignees, labels, and
// checklists in one shot. The global assignee registry is re-fetched here
// too, even though it isn't board-scoped, purely for convenience — one
// round trip that loads everything BoardModel needs to render, rather than
// threading a separate load through every caller.
func loadBoardState(db *database.DB, boardID int64) (boardState, error) {
	var st boardState

	lanes, err := db.ListLanesByBoard(boardID)
	if err != nil {
		return st, err
	}
	cards, err := db.ListCardsByBoard(boardID)
	if err != nil {
		return st, err
	}
	allAssignees, err := db.ListAssignees()
	if err != nil {
		return st, err
	}
	cardAssignees, err := db.ListAssigneesForBoard(boardID)
	if err != nil {
		return st, err
	}
	labels, err := db.ListLabelsByBoard(boardID)
	if err != nil {
		return st, err
	}
	cardLabels, err := db.ListLabelsForBoard(boardID)
	if err != nil {
		return st, err
	}
	checklists, err := db.ListChecklistItemsForBoard(boardID)
	if err != nil {
		return st, err
	}

	cardMap := make(map[int64][]models.Card)
	for _, l := range lanes {
		cardMap[l.ID] = nil
	}
	for _, c := range cards {
		cardMap[c.LaneID] = append(cardMap[c.LaneID], c)
	}

	st.lanes = lanes
	st.cards = cardMap
	st.allAssignees = allAssignees
	st.cardAssignees = cardAssignees
	st.labels = labels
	st.cardLabels = cardLabels
	st.checklists = checklists
	return st, nil
}

func (b BoardModel) cmdLoad() tea.Cmd {
	db := b.db
	return func() tea.Msg {
		boards, err := db.ListBoards()
		if err != nil {
			return dbErrMsg{err}
		}
		if len(boards) == 0 {
			return dbErrMsg{fmt.Errorf("no boards found")}
		}
		boardID := boards[0].ID
		st, err := loadBoardState(db, boardID)
		if err != nil {
			return dbErrMsg{err}
		}
		return boardLoadedMsg{
			boards: boards, boardID: boardID,
			lanes: st.lanes, cards: st.cards,
			allAssignees: st.allAssignees, cardAssignees: st.cardAssignees,
			labels: st.labels, cardLabels: st.cardLabels,
			checklists: st.checklists,
		}
	}
}

func (b BoardModel) cmdSwitchBoard(boardID int64) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		st, err := loadBoardState(db, boardID)
		if err != nil {
			return dbErrMsg{err}
		}
		return boardSwitchedMsg{
			boardID: boardID,
			lanes:   st.lanes, cards: st.cards,
			allAssignees: st.allAssignees, cardAssignees: st.cardAssignees,
			labels: st.labels, cardLabels: st.cardLabels,
			checklists: st.checklists,
		}
	}
}

func (b BoardModel) cmdCreateBoard(name string) tea.Cmd {
	db := b.db
	position := len(b.boards)
	return func() tea.Msg {
		board, err := db.CreateBoard(name, position)
		if err != nil {
			return dbErrMsg{err}
		}
		if err := db.SeedDefaultLanes(board.ID); err != nil {
			return dbErrMsg{err}
		}
		lanes, err := db.ListLanesByBoard(board.ID)
		if err != nil {
			return dbErrMsg{err}
		}
		return boardCreatedMsg{board: board, lanes: lanes}
	}
}

func (b BoardModel) cmdRenameBoard(board models.Board) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.UpdateBoard(board); err != nil {
			return dbErrMsg{err}
		}
		return boardRenamedMsg(board)
	}
}

func (b BoardModel) cmdDeleteBoard(id int64) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.DeleteBoard(id); err != nil {
			return dbErrMsg{err}
		}
		return boardDeletedMsg(id)
	}
}

// cmdSetCardLabels commits a card's final label assignment. It resolves IDs
// back to full Label values against the board-label snapshot captured at
// call time, so the returned message carries everything renderCardBadges
// needs without a second query.
func (b BoardModel) cmdSetCardLabels(cardID int64, labelIDs []int64) tea.Cmd {
	db := b.db
	all := b.boardLabels
	return func() tea.Msg {
		if err := db.SetCardLabels(cardID, labelIDs); err != nil {
			return dbErrMsg{err}
		}
		want := make(map[int64]bool, len(labelIDs))
		for _, id := range labelIDs {
			want[id] = true
		}
		var labels []models.Label
		for _, l := range all {
			if want[l.ID] {
				labels = append(labels, l)
			}
		}
		return cardLabelsChangedMsg{cardID: cardID, labels: labels}
	}
}

// cmdSetCardAssignees commits a card's final assignee list. It resolves IDs
// back to full Assignee values against the registry snapshot captured at
// call time, mirroring cmdSetCardLabels.
func (b BoardModel) cmdSetCardAssignees(cardID int64, assigneeIDs []int64) tea.Cmd {
	db := b.db
	all := b.allAssignees
	return func() tea.Msg {
		if err := db.SetCardAssignees(cardID, assigneeIDs); err != nil {
			return dbErrMsg{err}
		}
		want := make(map[int64]bool, len(assigneeIDs))
		for _, id := range assigneeIDs {
			want[id] = true
		}
		var assignees []models.Assignee
		for _, a := range all {
			if want[a.ID] {
				assignees = append(assignees, a)
			}
		}
		return cardAssigneesChangedMsg{cardID: cardID, assignees: assignees}
	}
}

// cmdCreateAssignee registers a new person in the global registry and
// reloads it afterward — mirrors cmdCreateLabel, minus a delete
// counterpart (see AssigneePickerModel's own comment for why deletion isn't
// exposed here).
func (b BoardModel) cmdCreateAssignee(name string) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		created, err := db.GetOrCreateAssignee(name)
		if err != nil {
			return dbErrMsg{err}
		}
		all, err := db.ListAssignees()
		if err != nil {
			return dbErrMsg{err}
		}
		return assigneesReloadedMsg{assignees: all, justCreated: created}
	}
}

// cmdCreateLabel and cmdDeleteLabel both reload the board's full label state
// afterward rather than patching in place — a label-definition change can
// affect many cards at once, so a full reload is simpler and safer than
// fine-grained reconciliation.
func (b BoardModel) cmdCreateLabel(name, color string) tea.Cmd {
	db := b.db
	boardID := b.currentBoardID
	return func() tea.Msg {
		if _, err := db.CreateLabel(boardID, name, color); err != nil {
			return dbErrMsg{err}
		}
		labels, cardLabels, err := reloadLabelState(db, boardID)
		if err != nil {
			return dbErrMsg{err}
		}
		return labelsReloadedMsg{labels: labels, cardLabels: cardLabels}
	}
}

func (b BoardModel) cmdDeleteLabel(id int64) tea.Cmd {
	db := b.db
	boardID := b.currentBoardID
	return func() tea.Msg {
		if err := db.DeleteLabel(id); err != nil {
			return dbErrMsg{err}
		}
		labels, cardLabels, err := reloadLabelState(db, boardID)
		if err != nil {
			return dbErrMsg{err}
		}
		return labelsReloadedMsg{labels: labels, cardLabels: cardLabels}
	}
}

func reloadLabelState(db *database.DB, boardID int64) ([]models.Label, map[int64][]models.Label, error) {
	labels, err := db.ListLabelsByBoard(boardID)
	if err != nil {
		return nil, nil, err
	}
	cardLabels, err := db.ListLabelsForBoard(boardID)
	if err != nil {
		return nil, nil, err
	}
	return labels, cardLabels, nil
}

// lanesReloadedCmd runs fn (the lane mutation itself) and, on success,
// reloads the whole board state via loadBoardState — every lane-manager
// command shares this shape, only fn differs.
func lanesReloadedCmd(db *database.DB, boardID int64, fn func() error) tea.Cmd {
	return func() tea.Msg {
		if err := fn(); err != nil {
			return dbErrMsg{err}
		}
		st, err := loadBoardState(db, boardID)
		if err != nil {
			return dbErrMsg{err}
		}
		return lanesReloadedMsg{
			lanes: st.lanes, cards: st.cards,
			allAssignees: st.allAssignees, cardAssignees: st.cardAssignees,
			labels: st.labels, cardLabels: st.cardLabels,
			checklists: st.checklists,
		}
	}
}

func (b BoardModel) cmdCreateLane(name string) tea.Cmd {
	db := b.db
	boardID := b.currentBoardID
	position := len(b.lanes)
	return lanesReloadedCmd(db, boardID, func() error {
		_, err := db.CreateLane(boardID, name, position)
		return err
	})
}

func (b BoardModel) cmdRenameLane(id int64, name string) tea.Cmd {
	db := b.db
	boardID := b.currentBoardID
	var color string
	for _, l := range b.lanes {
		if l.ID == id {
			color = l.Color
			break
		}
	}
	return lanesReloadedCmd(db, boardID, func() error {
		return db.UpdateLane(models.Lane{ID: id, Name: name, Color: color})
	})
}

func (b BoardModel) cmdDeleteLane(id int64) tea.Cmd {
	db := b.db
	boardID := b.currentBoardID
	return lanesReloadedCmd(db, boardID, func() error {
		return db.DeleteLane(id)
	})
}

func (b BoardModel) cmdSwapLanePositions(id1, id2 int64) tea.Cmd {
	db := b.db
	boardID := b.currentBoardID
	return lanesReloadedCmd(db, boardID, func() error {
		return db.SwapLanePositions(id1, id2)
	})
}

func (b BoardModel) cmdCreateChecklistItem(cardID int64, text string) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		item, err := db.CreateChecklistItem(cardID, text)
		if err != nil {
			return dbErrMsg{err}
		}
		return checklistItemAddedMsg{cardID: cardID, item: item}
	}
}

func (b BoardModel) cmdToggleChecklistItem(cardID, itemID int64) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.ToggleChecklistItem(itemID); err != nil {
			return dbErrMsg{err}
		}
		return checklistItemToggledMsg{cardID: cardID, itemID: itemID}
	}
}

func (b BoardModel) cmdDeleteChecklistItem(cardID, itemID int64) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.DeleteChecklistItem(itemID); err != nil {
			return dbErrMsg{err}
		}
		return checklistItemDeletedMsg{cardID: cardID, itemID: itemID}
	}
}

func (b BoardModel) cmdLoadCardEvents(cardID int64) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		events, err := db.ListCardEvents(cardID)
		if err != nil {
			return dbErrMsg{err}
		}
		return cardEventsLoadedMsg{cardID: cardID, events: events}
	}
}

func (b BoardModel) cmdAddComment(cardID int64, body string) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		event, err := db.AddComment(cardID, nil, body)
		if err != nil {
			return dbErrMsg{err}
		}
		return cardEventAddedMsg{cardID: cardID, event: event}
	}
}

// cmdCreateCard creates a card and, if assigneeIDs/labelIDs are non-empty,
// assigns them in the same goroutine right after — the card's ID doesn't
// exist until CreateCard returns, so SetCardAssignees/SetCardLabels can't
// be batched as separate concurrent tea.Cmds the way an edit-mode save can.
func (b BoardModel) cmdCreateCard(c models.Card, assigneeIDs []int64, labelIDs []int64) tea.Cmd {
	db := b.db
	allAssignees := b.allAssignees
	allLabels := b.boardLabels
	return func() tea.Msg {
		created, err := db.CreateCard(c)
		if err != nil {
			return dbErrMsg{err}
		}

		var assignees []models.Assignee
		if len(assigneeIDs) > 0 {
			if err := db.SetCardAssignees(created.ID, assigneeIDs); err != nil {
				return dbErrMsg{err}
			}
			want := make(map[int64]bool, len(assigneeIDs))
			for _, id := range assigneeIDs {
				want[id] = true
			}
			for _, a := range allAssignees {
				if want[a.ID] {
					assignees = append(assignees, a)
				}
			}
		}

		var labels []models.Label
		if len(labelIDs) > 0 {
			if err := db.SetCardLabels(created.ID, labelIDs); err != nil {
				return dbErrMsg{err}
			}
			want := make(map[int64]bool, len(labelIDs))
			for _, id := range labelIDs {
				want[id] = true
			}
			for _, l := range allLabels {
				if want[l.ID] {
					labels = append(labels, l)
				}
			}
		}

		return cardCreatedMsg{card: created, assignees: assignees, labels: labels}
	}
}

func (b BoardModel) cmdUpdateCard(old, new models.Card) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.UpdateCard(old, new); err != nil {
			return dbErrMsg{err}
		}
		return cardUpdatedMsg(new)
	}
}

func (b BoardModel) cmdDeleteCard(id int64) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.DeleteCard(id); err != nil {
			return dbErrMsg{err}
		}
		return cardDeletedMsg(id)
	}
}

func (b BoardModel) cmdMoveCard(card models.Card, toLaneID int64) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.MoveCard(card.ID, toLaneID); err != nil {
			return dbErrMsg{err}
		}
		return cardMovedMsg{card: card, toLaneID: toLaneID}
	}
}

// -- utilities ---------------------------------------------------------------

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
