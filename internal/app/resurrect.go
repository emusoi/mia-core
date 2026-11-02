package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/session"
)

// remembered is one worktree's session as last seen: its windows, and the
// tmux server it lived in.
type remembered struct {
	Server  string   `json:"server"`
	Windows []Window `json:"windows"`
}

func (a *App) sessionsFile() string { return filepath.Join(a.MiaDir, "sessions.json") }

func (a *App) readRemembered() map[string]remembered {
	all := map[string]remembered{}
	if data, err := os.ReadFile(a.sessionsFile()); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	return all
}

// tmuxServer names the running tmux server, "" when none runs.
// ponytail: its pid; a reboot that hands the new server the old pid makes
// resurrect see nothing to do — rare, and mia shell still opens each one.
func tmuxServer() string {
	out, err := run.Local("tmux").Capture("display-message", "-p", "#{pid}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Remember keeps each local session's windows, so that after tmux or the
// computer goes down, Resurrect can open them again. A session gone while
// its server still runs was closed on purpose, and is forgotten.
func (a *App) Remember() error {
	server := tmuxServer()
	if server == "" {
		return nil // no tmux: nothing to see, and what was seen stays
	}
	records, err := a.Store.Load()
	if err != nil {
		return err
	}
	saved := a.readRemembered()
	kept := map[string]remembered{}
	for _, record := range records {
		if a.sessionOf(record).SSH != "" {
			continue
		}
		before, had := saved[record.Name]
		switch {
		case session.Here().Exists(record.Name):
			kept[record.Name] = remembered{server, side{}.windows(record.Name, record.Path)}
		case had && before.Server != server:
			kept[record.Name] = before // its server went down with it
		}
	}
	data, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(a.sessionsFile(), data)
}

// Revived is a session Resurrect opened again.
type Revived struct {
	Name    string
	Windows []Window
}

// Resurrect opens again each session that went down with its tmux server,
// its windows in their folders and their programs restarted, an agent
// continuing its last conversation. dry only says which.
func (a *App) Resurrect(dry bool) ([]Revived, error) {
	server := tmuxServer()
	records, err := a.Store.Load()
	if err != nil {
		return nil, err
	}
	saved := a.readRemembered()
	var revived []Revived
	for _, record := range records {
		was, ok := saved[record.Name]
		if !ok || was.Server == server || len(was.Windows) == 0 || session.Here().Exists(record.Name) {
			continue
		}
		windows := slices.Clone(was.Windows)
		for i := range windows {
			windows[i].Command = again(windows[i].Command)
		}
		if !dry {
			if err := a.ensureSession(session.Here(), record, record.Path); err != nil {
				return revived, err
			}
			if err := (side{}).open(record.Name, record.Path, windows); err != nil {
				return revived, err
			}
		}
		revived = append(revived, Revived{record.Name, windows})
	}
	if !dry && len(revived) > 0 {
		return revived, a.Remember()
	}
	return revived, nil
}

// again is how to start a window's program anew: an agent continues its
// last conversation in that folder; anything else runs as it did.
func again(command string) string {
	words := strings.Fields(command)
	if len(words) == 0 {
		return ""
	}
	switch filepath.Base(words[0]) {
	case "claude":
		if slices.ContainsFunc(words[1:], func(w string) bool {
			return w == "-c" || w == "--continue" || w == "-r" || w == "--resume"
		}) {
			return command
		}
		return command + " --continue"
	case "codex":
		if len(words) == 1 {
			return "codex resume --last"
		}
	}
	return command
}

func writeAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".sessions-*")
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
	return os.Rename(temp.Name(), path)
}
