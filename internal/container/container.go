package container

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/run"
)

type Engine struct {
	Binary string
	SSH    string
}

func (e Engine) command(args ...string) *exec.Cmd {
	return e.tool().Command(args...)
}

var ErrNoEngine = errors.New("no container engine")

func Find() (Engine, error) {
	for _, candidate := range []string{"podman", "docker"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return Engine{Binary: path}, nil
		}
	}
	return Engine{}, fmt.Errorf("%w — environments need podman or docker: https://podman.io/docs/installation", ErrNoEngine)
}

func (e Engine) tool() run.Tool {
	return run.Tool{Binary: e.Binary, SSH: e.SSH, Label: e.SSH, Detail: whatWentWrong}
}

func (e Engine) run(args ...string) (string, error) {
	out, err := e.tool().Capture(args...)
	if err != nil {
		where := ""
		if e.SSH != "" {
			where = " on " + e.SSH
		}
		return "", fmt.Errorf("%s %s%s: %s", short(e.Binary), args[0], where, err)
	}
	return out, nil
}

func (e Engine) Ready() error {
	if _, err := e.run("info", "--format", "{{.Host.Arch}}"); err != nil {
		if e.SSH != "" {
			return fmt.Errorf("%s cannot run containers — `systemctl --user start podman.socket` there, or check the machine is up", e.SSH)
		}
		return fmt.Errorf("%v — `podman machine list` says which machines are up, and podman talks to whichever `podman system connection list` marks default: start that one, or `podman system connection default <name>` to point it at another", err)
	}
	return nil
}

type Spec struct {
	Name    string
	Image   string
	Mount   string
	Labels  map[string]string
	Pass    map[string]string
	Private map[string]string
}

func (e Engine) Start(spec Spec) error {
	switch e.State(spec.Name) {
	case "running":
		return nil
	case "exited", "created", "stopped":
		_, err := e.run("start", spec.Name)
		return err
	}

	args := []string{"run", "-d", "--name", spec.Name}
	mount := parentOf(spec.Mount)
	args = append(args, "-v", mount+":"+mount, "--security-opt", "label=disable")
	args = append(args, "-w", spec.Mount)
	for _, volume := range sorted(spec.Private) {
		args = append(args, "-v", volume+":"+spec.Private[volume])
	}
	for _, name := range sorted(spec.Pass) {
		args = append(args, "-e", name+"="+spec.Pass[name])
	}
	for key, value := range spec.Labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args, spec.Image, "sleep", "infinity")

	_, err := e.run(args...)
	return err
}

func sorted(pairs map[string]string) []string {
	names := make([]string, 0, len(pairs))
	for name := range pairs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e Engine) Running() map[string]bool {
	out, err := e.run("ps", "--format", "{{.Names}}")
	if err != nil {
		return nil
	}
	up := map[string]bool{}
	for _, name := range strings.Fields(out) {
		up[name] = true
	}
	return up
}

func (e Engine) State(name string) string {
	out, err := e.run("inspect", "--format", "{{.State.Status}}", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (e Engine) Stop(name string) error {
	if e.State(name) == "" {
		return nil
	}
	_, err := e.run("stop", "-t", "5", name)
	return err
}

func (e Engine) RemoveVolumes(names []string) {
	for _, name := range names {
		e.run("volume", "rm", "-f", name)
	}
}

func (e Engine) Remove(name string) error {
	if e.State(name) == "" {
		return nil
	}
	_, err := e.run("rm", "-f", name)
	return err
}

func (e Engine) Exec(name string, argv ...string) (string, error) {
	return e.run(append([]string{"exec", "-w", "/", name}, argv...)...)
}

func (e Engine) ExecInteractive(name, dir string, argv []string) error {
	args := append(append([]string{"exec", "-it", "-w", dir}, terminal...), append([]string{name}, argv...)...)
	tool := e.tool()
	tool.Tty = true
	return tool.Interactive(args...)
}

func (e Engine) ExecDetached(name, dir string, argv []string) error {
	_, err := e.run(append([]string{"exec", "-d", "-w", dir, name}, argv...)...)
	return err
}

func (e Engine) HasImage(image string) bool {
	_, err := e.run("image", "exists", image)
	return err == nil
}

func (e Engine) Pull(image string) error {
	return e.tool().Stream(os.Stderr, "pull", image)
}

func (e Engine) Listening(name string) ([]int, error) {
	out, err := e.Exec(name, "sh", "-c",
		"ss -ltnH 2>/dev/null || netstat -ltn 2>/dev/null || cat /proc/net/tcp /proc/net/tcp6 2>/dev/null")
	if err != nil {
		return nil, err
	}
	return ParseListening(out), nil
}

func ParseListening(output string) []int {
	seen := map[int]bool{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if strings.EqualFold(fields[3], "0A") {
			if _, hex, found := strings.Cut(fields[1], ":"); found {
				if port, err := strconv.ParseInt(hex, 16, 32); err == nil && valid(int(port)) {
					seen[int(port)] = true
				}
			}
			continue
		}
		if at := strings.LastIndex(fields[3], ":"); at >= 0 {
			if port, err := strconv.Atoi(fields[3][at+1:]); err == nil && valid(port) {
				seen[port] = true
			}
		}
	}
	ports := make([]int, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	sortInts(ports)
	return ports
}

func valid(port int) bool { return port > 0 && port <= 65535 }

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func parentOf(path string) string {
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i]
	}
	return path
}

func short(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func whatWentWrong(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "Error: "); found {
			return rest
		}
	}
	return firstLine(text)
}

func firstLine(text string) string {
	if i := strings.Index(text, "\n"); i >= 0 {
		return text[:i]
	}
	return text
}

func (e Engine) Build(tag, containerfile string) error {
	command := e.command("build", "-t", tag, "-f", "-", ".")
	command.Stdin = strings.NewReader(containerfile)
	command.Stdout, command.Stderr = os.Stderr, os.Stderr
	return command.Run()
}

var terminal = []string{"-e", "TERM=xterm-256color"}

func (e Engine) ExecArgv(name, dir string, argv []string) []string {
	return append(append([]string{e.Binary, "exec", "-it", "-w", dir}, terminal...), append([]string{name}, argv...)...)
}

func (e Engine) ExecStdin(name, dir string, argv []string, stdin io.Reader) error {
	command := e.command(append([]string{"exec", "-i", "-w", dir}, append([]string{name}, argv...)...)...)
	command.Stdin = stdin
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %s", argv[0], firstLine(strings.TrimSpace(stderr.String())))
	}
	return nil
}

func (e Engine) Exec2(name string, argv ...string) error {
	_, err := e.Exec(name, argv...)
	return err
}

func (e Engine) ExecStatus(name, dir string, argv []string) (int, string) {
	return e.tool().Status(append([]string{"exec", "-w", dir, name}, argv...)...)
}
