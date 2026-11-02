package app_test

import (
	"os"
	"os/exec"
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
