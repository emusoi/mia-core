package session

import (
	"fmt"
	"os"
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
}

// quietStatus is the right of a mia session's status line: nothing to see,
// but mia runs there every few seconds, which is what has the session's
// windows remembered and the machines asked while it is in front. Left
// empty, a session was remembered only when the radar happened to open.
// shortcut: here only; a session on a machine is remembered when its own
// tmux shows mia's status line.
func quietStatus(self string, remote bool) string {
	if remote || self == "" || strings.HasSuffix(self, ".test") {
		return ""
	}
	return "#(" + run.ShellJoin([]string{self, "status-line", "--quiet"}) + ` "#{pane_current_path}")`
}

var miaWindowStyle = [][2]string{
	{"window-status-style", "fg=default,bg=default"},
	{"window-status-current-style", "fg=default,bg=default"},
	{"window-status-format", " #[fg=colour8]#I:#W#F#[default] "},
	{"window-status-current-format", " #[fg=default,bold]#I:#W#F#[default] "},
	{"window-status-separator", " "},
	{"pane-border-style", "fg=colour8,bg=default"},
	{"pane-active-border-style", "fg=default,bg=default"},
	{"pane-border-format", " " + Title + " "},
	{"automatic-rename-format", Title},
}

// Style changes only this worktree's tmux session and its windows. The terminal's
// ANSI palette supplies the light or dark colours for the status and borders.
// It is one ask of tmux: the session's options and each window's together.
func (h Host) Style(worktree string) error {
	target := Target(worktree, "")
	self, _ := os.Executable()
	style := append(miaSessionStyle[:len(miaSessionStyle):len(miaSessionStyle)], [2]string{"status-right", quietStatus(self, h.SSH != "")})
	args := setting(nil, target, false, style)
	windows, err := h.run("list-windows", "-t", target, "-F", "#{window_id}")
	if err != nil {
		_ = h.setAll(target, args)
		return fmt.Errorf("list windows for %s: %w", worktree, err)
	}
	for _, window := range strings.Fields(windows) {
		args = setting(append(args, ";"), window, true, miaWindowStyle)
	}
	return h.setAll(target, args)
}

func (h Host) styleOptions(target string, window bool, options [][2]string) error {
	return h.setAll(target, setting(nil, target, window, options))
}

// setting is args with the commands that set a target's options, a
// window's or a session's, after them.
func setting(args []string, target string, window bool, options [][2]string) []string {
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
	return args
}

// setAll runs commands that set options, of target as a failure says it.
func (h Host) setAll(target string, args []string) error {
	// The persistent control client quotes every argument, including tmux's
	// command separator. Run these batched commands through tmux directly.
	command := run.Tool{Binary: "tmux", SSH: h.SSH}
	if _, err := command.Combined(args...); err != nil {
		return fmt.Errorf("style tmux target %s: %w", target, err)
	}
	return nil
}
