package app

import (
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// piSignal is pi's extension that says what pi does with mia signal.
//
//go:embed assets/pi/mia-signal.ts
var piSignal string

// signalsMark is how a file mia wrote to make an agent say what it does
// begins: one that begins otherwise is the person's, and is left alone.
const signalsMark = "// mia-signal: written by mia"

// What mia did, or with --dry-run would do, with the file that makes an
// agent say what it does.
const (
	SignalsWritten = "written" // there was none
	SignalsUpdated = "updated" // mia's own, of another version
	SignalsAlready = "already" // mia's own, as it is now
	SignalsKept    = "kept"    // the person's own under that name: not touched
)

// Signalling is what was done in one account's folder so that its agent
// says what it does with mia signal.
type Signalling struct {
	Agent string `json:"agent"`
	Name  string `json:"name"`
	// File is where the agent loads it from.
	File string `json:"file"`
	Did  string `json:"did"`
	// DryRun: nothing was written, and Did is what would be.
	DryRun bool `json:"dry_run"`
}

// SignalAccount writes, in the folder of an account of an agent, what
// makes the agent say what it does with mia signal: for pi, an extension.
// The account named, the agent's own folder when none is, or every one
// when all. A file there that mia did not write is left as it is; mia's
// own is written again only when it differs. dry writes nothing. It is run
// only when asked for, and looks at nothing else in the folder.
func (a *App) SignalAccount(agentName, name string, all, dry bool) ([]Signalling, error) {
	agent, known := agentNamed(a.agents(), agentName)
	switch {
	case !known:
		return nil, fmt.Errorf("mia knows no agent %s — `mia api agents` lists those it does", agentName)
	case agent.Name == "claude":
		return nil, errors.New("claude says what it does through hooks in its settings.json, which are yours to write — docs/tmux.md has them")
	case agent.signals == "":
		return nil, fmt.Errorf("mia has nothing that makes %s say what it does: only pi's extension", agent.Name)
	}
	names := []string{cmp.Or(name, DefaultAccount)}
	if all {
		names = accountsOf(agent)
	} else if !slices.Contains(accountsOf(agent), names[0]) {
		return nil, fmt.Errorf("%s has no account %s — it has %s", agent.Name, names[0], strings.Join(accountsOf(agent), ", "))
	}
	done := []Signalling{}
	for _, one := range names {
		did, err := signalIn(agent, one, dry)
		if err != nil {
			return done, fmt.Errorf("%s account %s: %w", agent.Name, one, err)
		}
		done = append(done, did)
	}
	return done, nil
}

func signalIn(agent Agent, name string, dry bool) (Signalling, error) {
	home := homeOf(agent, name)
	did := Signalling{Agent: agent.Name, Name: name, File: filepath.Join(home, filepath.FromSlash(agent.signals)), DryRun: dry}
	if home == "" {
		return did, errors.New("cannot find its folder")
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		return did, fmt.Errorf("%s keeps nothing in %s yet — start it there once", agent.Name, home)
	}
	there, err := os.Lstat(did.File)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		did.Did = SignalsWritten
	case err != nil:
		return did, err
	case !there.Mode().IsRegular():
		did.Did = SignalsKept // a link or a folder: not a file mia wrote
		return did, nil
	default:
		has, err := os.ReadFile(did.File)
		switch {
		case err != nil:
			return did, err
		case string(has) == agent.says:
			did.Did = SignalsAlready
			return did, nil
		case !strings.HasPrefix(string(has), signalsMark):
			did.Did = SignalsKept
			return did, nil
		}
		did.Did = SignalsUpdated
	}
	if dry {
		return did, nil
	}
	dir := filepath.Dir(did.File)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return did, err
	}
	// whole or not at all: an agent that starts meanwhile loads no half of it
	temp, err := os.CreateTemp(dir, ".mia-signal-*")
	if err != nil {
		return did, err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(agent.says); err != nil {
		temp.Close()
		return did, err
	}
	if err := temp.Close(); err != nil {
		return did, err
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return did, err
	}
	return did, os.Rename(temp.Name(), did.File)
}
