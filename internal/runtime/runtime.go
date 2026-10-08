package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/emusoi/mia-core/internal/run"
)

type Machine struct {
	Name string `json:"name"`

	SSH string `json:"ssh"`

	Engine string `json:"engine"`
}

const Local = "local"

type Store struct{ Path string }

func (s Store) All() ([]Machine, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var machines []Machine
	if err := json.Unmarshal(data, &machines); err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Path, err)
	}
	sort.Slice(machines, func(i, j int) bool { return machines[i].Name < machines[j].Name })
	return machines, nil
}

func (s Store) Get(name string) (Machine, error) {
	if name == "" || name == Local {
		return Machine{Name: Local}, nil
	}
	machines, err := s.All()
	if err != nil {
		return Machine{}, err
	}
	for _, machine := range machines {
		if machine.Name == name {
			return machine, nil
		}
	}
	known := []string{Local}
	for _, machine := range machines {
		known = append(known, machine.Name)
	}
	return Machine{}, fmt.Errorf("no machine called %q — there is %s", name, strings.Join(known, ", "))
}

func (s Store) Add(machine Machine) error {
	if machine.Name == Local {
		return fmt.Errorf("%q is this computer and is always available", Local)
	}
	machines, err := s.All()
	if err != nil {
		return err
	}
	replaced := false
	for i := range machines {
		if machines[i].Name == machine.Name {
			machines[i], replaced = machine, true
		}
	}
	if !replaced {
		machines = append(machines, machine)
	}
	return s.write(machines)
}

func (s Store) Remove(name string) error {
	machines, err := s.All()
	if err != nil {
		return err
	}
	kept := machines[:0]
	for _, machine := range machines {
		if machine.Name != name {
			kept = append(kept, machine)
		}
	}
	return s.write(kept)
}

func (s Store) write(machines []Machine) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	sort.Slice(machines, func(i, j int) bool { return machines[i].Name < machines[j].Name })
	data, err := json.MarshalIndent(machines, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.Path), ".mia-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.Path)
}

func Probe(target string) (Machine, error) {
	machine := Machine{SSH: target}
	out, err := Run(target, "sh", "-c", "command -v podman || command -v docker")
	if err != nil {
		return machine, fmt.Errorf("cannot reach %s over ssh: %w", target, err)
	}
	engine := strings.TrimSpace(run.FirstLine(out))
	if engine == "" {
		return machine, fmt.Errorf(
			"%s has no container engine — an environment is always a container: `sudo dnf install -y podman` there, then try again", target)
	}
	machine.Engine = engine
	return machine, nil
}

func Reachable(machine Machine) (bool, string) {
	out, err := Run(machine.SSH, "sh", "-c", "uname -sr; command -v podman || command -v docker")
	if err != nil {
		return false, run.FirstLine(err.Error())
	}
	return true, strings.ReplaceAll(strings.TrimSpace(out), "\n", " · ")
}

func Interactive(target string, argv ...string) *exec.Cmd {
	if len(argv) == 0 {
		return run.Shell(target)
	}
	tool := run.Tool{Binary: argv[0], SSH: target, Tty: true}
	command := tool.Command(argv[1:]...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command
}

func Command(target string, argv ...string) *exec.Cmd {
	return run.Tool{Binary: argv[0], SSH: target}.Command(argv[1:]...)
}

func Run(target string, argv ...string) (string, error) {
	tool := run.Tool{Binary: argv[0], SSH: target, Detail: run.FirstLine}
	out, err := tool.Capture(argv[1:]...)
	if err != nil {
		return "", fmt.Errorf("%s: %s", target, err)
	}
	return out, nil
}

func (m Machine) Home() (string, error) {
	if m.SSH == "" {
		return os.UserHomeDir()
	}
	homesMu.Lock()
	defer homesMu.Unlock()
	if home, ok := homes[m.SSH]; ok {
		return home, nil
	}
	home, err := Run(m.SSH, "sh", "-c", "echo $HOME")
	if err != nil {
		return "", err
	}
	homes[m.SSH] = home
	return home, nil
}

var (
	homesMu sync.Mutex
	homes   = map[string]string{}
)
