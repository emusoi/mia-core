package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/run"
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
	var said bytes.Buffer
	create := runtime.Interactive(machine.SSH, "sh", "-lc", `cd "$1" && `+remoteMia+` new "$2"`, "sh", dir, branch)
	create.Stdout = io.MultiWriter(os.Stdout, &said)
	if err := create.Run(); err != nil {
		return err
	}
	// the dash's n reads what it made off the last "name — " line: the
	// worktree as this computer names it, name@machine
	for _, line := range strings.Split(said.String(), "\n") {
		if name, _, ok := strings.Cut(strings.TrimSpace(line), " — "); ok && !strings.ContainsAny(name, " \t") {
			fmt.Printf("%s@%s — %s on %s\n", name, machine.Name, branch, machine.Name)
			break
		}
	}
	return nil
}

// MachineState is one machine as the dash shows it: whether it answers,
// how busy it is, and the worktrees its own mia keeps.
type MachineState struct {
	runtime.Machine
	Answers bool
	Detail  string  // why it did not answer
	Health  *Health // nil when it did not answer
	// Worktrees another machine keeps, as it last reported them; none for
	// this computer, whose worktrees the dash lists itself.
	Worktrees []Listing
}

const worktreesMark = "@@worktrees"

// Machines is this computer and every added machine, asked at once. A
// machine linked to this repository also reports its worktrees, and its
// branches are fetched. One that does not answer keeps the worktrees it
// last reported, marked unreachable.
func (a *App) Machines() []MachineState {
	machines, _ := a.Runtimes.All()
	states := make([]MachineState, len(machines)+1)
	dir, dirErr := a.RemoteDir()
	var wg sync.WaitGroup
	wg.Go(func() {
		out, err := runtime.Run("", "sh", "-c", probeScript, "sh", a.Root)
		states[0] = MachineState{Machine: runtime.Machine{Name: runtime.Local}, Answers: err == nil}
		if err == nil {
			health := parseHealth(out)
			states[0].Health = &health
		}
	})
	for i, machine := range machines {
		wg.Go(func() {
			linked := dirErr == nil && git.HasRemote(a.Root, machine.Name)
			states[i+1] = a.askMachine(machine, dir, linked)
		})
	}
	wg.Wait()
	return states
}

func (a *App) askMachine(machine runtime.Machine, dir string, linked bool) MachineState {
	state := MachineState{Machine: machine}
	cache := filepath.Join(a.MiaDir, "remote", machine.Name+".json")
	script, at := probeScript, "."
	if linked {
		script = "(" + probeScript + ")\necho " + worktreesMark + "\ncd \"$1\" && " + remoteMia + " api worktrees"
		at = dir
	}
	// a machine that did not answer is not asked again for 30s, so one that
	// is off costs one ssh timeout per 30s rather than every refresh
	if got, ok := a.reach.Load(machine.Name); ok {
		if r := got.(reachability); !r.ok && time.Since(r.at) < 30*time.Second {
			state.Detail = r.detail
			state.Worktrees = cachedWorktrees(cache, machine.Name, false)
			return state
		}
	}
	out, err := runtime.Run(machine.SSH, "sh", "-c", script, "sh", at)
	if err != nil {
		state.Detail = run.FirstLine(err.Error())
		a.reach.Store(machine.Name, reachability{false, state.Detail, time.Now()})
		state.Worktrees = cachedWorktrees(cache, machine.Name, false)
		return state
	}
	a.reach.Store(machine.Name, reachability{true, "", time.Now()})
	probe, worktrees, _ := strings.Cut(out, worktreesMark+"\n")
	health := parseHealth(probe)
	state.Answers, state.Health = true, &health
	if linked && strings.TrimSpace(worktrees) != "" {
		if err := os.MkdirAll(filepath.Dir(cache), 0o755); err == nil {
			_ = os.WriteFile(cache, []byte(worktrees), 0o644)
		}
		_ = git.Fetch(a.Root, machine.Name)
		state.Worktrees = cachedWorktrees(cache, machine.Name, true)
	}
	return state
}

// cachedWorktrees are the worktrees a machine last reported, as rows here.
func cachedWorktrees(cache, machine string, answers bool) []Listing {
	data, err := os.ReadFile(cache)
	if err != nil {
		return nil
	}
	var listings []Listing
	if err := json.Unmarshal(data, &listings); err != nil {
		return nil
	}
	kept := listings[:0]
	for _, listing := range listings {
		if listing.Main {
			continue
		}
		listing.Machine, listing.Unreachable = machine, !answers
		listing.Layers, listing.Stack = nil, ""
		kept = append(kept, listing)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	return kept
}

// RemoteWorktrees are every machine's worktrees, in one list.
func RemoteWorktrees(states []MachineState) []Listing {
	var all []Listing
	for _, state := range states {
		all = append(all, state.Worktrees...)
	}
	return all
}

// AttachOn opens the worktree's session on the machine, through its own mia.
func (a *App) AttachOn(machine runtime.Machine, name string) error {
	dir, err := a.RemoteDir()
	if err != nil {
		return err
	}
	return runtime.Interactive(machine.SSH, "sh", "-c", `cd "$1" && exec `+remoteMia+` shell "$2"`, "sh", dir, name).Run()
}
