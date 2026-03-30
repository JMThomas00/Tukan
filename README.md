# Tukan

A keyboard-driven, terminal UI Kanban board built in Go.

```
┌──────────────────────────────────────────────────────────────────────────┐
│  To-Do (2)          In Progress (1)     On Hold (0)     Done (1)         │
│ ╭────────────────╮ ╭────────────────╮  ╭────────────────╮ ╭───────────╮ │
│ │ Fix login bug  │ │ Refactor auth  │  │  (empty)       │ │ Setup CI  │ │
│ │ @alice         │ │ @bob           │  │ Press n to add │ │ @alice    │ │
│ │ Repro w/ token │ ╰────────────────╯  ╰────────────────╯ ╰───────────╯ │
│ ╰────────────────╯                                                        │
│ ╭────────────────╮                                                        │
│ │ Write tests    │                                                        │
│ │ @charlie       │                                                        │
│ ╰────────────────╯                                                        │
├──────────────────────────────────────────────────────────────────────────┤
│ n new  e edit  d del  m move  ←/→ lane  ↑/↓ card  q quit                │
└──────────────────────────────────────────────────────────────────────────┘
```

## Features

- **Four default swim lanes** — To-Do, In Progress, On Hold, Done
- **Card management** — create, edit, delete, and move cards between lanes
- **Card fields** — title, assignee, and an optional note
- **Keyboard-first** — fully navigable without a mouse; vim-style `hjkl` movement supported
- **Persistent storage** — all data saved to a local SQLite database automatically
- **Splash screen** — ASCII art logo on startup with a timed transition to the board
- **Self-contained binary** — no external runtime dependencies, no CGO

## Installation

### Pre-built binary

Download `tukan.exe` from the releases page and place it anywhere on your `PATH`.

### Build from source

Requires Go 1.22 or later.

```bash
git clone https://github.com/JMThomas00/tukan
cd tukan

# Windows
GOOS=windows GOARCH=amd64 GOCACHE="$LOCALAPPDATA/go-build" go build -ldflags="-s -w" -o tukan.exe ./cmd/tukan

# Other platforms
go build -o tukan ./cmd/tukan
```

## Usage

```bash
tukan
```

The application opens with a brief splash screen, then displays the Kanban board. All data is automatically saved to `%APPDATA%\tukan\tukan.db` on Windows (or `~/.config/tukan/tukan.db` on other platforms).

## Keyboard Reference

### Normal Mode

| Key | Action |
|---|---|
| `h` or `←` | Move focus to the left lane |
| `l` or `→` | Move focus to the right lane |
| `k` or `↑` | Move cursor up within the current lane |
| `j` or `↓` | Move cursor down within the current lane |
| `n` | Create a new card in the focused lane |
| `e` | Edit the focused card |
| `d` | Begin deleting the focused card (requires confirmation) |
| `m` | Pick up the focused card to move it to another lane |
| `q` or `Ctrl+C` | Quit |

### Move Mode (`m`)

When a card is picked up with `m`, the status bar shows which card is being moved.

| Key | Action |
|---|---|
| `h` or `←` | Shift the target lane left |
| `l` or `→` | Shift the target lane right |
| `Enter` | Drop the card into the highlighted lane |
| `Esc` | Cancel the move, return the card to its original lane |

### Delete Confirmation (`d`)

| Key | Action |
|---|---|
| `y` | Confirm deletion |
| `Esc` | Cancel |

### Card Form (`n` / `e`)

| Key | Action |
|---|---|
| `Tab` | Move to the next field |
| `Shift+Tab` | Move to the previous field |
| `Ctrl+S` | Save the card |
| `Esc` | Cancel without saving |

The form has three fields:

- **Title** *(required)* — a short description of the task
- **Assignee** *(required)* — the person responsible
- **Note** *(optional)* — free-form text for additional context

## Data Storage

Tukan stores all data in a single SQLite database file:

| Platform | Location |
|---|---|
| Windows | `%APPDATA%\tukan\tukan.db` |
| macOS / Linux | `~/.config/tukan/tukan.db` |

The directory and database are created automatically on first run. To reset the board, delete the `tukan.db` file.

## Project Structure

```
tukan/
├── cmd/tukan/main.go        Entry point
├── internal/
│   ├── config/              Application config and embedded logo asset
│   ├── database/            SQLite layer (schema, migrations, CRUD)
│   ├── models/              Domain types: Lane, Card
│   ├── styles/              Lip Gloss style definitions (Tokyo Night palette)
│   └── ui/                  Bubble Tea models
│       ├── app.go           Root model, screen routing
│       ├── splash.go        Startup splash screen
│       ├── board.go         Main Kanban board
│       ├── cardform.go      Create/edit card modal
│       ├── keys.go          Key binding definitions
│       └── help.go          Contextual help bar
├── TucanLogo.txt            ASCII art logo
├── Makefile                 Build targets
└── CLAUDE.md                Developer/AI reference guide
```

## Tech Stack

| Component | Library |
|---|---|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) v1.2.4 |
| Terminal styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) v1.1.0 |
| UI components | [Bubbles](https://github.com/charmbracelet/bubbles) v0.20.0 |
| Database | [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) v1.34.4 (pure Go, no CGO) |

## Architecture Notes

Tukan follows the [Elm Architecture](https://guide.elm-lang.org/architecture/) (Model/View/Update) as implemented by Bubble Tea.

- **`App`** is the root model. It holds a `SplashModel` and a `BoardModel` and routes messages between them. During the splash phase, board messages (like the initial DB load result) are still delivered to the board so data is ready the moment the splash ends.
- **`BoardModel`** manages all board state: lanes, cards, cursor position, scroll offsets, the current interaction mode, and the embedded `CardFormModel`. All database operations run as asynchronous `tea.Cmd` goroutines and never block the UI.
- **`CardFormModel`** is a modal overlay rendered with `lipgloss.Place` centered over the board. It supports three fields with `Tab`/`Shift+Tab` cycling and `Ctrl+S` to submit.
- **Styles** are pre-computed once in `init()` in `internal/styles/styles.go` and never allocated during rendering.

## Future Directions

The architecture is designed to accommodate these expansions without significant restructuring:

- **Additional lanes** — The board already supports horizontal viewport scrolling (`laneVPOff`) for when lanes overflow the terminal width.
- **Vertical swim lanes** — A `vertical_lane_id` column can be added to the cards table to group rows (e.g., by team or priority).
- **Card archival** — An `is_archived` flag can be added to hide completed work without deleting it.
- **Custom themes** — All colors are defined as named constants in `styles/styles.go`; swapping the palette changes the entire UI.
- **Due dates and priorities** — Additional card fields can be added to the schema and form without restructuring the existing code.
