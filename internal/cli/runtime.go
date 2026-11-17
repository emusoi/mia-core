package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/runtime"
)

// listedMachine is a machine in `mia machine ls`: as it was added, and as
// it stood when last asked; never asked, it has neither.
type listedMachine struct {
	runtime.Machine
	Reach *app.Reach `json:"reach,omitempty"`
	Asked *time.Time `json:"asked,omitempty"`
}

func cmdRuntime(args []string, asJSON bool) int {
	store := runtime.Store{Path: config.RuntimesPath()}
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "list", "ls":
		machines, err := store.All()
		if err != nil {
			return fail(err)
		}
		// how each stood when last asked — by `mia api standing`, which
		// the apps poll — without asking any: a listing stays instant
		listed := []listedMachine{{Machine: runtime.Machine{Name: runtime.Local}}}
		a, _ := app.AtMain()
		for _, machine := range machines {
			one := listedMachine{Machine: machine}
			if a != nil {
				if reach, at, ok := a.LastReach(machine.Name); ok {
					one.Reach, one.Asked = &reach, &at
				}
			}
			listed = append(listed, one)
		}
		if asJSON {
			return emit(listed)
		}
		fmt.Printf("%-12s %-24s %s\n", runtime.Local, "this computer", "")
		for _, one := range listed[1:] {
			stands := ""
			if one.Reach != nil {
				stands = string(one.Reach.State)
				if one.Reach.State != app.ReachOK {
					stands += " — " + one.Reach.Detail
					if one.Reach.Fix != "" {
						stands += " — " + one.Reach.Fix
					}
				}
				for _, warning := range one.Reach.Warnings {
					stands += " — " + warning.Detail + ": " + warning.Fix
				}
			}
			fmt.Println(strings.TrimRight(fmt.Sprintf("%-12s %-24s %-8s %s", one.Name, one.SSH, one.Engine, stands), " "))
		}
		return exitOK

	case "add":
		words := args[1:]
		if len(words) == 1 {
			words = strings.Fields(words[0])
		}
		if len(words) != 2 {
			return usageErr("mia machine add <name> <ssh-target>")
		}
		name, target := words[0], words[1]
		fmt.Fprintf(os.Stderr, "probing %s…\n", target)
		machine, err := runtime.Probe(target)
		if err != nil {
			return fail(err)
		}
		machine.Name = name
		if err := store.Add(machine); err != nil {
			return fail(err)
		}
		if err := app.Ship(machine); err != nil {
			return fail(err)
		}
		fmt.Printf("%s  %s  %s\n", machine.Name, machine.SSH, machine.Engine)
		return exitOK

	case "shell":
		if len(args) != 2 {
			return usageErr("mia machine shell <name>")
		}
		machine, err := store.Get(args[1])
		if err != nil {
			return fail(err)
		}
		if machine.Name == runtime.Local {
			return fail(fmt.Errorf("local is this computer — you are already here"))
		}
		return exitFor(runtime.Interactive(machine.SSH).Run())

	case "ship":
		if len(args) != 2 {
			return usageErr("mia machine ship <name>")
		}
		machine, err := store.Get(args[1])
		if err != nil {
			return fail(err)
		}
		if machine.Name == runtime.Local {
			return fail(fmt.Errorf("local is this computer — it runs this mia already"))
		}
		if err := app.Ship(machine); err != nil {
			return fail(err)
		}
		fmt.Printf("%s runs this mia\n", machine.Name)
		return exitOK

	case "open":
		if len(args) != 3 {
			return usageErr("mia machine open <name> <port>")
		}
		machine, err := store.Get(args[1])
		if err != nil {
			return fail(err)
		}
		host := "localhost"
		if machine.Name != runtime.Local {
			host = machine.SSH[strings.LastIndex(machine.SSH, "@")+1:]
		}
		url := "http://" + host + ":" + strings.TrimPrefix(args[2], ":")
		opener := "xdg-open"
		if _, err := exec.LookPath("open"); err == nil {
			opener = "open"
		}
		return exitFor(exec.Command(opener, url).Run())

	case "rm", "remove":
		if len(args) != 2 {
			return usageErr("mia machine rm <name>")
		}
		if err := store.Remove(args[1]); err != nil {
			return fail(err)
		}
		fmt.Printf("forgot %s — environments recorded there will not start until they are moved\n", args[1])
		return exitOK

	default:
		return usageErr("mia machine <list|add|shell|ship|open|attach|rm>")
	}
}

// cmdAttachOn is `mia machine attach <worktree>@<machine>`: the session of a
// worktree another machine's mia keeps.
func cmdAttachOn(a *app.App, args []string) int {
	if len(args) != 1 {
		return usageErr("mia machine attach <worktree>@<machine>")
	}
	name, on, ok := strings.Cut(args[0], "@")
	if !ok || name == "" {
		return usageErr("mia machine attach <worktree>@<machine>")
	}
	machine, err := a.Runtimes.Get(on)
	if err != nil {
		return fail(err)
	}
	return exitFor(a.AttachOn(machine, name))
}
