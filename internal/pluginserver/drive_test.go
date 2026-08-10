package pluginserver

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/ui"
)

func newTestBoard(t *testing.T) (*database.DB, ui.BoardModel, int64) {
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

	boards, err := db.ListBoards()
	if err != nil || len(boards) == 0 {
		t.Fatalf("list boards: %v (len=%d)", err, len(boards))
	}
	boardID := boards[0].ID

	if err := db.SeedDefaultLanes(boardID); err != nil {
		t.Fatalf("seed lanes: %v", err)
	}
	lanes, err := db.ListLanesByBoard(boardID)
	if err != nil || len(lanes) == 0 {
		t.Fatalf("list lanes: %v (len=%d)", err, len(lanes))
	}

	m, err := ui.NewBoardForID(db, boardID, 80, 24, "")
	if err != nil {
		t.Fatalf("NewBoardForID: %v", err)
	}
	return db, m, boardID
}

// runWithTimeout guards every drain() call in this file: an infinite loop
// (the exact bug drain.go is designed to avoid) would otherwise hang `go
// test` until its own global timeout, which is slow and gives a much less
// useful failure than "still running after 2s".
func runWithTimeout(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out — drain likely looped forever")
	}
}

// TestDrainAppliesBatchedCardFormSave drives the real 'n' -> type a title ->
// ctrl+s flow through BoardModel.Update, confirming drain correctly unpacks
// the tea.Batch(saveCmd, assigneesCmd, labelsCmd) the cardFormDoneMsg
// handler returns. Verified against the database directly (not BoardModel's
// unexported fields, which aren't reachable from this package) — the same
// boundary server mode itself will observe effects through.
func TestDrainAppliesBatchedCardFormSave(t *testing.T) {
	db, m, boardID := newTestBoard(t)

	runWithTimeout(t, func() {
		// Open the new-card form.
		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		m = drain(m2, cmd)

		// Type a title.
		m2, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Hello from drain")})
		m = drain(m2, cmd)

		// Save (ctrl+s) — this is the batched path.
		m2, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		m = drain(m2, cmd)
	})

	cards, err := db.ListCardsByBoard(boardID)
	if err != nil {
		t.Fatalf("list cards: %v", err)
	}
	found := false
	for _, c := range cards {
		if c.Title == "Hello from drain" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cards = %+v, want to find the saved card", cards)
	}
}

// TestDrainDoesNotHangOnBlink confirms opening the card form (whose Init()
// unconditionally returns textinput.Blink) doesn't loop forever — the exact
// failure mode drain.go's blink guard exists to prevent.
func TestDrainDoesNotHangOnBlink(t *testing.T) {
	_, m, _ := newTestBoard(t)

	runWithTimeout(t, func() {
		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		m = drain(m2, cmd)
	})
}

// TestIsBlinkMsgIdentifiesTextinputBlink confirms the reflection-based
// package-path check actually matches what textinput.Blink() really
// produces (cursor.initialBlinkMsg, unexported — this is the only way to
// name it from outside bubbles/cursor at all), and doesn't false-positive
// on an ordinary message.
func TestIsBlinkMsgIdentifiesTextinputBlink(t *testing.T) {
	msg := textinput.Blink()
	if !isBlinkMsg(msg) {
		t.Fatalf("isBlinkMsg(%#v) = false, want true", msg)
	}
	if isBlinkMsg(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("isBlinkMsg(tea.KeyMsg{...}) = true, want false")
	}
	if isBlinkMsg(nil) {
		t.Fatal("isBlinkMsg(nil) = true, want false")
	}
}

// TestIsBlinkCmdIdentifiesCursorBlinkCmd is the direct regression test for
// the ~530ms-per-keystroke bug found live against a real Concord session:
// cursor.Model.BlinkCmd()'s returned closure blocks internally (on a
// context timeout matching cursor's own BlinkSpeed) before it ever returns
// a message isBlinkMsg could inspect, so it must be recognized — and
// skipped — before ever being called. Confirms it's identified via a real
// *cursor.Model (not just textinput.Blink, which is a different, cheap-
// to-call function in a different package and correctly does NOT match
// here — see the comment on drain for why that's intentional), and that an
// ordinary cmd doesn't false-positive.
func TestIsBlinkCmdIdentifiesCursorBlinkCmd(t *testing.T) {
	c := cursor.New()
	c.Focus()
	blinkCmd := c.BlinkCmd()
	if !isBlinkCmd(blinkCmd) {
		t.Fatal("isBlinkCmd(cursor.Model.BlinkCmd()) = false, want true")
	}

	if isBlinkCmd(textinput.Blink) {
		t.Fatal("isBlinkCmd(textinput.Blink) = true, want false — that path is handled cheaply via isBlinkMsg instead, not this pre-invocation check")
	}
	ordinary := func() tea.Msg { return tea.KeyMsg{Type: tea.KeyEnter} }
	if isBlinkCmd(ordinary) {
		t.Fatal("isBlinkCmd(ordinary cmd) = true, want false")
	}
	if isBlinkCmd(nil) {
		t.Fatal("isBlinkCmd(nil) = true, want false")
	}
}

// TestDrainDoesNotBlockOnKeystrokeBlinkReset is the end-to-end regression
// test for the actual reported symptom: typing into a focused text field
// took ~530ms per keystroke in a live Concord session, exactly matching
// cursor.defaultBlinkSpeed. textinput.Model.Update calls
// cursor.Model.BlinkCmd() directly whenever the cursor position changes
// (not just on focus, to reset the blink cycle to solid-visible while
// actively typing), wrapped in a tea.Batch alongside the rest of that
// Update call's result — so this is exercised by an entirely ordinary
// second keystroke into an already-focused field, not a contrived setup.
func TestDrainDoesNotBlockOnKeystrokeBlinkReset(t *testing.T) {
	_, m, _ := newTestBoard(t)

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = drain(m2, cmd)

	start := time.Now()
	runWithTimeout(t, func() {
		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
		m = drain(m2, cmd)
		m2, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
		m = drain(m2, cmd)
	})
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("two keystrokes took %s, want well under cursor.defaultBlinkSpeed (530ms) — the blink-reset block is back", elapsed)
	}
}
