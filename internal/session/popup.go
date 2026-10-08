package session

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/runtime"

	exe "github.com/emusoi/mia-core/internal/run"
)

func (h Host) AttachPopup(worktree, dir string) error {
	if !isTerminal(os.Stdout) {
		return fmt.Errorf("a session popup needs a terminal")
	}
	run := func(args ...string) (string, error) {
		out, err := h.run(args...)
		if err != nil {
			return "", fmt.Errorf("tmux %s: %s: %w", args[0], strings.TrimSpace(out), err)
		}
		return strings.TrimSpace(out), nil
	}
	target := exact(Name(worktree))
	socket, err := run("display-message", "-p", "-t", target+":", "#{socket_path}")
	if err != nil {
		return err
	}
	parent := ""
	if h.SSH == "" && os.Getenv("TMUX_PANE") != "" {
		parent, _ = run("display-message", "-p", "-t", os.Getenv("TMUX_PANE"), "#{window_id}")
		if parent != "" {
			_, _ = h.run("set-option", "-w", "-t", target+":"+parent, "@mia_editor", "1")
		}
	}
	panes, err := run("list-panes", "-s", "-t", target, "-F", "#{window_id}|#{pane_current_command}|#{@mia_editor}")
	if err != nil {
		return err
	}
	var windows []string
	blocked := map[string]bool{}
	for line := range strings.SplitSeq(panes, "\n") {
		fields := strings.SplitN(line, "|", 3)
		if len(fields) < 2 {
			continue
		}
		window, command := fields[0], fields[1]
		editor := len(fields) == 3 && fields[2] == "1"
		if _, seen := blocked[window]; !seen {
			windows = append(windows, window)
		}
		blocked[window] = blocked[window] || editor || window == parent || command == "nvim" || command == "vim"
	}
	selected := ""
	for _, window := range windows {
		if !blocked[window] {
			selected = window
			break
		}
	}
	if selected == "" {
		selected, err = run("new-window", "-d", "-P", "-F", "#{window_id}", "-t", target+":", "-n", "shell", "-c", dir)
		if err != nil {
			return err
		}
		windows = append(windows, selected)
	}
	view := fmt.Sprintf("mia-popup-%d-%d", os.Getpid(), time.Now().UnixNano())
	viewTarget := exact(view)
	holder, err := run("new-session", "-d", "-P", "-F", "#{window_id}", "-s", view, "-n", "mia-popup-holder", "-c", dir)
	if err != nil {
		return err
	}
	defer h.run("kill-session", "-t", viewTarget)
	for _, window := range windows {
		if blocked[window] {
			continue
		}
		if _, err := run("link-window", "-a", "-s", target+":"+window, "-t", viewTarget+":"); err != nil {
			return err
		}
	}
	commands := [][]string{
		{"select-window", "-t", viewTarget + ":" + selected},
		{"kill-window", "-t", viewTarget + ":" + holder},
		{"set-option", "-t", viewTarget + ":", "@mia_worktree", worktree},
		{"set-option", "-t", viewTarget + ":", "detach-on-destroy", "on"},
		{"set-hook", "-t", viewTarget + ":", "after-new-window",
			fmt.Sprintf("run-shell \"tmux link-window -ad -s '#{window_id}' -t '%s:'\"", target)},
		{"set-hook", "-t", viewTarget + ":", "client-detached", "kill-session -t " + viewTarget},
	}
	for _, args := range commands {
		if _, err := run(args...); err != nil {
			return err
		}
	}
	attach := []string{"tmux", "-S", socket, "attach-session", "-t", viewTarget}
	command := exe.Local(attach[0]).Command(attach[1:]...)
	command.Env = append(os.Environ(), "TMUX=", "TMUX_PANE=")
	if h.SSH != "" {
		command = runtime.Interactive(h.SSH, attach...)
	}
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}
