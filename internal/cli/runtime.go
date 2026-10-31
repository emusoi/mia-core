package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/plugin"
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
		return usageErr("mia machine <list|add|shell|attach|rm>")
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

// machinesSide is the room's machines tab: each machine, whether it answers,
// and the branches of the worktrees it keeps. ⏎ on a machine is a shell there.
// No machines, no tab.
func machinesSide(a *app.App, local, remote []app.Listing) (plugin.SideTab, bool) {
	machines, err := a.Runtimes.All()
	if err != nil || len(machines) == 0 {
		return plugin.SideTab{}, false
	}
	self, err := os.Executable()
	if err != nil {
		self = "mia"
	}
	kept := map[string][]string{}
	for _, l := range local {
		if l.Adopted && !l.Main {
			kept[runtime.Local] = append(kept[runtime.Local], l.Branch)
		}
	}
	for _, l := range remote {
		kept[l.Machine] = append(kept[l.Machine], l.Branch)
	}
	// The right side sets each line flush right, so every line is as wide
	// as the widest to keep the block's left edge straight.
	const width = 28
	text := func(t, style string) plugin.Line {
		return plugin.Line{{Text: "  " + t, Width: width, Style: style}}
	}
	var lines []plugin.Item
	add := func(dot, dotStyle, name, where, note string, open *plugin.Open) {
		lines = append(lines,
			plugin.Item{Cells: plugin.Line{{Text: dot, Width: 2, Style: dotStyle}, {Text: name, Width: 12, Style: "bold"}, {Text: where, Width: width - 14, Align: "right", Style: "dim"}}, Open: open},
			plugin.Item{Cells: text(note, "dim"), Open: open})
		branches := kept[name]
		for i, branch := range branches {
			if i == 3 {
				lines = append(lines, plugin.Item{Cells: text(fmt.Sprintf("+%d more", len(branches)-3), "dim"), Open: open})
				break
			}
			lines = append(lines, plugin.Item{Cells: text(branch, ""), Open: open})
		}
		lines = append(lines, plugin.Item{})
	}
	add("●", "green", runtime.Local, "this computer", worktreeCount(len(kept[runtime.Local])), nil)
	up := 1
	for _, machine := range machines {
		open := &plugin.Open{Popup: []string{self, "machine", "shell", machine.Name}, Title: " " + machine.Name + " ", Interactive: true}
		if ok, detail := a.Reachable(machine); ok {
			up++
			add("●", "green", machine.Name, machine.SSH, worktreeCount(len(kept[machine.Name])), open)
		} else {
			add("○", "red", machine.Name, "not answering", detail, open)
		}
	}
	return plugin.SideTab{
		Plugin:     "mia",
		Side:       plugin.Side{ID: "machines", Where: "right", Label: "machines", Rank: 5},
		SideAnswer: plugin.SideAnswer{Count: fmt.Sprintf("%d/%d", up, len(machines)+1), Lines: lines},
	}, true
}

func worktreeCount(n int) string {
	if n == 1 {
		return "1 worktree"
	}
	return fmt.Sprintf("%d worktrees", n)
}
