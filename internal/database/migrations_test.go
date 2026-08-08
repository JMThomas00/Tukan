package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JMThomas00/tukan/internal/models"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func schemaVersion(t *testing.T, db *DB) int {
	t.Helper()
	var v int
	if err := db.sql.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	return v
}

func TestMigrateFreshDatabase(t *testing.T) {
	db := openTestDB(t)

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	want := migrations[len(migrations)-1].version
	if got := schemaVersion(t, db); got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}

	// A fresh database should already have its seeded "Main Board" (from the
	// v3 migration), and its tables should be usable end-to-end.
	boards, err := db.ListBoards()
	if err != nil {
		t.Fatalf("list boards after migrate: %v", err)
	}
	if len(boards) != 1 || boards[0].Name != "Main Board" {
		t.Fatalf("boards after migrate = %+v, want a single seeded Main Board", boards)
	}

	if _, err := db.ListLanesByBoard(boards[0].ID); err != nil {
		t.Fatalf("list lanes after migrate: %v", err)
	}
	if err := db.SeedDefaultLanes(boards[0].ID); err != nil {
		t.Fatalf("seed default lanes after migrate: %v", err)
	}
	lanes, err := db.ListLanesByBoard(boards[0].ID)
	if err != nil {
		t.Fatalf("list lanes after seed: %v", err)
	}
	if len(lanes) != 4 {
		t.Fatalf("len(lanes) = %d, want 4", len(lanes))
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openTestDB(t)

	if err := db.Migrate(); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	boards, err := db.ListBoards()
	if err != nil || len(boards) != 1 {
		t.Fatalf("list boards: %v (boards=%+v)", err, boards)
	}
	if err := db.SeedDefaultLanes(boards[0].ID); err != nil {
		t.Fatalf("seed default lanes: %v", err)
	}

	versionAfterFirst := schemaVersion(t, db)

	if err := db.Migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	if got := schemaVersion(t, db); got != versionAfterFirst {
		t.Fatalf("schema version changed on re-migrate: %d -> %d", versionAfterFirst, got)
	}

	// Data from before the second migrate call must survive untouched.
	boardsAfter, err := db.ListBoards()
	if err != nil || len(boardsAfter) != 1 {
		t.Fatalf("list boards after re-migrate: %v (boards=%+v)", err, boardsAfter)
	}
	lanes, err := db.ListLanesByBoard(boards[0].ID)
	if err != nil {
		t.Fatalf("list lanes after re-migrate: %v", err)
	}
	if len(lanes) != 4 {
		t.Fatalf("len(lanes) = %d after re-migrate, want 4", len(lanes))
	}
}

// TestMigrationVersionsAreSequential guards against a future migration being
// appended with a skipped or out-of-order version number, which would break
// runMigrations' "apply everything newer than current" logic.
func TestMigrationVersionsAreSequential(t *testing.T) {
	for i, m := range migrations {
		wantVersion := i + 1
		if m.version != wantVersion {
			t.Fatalf("migrations[%d] has version %d, want %d (sequential from 1)", i, m.version, wantVersion)
		}
		if m.stmts == nil && m.run == nil {
			t.Fatalf("migrations[%d] (version %d) has neither stmts nor run set", i, m.version)
		}
	}
}

// TestMigrateFromEachPriorVersion applies migrations one at a time up to
// each version in turn, confirming every step succeeds from the version
// immediately before it (not just as part of one big fresh-install run).
func TestMigrateFromEachPriorVersion(t *testing.T) {
	for _, m := range migrations {
		db := openTestDB(t)

		// Bring the database to the version immediately before this migration
		// by running everything up to (but not including) it.
		for _, prior := range migrations {
			if prior.version >= m.version {
				break
			}
			if err := applyMigration(db.sql, prior); err != nil {
				t.Fatalf("apply prior migration %d: %v", prior.version, err)
			}
		}

		if err := applyMigration(db.sql, m); err != nil {
			t.Fatalf("apply migration %d (%s) from version %d: %v", m.version, m.desc, m.version-1, err)
		}
	}
}

