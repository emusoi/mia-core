package radar

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/signal"
	"github.com/emusoi/mia-core/internal/stack"
)

func data() Data {
	layers := []stack.Layer{{Branch: "a"}, {Branch: "b", NeedsRestack: true, Parent: "a"}, {Branch: "c", Current: true}}
	return Data{Repo: "app", Root: "/r", Base: "main", Here: "nduruma", HereDir: "/r.nduruma", Branch: "c",
		Local: []app.Listing{
			{Name: "app", Branch: "main", Main: true, Adopted: true},
			{Name: "nduruma", Branch: "c", Adopted: true, Layers: layers, Stack: "machines"},
			{Name: "kijenge", Branch: "jk/retries", Adopted: true, Signals: []signal.Signal{{Kind: "asks", Rank: 10, From: "claude"}}},
			{Name: "temeke", Branch: "auth", Adopted: true},
			{Name: "lemara", Branch: "jk/dates", Adopted: true, Signals: []signal.Signal{{Kind: "done", Rank: 30, From: "claude"}}},
		},
		Machines: []app.MachineState{{Answers: true}, {Machine: runtime.Machine{Name: "fedora"}, Answers: true, Worktrees: []app.Listing{{Name: "elerai", Branch: "jk/land", Signals: []signal.Signal{{Kind: "tests red", Rank: 20, From: "watch"}}}}}},
	}
}

func labels(rows []row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.label)
	}
	return out
}

func TestTheRadarPutsSignalsFirstByRankThenWhatIsInProgress(t *testing.T) {
	m := model{data: data(), open: map[string]bool{}}
	got := labels(m.build())
	want := []string{"signals", "kijenge", "elerai", "lemara", "in progress  1", "nduruma", "resting  2  — type to find one"}
	if !slices.Equal(got, want) {
		t.Errorf("rows = %q\nwant  %q", got, want)
	}
	m.open["nduruma"] = true
	if got := labels(m.build()); !slices.Contains(got, "3  c") || !slices.Contains(got, "1  a") {
		t.Errorf("an open stack lists its layers, top first: %q", got)
	}
}

func TestTypingFindsWorktreesAndWhatToDoWithTheFirst(t *testing.T) {
	m := model{data: data(), open: map[string]bool{}, query: "ele"}
	rows := m.build()
	if rows[1].name != "elerai@fedora" || rows[1].machine != "fedora" {
		t.Errorf("found on fedora: %+v", rows[1])
	}
	got := labels(rows)
	if got[0] != "worktrees" || got[1] != "elerai" || got[2] != "do with elerai" || got[3] != "switch there" {
		t.Errorf("rows = %q", got)
	}
	if slices.Contains(got, "remove it") {
		t.Errorf("a worktree on another machine is not removed from here: %q", got)
	}
	m.query = "new"
	if got := labels(m.build()); !slices.Contains(got, "new worktree") {
		t.Errorf("typing new offers a new worktree: %q", got)
	}
}

func TestTheFormMakesTheWorktreeItSays(t *testing.T) {
	d := data()
	f := newForm(d)
	f.branch = "jk/next"
	if argv, dir := f.argv(d); !slices.Equal(argv, []string{"new", "--stack", "jk/next"}) || dir != "/r.nduruma" {
		t.Errorf("inside a stack it starts as a layer, in place: %v in %s", argv, dir)
	}
	f.asAt = 1
	if argv, _ := f.argv(d); !slices.Equal(argv, []string{"new", "--stack", "--worktree", "jk/next"}) {
		t.Errorf("own worktree: %v", argv)
	}
	f.fromAt, f.onAt = 0, 1
	if argv, dir := f.argv(d); !slices.Equal(argv, []string{"new", "--on", "fedora", "jk/next"}) || dir != "/r" {
		t.Errorf("from main on fedora: %v in %s", argv, dir)
	}
}

func TestWhatWasMadeIsTheLastNameLine(t *testing.T) {
	if got := madeName([]string{"shipping this mia to fedora…", "kalenga — jk/next in a worktree of its own", "/home/me/app.kalenga", "kalenga@fedora — jk/next on fedora"}); got != "kalenga@fedora" {
		t.Errorf("made = %q", got)
	}
}

func TestTheSetupGuessComesFromTheLockfile(t *testing.T) {
	dir := t.TempDir()
	if GuessSetup(dir) != nil {
		t.Error("no lockfile, no guess")
	}
	os.WriteFile(filepath.Join(dir, "package-lock.json"), nil, 0o644)
	if got := strings.Join(GuessSetup(dir), " "); got != "npm ci" {
		t.Errorf("guess = %q", got)
	}
	miaDir := filepath.Join(dir, ".git", "mia")
	if err := writeConfig(miaDir, Guess{Base: "main", Setup: []string{"npm", "ci"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(miaDir, "config.toml"))
	if !strings.Contains(string(got), `base = "main"`) || !strings.Contains(string(got), `setup = ["npm", "ci"]`) {
		t.Errorf("config = %s", got)
	}
}
