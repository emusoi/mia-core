package app

import (
	"cmp"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/paths"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/store"
)

// A setup is an agent with a model and an effort. Each time mia starts an
// agent with one, or switches one to another, it writes down which and
// where, in a history of the person's that keeps the last usesKept: the
// setups a person uses, the usual one first, are what a picker offers.

// Use is one start or switch of an agent with a model or an effort.
type Use struct {
	Agent  string `json:"agent"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
	// Repository is the one the agent was started in, by its main
	// checkout's folder name; the main agent's is its folder's, main.
	Repository string    `json:"repository"`
	Worktree   string    `json:"worktree"`
	Machine    string    `json:"machine"`
	At         time.Time `json:"at"`
}

// usesKept is how many uses the history keeps; the oldest go first.
const usesKept = 500

func usesPath() string { return paths.In("setups.json") }

// uses is the history, oldest first; none when there is none, or it
// cannot be read.
func uses() []Use {
	kept := []Use{}
	if data, err := os.ReadFile(usesPath()); err == nil {
		_ = json.Unmarshal(data, &kept) // unreadable is as nothing kept
	}
	return kept
}

// used writes down that an agent was started with, or switched to, what
// was chosen for it in a worktree on a machine. Nothing chosen is nothing
// written.
func (a *App) used(agent, worktree, machine string, chosen Chosen) error {
	path := usesPath()
	if path == "" || agent == "" || chosen == (Chosen{}) {
		return nil
	}
	unlock, err := store.Lock(filepath.Dir(path), "setups.lock")
	if err != nil {
		return err
	}
	defer unlock()
	all := append(uses(), Use{Agent: agent, Model: chosen.Model, Effort: chosen.Effort,
		Repository: filepath.Base(a.Root), Worktree: worktree, Machine: machine, At: time.Now().UTC()})
	data, err := json.Marshal(all[max(0, len(all)-usesKept):])
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// machineOf is the machine a worktree's agents run on: local for this one.
func machineOf(record model.Record) string {
	return cmp.Or(env.Placement(record).Runtime(), runtime.Local)
}

// Setup is an agent, a model and an effort, with how much they were used.
type Setup struct {
	Agent  string `json:"agent"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
	// Uses is how many times an agent was started with it or switched to
	// it; Sessions in how many worktrees.
	Uses     int `json:"uses"`
	Sessions int `json:"sessions"`
	// Last is where it was used last; nil for one never used.
	Last *LastUse `json:"last,omitempty"`
	// Usual is the one used in the most worktrees of the repository asked.
	Usual bool `json:"usual"`
}

// LastUse is where a setup was used last.
type LastUse struct {
	Repository string    `json:"repository"`
	Worktree   string    `json:"worktree"`
	Machine    string    `json:"machine"`
	At         time.Time `json:"at"`
}

// Setups is the setups used in a repository, or in every one for "":
// the repository's usual first, then the last used first, then those the
// config starts an agent with that were never used there.
func (a *App) Setups(repository string) []Setup {
	type key struct{ agent, model, effort string }
	at, worktrees := map[key]int{}, map[key]map[string]bool{}
	setups := []Setup{}
	for _, use := range uses() {
		if repository != "" && use.Repository != repository {
			continue
		}
		k := key{use.Agent, use.Model, use.Effort}
		i, seen := at[k]
		if !seen {
			i, at[k], worktrees[k] = len(setups), len(setups), map[string]bool{}
			setups = append(setups, Setup{Agent: use.Agent, Model: use.Model, Effort: use.Effort})
		}
		worktrees[k][use.Repository+"\x00"+use.Worktree] = true
		setups[i].Uses++
		setups[i].Sessions = len(worktrees[k])
		if setups[i].Last == nil || !use.At.Before(setups[i].Last.At) {
			setups[i].Last = &LastUse{Repository: use.Repository, Worktree: use.Worktree, Machine: use.Machine, At: use.At}
		}
	}
	slices.SortStableFunc(setups, func(x, y Setup) int { return y.Last.At.Compare(x.Last.At) })
	if repository != "" && len(setups) > 0 {
		usual := 0
		for i, one := range setups {
			if one.Sessions > setups[usual].Sessions { // a tie stays with the later used
				usual = i
			}
		}
		setups[usual].Usual = true
		setups = slices.Concat(setups[usual:usual+1], setups[:usual], setups[usual+1:])
	}
	for _, one := range a.configuredSetups() {
		if k := (key{one.Agent, one.Model, one.Effort}); !slices.ContainsFunc(setups, func(s Setup) bool { return key{s.Agent, s.Model, s.Effort} == k }) {
			setups = append(setups, one)
		}
	}
	return setups
}

// configuredSetups is what the config starts agents with: the main
// agent's first, then each in [agents] by its name.
func (a *App) configuredSetups() []Setup {
	setups := []Setup{}
	if agent, err := a.mainAgent(); err == nil {
		set := a.Config.Agents[agent.Name]
		if model, effort := cmp.Or(a.Config.Main.Model, set.Model), cmp.Or(a.Config.Main.Effort, set.Effort); model != "" || effort != "" {
			setups = append(setups, Setup{Agent: agent.Name, Model: model, Effort: effort})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(a.Config.Agents)) {
		if set := a.Config.Agents[name]; set.Model != "" || set.Effort != "" {
			setups = append(setups, Setup{Agent: name, Model: set.Model, Effort: set.Effort})
		}
	}
	return setups
}
