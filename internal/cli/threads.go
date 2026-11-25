package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/emusoi/mia-core/internal/app"
)

// The conversations had with agents, as one history to look through and go on from: `mia recall`
// in every project mia knows, each found one with how to resume it, for a person at the
// terminal, for the list tmux opens (`mia tmux list`) and for an agent asking over MCP.

// thread is a conversation that mentions what was asked for, in whichever project.
type thread struct {
	app.Recalled
	Project string `json:"project"`
	Root    string `json:"root"`   // its repository's main checkout: where resuming it is run
	Resume  string `json:"resume"` // the command that goes on with it, in its folder
}

// recalled is the conversations that mention text: of the repository at here, or with no here
// of every project mia knows, read at once; newest first, no more than limit.
func recalled(here, text string, limit int) ([]thread, error) {
	repos := []app.Repository{{Name: filepath.Base(here), Path: here}}
	if here == "" {
		var err error
		if repos, err = app.Repositories(); err != nil {
			return nil, err
		}
	}
	found := make([][]thread, len(repos))
	var wg sync.WaitGroup
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
	if len(args) == 0 {
		return usageErr("mia recall [--all] <text…>")
	}
	here := ""
	if dir, err := os.Getwd(); err == nil && !everywhere {
		if a, err := app.Open(dir); err == nil {
			here = a.Root
		}
	}
	found, err := recalled(here, strings.Join(args, " "), 10)
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
		fmt.Printf("%s  %s\n  %s: %s\n  cd %s && %s\n", r.At.Local().Format("2006-01-02 15:04"), filepath.Base(r.Cwd), who, r.Line, r.Cwd, r.Resume)
	}
	return exitOK
}
