package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/models"
)

// NewBoardForID synchronously loads a specific board by ID — for callers
// that already know which board they want (server mode, resolving a
// Concord channel to its mapped board) rather than NewBoard's own
// "whichever board is first in the file" behavior, which only makes sense
// for the standalone CLI (where there's always just one board on screen at
// a time; server mode's database holds one board per Concord channel, so
// "the first one" is meaningless there). Synchronous rather than returning
// a tea.Cmd: server mode isn't racing a splash screen the way the CLI's own
// startup does, so there's no reason to defer the load.
func NewBoardForID(db *database.DB, boardID int64, width, height int, themeName string) (BoardModel, error) {
	board, err := db.GetBoardByID(boardID)
	if err != nil {
		return BoardModel{}, fmt.Errorf("load board %d: %w", boardID, err)
	}
	st, err := loadBoardState(db, boardID)
	if err != nil {
		return BoardModel{}, fmt.Errorf("load board state for %d: %w", boardID, err)
	}

	fi := textinput.New()
	fi.Placeholder = "search title / assignee / note..."

	b := BoardModel{
		db:               db,
		width:            width,
		height:           height,
		boards:           []models.Board{board},
		currentBoardID:   boardID,
		lanes:            st.lanes,
		cards:            st.cards,
		allAssignees:     st.allAssignees,
		cardAssignees:    st.cardAssignees,
		boardLabels:      st.labels,
		cardLabels:       st.cardLabels,
		checklists:       st.checklists,
		laneScroll:       make([]int, len(st.lanes)),
		filterInput:      fi,
		currentThemeName: themeName,
	}
	b.clampCursor()
	return b, nil
}

// Reload re-fetches this board's full state from the database in place —
// the mechanism that makes multiple viewers on the same Concord channel
// collaborative: each holds its own independent BoardModel (its own
// cursor/mode/form state), so when one viewer's action changes the
// database, every other viewer needs an explicit nudge to notice. Mirrors
// exactly what the lanesReloadedMsg handler in Update() already does
// (reload lanes/cards/assignees/labels/checklists, clampCursor(), no
// cursor/mode reset) but as a direct synchronous call instead of round-
// tripping through the tea.Cmd/tea.Msg machinery, since this is always
// called from outside any Update() cycle — server mode's own event loop
// noticing a mutation happened, not a keypress the model itself triggered.
func (b *BoardModel) Reload() error {
	st, err := loadBoardState(b.db, b.currentBoardID)
	if err != nil {
		return fmt.Errorf("reload board %d: %w", b.currentBoardID, err)
	}
	b.lanes = st.lanes
	b.cards = st.cards
	b.allAssignees = st.allAssignees
	b.cardAssignees = st.cardAssignees
	b.boardLabels = st.labels
	b.cardLabels = st.cardLabels
	b.checklists = st.checklists
	b.laneScroll = make([]int, len(b.lanes))
	b.clampCursor()
	return nil
}

// IsMainView reports whether the board is on its normal top-level view —
// no modal open (card form, board switcher, card-history, theme switcher,
// lane manager) and mode == modeNormal (not mid confirm-delete/move/
// filter-edit) — the exact condition standalone's own 'q' quit binding
// requires (see handleKey's modeNormal case). Server mode uses this to
// decide whether a 'q' keypress means "leave the pane" — safe only here,
// since forwarding 'q' anywhere else would either do nothing useful (it's
// a no-op inside BoardModel.Update, since standalone's real tea.Quit
// handling lives in the real tea.Program runtime server mode doesn't have)
// or, worse, get consumed as an ordinary typed character inside a text
// field.
func (b BoardModel) IsMainView() bool {
	return !b.formActive && !b.switcherActive && !b.cardEventsActive &&
		!b.themeSwitcherActive && !b.laneManagerActive && b.mode == modeNormal
}

// ContentSnapshot is a comparable copy of everything on a board a human
// would consider its "content" — cards plus their assignees, labels, and
// checklist items — independent of any particular viewer's cursor/mode/
// scroll state. Two snapshots taken before and after driving one keypress
// through Update can be diffed to tell "the database actually changed"
// apart from pure navigation, without re-querying the database at all.
type ContentSnapshot struct {
	Cards      []models.Card
	Assignees  map[int64][]models.Assignee
	Labels     map[int64][]models.Label
	Checklists map[int64][]models.ChecklistItem
}

// Snapshot builds a ContentSnapshot from this BoardModel's own in-memory
// state — no database round trip. Added specifically so server mode can
// detect whether an input mutated the board cheaply enough to run on every
// keystroke: the earlier approach (re-querying the database via four list
// calls before and after each keypress) added real, human-perceptible
// input lag, since most keystrokes — ordinary text typing into a form
// field — never touch the database at all.
//
// Every field is deep-copied (fresh slices/maps, not the BoardModel's own)
// deliberately: b.cards/cardAssignees/cardLabels/checklists are maps, and
// Update's mutation handlers write into them in place (e.g.
// `b.cardAssignees[c.ID] = msg.assignees`) rather than replacing the map
// wholesale — a "before" snapshot that aliased those maps directly would
// observe the very mutation it's supposed to be compared against, since
// Go map headers are copied by value but still point at the same
// underlying storage. Two snapshots taken before/after Update() would
// otherwise always look identical.
func (b BoardModel) Snapshot() ContentSnapshot {
	var cards []models.Card
	for _, lane := range b.lanes {
		cards = append(cards, b.cards[lane.ID]...)
	}

	assignees := make(map[int64][]models.Assignee, len(b.cardAssignees))
	for id, a := range b.cardAssignees {
		cp := make([]models.Assignee, len(a))
		copy(cp, a)
		assignees[id] = cp
	}

	labels := make(map[int64][]models.Label, len(b.cardLabels))
	for id, l := range b.cardLabels {
		cp := make([]models.Label, len(l))
		copy(cp, l)
		labels[id] = cp
	}

	checklists := make(map[int64][]models.ChecklistItem, len(b.checklists))
	for id, c := range b.checklists {
		cp := make([]models.ChecklistItem, len(c))
		copy(cp, c)
		checklists[id] = cp
	}

	return ContentSnapshot{Cards: cards, Assignees: assignees, Labels: labels, Checklists: checklists}
}
