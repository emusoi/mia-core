package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/session"
)

// remembered is one worktree's session as last seen: its windows, the tmux
// server it lived in, when it was seen, and whether it was closed on purpose.
type remembered struct {
	Server  string    `json:"server"`
	Windows []Window  `json:"windows"`
	Seen    time.Time `json:"seen,omitzero"`
	Closed  bool      `json:"closed,omitempty"`
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
// computer goes down Resurrect can open them again. None is forgotten while
// its worktree is there: one gone while its server still runs was closed on
// purpose, and stays, marked so, for when it is wanted back by name.
func (a *App) Remember() error { return a.remember(nil) }

// remember is Remember, told which sessions Resurrect opened just now and
// with what: their programs are still starting, so a look sees only shells.
func (a *App) remember(opened map[string][]Window) error {
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
			windows := side{}.windows(a.agents(), record.Name, record.Path)
			if was, ok := opened[record.Name]; ok && len(was) == len(windows) {
				for i := range was { // opened in this order
					if then, now := was[i].panes(), windows[i].panes(); len(then) == len(now) {
						for j := range then {
							then[j].ID = now[j].ID
						}
					}
				}
				keepRunning(a.agents(), windows, was, a.Config.Keep)
			} else if had && before.Server == server {
				keepRunning(a.agents(), windows, before.Windows, a.Config.Keep)
			}
			kept[record.Name] = remembered{Server: server, Windows: windows, Seen: time.Now()}
		case had && before.Server == server && !before.Closed:
			before.Closed = true // gone while its server runs: closed on purpose
			kept[record.Name] = before
		case had:
			kept[record.Name] = before // went down with its server, or closed already
		}
	}
	data, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(a.sessionsFile(), data)
}

// forgetSession drops what is remembered of the session under a name.
func (a *App) forgetSession(name string) error {
	saved := a.readRemembered()
	if _, ok := saved[name]; !ok {
		return nil
	}
	delete(saved, name)
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(a.sessionsFile(), data)
}

// kept is what a pane keeps after it ends without being told: editors and
// dev servers, which run until stopped. The config's keep adds to it.
var kept = []string{"nvim", "vim", "hx", "npm run dev", "npm start", "pnpm dev", "pnpm run dev", "bun dev", "bun run dev", "yarn dev"}

// stays names the program a pane keeps after it ends — an agent, or one of
// kept and keep, by its first words — and is "" for any other.
func stays(table []Agent, command string, keep []string) string {
	if name, _ := agentOf(table, command); name != "" {
		return name
	}
	words := strings.Fields(command)
	if len(words) == 0 {
		return ""
	}
	words[0] = filepath.Base(words[0])
	for _, k := range slices.Concat(kept, keep) {
		if first := strings.Fields(k); len(first) > 0 && len(words) >= len(first) && slices.Equal(words[:len(first)], first) {
			return words[0]
		}
	}
	return ""
}

// keepRunning gives a pane now at its shell the program it was last seen
// running, when that is one that stays: so an agent, an editor or a dev
// server that ended before tmux went down still comes back. One quit on
// purpose comes back too, until its pane runs something else.
func keepRunning(table []Agent, now, before []Window, keep []string) {
	was := map[string]Pane{}
	for i := range before {
		for _, p := range before[i].panes() {
			was[p.ID] = *p
		}
	}
	for i := range now {
		for _, p := range now[i].panes() {
			if p.Command != "" || stays(table, was[p.ID].Command, keep) == "" {
				continue
			}
			p.Command, p.Conversation = was[p.ID].Command, was[p.ID].Conversation
		}
	}
}

// Revived is a session Resurrect opened again.
type Revived struct {
	Name    string
	Windows []Window
}

// Session is a remembered session, as mia resurrect --list says it.
type Session struct {
	Name    string    `json:"name"`
	State   string    `json:"state"` // running, down (went down with tmux) or closed
	Seen    time.Time `json:"seen,omitzero"`
	Windows []Window  `json:"windows"`
}

// Sessions is every remembered session, the latest seen first.
func (a *App) Sessions() []Session {
	server := tmuxServer()
	records, _ := a.Store.Load()
	var all []Session
	for name, was := range a.readRemembered() {
		if !slices.ContainsFunc(records, func(r model.Record) bool { return r.Name == name }) {
			continue // its worktree was removed since: there is nothing to open it in
		}
		state := "down"
		switch {
		case server != "" && session.Here().Exists(name):
			state = "running"
		case was.Closed || (server != "" && was.Server == server):
			state = "closed" // gone while the server it lived in still runs
		}
		all = append(all, Session{name, state, was.Seen, was.Windows})
	}
	slices.SortFunc(all, func(x, y Session) int { return y.Seen.Compare(x.Seen) })
	return all
}

// Resurrect opens sessions again, their windows in their folders and their
// programs restarted, a claude resuming the very conversation it had. With
// no names it opens each that went down with its tmux server; named, any
// remembered one that is not running, closed on purpose or not. dry only
// says which.
func (a *App) Resurrect(dry bool, names ...string) ([]Revived, error) {
	server := tmuxServer()
	records, err := a.Store.Load()
	if err != nil {
		return nil, err
	}
	saved := a.readRemembered()
	for _, name := range names {
		if _, ok := saved[name]; !ok {
			return nil, fmt.Errorf("mia remembers no session of %s", name)
		}
		if !slices.ContainsFunc(records, func(r model.Record) bool { return r.Name == name }) {
			return nil, fmt.Errorf("%s was removed, and its session with it — its branch stays: `mia new <branch>` makes a worktree of it again", name)
		}
	}
	var revived []Revived
	for _, record := range records {
		was, ok := saved[record.Name]
		wanted := slices.Contains(names, record.Name) || (len(names) == 0 && was.Server != server && !was.Closed)
		if !ok || !wanted || len(was.Windows) == 0 || session.Here().Exists(record.Name) {
			continue
		}
		windows := slices.Clone(was.Windows)
		for i := range windows {
			windows[i].Splits = slices.Clone(windows[i].Splits)
			for _, p := range windows[i].panes() {
				p.Command = again(a.agents(), p.Command, p.Conversation)
			}
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
		opened := map[string][]Window{}
		for _, r := range revived {
			opened[r.Name] = r.Windows
		}
		return revived, a.remember(opened)
	}
	return revived, nil
}

// again is how to start a window's program anew: an agent goes on with its
// folder's last conversation, a claude with the very one it had; anything
// else runs as it did.
// ponytail: an agent started with a prompt gets the prompt again, unquoted;
// keep each window's own conversation id, as for claude, if that bites.
func again(table []Agent, command, conversation string) string {
	name, args := agentOf(table, command)
	if name == "" {
		return command
	}
	agent, _ := agentNamed(table, name)
	start := name // by name: the path ps shows is one version's
	if name == "claude" {
		start = strings.Fields(command)[0]
	}
	if name == "claude" && conversation != "" {
		// whatever it resumed before, it resumes the one it had
		kept := []string{start}
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "-c", "--continue":
			case "-r", "--resume":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
				}
			default:
				kept = append(kept, args[i])
			}
		}
		return strings.Join(append(kept, "--resume", conversation), " ")
	}
	words := append([]string{start}, args...)
	resumes := slices.ContainsFunc(args, func(a string) bool { return slices.Contains(agent.has, a) })
	// codex resumes by a subcommand, which only flags may come before
	prompted := name == "codex" && slices.ContainsFunc(args, func(a string) bool { return !strings.HasPrefix(a, "-") })
	if !resumes && !prompted && agent.goOn != "" {
		words = append(words, agent.goOn)
	}
	return strings.Join(words, " ")
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
