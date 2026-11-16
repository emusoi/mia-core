package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/signal"
)

// An ask set from no pane waits as long as the process recorded with it
// runs. One with no process recorded waits as it always did; a process
// with the recorded number that began at another time is another process.
func TestAnAskFromNoPaneWaitsWhileItsProcessRuns(t *testing.T) {
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
	root := filepath.Join(dir, "shop")
	for _, args := range [][]string{{"init", "-q", "-b", "main", root}, {"-C", root, "-c", "user.name=user", "-c", "user.email=user@example.com", "commit", "-q", "--allow-empty", "-m", "first"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	a, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	agent := func() (*exec.Cmd, time.Time) {
		command := exec.Command("sleep", "300")
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { command.Process.Kill(); command.Wait() })
		one, there := processes(command.Process.Pid)[command.Process.Pid]
		if !there {
			t.Skip("ps does not say when a process began")
		}
		return command, one.started
	}
	stays, staysSince := agent()
	exits, exitsSince := agent()
	asks := map[string]signal.Signal{
		"running": {Pid: stays.Process.Pid, Started: staysSince},
		"exits":   {Pid: exits.Process.Pid, Started: exitsSince},
		"unknown": {},
		"another": {Pid: stays.Process.Pid, Started: staysSince.Add(-time.Hour)},
	}
	for name, one := range asks {
		if _, err := a.NewNamed("task-"+name, "", name); err != nil {
			t.Fatal(err)
		}
		one.Worktree, one.Kind, one.Rank, one.From, one.At = name, "asks", 10, "claude", time.Now()
		if err := a.Signals.Set(one); err != nil {
			t.Fatal(err)
		}
	}
	at, err := AtMain()
	if err != nil {
		t.Fatal(err)
	}
	waiting := func() []string {
		standing, err := at.Standing(false)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, one := range standing.Waiting {
			names = append(names, one.Worktree)
		}
		slices.Sort(names)
		return names
	}
	if got := waiting(); !slices.Equal(got, []string{"exits", "running", "unknown"}) {
		t.Errorf("one whose process runs waits, and one with none recorded; one whose number another process has does not: %v", got)
	}
	exits.Process.Kill()
	exits.Wait()
	if got := waiting(); !slices.Equal(got, []string{"running", "unknown"}) {
		t.Errorf("one whose process is gone waits no more: %v", got)
	}
	if kept, _ := a.Signals.Load(); len(kept) != 4 {
		t.Errorf("standing forgot a signal, and it writes nothing: %+v", kept)
	}
	listings, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, l := range listings {
		for _, s := range l.Signals {
			listed = append(listed, s.Worktree)
		}
	}
	slices.Sort(listed)
	if !slices.Equal(listed, []string{"running", "unknown"}) {
		t.Errorf("a listing goes by the same rule: %v", listed)
	}
	if kept, _ := a.Signals.Load(); len(kept) != 2 {
		t.Errorf("and forgets the two whose setter is gone: %+v", kept)
	}
}

// The process behind a signal is the agent whose hook set it, whatever
// shells are between and however ps shows the agent.
func TestTheSetterIsTheNearestAgentOrWhatIsNotAShell(t *testing.T) {
	began := time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local)
	for _, c := range []struct {
		name string
		all  map[int]process
		want int
	}{
		{"claude ran the hook in a shell", map[int]process{
			40: {parent: 30, command: "/bin/sh -c mia signal . asks"},
			30: {parent: 20, command: "claude"},
			20: {parent: 10, command: "-zsh"},
			10: {parent: 1, command: "/usr/bin/login -f user"}}, 30},
		{"codex shows behind node, under a launcher that is no shell", map[int]process{
			40: {parent: 35, command: "bash -c mia signal . asks"},
			35: {parent: 30, command: "/usr/bin/env bash hook"},
			30: {parent: 20, command: "node /usr/local/bin/codex"},
			20: {parent: 1, command: "zsh"}}, 30},
		{"no agent: what ran the shell", map[int]process{
			40: {parent: 30, command: "sh watch.sh"},
			30: {parent: 20, command: "watchexec -- sh watch.sh"},
			20: {parent: 1, command: "-zsh"}}, 30},
		{"shells all the way up: none", map[int]process{
			40: {parent: 20, command: "sh -c mia signal . note"},
			20: {parent: 1, command: "-zsh"}}, 0},
		{"a parent ps does not list: none", map[int]process{}, 0},
	} {
		for pid, one := range c.all {
			one.started = began.Add(time.Duration(pid) * time.Minute)
			c.all[pid] = one
		}
		pid, started := setterOf(builtin, c.all, 40)
		if pid != c.want || (pid != 0 && !started.Equal(c.all[pid].started)) || (pid == 0 && !started.IsZero()) {
			t.Errorf("%s: process %d began %v, want %d", c.name, pid, started, c.want)
		}
	}
}
