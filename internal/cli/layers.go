package cli

import (
	"fmt"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
)

func newLayer(a *app.App, in, name string, own bool) int {
	record, err := a.RecordOrHere(in)
	if err != nil {
		return fail(err)
	}
	parent := git.CurrentBranch(record.Path)
	if parent == "" {
		return fail(fmt.Errorf("%s is detached — a layer sits on a branch", record.Name))
	}
	branch := name
	if !strings.HasPrefix(branch, a.Config.Prefix) {
		branch = a.Config.Prefix + branch
	}
	if git.HasBranch(record.Path, branch) {
		return fail(fmt.Errorf("%s already exists", branch))
	}
	if own {
		if err := git.BranchAt(record.Path, branch, parent); err != nil {
			return fail(err)
		}
	} else if err := git.CreateBranch(record.Path, branch); err != nil {
		return fail(err)
	}
	if err := stackStore(a).Record(branch, parent); err != nil {
		return fail(err)
	}
	if own {
		made, err := a.New(branch)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s on %s, in its own worktree %s\n", branch, parent, made.Name)
		return exitOK
	}
	fmt.Printf("%s on %s, in %s\n", branch, parent, record.Name)
	return exitOK
}

func cmdUp(a *app.App) int   { return move(a, +1) }
func cmdDown(a *app.App) int { return move(a, -1) }

func move(a *app.App, direction int) int {
	record, err := a.RecordOrHere("")
	if err != nil {
		return fail(err)
	}
	branch := git.CurrentBranch(record.Path)
	if branch == "" {
		return fail(fmt.Errorf("%s is detached, and a stack is made of branches", record.Name))
	}
	layers, err := stackStore(a).Of(record.Path, branch, a.BaseBranch(record.Path))
	if err != nil {
		return fail(err)
	}
	for i, layer := range layers {
		if !layer.Current {
			continue
		}
		next := i + direction
		if next < 0 || next >= len(layers) {
			where := "the top"
			if direction < 0 {
				where = "the bottom"
			}
			fmt.Printf("%s is %s of the stack\n", branch, where)
			return exitOK
		}
		if err := toLayer(record.Path, branch, layers[next].Branch); err != nil {
			return fail(err)
		}
		a.Emit("stack.changed", record, stackChange{Change: "moved"})
		fmt.Println(layers[next].Branch)
		return exitOK
	}
	return fail(fmt.Errorf("%s is not in a stack — `mia new <name> --stack` starts one", branch))
}

func toLayer(dir, from, to string) error {
	if dirty, err := git.DirtyTracked(dir); err != nil {
		return fmt.Errorf("cannot tell whether %s is clean, so it stays on %s: %w", dir, from, err)
	} else if dirty {
		return fmt.Errorf("%s has uncommitted changes — commit or stash them first, or they go with you to %s", from, to)
	}
	return git.Checkout(dir, to)
}
