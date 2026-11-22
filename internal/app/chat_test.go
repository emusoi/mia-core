package app_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

// keysAgent stands for an agent's program: it shows each byte typed into
// its window, in hex.
const keysAgent = `#!/bin/sh
stty -icanon -echo -icrnl min 1
printf 'ready '
while :; do printf '%s ' "$(dd bs=1 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n')"; done
`

// What a chat does in an agent's window is the same keys for every agent
// mia knows, the window said by its number as a chat says it: what is
// sent is typed as it is with Enter after it, an answer is its option's
// digit with no Enter, and stop is the agent's key, Escape for each of
// these. A chat is told what the window runs: the agent, whether mia
// reads its conversations, and whether it is switched while it runs.
func TestSendAnswerAndStopAreTheSameKeysForEveryAgent(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	dir, bin := t.TempDir(), t.TempDir()
	program := filepath.Join(bin, "keys")
	if err := os.WriteFile(program, []byte(keysAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Root: dir}
	record := model.Record{Name: "typed", Path: dir}
	type agent struct {
		name        string
		reads, live bool
	}
	want := []agent{{"claude", true, true}, {"codex", true, true}, {"pi", true, true}, {"opencode", true, false}, {"agy", true, false}, {"gemini", false, false}, {"cursor-agent", false, false}}
	var known []agent
	for _, one := range a.AgentModels() {
		known = append(known, agent{name: one.Name})
	}
	if !slices.EqualFunc(known, want, func(x, y agent) bool { return x.name == y.name }) {
		t.Fatalf("the agents mia knows are %+v: this table says what a chat does for each", known)
	}
	sent := "say it; go on"
	keys := ""
	for _, b := range []byte(sent + "\r2\x1b") {
		keys += " " + hex.EncodeToString([]byte{b})
	}
	for _, c := range want {
		if _, err := a.NewWindow(record, c.name, session.What{}, []string{program}); err != nil {
			t.Fatal(err)
		}
		window := ""
		for _, w := range a.Windows(record) {
			if w.Name == c.name {
				window = strconv.Itoa(w.Index)
			}
		}
		shows(t, record.Name, window, "ready")
		if runs := a.WindowRuns(record, window); runs.Agent != c.name || runs.Reads != c.reads || runs.Live != c.live {
			t.Errorf("window %s runs %+v, want %+v", window, runs, c)
		}
		if _, on := a.AsksInWindow(record, window); on {
			t.Errorf("%s asks nothing, and is said to", c.name)
		}
		if err := a.SendToWindow(record, window, sent); err != nil {
			t.Fatalf("%s: send: %v", c.name, err)
		}
		if err := a.PressInWindow(record, window, "2"); err != nil {
			t.Fatalf("%s: press: %v", c.name, err)
		}
		if err := a.StopInWindow(record, window); err != nil {
			t.Fatalf("%s: stop: %v", c.name, err)
		}
		shows(t, record.Name, window, "ready"+keys)
	}
	if runs := a.WindowRuns(record, "99"); runs != (app.Runs{}) {
		t.Errorf("a window that is not there runs %+v", runs)
	}
}

// What a window's screen asks is said to a chat with its options, the
// option the agent's cursor is on among them.
func TestAChatIsToldWhatAWindowsScreenAsks(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	dir, bin := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "permission"), []byte(permissionAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Root: dir}
	record := model.Record{Name: "asker", Path: dir}
	if _, err := a.NewWindow(record, "agent", session.What{}, []string{filepath.Join(bin, "permission")}); err != nil {
		t.Fatal(err)
	}
	shows(t, record.Name, "agent", "Esc to cancel")
	asks, on := a.AsksInWindow(record, "agent")
	if !on || asks.Text != "Do you want to proceed?" || asks.Selected != 1 || !slices.Equal(asks.Options, []string{"Yes", "No, and tell Claude what to do differently (esc)"}) {
		t.Fatalf("asks = %+v, %v", asks, on)
	}
	if err := a.PressInWindow(record, "agent", "1"); err != nil {
		t.Fatal(err)
	}
	shows(t, record.Name, "agent", "answered 1")
	if _, on := a.AsksInWindow(record, "agent"); on {
		t.Error("answered, and it still asks")
	}
	if _, on := a.AsksInWindow(record, "nobody"); on {
		t.Error("a window that is not there asks")
	}
}

// An agent mia started by a shell, or one behind node, is told by what is
// in front in its window's terminal: ps is asked of that terminal alone.
func TestAWindowsAgentBehindItsShellIsToldByWhatIsInFront(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	dir, bin := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "pi"), []byte("#!/bin/sh\nprintf 'ready'\nsleep 300\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := &app.App{Root: dir}
	record := model.Record{Name: "behind", Path: dir}
	if _, err := a.NewWindow(record, "work", session.What{Kind: session.KindAgent}, []string{"sh", "-c", "pi --model anthropic/claude-opus-5; exec sh"}); err != nil {
		t.Fatal(err)
	}
	shows(t, record.Name, "work", "ready")
	if runs := a.WindowRuns(record, "work"); runs.Agent != "pi" || !runs.Reads {
		t.Errorf("the window behind its shell runs %+v", runs)
	}
}
