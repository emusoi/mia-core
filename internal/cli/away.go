package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/radar"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
)

// Where a machine keeps the note of the computer that handed it this
// terminal, and the worktree picked to go to from there.
const (
	awayNote   = "~/.local/share/mia/away.json"
	awayReturn = "~/.local/share/mia/away.return"
)

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
		leaveNote(a, machine)
		argv, err := a.AttachArgv(machine, name)
		if err != nil {
			return fail(err)
		}
		there := exec.Command(argv[0], argv[1:]...)
		there.Stdin, there.Stdout, there.Stderr = os.Stdin, os.Stdout, os.Stderr
		_ = there.Run()
		picked, _ := runtime.Run(machine.SSH, "sh", "-c", `f=`+awayReturn+`; [ -f $f ] && cat $f; rm -f $f `+awayNote)
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

// leaveNote tells the machine this computer's worktrees, and the other
// machines' as this computer last heard them, for its radar to offer.
func leaveNote(a *app.App, machine runtime.Machine) {
	dir, err := a.RemoteDir()
	if err != nil {
		return
	}
	note := radar.Away{From: shortHost(), Repo: dir}
	if local, err := a.List(); err == nil {
		for _, l := range local {
			if l.Adopted && !l.Main {
				note.Worktrees = append(note.Worktrees, l)
			}
		}
	}
	for _, state := range a.KnownMachines() {
		if state.Name == runtime.Local || state.Name == machine.Name {
			continue
		}
		note.Worktrees = append(note.Worktrees, state.Worktrees...)
	}
	data, err := json.Marshal(note)
	if err != nil {
		return
	}
	put := runtime.Command(machine.SSH, "sh", "-c", "mkdir -p ~/.local/share/mia && cat > "+awayNote)
	put.Stdin = bytes.NewReader(data)
	_ = put.Run()
}

// readNote is the note a computer left here when it handed this terminal
// over, if it is about this repository.
func readNote(a *app.App) *radar.Away {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(strings.Replace(awayNote, "~", home, 1))
	if err != nil {
		return nil
	}
	var note radar.Away
	if json.Unmarshal(data, &note) != nil {
		return nil
	}
	if dir, err := a.RemoteDir(); err != nil || dir != note.Repo {
		return nil
	}
	return &note
}

// goBack leaves the pick for the computer that handed this terminal over,
// and detaches, which ends its ssh and hands the terminal back.
func goBack(picked string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := strings.Replace(awayReturn, "~", home, 1)
	if err := os.WriteFile(path, []byte(picked+"\n"), 0o644); err != nil {
		return err
	}
	out, err := run.Local("tmux").Combined("detach-client")
	if err != nil {
		return fmt.Errorf("detach: %s", strings.TrimSpace(out))
	}
	return nil
}

func shortHost() string {
	host, err := os.Hostname()
	if err != nil {
		return "the computer you came from"
	}
	host, _, _ = strings.Cut(host, ".")
	return host
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
