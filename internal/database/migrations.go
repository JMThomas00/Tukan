package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// migration describes one schema change, applied at most once per database.
// Exactly one of stmts or run should be set: stmts covers the common case
// (executed together in one transaction); run is an escape hatch for
// migrations that need PRAGMA toggling (e.g. foreign_keys=OFF for a table
// rebuild), which cannot happen inside an active transaction.
type migration struct {
	version int
	desc    string
	stmts   []string
	run     func(*sql.DB) error
}

var migrations = []migration{
	{
		version: 1,
		desc:    "initial schema",
		stmts: []string{
			createLanes,
			createCards,
			createCardsTrigger,
			createCardsIndex,
		},
	},
	{
		version: 2,
		desc:    "add cards.due_date",
		stmts: []string{
			`ALTER TABLE cards ADD COLUMN due_date TEXT`,
		},
	},
	{
		version: 3,
		desc:    "add boards; scope lanes to a board",
		run:     migrateBoards,
	},
	{
		version: 4,
		desc:    "add labels and card_labels",
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS labels (
				id       INTEGER PRIMARY KEY AUTOINCREMENT,
				board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
				name     TEXT    NOT NULL,
				color    TEXT    NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_labels_board ON labels(board_id);`,
			`CREATE TABLE IF NOT EXISTS card_labels (
				card_id  INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
				label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
				PRIMARY KEY (card_id, label_id)
			);`,
			`CREATE INDEX IF NOT EXISTS idx_card_labels_label ON card_labels(label_id);`,
		},
	},
	{
		version: 5,
		desc:    "add checklist_items",
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS checklist_items (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				card_id    INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
				text       TEXT    NOT NULL,
				done       INTEGER NOT NULL DEFAULT 0,
				position   INTEGER NOT NULL DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);`,
			`CREATE INDEX IF NOT EXISTS idx_checklist_items_card_position ON checklist_items(card_id, position);`,
			`CREATE TRIGGER IF NOT EXISTS checklist_items_updated_at
				AFTER UPDATE ON checklist_items FOR EACH ROW
			BEGIN
				UPDATE checklist_items SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
			END;`,
		},
	},
	{
		version: 6,
		desc:    "add card_events",
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS card_events (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				card_id    INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
				kind       TEXT    NOT NULL,
				actor      TEXT,
				body       TEXT    NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);`,
			`CREATE INDEX IF NOT EXISTS idx_card_events_card_created ON card_events(card_id, created_at);`,
		},
	},
	{
		// Replaces cards.assignee (a single free-text string) with a proper
		// registry: an "assignees" table (so the same person is one
		// identity everywhere, not a string that has to match byte-for-byte
		// across cards) plus a card_assignees join table (so a card can
		// have more than one). Existing assignee strings are preserved by
		// splitting on commas — the only way the old single-field UI let
		// someone record more than one person ("Claude, JMThomas") — and
		// registering each name separately, before the old column is
		// dropped. SQLite has supported ALTER TABLE ... DROP COLUMN
		// natively since 3.35, so the column drop itself doesn't need the
		// old table-rebuild dance migrateBoards used; splitting names does
		// need real string handling SQL doesn't have, hence the run func.
		version: 7,
		desc:    "add assignee registry (splitting comma-separated names); drop cards.assignee",
		run:     migrateAssigneeRegistry,
	},
	{
		// Per-board ticket numbers (Jira/SalesForce-style): a monotonically
		// increasing counter (boards.next_ticket_no) rather than
		// MAX(ticket_no)+1 over existing cards — deleting the
		// highest-numbered card would let MAX()+1 hand out that same
		// number again, which a ticketing system's numbers must never do
		// (they're permanent references, not just a display order like
		// position). Existing cards are backfilled in id order, per
		// board, since ticket numbers didn't exist before this version and
		// their creation order is the only ordering that still makes sense
		// retroactively.
		version: 8,
		desc:    "add per-board ticket numbers",
		run:     migrateTicketNumbers,
	},
	{
		// Enables real Gantt-chart duration bars (start -> due) instead of
		// single-point due-date markers. Same shape as v2's due_date
		// addition — a plain nullable column, no backfill, since start
		// dates are a genuinely new concept with no prior data to carry
		// forward.
		version: 9,
		desc:    "add cards.start_date",
		stmts: []string{
			`ALTER TABLE cards ADD COLUMN start_date TEXT`,
		},
	},
	{
		// Maps a Concord channel to the Tukan board it hosts, for server
		// mode (Concord plugin integration). A separate table rather than
		// a column on boards — keeps Concord-specific linkage out of the
		// core kanban schema, same instinct as the assignee registry being
		// its own table rather than a column on cards. channel_id is
		// Concord's channel UUID stored as text (Tukan has no native UUID
		// type/dependency otherwise); a board can be mapped to at most one
		// channel, enforced by the primary key being channel_id itself
		// (not board_id) since the relationship is one Concord channel ->
		// one Tukan board, not the reverse.
		version: 10,
		desc:    "add plugin_channels (Concord channel -> Tukan board mapping)",
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS plugin_channels (
				channel_id TEXT    PRIMARY KEY,
				board_id   INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE
			);`,
		},
	},
}

