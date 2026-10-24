package env

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
)

type ComposeSettings struct {
	File    string `toml:"file" json:"file,omitempty"`
	Service string `toml:"service" json:"service,omitempty"`
}

func (c ComposeSettings) Enabled() bool { return c.File != "" }

func (c ComposeSettings) ServiceName() string {
	if c.Service == "" {
		return "app"
	}
	return c.Service
}

func (c ComposeSettings) Validate() error {
	if c.File == "" {
		if c.Service != "" {
			return fmt.Errorf("env.compose.service needs env.compose.file")
		}
		return nil
	}
	clean := filepath.Clean(c.File)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("env.compose.file %q must be inside the worktree", c.File)
	}
	if strings.ContainsAny(c.ServiceName(), "/ \t\r\n") {
		return fmt.Errorf("env.compose.service %q must be one service name", c.ServiceName())
	}
	return nil
}

type composeEnvironment struct {
	Project   string
	File      string
	Service   string
	Container string
}

func (m Manager) composeFor(record model.Record, where Location) (*composeEnvironment, error) {
	if !m.Settings.Compose.Enabled() {
		return nil, nil
	}
	project := ContainerName(Sanitise(record.Name))
	file := filepath.Join(where.Dir, m.Settings.Compose.File)
	config, err := where.Engine.ComposeConfig(project, file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	service := m.Settings.Compose.ServiceName()
	selected, ok := config.Services[service]
	if !ok {
		return nil, fmt.Errorf("compose service %q is not in %s; available: %s", service, file, strings.Join(slices.Sorted(maps.Keys(config.Services)), ", "))
	}
	for name, service := range config.Services {
		if len(service.Ports) > 0 {
			return nil, fmt.Errorf("compose service %s publishes host ports; use `expose` so environments cannot collide", name)
		}
		if service.NetworkMode == "host" {
			return nil, fmt.Errorf("compose service %s uses the host network; use an isolated compose network", name)
		}
	}
	container := selected.ContainerName
	if container == "" {
		container = project + "-" + service + "-1"
	}
	id, err := where.Engine.ComposeID(project, file, service)
	if err != nil {
		return nil, err
	}
	if id != "" {
		container = id
	}
	return &composeEnvironment{Project: project, File: file, Service: service, Container: container}, nil
}

func (m Manager) CheckCompose(record model.Record) error {
	where, err := m.Where(record)
	if err != nil {
		return err
	}
	_, err = m.composeFor(record, where)
	return err
}

func ContainerNameFor(record model.Record, settings Settings) string {
	if settings.Compose.Enabled() {
		if record.Env != nil && record.Env.Container != "" {
			return record.Env.Container
		}
		return ContainerName(Sanitise(record.Name)) + "-" + settings.Compose.ServiceName() + "-1"
	}
	return ContainerName(Sanitise(record.Name))
}

func (m Manager) ContainerName(record model.Record) string {
	return ContainerNameFor(record, m.Settings)
}

func withComposeContainer(record model.Record, name string) model.Record {
	if record.Env == nil {
		record.Env = &model.Environment{Placement: model.Local()}
	}
	record.Env.Container = name
	return record
}
