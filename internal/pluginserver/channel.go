package pluginserver

import (
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/ui"
)

// FramePush is one rendered frame ready to be sent to Concord as an
// OpPluginPaneFrame for a specific viewer.
type FramePush struct {
	ViewerID uuid.UUID
	Frame    string
	Seq      int64
}

// ChannelBoard is one Concord channel's board: which Tukan board it's
// mapped to, and one independent BoardModel per human currently viewing
// that channel's pane. All viewers share the same *database.DB — the
// shared SQLite file is the entire collaboration mechanism; ChannelBoard
// itself holds no board data of its own beyond which viewers are currently
// connected and each one's last-pushed frame sequence.
type ChannelBoard struct {
	mu      sync.Mutex
	boardID int64
	viewers map[uuid.UUID]*viewer
}

func newChannelBoard(boardID int64) *ChannelBoard {
	return &ChannelBoard{
		boardID: boardID,
		viewers: make(map[uuid.UUID]*viewer),
	}
}

// Enter creates a fresh BoardModel for a newly-connected viewer and returns
// its first frame at Seq 0. Each viewer gets its own BoardModel instance —
// independent cursor/mode/form state — per the plan's core architectural
// choice of reusing ui.BoardModel as-is rather than inventing a new
// shared-state layer.
func (cb *ChannelBoard) Enter(db *database.DB, viewerID uuid.UUID, width, height int, themeName string) (FramePush, error) {
	m, err := ui.NewBoardForID(db, cb.boardID, width, height, themeName)
	if err != nil {
		return FramePush{}, fmt.Errorf("enter viewer %s: %w", viewerID, err)
	}
	m.SetSize(width, height)

	cb.mu.Lock()
	defer cb.mu.Unlock()
	v := &viewer{BoardModel: m, width: width, height: height}
	cb.viewers[viewerID] = v
	return FramePush{ViewerID: viewerID, Frame: v.View(), Seq: 0}, nil
}

// Resize re-renders a viewer's board at its new terminal size. Reports
// false if the viewer isn't known (e.g. a stray resize after Leave).
func (cb *ChannelBoard) Resize(viewerID uuid.UUID, width, height int) (FramePush, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	v, ok := cb.viewers[viewerID]
	if !ok {
		return FramePush{}, false
	}
	v.width, v.height = width, height
	v.SetSize(width, height)
	v.seq++
	return FramePush{ViewerID: viewerID, Frame: v.View(), Seq: v.seq}, true
}

// Input drives one keypress through the acting viewer's BoardModel via
// drain (handling any tea.Batch a save produces) and returns the resulting
// frame, plus a before/after ui.ContentSnapshot pair the caller can diff
// (via notifyText) to decide whether to fire a notification — computed
// from the viewer's own in-memory BoardModel state, not a fresh database
// query. An earlier version re-queried the database before and after every
// keystroke to detect mutations; since most keystrokes (ordinary text
// typing into a form field) never touch the database at all, that added
// real, human-perceptible input lag to every single keypress. It does not
// by itself tell other viewers anything changed — BroadcastReload handles
// that, called separately by the caller.
func (cb *ChannelBoard) Input(viewerID uuid.UUID, msg tea.Msg) (push FramePush, before, after ui.ContentSnapshot, ok bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	v, ok := cb.viewers[viewerID]
	if !ok {
		return FramePush{}, ui.ContentSnapshot{}, ui.ContentSnapshot{}, false
	}
	before = v.BoardModel.Snapshot()
	m, cmd := v.BoardModel.Update(msg)
	v.BoardModel = drain(m, cmd)
	after = v.BoardModel.Snapshot()
	v.seq++
	return FramePush{ViewerID: viewerID, Frame: v.View(), Seq: v.seq}, before, after, true
}

// WantsLeave reports whether viewerID's board is currently on its main
// view (see ui.BoardModel.IsMainView) — used by handleInput to decide
// whether a 'q' keypress should trigger leaving the pane (via a
// leave_pane event to Concord) instead of being driven through Update at
// all. Reports false for an unknown viewer, the same as any other stray
// event.
func (cb *ChannelBoard) WantsLeave(viewerID uuid.UUID) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	v, ok := cb.viewers[viewerID]
	return ok && v.BoardModel.IsMainView()
}

// Leave drops a disconnected viewer's BoardModel entirely.
func (cb *ChannelBoard) Leave(viewerID uuid.UUID) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.viewers, viewerID)
}

// ViewerCount reports how many viewers currently have this channel's board
// open — lets the caller decide whether a ChannelBoard with nobody left
// viewing it should be dropped from its registry.
func (cb *ChannelBoard) ViewerCount() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return len(cb.viewers)
}

// BroadcastReload resyncs every OTHER viewer on the channel against the
// database and returns a frame push for each — this is what makes edits
// collaborative: every other connected human sees the change on their own
// next frame without pressing anything themselves. except is normally the
// viewer whose own Input just produced the mutation (already re-rendered by
// Input itself, so it's skipped here to avoid double-pushing it).
func (cb *ChannelBoard) BroadcastReload(except uuid.UUID) ([]FramePush, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	var pushes []FramePush
	for id, v := range cb.viewers {
		if id == except {
			continue
		}
		if err := v.Reload(); err != nil {
			return nil, fmt.Errorf("reload viewer %s: %w", id, err)
		}
		v.seq++
		pushes = append(pushes, FramePush{ViewerID: id, Frame: v.View(), Seq: v.seq})
	}
	return pushes, nil
}
