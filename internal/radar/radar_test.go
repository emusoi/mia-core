package radar

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
			{Name: "agent-a1", Path: "/r/.claude/worktrees/agent-a1", Branch: "worktree-agent-a1"},
			{Name: "agent-b2", Path: "/r/.claude/worktrees/agent-b2", Branch: "worktree-agent-b2", Dirty: true},
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
	want := []string{"signals", "kijenge", "elerai", "lemara", "in progress  1", "nduruma",
		"resting  2  — type to find one", "▸ not mia's  2 · 1 changed"}
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

func TestTheMachineIsAlwaysOneLineAway(t *testing.T) {
	d := data()
	f := newForm(d)
	if !f.layered() || f.offers(3) || !f.offers(2) {
		t.Errorf("inside a stack it starts as a layer, and a layer has no machine to choose")
	}
	f.fromAt = 0
	if f.layered() || !f.offers(3) || f.offers(2) {
		t.Errorf("from main the machine is chosen and as is not")
	}
	f.focus = 0
	if f.focus = f.move(1); f.focus != 1 {
		t.Errorf("↓ from the branch goes to from: %d", f.focus)
	}
	if f.focus = f.move(1); f.focus != 3 {
		t.Errorf("↓ from from skips as and lands on on: %d", f.focus)
	}
	if !slices.Equal(f.on, []string{"local", "fedora"}) {
		t.Errorf("on offers this computer and each machine: %v", f.on)
	}
}

func TestWithNothingSignalledItOpensOnTheWorktreeYouWereLastIn(t *testing.T) {
	d := data()
	d.Here = "temeke"
	for i := range d.Local {
		d.Local[i].Signals = nil
		switch d.Local[i].Name {
		case "temeke":
			d.Local[i].Seen = time.Now()
		case "lemara":
			d.Local[i].Seen = time.Now().Add(-time.Hour)
			d.Local[i].Ahead = 2
		}
	}
	d.Machines = nil
	m := model{data: d, open: map[string]bool{}}
	m.rows = m.build()
	if got := m.rows[m.opening()]; got.name != "lemara" || !got.last {
		t.Errorf("opens on %q; want lemara, last switched to, so ⏎ flips back", got.name)
	}
}

func TestAMachinesWorktreesHaveTheirOwnPlaceAndNeverRest(t *testing.T) {
	d := data()
	d.Machines[1].Worktrees = append(d.Machines[1].Worktrees, app.Listing{Name: "moshono", Branch: "test2"})
	m := model{data: d, open: map[string]bool{}}
	got := labels(m.build())
	at := slices.Index(got, "on fedora  1")
	if at < 0 || got[at+1] != "moshono" {
		t.Errorf("an idle worktree on fedora is listed under on fedora: %q", got)
	}
	if !slices.Contains(got, "elerai") || slices.Index(got, "elerai") > slices.Index(got, "in progress  1") {
		t.Errorf("one with a signal still leads, under signals: %q", got)
	}
}

func TestFromAMachineTheComputerYouCameFromIsOneSwitchAway(t *testing.T) {
	d := data()
	d.Away = &Away{From: "laptop", Here: true, Worktrees: []app.Listing{{Name: "mianzini", Branch: "dash"}, {Name: "kisongo", Branch: "x", Machine: "box"}}}
	m := model{data: d, open: map[string]bool{}}
	rows := m.build()
	got := labels(rows)
	if !slices.Contains(got, "on laptop  1") || !slices.Contains(got, "on box  1") {
		t.Fatalf("its worktrees listed by where they are: %q", got)
	}
	for _, r := range rows {
		if r.label == "mianzini" {
			if next, _ := switchTo(r)(m); next.chosen == nil || next.chosen.Return != "mianzini" {
				t.Errorf("⏎ goes back for mianzini: %+v", next.chosen)
			}
		}
		if r.label == "kisongo" {
			if next, _ := switchTo(r)(m); next.chosen == nil || next.chosen.Return != "kisongo@box" {
				t.Errorf("⏎ goes back, then on to box: %+v", next.chosen)
			}
		}
	}
}

func TestTypingOffersToMoveAWorktreeToWhereItIsNot(t *testing.T) {
	m := model{data: data(), open: map[string]bool{}, query: "temeke"}
	if got := labels(m.build()); !slices.Contains(got, "move to fedora — its changes and windows too") {
		t.Errorf("a worktree here can move to fedora: %q", got)
	}
	m.query = "elerai"
	if got := labels(m.build()); !slices.Contains(got, "move here — its changes and windows too") {
		t.Errorf("a worktree on fedora can come here: %q", got)
	}
}

func TestTypingABranchThatIsNotThereOffersToMakeIt(t *testing.T) {
	m := model{data: data(), open: map[string]bool{}, query: "payout-retries"}
	m.data.Prefix = "jk/"
	rows := m.build()
	last := rows[len(rows)-1]
	if last.label != "new worktree  payout-retries" {
		t.Fatalf("offered %q", labels(rows))
	}
	next, _ := last.act(m)
	if next.mode != making || next.form.branch != "jk/payout-retries" {
		t.Errorf("⏎ opens the form with the branch, prefix and all: %q", next.form.branch)
	}
}

