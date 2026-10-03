package concord

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JMThomas00/Concord/sdk/wire"
	"github.com/JMThomas00/tukan/internal/ui"
	"github.com/google/uuid"
)

// Records: Tukan counts the cards each member creates and finishes (moves
// into a Done lane), unlocks the achievements declared in plugin.toml, and
// sends Concord each member's record; members see it on Settings > About
// > Achievements, with the leaderboard by cards finished.

type memberRecord struct {
	Created  int                  `json:"created"`
	Finished int                  `json:"finished"`
	Unlocked map[string]time.Time `json:"unlocked,omitempty"`
}

// doneLane reports a lane that means "finished".
func doneLane(name string) bool {
	n := strings.ToLower(name)
	for _, w := range []string{"done", "complete", "finished", "shipped", "closed"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

func (s *Server) recordsFile() string {
	dir := "."
	if s.conn != nil && s.conn.DataDir() != "" {
		dir = s.conn.DataDir()
	}
	return filepath.Join(dir, "records.json")
}

func (s *Server) loadRecords() {
	if s.records != nil {
		return
	}
	s.records = map[uuid.UUID]*memberRecord{}
	if data, err := os.ReadFile(s.recordsFile()); err == nil {
		_ = json.Unmarshal(data, &s.records)
	}
}

// countChange adds a member's change to their record: cards they created,
// and cards they moved into a Done lane.
func (s *Server) countChange(by uuid.UUID, before, after ui.ContentSnapshot) {
	if by == uuid.Nil || s.conn == nil {
		return
	}
	old := map[int64]int64{}
	for _, c := range before.Cards {
		old[c.ID] = c.LaneID
	}
	created, finished := 0, 0
	for _, c := range after.Cards {
		was, existed := old[c.ID]
		switch {
		case !existed:
			created++
		case was != c.LaneID && doneLane(after.LaneNames[c.LaneID]) && !doneLane(before.LaneNames[was]):
			finished++
		}
	}
	if created == 0 && finished == 0 {
		return
	}
	s.loadRecords()
	r := s.records[by]
	if r == nil {
		r = &memberRecord{Unlocked: map[string]time.Time{}}
		s.records[by] = r
	}
	r.Created += created
	r.Finished += finished
	now := time.Now().UTC()
	unlock := func(id string, ok bool) {
		if _, done := r.Unlocked[id]; ok && !done {
			r.Unlocked[id] = now
		}
	}
	unlock("first_card", r.Created >= 1)
	unlock("cards_25", r.Created >= 25)
	unlock("first_done", r.Finished >= 1)
	unlock("done_10", r.Finished >= 10)
	unlock("done_100", r.Finished >= 100)
	if data, err := json.MarshalIndent(s.records, "", "  "); err == nil {
		path := s.recordsFile()
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		_ = os.WriteFile(path, data, 0o600)
	}
	rec := wire.PluginRecord{UserID: by, Stats: []wire.PluginStat{
		{Key: "finished", Label: "Cards finished", Value: fmt.Sprint(r.Finished), Num: float64(r.Finished)},
		{Key: "created", Label: "Cards created", Value: fmt.Sprint(r.Created), Num: float64(r.Created)},
	}}
	for id, at := range r.Unlocked {
		rec.Unlocked = append(rec.Unlocked, wire.PluginUnlock{ID: id, At: at})
	}
	_ = s.conn.SendRecord(rec)
}