// TestMigrateFromV1PreservesExistingData simulates upgrading a real
// pre-1.1 install (v1 schema, no due_date column, with real rows already in
// it) all the way to latest, and confirms: the v2 migration (ALTER TABLE
// cards ADD COLUMN due_date) preserves existing lanes/cards and leaves the
// new column usable; and the v7 migration (assignee registry) carries the
// pre-existing free-text assignee value forward into the new
// assignees/card_assignees tables rather than losing it when
// cards.assignee is dropped.
func TestMigrateFromV1PreservesExistingData(t *testing.T) {
	db := openTestDB(t)

	if err := applyMigration(db.sql, migrations[0]); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	if schemaVersion(t, db) != 1 {
		t.Fatalf("expected schema version 1 before upgrade")
	}

	if _, err := db.sql.Exec(`INSERT INTO lanes (id, name, position) VALUES (1, 'To-Do', 0)`); err != nil {
		t.Fatalf("seed lane: %v", err)
	}
	if _, err := db.sql.Exec(
		`INSERT INTO cards (id, lane_id, title, assignee, position) VALUES (1, 1, 'Pre-existing card', 'jordan', 0)`,
	); err != nil {
		t.Fatalf("seed card: %v", err)
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate to latest: %v", err)
	}

	cards, err := db.ListCardsByLane(1)
	if err != nil {
		t.Fatalf("list cards after upgrade: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("len(cards) = %d after upgrade, want 1", len(cards))
	}
	if cards[0].Title != "Pre-existing card" {
		t.Fatalf("pre-existing card corrupted after upgrade: %+v", cards[0])
	}
	if cards[0].DueDate != nil {
		t.Fatalf("pre-existing card due date = %v, want nil (column added but unset)", cards[0].DueDate)
	}

	assignees, err := db.ListAssigneesForCard(cards[0].ID)
	if err != nil {
		t.Fatalf("list assignees for pre-existing card: %v", err)
	}
	if len(assignees) != 1 || assignees[0].Name != "jordan" {
		t.Fatalf("pre-existing card's assignees = %+v, want just 'jordan' (carried forward from the old cards.assignee column)", assignees)
	}

	// The new column should be immediately usable on the upgraded schema.
	due := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	created, err := db.CreateCard(models.Card{LaneID: 1, Title: "Post-upgrade card", DueDate: &due})
	if err != nil {
		t.Fatalf("create card after upgrade: %v", err)
	}
	if created.DueDate == nil || !created.DueDate.Equal(due) {
		t.Fatalf("post-upgrade card due date = %v, want %v", created.DueDate, due)
	}

	// The assignee registry should be immediately usable too — a new card
	// can be assigned to the same "jordan" identity the migration created.
	if err := db.SetCardAssignees(created.ID, []int64{assignees[0].ID}); err != nil {
		t.Fatalf("assign post-upgrade card: %v", err)
	}
	postAssignees, err := db.ListAssigneesForCard(created.ID)
	if err != nil {
		t.Fatalf("list assignees for post-upgrade card: %v", err)
	}
	if len(postAssignees) != 1 || postAssignees[0].ID != assignees[0].ID {
		t.Fatalf("post-upgrade card's assignees = %+v, want the same 'jordan' identity (id %d)", postAssignees, assignees[0].ID)
	}
}

// TestV7MigrationSplitsCommaSeparatedAssigneeNames guards a real bug found
// by running this migration against production data: the old single-field
// UI's only way to record more than one person on a card was typing
// "Claude, JMThomas" as one free-text value. A naive migration registers
// that whole string as one bizarre identity instead of two real people —
// this confirms the v7 migration splits on commas instead, and that a name
// repeated across multiple cards (with different comma-mates) resolves to
// one shared identity, not one row per card.
func TestV7MigrationSplitsCommaSeparatedAssigneeNames(t *testing.T) {
	db := openTestDB(t)

	for _, m := range migrations {
		if m.version >= 7 {
			break
		}
		if err := applyMigration(db.sql, m); err != nil {
			t.Fatalf("apply migration %d: %v", m.version, err)
		}
	}

	// migrateBoards (v3, already applied above) seeds board id=1 itself.
	if _, err := db.sql.Exec(`INSERT INTO lanes (id, board_id, name, position) VALUES (1, 1, 'To-Do', 0)`); err != nil {
		t.Fatalf("seed lane: %v", err)
	}
	if _, err := db.sql.Exec(
		`INSERT INTO cards (id, lane_id, title, assignee, position) VALUES
			(1, 1, 'Card A', 'Claude, JMThomas', 0),
			(2, 1, 'Card B', 'JMThomas, Claude', 1),
			(3, 1, 'Card C', '  Jordan Thomas  ', 2)`,
	); err != nil {
		t.Fatalf("seed cards: %v", err)
	}

	if err := applyMigration(db.sql, migrations[6]); err != nil { // version 7
		t.Fatalf("apply v7 migration: %v", err)
	}

	all, err := db.ListAssignees()
	if err != nil {
		t.Fatalf("list assignees: %v", err)
	}
	byName := make(map[string]int64, len(all))
	for _, a := range all {
		byName[a.Name] = a.ID
	}
	if len(all) != 3 {
		t.Fatalf("registered assignees = %+v, want exactly 3 (Claude, JMThomas, Jordan Thomas — deduplicated and split)", all)
	}
	for _, want := range []string{"Claude", "JMThomas", "Jordan Thomas"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("assignees = %+v, missing %q", all, want)
		}
	}

	cardA, err := db.ListAssigneesForCard(1)
	if err != nil {
		t.Fatalf("list assignees for card A: %v", err)
	}
	if len(cardA) != 2 {
		t.Fatalf("card A assignees = %+v, want Claude and JMThomas (2 people, not 1 comma-joined string)", cardA)
	}

	cardB, err := db.ListAssigneesForCard(2)
	if err != nil {
		t.Fatalf("list assignees for card B: %v", err)
	}
	if len(cardB) != 2 {
		t.Fatalf("card B assignees = %+v, want Claude and JMThomas", cardB)
	}
	// Card A and Card B both name Claude and JMThomas (in different order) —
	// they must resolve to the same two registry ids, not four separate rows.
	idsA := map[int64]bool{cardA[0].ID: true, cardA[1].ID: true}
	for _, a := range cardB {
		if !idsA[a.ID] {
			t.Fatalf("card B assignee %+v uses a different identity than card A's %+v for the same name", a, cardA)
		}
	}

	cardC, err := db.ListAssigneesForCard(3)
	if err != nil {
		t.Fatalf("list assignees for card C: %v", err)
	}
	if len(cardC) != 1 || cardC[0].Name != "Jordan Thomas" {
		t.Fatalf("card C assignees = %+v, want just 'Jordan Thomas' (whitespace trimmed)", cardC)
	}
}

