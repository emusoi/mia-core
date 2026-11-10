package session

import "testing"

func TestAMiaSessionRunsMiaInItsStatusLineAndShowsNothing(t *testing.T) {
	if got, want := quietStatus("/go/bin/mia", false), `#('/go/bin/mia' 'status-line' '--quiet' "#{pane_current_path}")`; got != want {
		t.Errorf("here: %q, want %q", got, want)
	}
	// not on a machine, whose mia is elsewhere, and never a test binary, which would run its tests
	if got := quietStatus("/go/bin/mia", true) + quietStatus("/tmp/session.test", false); got != "" {
		t.Errorf("nothing runs: %q", got)
	}
}
