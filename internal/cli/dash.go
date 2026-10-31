package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/tui"

	"github.com/emusoi/mia-core/internal/run"
)

func cmdDash(a *app.App, args []string) int {
	if n := takeValue(&args, "--side"); n != "" {
		if !slices.Contains(tui.SideKeys, n) {
			return usageErr("mia dash --side <1|2|3|4>")
		}
		startSide = n
	}
	if take(&args, "--popup") {
		return popup(a)
	}
	pickTo := takeValue(&args, "--pick-to")
	switching := take(&args, "--switch")
	doing := takeValue(&args, "--do")
	name, target := "dashboard", ""
	if len(args) > 0 {
		name = args[0]
		if !strings.HasSuffix(name, "-panel") && name != "dashboard" && !strings.HasPrefix(name, "plugin:") {
			name += "-panel"
		}
	}
	if len(args) > 1 {
		target = args[1]
	}
	defer startServers(a)()
	if doing != "" {
		return do(a, doing, pickTo)
	}
	return browse(a, name, target, pickTo, switching)
}

// cmdSide is `mia side N`: one side of the room alone, as the prefix-m
// menu's small popup shows it.
func cmdSide(a *app.App, args []string) int {
	if len(args) != 1 {
		return usageErr("mia side <1-4|top|left|right|bottom>")
	}
	place := args[0]
	if i := slices.Index(tui.SideKeys, place); i >= 0 {
		place = plugin.Places[i]
	}
	if !slices.Contains(plugin.Places, place) {
		return usageErr("mia side <1-4|top|left|right|bottom>")
	}
	onlySide = place
	defer startServers(a)()
	return browse(a, "dashboard", "", "", false)
}

// onlySide is `mia side`'s side: the dash shows it alone.
var onlySide string

var doable = []string{"new", "pr", "branch"}

func do(a *app.App, action, pickTo string) int {
	if !slices.Contains(doable, action) {
		return usageErr("mia dash --do <" + strings.Join(doable, "|") + ">")
	}
	here := ""
	if record, err := a.RecordOrHere(""); err == nil {
		here = record.Name
	}
	doAction, doHere = action, here
	return browse(a, "dashboard", "", pickTo, false)
}

var doAction, doHere string

// startSide is --side: the side the dash opens with the cursor in.
var startSide string

const listingTTL = 10 * time.Second

