package app

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

func (a *App) Windows(record model.Record) []session.Window {
	return session.WindowsWith(a.sessionOf(record).Panes(record.Name), a.apps())
}

func (a *App) windowNamed(record model.Record, window string) (session.Host, string, error) {
	host := a.sessionOf(record)
	if index, err := strconv.Atoi(window); err == nil {
		for _, one := range host.WindowsOf(record.Name) {
			if one.Index == index {
				return host, strconv.Itoa(index), nil
			}
		}
	}
	if host.HasWindow(record.Name, window) {
		return host, window, nil
	}
	return host, "", fmt.Errorf("%s has no window called %s", record.Name, window)
}

func (a *App) OpenWindow(record model.Record, window string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	_ = a.Remember() // before landing: attaching may never come back here
	return host.AttachWindow(record.Name, window)
}

func (a *App) SelectWindow(record model.Record, window string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return host.SelectWindow(record.Name, window)
}

// NewWindow opens a window in the worktree's session, starting the session
// when there is none, and remembers the session as it is then: one made
// with no status line to poll it comes back all the same. Its pane is
// marked with what it is: what is given, or what its command is for. With
// no name, it is called as tmux titles its pane, told apart from a window
// of that name already there by a number.
func (a *App) NewWindow(record model.Record, window string, what session.What, argv []string) (string, error) {
	if what.Kind == "" {
		what = a.WhatOf(strings.Join(argv, " "))
	}
	if window == "" {
		window = freeWindow(a.sessionOf(record), record.Name, titleOf(what, argv, record.Path))
	}
	name, err := a.newWindow(record, window, what, argv)
	if err == nil {
		_ = a.Remember() // the window is open; what could not be written is seen at the next look
	}
	return name, err
}

func (a *App) newWindow(record model.Record, window string, what session.What, argv []string) (string, error) {
	dir, err := a.SessionDir(record)
	if err != nil {
		return "", err
	}
	host := a.sessionOf(record)
	if !host.Exists(record.Name) {
		if err := a.ensureSession(host, record, dir); err != nil {
			return "", err
		}
		if len(argv) == 0 {
			if err := host.Mark(session.Target(record.Name, "^"), what); err != nil {
				return "", err
			}
			return window, host.NameFirstWindow(record.Name, window)
		}
	}
	if host.HasWindow(record.Name, window) {
		return "", fmt.Errorf("%s already has a window called %s", record.Name, window)
	}
	return window, host.StartWindow(record.Name, dir, window, what, argv)
}

// titleOf is session.Title for a pane about to start: its label; else
// "shell", or the command it runs, and its folder.
func titleOf(what session.What, argv []string, dir string) string {
	if what.Label != "" {
		return what.Label
	}
	command := "shell"
	if words := argv[withoutEnv(argv):]; len(words) > 0 && !shells[filepath.Base(words[0])] {
		command = filepath.Base(words[0])
	}
	return command + " · " + filepath.Base(dir)
}

func (a *App) CloseWindow(record model.Record, window string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	if err := host.KillWindow(record.Name, window); err != nil {
		return err
	}
	_ = a.Remember() // without the window, so it is not brought back
	return nil
}

// SendToWindow types text into a window and Enter after it, unless the
// window asks something: Enter would answer it.
func (a *App) SendToWindow(record model.Record, window, text string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	if asks, on := askingIn(host, record.Name, window); on {
		return askedFirst(a.agentOfWindow(host, record.Name, window), asks, "mia window press "+record.Name+" "+window+" --")
	}
	return host.Send(record.Name, window, text)
}

// agentOfWindow is the agent a window runs, by name, or the window when
// mia knows none there.
func (a *App) agentOfWindow(host session.Host, worktree, window string) string {
	if _, known := agentNamed(a.agents(), window); known {
		return window
	}
	for _, pane := range host.Panes(worktree) {
		if pane.Window == window || strconv.Itoa(pane.Index) == window {
			if name := agentIn(a.agents(), pane.Command, pane.Title); name != "" {
				return name
			}
		}
	}
	return "window " + window
}

// PressInWindow presses keys in a window without Enter after them: a
// number where it asks with a cursor and no numbers moves the cursor to
// that option and presses Enter.
func (a *App) PressInWindow(record model.Record, window, keys string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return pressAnswer(host, record.Name, window, keys)
}

// KeysInWindow presses keys in a window by tmux's names for them — C-c,
// Escape, Enter, Up — where PressInWindow types text as it is.
func (a *App) KeysInWindow(record model.Record, window string, keys []string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return host.Keys(record.Name, window, keys)
}

// StopInWindow presses the key that stops the agent a window runs
// mid-work, as KeysInWindow presses one: Escape, unless the agent has
// another.
func (a *App) StopInWindow(record model.Record, window string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	agent, _ := agentNamed(a.agents(), a.agentOfWindow(host, record.Name, window))
	return host.Keys(record.Name, window, []string{agent.stopKey()})
}

func (a *App) ReadWindow(record model.Record, window string) (string, error) {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return "", err
	}
	text, err := host.Capture(record.Name, window, false)
	return strings.TrimSpace(text), err
}
