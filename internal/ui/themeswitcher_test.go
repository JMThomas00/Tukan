package ui

import "testing"

// TestThemeSwitcherDoneMsgHandledBeforeActiveGuard guards the same
// footgun documented for cardFormDoneMsg/formActive: themeSwitcherDoneMsg
// arrives as a Cmd result while themeSwitcherActive is still true, so it
// must be intercepted before that guard — not routed back into the
// (already-closing) switcher and silently dropped.
func TestThemeSwitcherDoneMsgHandledBeforeActiveGuard(t *testing.T) {
	_, b := newTestBoard(t)

	b.themeSwitcherActive = true
	b.themeSwitcher = NewThemeSwitcher(b.currentThemeName, b.width, b.height)

	updated, cmd := b.Update(themeSwitcherDoneMsg{themeName: "dracula"})
	if updated.themeSwitcherActive {
		t.Fatal("themeSwitcherActive still true after themeSwitcherDoneMsg — the done message was swallowed by the guard instead of being handled first")
	}
	if cmd != nil {
		t.Fatal("expected no follow-up command from a committed theme switch")
	}
	if updated.currentThemeName != "dracula" {
		t.Fatalf("currentThemeName = %q, want %q", updated.currentThemeName, "dracula")
	}
}

func TestThemeSwitcherCancelDoesNotChangeCurrentTheme(t *testing.T) {
	_, b := newTestBoard(t)
	b.currentThemeName = "tokyo-night"
	b.themeSwitcherActive = true
	b.themeSwitcher = NewThemeSwitcher(b.currentThemeName, b.width, b.height)

	updated, _ := b.Update(themeSwitcherDoneMsg{cancelled: true})
	if updated.themeSwitcherActive {
		t.Fatal("themeSwitcherActive still true after cancel")
	}
	if updated.currentThemeName != "tokyo-night" {
		t.Fatalf("currentThemeName = %q after cancel, want it unchanged (%q)", updated.currentThemeName, "tokyo-night")
	}
}

// TestNewThemeSwitcherSeedsCursorAtCurrentTheme confirms the switcher
// opens with its cursor on whatever theme is already active, not always
// at index 0 — important for a 45-item list.
func TestNewThemeSwitcherSeedsCursorAtCurrentTheme(t *testing.T) {
	s := NewThemeSwitcher("dracula", 80, 24)
	if s.cursor >= len(s.names) || s.names[s.cursor] != "dracula" {
		t.Fatalf("cursor = %d (name %q), want it seeded at 'dracula'", s.cursor, s.selectedName())
	}
}
