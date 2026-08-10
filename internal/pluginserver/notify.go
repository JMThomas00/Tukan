package pluginserver

import (
	"fmt"
	"reflect"

	"github.com/JMThomas00/tukan/internal/ui"
)

// notifyText compares two ui.ContentSnapshots of the same board taken
// immediately before and after one viewer's input (via ChannelBoard.Input,
// computed from the viewer's own in-memory state — no database round trip)
// and, if anything actually changed, returns human-readable text plus
// true.
//
// Distinguishes create/delete (the card count changed) from everything
// else; move, assign, label, and checklist edits all collapse into a
// generic "updated" notification for v1. This mirrors the plan's already-
// accepted limitation that notifications aren't attributed to a specific
// user (Concord's plugin protocol has no way to resolve one for a plugin
// connection — see the integration plan's gap #2): a coarse but genuine
// signal beats either spamming a notification per keystroke or building
// full per-field diffing against every mutation shape up front.
func notifyText(before, after ui.ContentSnapshot) (string, bool) {
	if len(after.Cards) > len(before.Cards) {
		if title := newCardTitle(before, after); title != "" {
			return fmt.Sprintf("A card was created: %s", title), true
		}
		return "A card was created", true
	}
	if len(after.Cards) < len(before.Cards) {
		return "A card was deleted", true
	}
	if !reflect.DeepEqual(before, after) {
		return "A card was updated", true
	}
	return "", false
}

func newCardTitle(before, after ui.ContentSnapshot) string {
	seen := make(map[int64]bool, len(before.Cards))
	for _, c := range before.Cards {
		seen[c.ID] = true
	}
	for _, c := range after.Cards {
		if !seen[c.ID] {
			return c.Title
		}
	}
	return ""
}
