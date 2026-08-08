package ui

import (
	"testing"

	"github.com/JMThomas00/tukan/internal/models"
)

func TestCardMatchesFilterEmptyMatchesEverything(t *testing.T) {
	fc := parseFilterQuery("")
	if !cardMatchesFilter(models.Card{Title: "anything"}, nil, fc) {
		t.Fatal("empty filter should match every card")
	}
}

func TestCardMatchesFilterCaseInsensitive(t *testing.T) {
	fc := parseFilterQuery("BUG")
	if !cardMatchesFilter(models.Card{Title: "Fix login bug"}, nil, fc) {
		t.Fatal("filter should be case-insensitive")
	}
}

func TestCardMatchesFilterAcrossFields(t *testing.T) {
	fc := parseFilterQuery("jordan")
	if !cardMatchesFilter(models.Card{Title: "x"}, []models.Assignee{{Name: "jordan"}}, fc) {
		t.Fatal("filter should match against assignee")
	}

	fc = parseFilterQuery("release notes")
	if !cardMatchesFilter(models.Card{Title: "x", Note: "write release notes"}, nil, fc) {
		t.Fatal("filter should match against note")
	}
}

func TestCardMatchesFilterNoMatch(t *testing.T) {
	fc := parseFilterQuery("nonexistent")
	if cardMatchesFilter(models.Card{Title: "Fix login bug", Note: "urgent"}, []models.Assignee{{Name: "jordan"}}, fc) {
		t.Fatal("filter should not match unrelated text")
	}
}

func TestCardMatchesFilterTrimsWhitespace(t *testing.T) {
	fc := parseFilterQuery("  bug  ")
	if !cardMatchesFilter(models.Card{Title: "bug fix"}, nil, fc) {
		t.Fatal("filter query should be trimmed before matching")
	}
}
