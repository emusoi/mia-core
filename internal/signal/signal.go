// Package signal keeps what anything outside mia says about a worktree: an
// agent's hook, a test watcher, a person. mia gives a signal no meaning; it
// keeps one per source per worktree, sorts them by the rank the setter gave,
// draws them in the style the setter asked for, and forgets them when their
// setter said to.
package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/emusoi/mia-core/internal/store"
)

// DefaultRank is a signal's rank when its setter gives none; lower is first.
const DefaultRank = 50

type Signal struct {
	Worktree string    `json:"worktree"`
	Kind     string    `json:"kind"`
	Text     string    `json:"text,omitempty"`
	Style    string    `json:"style,omitempty"`
	Rank     int       `json:"rank"`
	From     string    `json:"from"`
	At       time.Time `json:"at"`
	// Until is when it goes by itself; zero is never.
	Until time.Time `json:"until,omitzero"`
	// UntilSeen: it goes when the worktree is next switched to.
	UntilSeen bool `json:"until_seen,omitempty"`
	// Pane is the tmux pane the setter ran in, when it ran in one: where the
	// radar types an answer.
	Pane string `json:"pane,omitempty"`
	// Pid is the process that stands behind one set from no pane, the agent
	// whose hook set it, and Started when that process began, which tells it
	// from another given its number later. Zero when none could be told.
	Pid     int       `json:"pid,omitempty"`
	Started time.Time `json:"started,omitzero"`
	// Yes is the keys its setter says approve it, pressed with no Enter.
	Yes string `json:"yes,omitempty"`
}

// Calls: it is ranked to call for you — asks, failed, done — where one
// ranked above DefaultRank, claude's busy at 60, says how a worktree is.
func (s Signal) Calls() bool { return s.Rank <= DefaultRank }

type Store struct{ Dir string }

func (s Store) path() string { return filepath.Join(s.Dir, "signals.json") }

// Load is every signal still standing, best rank first, then newest.
func (s Store) Load() ([]Signal, error) {
	return s.read(time.Now())
}

func (s Store) read(now time.Time) ([]Signal, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []Signal
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("read %s: %w", s.path(), err)
	}
	standing := all[:0]
	for _, one := range all {
		if one.Until.IsZero() || now.Before(one.Until) {
			standing = append(standing, one)
		}
	}
	Sort(standing)
	return standing, nil
}

// Set puts the signal in its source's place for its worktree.
func (s Store) Set(signal Signal) error {
	return s.update(func(all []Signal) []Signal {
		kept := drop(all, func(one Signal) bool { return one.Worktree == signal.Worktree && one.From == signal.From })
		return append(kept, signal)
	})
}

// Clear removes the worktree's signals from one source, or from all of them
// when from is empty.
func (s Store) Clear(worktree, from string) error {
	return s.update(func(all []Signal) []Signal {
		return drop(all, func(one Signal) bool { return one.Worktree == worktree && (from == "" || one.From == from) })
	})
}

// Seen removes the worktree's signals that last until it is seen.
func (s Store) Seen(worktree string) error {
	return s.update(func(all []Signal) []Signal {
		return drop(all, func(one Signal) bool { return one.Worktree == worktree && one.UntilSeen })
	})
}

func (s Store) update(change func([]Signal) []Signal) error {
	unlock, err := store.Lock(s.Dir, "signals.lock")
	if err != nil {
		return err
	}
	defer unlock()
	all, err := s.Load()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(change(all), "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.Dir, ".signals-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.path())
}

// Of is the worktree's signals, in order.
func Of(all []Signal, worktree string) []Signal {
	var mine []Signal
	for _, one := range all {
		if one.Worktree == worktree {
			mine = append(mine, one)
		}
	}
	return mine
}

// Sort puts the best rank first, the newest first among equals.
func Sort(all []Signal) {
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Rank != all[j].Rank {
			return all[i].Rank < all[j].Rank
		}
		return all[i].At.After(all[j].At)
	})
}

func drop(all []Signal, gone func(Signal) bool) []Signal {
	var kept []Signal
	for _, one := range all {
		if !gone(one) {
			kept = append(kept, one)
		}
	}
	return kept
}
