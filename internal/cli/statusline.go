package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/signal"
)

// pollEvery is how stale the machines' last answers may get before the
// status line has them asked again, in the background.
const pollEvery = 30 * time.Second

// cmdStatusLine is mia status-line [path]: mia's part of the tmux status
// line for the pane at path. tmux runs it every few seconds, so it never
// waits on a machine: it reads their last answers and, when those are old,
// starts one `mia machine poll` to refresh them.
func cmdStatusLine(args []string) int {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	if path == "" {
		path, _ = os.Getwd()
	}
	if err := os.Chdir(path); err != nil {
		return exitOK
	}
	a, err := app.Open(path)
	if err != nil {
		return exitOK // not a repository: mia has nothing to say here
	}
	fmt.Print(statusLine(a, path))
	if a.PollDue(pollEvery) {
		pollInBackground(a.Root)
	}
	return exitOK
}

func statusLine(a *app.App, path string) string {
	machines := a.KnownMachines()
	type said struct {
		where string
		signal.Signal
	}
	var all []said
	if local, err := a.Signals.Load(); err == nil {
		for _, one := range local {
			all = append(all, said{one.Worktree, one})
		}
	}
	for _, state := range machines {
		for _, w := range state.Worktrees {
			for _, one := range w.Signals {
				all = append(all, said{w.Name + "@" + state.Name, one})
			}
		}
	}
	var parts []string
	if len(all) > 0 {
		best := 0
		for i, one := range all {
			if one.Rank < all[best].Rank || (one.Rank == all[best].Rank && one.At.After(all[best].At)) {
				best = i
			}
		}
		top := all[best]
		text := top.where + " " + top.Kind
		if top.Text != "" {
			text += " · " + fit(top.Text, 40)
		}
		lead := tmuxStyled("● "+text, top.Style)
		if more := len(all) - 1; more > 0 {
			lead += " " + tmuxStyled(fmt.Sprintf("+%d", more), top.Style)
		}
		parts = append(parts, lead)
	}
	if record, err := a.RecordOrHere(""); err == nil {
		branch := git.CurrentBranch(path)
		here := tmuxStyled(record.Name, "bold") + " " + tmuxStyled(branch, "dim")
		if layers, err := a.Layers(path, branch); err == nil && len(layers) > 1 {
			for i, layer := range layers {
				if layer.Current {
					here += fmt.Sprintf(" %d/%d", i+1, len(layers))
				}
			}
			for _, layer := range layers {
				if layer.NeedsRestack {
					here += " " + tmuxStyled("⟳", "yellow")
					break
				}
			}
		}
		if n := git.Changes(path); n > 0 {
			here += " " + tmuxStyled(fmt.Sprintf("±%d", n), "blue")
		}
		parts = append(parts, here)
	}
	if len(machines) > 1 {
		var dots []string
		for _, state := range machines[1:] {
			if state.Answers {
				dots = append(dots, tmuxStyled(state.Name, "dim")+" "+tmuxStyled("●", "green"))
			} else {
				dots = append(dots, tmuxStyled(state.Name+" ○", "red"))
			}
		}
		parts = append(parts, strings.Join(dots, " "))
	}
	return strings.Join(parts, tmuxStyled("  ·  ", "dim"))
}

// tmuxStyled is text in a setter's style, as tmux formats it: colours and
// bold, dim, italics, underline. A # in the text is doubled, so the text
// can never be read as formatting.
func tmuxStyled(text, style string) string {
	var attrs []string
	for _, word := range strings.Fields(style) {
		switch word {
		case "bold", "dim", "italics", "underscore", "reverse":
			attrs = append(attrs, word)
		case "underline":
			attrs = append(attrs, "underscore")
		case "red", "green", "yellow", "blue", "magenta", "cyan", "white", "black":
			attrs = append(attrs, "fg="+word)
		}
	}
	text = strings.ReplaceAll(text, "#", "##")
	if len(attrs) == 0 {
		return text
	}
	return "#[" + strings.Join(attrs, ",") + "]" + text + "#[default]"
}

func fit(text string, width int) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) <= width {
		return string(runes)
	}
	return string(runes[:width-1]) + "…"
}

// pollInBackground starts `mia machine poll` in the repository, cut loose
// from the status line so tmux never waits on it.
func pollInBackground(root string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	poll := exec.Command(self, "machine", "poll")
	poll.Dir = root
	poll.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if poll.Start() == nil {
		go poll.Process.Release()
	}
}
