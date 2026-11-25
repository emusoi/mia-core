package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/runtime"
)

// The conversations had with agents, as one history to look through and go on from: `mia recall`
// in every project mia knows, each found one with how to resume it, for a person at the
// terminal, for the list tmux opens (`mia tmux list`) and for an agent asking over MCP.

// thread is a conversation that mentions what was asked for, in whichever project.
type thread struct {
	app.Recalled
	Project string `json:"project"`
	Root    string `json:"root"`              // its repository's main checkout: where resuming it is run
	Resume  string `json:"resume"`            // the command that goes on with it, in its folder
	Machine string `json:"machine,omitempty"` // the machine it is on, when not this one
}

// elsewhere asks a machine's mia for the conversations there that mention text, given
// three seconds: a machine that is off, or whose mia is too old to look in every project,
// has none to offer and holds nothing up.
var elsewhere = func(machine runtime.Machine, text string, limit int) []thread {
	out, err := runtime.RunWithin(3*time.Second, machine.SSH, "mia", "recall", "--all", "--here", "--json", text)
	var found []thread
	if err != nil || json.Unmarshal([]byte(out), &found) != nil {
		return nil
	}
	for i := range found {
		found[i].Machine = machine.Name
	}
	return found[:min(len(found), limit)]
}

// recalled is the conversations that mention text: of the repository at here, or with no here
// of every project mia knows, read at once, and with machines of every project on each machine
// mia knows too; newest first, no more than limit.
func recalled(here, text string, limit int, machines ...runtime.Machine) ([]thread, error) {
	repos := []app.Repository{{Name: filepath.Base(here), Path: here}}
	if here == "" {
		var err error
		if repos, err = app.Repositories(); err != nil {
			return nil, err
		}
	}
	found := make([][]thread, len(repos)+len(machines))
	var wg sync.WaitGroup
	for i, machine := range machines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			found[len(repos)+i] = elsewhere(machine, text, limit)
		}()
	}
	for i, repo := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := app.Open(repo.Path)
			if err != nil {
				return // a project whose folder is gone has nothing to resume
			}
			for _, r := range a.Recall(text, limit) {
				found[i] = append(found[i], thread{Recalled: r, Project: repo.Name, Root: a.Root, Resume: a.ResumeCommand(r.Agent, r.Session)})
			}
		}()
	}
	wg.Wait()
	all := slices.Concat(found...)
	slices.SortStableFunc(all, func(x, y thread) int { return y.At.Compare(x.At) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []thread{} // printed as JSON
	}
	return all, nil
}

// cmdRecall is mia recall [--all] <text…>: in the repository here, and with --all or in no
// repository, in every project.
func cmdRecall(args []string, asJSON bool) int {
	everywhere := take(&args, "--all") || take(&args, "-a")
	only := take(&args, "--here") // of --all: this machine alone, as one machine asks another
	if len(args) == 0 {
		return usageErr("mia recall [--all [--here]] <text…>")
	}
	here := ""
	if dir, err := os.Getwd(); err == nil && !everywhere {
		if a, err := app.Open(dir); err == nil {
			here = a.Root
		}
	}
	var machines []runtime.Machine
	if here == "" && !only {
		machines = otherMachines()
	}
	found, err := recalled(here, strings.Join(args, " "), 10, machines...)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(found)
	}
	if len(found) == 0 {
		fmt.Println("no conversation mentions it")
	}
	for i, r := range found {
		if i > 0 {
			fmt.Println()
		}
		who := r.Agent
		if r.Yours {
			who = "you"
		}
		where := filepath.Base(r.Cwd)
		if r.Machine != "" {
			where += " on " + r.Machine
		}
		fmt.Printf("%s  %s\n  %s: %s\n  cd %s && %s\n", r.At.Local().Format("2006-01-02 15:04"), where, who, r.Line, r.Cwd, r.Resume)
	}
	return exitOK
}

// otherMachines is the machines mia knows that are reached over ssh.
func otherMachines() []runtime.Machine {
	all, _ := runtime.Store{Path: config.RuntimesPath()}.All()
	return slices.DeleteFunc(all, func(m runtime.Machine) bool { return m.SSH == "" })
}
