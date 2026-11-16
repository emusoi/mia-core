package app

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/paths"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/store"
)

// The main agent is the one a person talks to, which runs the others: one
// for the person, on one machine, in a session of its own that is no
// worktree's. Its folder is mia's, under the user-wide config, and what it
// did is read from its own conversation.

// mainName is its session's, mia-main, held in the name registry so that
// no worktree is given it.
const mainName = "main"

// mainNotes is what its folder tells it: who it is, and what it may answer.
//
//go:embed main.md
var mainNotes string

// AtMain is mia at the main agent's folder, outside any repository: it
// knows the machines, the repositories and the main agent, and nothing one
// repository keeps.
func AtMain() (*App, error) {
	home := paths.In(mainName)
	if home == "" {
		return nil, errors.New("cannot find a home directory to keep the main agent in")
	}
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}
	nameStore, err := globalNames()
	if err != nil {
		return nil, err
	}
	state := filepath.Join(home, ".mia")
	return &App{
		Root:     git.Resolve(home),
		MiaDir:   state,
		Config:   cfg,
		Store:    store.Store{Dir: state},
		Names:    nameStore,
		Runtimes: runtime.Store{Path: config.RuntimesPath()},
	}, nil
}

// MainState is the main agent as an app shows it.
type MainState struct {
	Running bool `json:"running"`
	// Agent is the one that runs, or the configured one when none does.
	Agent   string `json:"agent"`
	Machine string `json:"machine"`
	Home    string `json:"home"`
	Session string `json:"session"`
	// Window is the agent's in the session; left out when it is not running.
	Window *WindowRef `json:"window,omitempty"`
	// Conversation is the one it has open, or would go on with; "" when it
	// has had none, or is an agent mia cannot read the conversations of.
	Conversation string `json:"conversation"`
	// Working: it is doing something now, as its pane says.
	Working bool `json:"working"`
	// Since is when its session was started.
	Since time.Time `json:"since,omitzero"`
}

func (a *App) mainHost() session.Host {
	host := session.Here()
	host.Shell = a.Config.Dotfiles.Shell
	return host
}

// mainAgent is the agent the config names as the main one, from the table.
func (a *App) mainAgent() (Agent, error) {
	name := a.Config.Main.Agent
	if name == "" {
		name = "claude"
	}
	agent, known := agentNamed(a.agents(), name)
	if !known {
		return Agent{}, fmt.Errorf("[main] agent = %q, and mia knows no agent of that name — a plugin that adds it has to be enabled", name)
	}
	return agent, nil
}

// mainWindow is the window of its session an agent was started in: each is
// named for its agent, and goes when the agent ends.
func (a *App) mainWindow(host session.Host) (session.Window, session.Pane, bool) {
	panes := host.Panes(mainName)
	for _, window := range session.WindowsFrom(panes) {
		if _, known := agentNamed(a.agents(), window.Name); !known {
			continue
		}
		at := slices.IndexFunc(panes, func(p session.Pane) bool { return p.Index == window.Index })
		return window, panes[at], true
	}
	return session.Window{}, session.Pane{}, false
}

// workingFor is how lately a pane must have written for an agent that
// gives its pane no title to count as working.
const workingFor = 3 * time.Second

// Main is where the main agent stands. Nothing is started or written.
func (a *App) Main() MainState {
	host := a.mainHost()
	state := MainState{Machine: runtime.Local, Home: a.Root, Session: session.Name(mainName)}
	agent, _ := a.mainAgent()
	state.Agent = agent.Name
	if state.Agent == "" {
		state.Agent = a.Config.Main.Agent
	}
	window, pane, running := a.mainWindow(host)
	if running {
		agent, _ = agentNamed(a.agents(), window.Name)
		state.Running, state.Agent = true, agent.Name
		state.Window = &WindowRef{Index: window.Index, Name: window.Name}
		state.Since = host.Created(mainName).UTC()
		// claude says it in its pane's title; of another, writing is the sign
		state.Working = session.Spinning(pane.Title) ||
			(session.AgentTitle(pane.Title) == "" && !pane.Activity.IsZero() && time.Since(pane.Activity) < workingFor)
	}
	state.Conversation = a.mainConversation(agent, pane.PID)
	return state
}

