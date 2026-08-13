package pluginserver

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/wire"
)

// Server is the top-level orchestrator: one wire.Client connection, routing
// Concord's plugin-pane lifecycle events to the right ChannelBoard by
// ChannelID and pushing rendered frames back. Concord's own process
// supervision is what gives an install exactly one tukan-server running
// this, not per-channel — Server itself fans out to many ChannelBoards from
// that single connection.
type Server struct {
	client    *wire.Client
	db        *database.DB
	pluginID  string
	themeName string

	mu       sync.Mutex
	channels map[string]*ChannelBoard // keyed by Concord channel ID (string form of its UUID)
}

// New constructs a Server. themeName is applied to every viewer's
// BoardModel — the plugin protocol has no per-viewer theme field yet (see
// the integration plan's theme-deferral gap), so every human viewing a
// Tukan pane through Concord currently sees the same theme.
func New(client *wire.Client, db *database.DB, pluginID, themeName string) *Server {
	return &Server{
		client:    client,
		db:        db,
		pluginID:  pluginID,
		themeName: themeName,
		channels:  make(map[string]*ChannelBoard),
	}
}

// Run blocks, dispatching events until the connection closes or fails.
func (s *Server) Run() error {
	return s.client.ReadLoop(s.handle)
}

func (s *Server) handle(msg *wire.Message) {
	var err error
	switch msg.Type {
	case wire.EventChannelCreate:
		err = s.handleChannelCreate(msg.Data)
	case wire.EventChannelUpdate:
		err = s.handleChannelUpdate(msg.Data)
	case wire.EventPluginPaneEnter:
		err = s.handleEnter(msg.Data)
	case wire.EventPluginPaneResize:
		err = s.handleResize(msg.Data)
	case wire.EventPluginPaneInput:
		err = s.handleInput(msg.Data)
	case wire.EventPluginPaneLeave:
		err = s.handleLeave(msg.Data)
	}
	if err != nil {
		log.Printf("pluginserver: handling %s: %v", msg.Type, err)
	}
}

// handleChannelCreate is how a plugin connection learns about a channel at
// all — see the integration plan's gap #1: a plugin's service-account
// connection can't discover pre-existing channels, only ones created while
// it's online. Ignores channels that don't belong to this plugin, and ones
// already known (a duplicate CHANNEL_CREATE, or one this process already
// lazily registered via resolveChannel after a restart).
func (s *Server) handleChannelCreate(data json.RawMessage) error {
	var payload wire.ChannelCreatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal channel create: %w", err)
	}
	if payload.Channel == nil || payload.PluginID != s.pluginID {
		return nil
	}
	channelID := payload.Channel.ID.String()

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.channels[channelID]; exists {
		return nil
	}

	boardName := payload.PluginConfig["board_name"]
	if boardName == "" {
		boardName = payload.Channel.Name
	}
	board, err := s.db.CreateBoardForChannel(channelID, boardName)
	if err != nil {
		return fmt.Errorf("create board for channel %s: %w", channelID, err)
	}
	s.channels[channelID] = newChannelBoard(board.ID)
	return nil
}

// handleChannelUpdate repairs a plugin channel's board mapping when
// resolveChannel can't find it in either this process's memory or Tukan's
// own database — the same "genuinely unknown channel" case
// handleChannelCreate already handles for a brand new channel, just reached
// via Concord's CHANNEL_UPDATE push (a live edit, or — since 2026-08-12 —
// once for every owned channel right after identify/reconnect) instead of
// channel creation. A safe no-op once the mapping already exists either way.
func (s *Server) handleChannelUpdate(data json.RawMessage) error {
	var payload wire.ChannelUpdatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal channel update: %w", err)
	}
	if payload.Channel == nil || payload.PluginID != s.pluginID {
		return nil
	}
	channelID := payload.Channel.ID.String()

	cb, err := s.resolveChannel(channelID)
	if err != nil {
		return err
	}
	if cb != nil {
		return nil
	}

	boardName := payload.PluginConfig["board_name"]
	if boardName == "" {
		boardName = payload.Channel.Name
	}
	board, err := s.db.CreateBoardForChannel(channelID, boardName)
	if err != nil {
		return fmt.Errorf("create board for channel %s: %w", channelID, err)
	}

	s.mu.Lock()
	s.channels[channelID] = newChannelBoard(board.ID)
	s.mu.Unlock()
	return nil
}

// resolveChannel returns the channel's ChannelBoard, lazily registering one
// from the database's channel->board mapping if this process doesn't have
// it in memory yet — the case after a tukan-server restart, where the
// mapping already exists from a past CHANNEL_CREATE but the in-memory
// registry starts empty. Returns a nil *ChannelBoard (no error) for a
// channel that's genuinely unknown to Tukan at all.
func (s *Server) resolveChannel(channelID string) (*ChannelBoard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cb, ok := s.channels[channelID]; ok {
		return cb, nil
	}
	boardID, ok, err := s.db.GetBoardIDForChannel(channelID)
	if err != nil {
		return nil, fmt.Errorf("resolve channel %s: %w", channelID, err)
	}
	if !ok {
		return nil, nil
	}
	cb := newChannelBoard(boardID)
	s.channels[channelID] = cb
	return cb, nil
}

