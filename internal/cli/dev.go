package cli

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/dev"
	"github.com/emusoi/mia-core/internal/git"
)

type devState struct {
	Lent     bool   `json:"lent"`
	Worktree string `json:"worktree,omitempty"`
	Path     string `json:"path,omitempty"`
	Stashed  bool   `json:"stashed,omitempty"`
}

func devStateOf(current dev.Lent) devState {
	return devState{Lent: current.Worktree != "", Worktree: current.Worktree, Path: current.Path, Stashed: current.Stashed}
}

func cmdDev(a *app.App, args []string, asJSON bool) int {
	follow := take(&args, "--follow")
	if follow && asJSON {
		return usageErr("mia dev --follow cannot be combined with --json")
	}
	if len(args) > 1 {
		return usageErr("mia dev [<worktree> [--follow]|off]")
	}
	main := a.Dev()
	if len(args) == 0 {
		current, _, err := main.Current()
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(devStateOf(current))
		}
		if current.Worktree == "" {
			fmt.Println("main's dev server has main's own files")
			return exitOK
		}
		fmt.Printf("main's dev server has %s's files — `mia dev off` gives main its own back\n", current.Worktree)
		return exitOK
	}
	if args[0] == "off" {
		return devOff(main, asJSON)
	}
	if path, err := a.Locate(args[0]); err == nil && git.Resolve(path) == git.Resolve(a.Root) {
		return devOff(main, asJSON)
	}
	record, err := a.RecordOrHere(args[0])
	if err != nil {
		return fail(err)
	}
	if err := main.Lend(record.Name, record.Path); err != nil {
		return fail(err)
	}
	if asJSON {
		current, _, err := main.Current()
		if err != nil {
			return fail(err)
		}
		return emit(devStateOf(current))
	}
	fmt.Printf("main's dev server now has %s's files — `mia dev off` gives main its own back\n", record.Name)
	if !follow {
		return exitOK
	}
	return devFollow(record.Path, main)
}

func devOff(main dev.Main, asJSON bool) int {
	was, err := main.Off()
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(devStateOf(dev.Lent{}))
	}
	if was.Worktree == "" {
		fmt.Println("main's dev server already has main's own files")
		return exitOK
	}
	fmt.Printf("main has its own files back (was %s's)\n", was.Worktree)
	return exitOK
}

func devFollow(from string, main dev.Main) int {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	fmt.Println("following — ctrl-c stops; main keeps the files until `mia dev off`")
	for {
		select {
		case <-stop:
			return exitOK
		case <-tick.C:
			changed, err := dev.Mirror(from, main.Path)
			if err != nil {
				return fail(err)
			}
			if changed > 0 {
				fmt.Printf("%s  %d changed\n", time.Now().Format("15:04:05"), changed)
			}
		}
	}
}
