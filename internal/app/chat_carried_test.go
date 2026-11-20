package app

import "testing"

// What a window's agent goes on from is kept on the window as one line,
// and read back as it was; nothing kept, what does not read and what
// names no file is nothing carried.
func TestWhatAWindowGoesOnFromIsReadBackAsItWasKept(t *testing.T) {
	kept := Carried{From: "claude", ID: "c1", File: "/repo/.git/mia/handoffs/monduli/claude-c1.md", Full: "/repo/full — all.md"}
	if got, on := carriedIn(kept.option() + "\n"); !on || got != kept {
		t.Errorf("read back %+v, %v", got, on)
	}
	for _, option := range []string{"", "\n", "1", "not json", `{"from":"claude","id":"c1"}`} {
		if got, on := carriedIn(option); on {
			t.Errorf("%q is read as %+v", option, got)
		}
	}
}
