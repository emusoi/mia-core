package env

import (
	"fmt"
	"github.com/emusoi/mia-core/internal/mirror"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

type Settings struct {
	Image string `toml:"image"`

	Setup []string `toml:"setup"`

	ImageSetup []string `toml:"image_setup"`

	Ports []int `toml:"ports"`

	Page int `toml:"page"`

	PassHost bool `toml:"pass_host"`

	HostPorts []int `toml:"host_ports"`

	ContainerOnly []string `toml:"container_only"`

	Cache []string `toml:"cache"`

	Pass []string `toml:"pass"`
}

type Environment struct {
	Worktree  string          `json:"worktree"`
	Name      string          `json:"name"`
	Container string          `json:"container"`
	Image     Image           `json:"image"`
	State     string          `json:"state"`
	Listening []int           `json:"listening,omitempty"`
	Placement model.Placement `json:"placement"`
	Machine   string          `json:"machine"`
	Mirror    string          `json:"mirror,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type Manager struct {
	Engine   container.Engine
	Runtimes runtime.Store
	Repo     string
	Settings Settings
	Services []Service
	Dotfiles Dotfiles
}

type Snapshot struct {
	Environment
	Ports         Ports    `json:"ports"`
	Services      []Status `json:"services"`
	PortsError    string   `json:"portsError,omitempty"`
	ServicesError string   `json:"servicesError,omitempty"`
}

func (m Manager) Snapshot(record model.Record) Snapshot {
	where, whereErr := m.Where(record)
	e, portsErr := m.describe(record, where, whereErr)
	snapshot := Snapshot{Environment: e, Ports: Ports{Listening: e.Listening, Declared: m.Settings.Ports, Page: m.Settings.Page}}
	if portsErr != nil {
		snapshot.PortsError = portsErr.Error()
	}
	if whereErr != nil {
		snapshot.ServicesError = whereErr.Error()
	} else {
		snapshot.Services = m.serviceStatuses(where, e.Container, e.State == "running")
	}
	return snapshot
}

func (m Manager) Describe(record model.Record) Environment {
	where, err := m.Where(record)
	e, _ := m.describe(record, where, err)
	return e
}

func (m Manager) describe(record model.Record, where Location, whereErr error) (Environment, error) {
	name := ContainerName(Sanitise(record.Name))
	e := Environment{
		Worktree:  record.Path,
		Name:      record.Name,
		Container: name,
		Placement: Placement(record),
		Machine:   runtime.Local,
	}
	if machine := e.Placement.Runtime(); machine != "" {
		e.Machine = machine
	}
	if whereErr != nil {
		e.Image = DiscoverImage(record.Path, m.Settings.Image)
		e.Mirror = whereErr.Error()
		e.Error = whereErr.Error()
		return e, whereErr
	}
	e.Image, _ = m.imageFor(where, record)
	if where.Remote() {
		e.Machine = where.Machine.Name
		if session, found, _ := where.Mirror.Status(record.Name); found {
			e.Mirror = session.Describe()
		} else {
			e.Mirror = "not running"
		}
	}
	e.State = where.Engine.State(name)
	if e.State == "" {
		if err := where.Engine.Ready(); err != nil {
			e.Error = err.Error()
		}
	}
	if e.State == "running" {
		ports, err := where.Engine.Listening(name)
		if err != nil {
			return e, err
		}
		e.Listening = m.Settings.withoutForwarded(ports)
	}
	return e, nil
}

func (m Manager) Up(record model.Record, out *os.File) (Environment, error) {
	where, err := m.Where(record)
	if err != nil {
		return Environment{}, err
	}
	if err := where.Engine.Ready(); err != nil {
		return Environment{}, err
	}
	name := ContainerName(Sanitise(record.Name))
	image, _ := m.imageFor(where, record)

	if where.Remote() {
		fmt.Fprintf(out, "staging %s on %s\n", record.Name, where.Machine.Name)
		if _, err := runtime.Run(where.Machine.SSH, "mkdir", "-p", where.Dir); err != nil {
			return Environment{}, err
		}
		if err := where.Mirror.Ensure(record.Name, record.Path, where.Machine.SSH, where.Dir); err != nil {
			return Environment{}, err
		}
		// A manual worktree's session stays paused: it was seeded when it was
		// made, and moves again on `mia env sync <wt> now`. An auto one that
		// was paused follows again.
		session, found, _ := where.Mirror.Status(record.Name)
		switch {
		case found && session.Paused && record.Manual():
			fmt.Fprintf(out, "sync is manual for %s — `mia env sync %s now` to move changes\n", record.Name, record.Name)
		default:
			if found && session.Paused {
				if err := where.Mirror.Resume(record.Name); err != nil {
					return Environment{}, err
				}
			}
			if err := where.Mirror.Seed(record.Name); err != nil {
				return Environment{}, err
			}
		}
	}

	fresh := where.Engine.State(name) == ""
	if fresh && !where.Engine.HasImage(image.Ref) {
		fmt.Fprintf(out, "pulling %s (%s)\n", image.Ref, image.Source)
		if err := where.Engine.Pull(image.Ref); err != nil {
			return Environment{}, err
		}
	}

	volumes := m.privateVolumes(record, where, name)
	caches, notAbsolute := m.cacheVolumes()
	for _, dir := range notAbsolute {
		fmt.Fprintf(out, "⚠ [env] cache %q is not an absolute path inside the container — skipped\n", dir)
	}
	maps.Copy(volumes, caches)

	if err := where.Engine.Start(container.Spec{
		Name:    name,
		Image:   image.Ref,
		Mount:   where.Dir,
		Labels:  map[string]string{Label: record.Name},
		Pass:    m.Settings.Passing(),
		Private: volumes,
	}); err != nil {
		return Environment{}, err
	}

	if fresh && len(m.Settings.Setup) > 0 {
		fmt.Fprintf(out, "setup: %s\n", strings.Join(m.Settings.Setup, " "))
		if err := where.Engine.ExecInteractive(name, where.Dir, m.Settings.Setup); err != nil {
			return m.Describe(record), fmt.Errorf("setup failed: %w — the environment is up; `mia env shell %s` to look", err, record.Name)
		}
	}
	if len(m.Settings.HostPorts) > 0 {
		if _, err := where.Engine.Exec(name, "sh", "-c", "command -v socat >/dev/null"); err != nil {
			fmt.Fprintf(out, "⚠ host_ports needs socat in the image — add it to `image_setup`\n")
		}
	}
	for _, problem := range m.StartAutostart(record) {
		fmt.Fprintf(out, "⚠ %v\n", problem)
	}
	return m.Describe(record), nil
}

func (m Manager) Setup(record model.Record, out *os.File) error {
	if len(m.Settings.Setup) == 0 {
		return fmt.Errorf("no environment setup is configured — `[env] setup = [...]` in `mia config` names it")
	}
	where, name, err := m.running(record)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "setup: %s\n", strings.Join(m.Settings.Setup, " "))
	return where.Engine.ExecInteractive(name, where.Dir, m.Settings.Setup)
}

func (m Manager) Down(record model.Record) error {
	where, err := m.Where(record)
	if err != nil {
		return err
	}
	return where.Engine.Stop(ContainerName(Sanitise(record.Name)))
}

func (m Manager) Remove(record model.Record) error {
	where, err := m.Where(record)
	if err != nil {
		return err
	}
	name := ContainerName(Sanitise(record.Name))
	volumes := m.privateVolumes(record, where, name)
	if err := where.Engine.Remove(name); err != nil {
		return err
	}
	where.Engine.RemoveVolumes(slices.Sorted(maps.Keys(volumes)))
	if where.Remote() {
		if err := where.Mirror.Terminate(record.Name); err != nil {
			return err
		}
	}
	return nil
}

// HostSuffix ends the names the gateway serves an environment's ports at.
const HostSuffix = ".mia"

var loginShell = []string{"/bin/sh", "-lc", "exec ${SHELL:-/bin/bash} -l"}

func (m Manager) Shell(record model.Record) error {
	where, name, err := m.running(record)
	if err != nil {
		return err
	}
	return where.Engine.ExecInteractive(name, where.Dir, m.shellCommand())
}

// Sync carries out `mia env sync`: "auto" follows live, "manual" holds changes
// on each side, "now" moves them once and keeps the mode. It returns the
// session as it stands afterwards. The mode itself is the caller's to save.
func (m Manager) Sync(record model.Record, action string) (mirror.Session, error) {
	where, err := m.Where(record)
	if err != nil {
		return mirror.Session{}, err
	}
	if !where.Remote() {
		return mirror.Session{}, fmt.Errorf("%s runs on this machine — there is no copy to sync", record.Name)
	}
	session, found, err := where.Mirror.Status(record.Name)
	if err != nil || !found {
		return mirror.Session{}, fmt.Errorf("%s has no sync yet — `mia env up %s` makes it", record.Name, record.Name)
	}
	switch action {
	case "auto", "now":
		if session.Paused {
			if err := where.Mirror.Resume(record.Name); err != nil {
				return mirror.Session{}, err
			}
		}
		if err := where.Mirror.Seed(record.Name); err != nil {
			return mirror.Session{}, err
		}
		if action == "now" && record.Manual() {
			if err := where.Mirror.Pause(record.Name); err != nil {
				return mirror.Session{}, err
			}
		}
	case "manual":
		// Settle what is in flight first, so nothing is left half moved.
		if !session.Paused {
			if err := where.Mirror.Seed(record.Name); err != nil {
				return mirror.Session{}, err
			}
			if err := where.Mirror.Pause(record.Name); err != nil {
				return mirror.Session{}, err
			}
		}
	case "":
	default:
		return mirror.Session{}, fmt.Errorf("sync is auto, manual or now, not %q", action)
	}
	session, _, err = where.Mirror.Status(record.Name)
	return session, err
}

// ShellArgv is the command that opens a login shell inside the running
// environment, for a tmux window on the machine the environment runs on.
func (m Manager) ShellArgv(record model.Record) ([]string, error) {
	where, name, err := m.running(record)
	if err != nil {
		return nil, err
	}
	return where.Engine.ExecArgv(name, where.Dir, m.shellCommand()), nil
}

func (m Manager) Exec(record model.Record, argv []string) error {
	where, name, err := m.running(record)
	if err != nil {
		return err
	}
	return where.Engine.ExecInteractive(name, where.Dir, argv)
}

func (m Manager) running(record model.Record) (Location, string, error) {
	where, err := m.Where(record)
	if err != nil {
		return Location{}, "", err
	}
	name := ContainerName(Sanitise(record.Name))
	if where.Engine.State(name) != "running" {
		return Location{}, "", fmt.Errorf("%s has no environment running — `mia env up %s`", record.Name, record.Name)
	}
	return where, name, nil
}

type Ports struct {
	Listening []int `json:"listening"`
	Declared  []int `json:"declared,omitempty"`
	Page      int   `json:"page,omitempty"`
}

func (m Manager) PortsOf(record model.Record) (Ports, error) {
	ports := Ports{Declared: m.Settings.Ports, Page: m.Settings.Page}
	where, err := m.Where(record)
	if err != nil {
		return ports, err
	}
	name := ContainerName(Sanitise(record.Name))
	if where.Engine.State(name) != "running" {
		return ports, nil
	}
	listening, err := where.Engine.Listening(name)
	if err != nil {
		return ports, err
	}
	ports.Listening = m.Settings.withoutForwarded(listening)
	return ports, nil
}

func (m Manager) Check(record model.Record, argv []string) (exit int, output string, where string) {
	location, err := m.Where(record)
	if err == nil {
		name := ContainerName(Sanitise(record.Name))
		if location.Engine.State(name) == "running" {
			exit, out := location.Engine.ExecStatus(name, location.Dir, []string{"sh", "-c", shellJoin(argv)})
			return exit, out, "inside " + record.Name
		}
	}
	return CheckHere(record, argv)
}

func CheckHere(record model.Record, argv []string) (exit int, output string, where string) {
	out, err := runHere(record.Path, argv)
	return exitOf(err), out, "in the worktree"
}

func (s Settings) withoutForwarded(ports []int) []int {
	if len(s.HostPorts) == 0 {
		return ports
	}
	kept := make([]int, 0, len(ports))
	for _, port := range ports {
		if !slices.Contains(s.HostPorts, port) {
			kept = append(kept, port)
		}
	}
	return kept
}

// Passing is the environment variables [env] pass names, as this shell has
// them, for the container.
func (s Settings) Passing() map[string]string {
	carry := map[string]string{}
	for _, name := range s.Pass {
		if value := os.Getenv(name); value != "" {
			carry[name] = value
		}
	}
	return carry
}
