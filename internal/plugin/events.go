package plugin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
)

var Events = []string{
	"worktree.created", "worktree.removed", "session.started",
	"env.up", "env.down", "stack.changed",
}

type Event struct {
	Protocol int    `json:"protocol"`
	Event    string `json:"event"`
	Repo     string `json:"repo"`
	Worktree string `json:"worktree"`
	Path     string `json:"path"`
	Branch   string `json:"branch,omitempty"`
	Detail   any    `json:"detail,omitempty"`
}

func (p Plugin) Wants(event string) bool {
	return slices.Contains(p.Manifest.Events, event) || slices.Contains(p.Manifest.Events, "*")
}

func (p Plugin) Send(c Context, e Event) error {
	if p.Server != nil {
		return p.Server.Notify("event", e)
	}
	env, err := p.Env(c)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(p.DataDir(c), "events.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	read, write, err := os.Pipe()
	if err != nil {
		return err
	}
	defer read.Close()
	cmd := exec.Command(p.Path, "event", e.Event)
	cmd.Env = env
	cmd.Stdin = read
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		write.Close()
		return err
	}
	go cmd.Wait()
	_, err = write.Write(append(payload, '\n'))
	if closeErr := write.Close(); err == nil {
		err = closeErr
	}
	return err
}
