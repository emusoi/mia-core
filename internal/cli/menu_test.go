package cli

import (
	"slices"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
)

func TestTheMenuCountsWhatCanGoAndKeysEveryPlugin(t *testing.T) {
	old := time.Now().Add(-30 * 24 * time.Hour)
	local := []app.Listing{
		{Name: "app", Main: true, Adopted: true},
		{Name: "segerea", Adopted: true, LastWork: old},
		{Name: "nkasi", Adopted: true, LastWork: old, Session: true},
		{Name: "agent-a1"},
		{Name: "agent-b2", Dirty: true},
	}
	items := menuItems("mia", "new_idea", "", local, []string{"cost", "gc-ish", "quiz"})
	names := map[string]string{}
	keys := map[string]bool{}
	for i := 2; i < len(items); {
		if items[i] == "" {
			i++
			continue
		}
		names[items[i]], i = items[i+1], i+3
		if k := names[items[i-3]]; k != "" && k != "Space" {
			if keys[k] {
				t.Errorf("key %q used twice: %q", k, items)
			}
			keys[k] = true
		}
	}
	for _, want := range []string{"clean up idle  1", "clean up not mia's  1 of 2", "cost", "quiz", "radar"} {
		if _, ok := names[want]; !ok {
			t.Errorf("no %q in %q", want, items)
		}
	}
	if items[1] != " mia · new_idea " {
		t.Errorf("title: %q", items[1])
	}
	none := menuItems("mia", "r", "", local[:1], nil)
	if !slices.Contains(none, "-clean up idle  0") || !slices.Contains(none, "-clean up not mia's  0 of 0") {
		t.Errorf("nothing to clean is greyed out: %q", none)
	}
}
