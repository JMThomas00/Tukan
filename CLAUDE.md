# Tukan — Claude Reference

## Project Overview
Tukan is a TUI-based Kanban board built in Go using the Charm.land library stack. It is a personal productivity tool designed to be simple but architecturally flexible for future expansion.

## Module
```
github.com/JMThomas00/tukan
```

## Key Dependencies
| Package | Version | Purpose |
|---|---|---|
| `github.com/charmbracelet/bubbletea` | v1.2.4 | TUI framework (Elm Architecture / MVU) |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Terminal styling and layout |
| `github.com/charmbracelet/bubbles` | v0.20.0 | UI components (textinput, textarea) |
| `modernc.org/sqlite` | v1.34.4 | Pure-Go SQLite, no CGO required |

## Directory Structure
```
d:\Tukan\
├── cmd/tukan/main.go               Entry point: DB init, seed, tea.NewProgram
├── internal/
│   ├── config/
│   │   ├── config.go               Config struct and Default() — DB path, splash duration
│   │   └── assets.go               //go:embed logo.txt — logo embedded at compile time
│   │   └── logo.txt                Copy of TucanLogo.txt used for embedding
│   ├── database/
│   │   ├── db.go                   Open/Close/Migrate — WAL pragmas applied on open
│   │   ├── schema.go               SQL CREATE TABLE/TRIGGER/INDEX constants
│   │   ├── lanes.go                ListLanes, CreateLane, SeedDefaultLanes, DeleteLane
│   │   └── cards.go                CRUD + MoveCard (transactional, maintains position)
│   ├── models/
│   │   ├── lane.go                 Lane struct {ID, Name, Position, Color}
│   │   └── card.go                 Card struct {ID, LaneID, Title, Assignee, Note, Position, ...}
│   ├── styles/
│   │   └── styles.go               All lipgloss.Style vars pre-computed in init(). Tokyo Night palette.
│   └── ui/
│       ├── app.go                  Root App model — routes splash ↔ board
│       ├── splash.go               SplashModel — 4s ASCII art splash with tick-based transition
│       ├── board.go                BoardModel — main kanban view, all navigation and rendering
│       ├── cardform.go             CardFormModel — create/edit card modal overlay
│       ├── keys.go                 KeyMap — all key.Binding definitions (DefaultKeyMap)
│       └── help.go                 RenderHelp() — context-sensitive one-line help bar + boardMode type
├── TucanLogo.txt                   Original ASCII art logo asset
├── TucanLogo.jpg                   Original logo image (reference only)
├── Makefile                        build-windows, build, run, clean targets
└── tukan.exe                       Compiled Windows binary
```

## Architecture

### Bubble Tea MVU Pattern
Follows the standard Model/View/Update pattern. The root `App` model owns `SplashModel` and `BoardModel` as value fields and delegates messages to them.

### Message Routing (Critical)
**Board messages must always be routed to the board, even during the splash screen.**
The DB load (`boardLoadedMsg`) fires immediately on startup as a `tea.Cmd`. If the splash screen is active and board messages are routed to the splash, the data is silently dropped and the board gets stuck on "Loading…".

The fix in `app.go`: `splashTickMsg` goes exclusively to splash; all other messages (including board data messages) go to the board.

### Form Message Routing (Critical)
`cardFormDoneMsg` is checked **before** the `formActive` guard in `board.Update`. When the form emits `cardFormDoneMsg` as a `tea.Cmd`, it arrives while `formActive` is still `true`. If the `formActive` branch runs first, the message is re-routed into the form and lost.

### Key API Notes
- **`bubbles@v0.20.0`** uses `key.Matches(msg, binding)` as a **function**, not `binding.Matches(msg)` as a method. The method form does not exist in this version.
- All DB operations are `tea.Cmd` closures (goroutines) — they never block `Update`.

### Rendering
- Lane columns: `lipgloss.JoinHorizontal(lipgloss.Top, laneStrings...)`
- Board + help bar: `lipgloss.JoinVertical(lipgloss.Left, board, statusBar, helpBar)`
- Card form modal overlay: `lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, formView)`
- Splash: `lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, logo)`
- Lane width: `(termWidth - gutters) / len(lanes)`, minimum 18 chars
- Vertical card scroll: manual `laneScroll[]int` per lane (first visible card index)
- Horizontal lane overflow: `laneVPOff int` (leftmost visible lane index, for future expansion)

## Database Schema
```sql
lanes  (id, name, position, color)
cards  (id, lane_id, title, assignee, note, position, created_at, updated_at)
```
- WAL mode + foreign keys enabled on every open
- `ON DELETE CASCADE` on cards when a lane is deleted
- `cards_updated_at` trigger auto-updates `updated_at` on card UPDATE
- `SeedDefaultLanes` is idempotent — only inserts if `COUNT(*) FROM lanes = 0`

## Default Lanes
| Name | Position | Color |
|---|---|---|
| To-Do | 0 | `#7aa2f7` (blue) |
| In Progress | 1 | `#e0af68` (yellow) |
| On Hold | 2 | `#f7768e` (red) |
| Done | 3 | `#9ece6a` (green) |

## Keyboard Bindings
| Key | Mode | Action |
|---|---|---|
| `h` / `←` | normal | Focus previous lane |
| `l` / `→` | normal | Focus next lane |
| `k` / `↑` | normal | Move cursor up in lane |
| `j` / `↓` | normal | Move cursor down in lane |
| `n` | normal | Open create-card form |
| `e` | normal | Open edit-card form |
| `d` | normal | Enter confirm-delete mode |
| `y` | confirm-delete | Confirm deletion |
| `esc` | confirm-delete | Cancel |
| `m` | normal | Enter move-card mode |
| `h` / `←` | moving | Shift target lane left |
| `l` / `→` | moving | Shift target lane right |
| `enter` | moving | Drop card into target lane |
| `esc` | moving | Cancel move |
| `tab` | form | Next field |
| `shift+tab` | form | Previous field |
| `ctrl+s` | form | Save |
| `esc` | form | Cancel |
| `q` / `ctrl+c` | normal | Quit |

## Building
```bash
# Windows binary (production)
GOOS=windows GOARCH=amd64 GOCACHE="$LOCALAPPDATA/go-build" go build -ldflags="-s -w" -o tukan.exe ./cmd/tukan

# Note: `make build-windows` fails in bash make due to GOCACHE being reset to C:\WINDOWS\.
# Run the go build command directly instead.

# Development run
go run ./cmd/tukan
```

## Data Storage
The database is stored at `%APPDATA%\tukan\tukan.db` (Windows). The directory is created automatically on first run.

## Future Expansion Hooks
- **More horizontal lanes**: `laneVPOff int` in `BoardModel` handles viewport overflow. `visibleCount = floor(termWidth / (minLaneWidth + gutter))`.
- **Vertical swim lanes**: Schema stub — add `vertical_lane_id INTEGER` to cards table.
- **Card archival**: Schema stub — add `is_archived INTEGER NOT NULL DEFAULT 0` to cards table.
- **Theming**: `styles/styles.go` uses named color constants — swap the palette block to change the entire theme.
