package app

import (
	"fmt"
	"path/filepath"

	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

func (a *App) removeEnvironment(record model.Record) error {
	engine, err := container.Find()
	if err != nil {
		if record.Env != nil {
			return err
		}
		return nil
	}
	manager := env.Manager{
		Engine:   engine,
		Runtimes: a.Runtimes,
		Repo:     a.Root,
		Settings: a.Config.Env,
		Services: a.Config.Services,
		Dotfiles: a.Config.Dotfiles,
	}
	if err := manager.Remove(record); err != nil {
		return fmt.Errorf("environment cleanup failed for %s; the worktree and its records were kept so you can retry: %w", record.Name, err)
	}
	return nil
}

func (a *App) Host(record model.Record, machine runtime.Machine) (model.Record, error) {
	placement := model.Local()
	if machine.Name != runtime.Local {
		home, err := machine.Home()
		if err != nil {
			return record, err
		}
		placement, err = model.OnRuntime(machine.Name, env.Staging(home, filepath.Base(a.Root), record.Name))
		if err != nil {
			return record, err
		}
	}
	sync := ""
	if record.Env != nil {
		sync = record.Env.Sync
	}
	record.Env = &model.Environment{Placement: placement, Sync: sync}
	return record, a.Store.Put(record)
}
