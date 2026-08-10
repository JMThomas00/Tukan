package wire

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestMessageEnvelopeOmitsEmptyFields confirms Message's own JSON shape
// matches protocol.Message's field tags exactly (op/d/s/t, with d/s/t all
// omitempty) — this is the one struct every message on the wire is wrapped
// in, so a mismatch here would break everything, not just one payload.
func TestMessageEnvelopeOmitsEmptyFields(t *testing.T) {
	msg := Message{Op: OpIdentify}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"op":0}`
	if string(raw) != want {
		t.Fatalf("Message{Op: OpIdentify} = %s, want %s (Data/Seq/Type must be omitted when empty)", raw, want)
	}
}

// TestNewMessageMarshalsPayload confirms NewMessage produces the same
// {"op":N,"d":{...}} shape testplugin's own p.send helper relies on.
func TestNewMessageMarshalsPayload(t *testing.T) {
	msg, err := NewMessage(OpPluginEvent, PluginEventPayload{PluginID: "Tukan", Kind: "notify"})
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if msg.Op != OpPluginEvent {
		t.Fatalf("Op = %v, want OpPluginEvent", msg.Op)
	}
	var decoded PluginEventPayload
	if err := json.Unmarshal(msg.Data, &decoded); err != nil {
		t.Fatalf("unmarshal Data: %v", err)
	}
	if decoded.PluginID != "Tukan" || decoded.Kind != "notify" {
		t.Fatalf("decoded = %+v, want PluginID=Tukan Kind=notify", decoded)
	}
}

// TestPluginEventPayloadViewerIDShape confirms the exact field name/casing
// (matching protocol.PluginEventPayload's own tag). Note ViewerID's
// `omitempty` has no actual effect — uuid.UUID is a fixed-size [16]byte
// array, a kind encoding/json's omitempty never treats as "empty" (that
// only applies to bools/numbers/strings and nil-able pointer/slice/map/
// interface kinds) — so a "notify" event (no specific viewer) still
// marshals an all-zero viewer_id. Harmless: HandlePluginEvent on Concord's
// side compares req.ViewerID against uuid.Nil in Go, not JSON presence.
func TestPluginEventPayloadViewerIDShape(t *testing.T) {
	notify := PluginEventPayload{PluginID: "Tukan", Kind: "notify", Payload: json.RawMessage(`{}`)}
	raw, err := json.Marshal(notify)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"plugin_id":"Tukan","kind":"notify","payload":{},"viewer_id":"00000000-0000-0000-0000-000000000000"}`
	if string(raw) != want {
		t.Fatalf("notify event marshaled as %s, want %s", raw, want)
	}

	viewerID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	targeted := PluginEventPayload{PluginID: "Tukan", Kind: "leave_pane", Payload: json.RawMessage(`{}`), ViewerID: viewerID}
	raw, err = json.Marshal(targeted)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want = `{"plugin_id":"Tukan","kind":"leave_pane","payload":{},"viewer_id":"44444444-4444-4444-4444-444444444444"}`
	if string(raw) != want {
		t.Fatalf("targeted event marshaled as %s, want %s", raw, want)
	}
}

// TestPluginPaneClosePayloadShape confirms the exact wire shape for the
// leave_pane signal's own payload.
func TestPluginPaneClosePayloadShape(t *testing.T) {
	channelID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	raw, err := json.Marshal(PluginPaneClosePayload{ChannelID: channelID})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"channel_id":"55555555-5555-5555-5555-555555555555"}`
	if string(raw) != want {
		t.Fatalf("PluginPaneClosePayload marshaled as %s, want %s", raw, want)
	}
}

// TestPluginPaneFramePayloadShape confirms the exact field names or a real
// Concord server would fail to relay it — Seq/Frame/ViewerID/ChannelID all
// required (no omitempty) per protocol.PluginPaneFramePayload.
func TestPluginPaneFramePayloadShape(t *testing.T) {
	channelID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	viewerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	payload := PluginPaneFramePayload{
		ChannelID: channelID,
		ViewerID:  viewerID,
		Frame:     "hello",
		Seq:       3,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"channel_id":"11111111-1111-1111-1111-111111111111","viewer_id":"22222222-2222-2222-2222-222222222222","frame":"hello","seq":3}`
	if string(raw) != want {
		t.Fatalf("PluginPaneFramePayload marshaled as %s, want %s", raw, want)
	}
}

// TestPluginPaneInputPayloadRoundTrip confirms KeyType/Runes/Alt survive a
// round trip intact — these feed directly into reconstructing a
// tea.KeyMsg, so any lossy encoding here would corrupt every keystroke.
func TestPluginPaneInputPayloadRoundTrip(t *testing.T) {
	original := PluginPaneInputPayload{
		ChannelID: uuid.New(),
		ViewerID:  uuid.New(),
		KeyType:   -1,
		Runes:     []rune{'a', 'b'},
		Alt:       true,
		KeyString: "alt+ab",
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded PluginPaneInputPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.KeyType != original.KeyType || string(decoded.Runes) != string(original.Runes) || decoded.Alt != original.Alt {
		t.Fatalf("round trip = %+v, want %+v", decoded, original)
	}
}

// TestChannelCreatePayloadUnmarshalsEmbeddedChannel confirms the embedded
// *Channel field correctly promotes to the top level on unmarshal — this
// mirrors protocol.ChannelCreatePayload's *models.Channel embedding, and
// the JSON here is written the way Concord's server would actually send
// it (channel fields and plugin_config as siblings at the top level, not
// nested under a "channel" key), not a shape Tukan invented.
func TestChannelCreatePayloadUnmarshalsEmbeddedChannel(t *testing.T) {
	raw := []byte(`{
		"id": "33333333-3333-3333-3333-333333333333",
		"name": "Kanban Board",
		"plugin_id": "Tukan",
		"plugin_channel_kind": "board",
		"plugin_config": {"board_name": "Team Kanban"}
	}`)

	var payload ChannelCreatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Channel == nil {
		t.Fatal("payload.Channel is nil — embedded pointer promotion didn't populate it")
	}
	if payload.Channel.Name != "Kanban Board" {
		t.Fatalf("Channel.Name = %q, want %q", payload.Channel.Name, "Kanban Board")
	}
	if payload.Channel.PluginID != "Tukan" || payload.Channel.PluginChannelKind != "board" {
		t.Fatalf("Channel = %+v, want PluginID=Tukan PluginChannelKind=board", payload.Channel)
	}
	if payload.PluginConfig["board_name"] != "Team Kanban" {
		t.Fatalf("PluginConfig = %+v, want board_name=Team Kanban", payload.PluginConfig)
	}
}
