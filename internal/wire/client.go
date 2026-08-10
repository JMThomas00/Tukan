package wire

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// Client is a minimal Concord plugin-protocol client: dial, identify, send,
// and a blocking read loop. Standard WebSocket ping/pong keepalive is
// handled automatically by gorilla/websocket's default PingHandler —
// confirmed against Concord's own server (internal/server/client.go pings
// every ~54s, expects a pong within 60s); there's no application-level
// heartbeat message to send, matching the reference plugin
// (d:\Concord\cmd\testplugin\main.go), which has none either.
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex // serializes writes; gorilla/websocket requires this for concurrent senders
}

// Dial opens the WebSocket connection. It does not identify — call
// Identify separately so a failed handshake is a clear, distinct error
// from a failed dial.
func Dial(wsURL string) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", wsURL, err)
	}
	return &Client{conn: conn}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Send marshals and writes one message.
func (c *Client) Send(op OpCode, data any) error {
	msg, err := NewMessage(op, data)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, raw)
}

// readOne blocks for the next message on the connection. Not
// safe to call from more than one goroutine — only ReadLoop and Identify
// call it, and Identify always completes (or fails) before ReadLoop starts.
func (c *Client) readOne() (*Message, error) {
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}
	return &msg, nil
}

// maxIdentifyReads bounds how many non-Ready messages Identify will skip
// past before giving up — a safety net against a connection that never
// sends OpReady at all, not a number expected to matter in practice (Hello
// is normally the only thing that arrives first).
const maxIdentifyReads = 10

// Identify sends OpIdentify with ClientType "plugin" and waits for
// Concord's OpReady acknowledgment, surfacing a clear error if the
// handshake fails rather than letting the caller discover it buried inside
// a generic message handler.
//
// Concord sends OpHello immediately upon connection, independent of when
// (or whether) the client has sent OpIdentify yet — confirmed against a
// live server, where a strict "the next message must be OpReady" read
// failed because it received Hello first. The reference plugin
// (testplugin/main.go) never actually waits for a specific reply at all;
// it just loops reading and only reacts to OpReady when it sees it,
// silently ignoring everything else including Hello. This mirrors that
// tolerance while still surfacing a real failure (OpInvalidSession, or a
// connection that never produces OpReady) as a clear error rather than
// hanging or silently succeeding.
func (c *Client) Identify(token string) error {
	if err := c.Send(OpIdentify, IdentifyPayload{Token: token, ClientType: "plugin"}); err != nil {
		return fmt.Errorf("send identify: %w", err)
	}
	for i := 0; i < maxIdentifyReads; i++ {
		msg, err := c.readOne()
		if err != nil {
			return fmt.Errorf("read identify response: %w", err)
		}
		switch msg.Op {
		case OpReady:
			return nil
		case OpInvalidSession:
			return fmt.Errorf("identify rejected: invalid session")
		}
	}
	return fmt.Errorf("identify failed: did not receive OpReady after %d messages", maxIdentifyReads)
}

// ReadLoop blocks, calling handle for every message received, until the
// connection closes or a read fails — at which point it returns that
// error. Only ever call after a successful Identify.
func (c *Client) ReadLoop(handle func(*Message)) error {
	for {
		msg, err := c.readOne()
		if err != nil {
			return err
		}
		handle(msg)
	}
}
