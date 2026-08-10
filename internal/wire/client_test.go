package wire

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// testServer spins up a minimal local WebSocket server that speaks just
// enough of Concord's handshake to exercise Client end-to-end: sends
// OpHello immediately on connect (confirmed against a live Concord server —
// it arrives independent of when the client sends OpIdentify, which is
// exactly what broke Client.Identify's original strict "next message must
// be OpReady" implementation), then expects an OpIdentify, replies OpReady,
// then relays whatever it's told to send next. Not a mock of Concord's
// actual server logic — a stand-in for the wire shape, since there's no
// running Concord instance to test against from this environment (see the
// plan's own verification section).
func testServer(t *testing.T, onIdentify func(IdentifyPayload) bool, afterReady func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		hello, _ := NewMessage(OpHello, nil)
		helloRaw, _ := json.Marshal(hello)
		if err := conn.WriteMessage(websocket.TextMessage, helloRaw); err != nil {
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		if msg.Op != OpIdentify {
			return
		}
		var identify IdentifyPayload
		_ = json.Unmarshal(msg.Data, &identify)

		if onIdentify != nil && !onIdentify(identify) {
			// Simulate a rejected identify: close without replying OpReady.
			return
		}

		ready, _ := NewMessage(OpReady, nil)
		raw, _ := json.Marshal(ready)
		if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
			return
		}

		if afterReady != nil {
			afterReady(conn)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

// TestClientIdentifySucceeds drives the real Dial -> Identify sequence
// against a local server that replies OpReady, confirming Identify returns
// cleanly and the token/ClientType it sent matches what a plugin connection
// must send per the integration note ("plugin", not the default "user").
func TestClientIdentifySucceeds(t *testing.T) {
	var gotToken string
	var gotClientType string
	srv := testServer(t, func(p IdentifyPayload) bool {
		gotToken = p.Token
		gotClientType = p.ClientType
		return true
	}, nil)

	c, err := Dial(wsURL(srv.URL))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	if err := c.Identify("secret-token"); err != nil {
		t.Fatalf("identify: %v", err)
	}
	if gotToken != "secret-token" {
		t.Fatalf("server saw token = %q, want %q", gotToken, "secret-token")
	}
	if gotClientType != "plugin" {
		t.Fatalf("server saw client_type = %q, want %q", gotClientType, "plugin")
	}
}

// TestClientIdentifyToleratesLeadingHello is the direct regression test for
// the bug found against a live Concord server: Client.Identify originally
// read exactly one message after sending OpIdentify and required it to be
// OpReady, which broke the instant Concord's OpHello (sent immediately on
// connect, independent of identify) arrived first — Concord's own
// "Plugin authenticated successfully" log line proved the handshake itself
// was fine; only Tukan's client-side read was too strict. testServer now
// always sends Hello first for every test in this file, so this is really
// just naming the scenario explicitly rather than relying on it being
// incidental to every other test too.
func TestClientIdentifyToleratesLeadingHello(t *testing.T) {
	srv := testServer(t, func(IdentifyPayload) bool { return true }, nil)

	c, err := Dial(wsURL(srv.URL))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	if err := c.Identify("secret-token"); err != nil {
		t.Fatalf("Identify should tolerate a leading OpHello, got: %v", err)
	}
}

// TestClientIdentifyFailsWithoutReady confirms a connection that never gets
// an OpReady (the server just hangs up) surfaces as a clear error from
// Identify, not a silent hang or a panic.
func TestClientIdentifyFailsWithoutReady(t *testing.T) {
	srv := testServer(t, func(IdentifyPayload) bool { return false }, nil)

	c, err := Dial(wsURL(srv.URL))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	if err := c.Identify("secret-token"); err == nil {
		t.Fatal("expected Identify to fail when the server never sends OpReady")
	}
}

// TestClientReadLoopDispatchesEvents confirms ReadLoop correctly unmarshals
// and hands off subsequent dispatched events (the shape every
// EventPluginPane* event arrives as: OpDispatch with Type set) after a
// successful identify.
func TestClientReadLoopDispatchesEvents(t *testing.T) {
	srv := testServer(t, nil, func(conn *websocket.Conn) {
		payload := PluginPaneEnterPayload{Width: 80, Height: 24}
		raw, _ := json.Marshal(payload)
		dispatch := Message{Op: OpDispatch, Type: EventPluginPaneEnter, Data: raw}
		data, _ := json.Marshal(dispatch)
		_ = conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(50 * time.Millisecond) // give the client time to read before the handler closes the conn
	})

	c, err := Dial(wsURL(srv.URL))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.Identify("token"); err != nil {
		t.Fatalf("identify: %v", err)
	}

	received := make(chan *Message, 1)
	go func() {
		_ = c.ReadLoop(func(msg *Message) {
			received <- msg
		})
	}()

	select {
	case msg := <-received:
		if msg.Type != EventPluginPaneEnter {
			t.Fatalf("Type = %v, want EventPluginPaneEnter", msg.Type)
		}
		var payload PluginPaneEnterPayload
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.Width != 80 || payload.Height != 24 {
			t.Fatalf("payload = %+v, want Width=80 Height=24", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadLoop to dispatch the event")
	}
}
