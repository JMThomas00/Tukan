package models

import "time"

// CardEventKind discriminates a card_events row: a user-authored comment,
// or an auto-generated activity entry (created, moved, edited, ...).
type CardEventKind string

const (
	EventComment  CardEventKind = "comment"
	EventActivity CardEventKind = "activity"
)

// CardEvent is one entry in a card's combined comment + activity thread.
type CardEvent struct {
	ID     int64
	CardID int64
	Kind   CardEventKind
	// Actor is nullable free text (no user/auth concept exists yet). When
	// real identities exist, the non-breaking path is an additive
	// actor_user_id column — historical rows stay NULL/free-text, new rows
	// populate both; nothing about this struct needs to change for that.
	Actor     *string
	Body      string
	CreatedAt time.Time
}
