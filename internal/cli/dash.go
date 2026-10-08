package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/tui"

	"github.com/emusoi/mia-core/internal/run"
)

func cmdDash(a *app.App, args []string) int {
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

const listingTTL = 10 * time.Second

func browse(a *app.App, name, target, pickTo string, switching bool) int {
	session.UseControl()
	var cached []app.Listing
	var fetched time.Time
	reload := func() (panel.Panel, error) {
		if name != "dashboard" {
			return panelNamed(a, name, target)
		}
		if cached == nil || time.Since(fetched) > listingTTL {
			if cfg, err := config.Load(a.MiaDir); err == nil {
				a.Config = cfg
			}
			listings, err := a.List()
			if err != nil {
				return panel.Panel{}, err
			}
			cached, fetched = listings, time.Now()
		}
		p := withChoices(panel.Dashboard(cached, a.Config.Prefix, pluginRows(a, cached)))
		p.Bind(a.Config.KeyMap())
		return p, nil
	}
	hooks := hooksFor(a, name, target)
	hooks.Drafts = loadDrafts(a)
	hooks.Keep = func(drafts map[string]string) { saveDrafts(a, drafts) }
	hooks.Switch = switching
	if name == "dashboard" {
		hooks.Do, hooks.Here = doAction, doHere
	}
	if pickTo != "" {
		hooks.Picks = func(chosen tui.Chosen) bool { return len(chosen.Argv) > 0 && picks(chosen) }
	}
	hooks.Watch = watchNudge
	hooks.Refresh = func() {
		cached = nil
		a.ForgetDirtiness()
	}
	title := "Worktrees / " + filepath.Base(a.Root)
	if name != "dashboard" {
		title = strings.TrimSuffix(name, "-panel") + " / " + target
	}
	var open map[string]bool
	for {
		chosen, err := tui.Open(title, reload, hooks, open)
		open = chosen.Open
		cached = nil
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
			if switching || doAction != "" || landsHere(chosen) {
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
		"-d", here, "-w", width, "-h", height, self+" dash"); err != nil {
		return fail(err)
	}
	return exitOK
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
			return session.Popup(chosen.Argv, os.Getenv("TMUX_PANE"), title, width, height)
		}
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
