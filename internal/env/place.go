package env

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/mirror"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

func Staging(home, repo, name string) string {
	return path.Join(home, ".mia", repo, name)
}

func Placement(record model.Record) model.Placement {
	if record.Env == nil {
		return model.Local()
	}
	return record.Env.Placement
}

type Location struct {
	Machine runtime.Machine
	Engine  container.Engine
	Dir     string
	Mirror  *mirror.Mirror
}

func (l Location) Remote() bool { return l.Machine.SSH != "" }

func (m Manager) Where(record model.Record) (Location, error) {
	placement := Placement(record)
	name := placement.Runtime()
	if name == "" || name == runtime.Local {
		return Location{Engine: m.Engine, Dir: record.Path}, nil
	}

	machine, err := m.Runtimes.Get(name)
	if err != nil {
		return Location{}, err
	}
	if machine.Engine == "" {
		return Location{}, fmt.Errorf("%s has no recorded container engine — `mia machine add %s %s` again", name, name, machine.SSH)
	}
	home, err := machine.Home()
	if err != nil {
		return Location{}, err
	}
	tool, err := mirror.Find()
	if err != nil {
		return Location{}, err
	}
	return Location{
		Machine: machine,
		Engine:  container.Engine{Binary: machine.Engine, SSH: machine.SSH},
		Dir:     Staging(home, filepath.Base(m.Repo), record.Name),
		Mirror:  &tool,
	}, nil
}
