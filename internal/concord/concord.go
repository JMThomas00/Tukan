// Package concord runs Tukan boards in Concord channels, on the Concord
// plugin SDK: each channel of Tukan's kind is mapped to one board, and
// everyone viewing the channel gets their own BoardModel (own cursor, own
// open form), all over the same database. When one viewer changes the
// board, the others reload and see it at once.
package concord

import (
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"github.com/JMThomas00/Concord/sdk/pane"
	"github.com/JMThomas00/Concord/sdk/plugin"
	"github.com/JMThomas00/Concord/sdk/wire"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/ui"
)

// Server maps channels to boards and hosts a board per viewer.
type Server struct {
	db        *database.DB
	themeName string
	host      *pane.Host

	mu     sync.Mutex
	boards map[uuid.UUID]int64 // channel → board, as far as this process knows

	conn    *plugin.Conn // set on connect; used only from callbacks
	pending map[uuid.UUID]*pendingNotice
}

// New builds a Server over db; themeName is Tukan's theme for every viewer.
func New(db *database.DB, themeName string) *Server {
	s := &Server{db: db, themeName: themeName, boards: map[uuid.UUID]int64{}, pending: map[uuid.UUID]*pendingNotice{}}
	s.host = pane.NewHost(s.newModel)
	return s
}

// Handler is what plugin.Run needs.
func (s *Server) Handler() plugin.Handler {
	h := s.host.Handler()
	h.OnReady = func(c *plugin.Conn, _ *wire.User) { s.conn = c }
	h.OnChannel = s.onChannel
	h.OnChannelDelete = func(_ *plugin.Conn, e wire.ChannelDeletePayload) {
		s.mu.Lock()
		delete(s.boards, e.ChannelID) // the board itself stays in the database
		s.mu.Unlock()
	}
	return h
}

// onChannel makes sure a channel has its board: Concord sends every
// channel Tukan owns on connect, and each new one as it's created.
func (s *Server) onChannel(_ *plugin.Conn, ch wire.Channel) {
	if _, err := s.boardFor(ch.ID, ch.Name, ch.PluginConfig["board_name"]); err != nil {
		log.Printf("tukan: board for channel %s: %v", ch.ID, err)
	}
}

// boardFor returns channelID's board, creating it (named name, else the
// channel's name) the first time the channel is seen.
func (s *Server) boardFor(channelID uuid.UUID, channelName, name string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.boards[channelID]; ok {
		return id, nil
	}
	id, ok, err := s.db.GetBoardIDForChannel(channelID.String())
	if err != nil {
		return 0, err
	}
	if !ok {
		if channelName == "" && name == "" {
			return 0, fmt.Errorf("channel %s has no board yet", channelID)
		}
		if name == "" {
			name = channelName
		}
		board, err := s.db.CreateBoardForChannel(channelID.String(), name)
		if err != nil {
			return 0, err
		}
		id = board.ID
	}
	s.boards[channelID] = id
	return id, nil
}

// newModel builds a viewer's board.
func (s *Server) newModel(v *pane.Viewer) tea.Model {
	boardID, err := s.boardFor(v.ChannelID, "", "")
	if err != nil {
		return message("This board isn't set up yet. Try again in a moment.")
	}
	b, err := ui.NewBoardForID(s.db, boardID, v.Width, v.Height, s.themeName)
	if err != nil {
		log.Printf("tukan: open board %d for %s: %v", boardID, v.Name, err)
		return message("Couldn't open this board.")
	}
	return &boardModel{board: b, viewer: v.ID, channel: v.ChannelID, changed: s.changed}
}

// changed is called when a viewer's action changed channelID's board: the
// other viewers reload at once, and the activity channel hears about it
// once the burst settles (saving a card and then its labels is one change,
// not two).
func (s *Server) changed(channelID, by uuid.UUID, before, after ui.ContentSnapshot) {
	s.host.Broadcast(channelID, reloadMsg{by: by})
	if s.conn == nil {
		return
	}
	if p, ok := s.pending[channelID]; ok {
		p.after = after
		return
	}
	s.pending[channelID] = &pendingNotice{before: before, after: after}
	conn := s.conn
	time.AfterFunc(notifyDelay, func() {
		conn.Post(func() { // back on the callback goroutine
			p := s.pending[channelID]
			delete(s.pending, channelID)
			if text, ok := notifyText(p.before, p.after); ok {
				_ = conn.Notify(text)
			}
		})
	})
}

// notifyDelay is how long a burst of changes is gathered into one notice.
const notifyDelay = 400 * time.Millisecond

