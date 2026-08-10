// Package pluginserver hosts Tukan boards for Concord's plugin platform:
// one ui.BoardModel per connected viewer, all reading/writing the same
// underlying database, driven by hand (no tea.Program/real terminal exists
// in this process) via the drain helper below.
package pluginserver

import (
	"reflect"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JMThomas00/tukan/internal/ui"
)

// blinkPkgPath is where bubbles/textinput's cursor-blink message chain
// actually lives — checked via reflection, not a direct type import,
// because the FIRST message in the chain (cursor.initialBlinkMsg) is
// unexported and can't be named from outside that package at all; the
// same reflection check also catches the exported cursor.BlinkMsg that
// follows it, so one mechanism covers the whole chain instead of needing
// two different detection strategies for what's really one problem.
const blinkPkgPath = "github.com/charmbracelet/bubbles/cursor"

// drain runs cmd — and, recursively, anything it triggers — to completion
// against m, standing in for what a real tea.Program's runtime loop does
// when driving Update(). Sequential, not concurrent (unlike the real
// runtime's goroutines/errgroup for tea.BatchMsg): correctness matters
// here, not terminal-frame-rate responsiveness. Confirmed against
// bubbletea's own tea.Program loop (tea.go) that tea.BatchMsg is the only
// wrapper type BoardModel's actual Cmd usage produces — no tea.Sequence
// appears anywhere in internal/ui outside splash.go, which server mode
// never touches.
//
// Deliberately skips — without ever calling it — any cmd whose underlying
// function lives in bubbles/cursor (isBlinkCmd), and deliberately stops
// (without feeding it into Update) the moment it sees a cursor-blink
// message (isBlinkMsg). These are two different problems, found in two
// different ways, and both are needed:
//
//   - isBlinkMsg alone (the original fix) correctly stops the *chain* from
//     recursing forever once a blink message comes back, but a live timing
//     log against a real Concord session showed every single keystroke
//     taking ~530ms end to end regardless — exactly cursor.defaultBlinkSpeed.
//     textinput.Model.Update calls cursor.Model.BlinkCmd() directly and
//     synchronously on every keystroke that moves the cursor (not just on
//     Init/Focus, to reset the blink cycle to solid-visible while actively
//     typing — see textinput.go's `oldPos != m.pos` check), and BlinkCmd's
//     returned closure *blocks internally* on a context timeout
//     (`<-ctx.Done()`) for the full BlinkSpeed before it ever returns
//     anything isBlinkMsg could inspect. By the time a message exists to
//     check, the damage is already done — the block already happened.
//   - isBlinkCmd closes that gap by identifying the cmd itself via its
//     underlying function's runtime name (reflect+runtime, not a type
//     assertion — closures aren't comparable and cursor.Model.BlinkCmd
//     isn't an exported type this package could name anyway), entirely
//     without invoking it, so the blocking call never happens at all.
//
// Both checks stay: isBlinkMsg still matters for textinput.Blink's own
// Init()-returned chain (cheap to call — cursor.Blink() just returns
// initialBlinkMsg{} synchronously — so it doesn't need the pre-invocation
// treatment, but still shouldn't be fed back into Update, or it would
// reach the exact same BlinkCmd() call this fix avoids).
func drain(m ui.BoardModel, cmd tea.Cmd) ui.BoardModel {
	for cmd != nil {
		if isBlinkCmd(cmd) {
			return m
		}
		msg := cmd()
		if isBlinkMsg(msg) {
			return m
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				m = drain(m, c)
			}
			return m
		}
		m, cmd = m.Update(msg)
	}
	return m
}

func isBlinkMsg(msg tea.Msg) bool {
	t := reflect.TypeOf(msg)
	if t == nil {
		return false
	}
	return t.PkgPath() == blinkPkgPath
}

// isBlinkCmd reports whether cmd is bubbles/cursor's BlinkCmd (or a
// closure it produced), identified by the underlying function's own
// runtime-registered name — safe to call on any tea.Cmd, since it never
// invokes cmd itself, only inspects what it already is.
func isBlinkCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	name := runtime.FuncForPC(reflect.ValueOf(cmd).Pointer()).Name()
	return strings.Contains(name, "/bubbles/cursor.")
}