func migrateTicketNumbers(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`ALTER TABLE boards ADD COLUMN next_ticket_no INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE cards ADD COLUMN ticket_no INTEGER`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}

	boardRows, err := tx.Query(`SELECT id FROM boards`)
	if err != nil {
		return err
	}
	var boardIDs []int64
	for boardRows.Next() {
		var id int64
		if err := boardRows.Scan(&id); err != nil {
			boardRows.Close()
			return err
		}
		boardIDs = append(boardIDs, id)
	}
	if err := boardRows.Close(); err != nil {
		return err
	}

	for _, boardID := range boardIDs {
		cardRows, err := tx.Query(
			`SELECT c.id FROM cards c JOIN lanes l ON l.id = c.lane_id WHERE l.board_id = ? ORDER BY c.id`,
			boardID,
		)
		if err != nil {
			return err
		}
		var cardIDs []int64
		for cardRows.Next() {
			var id int64
			if err := cardRows.Scan(&id); err != nil {
				cardRows.Close()
				return err
			}
			cardIDs = append(cardIDs, id)
		}
		if err := cardRows.Close(); err != nil {
			return err
		}

		for i, cardID := range cardIDs {
			if _, err := tx.Exec(`UPDATE cards SET ticket_no=? WHERE id=?`, i+1, cardID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE boards SET next_ticket_no=? WHERE id=?`, len(cardIDs)+1, boardID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func migrateAssigneeRegistry(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS assignees (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS card_assignees (
			card_id     INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
			assignee_id INTEGER NOT NULL REFERENCES assignees(id) ON DELETE CASCADE,
			PRIMARY KEY (card_id, assignee_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_card_assignees_assignee ON card_assignees(assignee_id);`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}

	rows, err := tx.Query(`SELECT id, assignee FROM cards`)
	if err != nil {
		return err
	}
	type cardAssignee struct {
		cardID int64
		name   string
	}
	var links []cardAssignee
	for rows.Next() {
		var cardID int64
		var raw string
		if err := rows.Scan(&cardID, &raw); err != nil {
			rows.Close()
			return err
		}
		for _, name := range strings.Split(raw, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				links = append(links, cardAssignee{cardID: cardID, name: name})
			}
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}

	assigneeID := make(map[string]int64, len(links))
	for _, l := range links {
		if _, ok := assigneeID[l.name]; ok {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO assignees (name) VALUES (?) ON CONFLICT(name) DO NOTHING`, l.name); err != nil {
			return err
		}
		var id int64
		if err := tx.QueryRow(`SELECT id FROM assignees WHERE name=?`, l.name).Scan(&id); err != nil {
			return err
		}
		assigneeID[l.name] = id
	}

	for _, l := range links {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO card_assignees (card_id, assignee_id) VALUES (?, ?)`,
			l.cardID, assigneeID[l.name],
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`ALTER TABLE cards DROP COLUMN assignee`); err != nil {
		return err
	}

	return tx.Commit()
}

// migrateBoards adds a boards table and scopes lanes to one via a
// NOT NULL board_id FK. SQLite can't add a NOT NULL/REFERENCES column to a
// populated table, so lanes is rebuilt (new table, copy, drop, rename) —
// the standard SQLite "12-step" ALTER TABLE procedure. That requires
// foreign_keys=OFF for the duration (cards.lane_id references lanes(id),
// and the target table is being dropped/recreated mid-migration), which
// can't be toggled inside an active transaction — hence the run escape
// hatch instead of a plain stmts list.
//
// Every pre-existing lane backfills to board_id=1 ("Main Board"), so
// upgrading installs keep exactly the board/lane/card layout they had.
func migrateBoards(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer db.Exec("PRAGMA foreign_keys=ON")

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS boards (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			position   INTEGER NOT NULL DEFAULT 0,
			color      TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TRIGGER IF NOT EXISTS boards_updated_at
			AFTER UPDATE ON boards FOR EACH ROW
		BEGIN
			UPDATE boards SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
		END;`,
		`INSERT OR IGNORE INTO boards (id, name, position) VALUES (1, 'Main Board', 0);`,
		`DROP TABLE IF EXISTS lanes_new;`,
		`CREATE TABLE lanes_new (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
			name     TEXT    NOT NULL,
			position INTEGER NOT NULL,
			color    TEXT
		);`,
		`INSERT INTO lanes_new (id, board_id, name, position, color)
			SELECT id, 1, name, position, color FROM lanes;`,
		`DROP TABLE lanes;`,
		`ALTER TABLE lanes_new RENAME TO lanes;`,
		`CREATE INDEX IF NOT EXISTS idx_lanes_board_position ON lanes(board_id, position);`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func execStmtsInTx(db *sql.DB, stmts []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// applyMigration runs a single migration's statements (or its run func) and
// then bumps PRAGMA user_version to its version. Exported to the package for
// tests that need to apply migrations one at a time.
func applyMigration(db *sql.DB, m migration) error {
	var err error
	if m.run != nil {
		err = m.run(db)
	} else {
		err = execStmtsInTx(db, m.stmts)
	}
	if err != nil {
		return fmt.Errorf("migration %d (%s): %w", m.version, m.desc, err)
	}

	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", m.version)); err != nil {
		return fmt.Errorf("set schema version %d: %w", m.version, err)
	}
	return nil
}

// runMigrations applies every migration newer than the database's current
// PRAGMA user_version, in order, bumping the version after each one.
func runMigrations(db *sql.DB) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return err
		}
	}
	return nil
}
