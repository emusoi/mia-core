package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
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
	case "capabilities":
		return emit(apiCapabilities())

	case "worktrees":
		listings, err := a.List(args[1:]...)
		if err != nil {
			return fail(err)
		}
		return emit(listings)

	case "machines":
		return emit(a.Machines())

	case "here":
		return emit(a.Here())

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
		fmt.Fprintf(os.Stderr, "mia: no such query %q — try one of %s\n", args[0], apiQueries)
		return exitNotFound
	}
}

var apiQueryList = []string{"capabilities", "worktrees", "machines", "here", "repos", "env", "stack", "base"}

var apiQueries = strings.Join(apiQueryList, "|")

// apiVersion is what `mia api capabilities` says the JSON speaks; it goes
// up when a field changes meaning or goes, not when one is added.
const apiVersion = 1

type capabilities struct {
	Version  int      `json:"version"`
	Queries  []string `json:"queries"`
	Features []string `json:"features"`
}

func apiCapabilities() capabilities {
	return capabilities{
		Version: apiVersion,
		Queries: apiQueryList,
		Features: []string{
			"direct-http", "project-config", "doctor", "environment-readiness", "environment-drift",
			"local-sessions", "remote-sync-preview", "sync-conflicts", "safe-cleanup", "stack-sync", "merged-gc", "compose-environments",
		},
	}
}
