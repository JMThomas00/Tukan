package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds all key bindings for the application.
type KeyMap struct {
	// Board navigation
	MoveLeft  key.Binding
	MoveRight key.Binding
	MoveUp    key.Binding
	MoveDown  key.Binding

	// Card actions
	NewCard    key.Binding
	EditCard   key.Binding
	DeleteCard key.Binding
	MoveCard   key.Binding

	// Board actions
	SwitchBoard   key.Binding
	LabelPicker   key.Binding
	Checklist     key.Binding
	CardEvents    key.Binding
	Filter        key.Binding
	ThemeSwitcher key.Binding
	LaneManager   key.Binding
	GanttView     key.Binding

	// Move-card mode
	DropCard key.Binding

	// Form navigation
	NextField key.Binding
	PrevField key.Binding
	Submit    key.Binding

	// Confirm delete
	Confirm key.Binding

	// Global
	Cancel key.Binding
	Quit   key.Binding
}

// DefaultKeyMap is the application's default key configuration.
var DefaultKeyMap = KeyMap{
	MoveLeft: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("←/h", "lane left"),
	),
	MoveRight: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("→/l", "lane right"),
	),
	MoveUp: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("↑/k", "card up"),
	),
	MoveDown: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("↓/j", "card down"),
	),
	NewCard: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	EditCard: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit"),
	),
	DeleteCard: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	MoveCard: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "move"),
	),
	SwitchBoard: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "boards"),
	),
	LabelPicker: key.NewBinding(
		key.WithKeys("L"),
		key.WithHelp("L", "labels"),
	),
	Checklist: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "checklist"),
	),
	CardEvents: key.NewBinding(
		key.WithKeys("v"),
		key.WithHelp("v", "history"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	ThemeSwitcher: key.NewBinding(
		key.WithKeys("T"),
		key.WithHelp("T", "theme"),
	),
	LaneManager: key.NewBinding(
		key.WithKeys("S"),
		key.WithHelp("S", "lanes"),
	),
	GanttView: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "gantt"),
	),
	DropCard: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "drop here"),
	),
	NextField: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	PrevField: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Submit: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "save"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "yes"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