func browse(a *app.App, name, target, pickTo string, switching bool) int {
	session.UseControl()
	var listing listingCache
	reload := func() (panel.Panel, error) {
		if name != "dashboard" {
			return panelNamed(a, name, target)
		}
		cached, err := listing.get(func() ([]app.Listing, error) {
			if cfg, err := config.Load(a.MiaDir); err == nil {
				a.Config = cfg
			}
			others := make(chan []app.Listing, 1)
			go func() { others <- a.RemoteListings() }()
			listings, err := a.List()
			if err != nil {
				return nil, err
			}
			return append(listings, <-others...), nil
		})
		if err != nil {
			return panel.Panel{}, err
		}
		all := cached
		cached = slices.DeleteFunc(slices.Clone(all), func(l app.Listing) bool { return l.Machine != "" })
		plugins, _ := enabledPlugins(a)
		if onlySide != "" {
			p := panel.Panel{Version: panel.Version, Room: pluginRoom(a, plugins, cached)}
			p.Bind(a.Config.KeyMap())
			return p, nil
		}
		room := make(chan *panel.Room, 1)
		go func() { room <- pluginRoom(a, plugins, cached) }()
		p := withChoices(panel.Dashboard(all, a.Config.Prefix, pluginRows(a, plugins, cached)))
		namesBase(p, a.Config.Base, cached)
		p.Room = <-room
		p.Bind(a.Config.KeyMap())
		return p, nil
	}
	hooks := hooksFor(a, name, target)
	hooks.Drafts = loadDrafts(a)
	hooks.Keep = func(drafts map[string]string) { saveDrafts(a, drafts) }
	hooks.Switch = switching
	if name == "dashboard" {
		hooks.Do, hooks.Here, hooks.Side, hooks.Only = doAction, doHere, startSide, onlySide
	}
	if pickTo != "" {
		hooks.Picks = func(chosen tui.Chosen) bool { return len(chosen.Argv) > 0 && picks(chosen) }
	}
	hooks.Watch = watchNudge
	hooks.Refresh = func() {
		listing.forget()
		a.ForgetDirtiness()
	}
	title := "Worktrees / " + filepath.Base(a.Root)
	if name != "dashboard" {
		title = strings.TrimSuffix(name, "-panel") + " / " + target
	}
	var open map[string]bool
	hooks.Save = func(sides tui.SideState) { saveSides(a, sides) }
	for {
		hooks.Sides = loadSides(a)
		chosen, err := tui.Open(title, reload, hooks, open)
		open = chosen.Open
		listing.forget()
		trace("tui.Run returned argv=%v panel=%q err=%v", chosen.Argv, chosen.Panel, err)
		if err != nil {
			return fail(err)
		}
		switch {
		case chosen.Panel != "":
			if code := browse(a, chosen.Panel, chosen.RowID, pickTo, false); code != exitOK {
				return code
			}
		case len(chosen.Argv) == 0:
			return exitOK
		case pickTo != "" && picks(chosen):
			markOpened(a, chosen)
			path, err := pick(a, chosen)
			if err != nil {
				return fail(err)
			}
			if err := os.WriteFile(pickTo, []byte(path+"\n"), 0o600); err != nil {
				return fail(err)
			}
			return exitOK
		default:
			markOpened(a, chosen)
			perform(chosen)
			if switching || doAction != "" || onlySide != "" || landsHere(chosen) {
				return exitOK
			}
		}
	}
}

func markOpened(a *app.App, chosen tui.Chosen) {
	argv := chosen.Argv
	opens := len(argv) > 0 && argv[0] == "shell"
	if !opens || chosen.RowID == "" {
		return
	}
	if err := a.MarkSeen(chosen.RowID); err != nil {
		trace("mark seen %s: %v", chosen.RowID, err)
	}
}

func landsHere(chosen tui.Chosen) bool {
	if chosen.Argv[0] != "shell" {
		return false
	}
	here := session.Current()
	return here != "" && here == session.Name(chosen.RowID)
}

func picks(chosen tui.Chosen) bool {
	switch chosen.Argv[0] {
	case "shell", "config":
		return true
	case "new":
		return !slices.ContainsFunc(chosen.Argv, func(arg string) bool {
			return arg == "--stack" || arg == "--worktree" || arg == "--pr"
		})
	}
	return false
}

func pick(a *app.App, chosen tui.Chosen) (string, error) {
	switch chosen.Argv[0] {
	case "config":
		path, err := configPath(a)
		return "edit " + path, err
	}
	if chosen.Argv[0] == "new" {
		branch := chosen.Argv[len(chosen.Argv)-1]
		record, err := a.New(branch)
		if record.Path == "" {
			return "", err
		}
		return record.Path, nil
	}
	resolver, err := a.Resolver()
	if err != nil {
		return "", err
	}
	return resolver.Worktree(chosen.RowID)
}

func perform(chosen tui.Chosen) {
	action, argv := chosen.Action, chosen.Argv
	fmt.Fprint(os.Stderr, "\033[2J\033[H")
	for {
		fmt.Fprintf(os.Stderr, "mia %s\n", joinArgs(argv))
		refusal, err := runChild(argv)
		if err == nil {
			if action.Report {
				pause()
			}
			return
		}
		retry, ok := action.RetryFor(refusal)
		if !ok {
			pause()
			return
		}
		if !ask(retry.ConfirmFor(chosen.RowID)) {
			return
		}
		action, argv = retry, retry.Command(chosen.RowID)
	}
}

