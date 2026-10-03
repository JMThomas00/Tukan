package concord

import (
	"context"
	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/ui"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JMThomas00/Concord/sdk/plugin"
	"github.com/JMThomas00/Concord/sdk/plugintest"
	"github.com/JMThomas00/Concord/sdk/wire"

	"github.com/JMThomas00/tukan/internal/database"
)

func rig(t *testing.T) (*plugintest.Server, uuid.UUID, *database.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "tukan.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	srv := plugintest.NewServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { plugin.Run(ctx, srv.Config(), New(db, "").Handler()); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	srv.WaitReady()
	ch := uuid.New()
	srv.Channel(wire.Channel{ID: ch, Name: "sprint", PluginConfig: map[string]string{"board_name": "Sprint Board"}})
	return srv, ch, db
}

// A channel gets its board on first sight, named from the channel's setting.
func TestChannelGetsItsBoard(t *testing.T) {
	srv, ch, db := rig(t)
	v := srv.Enter(ch, "alice", 100, 30)
	srv.FrameContaining(v, "Sprint Board")
	if _, ok, err := db.GetBoardIDForChannel(ch.String()); err != nil || !ok {
		t.Fatalf("no board mapped for the channel: %v", err)
	}
}

// One viewer adds a card: the other sees it without pressing anything, and
// the activity channel hears about it once.
func TestEditsReachOtherViewersAndNotifyOnce(t *testing.T) {
	srv, ch, _ := rig(t)
	alice := srv.Enter(ch, "alice", 100, 30)
	bob := srv.Enter(ch, "bob", 100, 30)
	srv.FrameContaining(alice, "Sprint Board")
	srv.FrameContaining(bob, "Sprint Board")

	srv.Key(alice, "n")
	srv.Type(alice, "Ship streaming")
	srv.Key(alice, "ctrl+s")
	srv.FrameContaining(bob, "Ship streaming")

	// alice's record (a card created) comes first, then the notice.
	ev := srv.NextEvent()
	if ev.Kind != wire.PluginEventRecord || !strings.Contains(string(ev.Payload), `"first_card"`) {
		t.Fatalf("record = %s %s", ev.Kind, ev.Payload)
	}
	ev = srv.NextEvent()
	if ev.Kind != wire.PluginEventNotify || !strings.Contains(string(ev.Payload), "Ship streaming created in ") {
		t.Fatalf("notify = %s %s", ev.Kind, ev.Payload)
	}
	time.Sleep(2 * notifyDelay)
	if more := srv.DrainEvents(); len(more) != 0 {
		t.Fatalf("one card, %d more notices: %+v", len(more), more)
	}
}

// q on the main view hands the keyboard back; inside a form it's typing.
func TestQLeavesOnlyFromTheMainView(t *testing.T) {
	srv, ch, _ := rig(t)
	v := srv.Enter(ch, "alice", 100, 30)
	srv.FrameContaining(v, "Sprint Board")

	srv.Key(v, "n")
	srv.Type(v, "quick fix")
	srv.FrameContaining(v, "quick fix")
	srv.Key(v, "esc")

	srv.Key(v, "q")
	ev := srv.NextEvent()
	if ev.Kind != wire.PluginEventLeavePane || ev.ViewerID != v.ID {
		t.Fatalf("q on the main view sent %s", ev.Kind)
	}
}

// The card form claims Esc, Tab and Shift+Tab (to cancel and to move
// between fields); the plain board claims none, so in Concord they move
// focus between panels.
func TestNavigationKeysClaimedOnlyByOverlays(t *testing.T) {
	srv, ch, _ := rig(t)
	v := srv.Enter(ch, "alice", 100, 30)
	srv.FrameContaining(v, "Sprint Board")
	if c := srv.Claimed(v); len(c) != 0 {
		t.Fatalf("the board claims %v", c)
	}
	srv.Key(v, "n")
	srv.Type(v, "draft")
	srv.FrameContaining(v, "draft")
	if c := srv.Claimed(v); len(c) != 3 {
		t.Fatalf("the card form claims %v", c)
	}
	if !srv.Key(v, "tab") || !srv.Key(v, "esc") {
		t.Fatal("the form's keys didn't reach it")
	}
	for strings.Contains(srv.NextFrame(v), "draft") { // until the form closes
	}
	if c := srv.Claimed(v); len(c) != 0 {
		t.Fatalf("still claiming %v after the form closed", c)
	}
}

func TestNotifyTextNamesTheCardAndWhatHappened(t *testing.T) {
	lanes := map[int64]string{1: "To Do", 2: "In Progress"}
	card := models.Card{ID: 7, LaneID: 1, Title: "Plugins Help", TicketNo: 1}
	before := ui.ContentSnapshot{Cards: []models.Card{card}, LaneNames: lanes}
	moved := card
	moved.LaneID = 2
	after := ui.ContentSnapshot{Cards: []models.Card{moved}, LaneNames: lanes}
	if got, _ := notifyText(before, after); got != "#1 Plugins Help moved to In Progress" {
		t.Fatalf("got %q", got)
	}
	if got, _ := notifyText(after, ui.ContentSnapshot{LaneNames: lanes}); got != "#1 Plugins Help deleted" {
		t.Fatalf("got %q", got)
	}
	if _, ok := notifyText(before, before); ok {
		t.Fatal("a notice for no change")
	}
}
