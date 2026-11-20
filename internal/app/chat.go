package app

import (
	"slices"
	"strconv"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

// Runs is what a worktree's window runs, as a chat of it says it: the
// agent, what mia started it with or last switched it to, and what mia can
// do with it there.
type Runs struct {
	// Agent is the agent's name; "" when the window runs none mia knows.
	Agent string
	// Model, Effort and Account are a list of conversations' for the one
	// the window has open: what mia chose, and the agent's default account
	// when it keeps accounts and none was chosen.
	Model, Effort, Account string
	// Reads: mia reads this agent's conversations, so a chat has its replies.
	Reads bool
	// Live: it is switched to another model while it runs.
	Live bool
}

// WindowRuns is what the window of a worktree's session runs, the window
// said by its name or number.
func (a *App) WindowRuns(record model.Record, window string) Runs {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return Runs{}
	}
	// as SwitchWindow tells it: a window is its first pane, claude's binary
	// named for its version is claude, and one behind node says who it is
	panes := host.Panes(record.Name)
	var pane session.Window
	for _, one := range session.WindowsFrom(panes) {
		if one.Name == window || strconv.Itoa(one.Index) == window {
			pane = one
		}
	}
	title := ""
	for _, one := range slices.Backward(panes) {
		if one.Index == pane.Index {
			title = one.Title
		}
	}
	command := pane.Command
	if (command == "node" || command == "bun") && host.SSH == "" {
		command = foreground()[pane.PID]
	}
	agent, known := agentNamed(a.agents(), agentIn(a.agents(), command, title))
	if !known {
		agent, known = agentNamed(a.agents(), pane.Name) // a window named for its agent
	}
	if !known {
		return Runs{}
	}
	kept := a.chosen()[chosenKey(session.Name(record.Name), pane.PID)]
	runs := Runs{Agent: agent.Name, Model: kept.Model, Effort: kept.Effort, Account: kept.Account, Reads: agent.talks != nil, Live: agent.to != nil}
	if runs.Account == "" && agent.AccountEnv != "" {
		runs.Account = DefaultAccount
	}
	return runs
}

// SayToWindow types text into a window as one message and submits it, as
// the main agent is said something: pasted, so a line break in it is not
// an Enter, and never into a question, which Enter would answer.
func (a *App) SayToWindow(record model.Record, window, text string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return say(host, record.Name, window, a.agentOfWindow(host, record.Name, window), "mia window press "+record.Name+" "+window+" --", text)
}

// WindowHasPane reports whether a pane, by tmux's id of it, is in a window
// of a worktree's session: a program asks before it closes a window that
// may be its own.
func (a *App) WindowHasPane(record model.Record, window, pane string) bool {
	host, window, err := a.windowNamed(record, window)
	if err != nil || pane == "" {
		return false
	}
	return slices.ContainsFunc(host.Panes(record.Name), func(p session.Pane) bool {
		return p.ID == pane && (p.Window == window || strconv.Itoa(p.Index) == window)
	})
}

// AsksInWindow is the question a window's screen shows, when it shows one:
// what SendToWindow refuses to type over, and PressInWindow answers.
func (a *App) AsksInWindow(record model.Record, window string) (Asks, bool) {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return Asks{}, false
	}
	return askingIn(host, record.Name, window)
}