func TestTheMakingIsReadAsSteps(t *testing.T) {
	state := func(steps []step) string {
		var out []string
		for _, s := range steps {
			out = append(out, s.name+":"+s.state)
		}
		return strings.Join(out, " ")
	}
	f := form{lines: []string{"kalenga: setting up with npm ci", "npm warn deprecated eslint@9", "added 812 packages"}}
	steps := f.steps()
	if got := state(steps); got != "worktree:done setup:run session:wait" {
		t.Errorf("while setup runs: %s", got)
	}
	if steps[0].detail != "kalenga" || steps[1].detail != "npm ci" || steps[1].live != "added 812 packages" {
		t.Errorf("details: %+v", steps)
	}
	f.lines = append(f.lines, "kalenga — jk/test2 in a worktree of its own", "/home/me/app.kalenga")
	f.done = true
	if got := state(f.steps()); got != "worktree:done setup:done session:done" {
		t.Errorf("when made: %s", got)
	}
	failed := form{lines: []string{"kalenga: setting up with npm ci", "npm ERR! 404"}, done: true, err: errors.New("exit status 1")}
	if got := state(failed.steps()); got != "worktree:done setup:fail session:wait" {
		t.Errorf("a failed setup is the step that failed: %s", got)
	}
	shipping := form{lines: []string{"shipping this mia to fedora (linux/amd64)…"}}
	if got := state(shipping.steps()); got != "ship mia:run worktree:run session:wait" {
		t.Errorf("shipping first: %s", got)
	}
}

func TestArrivingAnotherWayItShowsTheComputerButCannotGoBack(t *testing.T) {
	d := data()
	d.Away = &Away{From: "laptop", Worktrees: []app.Listing{{Name: "mianzini", Branch: "dash"}}}
	m := model{data: d, open: map[string]bool{}}
	for _, r := range m.build() {
		if r.label == "mianzini" {
			next, _ := switchTo(r)(m)
			if next.chosen != nil || !strings.Contains(next.confirm, "switch from laptop") {
				t.Errorf("no way back from here: chosen %+v, said %q", next.chosen, next.confirm)
			}
			return
		}
	}
	t.Error("the laptop's worktrees are still listed")
}

func TestTypingFindsAPluginsCommandAndRunsItInThePopup(t *testing.T) {
	d := data()
	d.Verbs = []Verb{{Name: "cost", Help: "what each worktree's coding agents have cost"}}
	m := model{data: d, open: map[string]bool{}, query: "cost"}
	for _, r := range m.build() {
		if strings.HasPrefix(r.label, "cost") {
			next, _ := r.act(m)
			if next.chosen == nil || !slices.Equal(next.chosen.Argv, []string{"cost"}) || !next.chosen.Wait {
				t.Errorf("⏎ runs mia cost and keeps its output up: %+v", next.chosen)
			}
			return
		}
	}
	t.Errorf("cost not offered: %q", labels(m.build()))
}

func TestNotMiasFoldAwayAndOpenToWhatCanGo(t *testing.T) {
	m := model{data: data(), open: map[string]bool{}}
	m.rows = m.build()
	at := slices.Index(labels(m.rows), "▸ not mia's  2 · 1 changed")
	m.cursor = at
	next, _ := m.browseKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := labels(next.(model).rows)
	want := []string{"▾ not mia's  2 · 1 changed", "agent-a1", "agent-b2", "remove the 1 clean ones — branches stay"}
	if !slices.Equal(got[at:], want) {
		t.Errorf("opened others = %q\nwant %q", got[at:], want)
	}
	if m := (model{data: data()}); len(m.worktrees()) == 0 || m.filters()[0].keep(m.worktrees()[len(m.data.Local)-2]) {
		t.Errorf("others are not counted in all")
	}
}

func TestRemovingOneMiaDidNotMakeGoesOnlyWhenClean(t *testing.T) {
	m := model{data: data(), open: map[string]bool{"·not mia's": true}}
	press := func(r row) model {
		next, _ := removal(r)(m)
		return next
	}
	clean := row{name: "agent-a1", dir: "/r/.claude/worktrees/agent-a1", other: true, listing: &app.Listing{}}
	got := press(clean)
	if done, _ := got.browseKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}); !slices.Equal(done.(model).chosen.Argv, []string{"gc", "--others", "--apply", "/r/.claude/worktrees/agent-a1"}) {
		t.Errorf("a clean one goes through gc --others: %+v", done.(model).chosen)
	}
	if got := press(row{name: "agent-b2", other: true, listing: &app.Listing{Dirty: true}}); !strings.Contains(got.confirm, "uncommitted") {
		t.Errorf("one with changes is refused: %q", got.confirm)
	}
}
