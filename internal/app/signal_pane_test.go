package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/signal"
)

func TestASignalFromAPaneThatIsGoneIsForgotten(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-gone-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("asking")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "agent", []string{"sleep", "300"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "other", []string{"sleep", "300"}); err != nil {
		t.Fatal(err)
	}
	pane := func(window string) string {
		out, _ := exec.Command("tmux", "display-message", "-p", "-t", "=mia-"+record.Name+":"+window, "#{pane_id}").Output()
		return strings.TrimSpace(string(out))
	}
	agent, other := pane("agent"), pane("other")
	for from, p := range map[string]string{"claude": agent, "codex": other, "watch": ""} {
		if err := a.Signals.Set(signal.Signal{Worktree: record.Name, Kind: "asks", Rank: 10, From: from, Pane: p, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	exec.Command("tmux", "kill-pane", "-t", agent).Run()
	listings, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	var froms []string
	for _, l := range listings {
		if l.Name == record.Name {
			for _, s := range l.Signals {
				froms = append(froms, s.From)
			}
		}
	}
	if strings.Join(froms, ",") != "codex,watch" && strings.Join(froms, ",") != "watch,codex" {
		t.Errorf("the dead pane's signal goes; a live pane's and a paneless one stay: %v", froms)
	}
	if kept, _ := a.Signals.Load(); len(kept) != 2 {
		t.Errorf("and it is forgotten, not only hidden: %+v", kept)
	}
}

// Standing passes over an ask nobody can answer by the listing's own rule,
// and forgets nothing: it writes nothing.
func TestStandingDoesNotWaitOnAnAskWhosePaneIsGone(t *testing.T) {
	// a repository in a temporary folder is not one mia lists: under the real home, as standing's own test has it
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	dir, err := os.MkdirTemp(home, ".mia-standing-test-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-gone-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	root := filepath.Join(dir, "shop")
	for _, args := range [][]string{{"init", "-q", "-b", "main", root}, {"-C", root, "-c", "user.name=user", "-c", "user.email=user@example.com", "commit", "-q", "--allow-empty", "-m", "first"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	a, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	panes := map[string]string{}
	for _, name := range []string{"died", "asking", "outside"} {
		record, err := a.NewNamed("task-"+name, "", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.NewWindow(record, "agent", []string{"sleep", "300"}); err != nil {
			t.Fatal(err)
		}
		out, _ := exec.Command("tmux", "display-message", "-p", "-t", "=mia-"+name+":agent", "#{pane_id}").Output()
		panes[name] = strings.TrimSpace(string(out))
	}
	panes["outside"] = "" // set from no pane, to outlive one
	for name, pane := range panes {
		if err := a.Signals.Set(signal.Signal{Worktree: name, Kind: "asks", Rank: 10, From: "claude", Pane: pane, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	exec.Command("tmux", "kill-pane", "-t", panes["died"]).Run()

	at, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	standing, err := at.Standing(false)
	if err != nil {
		t.Fatal(err)
	}
	waiting := map[string]bool{}
	for _, one := range standing.Waiting {
		waiting[one.Worktree] = one.Window != nil
	}
	if window, there := waiting["asking"]; len(waiting) != 2 || !there || !window {
		t.Errorf("the one whose pane is there waits, with its window: %+v", standing.Waiting)
	}
	if window, there := waiting["outside"]; !there || window {
		t.Errorf("one set from no pane waits, with no window: %+v", standing.Waiting)
	}
	if kept, _ := a.Signals.Load(); len(kept) != 3 {
		t.Errorf("standing forgot a signal, and it writes nothing: %+v", kept)
	}
}
