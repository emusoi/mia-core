package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// A conversation asked for whole each time gives what is new, and a turn
// that grew again under its n; nothing when nothing changed.
func TestPollingGivesWhatIsNewAndWhatGrew(t *testing.T) {
	asked := [][]Turn{
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will"}},
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will look"}},
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will look"}},
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will look"}, {N: 3, Kind: turnTool, Text: "Read 3 files"}},
	}
	next := polling(func() ([]Turn, bool, error) {
		turns := asked[0]
		asked = asked[1:]
		return turns, true, nil
	})
	for _, want := range [][]int{{1, 2}, {2}, nil, {3}} {
		turns, found, err := next()
		var got []int
		for _, turn := range turns {
			got = append(got, turn.N)
		}
		if !found || err != nil || !slices.Equal(got, want) {
			t.Errorf("turns %v, found %v, %v; want %v", got, found, err, want)
		}
	}
}

// A reader gated on where its agent writes is asked first, then again
// only once the agent wrote there, or the backstop went by.
func TestWhenWrittenAsksOnlyOnceTheAgentWrote(t *testing.T) {
	db := filepath.Join(t.TempDir(), "opencode.db")
	asked := 0
	next := whenWritten(func() []string { return []string{db, db + "-wal"} }, time.Hour, func() ([]Turn, bool, error) {
		asked++
		return []Turn{{N: asked}}, true, nil
	})
	call := func(want int) {
		t.Helper()
		if _, found, err := next(); !found || err != nil || asked != want {
			t.Fatalf("asked %d times, found %v, %v; want %d", asked, found, err, want)
		}
	}
	call(1) // the first time, whatever is there
	call(1) // nothing written since
	if err := os.WriteFile(db+"-wal", []byte("a page"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(2) // the log written ahead of the database came
	call(2)
	if err := os.WriteFile(db+"-wal", []byte("a longer page"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(3) // and grew

	backstopped := 0
	again := whenWritten(func() []string { return []string{db} }, 0, func() ([]Turn, bool, error) {
		backstopped++
		return nil, false, nil
	})
	for range 3 {
		if _, found, _ := again(); found {
			t.Fatal("found is what it said when asked")
		}
	}
	if backstopped != 3 {
		t.Errorf("past its backstop it is asked whatever is written: %d", backstopped)
	}
}
