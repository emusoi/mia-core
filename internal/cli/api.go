package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/stack"
)

func cmdAPI(a *app.App, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mia api <"+apiQueries+"> [worktree]")
		return exitUsage
	}
	target := ""
	if len(args) > 1 {
		target = args[1]
	}
	switch args[0] {
	case "worktrees":
		listings, err := a.List(args[1:]...)
		if err != nil {
			return fail(err)
		}
		return emit(listings)

	case "dashboard", "env-panel", "stack-panel", "blocks-panel", "machines-panel":
		view, err := panelNamed(a, args[0], target)
		if err != nil {
			return fail(err)
		}
		return emit(view)

	case "env":
		record, err := a.RecordOrHere(target)
		if err != nil {
			return fail(err)
		}
		manager, err := managerFor(a)
		if err != nil {
			return fail(err)
		}
		return emit(manager.Snapshot(record))

	case "stack":
		record, err := a.RecordOrHere(target)
		if err != nil {
			return fail(err)
		}
		branch := git.CurrentBranch(record.Path)
		if branch == "" {
			return emit([]stack.Layer{})
		}
		layers, err := stackStore(a).Of(record.Path, branch, a.BaseBranch(record.Path))
		if err != nil {
			return fail(err)
		}
		return emit(layers)

	case "base":
		return emit(struct {
			Branch string `json:"branch"`
		}{Branch: a.BaseBranch(a.Root)})

	default:
		if strings.HasPrefix(args[0], "plugin:") {
			view, err := panelNamed(a, args[0], target)
			if err != nil {
				return fail(err)
			}
			return emit(view)
		}
		fmt.Fprintf(os.Stderr, "mia: no such query %q — try one of %s\n", args[0], apiQueries)
		return exitNotFound
	}
}

const apiQueries = "worktrees|dashboard|env-panel|stack-panel|blocks-panel|machines-panel|plugin:<name>:<panel>|env|stack|base"

func panelNamed(a *app.App, name, target string) (panel.Panel, error) {
	p, err := buildPanel(a, name, target)
	if err != nil {
		return p, err
	}
	p.Bind(a.Config.KeyMap())
	return p, nil
}

func withChoices(p panel.Panel) panel.Panel {
	if editor, ok := p.Actions["editor"]; ok {
		editor.Choices = Openers()
		p.Actions["editor"] = editor
	}
	return p
}

func buildPanel(a *app.App, name, target string) (panel.Panel, error) {
	if owner, id, ok := strings.Cut(strings.TrimPrefix(name, "plugin:"), ":"); ok && strings.HasPrefix(name, "plugin:") {
		return pluginPanel(a, owner, id, target)
	}
	switch name {
	case "dashboard":
		listings, err := a.List()
		if err != nil {
			return panel.Panel{}, err
		}
		return withChoices(panel.Dashboard(listings, a.Config.Prefix, pluginRows(a, listings))), nil

	case "env-panel":
		record, err := a.RecordOrHere(target)
		if err != nil {
			return panel.Panel{}, err
		}
		manager, err := managerFor(a)
		if err != nil {
			return panel.Panel{}, err
		}
		snapshot := manager.Snapshot(record)
		p := panel.Env(record, snapshot.Environment, snapshot.Ports, snapshot.Services)
		if machines, err := a.Runtimes.All(); err == nil {
			host := p.Actions["host"]
			host.Choices = []string{runtime.Local}
			for _, machine := range machines {
				host.Choices = append(host.Choices, machine.Name)
			}
			p.Actions["host"] = host
		}
		return p, nil

	case "blocks-panel":
		record, err := a.RecordOrHere(target)
		if err != nil {
			return panel.Panel{}, err
		}
		return panel.Blocks(record, a.WindowsWithScreens(record), a.Config.Launches()), nil

	case "machines-panel":
		machines, err := a.Runtimes.All()
		if err != nil {
			return panel.Panel{}, err
		}
		if len(machines) == 0 {
			return panel.Machines(nil), nil
		}
		listings, _ := a.List()
		rows := make([]panel.MachineRow, len(machines))
		var wg sync.WaitGroup
		for i, machine := range machines {
			wg.Add(1)
			go func(i int, machine runtime.Machine) {
				defer wg.Done()
				ok, detail := a.Reachable(machine)
				row := panel.MachineRow{Machine: machine, Reachable: ok, Detail: detail}
				for _, listing := range listings {
					if listing.Where == machine.Name {
						row.Hosting = append(row.Hosting, listing.Name)
					}
				}
				rows[i] = row
			}(i, machine)
		}
		wg.Wait()
		return panel.Machines(rows), nil

	case "stack-panel":
		if path, err := a.Locate(target); err == nil && target != "" {
			target = path
		}
		record, err := a.RecordOrHere(target)
		if err != nil {
			return panel.Panel{}, err
		}
		var layers []stack.Layer
		if branch := git.CurrentBranch(record.Path); branch != "" {
			if layers, err = a.Layers(record.Path, branch); err != nil {
				return panel.Panel{}, err
			}
		}
		return panel.Stack(record, layers), nil
	}
	return panel.Panel{}, fmt.Errorf("no panel called %q", name)
}
