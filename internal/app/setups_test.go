package app

import (
	"os"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/gittest"
)

// Each start or switch is written down, the last usesKept of them; a
// start with nothing chosen is not.
func TestAUseIsWrittenDownAndTheOldestGo(t *testing.T) {
	gittest.Isolate(t)
	a := &App{Root: "/src/mia-phone"}
	if err := a.used("claude", "goba", "local", Chosen{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(usesPath()); err == nil {
		t.Error("nothing chosen was written down")
	}
	for i := range usesKept + 2 {
		if err := a.used("claude", "goba", "local", Chosen{Model: "opus", Effort: string(rune('a' + i%26))}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.used("codex", "olasiti", "fedora", Chosen{Model: "5.1-codex"}); err != nil {
		t.Fatal(err)
	}
	kept := uses()
	if len(kept) != usesKept {
		t.Fatalf("kept %d uses", len(kept))
	}
	if last := kept[len(kept)-1]; last.Agent != "codex" || last.Model != "5.1-codex" || last.Effort != "" || last.Repository != "mia-phone" ||
		last.Worktree != "olasiti" || last.Machine != "fedora" || time.Since(last.At) > time.Minute {
		t.Errorf("the last = %+v", last)
	}
	if first := kept[0]; first.Effort != string(rune('a'+3)) {
		t.Errorf("the oldest kept = %+v, not the fourth written", first)
	}
}
