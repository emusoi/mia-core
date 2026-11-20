package app

import (
	"slices"
	"testing"
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
