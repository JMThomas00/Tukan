package database

import (
	"strings"
	"testing"

	"github.com/JMThomas00/tukan/internal/models"
)

func eventBodies(events []models.CardEvent, kind models.CardEventKind) []string {
	var out []string
	for _, e := range events {
		if e.Kind == kind {
			out = append(out, e.Body)
		}
	}
	return out
}

func TestAddCommentAndListCardEvents(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	actor := "jordan"
	comment, err := db.AddComment(card.ID, &actor, "Looks good to me")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if comment.Kind != models.EventComment || comment.Actor == nil || *comment.Actor != "jordan" {
		t.Fatalf("comment = %+v, want kind=comment actor=jordan", comment)
	}

	// Nil actor (no identity concept yet) must round-trip as nil, not "".
	anonComment, err := db.AddComment(card.ID, nil, "no actor set")
	if err != nil {
		t.Fatalf("add anonymous comment: %v", err)
	}
	if anonComment.Actor != nil {
		t.Fatalf("anonymous comment actor = %v, want nil", anonComment.Actor)
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events: %v", err)
	}
	// "created" activity (from CreateCard) + 2 comments = 3 events.
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(events))
	}
	if events[0].Kind != models.EventActivity || events[0].Body != "created" {
		t.Fatalf("events[0] = %+v, want the auto-logged 'created' activity first", events[0])
	}
}

func TestCreateCardLogsActivity(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events: %v", err)
	}
	if len(events) != 1 || events[0].Body != "created" {
		t.Fatalf("events = %+v, want a single 'created' activity", events)
	}
}

func TestUpdateCardLogsCombinedDiff(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	updated := card
	updated.Title = "Ship it fast"
	updated.Note = "now urgent"
	if err := db.UpdateCard(card, updated); err != nil {
		t.Fatalf("update card: %v", err)
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events: %v", err)
	}
	activity := eventBodies(events, models.EventActivity)
	if len(activity) != 2 {
		t.Fatalf("activity = %+v, want 'created' + one combined 'updated' entry", activity)
	}
	last := activity[len(activity)-1]
	if !strings.HasPrefix(last, "updated: ") || !strings.Contains(last, "title") || !strings.Contains(last, "note") {
		t.Fatalf("last activity = %q, want a combined 'updated: title, note'-style entry", last)
	}

	// A no-op update (nothing actually changed) must not log anything new.
	if err := db.UpdateCard(updated, updated); err != nil {
		t.Fatalf("no-op update card: %v", err)
	}
	events, err = db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events after no-op update: %v", err)
	}
	if len(eventBodies(events, models.EventActivity)) != 2 {
		t.Fatalf("activity count changed after a no-op update: %+v", eventBodies(events, models.EventActivity))
	}
}

func TestMoveCardLogsFromAndToLaneNames(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	other, err := db.CreateLane(lane.BoardID, "In Progress", 1)
	if err != nil {
		t.Fatalf("create second lane: %v", err)
	}
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	if err := db.MoveCard(card.ID, other.ID); err != nil {
		t.Fatalf("move card: %v", err)
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events: %v", err)
	}
	activity := eventBodies(events, models.EventActivity)
	last := activity[len(activity)-1]
	if !strings.Contains(last, lane.Name) || !strings.Contains(last, other.Name) {
		t.Fatalf("move activity = %q, want it to mention both %q and %q", last, lane.Name, other.Name)
	}
}

func TestSetCardLabelsLogsAddedAndRemoved(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	bug, err := db.CreateLabel(lane.BoardID, "Bug", "#f7768e")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	ui, err := db.CreateLabel(lane.BoardID, "UI", "#7aa2f7")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}

	if err := db.SetCardLabels(card.ID, []int64{bug.ID, ui.ID}); err != nil {
		t.Fatalf("set card labels: %v", err)
	}
	if err := db.SetCardLabels(card.ID, []int64{bug.ID}); err != nil {
		t.Fatalf("set card labels (drop UI): %v", err)
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events: %v", err)
	}
	activity := eventBodies(events, models.EventActivity)

	var added, removed int
	for _, body := range activity {
		if strings.Contains(body, "added") {
			added++
		}
		if strings.Contains(body, "removed") {
			removed++
		}
	}
	if added != 2 {
		t.Fatalf("added-label activity count = %d, want 2 (Bug, UI)", added)
	}
	if removed != 1 {
		t.Fatalf("removed-label activity count = %d, want 1 (UI)", removed)
	}
}

func TestChecklistMutationsLogActivity(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	item, err := db.CreateChecklistItem(card.ID, "Write tests")
	if err != nil {
		t.Fatalf("create checklist item: %v", err)
	}
	if err := db.ToggleChecklistItem(item.ID); err != nil {
		t.Fatalf("toggle checklist item: %v", err)
	}
	if err := db.ToggleChecklistItem(item.ID); err != nil {
		t.Fatalf("toggle checklist item back: %v", err)
	}
	if err := db.DeleteChecklistItem(item.ID); err != nil {
		t.Fatalf("delete checklist item: %v", err)
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events: %v", err)
	}
	activity := eventBodies(events, models.EventActivity)
	// created, item added, checked off, unchecked, item removed.
	if len(activity) != 5 {
		t.Fatalf("activity = %+v, want 5 entries", activity)
	}
	if !strings.Contains(activity[1], "added") {
		t.Fatalf("activity[1] = %q, want an 'added' entry", activity[1])
	}
	if !strings.Contains(activity[2], "checked off") {
		t.Fatalf("activity[2] = %q, want a 'checked off' entry", activity[2])
	}
	if !strings.Contains(activity[3], "unchecked") {
		t.Fatalf("activity[3] = %q, want an 'unchecked' entry", activity[3])
	}
	if !strings.Contains(activity[4], "removed") {
		t.Fatalf("activity[4] = %q, want a 'removed' entry", activity[4])
	}
}

// TestCardEventsCascadeOnCardDelete confirms card_events rows disappear
// along with their card (no orphaned history) — DeleteCard deliberately
// doesn't log a "deleted" event, since it would be deleted in the same
// cascade before anyone could read it.
func TestCardEventsCascadeOnCardDelete(t *testing.T) {
	db := openTestDB(t)
	lane := newTestLane(t, db)
	card, err := db.CreateCard(models.Card{LaneID: lane.ID, Title: "Ship it"})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	if _, err := db.AddComment(card.ID, nil, "a comment"); err != nil {
		t.Fatalf("add comment: %v", err)
	}

	if err := db.DeleteCard(card.ID); err != nil {
		t.Fatalf("delete card: %v", err)
	}

	events, err := db.ListCardEvents(card.ID)
	if err != nil {
		t.Fatalf("list card events after delete: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events survived card deletion: %+v", events)
	}
}
