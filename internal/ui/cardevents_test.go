package ui

import "testing"

// TestCardEventsLoadedMsgHandledBeforeActiveGuard guards the same
// stay-open footgun as checklist's: cardEventsLoadedMsg arrives while
// cardEventsActive is still true (the modal opens showing a loading
// placeholder, then this message fills it in) and must be intercepted
// before that guard, or the loaded events never reach the open modal.
func TestCardEventsLoadedMsgHandledBeforeActiveGuard(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.cardEventsActive = true
	b.cardEvents = NewCardEvents(card.ID, card.Title, b.width, b.height)
	if !b.cardEvents.loading {
		t.Fatal("expected the modal to start in a loading state")
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events: %v", err)
	}
	if len(events) != 1 || events[0].Body != "created" {
		t.Fatalf("events = %+v, want a single 'created' activity from card creation", events)
	}

	updated, _ := b.Update(cardEventsLoadedMsg{cardID: card.ID, events: events})
	if !updated.cardEventsActive {
		t.Fatal("cardEventsActive should still be true — loading events doesn't close the modal")
	}
	if updated.cardEvents.loading {
		t.Fatal("loading flag still true after cardEventsLoadedMsg — the message was dropped")
	}
	if len(updated.cardEvents.events) != 1 {
		t.Fatalf("cardEvents.events = %+v after load, want 1", updated.cardEvents.events)
	}
}

// TestCardEventsComposeFlowStaysOpen exercises posting a comment: the
// compose message triggers a DB write, and the resulting cardEventAddedMsg
// must patch the still-open modal's thread, not get swallowed by the
// cardEventsActive guard.
func TestCardEventsComposeFlowStaysOpen(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.cardEventsActive = true
	b.cardEvents = NewCardEvents(card.ID, card.Title, b.width, b.height)
	b.cardEvents.loading = false

	updated, cmd := b.Update(cardEventsComposeMsg{cardID: card.ID, body: "Looks good"})
	if !updated.cardEventsActive {
		t.Fatal("cardEventsActive became false on a compose message — only cardEventsClosedMsg should close the modal")
	}
	if cmd == nil {
		t.Fatal("expected an add-comment command, got nil")
	}

	msg := cmd()
	added, ok := msg.(cardEventAddedMsg)
	if !ok {
		t.Fatalf("expected cardEventAddedMsg, got %T", msg)
	}
	if added.cardID != card.ID || added.event.Body != "Looks good" {
		t.Fatalf("cardEventAddedMsg = %+v, want card %d body 'Looks good'", added, card.ID)
	}

	final, _ := updated.Update(added)
	if len(final.cardEvents.events) != 1 || final.cardEvents.events[0].Body != "Looks good" {
		t.Fatalf("cardEvents.events = %+v after compose, want the new comment reflected", final.cardEvents.events)
	}
	if !final.cardEventsActive {
		t.Fatal("cardEventsActive should still be true after posting a comment")
	}
}

func TestCardEventsClosedMsgClosesModal(t *testing.T) {
	db, b := newTestBoard(t)
	card := addTestCard(t, db, &b)

	b.cardEventsActive = true
	b.cardEvents = NewCardEvents(card.ID, card.Title, b.width, b.height)

	updated, cmd := b.Update(cardEventsClosedMsg{})
	if updated.cardEventsActive {
		t.Fatal("cardEventsActive still true after cardEventsClosedMsg")
	}
	if cmd != nil {
		t.Fatal("expected no command from cardEventsClosedMsg")
	}
}
