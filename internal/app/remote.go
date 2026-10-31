package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/runtime"
)

// RemoteDir is where this repository lives on another machine: the same
// path under that machine's home as it has under yours.
func (a *App) RemoteDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	rel, err := filepath.Rel(home, a.Root)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%s is not under your home, so it has no place on another machine", a.Root)
	}
	return filepath.ToSlash(rel), nil
}

const linkScript = `mkdir -p "$1" && cd "$1" && { [ -e .git ] || git init -q -b "$2"; } &&
git config receive.denyCurrentBranch updateInstead`

// remoteMia is the mia Ship installs, named in full so an older one earlier
// on the machine's PATH cannot answer instead.
const remoteMia = "$HOME/.local/bin/mia"

// Link makes the machine a git remote of this repository, named after it:
// a repository there that takes pushes, holding the base branch. The
// machine never needs to reach GitHub or this computer; git only moves
// when this computer pushes or fetches.
func (a *App) Link(machine runtime.Machine) (string, error) {
	dir, err := a.RemoteDir()
	if err != nil {
		return "", err
	}
	if err := a.Ship(machine); err != nil {
		return "", err
	}
	base := a.BaseBranch(a.Root)
	if _, err := runtime.Run(machine.SSH, "sh", "-lc", linkScript, "sh", dir, base); err != nil {
		return "", err
	}
	if !git.HasRemote(a.Root, machine.Name) {
		if err := git.AddRemote(a.Root, machine.Name, machine.SSH+":"+dir); err != nil {
			return "", err
		}
	}
	if err := git.Push(a.Root, machine.Name, base); err != nil {
		return "", fmt.Errorf("push %s to %s: %w", base, machine.Name, err)
	}
	return dir, nil
}

// NewOn makes the worktree on the machine, with its own mia: the worktree,
// its session and its agent live there, and this computer may sleep.
func (a *App) NewOn(machine runtime.Machine, branch string) error {
	dir, err := a.Link(machine)
	if err != nil {
		return err
	}
	if git.BranchExists(a.Root, branch) {
		if err := git.Push(a.Root, machine.Name, branch); err != nil {
			return fmt.Errorf("push %s to %s: %w", branch, machine.Name, err)
		}
	}
	return runtime.Interactive(machine.SSH, "sh", "-lc", `cd "$1" && `+remoteMia+` new "$2"`, "sh", dir, branch).Run()
}
