package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	if err := Ship(machine); err != nil {
		return "", err
	}
	base := a.BaseBranch(a.Root)
	if _, err := runtime.Run(machine.SSH, "sh", "-lc", linkScript, "sh", dir, base); err != nil {
		return "", err
	}
	if err := a.copyConfig(machine, dir); err != nil {
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

// copyConfig gives the machine's repository this one's configuration, so
// setup, checks and agents are the same there. This computer's copy wins.
func (a *App) copyConfig(machine runtime.Machine, dir string) error {
	data, err := os.ReadFile(filepath.Join(a.MiaDir, "config.toml"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	put := runtime.Command(machine.SSH, "sh", "-c", `mkdir -p "$1/.git/mia" && cat > "$1/.git/mia/config.toml"`, "sh", dir)
	put.Stdin = bytes.NewReader(data)
	if out, err := put.CombinedOutput(); err != nil {
		return fmt.Errorf("copy the configuration to %s: %v %s", machine.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
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

// RemoteListings are the worktrees each machine linked to this repository
// keeps, as its own mia reports them. Asking also fetches the machine's
// branches. A machine that does not answer gives its last answer, marked
// unreachable.
func (a *App) RemoteListings() []Listing {
	machines, err := a.Runtimes.All()
	if err != nil || len(machines) == 0 {
		return nil
	}
	dir, err := a.RemoteDir()
	if err != nil {
		return nil
	}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		all []Listing
	)
	for _, machine := range machines {
		if !git.HasRemote(a.Root, machine.Name) {
			continue
		}
		wg.Go(func() {
			got := a.remoteListings(machine, dir)
			mu.Lock()
			all = append(all, got...)
			mu.Unlock()
		})
	}
	wg.Wait()
	sort.Slice(all, func(i, j int) bool { return all[i].Machine+all[i].Name < all[j].Machine+all[j].Name })
	return all
}

func (a *App) remoteListings(machine runtime.Machine, dir string) []Listing {
	cache := filepath.Join(a.MiaDir, "remote", machine.Name+".json")
	var out string
	// ponytail: an unanswering machine costs one ssh timeout per 30s of
	// Reachable's memory; ask in the background if that stalls the dash.
	ok, _ := a.Reachable(machine)
	if ok {
		out, ok = a.askRemote(machine, dir, cache)
	}
	if !ok {
		data, err := os.ReadFile(cache)
		if err != nil {
			return nil
		}
		out = string(data)
	}
	var listings []Listing
	if err := json.Unmarshal([]byte(out), &listings); err != nil {
		return nil
	}
	kept := listings[:0]
	for _, listing := range listings {
		if listing.Main {
			continue
		}
		listing.Machine, listing.Unreachable = machine.Name, !ok
		listing.Layers, listing.Stack = nil, ""
		kept = append(kept, listing)
	}
	return kept
}

func (a *App) askRemote(machine runtime.Machine, dir, cache string) (string, bool) {
	out, err := runtime.Run(machine.SSH, "sh", "-c", `cd "$1" && `+remoteMia+` api worktrees`, "sh", dir)
	if err != nil {
		return "", false
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err == nil {
		_ = os.WriteFile(cache, []byte(out), 0o644)
	}
	_ = git.Fetch(a.Root, machine.Name)
	return out, true
}

// AttachOn opens the worktree's session on the machine, through its own mia.
func (a *App) AttachOn(machine runtime.Machine, name string) error {
	dir, err := a.RemoteDir()
	if err != nil {
		return err
	}
	return runtime.Interactive(machine.SSH, "sh", "-c", `cd "$1" && exec `+remoteMia+` shell "$2"`, "sh", dir, name).Run()
}
