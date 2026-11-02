package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/radar"
	"github.com/emusoi/mia-core/internal/run"
)

// cmdTmuxMenu is mia tmux menu <client>: prefix m. tmux's own menu, built
// each time it is pressed, so its counts and plugins are as they are now.
func cmdTmuxMenu(a *app.App, args []string) int {
	if len(args) != 1 {
		return usageErr("mia tmux menu <client>")
	}
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	local, err := a.List()
	if err != nil {
		return fail(err)
	}
	here := ""
	if record, err := a.RecordOrHere(""); err == nil {
		here = record.Name
	}
	var plugins []string
	if enabled, err := enabledPlugins(a); err == nil {
		for _, p := range enabled {
			if len(p.Manifest.Verbs) > 0 {
				plugins = append(plugins, p.Manifest.Verbs[0].Name)
			}
		}
	}
	argv := menuItems(run.ShellJoin([]string{self}), filepath.Base(a.Root), here, local, plugins)
	if out, err := run.Local("tmux").Combined(append([]string{"display-menu", "-c", args[0]}, argv...)...); err != nil {
		return fail(fmt.Errorf("tmux display-menu: %s", strings.TrimSpace(out)))
	}
	return exitOK
}

// menuItems is display-menu's title and items, by threes — name, key,
// command — with "" between sections. A name starting with - is greyed out.
func menuItems(mia, repo, here string, local []app.Listing, plugins []string) []string {
	idle, others, clean := 0, 0, 0
	for _, l := range local {
		switch {
		case !l.Main && !l.Adopted:
			others++
			if !l.Dirty {
				clean++
			}
		case radar.Idle(l, here):
			idle++
		}
	}
	popup := func(w, h, title, command string) string {
		return fmt.Sprintf(`display-popup -E -d "#{pane_current_path}" -w %s -h %s -T " %s " "%s"`, w, h, title, command)
	}
	items := []string{"-T", " mia · " + repo + " ",
		"radar", "Space", popup("72", "60%", "mia", mia+" radar"),
		"dash", "g", popup("100%", "100%", "mia dash", mia+" dash"),
		"",
		"new worktree", "n", popup("110", "14", "new worktree", mia+" radar --new"),
		"",
	}
	cleanup := func(label string, n int, key, title, command string) {
		if n == 0 {
			label = "-" + label
		}
		items = append(items, label, key, popup("100", "60%", title, command))
	}
	cleanup(fmt.Sprintf("clean up idle  %d", idle), idle, "c", "idle", mia+" gc --ask")
	cleanup(fmt.Sprintf("clean up not mia's  %d of %d", clean, others), clean, "o", "not mia's", mia+" gc --others --ask")
	taken := map[rune]bool{'g': true, 'n': true, 'c': true, 'o': true}
	for i, verb := range plugins {
		if i == 0 {
			items = append(items, "")
		}
		key := ""
		for _, r := range verb {
			if r >= 'a' && r <= 'z' && !taken[r] {
				taken[r], key = true, string(r)
				break
			}
		}
		// shortcut: no -E, so a verb that prints and exits stays up to be read
		items = append(items, verb, key, fmt.Sprintf(`display-popup -d "#{pane_current_path}" -w 80%% -h 70%% -T " %s " "%s %s"`, verb, mia, verb))
	}
	return items
}

// askYes asks on the terminal and is true only for y.
func askYes(question string) bool {
	fmt.Print(question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line) == "y"
}

// pressKey keeps a popup's last words up until ⏎.
func pressKey() {
	fmt.Print("\n⏎ close")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
