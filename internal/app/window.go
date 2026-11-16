package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

func (a *App) Windows(record model.Record) []session.Window {
	return a.sessionOf(record).WindowsOf(record.Name)
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
// with no status line to poll it comes back all the same.
func (a *App) NewWindow(record model.Record, window string, argv []string) (string, error) {
	name, err := a.newWindow(record, window, argv)
	if err == nil {
		_ = a.Remember() // the window is open; what could not be written is seen at the next look
	}
	return name, err
}

func (a *App) newWindow(record model.Record, window string, argv []string) (string, error) {
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
			return window, host.NameFirstWindow(record.Name, window)
		}
	}
	if host.HasWindow(record.Name, window) {
		return "", fmt.Errorf("%s already has a window called %s", record.Name, window)
	}
	return window, host.StartWindow(record.Name, dir, window, argv)
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

func (a *App) SendToWindow(record model.Record, window, text string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return host.Send(record.Name, window, text)
}

// PressInWindow presses keys in a window without Enter after them.
func (a *App) PressInWindow(record model.Record, window, keys string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return host.Press(record.Name, window, keys)
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

func (a *App) ReadWindow(record model.Record, window string) (string, error) {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return "", err
	}
	text, err := host.Capture(record.Name, window, false)
	return strings.TrimSpace(text), err
}
