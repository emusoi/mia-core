package app

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
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
