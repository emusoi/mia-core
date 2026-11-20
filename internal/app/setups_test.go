package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/config"
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

// A repository's setups are its usual one first — the one used in the
// most of its worktrees, the later used of a tie — then the last used
// first, then what the config starts agents with that was never used. A
// setup with neither a model nor an effort is none.
func TestSetupsAreTheUsualFirstThenTheLastUsed(t *testing.T) {
	gittest.Isolate(t)
	day := func(n int) time.Time { return time.Date(2026, 11, n, 10, 0, 0, 0, time.UTC) }
	history := []Use{
		{"claude", "sonnet", "medium", "mia-phone", "duluti", "local", day(1), ""},
		{"claude", "sonnet", "medium", "mia-phone", "goba", "local", day(2), ""},
		{"claude", "opus", "high", "mia-phone", "olasiti", "local", day(3), "work"}, // an account is kept for the history, and is no part of a setup
		{"claude", "opus", "high", "mia-phone", "sakina", "fedora", day(4), ""},
		{"claude", "sonnet", "medium", "mia-desk", "kijenge", "local", day(5), ""},
		{"claude", "opus", "high", "mia-phone", "olasiti", "local", day(6), ""},
		{"codex", "5.1-codex", "high", "mia-phone", "duluti", "local", day(7), ""},
		{"claude", "", "", "mia-phone", "goba", "local", day(8), ""}, // nothing chosen: written by hand, or by an older mia
	}
	data, _ := json.Marshal(history)
	if err := os.MkdirAll(filepath.Dir(usesPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(usesPath(), data, 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{Config: config.Config{
		Main: config.Main{Model: "sonnet"},
		// opencode is started with no effort, and nobody is no agent
		Agents: map[string]config.Agent{"claude": {Model: "opus", Effort: "high"}, "pi": {Effort: "low"}, "codex": {},
			"opencode": {Effort: "high"}, "nobody": {Model: "x"}},
	}}
	type row struct {
		agent, model, effort string
		uses, sessions       int
		last                 string
		at                   int
		usual                bool
	}
	rowsOf := func(setups []Setup) []row {
		rows := []row{}
		for _, s := range setups {
			r := row{agent: s.Agent, model: s.Model, effort: s.Effort, uses: s.Uses, sessions: s.Sessions, usual: s.Usual}
			if s.Last != nil {
				r.last, r.at = s.Last.Repository+"/"+s.Last.Worktree+"@"+s.Last.Machine, s.Last.At.Day()
			}
			rows = append(rows, r)
		}
		return rows
	}
	// [main]'s, sonnet with [agents.claude]'s effort, then [agents] by name
	mainSet, claudeSet, piSet := row{agent: "claude", model: "sonnet", effort: "high"}, row{agent: "claude", model: "opus", effort: "high"}, row{agent: "pi", effort: "low"}
	for _, c := range []struct {
		repository string
		want       []row
	}{
		{"mia-phone", []row{
			{"claude", "opus", "high", 3, 2, "mia-phone/olasiti@local", 6, true},
			{"codex", "5.1-codex", "high", 1, 1, "mia-phone/duluti@local", 7, false},
			{"claude", "sonnet", "medium", 2, 2, "mia-phone/goba@local", 2, false},
			mainSet, piSet,
		}},
		{"mia-desk", []row{
			{"claude", "sonnet", "medium", 1, 1, "mia-desk/kijenge@local", 5, true},
			mainSet, claudeSet, piSet,
		}},
		{"", []row{
			{"codex", "5.1-codex", "high", 1, 1, "mia-phone/duluti@local", 7, false},
			{"claude", "opus", "high", 3, 2, "mia-phone/olasiti@local", 6, false},
			{"claude", "sonnet", "medium", 3, 3, "mia-desk/kijenge@local", 5, false},
			mainSet, piSet,
		}},
		{"api", []row{mainSet, claudeSet, piSet}},
	} {
		want := c.want
		got := rowsOf(a.Setups(c.repository))
		if len(got) != len(want) {
			t.Errorf("%q: %+v", c.repository, got)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%q, %d: %+v, want %+v", c.repository, i, got[i], want[i])
			}
		}
	}
}
