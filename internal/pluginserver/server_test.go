package pluginserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/wire"
)

// openPluginServerTestDB is for tests that only need a migrated database —
// no board/lane seeding beyond what Migrate() itself seeds — because the
// scenario under test is about channel->board mapping, not board content.
func openPluginServerTestDB(t *testing.T) *database.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// newConcordStub stands in for Concord's server: accepts one WebSocket
// connection, completes the identify handshake, and hands back the raw
// conn so the test can read what Tukan pushes. Mirrors the pattern already
// used in internal/wire/client_test.go — there's no live Concord instance
// reachable from this environment to test against.
func newConcordStub(t *testing.T) (*wire.Client, *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		if _, _, err := conn.ReadMessage(); err != nil { // identify
			return
		}
		ready, _ := wire.NewMessage(wire.OpReady, nil)
		raw, _ := json.Marshal(ready)
		if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
			return
		}
		connCh <- conn
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, err := wire.Dial(wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Identify("token"); err != nil {
		t.Fatalf("identify: %v", err)
	}

	var conn *websocket.Conn
	select {
	case conn = <-connCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stub to accept connection")
	}
	return client, conn
}

// drainMessages reads whatever the server has already pushed to the stub,
// stopping once no message arrives within timeout. The messages are
// written synchronously (client.Send blocks on the TCP write) before
// handleInput/handleEnter return, so by the time this is called everything
// is already sitting in the socket buffer — timeout only needs to cover
// scheduling jitter, not real waiting.
func drainMessages(t *testing.T, conn *websocket.Conn, n int, timeout time.Duration) []wire.Message {
	t.Helper()
	var msgs []wire.Message
	deadline := time.Now().Add(timeout)
	for len(msgs) < n {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message %d/%d: %v", len(msgs)+1, n, err)
		}
		var m wire.Message
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal stub-received message: %v", err)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func mustEnter(t *testing.T, srv *Server, channelID, viewerID uuid.UUID) {
	t.Helper()
	payload := wire.PluginPaneEnterPayload{ChannelID: channelID, ViewerID: viewerID, Width: 80, Height: 24}
	data, _ := json.Marshal(payload)
	if err := srv.handleEnter(data); err != nil {
		t.Fatalf("handleEnter: %v", err)
	}
}

// TestServerHandleChannelCreateRegistersBoard confirms a CHANNEL_CREATE for
// this plugin creates the board+mapping and registers the channel — and
// that a duplicate/second CHANNEL_CREATE for the same channel is a no-op
// rather than creating a second board.
func TestServerHandleChannelCreateRegistersBoard(t *testing.T) {
	db := openPluginServerTestDB(t)
	client, _ := newConcordStub(t)
	srv := New(client, db, "Tukan", "")

	channelID := uuid.New()
	payload := wire.ChannelCreatePayload{
		Channel: &wire.Channel{
			ID:                channelID,
			Name:              "Kanban Board",
			PluginID:          "Tukan",
			PluginChannelKind: "board",
		},
		PluginConfig: map[string]string{"board_name": "Team Kanban"},
	}
	data, _ := json.Marshal(payload)

	if err := srv.handleChannelCreate(data); err != nil {
		t.Fatalf("handleChannelCreate: %v", err)
	}

	boardID, ok, err := db.GetBoardIDForChannel(channelID.String())
	if err != nil || !ok {
		t.Fatalf("GetBoardIDForChannel: ok=%v err=%v", ok, err)
	}
	board, err := db.GetBoardByID(boardID)
	if err != nil {
		t.Fatalf("GetBoardByID: %v", err)
	}
	if board.Name != "Team Kanban" {
		t.Fatalf("board.Name = %q, want %q", board.Name, "Team Kanban")
	}

	srv.mu.Lock()
	_, registered := srv.channels[channelID.String()]
	srv.mu.Unlock()
	if !registered {
		t.Fatal("channel not registered in srv.channels after handleChannelCreate")
	}

	// A duplicate CHANNEL_CREATE for the same channel must not create a
	// second board.
	if err := srv.handleChannelCreate(data); err != nil {
		t.Fatalf("handleChannelCreate (duplicate): %v", err)
	}
	boards, err := db.ListBoards()
	if err != nil {
		t.Fatalf("ListBoards: %v", err)
	}
	count := 0
	for _, b := range boards {
		if b.Name == "Team Kanban" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("boards named %q = %d, want exactly 1 (duplicate CHANNEL_CREATE created another)", "Team Kanban", count)
	}
}

