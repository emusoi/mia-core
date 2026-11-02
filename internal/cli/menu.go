package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/radar"
	"github.com/emusoi/mia-core/internal/run"
)

// cmdTmuxMenu is mia tmux menu <client>: prefix m. tmux's own menu, built
// each time it is pressed, so its counts are as they are now. The plugins'
// verbs it offers are the config's menu, those of them that are on.
func cmdTmuxMenu(a *app.App, args []string) int {
	if len(args) != 1 {
		return usageErr("mia tmux menu <client>")
	}
	self, err := os.Executable()
	if err != nil {
		return tmuxSay(args, err)
	}
	local, err := a.List()
	if err != nil {
		return tmuxSay(args, err)
	}
	here := ""
	if record, err := a.RecordOrHere(""); err == nil {
		here = record.Name
	}
	var plugins []string
	if enabled, err := enabledPlugins(a); err == nil {
		for _, verb := range a.Config.Menu {
			if slices.ContainsFunc(enabled, func(p plugin.Plugin) bool {
				return slices.ContainsFunc(p.Manifest.Verbs, func(v plugin.Verb) bool { return v.Name == verb })
			}) {
				plugins = append(plugins, verb)
			}
		}
	}
	argv := menuItems(run.ShellJoin([]string{self}), filepath.Base(a.Root), here, local, plugins)
	if out, err := run.Local("tmux").Combined(append([]string{"display-menu", "-c", args[0]}, argv...)...); err != nil {
		return tmuxSay(args, fmt.Errorf("tmux display-menu: %s", strings.TrimSpace(out)))
	}
	return exitOK
}

// cmdTmuxNext is mia tmux next <client>: prefix a. The client goes to the
// next worktree here that needs you, after the one it is in, round again
// at the end; best rank first.
func cmdTmuxNext(a *app.App, args []string) int {
	if len(args) != 1 {
		return usageErr("mia tmux next <client>")
	}
	local, err := a.List()
	if err != nil {
		return tmuxSay(args, err)
	}
	here := ""
	if record, err := a.RecordOrHere(""); err == nil {
		here = record.Name
	}
	next := nextNeedingYou(local, here)
	if next == "" {
		_, _ = run.Local("tmux").Combined("display-message", "-c", args[0], "mia: nothing here needs you")
		return exitOK
	}
	if err := a.SwitchClient(next, args[0]); err != nil {
		return tmuxSay(args, err)
	}
	return exitOK
}

// nextNeedingYou is the worktree after here among those that need you, in
// rank order then by name; "" when none does but here.
// ponytail: this computer's alone; a machine's needs the radar's handover.
func nextNeedingYou(local []app.Listing, here string) string {
	var waiting []app.Listing
	for _, l := range local {
		if radar.NeedsYou(l.Signals) {
			waiting = append(waiting, l)
		}
	}
	slices.SortFunc(waiting, func(x, y app.Listing) int {
		if c := x.Signals[0].Rank - y.Signals[0].Rank; c != 0 {
			return c
		}
		return strings.Compare(x.Name, y.Name)
	})
	at := slices.IndexFunc(waiting, func(l app.Listing) bool { return l.Name == here })
	for i := 1; i <= len(waiting); i++ {
		if l := waiting[(at+i+len(waiting))%len(waiting)]; l.Name != here {
			return l.Name
		}
	}
	return ""
}

// tmuxSay shows what went wrong on the client that pressed prefix m: a
// background run-shell has nowhere else to say it.
func tmuxSay(args []string, err error) int {
	if len(args) == 1 {
		_, _ = run.Local("tmux").Combined("display-message", "-c", args[0], "mia: "+err.Error())
	}
	return fail(err)
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
	taken := map[rune]bool{'n': true, 'c': true, 'o': true}
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
		items = append(items, verb, key, popup("100", "30", verb, mia+" "+verb))
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
