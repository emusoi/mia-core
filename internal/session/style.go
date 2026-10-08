package session

import (
	"fmt"
	"strings"

	"github.com/emusoi/mia-core/internal/run"
)

var miaSessionStyle = [][2]string{
	{"status", "on"},
	{"status-position", "bottom"},
	{"status-justify", "left"},
	{"status-style", "fg=default,bg=default"},
	{"status-left", " #[fg=default,bold]#S#[default]  "},
	{"status-left-length", "40"},
	{"status-right", ""},
}

var miaWindowStyle = [][2]string{
	{"window-status-style", "fg=default,bg=default"},
	{"window-status-current-style", "fg=default,bg=default"},
	{"window-status-format", " #[fg=colour8]#I:#W#F#[default] "},
	{"window-status-current-format", " #[fg=default,bold]#I:#W#F#[default] "},
	{"window-status-separator", " "},
	{"pane-border-style", "fg=colour8,bg=default"},
	{"pane-active-border-style", "fg=default,bg=default"},
	{"pane-border-format", " #{pane_index} · #{b:pane_current_path} "},
}

// Style changes only this worktree's tmux session and its windows. The terminal's
// ANSI palette supplies the light or dark colours for the status and borders.
func (h Host) Style(worktree string) error {
	target := Target(worktree, "")
	if err := h.styleOptions(target, false, miaSessionStyle); err != nil {
		return err
	}
	windows, err := h.run("list-windows", "-t", target, "-F", "#{window_id}")
	if err != nil {
		return fmt.Errorf("list windows for %s: %w", worktree, err)
	}
	for _, window := range strings.Fields(windows) {
		if err := h.styleOptions(window, true, miaWindowStyle); err != nil {
			return err
		}
	}
	return nil
}

func (h Host) styleOptions(target string, window bool, options [][2]string) error {
	var args []string
	for index, option := range options {
		if index > 0 {
			args = append(args, ";")
		}
		args = append(args, "set-option")
		if window {
			args = append(args, "-w")
		}
		args = append(args, "-t", target, option[0], option[1])
	}
	// The persistent control client quotes every argument, including tmux's
	// command separator. Run these batched commands through tmux directly.
	command := run.Tool{Binary: "tmux", SSH: h.SSH}
	if _, err := command.Combined(args...); err != nil {
		return fmt.Errorf("style tmux target %s: %w", target, err)
	}
	return nil
}
