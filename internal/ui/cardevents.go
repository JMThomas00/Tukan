package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JMThomas00/tukan/internal/models"
	"github.com/JMThomas00/tukan/internal/styles"
)

type cardEventsSubMode int

const (
	cardEventsBrowsing cardEventsSubMode = iota
	cardEventsComposing
)

// cardEventsComposeMsg requests posting a new comment while the thread
// stays open — the modal doesn't close on submit, matching the checklist's
// stay-open pattern rather than the card-form/label-picker close-on-submit one.
type cardEventsComposeMsg struct {
	cardID int64
	body   string
}

// cardEventsClosedMsg is emitted on esc.
type cardEventsClosedMsg struct{}

// CardEventsModel is the per-card comment + activity thread. Lazy-loaded:
// it opens showing a loading placeholder, since a thread can grow
// unbounded and — unlike labels/checklists — isn't worth bulk-loading for
// every card on every board load.
type CardEventsModel struct {
	cardID    int64
	cardTitle string
	loading   bool
	events    []models.CardEvent
	scroll    int
	mode      cardEventsSubMode
	input     textarea.Model
	width     int
	height    int
}

// NewCardEvents creates a card-events modal. The caller is expected to also
// fire cmdLoadCardEvents so it stops showing the loading placeholder.
func NewCardEvents(cardID int64, cardTitle string, w, h int) CardEventsModel {
	ta := textarea.New()
	ta.Placeholder = "Write a comment..."
	ta.CharLimit = 500
	ta.SetWidth(50)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	return CardEventsModel{
		cardID:    cardID,
		cardTitle: cardTitle,
		loading:   true,
		input:     ta,
		width:     w,
		height:    h,
	}
}

func (c CardEventsModel) Init() tea.Cmd {
	return nil
}

func (c CardEventsModel) Update(msg tea.Msg) (CardEventsModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	km := DefaultKeyMap

	if c.mode == cardEventsComposing {
		return c.updateComposing(keyMsg, km)
	}
	return c.updateBrowsing(keyMsg, km)
}

func (c CardEventsModel) updateBrowsing(msg tea.KeyMsg, km KeyMap) (CardEventsModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		return c, func() tea.Msg { return cardEventsClosedMsg{} }

	case key.Matches(msg, km.MoveUp):
		if c.scroll > 0 {
			c.scroll--
		}

	case key.Matches(msg, km.MoveDown):
		if c.scroll < len(c.events)-1 {
			c.scroll++
		}

	case msg.String() == "a":
		if !c.loading {
			c.mode = cardEventsComposing
			c.input.SetValue("")
			c.input.Focus()
		}
	}
	return c, nil
}

func (c CardEventsModel) updateComposing(msg tea.KeyMsg, km KeyMap) (CardEventsModel, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Cancel):
		c.mode = cardEventsBrowsing
		c.input.Blur()
		return c, nil

	case key.Matches(msg, km.Submit):
		body := strings.TrimSpace(c.input.Value())
		if body == "" {
			return c, nil
		}
		c.mode = cardEventsBrowsing
		c.input.Blur()
		return c, func() tea.Msg {
			return cardEventsComposeMsg{cardID: c.cardID, body: body}
		}
	}

	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func (c CardEventsModel) View() string {
	var b strings.Builder

	title := c.cardTitle
	if title == "" {
		title = "History"
	}
	b.WriteString(styles.FormLabelStyle.Render("History — "+title) + "\n\n")

	switch {
	case c.loading:
		b.WriteString(styles.CardNoteStyle.Render("Loading…") + "\n")
	case len(c.events) == 0:
		b.WriteString(styles.CardNoteStyle.Render("No activity yet") + "\n")
	default:
		for _, e := range c.events {
			ts := e.CreatedAt.Format("Jan 2 15:04")
			if e.Kind == models.EventActivity {
				b.WriteString(styles.CardNoteStyle.Render(fmt.Sprintf("• %s — %s", e.Body, ts)) + "\n")
				continue
			}
			actor := "you"
			if e.Actor != nil && *e.Actor != "" {
				actor = *e.Actor
			}
			header := styles.FormLabelStyle.Render(actor) + "  " + styles.HelpDescStyle.Render(ts)
			b.WriteString(header + "\n" + e.Body + "\n")
		}
	}

	if c.mode == cardEventsComposing {
		b.WriteString("\n" + c.input.View() + "\n\n")
		b.WriteString(styles.HelpDescStyle.Render("ctrl+s post  esc cancel"))
	} else {
		b.WriteString("\n" + styles.HelpDescStyle.Render("↑/↓ scroll  a comment  esc close"))
	}

	box := styles.ModalBoxStyle.Width(60).Render(b.String())
	if c.width > 0 && c.height > 0 {
		return lipgloss.Place(c.width, c.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
