package app

import (
	"path/filepath"

	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

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