func runChild(argv []string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	var said bytes.Buffer
	command := run.Local(self).Command(argv...)
	command.Stdin, command.Stdout = os.Stdin, os.Stdout
	command.Stderr = io.MultiWriter(os.Stderr, &said)
	err = command.Run()
	return said.String(), err
}

func ask(question string) bool {
	fmt.Fprintf(os.Stderr, "%s  [y/N] ", question)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.TrimSpace(answer)
	return answer == "y" || answer == "Y"
}

func pause() {
	fmt.Fprint(os.Stderr, "\n⏎ to return ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func popup(a *app.App) int {
	if os.Getenv("TMUX") == "" {
		return fail(fmt.Errorf("a popup needs tmux — run `mia dash` instead, or `mia help tmux` for the key to bind"))
	}
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	here, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	width, height := a.Config.Popup.Size()
	if err := run.Local("tmux").Interactive("display-popup", "-B", "-E", "-e", "MIA_POPUP=1", "-e", "MIA_TMUX_PANE="+os.Getenv("TMUX_PANE"),
		"-d", here, "-w", width, "-h", height, run.ShellJoin(dashArgv(self))); err != nil {
		return fail(err)
	}
	return exitOK
}

func dashArgv(self string) []string {
	if startSide != "" {
		return []string{self, "dash", "--side", startSide}
	}
	return []string{self, "dash"}
}

func joinArgs(argv []string) string {
	return strings.Join(argv, " ")
}

func hooksFor(a *app.App, name, target string) tui.Hooks {
	hooks := tui.Hooks{Close: func() { session.Here().CloseHover(os.Getenv("TMUX_PANE")) }}
	if name == "blocks-panel" {
		hooks = windowHooks(a, target)
	}
	hooks.Perform = func(chosen tui.Chosen) tui.Outcome {
		markOpened(a, chosen)
		return quietly(chosen)
	}
	if os.Getenv("TMUX_PANE") != "" {
		hooks.Popup = func(chosen tui.Chosen) error {
			markOpened(a, chosen)
			title := fmt.Sprintf(" mia %s — prefix d or exit returns to the list ", strings.Join(chosen.Argv, " "))
			trace("popup %v", chosen.Argv)
			width, height := a.Config.Popup.Size()
			var x, y string
			if at := chosen.Action.Place; at != nil {
				width, height, x, y = at.Width, at.Height, at.X, at.Y
			}
			return session.Popup(chosen.Argv, os.Getenv("TMUX_PANE"), title, width, height, x, y)
		}
	}
	if os.Getenv("TMUX_PANE") != "" {
		hooks.Show = func(open plugin.Open) error {
			return session.Show(open.Popup, os.Getenv("TMUX_PANE"), a.Root, open.Title, open.Interactive)
		}
	}
	hooks.ShowIn = func(open plugin.Open) *exec.Cmd {
		return session.ShowHere(open.Popup, a.Root)
	}
	hooks.Load = func(name, target string) (panel.Panel, tui.Reload, tui.Hooks, error) {
		p, err := panelNamed(a, name, target)
		if err != nil {
			return panel.Panel{}, nil, tui.Hooks{}, err
		}
		reload := func() (panel.Panel, error) { return panelNamed(a, name, target) }
		return p, reload, hooksFor(a, name, target), nil
	}
	return hooks
}

// namesBase says in new's and + 's labels where a new branch starts: the
// configured base, else the branch the main checkout is on, as mia new does.
func namesBase(p panel.Panel, base string, listings []app.Listing) {
	main := ""
	for _, l := range listings {
		if l.Main {
			main = l.Branch
		}
	}
	if action, ok := p.Actions["base"]; ok {
		action.Choices = slices.Compact(slices.DeleteFunc([]string{base, main}, func(b string) bool { return b == "" }))
		p.Actions["base"] = action
	}
	if base == "" {
		base = main
	}
	if base == "" {
		return
	}
	for id, label := range map[string]string{"new": "new worktree from " + base, "branch": "new worktree from " + base + ", and go there"} {
		if action, ok := p.Actions[id]; ok {
			action.Label = label
			action.Help = "a new branch from " + base + ", or a branch that exists, in a worktree of its own with its setup run"
			p.Actions[id] = action
		}
	}
}

// listingCache is the dashboard's listing between reloads. Reloads run off
// the UI's goroutine, a tick's and a nudge's at once at times: one lists at
// a time, each gets its own copy, and forget only marks the listing stale —
// clearing it under a reload's feet once drew a dashboard of no worktrees.
type listingCache struct {
	mu       sync.Mutex
	listings []app.Listing
	at       time.Time
	stale    atomic.Bool
}

func (c *listingCache) get(list func() ([]app.Listing, error)) ([]app.Listing, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stale.Swap(false) || c.listings == nil || time.Since(c.at) > listingTTL {
		listings, err := list()
		if err != nil {
			return nil, err
		}
		c.listings, c.at = listings, time.Now()
	}
	return c.listings, nil
}

// forget makes the next get list again; it never waits on one listing.
func (c *listingCache) forget() { c.stale.Store(true) }

func quietly(chosen tui.Chosen) tui.Outcome {
	self, err := os.Executable()
	if err != nil {
		return tui.Outcome{Err: err}
	}
	trace("perform %v", chosen.Argv)
	var said bytes.Buffer
	command := run.Local(self).Command(chosen.Argv...)
	command.Stdout, command.Stderr = &said, &said
	err = command.Run()
	outcome := tui.Outcome{Output: said.String(), Err: err}
	if err != nil {
		if retry, ok := chosen.Action.RetryFor(said.String()); ok {
			outcome.Retry = &retry
		}
	}
	return outcome
}

func windowHooks(a *app.App, target string) tui.Hooks {
	record, err := a.RecordOrHere(target)
	if err != nil {
		return tui.Hooks{}
	}
	host, err := a.SessionHostOf(record)
	if err != nil {
		return tui.Hooks{}
	}
	return tui.Hooks{
		Peek: func(window, _ string) (string, []string, bool) {
			text, err := host.Capture(record.Name, window, true)
			if err != nil {
				return "", nil, false
			}
			return window, panel.LastLines(text, 40), true
		},
		Close: func() { session.Here().CloseHover(os.Getenv("TMUX_PANE")) },
	}
}

func trace(format string, args ...any) {
	path := os.Getenv("MIA_LOG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"\n", args...)
}

func draftsPath(a *app.App) string { return filepath.Join(a.MiaDir, "cache", "drafts.json") }

func loadDrafts(a *app.App) map[string]string {
	drafts := map[string]string{}
	if data, err := os.ReadFile(draftsPath(a)); err == nil {
		_ = json.Unmarshal(data, &drafts)
	}
	return drafts
}

func saveDrafts(a *app.App, drafts map[string]string) {
	data, err := json.Marshal(drafts)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(draftsPath(a)), 0o755)
	if err := os.WriteFile(draftsPath(a), data, 0o600); err != nil {
		trace("save drafts: %v", err)
	}
}

func sidesPath(a *app.App) string { return filepath.Join(a.MiaDir, "dash-sides.json") }

func loadSides(a *app.App) tui.SideState {
	var sides tui.SideState
	if data, err := os.ReadFile(sidesPath(a)); err == nil {
		_ = json.Unmarshal(data, &sides)
	}
	return sides
}

func saveSides(a *app.App, sides tui.SideState) {
	data, err := json.Marshal(sides)
	if err != nil {
		return
	}
	_ = os.MkdirAll(a.MiaDir, 0o755)
	if err := os.WriteFile(sidesPath(a), data, 0o644); err != nil {
		trace("save sides: %v", err)
	}
}
