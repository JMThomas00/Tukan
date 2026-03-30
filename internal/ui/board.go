package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

// -- async messages ----------------------------------------------------------

type boardLoadedMsg struct {
	lanes []models.Lane
	cards map[int64][]models.Card
}

type cardCreatedMsg models.Card
type cardUpdatedMsg models.Card
type cardDeletedMsg int64
type cardMovedMsg struct {
	card     models.Card
	toLaneID int64
}
type dbErrMsg struct{ err error }

// -- model -------------------------------------------------------------------

// BoardModel is the main Kanban board Bubble Tea model.
type BoardModel struct {
	lanes      []models.Lane
	cards      map[int64][]models.Card
	focusLane  int
	focusCard  int
	mode       boardMode
	form       CardFormModel
	formActive bool
	moving     *models.Card
	laneScroll []int // first visible card index per lane
	laneVPOff  int   // index of leftmost visible lane
	db         *database.DB
	width      int
	height     int
	statusMsg  string
	statusErr  bool
	errTicks   int
}

// NewBoard creates a BoardModel and issues the initial DB load command.
func NewBoard(db *database.DB, width, height int) (BoardModel, tea.Cmd) {
	b := BoardModel{
		db:     db,
		width:  width,
		height: height,
		cards:  make(map[int64][]models.Card),
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
				return b, b.cmdCreateCard(done.card)
			}
			return b, b.cmdUpdateCard(done.card)
		}
		return b, nil
	}

	// If the form is active, delegate all other messages to it.
	if b.formActive {
		return b.updateForm(msg)
	}

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		b.width = msg.Width
		b.height = msg.Height

	case boardLoadedMsg:
		b.lanes = msg.lanes
		b.cards = msg.cards
		b.laneScroll = make([]int, len(b.lanes))
		b.clampCursor()

	case cardCreatedMsg:
		c := models.Card(msg)
		b.cards[c.LaneID] = append(b.cards[c.LaneID], c)
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
	}

	// modeNormal
	switch {
	case key.Matches(msg, km.Quit):
		return b, tea.Quit
	case key.Matches(msg, km.MoveLeft):
		b.moveFocusLane(-1)
	case key.Matches(msg, km.MoveRight):
		b.moveFocusLane(1)
	case key.Matches(msg, km.MoveUp):
		b.moveFocusCard(-1)
	case key.Matches(msg, km.MoveDown):
		b.moveFocusCard(1)
	case key.Matches(msg, km.NewCard):
		if len(b.lanes) > 0 {
			laneID := b.lanes[b.focusLane].ID
			b.form = NewCardForm(laneID, b.width, b.height)
			b.formActive = true
			return b, b.form.Init()
		}
	case key.Matches(msg, km.EditCard):
		card := b.focusedCard()
		if card != nil {
			b.form = EditCardForm(*card, b.width, b.height)
			b.formActive = true
			return b, b.form.Init()
		}
	case key.Matches(msg, km.DeleteCard):
		if b.focusedCard() != nil {
			b.mode = modeConfirmDelete
		}
	case key.Matches(msg, km.MoveCard):
		card := b.focusedCard()
		if card != nil {
			cp := *card
			b.moving = &cp
			b.mode = modeMoving
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

	if len(b.lanes) == 0 {
		return lipgloss.Place(b.width, b.height, lipgloss.Center, lipgloss.Center,
			styles.CardTitleStyle.Render("Loading…"))
	}

	laneStrings := make([]string, 0, len(b.lanes))
	laneWidth := b.laneWidth()

	for i, lane := range b.lanes {
		focused := i == b.focusLane
		laneStrings = append(laneStrings, b.renderLane(i, lane, laneWidth, focused))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, laneStrings...)
	helpBar := RenderHelp(b.mode, b.width)
	statusBar := b.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, board, statusBar, helpBar)
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
	cardRenderHeight = 5 // title + assignee + (note or blank) + borders + margin
	boardChrome      = 4 // status bar + help bar + lane header + borders
)

func (b BoardModel) renderLane(idx int, lane models.Lane, width int, focused bool) string {
	cards := b.cards[lane.ID]

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

	title := styles.CardTitleStyle.Width(width).Render(card.Title)
	assignee := styles.CardAssigneeStyle.Width(width).Render("@" + card.Assignee)

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
		note = styles.CardNoteStyle.Width(width).Render(line)
	}

	content := title + "\n" + assignee
	if note != "" {
		content += "\n" + note
	}

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
	n := len(b.cards[laneID])
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
	n := len(b.cards[laneID])
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
	scroll := b.laneScroll[b.focusLane]
	if b.focusCard < scroll {
		b.laneScroll[b.focusLane] = b.focusCard
	} else if b.focusCard >= scroll+visibleCount {
		b.laneScroll[b.focusLane] = b.focusCard - visibleCount + 1
	}
}

func (b BoardModel) focusedCard() *models.Card {
	if len(b.lanes) == 0 {
		return nil
	}
	laneID := b.lanes[b.focusLane].ID
	lane := b.cards[laneID]
	if len(lane) == 0 || b.focusCard >= len(lane) {
		return nil
	}
	c := lane[b.focusCard]
	return &c
}

// -- async DB commands -------------------------------------------------------

func (b BoardModel) cmdLoad() tea.Cmd {
	db := b.db
	return func() tea.Msg {
		lanes, err := db.ListLanes()
		if err != nil {
			return dbErrMsg{err}
		}
		allCards, err := db.ListAllCards()
		if err != nil {
			return dbErrMsg{err}
		}
		cardMap := make(map[int64][]models.Card)
		for _, l := range lanes {
			cardMap[l.ID] = nil
		}
		for _, c := range allCards {
			cardMap[c.LaneID] = append(cardMap[c.LaneID], c)
		}
		return boardLoadedMsg{lanes: lanes, cards: cardMap}
	}
}

func (b BoardModel) cmdCreateCard(c models.Card) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		created, err := db.CreateCard(c)
		if err != nil {
			return dbErrMsg{err}
		}
		return cardCreatedMsg(created)
	}
}

func (b BoardModel) cmdUpdateCard(c models.Card) tea.Cmd {
	db := b.db
	return func() tea.Msg {
		if err := db.UpdateCard(c); err != nil {
			return dbErrMsg{err}
		}
		return cardUpdatedMsg(c)
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
