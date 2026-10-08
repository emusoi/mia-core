package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

func (a *App) Windows(record model.Record) []session.Window {
	return a.sessionOf(record).WindowsOf(record.Name)
}

type shot struct {
	quiet time.Duration
	at    time.Time
	text  string
}

func (a *App) WindowsWithScreens(record model.Record) []session.Window {
	host := a.sessionOf(record)
	windows := a.Windows(record)
	for i, w := range windows {
		key := record.Name + "\x00" + strconv.Itoa(w.Index)
		if got, ok := a.shots.Load(key); ok {
			if s := got.(shot); w.Quiet >= s.quiet && time.Since(s.at) < 30*time.Second {
				windows[i].Screen = s.text
				continue
			}
		}
		text, _ := host.Capture(record.Name, strconv.Itoa(w.Index), true)
		windows[i].Screen = text
		a.shots.Store(key, shot{quiet: w.Quiet, at: time.Now(), text: text})
	}
	return windows
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
	return host.AttachWindow(record.Name, window)
}

func (a *App) SelectWindow(record model.Record, window string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return host.SelectWindow(record.Name, window)
}

func (a *App) NewWindow(record model.Record, window string, argv []string) (string, error) {
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
	return host.KillWindow(record.Name, window)
}

func (a *App) SendToWindow(record model.Record, window, text string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	return host.Send(record.Name, window, text)
}

func (a *App) WindowText(record model.Record, window string) string {
	text, _ := a.ReadWindow(record, window)
	return text
}

func (a *App) ReadWindow(record model.Record, window string) (string, error) {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return "", err
	}
	text, err := host.Capture(record.Name, window, false)
	return strings.TrimSpace(text), err
}
