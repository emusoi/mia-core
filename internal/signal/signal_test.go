package signal

import (
	"testing"
	"time"
)

func TestASignalHasOnePlacePerSourceAndGoesWhenItsSetterSaid(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	now := time.Now()
	set := func(one Signal) {
		t.Helper()
		if err := s.Set(one); err != nil {
			t.Fatal(err)
		}
	}
	set(Signal{Worktree: "kijenge", Kind: "busy", Rank: 60, From: "claude", At: now})
	set(Signal{Worktree: "kijenge", Kind: "asks", Text: "retry?", Rank: 10, From: "claude", At: now})
	set(Signal{Worktree: "kijenge", Kind: "tests red", Rank: 20, From: "watch", At: now})
	set(Signal{Worktree: "elerai", Kind: "done", Rank: 30, From: "claude", At: now, UntilSeen: true})
	set(Signal{Worktree: "elerai", Kind: "deploying", Rank: 40, From: "ci", At: now, Until: now.Add(-time.Second)})

	all, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, one := range all {
		kinds = append(kinds, one.Kind)
	}
	if want := []string{"asks", "tests red", "done"}; len(kinds) != 3 || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] {
		t.Fatalf("by rank, one per source, the expired gone: %v", kinds)
	}

	if err := s.Clear("kijenge", "watch"); err != nil {
		t.Fatal(err)
	}
	if mine := Of(mustLoad(t, s), "kijenge"); len(mine) != 1 || mine[0].From != "claude" {
		t.Errorf("clear takes only its own source's: %+v", mine)
	}
	if err := s.Seen("elerai"); err != nil {
		t.Fatal(err)
	}
	if mine := Of(mustLoad(t, s), "elerai"); len(mine) != 0 {
		t.Errorf("until seen goes when seen: %+v", mine)
	}
	if err := s.Clear("kijenge", ""); err != nil {
		t.Fatal(err)
	}
	if left := mustLoad(t, s); len(left) != 0 {
		t.Errorf("clear without a source takes them all: %+v", left)
	}
}

func mustLoad(t *testing.T, s Store) []Signal {
	t.Helper()
	all, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	return all
}
