package cli

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/runtime"
)

// Where a machine keeps the note of the computer that handed it this
// terminal, and the worktree picked to go to from there.
const awayReturn = "~/.local/share/mia/away.return"

// cmdAway is mia away <worktree>@<machine> --back <socket> <session>: this
// terminal on the machine's tmux, until it is detached there. The machine
// is told which worktrees this computer has, so its radar can offer them;
// what was picked there is attached here — or handed to the next machine —
// and with nothing picked the session this terminal left comes back.
func cmdAway(a *app.App, args []string) int {
	back := takeValue(&args, "--back")
	if len(args) != 2 || back == "" {
		return usageErr("mia away <worktree>@<machine> --back <socket> <session>")
	}
	target, session := args[0], args[1]
	for {
		name, on, _ := strings.Cut(target, "@")
		machine, err := a.Runtimes.Get(on)
		if err != nil {
			return fail(err)
		}
		_ = a.LeaveNote(machine)
		dir, _ := a.RemoteDir()
		_, _ = runtime.Run(machine.SSH, "sh", "-c", `mkdir -p "$(dirname "$HOME/$1")" && touch "$HOME/$1"`, "sh", app.HerePath(dir))
		argv, err := a.AttachArgv(machine, name)
		if err != nil {
			return fail(err)
		}
		there := exec.Command(argv[0], argv[1:]...)
		there.Stdin, there.Stdout, there.Stderr = os.Stdin, os.Stdout, os.Stderr
		stop := keepNoting(a, machine)
		_ = there.Run()
		stop()
		picked, _ := runtime.Run(machine.SSH, "sh", "-c", `f=`+awayReturn+`; [ -f $f ] && cat $f; rm -f $f "$HOME/$1"`, "sh", app.HerePath(dir))
		picked = strings.TrimSpace(picked)
		switch {
		case picked == "":
			return exitFor(syscallExec("tmux", "-S", back, "attach-session", "-t", "="+session))
		case strings.Contains(picked, "@"):
			target = picked // on to another machine
		default:
			return exitFor(a.Shell(picked, false))
		}
	}
}

// keepNoting tells the machine again, every pollEvery while the terminal is
// there, what this computer has: its status line, which would, is not
// drawn while the terminal is away. stop waits for a note being left.
func keepNoting(a *app.App, machine runtime.Machine) (stop func()) {
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(pollEvery)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				_ = a.LeaveNote(machine)
			}
		}
	}()
	return func() { close(done); <-stopped }
}

func syscallExec(argv ...string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	cmd := exec.Command(path, argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
