package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/signal"
)

// A key that went keeps its place: every key after it sends the number it sent before, and
// the retired ones are bound to nothing and given no terminal key.
func TestRetiredKeysKeepEveryOtherKeysNumber(t *testing.T) {
	lines, err := terminalKeys("kitty")
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{`cmd+k send_text all \x1b[99;1~`, `cmd+o send_text all \x1b[99;2~`, `cmd+j send_text all \x1b[99;6~`,
		`cmd+[ send_text all \x1b[99;8~`, `cmd+] send_text all \x1b[99;9~`, `cmd+t send_text all \x1b[99;10~`, `cmd+1 send_text all \x1b[99;14~`, `cmd+9 send_text all \x1b[99;22~`} {
		if !strings.Contains(lines, "map "+kept+"\n") {
			t.Errorf("no %q in %s", kept, lines)
		}
	}
	for _, gone := range []string{"cmd+shift+j ", "cmd+n ", "cmd+. ", "[99;3~", "[99;4~", "[99;5~", "[99;7~"} {
		if strings.Contains(lines, gone) {
			t.Errorf("a retired key is sent: %q in %s", gone, lines)
		}
	}
	snippet := tmuxSnippet("mia")
	for _, gone := range []string{"User42", "User43", "User44", "User46", "bind Space", "bind n ", "bind m ", "bind e ", " radar", "tmux menu", "tmux act"} {
		if strings.Contains(snippet, gone) {
			t.Errorf("a retired key is bound: %q in %s", gone, snippet)
		}
	}
	for _, kept := range []string{"bind -n User40 ", "bind -n User45 ", "bind -n User47 ", "bind -n User61 ", "bind k ", "bind a ", "bind b ", "bind B "} {
		if !strings.Contains(snippet, kept) {
			t.Errorf("no %q in %s", kept, snippet)
		}
	}
}

// What only the radar and the menus did is typed in the one list, each its rows, the first
// Enter's: a worktree on a machine, words to whoever waits, a signal cleared, what can go
// removed, sessions brought back, the agent stopped, a fork, this window's own agent on
// another model, and a verb of the config's menu.
func TestWhatTheRadarAndTheMenusDidIsTypedInTheList(t *testing.T) {
	old := time.Now().Add(-30 * 24 * time.Hour)
	asks := []signal.Signal{{Kind: "asks", Text: "Run it now?", Rank: 10}}
	m := listModel{here: "demo", root: "/d", tree: "kijenge", machines: []string{"fedora", "box"}, down: 2, verbs: []string{"cost"},
		agents: []agentKnown{{Name: "pi", Installed: true}, {Name: "codex", Installed: true, Models: []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		}{{ID: "gpt-5"}, {ID: "gpt-5-mini"}}}},
		local: []app.Listing{
			{Name: "demo", Main: true, Adopted: true, LastWork: old},
			{Name: "kijenge", Adopted: true, Session: true, Signals: asks},
			{Name: "segerea", Adopted: true, LastWork: old},
			{Name: "nkasi", Adopted: true, LastWork: old},
			{Name: "starred", Adopted: true, LastWork: old, Starred: true},
			{Name: "agent-a1"},
			{Name: "agent-b2", Dirty: true},
			{Name: "far", Adopted: true, Machine: "fedora", LastWork: old, Signals: asks},
		}}
	m.stand.Waiting = []app.Stand{{Worktree: "sinza", Repository: "other", Machine: "local"}, {Worktree: "mbezi", Repository: "demo", Machine: "fedora"}, {Worktree: "kijenge", Repository: "demo"}}
	says := func(m listModel, typed string) string {
		m.query = typed
		var words []string
		for _, r := range m.rows() {
			words = append(words, r.words)
		}
		return strings.Join(words, " | ")
	}
	for typed, want := range map[string]string{
		"new fix cart on fed": "new worktree on fix-cart on fedora",
		"new fix on nowhere":  "new worktree on fix-on-nowhere in demo",
		"new on box":          "new worktree on on-box in demo",
		"say yes, go on":      "say to sinza: yes, go on | say to kijenge: yes, go on",
		"clear":               "clear kijenge's signal",
		"clean":               "remove 2 idle worktrees: segerea, nkasi | remove 1 clean worktree mia did not make: agent-a1",
		"back":                "bring back 2 sessions that went down",
		"stop":                "stop the agent in this window",
		"fork try two":        "fork this worktree into try-two",
		"cost":                "run cost",
		"> co mini":           "move this conversation to codex, on gpt-5-mini",
	} {
		if got := says(m, typed); got != want {
			t.Errorf("typed %q: %q, want %q", typed, got, want)
		}
		if m.query = typed; m.words() != "" {
			t.Errorf("typed %q is looked for in past conversations", typed)
		}
	}
	// (In the window of its own agent, that agent on another model where it runs.)
	front := m
	front.front = "codex"
	for typed, want := range map[string]string{
		"> co mini": "codex in this window on gpt-5-mini",
		"> co gpt":  "codex in this window on gpt-5 | codex in this window on gpt-5-mini",
		"> pi":      "move this conversation to pi",
	} {
		if got := says(front, typed); got != want {
			t.Errorf("in codex's window, typed %q: %q, want %q", typed, got, want)
		}
	}
	// (Nothing to do is no row; nowhere to fork from, no fork.)
	none := listModel{here: "demo", root: "/d"}
	for _, typed := range []string{"clean", "back", "clear", "say hi", "fork x", "cost"} {
		if got := says(none, typed); got != "" {
			t.Errorf("with nothing to do, typed %q: %q", typed, got)
		}
	}
	help := says(m, "?")
	for _, want := range []string{"new a-branch on fedora", "say yes, go on", "clear", "clean", "back", "stop", "fork a-branch", "> its agent opus", "cost"} {
		if !strings.Contains(help, want) {
			t.Errorf("? says nothing of %q: %s", want, help)
		}
	}
}

func TestPrefixAGoesRoundWhatNeedsYouBestFirst(t *testing.T) {
	sig := func(rank int) []signal.Signal { return []signal.Signal{{Kind: "k", Rank: rank}} }
	local := []app.Listing{
		{Name: "busy", Signals: sig(60)},
		{Name: "done", Signals: sig(30)},
		{Name: "asks", Signals: sig(10)},
		{Name: "quiet"},
	}
	for here, want := range map[string]string{"quiet": "asks", "asks": "done", "done": "asks"} {
		if got := nextNeedingYou(local, here); got != want {
			t.Errorf("from %s: %q, want %q", here, got, want)
		}
	}
	if got := nextNeedingYou(local[:1], "busy"); got != "" {
		t.Errorf("a busy signal does not call for you: %q", got)
	}
}