type pendingNotice struct{ before, after ui.ContentSnapshot }

// reloadMsg tells a board another viewer changed it.
type reloadMsg struct{ by uuid.UUID }

// boardModel adapts ui.BoardModel to tea.Model for the pane host, and
// notices when an update changed the board's content.
type boardModel struct {
	board   ui.BoardModel
	viewer  uuid.UUID
	channel uuid.UUID
	changed func(channel, by uuid.UUID, before, after ui.ContentSnapshot)
}

func (m *boardModel) Init() tea.Cmd { return nil }

func (m *boardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.board.SetSize(msg.Width, msg.Height)
		return m, nil
	case reloadMsg:
		if msg.by != m.viewer {
			if err := m.board.Reload(); err != nil {
				log.Printf("tukan: reload: %v", err)
			}
		}
		return m, nil
	case tea.KeyMsg:
		// q on the main view is standalone Tukan's quit: here it hands the
		// keyboard back to Concord. Anywhere else it's typing.
		if msg.String() == "q" && m.board.IsMainView() {
			return m, tea.Quit
		}
	}
	before := m.board.Snapshot()
	b, cmd := m.board.Update(msg)
	m.board = b
	if after := m.board.Snapshot(); !reflect.DeepEqual(before, after) {
		m.changed(m.channel, m.viewer, before, after)
	}
	return m, cmd
}

func (m *boardModel) View() string { return m.board.View() }

// ClaimedKeys (pane.KeyClaimer) passes on which of Esc, Tab and Shift+Tab
// the board needs: the rest of the time they move focus around Concord.
func (m *boardModel) ClaimedKeys() []string { return m.board.ClaimedKeys() }

// message is a model that just shows text.
type message string

func (m message) Init() tea.Cmd                       { return nil }
func (m message) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m message) View() string                        { return lipgloss.NewStyle().Padding(1, 2).Render(string(m)) }

// notifyText describes a change for the activity channel, naming the card
// by its ticket number and title and saying what happened to it:
// "#1 Plugins Help moved to In Progress".
func notifyText(before, after ui.ContentSnapshot) (string, bool) {
	if reflect.DeepEqual(before, after) {
		return "", false
	}
	old := make(map[int64]models.Card, len(before.Cards))
	for _, c := range before.Cards {
		old[c.ID] = c
	}
	now := make(map[int64]models.Card, len(after.Cards))
	for _, c := range after.Cards {
		now[c.ID] = c
	}
	name := func(c models.Card) string { return fmt.Sprintf("#%d %s", c.TicketNo, c.Title) }
	lane := func(s ui.ContentSnapshot, id int64) string {
		if n := s.LaneNames[id]; n != "" {
			return n
		}
		return "another lane"
	}
	for _, c := range after.Cards {
		if _, ok := old[c.ID]; !ok {
			return name(c) + " created in " + lane(after, c.LaneID), true
		}
	}
	for _, c := range before.Cards {
		if _, ok := now[c.ID]; !ok {
			return name(c) + " deleted", true
		}
	}
	for _, c := range after.Cards {
		b := old[c.ID]
		switch {
		case b.LaneID != c.LaneID:
			return name(c) + " moved to " + lane(after, c.LaneID), true
		case b.Title != c.Title:
			return fmt.Sprintf("#%d %s renamed to %s", c.TicketNo, b.Title, c.Title), true
		case b.Note != c.Note:
			return name(c) + ": note edited", true
		case !sameDate(b.DueDate, c.DueDate):
			if c.DueDate == nil {
				return name(c) + ": due date removed", true
			}
			return name(c) + ": due " + c.DueDate.Format("Jan 2"), true
		case !sameDate(b.StartDate, c.StartDate):
			return name(c) + ": start date changed", true
		case !reflect.DeepEqual(before.Assignees[c.ID], after.Assignees[c.ID]):
			return name(c) + ": assignees changed", true
		case !reflect.DeepEqual(before.Labels[c.ID], after.Labels[c.ID]):
			return name(c) + ": labels changed", true
		case !reflect.DeepEqual(before.Checklists[c.ID], after.Checklists[c.ID]):
			done, total := 0, len(after.Checklists[c.ID])
			for _, it := range after.Checklists[c.ID] {
				if it.Done {
					done++
				}
			}
			return fmt.Sprintf("%s: checklist %d/%d", name(c), done, total), true
		case b.Position != c.Position:
			return name(c) + " reordered in " + lane(after, c.LaneID), true
		}
	}
	return "The board was updated", true
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
