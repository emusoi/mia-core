package cli

import (
	"fmt"
	"os"

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

const apiQueries = "worktrees|machines|here|repos|env|stack|base"
