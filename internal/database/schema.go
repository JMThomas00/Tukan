package database

const createLanes = `
CREATE TABLE IF NOT EXISTS lanes (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    color    TEXT
);`

const createCards = `
CREATE TABLE IF NOT EXISTS cards (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    lane_id    INTEGER NOT NULL REFERENCES lanes(id) ON DELETE CASCADE,
    title      TEXT    NOT NULL,
    assignee   TEXT    NOT NULL,
    note       TEXT,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createCardsTrigger = `
CREATE TRIGGER IF NOT EXISTS cards_updated_at
    AFTER UPDATE ON cards
    FOR EACH ROW
BEGIN
    UPDATE cards SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;`

const createCardsIndex = `
CREATE INDEX IF NOT EXISTS idx_cards_lane_position
    ON cards(lane_id, position);`
