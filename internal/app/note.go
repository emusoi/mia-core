package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/runtime"
)

// Note is what this computer tells a machine about itself, for one
// repository: every checkout it has there, and the other machines' as it
// last heard them. The machine's radar lists them; it is refreshed each
// time this computer asks the machine how it is, so the machine knows even
// when no terminal was handed to it.
type Note struct {
	From      string    `json:"from"`
	Repo      string    `json:"repo"`
	At        time.Time `json:"at"`
	Worktrees []Listing `json:"worktrees"`
	// Here: a terminal was handed over from there and is still here, so
	// picking one of its worktrees can hand it back.
	Here bool `json:"-"`
}

const notesDir = ".local/share/mia/away"

func noteKey(repo string) string { return strings.ReplaceAll(strings.Trim(repo, "/"), "/", "-") }

// NotePath is where a machine keeps the note for a repository, under its home.
func NotePath(repo string) string { return notesDir + "/" + noteKey(repo) + ".json" }

// HerePath marks that a terminal handed over for the repository is there.
func HerePath(repo string) string { return notesDir + "/" + noteKey(repo) + ".here" }

// LeaveNote tells the machine this computer's checkouts of this repository
// and the other machines' worktrees as this computer last heard them.
func (a *App) LeaveNote(machine runtime.Machine) error {
	dir, err := a.RemoteDir()
	if err != nil {
		return err
	}
	note := Note{From: ShortHost(), Repo: dir, At: time.Now()}
	if local, err := a.List(); err == nil {
		note.Worktrees = checkouts(local)
	}
	for _, state := range a.KnownMachines() {
		if state.Name != runtime.Local && state.Name != machine.Name {
			note.Worktrees = append(note.Worktrees, state.Worktrees...)
		}
	}
	data, err := json.Marshal(note)
	if err != nil {
		return err
	}
	put := runtime.Command(machine.SSH, "sh", "-c", `mkdir -p "$HOME/`+notesDir+`" && cat > "$HOME/$1"`, "sh", NotePath(dir))
	put.Stdin = bytes.NewReader(data)
	return put.Run()
}

// ReadNote is the note a computer left here about this repository, if any.
func (a *App) ReadNote() *Note {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir, err := a.RemoteDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, NotePath(dir)))
	if err != nil {
		return nil
	}
	var note Note
	if json.Unmarshal(data, &note) != nil || note.Repo != dir {
		return nil
	}
	_, err = os.Stat(filepath.Join(home, HerePath(dir)))
	note.Here = err == nil
	return &note
}

// ShortHost is this computer's name, as a machine's radar shows it.
func ShortHost() string {
	host, err := os.Hostname()
	if err != nil {
		return "the computer you came from"
	}
	host, _, _ = strings.Cut(host, ".")
	return host
}

// checkouts are the listings someone could switch to: every adopted
// worktree, and the main checkout, which is where a lot of work happens.
func checkouts(listings []Listing) []Listing {
	var kept []Listing
	for _, l := range listings {
		if l.Adopted || l.Main {
			kept = append(kept, l)
		}
	}
	return kept
}