// mainConversation is the conversation the agent has open in the pane, when
// it says, and otherwise the one of its folder last written to.
func (a *App) mainConversation(agent Agent, pid int) string {
	if agent.talks == nil {
		return ""
	}
	if agent.held != "" && pid > 0 {
		out, _ := run.Local("sh").Capture("-c", agent.held, "sh", strconv.Itoa(pid))
		if held := sessionIn(out); held != "" {
			return held
		}
	}
	in, _ := a.where()
	heads, _ := agent.talks.heads(in)
	if len(heads) == 0 {
		return ""
	}
	return slices.MaxFunc(heads, func(x, y Head) int { return x.Active.Compare(y.Active) }).ID
}

// StartMain makes the main agent's folder and session when they are not
// there and starts the agent in it, going on with its last conversation
// when it has one. One that runs already is left as it is.
func (a *App) StartMain() (MainState, error) {
	agent, err := a.mainAgent()
	if err != nil {
		return MainState{}, err
	}
	host := a.mainHost()
	if !host.Available() {
		return MainState{}, errors.New("tmux not found — the main agent runs in a session: https://github.com/tmux/tmux/wiki/Installing")
	}
	home := paths.In(mainName)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return MainState{}, err
	}
	a.Root = git.Resolve(home) // there now, so as an agent's transcript says it
	if err := a.Names.Claim(mainName, a.Root); err != nil {
		return MainState{}, fmt.Errorf("the main agent's session is mia-%s: %w", mainName, err)
	}
	notes := filepath.Join(a.Root, agent.notesFile())
	if was, _ := os.ReadFile(notes); string(was) != mainNotes {
		if err := os.WriteFile(notes, []byte(mainNotes), 0o644); err != nil {
			return MainState{}, err
		}
	}
	if _, _, running := a.mainWindow(host); running {
		return a.Main(), nil
	}
	command := agent.start()
	if last := a.mainConversation(agent, 0); last != "" && agent.resume != nil {
		if command, err = agent.resume(last); err != nil {
			return MainState{}, fmt.Errorf("agent %s: %w", agent.Name, err)
		}
	}
	if err := host.Ensure(mainName, a.Root); err != nil {
		return MainState{}, err
	}
	shell := a.Config.Dotfiles.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	// by a login shell, for its PATH, and with nothing after: the window is
	// the agent, and goes when it ends
	if err := host.StartWindow(mainName, a.Root, agent.Name, []string{shell, "-lc", command}); err != nil {
		return MainState{}, err
	}
	time.Sleep(time.Second)
	if _, _, running := a.mainWindow(host); !running {
		return MainState{}, fmt.Errorf("%s ended as soon as it started — `%s` in %s shows why", agent.Name, command, a.Root)
	}
	return a.Main(), nil
}

// StopMain ends the main agent's session. Its folder and conversations stay.
func (a *App) StopMain() error {
	return a.mainHost().Kill(mainName)
}

var errMainDown = errors.New("the main agent is not running — `mia main start`")

// SayToMain types text into the main agent as one message and submits it:
// pasted, so a line break in it is not an Enter, and Enter after.
func (a *App) SayToMain(text string) error {
	host := a.mainHost()
	window, _, running := a.mainWindow(host)
	if !running {
		return errMainDown
	}
	if err := host.Paste(mainName, window.Name, text); err != nil {
		return err
	}
	return host.Keys(mainName, window.Name, []string{"Enter"})
}

// InterruptMain presses the key that stops the main agent mid-work.
func (a *App) InterruptMain() error {
	host := a.mainHost()
	window, _, running := a.mainWindow(host)
	if !running {
		return errMainDown
	}
	agent, _ := agentNamed(a.agents(), window.Name)
	return host.Keys(mainName, window.Name, []string{agent.stopKey()})
}

// MainTurns is the main agent's conversation as turns, those after turn
// number after, with each of mia's verbs it ran that changed something
// said as what it did.
func (a *App) MainTurns(after int) ([]Turn, error) {
	state := a.Main()
	if state.Conversation == "" {
		return []Turn{}, nil
	}
	turns, err := a.ConversationTurns(state.Conversation, after)
	if err != nil {
		return nil, err
	}
	places := a.places()
	for i := range turns {
		turns[i] = a.did(turns[i], places)
	}
	return turns, nil
}
