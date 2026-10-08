package app

import (
	"path/filepath"

	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
)

func (a *App) SessionHostOf(record model.Record) (session.Host, error) {
	host := session.Here()
	host.Shell = a.Config.Dotfiles.Shell
	name := env.Placement(record).Runtime()
	if name == "" || name == runtime.Local {
		return host, nil
	}
	machine, err := a.Runtimes.Get(name)
	if err != nil {
		return session.Host{}, err
	}
	host.SSH = machine.SSH
	return host, nil
}

func (a *App) SessionDir(record model.Record) (string, error) {
	name := env.Placement(record).Runtime()
	if name == "" || name == runtime.Local {
		return record.Path, nil
	}
	machine, err := a.Runtimes.Get(name)
	if err != nil {
		return "", err
	}
	home, err := machine.Home()
	if err != nil {
		return "", err
	}
	return env.Staging(home, filepath.Base(a.Root), record.Name), nil
}

func (a *App) sessionOf(record model.Record) session.Host {
	host, err := a.SessionHostOf(record)
	if err != nil {
		return session.Here()
	}
	return host
}