// TestServerHandleEnterUnknownChannelPushesNothing confirms an ENTER for a
// channel Tukan has never heard of (no plugin_channels row, never seen a
// CHANNEL_CREATE) is logged and ignored rather than erroring or crashing —
// per the integration plan's gap #1, this is expected to happen for real
// once tukan-server has been offline during a channel's creation.
func TestServerHandleEnterUnknownChannelPushesNothing(t *testing.T) {
	db := openPluginServerTestDB(t)
	client, conn := newConcordStub(t)
	srv := New(client, db, "Tukan", "")

	payload := wire.PluginPaneEnterPayload{ChannelID: uuid.New(), ViewerID: uuid.New(), Width: 80, Height: 24}
	data, _ := json.Marshal(payload)
	if err := srv.handleEnter(data); err != nil {
		t.Fatalf("handleEnter: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected no frame to be pushed for an unknown channel, but one arrived")
	}
}

// TestServerHandleInputBroadcastsToOtherViewersAndNotifies drives the full
// collaborative flow end-to-end: two viewers enter the same channel, one of
// them creates a card across three keystrokes (open form, type title,
// save). Confirms each keystroke pushes the acting viewer's own frame,
// exactly the save step fires a notify event (the other two steps don't
// change the database), and the OTHER viewer gets a broadcast frame after
// every step (the "always broadcast" design the plan settled on, not just
// after mutating ones).
func TestServerHandleInputBroadcastsToOtherViewersAndNotifies(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	client, conn := newConcordStub(t)
	srv := New(client, db, "Tukan", "")

	channelID := uuid.New()
	srv.mu.Lock()
	srv.channels[channelID.String()] = newChannelBoard(boardID)
	srv.mu.Unlock()

	viewerA, viewerB := uuid.New(), uuid.New()
	mustEnter(t, srv, channelID, viewerA)
	mustEnter(t, srv, channelID, viewerB)
	drainMessages(t, conn, 2, time.Second) // both viewers' initial frames

	steps := []wire.PluginPaneInputPayload{
		{ChannelID: channelID, ViewerID: viewerA, KeyType: int(tea.KeyRunes), Runes: []rune("n")},
		{ChannelID: channelID, ViewerID: viewerA, KeyType: int(tea.KeyRunes), Runes: []rune("X3")},
		{ChannelID: channelID, ViewerID: viewerA, KeyType: int(tea.KeyCtrlS)},
	}
	for _, step := range steps {
		data, _ := json.Marshal(step)
		if err := srv.handleInput(data); err != nil {
			t.Fatalf("handleInput: %v", err)
		}
	}

	// 3 steps * (own frame + broadcast frame to B) + 1 notify (save step only).
	msgs := drainMessages(t, conn, 7, time.Second)

	var frameA, frameB, notify int
	var notifyContent string
	for _, m := range msgs {
		switch m.Op {
		case wire.OpPluginPaneFrame:
			var f wire.PluginPaneFramePayload
			if err := json.Unmarshal(m.Data, &f); err != nil {
				t.Fatalf("unmarshal frame payload: %v", err)
			}
			switch f.ViewerID {
			case viewerA:
				frameA++
			case viewerB:
				frameB++
			default:
				t.Fatalf("frame for unexpected viewer %s", f.ViewerID)
			}
		case wire.OpPluginEvent:
			var ev wire.PluginEventPayload
			if err := json.Unmarshal(m.Data, &ev); err != nil {
				t.Fatalf("unmarshal event payload: %v", err)
			}
			if ev.Kind != "notify" {
				t.Fatalf("event kind = %q, want notify", ev.Kind)
			}
			var n wire.PluginNotifyEventPayload
			if err := json.Unmarshal(ev.Payload, &n); err != nil {
				t.Fatalf("unmarshal notify payload: %v", err)
			}
			notify++
			notifyContent = n.Content
		default:
			t.Fatalf("unexpected op %d", m.Op)
		}
	}

	if frameA != 3 {
		t.Fatalf("frames pushed to viewer A = %d, want 3 (one per keystroke)", frameA)
	}
	if frameB != 3 {
		t.Fatalf("broadcast frames pushed to viewer B = %d, want 3 (one per keystroke, always-broadcast design)", frameB)
	}
	if notify != 1 {
		t.Fatalf("notify events = %d, want exactly 1 (only the save step mutates the database)", notify)
	}
	if !strings.Contains(notifyContent, "X3") {
		t.Fatalf("notify content = %q, want it to mention the created card's title", notifyContent)
	}
}