func (s *Server) handleEnter(data json.RawMessage) error {
	var payload wire.PluginPaneEnterPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal pane enter: %w", err)
	}
	channelID := payload.ChannelID.String()
	cb, err := s.resolveChannel(channelID)
	if err != nil {
		return err
	}
	if cb == nil {
		log.Printf("pluginserver: PLUGIN_PANE_ENTER for unknown channel %s, ignoring", channelID)
		return nil
	}
	push, err := cb.Enter(s.db, payload.ViewerID, payload.Width, payload.Height, s.themeName)
	if err != nil {
		return fmt.Errorf("enter viewer %s: %w", payload.ViewerID, err)
	}
	return s.pushFrame(payload.ChannelID, push)
}

func (s *Server) handleResize(data json.RawMessage) error {
	var payload wire.PluginPaneResizePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal pane resize: %w", err)
	}
	channelID := payload.ChannelID.String()
	cb, err := s.resolveChannel(channelID)
	if err != nil {
		return err
	}
	if cb == nil {
		return nil
	}
	push, ok := cb.Resize(payload.ViewerID, payload.Width, payload.Height)
	if !ok {
		return nil // resize for a viewer that already left — nothing to push
	}
	return s.pushFrame(payload.ChannelID, push)
}

// handleInput is the collaborative core: drive the acting viewer's own
// keypress, push their frame, then — if anything actually changed (cheaply
// detected from the viewer's own in-memory state, not a database query) —
// notify and resync every other viewer on the channel so they see it
// without pressing anything themselves.
func (s *Server) handleInput(data json.RawMessage) error {
	var payload wire.PluginPaneInputPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal pane input: %w", err)
	}
	channelID := payload.ChannelID.String()
	cb, err := s.resolveChannel(channelID)
	if err != nil {
		return err
	}
	if cb == nil {
		return nil
	}

	keyMsg := tea.KeyMsg{Type: tea.KeyType(payload.KeyType), Runes: payload.Runes, Alt: payload.Alt}

	// 'q' mirrors standalone Tukan's own quit binding, but only from the
	// board's main view — mid-form it's an ordinary character a user might
	// type (e.g. a card titled "Quick fix"), and driving it through
	// Update() anywhere else is a no-op anyway (tea.Quit only means
	// something to a real tea.Program's runtime, which server mode
	// doesn't have). Handled here, before cb.Input, rather than letting it
	// flow through Update: the pane needs to close on Concord's side, and
	// nothing about that comes back through the normal render/push path.
	if keyMsg.String() == "q" && cb.WantsLeave(payload.ViewerID) {
		return s.sendLeavePane(payload.ChannelID, payload.ViewerID)
	}

	push, before, after, ok := cb.Input(payload.ViewerID, keyMsg)
	if !ok {
		return nil // input for a viewer that already left
	}
	if err := s.pushFrame(payload.ChannelID, push); err != nil {
		return fmt.Errorf("push frame: %w", err)
	}

	if text, changed := notifyText(before, after); changed {
		s.sendNotify(text)
	}

	pushes, err := cb.BroadcastReload(payload.ViewerID)
	if err != nil {
		return fmt.Errorf("broadcast reload: %w", err)
	}
	for _, p := range pushes {
		if err := s.pushFrame(payload.ChannelID, p); err != nil {
			log.Printf("pluginserver: push broadcast frame to viewer %s: %v", p.ViewerID, err)
		}
	}
	return nil
}

func (s *Server) handleLeave(data json.RawMessage) error {
	var payload wire.PluginPaneLeavePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal pane leave: %w", err)
	}
	channelID := payload.ChannelID.String()
	s.mu.Lock()
	cb, ok := s.channels[channelID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	cb.Leave(payload.ViewerID)
	return nil
}

func (s *Server) pushFrame(channelID uuid.UUID, push FramePush) error {
	return s.client.Send(wire.OpPluginPaneFrame, wire.PluginPaneFramePayload{
		ChannelID: channelID,
		ViewerID:  push.ViewerID,
		Frame:     push.Frame,
		Seq:       push.Seq,
	})
}

// sendLeavePane tells the named viewer's Concord client to leave the pane
// for channelID, via the generic OpPluginEvent envelope targeted with
// ViewerID (see HandlePluginEvent on Concord's side, which relays any
// non-"notify" Kind straight to that viewer's own client when ViewerID is
// set — no dedicated opcode needed).
func (s *Server) sendLeavePane(channelID, viewerID uuid.UUID) error {
	payload, err := json.Marshal(wire.PluginPaneClosePayload{ChannelID: channelID})
	if err != nil {
		return fmt.Errorf("marshal leave_pane payload: %w", err)
	}
	return s.client.Send(wire.OpPluginEvent, wire.PluginEventPayload{
		PluginID: s.pluginID,
		Kind:     "leave_pane",
		Payload:  payload,
		ViewerID: viewerID,
	})
}

func (s *Server) sendNotify(text string) {
	payload, err := json.Marshal(wire.PluginNotifyEventPayload{Content: text})
	if err != nil {
		log.Printf("pluginserver: marshal notify payload: %v", err)
		return
	}
	if err := s.client.Send(wire.OpPluginEvent, wire.PluginEventPayload{
		PluginID: s.pluginID,
		Kind:     "notify",
		Payload:  payload,
	}); err != nil {
		log.Printf("pluginserver: send notify event: %v", err)
	}
}
