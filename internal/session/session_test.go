package session_test

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/session"
)

func TestEverySessionTargetIsExact(t *testing.T) {
	for _, target := range []string{
		session.Target("tanga", ""),
		session.Target("tanga", "web-server"),
	} {
		if !strings.HasPrefix(target, "=") {
			t.Errorf("target %q is not exact-matched — it would also match a longer session name", target)
		}
	}
	if got := session.Target("tanga", ""); got != "=mia-tanga:" {
		t.Errorf("session target = %q, want a trailing colon so pane commands resolve it", got)
	}
	if got := session.Target("tanga", "web-server"); got != "=mia-tanga:web-server" {
		t.Errorf("window target = %q", got)
	}
}

func TestDetachedSessionsAreCreatedWideEnoughToRead(t *testing.T) {
	if session.GeometryWidth < 120 {
		t.Errorf("sessions are created %d columns wide; a pane read at that width truncates", session.GeometryWidth)
	}
	if session.GeometryHeight < 30 {
		t.Errorf("sessions are created %d rows tall; a snapshot loses its most recent lines", session.GeometryHeight)
	}
}

func TestWindowsWithTheSameNameKeepTheirDistinctIndices(t *testing.T) {
	windows := session.WindowsFrom([]session.Pane{
		{Index: 1, Window: "zsh", PID: 101},
		{Index: 1, Window: "zsh", PID: 102},
		{Index: 3, Window: "zsh", PID: 103, Active: true},
	})
	if len(windows) != 2 {
		t.Fatalf("windows = %+v, want both zsh windows and one entry per tmux window", windows)
	}
	if windows[0].Index != 1 || windows[1].Index != 3 || !windows[1].Active {
		t.Errorf("window indices and active state = %+v", windows)
	}
	if windows[0].PID != 101 || windows[1].PID != 103 {
		t.Errorf("window pids = %d, %d; want each window's first pane, so a plugin can tell what runs in it", windows[0].PID, windows[1].PID)
	}
}

func TestAWindowNamedLikeAVersionIsTargetedByIndex(t *testing.T) {
	if got := (session.Pane{Index: 1, Window: "2.1.289"}).Ref(); got != "1" {
		t.Errorf("ref = %q, want the index", got)
	}
	if got := (session.Pane{Index: 2, Window: "web-server"}).Ref(); got != "web-server" {
		t.Errorf("ref = %q, want the name", got)
	}
}
