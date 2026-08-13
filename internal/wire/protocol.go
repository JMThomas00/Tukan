// Package wire is Tukan's own copy of the subset of Concord's plugin wire
// protocol server mode actually speaks. It cannot import
// github.com/concord-chat/concord/internal/protocol directly — Go's
// internal/ visibility rules only allow imports from within the same
// module tree, and Tukan (github.com/JMThomas00/tukan) and Concord are
// separate modules — so these types are hand-mirrored against
// d:\Concord\internal\protocol\messages.go's actual field tags, not
// guessed. Only what Tukan needs is included; there is no bespoke
// "board" opcode and never will be, per the protocol's own design (see
// Tukan - Concord Plugin Integration.md).
package wire

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// OpCode mirrors protocol.OpCode's underlying representation exactly
// (plain int, not iota-derived on Tukan's side — these values must match
// Concord's numbering byte-for-byte since they're the actual wire values,
// not a private enum).
type OpCode int

const (
	OpIdentify        OpCode = 0
	OpDispatch        OpCode = 10
	OpHello           OpCode = 12
	OpReady           OpCode = 13
	OpInvalidSession  OpCode = 14
	OpPluginPaneFrame OpCode = 53
	OpPluginEvent     OpCode = 54
)

// EventType mirrors protocol.EventType — the values Concord dispatches to
// a plugin's own connection, identified by Message.Type once Op == OpDispatch.
type EventType string

const (
	EventChannelCreate    EventType = "CHANNEL_CREATE"
	EventChannelUpdate    EventType = "CHANNEL_UPDATE"
	EventPluginPaneEnter  EventType = "PLUGIN_PANE_ENTER"
	EventPluginPaneInput  EventType = "PLUGIN_PANE_INPUT"
	EventPluginPaneLeave  EventType = "PLUGIN_PANE_LEAVE"
	EventPluginPaneResize EventType = "PLUGIN_PANE_RESIZE"
)

// Message is the wire envelope — field names/tags/omitempty must match
// protocol.Message exactly, since both ends marshal/unmarshal the same
// JSON.
type Message struct {
	Op   OpCode          `json:"op"`
	Data json.RawMessage `json:"d,omitempty"`
	Seq  *int64          `json:"s,omitempty"`
	Type EventType       `json:"t,omitempty"`
}

// NewMessage mirrors protocol.NewMessage.
func NewMessage(op OpCode, data any) (*Message, error) {
	var raw json.RawMessage
	if data != nil {
		var err error
		raw, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
	}
	return &Message{Op: op, Data: raw}, nil
}

// IdentifyPayload — Tukan only ever sends ClientType "plugin", so
// ConnectionProperties (present on Concord's own struct) is omitted
// entirely rather than mirrored unused.
type IdentifyPayload struct {
	Token      string `json:"token"`
	ClientType string `json:"client_type,omitempty"`
}

type PluginPaneEnterPayload struct {
	ChannelID uuid.UUID `json:"channel_id"`
	ViewerID  uuid.UUID `json:"viewer_id,omitempty"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
}

type PluginPaneResizePayload struct {
	ChannelID uuid.UUID `json:"channel_id"`
	ViewerID  uuid.UUID `json:"viewer_id,omitempty"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
}

// PluginPaneInputPayload's KeyType/Runes/Alt map directly onto
// tea.KeyMsg{Type: tea.KeyType(KeyType), Runes: Runes, Alt: Alt} — both
// ends are Bubble Tea, per the integration note.
type PluginPaneInputPayload struct {
	ChannelID uuid.UUID `json:"channel_id"`
	ViewerID  uuid.UUID `json:"viewer_id,omitempty"`
	KeyType   int       `json:"key_type"`
	Runes     []rune    `json:"runes,omitempty"`
	Alt       bool      `json:"alt,omitempty"`
	KeyString string    `json:"key_string"`
}

type PluginPaneLeavePayload struct {
	ChannelID uuid.UUID `json:"channel_id"`
	ViewerID  uuid.UUID `json:"viewer_id,omitempty"`
}

// PluginPaneFramePayload is pushed BY the plugin (Tukan, as its
// service-account client) — Seq must be monotonically increasing per
// viewer; Concord's client drops frames with Seq <= last-applied.
type PluginPaneFramePayload struct {
	ChannelID uuid.UUID `json:"channel_id"`
	ViewerID  uuid.UUID `json:"viewer_id"`
	Frame     string    `json:"frame"`
	Seq       int64     `json:"seq"`
}

// PluginEventPayload is the generic {plugin_id, kind, payload} envelope —
// there is no board-specific opcode and never will be; anything Tukan
// needs beyond pane rendering goes through Kind as a new string. Setting
// ViewerID targets the envelope at that specific viewer's own client
// (relayed via EventPluginEvent) instead of the server-wide "notify"
// behavior — e.g. Kind "leave_pane" to tell one viewer's pane to close.
type PluginEventPayload struct {
	PluginID string          `json:"plugin_id"`
	Kind     string          `json:"kind"`
	Payload  json.RawMessage `json:"payload"`
	ViewerID uuid.UUID       `json:"viewer_id,omitempty"`
}

// PluginNotifyEventPayload is the Payload shape for
// PluginEventPayload{Kind: "notify"}.
type PluginNotifyEventPayload struct {
	Content string `json:"content"`
}

// PluginPaneClosePayload is the Payload shape for
// PluginEventPayload{Kind: "leave_pane"} — tells the named viewer's client
// to leave a plugin pane it's currently displaying. Tukan sends this when a
// viewer presses 'q' while their board is on its main view (mirroring
// standalone Tukan's own quit binding), rather than forwarding 'q' into
// BoardModel.Update where it would just be a no-op.
type PluginPaneClosePayload struct {
	ChannelID uuid.UUID `json:"channel_id"`
}

// Channel is a minimal mirror of models.Channel — only the fields Tukan
// actually reads off a CHANNEL_CREATE/CHANNEL_UPDATE event (identifying it
// as one of Tukan's own "board" channels, its display name, and its current
// create_field config). Concord's full Channel struct carries far more
// (permission overwrites, DM recipients, voice settings) that Tukan has no
// use for.
type Channel struct {
	ID                uuid.UUID         `json:"id"`
	Name              string            `json:"name"`
	PluginID          string            `json:"plugin_id,omitempty"`
	PluginChannelKind string            `json:"plugin_channel_kind,omitempty"`
	PluginConfig      map[string]string `json:"plugin_config,omitempty"`
}

// ChannelCreatePayload mirrors protocol.ChannelCreatePayload's embedding of
// *models.Channel — Go's JSON encoding promotes an embedded struct
// pointer's own fields to the parent object, so embedding *Channel here
// (not a named field) is required to unmarshal Concord's actual JSON
// shape, not just a style choice.
type ChannelCreatePayload struct {
	*Channel
	PluginConfig map[string]string `json:"plugin_config,omitempty"`
}

// ChannelUpdatePayload mirrors protocol.ChannelUpdatePayload, which embeds
// *models.Channel with no separate field of its own — models.Channel
// carries PluginConfig directly (Concord added this after ChannelCreatePayload
// was first written, hence that one's redundant-looking own field above),
// so it's promoted straight through here too. Concord pushes this to a
// plugin's own connection both on a live channel edit and — since
// 2026-08-12 — once for every channel a plugin owns, right after every
// identify/reconnect (see identifyAsPlugin on Concord's side), specifically
// so a plugin can repair a missing/forgotten channel registration without
// needing a brand new CHANNEL_CREATE.
type ChannelUpdatePayload struct {
	*Channel
}
