package cli

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/panel"
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

// machinesView is the dash's machines view: for this computer and each
// added machine, whether it answers, how busy it is, the worktrees it keeps
// and the ports they listen on. ⏎ on a machine is a shell there.
func machinesView(states []app.MachineState, local []app.Listing) plugin.SideTab {
	self, err := os.Executable()
	if err != nil {
		self = "mia"
	}
	var lines []plugin.Item
	down := 0
	for i, state := range states {
		if i > 0 {
			lines = append(lines, plugin.Item{})
		}
		worktrees := state.Worktrees
		where := state.SSH
		var open *plugin.Open
		if state.Name == runtime.Local {
			where = "this computer"
			for _, l := range local {
				if l.Adopted && !l.Main {
					worktrees = append(worktrees, l)
				}
			}
		} else {
			open = &plugin.Open{Popup: []string{self, "machine", "shell", state.Name}, Title: " " + state.Name + " ", Interactive: true}
		}
		dot, dotStyle, headline, headStyle := "●", "green", "", ""
		if !state.Answers {
			down++
			dot, dotStyle, headline, headStyle = "○", "red", "not answering", "red"
		} else if h := state.Health; h != nil {
			headline = busyness(*h)
			if h.Cores > 0 && h.Load[0] > float64(h.Cores) {
				headStyle = "red"
			}
		}
		lines = append(lines, plugin.Item{Open: open, Cells: plugin.Line{
			{Text: dot, Width: 2, Style: dotStyle}, {Text: state.Name, Width: 12, Style: "bold"},
			{Text: where, Grow: true, Style: "dim"}, {Text: headline, Align: "right", Style: headStyle}}})
		if !state.Answers {
			lines = append(lines, plugin.Item{Cells: plugin.Line{{Text: "  " + state.Detail, Style: "red"}}})
		}
		if h := state.Health; h != nil {
			loadStyle := ""
			if h.Cores > 0 && h.Load[0] > float64(h.Cores) {
				loadStyle = "red"
			}
			lines = append(lines,
				meter("load", h.Load[0], float64(max(h.Cores, 1)*2), fmt.Sprintf("%.1f · %.1f · %.1f", h.Load[0], h.Load[1], h.Load[2]), loadStyle),
				meter("memory", h.MemUsed, h.MemTotal, fmt.Sprintf("%.1fG of %.0fG", h.MemUsed, h.MemTotal), ""),
				meter("disk", h.DiskUsed, h.DiskTotal, fmt.Sprintf("%.0fG free", h.DiskTotal-h.DiskUsed), ""))
		}
		lines = append(lines, worktreesLine(worktrees, state.Answers))
		if h := state.Health; h != nil {
			lines = append(lines, portLines(state.Name, h.Ports, worktrees)...)
		}
	}
	head := fmt.Sprintf("%d machines, all answering", len(states))
	if down > 0 {
		head = fmt.Sprintf("%d answering · %d down", len(states)-down, down)
	}
	return plugin.SideTab{
		Plugin: "mia",
		Side:   plugin.Side{ID: "machines", Where: "right", Label: "machines", Rank: 5},
		SideAnswer: plugin.SideAnswer{
			Count: fmt.Sprint(len(states)), Head: head,
			Note:  "each machine, how busy it is, and the worktrees it keeps",
			Lines: lines,
		},
	}
}

// machineMarks are the header's dots: one a machine, whether it answers.
func machineMarks(states []app.MachineState) []panel.MachineMark {
	marks := make([]panel.MachineMark, len(states))
	for i, state := range states {
		marks[i] = panel.MachineMark{Name: state.Name, Up: state.Answers}
	}
	return marks
}

func busyness(h app.Health) string {
	word := "Calm"
	switch cores := float64(max(h.Cores, 1)); {
	case h.Load[0] > cores:
		word = "Busy"
	case h.Load[0] > cores/2:
		word = "Working"
	}
	load := fmt.Sprintf("%.0f", h.Load[0])
	if h.Load[0] < 10 {
		load = fmt.Sprintf("%.1f", h.Load[0])
	}
	return fmt.Sprintf("%s · load %s on %d cores", word, load, h.Cores)
}

func meter(label string, value, most float64, text, style string) plugin.Item {
	return plugin.Item{Cells: plugin.Line{
		{Text: "  " + label, Width: 10, Style: "dim"},
		{Width: 12, Grow: true, Style: style, Meter: &plugin.Meter{Value: value, Max: most}},
		{Text: text}}}
}

func worktreesLine(worktrees []app.Listing, answers bool) plugin.Item {
	style := ""
	var parts []string
	for i, w := range worktrees {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("+%d more", len(worktrees)-3))
			break
		}
		parts = append(parts, w.Name+"  "+w.Branch)
	}
	text := strings.Join(parts, "    ")
	switch {
	case len(worktrees) == 0:
		text, style = "none", "dim"
	case !answers:
		text, style = text+"    as it said last", "dim"
	}
	return plugin.Item{Cells: plugin.Line{{Text: "  worktrees", Width: 12, Style: "dim"}, {Text: text, Style: style}}}
}

// portLines are the ports each worktree's processes listen on; o opens one.
func portLines(machine string, ports map[string][]int, worktrees []app.Listing) []plugin.Item {
	var lines []plugin.Item
	for _, w := range worktrees {
		var found []int
		for dir, list := range ports {
			if w.Path != "" && (dir == w.Path || strings.HasPrefix(dir, w.Path+"/")) {
				found = append(found, list...)
			}
		}
		if len(found) == 0 {
			continue
		}
		slices.Sort(found)
		words := make([]string, len(found))
		for i, port := range found {
			words[i] = fmt.Sprintf(":%d", port)
		}
		first := fmt.Sprint(found[0])
		lines = append(lines, plugin.Item{
			Cells: plugin.Line{{Text: "  ports", Width: 12, Style: "dim"}, {Text: w.Name, Width: 16}, {Text: strings.Join(words, "  "), Style: "dim"}},
			Keys:  []plugin.ItemKey{{Key: "o", Label: "open :" + first, Open: plugin.Open{Verb: "machine", Args: []string{"open", machine, first}}}},
		})
	}
	return lines
}
