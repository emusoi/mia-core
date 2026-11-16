package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/signal"
)

// mia signal ls goes by the rule a listing does: an ask whose process is
// gone is not listed, and is forgotten; one whose process runs is, and a
// done that waits to be seen outlives its process.
func TestSignalLsLeavesOutOneWhoseProcessIsGone(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	agent := func() (*exec.Cmd, time.Time) {
		command := exec.Command("sleep", "300")
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { command.Process.Kill(); command.Wait() })
		ps := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(command.Process.Pid))
		ps.Env = append(os.Environ(), "LC_ALL=C")
		out, err := ps.Output()
		started, perr := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(string(out)), " "), time.Local)
		if err != nil || perr != nil {
			t.Skip("ps does not say when a process began")
		}
		return command, started
	}
	runs, runsSince := agent()
	died, diedSince := agent()
	for _, one := range []signal.Signal{
		{Worktree: "running", Kind: "asks", Rank: 10, Pid: runs.Process.Pid, Started: runsSince},
		{Worktree: "died", Kind: "asks", Rank: 10, Pid: died.Process.Pid, Started: diedSince},
		{Worktree: "finished", Kind: "done", Rank: 30, Pid: died.Process.Pid, Started: diedSince, UntilSeen: true},
	} {
		one.From, one.At = "claude", time.Now()
		if err := a.Signals.Set(one); err != nil {
			t.Fatal(err)
		}
	}
	died.Process.Kill()
	died.Wait()

	data, code := printed(t, "signal", "ls", "--json")
	var listed []signal.Signal
	if err := json.Unmarshal(data, &listed); err != nil || code != exitOK {
		t.Fatalf("signal ls --json = %s, exit %d, %v", data, code, err)
	}
	var names []string
	for _, one := range listed {
		names = append(names, one.Worktree)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"finished", "running"}) {
		t.Errorf("one whose process died is not listed; one whose process runs and a done until seen are: %v", names)
	}
	if kept, _ := a.Signals.Load(); len(kept) != 2 {
		t.Errorf("and it is forgotten, as a listing forgets it: %+v", kept)
	}
}