// TestServerHandleInputQOnMainViewSendsLeavePaneNotFrame confirms the
// full q-to-leave flow: pressing 'q' while the board is on its main view
// sends a leave_pane OpPluginEvent targeted at that viewer instead of a
// normal frame push.
func TestServerHandleInputQOnMainViewSendsLeavePaneNotFrame(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	client, conn := newConcordStub(t)
	srv := New(client, db, "Tukan", "")

	channelID := uuid.New()
	srv.mu.Lock()
	srv.channels[channelID.String()] = newChannelBoard(boardID)
	srv.mu.Unlock()

	viewerID := uuid.New()
	mustEnter(t, srv, channelID, viewerID)
	drainMessages(t, conn, 1, time.Second) // initial frame

	qPayload := wire.PluginPaneInputPayload{ChannelID: channelID, ViewerID: viewerID, KeyType: int(tea.KeyRunes), Runes: []rune("q")}
	data, _ := json.Marshal(qPayload)
	if err := srv.handleInput(data); err != nil {
		t.Fatalf("handleInput(q on main view): %v", err)
	}

	msgs := drainMessages(t, conn, 1, time.Second)
	if msgs[0].Op != wire.OpPluginEvent {
		t.Fatalf("Op = %v, want OpPluginEvent (leave_pane), got a frame push instead", msgs[0].Op)
	}
	var ev wire.PluginEventPayload
	if err := json.Unmarshal(msgs[0].Data, &ev); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if ev.Kind != "leave_pane" {
		t.Fatalf("Kind = %q, want leave_pane", ev.Kind)
	}
	if ev.ViewerID != viewerID {
		t.Fatalf("ViewerID = %s, want %s", ev.ViewerID, viewerID)
	}
	var closePayload wire.PluginPaneClosePayload
	if err := json.Unmarshal(ev.Payload, &closePayload); err != nil {
		t.Fatalf("unmarshal close payload: %v", err)
	}
	if closePayload.ChannelID != channelID {
		t.Fatalf("ChannelID = %s, want %s", closePayload.ChannelID, channelID)
	}
}

// TestServerHandleInputQInsideFormIsForwardedNotLeft confirms 'q' typed
// while a form is open (an ordinary character a card title might contain,
// e.g. "Quick fix") is forwarded to the board normally rather than
// triggering a pane leave.
func TestServerHandleInputQInsideFormIsForwardedNotLeft(t *testing.T) {
	db, _, boardID := newTestBoard(t)
	client, conn := newConcordStub(t)
	srv := New(client, db, "Tukan", "")

	channelID := uuid.New()
	srv.mu.Lock()
	srv.channels[channelID.String()] = newChannelBoard(boardID)
	srv.mu.Unlock()

	viewerID := uuid.New()
	mustEnter(t, srv, channelID, viewerID)
	drainMessages(t, conn, 1, time.Second) // initial frame

	open := wire.PluginPaneInputPayload{ChannelID: channelID, ViewerID: viewerID, KeyType: int(tea.KeyRunes), Runes: []rune("n")}
	data, _ := json.Marshal(open)
	if err := srv.handleInput(data); err != nil {
		t.Fatalf("handleInput(n): %v", err)
	}
	drainMessages(t, conn, 1, time.Second) // frame after opening the form

	qPayload := wire.PluginPaneInputPayload{ChannelID: channelID, ViewerID: viewerID, KeyType: int(tea.KeyRunes), Runes: []rune("q")}
	data, _ = json.Marshal(qPayload)
	if err := srv.handleInput(data); err != nil {
		t.Fatalf("handleInput(q inside form): %v", err)
	}

	msgs := drainMessages(t, conn, 1, time.Second)
	if msgs[0].Op != wire.OpPluginPaneFrame {
		t.Fatalf("Op = %v, want OpPluginPaneFrame (q inside a form should be forwarded as text, not trigger leave_pane)", msgs[0].Op)
	}
}
