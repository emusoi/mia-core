package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/store"
)

// Carried is a conversation a window's agent goes on from and was not told
// of yet: whose it was and which, the file mia wrote it out to, and the
// file that has every turn when that one leaves some out. It is kept on
// the window itself, as tmux's window option @mia_context, so that any
// chat of the window says it in front of the first thing sent, whichever
// process that is, and it goes with the window.
type Carried struct {
	From string `json:"from"`
	ID   string `json:"id"`
	File string `json:"file"`
	Full string `json:"full,omitempty"`
}

const carriedOption = "@mia_context"

// option is what is kept on the window, and carriedIn reads it back:
// nothing kept, or what does not read, is nothing carried.
func (c Carried) option() string {
	data, _ := json.Marshal(c) // of strings: it does not fail
	return string(data)
}

func carriedIn(option string) (Carried, bool) {
	var c Carried
	if json.Unmarshal([]byte(strings.TrimSpace(option)), &c) != nil || c.File == "" {
		return Carried{}, false
	}
	return c, true
}

// CarryInWindow keeps on a window what its agent goes on from.
func (a *App) CarryInWindow(record model.Record, window string, carried Carried) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	tmux := run.Tool{Binary: "tmux", SSH: host.SSH}
	if out, err := tmux.Combined("set-option", "-w", "-t", session.Target(record.Name, window), carriedOption, carried.option()); err != nil {
		return fmt.Errorf("keep what window %s of %s goes on from: %s", window, record.Name, strings.TrimSpace(out))
	}
	return nil
}

// CarriedInWindow is what a window's agent goes on from and was not told
// of yet, when there is any.
func (a *App) CarriedInWindow(record model.Record, window string) (Carried, bool) {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return Carried{}, false
	}
	out, err := run.Tool{Binary: "tmux", SSH: host.SSH}.Combined("show-options", "-w", "-q", "-v", "-t", session.Target(record.Name, window), carriedOption)
	if err != nil {
		return Carried{}, false
	}
	return carriedIn(out)
}

// TakeCarried is CarriedInWindow, taken off the window: of two chats of
// one window that send at once, one has it. Whoever takes it and does not
// send it keeps it again with CarryInWindow.
func (a *App) TakeCarried(record model.Record, window string) (Carried, bool) {
	if a.MiaDir != "" {
		if unlock, err := store.Lock(a.MiaDir, "context.lock"); err == nil {
			defer unlock()
		}
	}
	carried, on := a.CarriedInWindow(record, window)
	if !on {
		return Carried{}, false
	}
	host, window, _ := a.windowNamed(record, window)
	if _, err := (run.Tool{Binary: "tmux", SSH: host.SSH}).Combined("set-option", "-w", "-u", "-t", session.Target(record.Name, window), carriedOption); err != nil {
		return Carried{}, false // still on the window: said by the send that takes it off
	}
	return carried, true
}

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
	// Again: it is switched by being started again on its conversation.
	Again bool
}

// WindowRuns is what the window of a worktree's session runs, the window
// said by its name or number.
func (a *App) WindowRuns(record model.Record, window string) Runs {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return Runs{}
	}
	var front func() map[int]string
	if host.SSH == "" {
		front = foreground
	}
	agent, pane, known := agentInWindow(a.agents(), host.Panes(record.Name), window, front)
	if !known {
		return Runs{}
	}
	kept := a.chosen()[chosenKey(session.Name(record.Name), pane.PID)]
	runs := Runs{Agent: agent.Name, Model: kept.Model, Effort: kept.Effort, Account: kept.Account, Reads: agent.talks != nil,
		Live: agent.to != nil, Again: agent.again}
	if runs.Account == "" && agent.AccountEnv != "" {
		runs.Account = DefaultAccount
	}
	return runs
}

// agentInWindow is the agent a window of a session's panes runs, the
// window said by its name or number, and the window. It is told as
// SwitchWindow tells it — a window is its first pane, claude's binary
// named for its version is claude, and one behind node, or behind the
// shell mia started it by, is who front says it is — and failing that by
// its window's name, when it is named for its agent.
func agentInWindow(table []Agent, panes []session.Pane, window string, front func() map[int]string) (Agent, session.Window, bool) {
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
	agent, known := agentNamed(table, agentBehind(table, pane.Command, title, pane.Kind, pane.PID, front))
	if !known {
		agent, known = agentNamed(table, pane.Name)
	}
	return agent, pane, known
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
