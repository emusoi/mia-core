package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

// An agent killed in its pane leaves the pane behind, its shell still
// there: what it asked goes with the agent, not with the pane. A done
// outlives it, as news does, and a note set with TMUX_PANE= emptied keeps
// neither pane nor process. Each hook is mia's own, run by a stand-in
// named claude, as claude runs one.
func TestAnAskGoesWithTheAgentKilledInAPaneThatLives(t *testing.T) {
	goEnv, err := exec.Command("go", "env", "GOROOT", "GOCACHE", "GOMODCACHE", "GOPATH").Output()
	if err != nil {
		t.Skip("no go to build mia and a stand-in with")
	}
	paths := strings.Split(strings.TrimSpace(string(goEnv)), "\n")
	build := func(out string, args ...string) {
		t.Helper()
		command := exec.Command(filepath.Join(paths[0], "bin", "go"), append([]string{"build", "-o", out}, args...)...)
		command.Env = append(os.Environ(), "GOCACHE="+paths[1], "GOMODCACHE="+paths[2], "GOPATH="+paths[3], "GOTOOLCHAIN=local", "GOFLAGS=")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("go build %v: %v %s", args, err, out)
		}
	}
	lab(t, nil)
	bin := filepath.Join(os.Getenv("TMUX_TMPDIR"), "bin")
	build(filepath.Join(bin, "mia"), "../../cmd/mia")
	// it runs its hook in a shell, as claude does, and then waits
	source := filepath.Join(t.TempDir(), "claude.go")
	if err := os.WriteFile(source, []byte(`package main

import (
	"os"
	"os/exec"
	"time"
)

func main() {
	hook := exec.Command("/bin/sh", "-c", os.Args[1])
	hook.Stdout, hook.Stderr = os.Stdout, os.Stderr
	hook.Run()
	time.Sleep(5 * time.Minute)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	build(filepath.Join(bin, "claude"), source)
	labOnly(t, "claude", "mia")

	a, err := app.Open(gittest.New(t).Root)
	if err != nil {
		t.Fatal(err)
	}
	hooks := map[string]string{
		"died":    `mia signal . asks "wants to use Bash" --rank 10 --yes 1 --from claude`,
		"running": `mia signal . asks "wants to use Bash" --rank 10 --yes 1 --from claude`,
		"done":    `mia signal . done --rank 30 --until seen --from claude`,
		"note":    `TMUX_PANE= mia signal . note "tests green" --rank 60 --from note`,
	}
	agents := map[string]int{}
	for name, hook := range hooks {
		record, err := a.NewNamed("task-"+name, "", name)
		if err != nil {
			t.Fatal(err)
		}
		said := filepath.Join(os.Getenv("TMUX_TMPDIR"), "said-"+name)
		// the agent's shell stays when the agent is killed, as a pane's does
		script := `claude '` + hook + ` && echo $PPID > ` + said + `'; exec /bin/sh`
		if _, err := a.NewWindow(record, "claude", session.What{}, []string{"/bin/sh", "-c", script}); err != nil {
			t.Fatal(err)
		}
		var pid []byte
		for range 500 {
			if pid, _ = os.ReadFile(said); len(pid) > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if agents[name], err = strconv.Atoi(strings.TrimSpace(string(pid))); err != nil {
			t.Fatalf("%s: the hook did not run: %q", name, pid)
		}
	}
	set, _ := a.Signals.Load()
	if len(set) != 4 {
		t.Fatalf("four signals set: %+v", set)
	}
	for _, one := range set {
		if (one.Pid != 0) == (one.Worktree == "note") || (one.Pane != "") == (one.Worktree == "note") {
			t.Errorf("an agent's hook records its pane and its process; a note neither: %+v", one)
		}
	}
	for _, name := range []string{"died", "done", "note"} {
		syscall.Kill(agents[name], syscall.SIGKILL) // killed: it says nothing as it goes
		for range 500 {
			if syscall.Kill(agents[name], 0) != nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", "=mia-died:claude", "#{pane_current_command}").Output()
	if shell := strings.TrimSpace(string(out)); err != nil || shell == "" || shell == "claude" {
		t.Fatalf("the pane the agent died in lives on, its shell in it: %q %v", out, err)
	}

	listings, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, l := range listings {
		for _, s := range l.Signals {
			left = append(left, s.Worktree+" "+s.Kind)
		}
	}
	slices.Sort(left)
	if want := []string{"done done", "note note", "running asks"}; !slices.Equal(left, want) {
		t.Errorf("the dead agent's ask goes; the running one's, a done and a note stay: %v, want %v", left, want)
	}
	if kept, _ := a.Signals.Load(); len(kept) != 3 {
		t.Errorf("and it is forgotten, not only hidden: %+v", kept)
	}
}
