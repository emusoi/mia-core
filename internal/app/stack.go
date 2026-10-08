package app

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/stack"
)

var ErrRebaseStopped = errors.New("the rebase stopped for a conflict — resolve it, `git rebase --continue`, then `mia stack restack` again")

func (a *App) Stacks() stack.Store {
	return stack.Store{Path: filepath.Join(a.MiaDir, "stack.json")}
}

func (a *App) Layers(dir, branch string) ([]stack.Layer, error) {
	trees := a.trees
	if trees == nil {
		trees, _ = git.Worktrees(a.Root)
	}
	return a.layersAmong(trees, dir, branch)
}

func (a *App) layersAmong(trees []git.Worktree, dir, branch string) ([]stack.Layer, error) {
	if a.counts == nil {
		a.counts = stack.CountsAt(filepath.Join(a.MiaDir, "cache", "counts.json"))
	}
	defer a.counts.Save()
	layers, err := a.Stacks().OfSurveyed(dir, branch, a.BaseBranch(dir), a.counts, a.survey)
	if err != nil || len(layers) == 0 {
		return nil, err
	}
	records, _ := a.Store.Load()
	names := map[string]string{}
	for _, record := range records {
		names[record.Path] = record.Name
	}
	for i, layer := range layers {
		for _, tree := range trees {
			if tree.Branch == layer.Branch {
				name := names[tree.Path]
				if name == "" {
					name = filepath.Base(tree.Path)
				}
				layers[i].Worktree = name
			}
		}
	}
	return layers, nil
}

func (a *App) Restack(dir, branch, base string, each func(stack.Layer)) error {
	layers, err := a.Stacks().Of(dir, branch, base)
	if err != nil {
		return err
	}
	if dirty, err := git.Dirty(dir); err != nil {
		return fmt.Errorf("cannot tell whether %s is clean, so nothing is restacked: %w", dir, err)
	} else if dirty {
		return fmt.Errorf("there are uncommitted changes — a restack rewrites history under them")
	}
	before := map[string]string{}
	for _, layer := range layers {
		before[layer.Branch] = git.RevParse(dir, layer.Branch)
	}
	for _, layer := range layers {
		if layer.Landed || git.IsAncestor(dir, layer.Parent, layer.Branch) {
			continue
		}
		old := before[layer.Parent]
		if old == "" || !git.IsAncestor(dir, old, layer.Branch) {
			old = git.ForkPoint(dir, layer.Parent, layer.Branch)
		}
		if err := git.Rebase(dir, layer.Branch, layer.Parent, old); err != nil {
			if git.RebaseInProgress(dir) {
				return fmt.Errorf("%w: %v", ErrRebaseStopped, err)
			}
			return err
		}
		if each != nil {
			each(layer)
		}
	}
	return git.Checkout(dir, branch)
}

type Merged struct {
	Target string
	Layers []string
	Moved  []string
}

func (a *App) MergeDown(dir, top, target string) (Merged, error) {
	layers, err := a.Layers(dir, top)
	if err != nil {
		return Merged{}, err
	}
	iTop, iTarget := -1, -1
	for i, layer := range layers {
		if layer.Branch == top {
			iTop = i
		}
		if layer.Branch == target {
			iTarget = i
		}
	}
	base := a.BaseBranch(dir)
	switch {
	case iTop < 0:
		return Merged{}, fmt.Errorf("%s is not a layer of this stack", top)
	case target == base || iTarget < 0:
		return Merged{}, fmt.Errorf("mia never merges into %s — the bottom layer lands through a pull request", target)
	case iTarget >= iTop:
		return Merged{}, fmt.Errorf("%s is not below %s", target, top)
	}
	for i := iTarget + 1; i <= iTop; i++ {
		if !git.IsAncestor(dir, layers[i-1].Branch, layers[i].Branch) {
			return Merged{}, fmt.Errorf("%s is behind %s — `mia stack restack` first", layers[i].Branch, layers[i-1].Branch)
		}
	}
	trees, err := git.Worktrees(a.Root)
	if err != nil {
		return Merged{}, err
	}
	at := map[string]string{}
	for _, tree := range trees {
		at[tree.Branch] = tree.Path
	}
	if path, ok := at[target]; ok {
		if dirty, err := git.Dirty(path); err != nil {
			return Merged{}, fmt.Errorf("cannot tell whether %s is clean, so nothing is merged: %w", path, err)
		} else if dirty {
			return Merged{}, fmt.Errorf("%s has uncommitted changes where %s is checked out", layers[iTarget].Worktree, target)
		}
		if err := git.FastForward(path, top); err != nil {
			return Merged{}, err
		}
	} else if err := git.UpdateRef(dir, "refs/heads/"+target, git.RevParse(dir, top)); err != nil {
		return Merged{}, err
	}
	merged := Merged{Target: target}
	for i := iTop; i > iTarget; i-- {
		branch := layers[i].Branch
		if path, ok := at[branch]; ok {
			if err := git.Checkout(path, target); err != nil {
				return merged, err
			}
			merged.Moved = append(merged.Moved, layers[i].Worktree)
		}
		if err := git.DeleteBranch(dir, branch, true); err != nil {
			return merged, err
		}
		if err := a.Stacks().Forget(branch); err != nil {
			return merged, err
		}
		merged.Layers = append(merged.Layers, branch)
	}
	return merged, nil
}

type StackRemoval struct {
	Layers    []string
	Worktrees []string
	Branches  []string
}

func (a *App) RemoveStack(dir, branch string, worktrees, branches, force bool) (StackRemoval, error) {
	layers, err := a.Layers(dir, branch)
	if err != nil {
		return StackRemoval{}, err
	}
	if len(layers) == 0 {
		return StackRemoval{}, fmt.Errorf("%s is not in a stack", branch)
	}
	var out StackRemoval
	for _, layer := range layers {
		out.Layers = append(out.Layers, layer.Branch)
	}
	if worktrees {
		for _, layer := range layers {
			if layer.Worktree == "" {
				continue
			}
			removal, err := a.PlanRemoval(layer.Worktree)
			if err != nil {
				return out, err
			}
			if removal.Record.Path == a.Root {
				continue
			}
			if err := removal.Blocked(force, force); err != nil {
				return out, err
			}
			if err := a.Remove(removal, force, force); err != nil {
				return out, err
			}
			out.Worktrees = append(out.Worktrees, layer.Worktree)
		}
	}
	if err := a.Stacks().Drop(out.Layers); err != nil {
		return out, err
	}
	if branches {
		trees, _ := git.Worktrees(a.Root)
		for _, layer := range layers {
			held := false
			for _, tree := range trees {
				held = held || tree.Branch == layer.Branch
			}
			if held {
				continue
			}
			if err := git.DeleteBranch(a.Root, layer.Branch, force); err != nil {
				return out, fmt.Errorf("%w — %s has work no other branch has; --force deletes it anyway", err, layer.Branch)
			}
			out.Branches = append(out.Branches, layer.Branch)
		}
	}
	return out, nil
}
