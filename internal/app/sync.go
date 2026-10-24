package app

import (
	"fmt"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/stack"
)

type SyncResult struct {
	Base      string   `json:"base"`
	Current   string   `json:"current"`
	Dropped   []string `json:"dropped,omitempty"`
	Restacked int      `json:"restacked"`
}

func (a *App) Sync(dir, branch string) (SyncResult, error) {
	result := SyncResult{Base: a.BaseBranch(dir), Current: branch}
	if !a.Stacks().Member(branch) {
		return result, fmt.Errorf("%s is not in a stack", branch)
	}
	if dirty, err := git.Dirty(dir); err != nil {
		return result, fmt.Errorf("cannot tell whether %s is clean, so nothing is synced: %w", dir, err)
	} else if dirty {
		return result, fmt.Errorf("there are uncommitted changes — sync fetches and rewrites history")
	}
	base, err := a.syncBase(dir, result.Base)
	if err != nil {
		return result, err
	}
	result.Base = base
	layers, err := a.Layers(dir, branch)
	if err != nil {
		return result, err
	}
	current := -1
	var landed []stack.Layer
	for i, layer := range layers {
		if layer.Branch == branch {
			current = i
		}
		if layer.Landed {
			landed = append(landed, layer)
		}
	}
	if current >= 0 && layers[current].Landed && len(landed) < len(layers) {
		target := ""
		for i := current - 1; i >= 0 && target == ""; i-- {
			if !layers[i].Landed {
				target = layers[i].Branch
			}
		}
		for i := current + 1; i < len(layers) && target == ""; i++ {
			if !layers[i].Landed {
				target = layers[i].Branch
			}
		}
		if target != "" {
			if err := git.Checkout(dir, target); err != nil {
				return result, err
			}
			result.Current = target
		}
	}
	for _, layer := range landed {
		if err := a.Stacks().Forget(layer.Branch); err != nil {
			return result, err
		}
		result.Dropped = append(result.Dropped, layer.Branch)
	}
	if len(landed) == len(layers) {
		return result, nil
	}
	if err := a.Restack(dir, result.Current, result.Base, func(stack.Layer) { result.Restacked++ }); err != nil {
		return result, err
	}
	return result, nil
}

func (a *App) syncBase(dir, base string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("no base branch — set base in `mia config`")
	}
	upstream := git.Upstream(dir, base)
	if upstream == "" {
		remote, _, hasSlash := strings.Cut(base, "/")
		switch {
		case hasSlash && git.HasRemote(dir, remote) && git.RevParse(dir, base) != "":
			upstream = base
		case git.HasRemote(dir, "origin"):
			upstream = "origin/" + base
		default:
			return "", fmt.Errorf("%s has no upstream — `mia stack restack` works without fetching", base)
		}
	}
	remote, branch, ok := strings.Cut(upstream, "/")
	if !ok || remote == "" || branch == "" {
		return "", fmt.Errorf("%s is not a remote-tracking upstream for %s", upstream, base)
	}
	localBase := upstream != base
	var trees []git.Worktree
	var err error
	if localBase {
		trees, err = git.Worktrees(dir)
		if err != nil {
			return "", err
		}
		for _, tree := range trees {
			if tree.Branch != base {
				continue
			}
			dirty, err := git.Dirty(tree.Path)
			if err != nil {
				return "", err
			}
			if dirty {
				return "", fmt.Errorf("base worktree %s has uncommitted changes — commit or stash them before syncing", tree.Path)
			}
		}
	}
	if err := git.Fetch(dir, remote, branch); err != nil {
		return "", err
	}
	if !localBase {
		return base, nil
	}
	if !git.IsAncestor(dir, base, upstream) {
		return "", fmt.Errorf("%s does not fast-forward to %s — reconcile the base manually before syncing", base, upstream)
	}
	for _, tree := range trees {
		if tree.Branch == base {
			return base, git.FastForward(tree.Path, upstream)
		}
	}
	if err := git.UpdateRef(dir, "refs/heads/"+base, git.RevParse(dir, upstream)); err != nil {
		return "", err
	}
	return base, nil
}