// TestV8MigrationBackfillsTicketNumbers confirms pre-existing cards (from
// before ticket numbers existed) get sequential numbers in id order, scoped
// per board, and that boards.next_ticket_no is left pointing at the right
// next value — not just for display, but so a card created immediately
// after upgrading doesn't collide with or skip past the backfilled numbers.
func TestV8MigrationBackfillsTicketNumbers(t *testing.T) {
	db := openTestDB(t)

	for _, m := range migrations {
		if m.version >= 8 {
			break
		}
		if err := applyMigration(db.sql, m); err != nil {
			t.Fatalf("apply migration %d: %v", m.version, err)
		}
	}

	// migrateBoards (v3, already applied above) seeds board id=1 itself.
	if _, err := db.sql.Exec(`INSERT INTO boards (id, name, position) VALUES (2, 'Second Board', 1)`); err != nil {
		t.Fatalf("seed second board: %v", err)
	}
	if _, err := db.sql.Exec(`INSERT INTO lanes (id, board_id, name, position) VALUES (1, 1, 'To-Do', 0), (2, 2, 'To-Do', 0)`); err != nil {
		t.Fatalf("seed lanes: %v", err)
	}
	// Board 1 gets three pre-existing cards; board 2 gets one, to confirm
	// numbering is scoped per board, not global.
	if _, err := db.sql.Exec(
		`INSERT INTO cards (id, lane_id, title, position) VALUES
			(1, 1, 'Card A', 0),
			(2, 1, 'Card B', 1),
			(3, 1, 'Card C', 2),
			(4, 2, 'Other board card', 0)`,
	); err != nil {
		t.Fatalf("seed cards: %v", err)
	}

	if err := applyMigration(db.sql, migrations[7]); err != nil { // version 8
		t.Fatalf("apply v8 migration: %v", err)
	}

	cards, err := db.ListCardsByBoard(1)
	if err != nil {
		t.Fatalf("list cards for board 1: %v", err)
	}
	got := make(map[int64]int, len(cards))
	for _, c := range cards {
		got[c.ID] = c.TicketNo
	}
	want := map[int64]int{1: 1, 2: 2, 3: 3}
	for id, wantNo := range want {
		if got[id] != wantNo {
			t.Fatalf("card %d ticket_no = %d, want %d (backfill = %+v)", id, got[id], wantNo, got)
		}
	}

	otherBoardCards, err := db.ListCardsByBoard(2)
	if err != nil {
		t.Fatalf("list cards for board 2: %v", err)
	}
	if len(otherBoardCards) != 1 || otherBoardCards[0].TicketNo != 1 {
		t.Fatalf("board 2's only card ticket_no = %+v, want 1 (independent per-board sequence)", otherBoardCards)
	}

	// next_ticket_no must be usable immediately: a card created after the
	// upgrade should continue the backfilled sequence, not collide with it.
	created, err := db.CreateCard(models.Card{LaneID: 1, Title: "Post-upgrade card"})
	if err != nil {
		t.Fatalf("create card after v8 upgrade: %v", err)
	}
	if created.TicketNo != 4 {
		t.Fatalf("post-upgrade card ticket_no = %d, want 4 (continuing after the 3 backfilled cards)", created.TicketNo)
	}
}
