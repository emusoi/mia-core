package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/runtime"
)

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
		if asJSON {
			return emit(append([]runtime.Machine{{Name: runtime.Local}}, machines...))
		}
		fmt.Printf("%-12s %-24s %s\n", runtime.Local, "this computer", "")
		for _, machine := range machines {
			fmt.Printf("%-12s %-24s %s\n", machine.Name, machine.SSH, machine.Engine)
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
		return usageErr("mia machine <list|add|shell|rm>")
	}
}
